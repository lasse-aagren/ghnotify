package auth

import (
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
)

// Server represents a gh-authenticated GitHub host.
type Server struct {
	Host     string
	Username string
}

// Manager discovers hosts and tokens from the gh CLI.
type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

// Servers returns all hosts currently authenticated with gh.
func (m *Manager) Servers() []Server {
	slog.Debug("discovering gh servers")
	out, err := exec.Command("gh", "auth", "status", "--json", "hosts").Output()
	if err != nil {
		slog.Debug("gh auth status failed", "err", err)
		return nil
	}
	servers := parseAuthStatus(out)
	slog.Debug("discovered servers", "count", len(servers))
	return servers
}

// GetToken returns the token for host from the gh CLI.
func (m *Manager) GetToken(host string) (string, error) {
	out, err := exec.Command("gh", "auth", "token", "--hostname", host).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAuthenticated reports whether gh has a usable token for host.
func (m *Manager) IsAuthenticated(host string) bool {
	tok, _ := m.GetToken(host)
	return tok != ""
}

// GetUsername returns the gh username for host, or empty string.
func (m *Manager) GetUsername(host string) string {
	for _, s := range m.Servers() {
		if s.Host == host {
			return s.Username
		}
	}
	return ""
}

// parseAuthStatus parses the JSON output of "gh auth status --json hosts".
func parseAuthStatus(data []byte) []Server {
	var payload struct {
		Hosts map[string][]struct {
			Host   string `json:"host"`
			Login  string `json:"login"`
			Active bool   `json:"active"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Debug("failed to parse gh auth status JSON", "err", err)
		return nil
	}
	var servers []Server
	for _, accounts := range payload.Hosts {
		for _, a := range accounts {
			if a.Active {
				servers = append(servers, Server{Host: a.Host, Username: a.Login})
			}
		}
	}
	return servers
}
