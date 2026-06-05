package auth

import (
	"fmt"
	"log"
	"sync"

	"github.com/boyvinall/ghnotify/internal/config"
)

// Manager owns all authentication state. It is the single source of truth for
// tokens and calls Keychain for persistence.
type Manager struct {
	mu  sync.RWMutex
	cfg *config.AppConfig

	// onChange is called (in a goroutine) whenever auth state changes on a host.
	onChange func(host string)
}

func NewManager(cfg *config.AppConfig) *Manager {
	return &Manager{cfg: cfg}
}

// OnChange registers a callback invoked (in a new goroutine) when a server's
// auth state changes. Only one callback is supported; set before starting.
func (m *Manager) OnChange(fn func(host string)) {
	m.onChange = fn
}

func (m *Manager) notify(host string) {
	if m.onChange != nil {
		go m.onChange(host)
	}
}

// Servers returns a snapshot of configured servers.
func (m *Manager) Servers() []config.ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]config.ServerConfig, len(m.cfg.Servers))
	copy(out, m.cfg.Servers)
	return out
}

// IsAuthenticated returns true if a usable token exists for host.
func (m *Manager) IsAuthenticated(host string) bool {
	tok, _ := m.GetToken(host)
	return tok != ""
}

// GetToken returns the best available token for host: access token first, PAT fallback.
func (m *Manager) GetToken(host string) (string, error) {
	if tok, err := GetSecret(host, KeyAccessToken); err == nil && tok != "" {
		return tok, nil
	}
	return GetSecret(host, KeyPAT)
}

// GetUsername returns the stored GitHub username for host, or empty string.
func (m *Manager) GetUsername(host string) string {
	u, _ := GetSecret(host, KeyUsername)
	return u
}

// LoginOAuth runs the full PKCE OAuth flow for host. Blocks until the browser
// callback completes (or 5-minute timeout). Stores tokens in Keychain.
func (m *Manager) LoginOAuth(host string) error {
	sc := m.serverConfig(host)
	if sc == nil {
		return fmt.Errorf("server %q not in config", host)
	}
	clientSecret, err := GetSecret(host, KeyOAuthClientSecret)
	if err != nil || clientSecret == "" {
		return fmt.Errorf("OAuth client secret not stored for %q", host)
	}

	result, err := StartOAuthFlow(host, sc.ClientID, clientSecret)
	if err != nil {
		return err
	}

	if err := SetSecret(host, KeyAccessToken, result.AccessToken); err != nil {
		return fmt.Errorf("store access token: %w", err)
	}
	if result.RefreshToken != "" {
		if err := SetSecret(host, KeyRefreshToken, result.RefreshToken); err != nil {
			log.Printf("warning: could not store refresh token for %s: %v", host, err)
		}
	}
	if result.Username != "" {
		_ = SetSecret(host, KeyUsername, result.Username)
	}

	m.notify(host)
	return nil
}

// RefreshToken exchanges the stored refresh token for a new access token.
func (m *Manager) RefreshToken(host string) error {
	sc := m.serverConfig(host)
	if sc == nil {
		return fmt.Errorf("server %q not in config", host)
	}
	refreshToken, err := GetSecret(host, KeyRefreshToken)
	if err != nil || refreshToken == "" {
		return fmt.Errorf("no refresh token for %q", host)
	}
	clientSecret, err := GetSecret(host, KeyOAuthClientSecret)
	if err != nil {
		return fmt.Errorf("client secret not found for %q", host)
	}

	result, err := RefreshAccessToken(host, sc.ClientID, clientSecret, refreshToken)
	if err != nil {
		return err
	}

	if err := SetSecret(host, KeyAccessToken, result.AccessToken); err != nil {
		return err
	}
	if result.RefreshToken != "" {
		_ = SetSecret(host, KeyRefreshToken, result.RefreshToken)
	}

	m.notify(host)
	return nil
}

// StorePAT validates token against host's API and then stores it in Keychain.
func (m *Manager) StorePAT(host, token string) error {
	username, err := ValidatePAT(host, token)
	if err != nil {
		return fmt.Errorf("PAT validation failed: %w", err)
	}
	if err := SetSecret(host, KeyPAT, token); err != nil {
		return fmt.Errorf("store PAT: %w", err)
	}
	_ = SetSecret(host, KeyUsername, username)
	m.notify(host)
	return nil
}

// AddServer adds a new server to the config, stores the client secret if OAuth,
// and saves config to disk.
func (m *Manager) AddServer(host string, authType config.AuthType, clientID, clientSecret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.cfg.Servers {
		if s.Host == host {
			return fmt.Errorf("server %q already configured", host)
		}
	}
	m.cfg.Servers = append(m.cfg.Servers, config.ServerConfig{
		Host:     host,
		AuthType: authType,
		ClientID: clientID,
	})
	if authType == config.AuthTypeOAuth && clientSecret != "" {
		if err := SetSecret(host, KeyOAuthClientSecret, clientSecret); err != nil {
			return fmt.Errorf("store client secret: %w", err)
		}
	}
	return m.cfg.Save()
}

// RemoveServer removes a server from config and deletes its Keychain entries.
func (m *Manager) RemoveServer(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	servers := m.cfg.Servers[:0]
	for _, s := range m.cfg.Servers {
		if s.Host != host {
			servers = append(servers, s)
		}
	}
	m.cfg.Servers = servers
	if err := m.cfg.Save(); err != nil {
		return err
	}

	// Best-effort cleanup; ignore errors.
	_ = DeleteSecret(host, KeyAccessToken)
	_ = DeleteSecret(host, KeyRefreshToken)
	_ = DeleteSecret(host, KeyPAT)
	_ = DeleteSecret(host, KeyOAuthClientSecret)
	_ = DeleteSecret(host, KeyUsername)
	return nil
}

// Logout removes all stored tokens for host.
func (m *Manager) Logout(host string) error {
	_ = DeleteSecret(host, KeyAccessToken)
	_ = DeleteSecret(host, KeyRefreshToken)
	_ = DeleteSecret(host, KeyPAT)
	_ = DeleteSecret(host, KeyUsername)
	m.notify(host)
	return nil
}

func (m *Manager) serverConfig(host string) *config.ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.cfg.Servers {
		if m.cfg.Servers[i].Host == host {
			return &m.cfg.Servers[i]
		}
	}
	return nil
}
