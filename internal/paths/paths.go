package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// EnvDataDir is the environment variable used to override the application data root.
	EnvDataDir = "LIFELOG_DATA_DIR"

	// AppDirName is the application directory name used under Application Support.
	AppDirName = "LifeLog"

	// Standard file names.
	ConfigFile   = "config.json"
	DatabaseFile = "lifelog.db"
	LogsDirName  = "logs"
	SocketFile   = "lifelog.sock"

	// Restrictive file and directory permissions (owner-only access).
	DirPerm  = 0700
	FilePerm = 0600
)

// Paths holds the resolved filesystem paths for LIFELOG data and state.
type Paths struct {
	DataDir string
	Config  string
	DB      string
	Logs    string
	Socket  string
}

// Resolve resolves the LIFELOG paths using the ambient system environment.
func Resolve() (*Paths, error) {
	return ResolveWithEnv(os.Getenv)
}

// ResolveWithEnv resolves paths using a provided environment lookup function.
func ResolveWithEnv(getenv func(string) string) (*Paths, error) {
	var dataDir string

	if custom := getenv(EnvDataDir); custom != "" {
		dataDir = filepath.Clean(custom)
	} else {
		resolved, err := defaultDataDir()
		if err != nil {
			return nil, fmt.Errorf("resolving data directory: %w", err)
		}
		dataDir = resolved
	}

	return &Paths{
		DataDir: dataDir,
		Config:  filepath.Join(dataDir, ConfigFile),
		DB:      filepath.Join(dataDir, DatabaseFile),
		Logs:    filepath.Join(dataDir, LogsDirName),
		Socket:  filepath.Join(dataDir, SocketFile),
	}, nil
}

func defaultDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("neither config dir (%v) nor home dir (%w) is accessible", err, homeErr)
		}
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support", AppDirName), nil
		}
		return filepath.Join(home, ".config", AppDirName), nil
	}

	return filepath.Join(configDir, AppDirName), nil
}

// EnsureDir verifies that the given directory exists or creates it with restrictive (0700) permissions.
func EnsureDir(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", dir)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("checking directory %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("creating directory %s with permissions %04o: %w", dir, DirPerm, err)
	}
	return nil
}

// Canonical resolves an absolute, cleaned, symlink-resolved path.
// If the path does not exist on disk, it returns the absolute cleaned path.
func Canonical(p string) (string, error) {
	if p == "" {
		return "", errors.New("path cannot be empty")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}
	abs = filepath.Clean(abs)
	if eval, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(eval), nil
	}
	return abs, nil
}

// IsSubpath reports whether child is equal to parent or a subpath within parent.
// Both paths should be canonicalized or absolute before calling.
func IsSubpath(parent, child string) bool {
	p := filepath.Clean(parent)
	c := filepath.Clean(child)
	if p == c {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(p, sep) {
		p += sep
	}
	return strings.HasPrefix(c, p)
}
