package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func appDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "ghnotify"), nil
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "ghnotify"), nil
		}
		return filepath.Join(home, ".config", "ghnotify"), nil
	}
}

func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "ghnotify"), nil
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "ghnotify"), nil
		}
		return filepath.Join(home, ".local", "share", "ghnotify"), nil
	}
}

// ConfigFilePath returns the platform-appropriate path for config.toml.
func ConfigFilePath() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// StateFilePath returns the platform-appropriate path for state.json.
func StateFilePath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// SnoozeFilePath returns the platform-appropriate path for snooze.json.
func SnoozeFilePath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "snooze.json"), nil
}
