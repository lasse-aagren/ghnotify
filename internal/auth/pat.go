package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ValidatePAT calls GET /user on the given host with the token and returns the
// authenticated username. Returns an error if the token is invalid.
func ValidatePAT(host, token string) (username string, err error) {
	url := apiBase(host) + "/user"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("validate PAT: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return "", fmt.Errorf("invalid token (401 Unauthorized)")
	default:
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, host)
	}

	var u struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("decode user response: %w", err)
	}
	return u.Login, nil
}
