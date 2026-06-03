package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrProjectConfigNotFound is returned when no .envy/config.json exists at or above the starting directory.
var ErrProjectConfigNotFound = errors.New("project config not found")

// ProjectConfig is the repository-local .envy/config.json shape.
type ProjectConfig struct {
	APIURL      string `json:"api_url"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
}

// LoadProjectConfig walks upward from cwd looking for .envy/config.json and decodes it.
func LoadProjectConfig(cwd string) (ProjectConfig, error) {
	if cwd == "" {
		return ProjectConfig{}, fmt.Errorf("cwd is required")
	}

	dir, err := filepath.Abs(cwd)
	if err != nil {
		return ProjectConfig{}, err
	}

	for {
		path := filepath.Join(dir, ".envy", "config.json")
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg ProjectConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return ProjectConfig{}, fmt.Errorf("parse %s: %w", path, err)
			}
			return cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return ProjectConfig{}, fmt.Errorf("read %s: %w", path, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ProjectConfig{}, ErrProjectConfigNotFound
}

// LoadCredentials reads the user's ~/.envy/credentials token.
//
// The credentials file must have permission bits exactly 0600. The current file
// format is a plain token; leading and trailing whitespace is ignored.
func LoadCredentials() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("user home directory not found")
	}

	path := filepath.Join(home, ".envy", "credentials")
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("credentials path is a directory: %s", path)
	}
	if info.Mode().Perm() != 0o600 {
		return "", fmt.Errorf("credentials file %s must have permissions 0600", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}
