package paths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lifelog/lifelog/internal/paths"
)

func TestResolveWithEnv_Override(t *testing.T) {
	temp := t.TempDir()
	p, err := paths.ResolveWithEnv(func(key string) string {
		if key == paths.EnvDataDir {
			return temp
		}
		return ""
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.DataDir != temp {
		t.Errorf("got DataDir %q, want %q", p.DataDir, temp)
	}
	if p.Config != filepath.Join(temp, paths.ConfigFile) {
		t.Errorf("got Config %q, want %q", p.Config, filepath.Join(temp, paths.ConfigFile))
	}
	if p.DB != filepath.Join(temp, paths.DatabaseFile) {
		t.Errorf("got DB %q, want %q", p.DB, filepath.Join(temp, paths.DatabaseFile))
	}
	if p.Logs != filepath.Join(temp, paths.LogsDirName) {
		t.Errorf("got Logs %q, want %q", p.Logs, filepath.Join(temp, paths.LogsDirName))
	}
}

func TestResolveWithEnv_Default(t *testing.T) {
	p, err := paths.ResolveWithEnv(func(string) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error resolving default paths: %v", err)
	}

	if runtime.GOOS == "darwin" {
		if !strings.Contains(p.DataDir, filepath.Join("Library", "Application Support", paths.AppDirName)) {
			t.Errorf("macOS default path %q should contain 'Library/Application Support/LifeLog'", p.DataDir)
		}
	} else {
		if !strings.HasSuffix(p.DataDir, paths.AppDirName) {
			t.Errorf("default path %q should end with %q", p.DataDir, paths.AppDirName)
		}
	}
}

func TestEnsureDir(t *testing.T) {
	temp := t.TempDir()
	target := filepath.Join(temp, "nested", "lifelog_dir")

	// 1. Directory creation
	if err := paths.EnsureDir(target); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected a directory at %s", target)
	}

	// Perm check (on Unix systems, check owner permissions)
	perm := info.Mode().Perm()
	if perm&0700 != 0700 {
		t.Errorf("expected permissions %04o, got %04o", 0700, perm)
	}

	// 2. Idempotent check
	if err := paths.EnsureDir(target); err != nil {
		t.Fatalf("re-running EnsureDir failed: %v", err)
	}

	// 3. Error when target is an existing regular file
	filePath := filepath.Join(temp, "regular_file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := paths.EnsureDir(filePath); err == nil {
		t.Fatalf("expected error when EnsureDir called on regular file, got nil")
	}
}

func TestCanonical(t *testing.T) {
	if _, err := paths.Canonical(""); err == nil {
		t.Errorf("expected error on empty path, got nil")
	}

	temp := t.TempDir()
	can, err := paths.Canonical(temp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if can == "" {
		t.Errorf("expected non-empty canonical path")
	}

	// Test non-existent path canonicalization still succeeds via absolute clean path
	nonExistent := filepath.Join(temp, "does_not_exist", "sub")
	canNonExistent, err := paths.Canonical(nonExistent)
	if err != nil {
		t.Fatalf("unexpected error on non-existent path: %v", err)
	}
	if !filepath.IsAbs(canNonExistent) {
		t.Errorf("expected absolute path, got %q", canNonExistent)
	}
}

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/b/c/d.txt", true},
		{"/a/b", "/a/b-other", false},
		{"/a/b", "/a/c", false},
		{"/a/b", "/a", false},
	}

	for _, tt := range tests {
		got := paths.IsSubpath(tt.parent, tt.child)
		if got != tt.want {
			t.Errorf("IsSubpath(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}
