package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type AuthType string

const (
	AuthTypeOAuth AuthType = "oauth"
	AuthTypePAT   AuthType = "pat"
)

type ServerConfig struct {
	Host     string   `toml:"host"`
	AuthType AuthType `toml:"auth_type"`
	ClientID string   `toml:"client_id,omitempty"`
}

type NotificationConfig struct {
	NewReviewRequests bool `toml:"new_review_requests"`
	PRApproved        bool `toml:"pr_approved"`
	PRMerged          bool `toml:"pr_merged"`
	CIStatusChange    bool `toml:"ci_status_change"`
	NewComments       bool `toml:"new_comments"`
}

type AppConfig struct {
	PollInterval     string             `toml:"poll_interval"`
	MaxPRsPerSection int                `toml:"max_prs_per_section"`
	Notifications    NotificationConfig `toml:"notifications"`
	Servers          []ServerConfig     `toml:"servers"`
}

func Default() *AppConfig {
	return &AppConfig{
		PollInterval:     "60s",
		MaxPRsPerSection: 20,
		Notifications: NotificationConfig{
			NewReviewRequests: true,
			PRApproved:        true,
			PRMerged:          true,
			CIStatusChange:    false,
			NewComments:       false,
		},
	}
}

func Load() (*AppConfig, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if saveErr := cfg.save(path); saveErr != nil {
			log.Printf("warning: could not write default config: %v", saveErr)
		}
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *AppConfig) Save() error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	return c.save(path)
}

func (c *AppConfig) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
