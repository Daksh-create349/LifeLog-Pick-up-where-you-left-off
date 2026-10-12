package cli_test

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lifelog/lifelog/internal/cli"
	"github.com/lifelog/lifelog/internal/ipc"
	"github.com/lifelog/lifelog/internal/paths"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ll_cli_*")
	if err != nil {
		t.Fatalf("failed to create short temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestRun_Daemon_Validation(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// 1. Unexpected positional argument
	code := cli.Run([]string{"daemon", "unexpected_arg"}, &stdout, &stderr)
	if code != cli.ExitUsageError {
		t.Fatalf("expected ExitUsageError (%d), got %d", cli.ExitUsageError, code)
	}
	if !strings.Contains(stderr.String(), "unexpected positional argument") {
		t.Errorf("stderr missing expected error message: %s", stderr.String())
	}

	// 2. Help flag
	stdout.Reset()
	stderr.Reset()
	code = cli.Run([]string{"daemon", "--help"}, &stdout, &stderr)
	if code != cli.ExitUsageError && code != cli.ExitSuccess {
		t.Fatalf("expected 0 or 2 on --help, got %d", code)
	}
}

func TestRun_Daemon_Lifecycle(t *testing.T) {
	tempDataDir := t.TempDir()
	t.Setenv(paths.EnvDataDir, tempDataDir)

	token := "cli_daemon_secret_token_12345678"
	t.Setenv(cli.EnvIPCToken, token)

	sockDir := shortTempDir(t)
	sockPath := filepath.Join(sockDir, "daemon.sock")

	var stdout, stderr bytes.Buffer
	doneCh := make(chan int, 1)

	go func() {
		code := cli.Run([]string{"daemon", "--socket=" + sockPath}, &stdout, &stderr)
		doneCh <- code
	}()

	// Wait up to 3 seconds for the daemon socket to be listening
	deadline := time.Now().Add(3 * time.Second)
	var ready bool
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sockPath)
		if err == nil {
			_ = conn.Close()
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !ready {
		t.Fatalf("daemon socket %s did not become ready in time: stderr=%s", sockPath, stderr.String())
	}

	// Connect client and authenticate
	client, err := ipc.Dial(sockPath)
	if err != nil {
		t.Fatalf("failed to dial daemon socket: %v", err)
	}

	authRes, err := client.Auth(token)
	if err != nil {
		client.Close()
		t.Fatalf("auth handshake failed: %v", err)
	}
	if !authRes.Authenticated {
		client.Close()
		t.Fatalf("expected Authenticated: true")
	}

	// Verify health
	health, err := client.Health()
	if err != nil {
		client.Close()
		t.Fatalf("health check failed: %v", err)
	}
	if health.DaemonStatus != "healthy" {
		t.Errorf("expected daemon status 'healthy', got %s", health.DaemonStatus)
	}
	client.Close()

	// Send SIGTERM to current process to trigger graceful shutdown
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)

	select {
	case code := <-doneCh:
		if code != cli.ExitSuccess {
			t.Fatalf("expected daemon to exit with ExitSuccess (0), got %d, stderr: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon did not shut down within 5 seconds")
	}

	// Verify socket was cleaned up
	if _, err := os.Lstat(sockPath); !os.IsNotExist(err) {
		t.Errorf("expected socket %s to be deleted on shutdown", sockPath)
	}
}
