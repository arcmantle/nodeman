package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arcmantle/nodeman/internal/platform"
)

// Config represents the persistent nodeman configuration.
type Config struct {
	ActiveVersion   string            `json:"active_version"`
	PreviousVersion string            `json:"previous_version,omitempty"`
	PackageAuth     PackageAuthConfig `json:"package_auth,omitempty"`
}

// PackageAuthConfig controls package-manager authentication injection.
type PackageAuthConfig struct {
	Enabled    bool                  `json:"enabled,omitempty"`
	Registries []PackageAuthRegistry `json:"registries,omitempty"`
}

// PackageAuthRegistry describes one registry mapping whose token is stored in the OS keychain.
type PackageAuthRegistry struct {
	Registry string `json:"registry"`
	Scope    string `json:"scope,omitempty"`
	Enabled  bool   `json:"enabled,omitempty"`
	Method   string `json:"method,omitempty"`
}

func configPath() (string, error) {
	root, err := platform.RootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

// Load reads the config from disk. Returns a zero-value Config if the file doesn't exist.
func Load() (*Config, error) {
	p, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	if err := platform.EnsureDirs(); err != nil {
		return err
	}

	p, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
