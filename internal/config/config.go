package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lifelog/lifelog/internal/paths"
)

const (
	// CurrentVersion is the active schema version of the configuration file.
	CurrentVersion = 1
)

var (
	// ErrMalformedConfig is returned when the config file cannot be parsed as JSON.
	ErrMalformedConfig = errors.New("malformed configuration file")

	// ErrUnsupportedVersion is returned when the config version is unknown or unsupported.
	ErrUnsupportedVersion = errors.New("unsupported configuration version")
)

// Config represents the persisted application configuration.
type Config struct {
	Version      int       `json:"version"`
	DataDir      string    `json:"data_dir"`
	DatabasePath string    `json:"database_path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Default creates a valid default Config instance.
func Default(p *paths.Paths) *Config {
	now := time.Now().UTC().Truncate(time.Second)
	return &Config{
		Version:      CurrentVersion,
		DataDir:      p.DataDir,
		DatabasePath: p.DB,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Validate checks semantic and structural validity of the Config.
func (c *Config) Validate() error {
	if c.Version < 1 {
		return fmt.Errorf("%w: version %d must be at least 1", ErrUnsupportedVersion, c.Version)
	}
	if c.Version > CurrentVersion {
		return fmt.Errorf("%w: version %d is newer than supported version %d", ErrUnsupportedVersion, c.Version, CurrentVersion)
	}
	if c.DataDir == "" {
		return errors.New("data_dir cannot be empty")
	}
	if c.DatabasePath == "" {
		return errors.New("database_path cannot be empty")
	}
	return nil
}

// Load loads the configuration from disk.
// If the configuration file is missing, it initializes a valid default configuration.
// If the file is malformed or invalid, an explicit error is returned without overwriting user data.
func Load(p *paths.Paths) (*Config, error) {
	if err := paths.EnsureDir(p.DataDir); err != nil {
		return nil, fmt.Errorf("ensuring data directory: %w", err)
	}

	data, err := os.ReadFile(p.Config)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default(p)
			if err := Save(p.Config, cfg); err != nil {
				return nil, fmt.Errorf("writing default config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", p.Config, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %s (%v)", ErrMalformedConfig, p.Config, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config in %s: %w", p.Config, err)
	}

	return &cfg, nil
}

// Save writes the configuration atomically using a temporary file and rename.
func Save(configPath string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("cannot save invalid config: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(configPath)
	if err := paths.EnsureDir(dir); err != nil {
		return fmt.Errorf("ensuring directory for config %s: %w", configPath, err)
	}

	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(paths.FilePerm); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("setting temp file permissions: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing config data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("syncing temp config file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp config file: %w", err)
	}

	if err := os.Rename(tmpName, configPath); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpName, configPath, err)
	}

	cleanup = false
	return nil
}
