package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/paths"
)

func newTestPaths(t *testing.T) *paths.Paths {
	t.Helper()
	dir := t.TempDir()
	p, err := paths.ResolveWithEnv(func(k string) string {
		if k == paths.EnvDataDir {
			return dir
		}
		return ""
	})
	if err != nil {
		t.Fatalf("failed to resolve test paths: %v", err)
	}
	return p
}

func TestLoad_DefaultCreation(t *testing.T) {
	p := newTestPaths(t)

	cfg, err := config.Load(p)
	if err != nil {
		t.Fatalf("expected successful default load, got: %v", err)
	}

	if cfg.Version != config.CurrentVersion {
		t.Errorf("got version %d, want %d", cfg.Version, config.CurrentVersion)
	}
	if cfg.DataDir != p.DataDir {
		t.Errorf("got DataDir %q, want %q", cfg.DataDir, p.DataDir)
	}
	if cfg.DatabasePath != p.DB {
		t.Errorf("got DatabasePath %q, want %q", cfg.DatabasePath, p.DB)
	}

	// Verify file was written to disk with restrictive 0600 permissions
	info, err := os.Stat(p.Config)
	if err != nil {
		t.Fatalf("config file was not created on disk: %v", err)
	}
	perm := info.Mode().Perm()
	if perm&0600 != 0600 {
		t.Errorf("expected 0600 permissions, got %04o", perm)
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	p := newTestPaths(t)

	original := &config.Config{
		Version:      config.CurrentVersion,
		DataDir:      p.DataDir,
		DatabasePath: filepath.Join(p.DataDir, "custom.db"),
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := config.Save(p.Config, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := config.Load(p)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.DatabasePath != original.DatabasePath {
		t.Errorf("loaded DB path %q != original %q", loaded.DatabasePath, original.DatabasePath)
	}
	if loaded.Version != original.Version {
		t.Errorf("loaded Version %d != original %d", loaded.Version, original.Version)
	}
}

func TestLoad_MalformedConfig(t *testing.T) {
	p := newTestPaths(t)
	if err := paths.EnsureDir(p.DataDir); err != nil {
		t.Fatal(err)
	}

	brokenContent := []byte(`{ "version": 1, "data_dir": broken JSON`)
	if err := os.WriteFile(p.Config, brokenContent, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(p)
	if err == nil {
		t.Fatalf("expected error on malformed config, got nil")
	}
	if !errors.Is(err, config.ErrMalformedConfig) {
		t.Errorf("expected ErrMalformedConfig, got: %v", err)
	}

	// Ensure file was NOT overwritten
	contentAfter, err := os.ReadFile(p.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(contentAfter) != string(brokenContent) {
		t.Errorf("malformed file was unexpectedly altered")
	}
}

func TestLoad_UnsupportedVersion(t *testing.T) {
	p := newTestPaths(t)
	if err := paths.EnsureDir(p.DataDir); err != nil {
		t.Fatal(err)
	}

	futureContent := []byte(`{
		"version": 999,
		"data_dir": "/tmp/test",
		"database_path": "/tmp/test/db.sqlite"
	}`)
	if err := os.WriteFile(p.Config, futureContent, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(p)
	if err == nil {
		t.Fatalf("expected error on future version, got nil")
	}
	if !errors.Is(err, config.ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion, got: %v", err)
	}
}

func TestConfig_Validation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg: config.Config{
				Version:      1,
				DataDir:      "/data",
				DatabasePath: "/data/db.sqlite",
			},
			wantErr: false,
		},
		{
			name: "zero version",
			cfg: config.Config{
				Version:      0,
				DataDir:      "/data",
				DatabasePath: "/data/db.sqlite",
			},
			wantErr: true,
		},
		{
			name: "empty data_dir",
			cfg: config.Config{
				Version:      1,
				DataDir:      "",
				DatabasePath: "/data/db.sqlite",
			},
			wantErr: true,
		},
		{
			name: "empty db path",
			cfg: config.Config{
				Version:      1,
				DataDir:      "/data",
				DatabasePath: "",
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
