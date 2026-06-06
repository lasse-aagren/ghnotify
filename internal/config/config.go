package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// NotificationConfig holds user preferences for which PR events should trigger OS notifications.
type NotificationConfig struct {
	NewReviewRequests bool `toml:"new_review_requests"`
	PRApproved        bool `toml:"pr_approved"`
	PRMerged          bool `toml:"pr_merged"`
	CIStatusChange    bool `toml:"ci_status_change"`
	NewComments       bool `toml:"new_comments"`
}

// AppConfig holds all user preferences and app settings.
type AppConfig struct {
	PollInterval     string             `toml:"poll_interval"`
	MaxPRAge         string             `toml:"max_pr_age"`
	MaxPRsPerSection int                `toml:"max_prs_per_section"`
	ExcludeAuthors   []string           `toml:"exclude_authors"`
	Notifications    NotificationConfig `toml:"notifications"`
}

// ParseMaxPRAge returns the configured PR age limit as a duration.
// Returns zero if the value is unset, empty, or parses to zero — meaning no filter.
func (c *AppConfig) ParseMaxPRAge() time.Duration {
	if c.MaxPRAge == "" {
		return 0
	}
	d, err := time.ParseDuration(c.MaxPRAge)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// Default returns an AppConfig with default values for all fields.
func Default() *AppConfig {
	return &AppConfig{
		PollInterval:     "120s",
		MaxPRAge:         "168h", // 1 week
		MaxPRsPerSection: 20,
		ExcludeAuthors:   []string{"app/renovate", "app/dependabot"},
		Notifications: NotificationConfig{
			NewReviewRequests: true,
			PRApproved:        true,
			PRMerged:          true,
			CIStatusChange:    true,
			NewComments:       true,
		},
	}
}

// Load reads the config file from disk and decodes it into an AppConfig struct.
// If the file does not exist, returns a default config and saves it to disk.
func Load() (*AppConfig, error) {
	path, err := GetFilePath()
	if err != nil {
		return nil, err
	}
	slog.Debug("loading config", "path", path)
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		slog.Debug("config file does not exist, creating default", "path", path)
		if saveErr := cfg.save(path); saveErr != nil {
			slog.Warn("could not write default config", "err", saveErr)
		}
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		slog.Warn("could not parse config file", "err", err)
		return nil, err
	}
	return cfg, nil
}

// Save writes the AppConfig to disk at the default config file path.
func (c *AppConfig) Save() error {
	path, err := GetFilePath()
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
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
