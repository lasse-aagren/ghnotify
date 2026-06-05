package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const oauthScope = "repo read:org"

// OAuthResult holds tokens returned from a completed OAuth flow.
type OAuthResult struct {
	AccessToken  string
	RefreshToken string    // empty if the server doesn't issue refresh tokens
	ExpiresAt    time.Time // zero if token doesn't expire
	Username     string    // populated after a /user call post-exchange
}

// StartOAuthFlow opens the browser, waits for the localhost callback, and
// exchanges the code for tokens. It blocks until the user completes or cancels
// (5-minute timeout).
func StartOAuthFlow(host, clientID, clientSecret string) (*OAuthResult, error) {
	verifier, err := generateVerifier()
	if err != nil {
		return nil, fmt.Errorf("pkce verifier: %w", err)
	}
	challenge := computeChallenge(verifier)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if code := r.URL.Query().Get("code"); code != "" {
			codeCh <- code
			fmt.Fprint(w, "<html><body>Authenticated! You may close this tab.</body></html>") //nolint:errcheck
		} else {
			desc := r.URL.Query().Get("error_description")
			errCh <- fmt.Errorf("oauth denied: %s", desc)
			fmt.Fprint(w, "<html><body>Authentication failed. You may close this tab.</body></html>") //nolint:errcheck
		}
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck
	defer srv.Close() //nolint:errcheck

	authorizeURL := buildAuthorizeURL(host, clientID, challenge, redirectURI)
	if err := exec.Command("open", authorizeURL).Start(); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("oauth timeout: no callback within 5 minutes")
	}

	result, err := exchangeCode(host, clientID, clientSecret, code, verifier, redirectURI)
	if err != nil {
		return nil, err
	}

	// Resolve username for display.
	if u, err := fetchUsername(host, result.AccessToken); err == nil {
		result.Username = u
	}
	return result, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token.
func RefreshAccessToken(host, clientID, clientSecret, refreshToken string) (*OAuthResult, error) {
	body := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return doTokenRequest(host, body)
}

// --- helpers -----------------------------------------------------------------

func generateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func computeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func buildAuthorizeURL(host, clientID, challenge, redirectURI string) string {
	v := url.Values{
		"client_id":             {clientID},
		"scope":                 {oauthScope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"redirect_uri":          {redirectURI},
	}
	return fmt.Sprintf("https://%s/login/oauth/authorize?%s", host, v.Encode())
}

func exchangeCode(host, clientID, clientSecret, code, verifier, redirectURI string) (*OAuthResult, error) {
	body := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	return doTokenRequest(host, body)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func doTokenRequest(host string, body url.Values) (*OAuthResult, error) {
	tokenURL := fmt.Sprintf("https://%s/login/oauth/access_token", host)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	data, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token error %q: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	result := &OAuthResult{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
	}
	if tr.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return result, nil
}

func fetchUsername(host, token string) (string, error) {
	apiURL := apiBase(host) + "/user"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	var u struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func apiBase(host string) string {
	if host == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + host + "/api/v3"
}
