package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/lifelog/lifelog/internal/paths"
)

var (
	// ErrSocketPathEmpty is returned when socket path is empty.
	ErrSocketPathEmpty = errors.New("socket path cannot be empty")
	// ErrSocketPathTooLong is returned when socket path exceeds OS sockaddr_un buffer.
	ErrSocketPathTooLong = errors.New("socket path exceeds maximum length for operating system")
	// ErrSocketIsSymlink is returned when target path is a symlink.
	ErrSocketIsSymlink = errors.New("socket path is a symlink")
	// ErrSocketNotASocket is returned when target path is not a socket file.
	ErrSocketNotASocket = errors.New("target path exists and is not a socket")
	// ErrSocketWrongOwner is returned when target socket is owned by another user.
	ErrSocketWrongOwner = errors.New("socket is owned by another user")
	// ErrDaemonAlreadyRunning is returned when an active daemon responds on socket.
	ErrDaemonAlreadyRunning = errors.New("an active LIFELOG daemon is already listening on socket")
)

// PrepareSocket verifies the socket path, validates parent directory permissions (0700),
// guards against deleting symlinks or regular files, checks ownership, and removes stale sockets.
func PrepareSocket(socketPath string) error {
	if socketPath == "" {
		return ErrSocketPathEmpty
	}

	cleanPath := filepath.Clean(socketPath)

	// OS-level Unix domain socket path buffer limit (Darwin: 104 bytes including null byte)
	if runtime.GOOS == "darwin" && len(cleanPath) >= 104 {
		return fmt.Errorf("%w: length %d exceeds macOS limit of 103 characters (%s)", ErrSocketPathTooLong, len(cleanPath), cleanPath)
	}

	dir := filepath.Dir(cleanPath)

	if err := paths.EnsureDir(dir); err != nil {
		return fmt.Errorf("ensuring socket directory %s: %w", dir, err)
	}

	info, err := os.Lstat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking socket path %s: %w", cleanPath, err)
	}

	// Security: Never follow or delete symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrSocketIsSymlink, cleanPath)
	}

	// Security: Never remove regular files, directories, or named pipes
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: %s (mode: %v)", ErrSocketNotASocket, cleanPath, info.Mode())
	}

	// Security: Verify socket ownership matches current process UID
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if stat.Uid != uint32(os.Getuid()) {
			return fmt.Errorf("%w: owned by uid %d (current uid: %d)", ErrSocketWrongOwner, stat.Uid, os.Getuid())
		}
	}

	// Test if an active process is currently serving on the socket
	conn, dialErr := net.DialTimeout("unix", cleanPath, 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("%w: %s", ErrDaemonAlreadyRunning, cleanPath)
	}

	// Socket exists, is verified to be a unix socket owned by current user, and no process is listening.
	// Safe to remove stale socket file.
	if err := os.Remove(cleanPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket %s: %w", cleanPath, err)
	}

	return nil
}

// Listen creates and binds a Unix domain socket listener with restrictive 0600 permissions.
func Listen(socketPath string) (net.Listener, error) {
	if err := PrepareSocket(socketPath); err != nil {
		return nil, err
	}

	cleanPath := filepath.Clean(socketPath)
	listener, err := net.Listen("unix", cleanPath)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %s: %w", cleanPath, err)
	}

	// Ensure restrictive 0600 permissions on the created socket
	if err := os.Chmod(cleanPath, paths.FilePerm); err != nil {
		_ = listener.Close()
		_ = os.Remove(cleanPath)
		return nil, fmt.Errorf("setting permissions on socket %s: %w", cleanPath, err)
	}

	return listener, nil
}
