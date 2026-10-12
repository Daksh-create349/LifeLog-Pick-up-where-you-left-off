package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lifelog/lifelog/internal/apps"
	"github.com/lifelog/lifelog/internal/ipc"
	"github.com/lifelog/lifelog/internal/paths"
)

// EnvIPCToken is the environment variable used by the parent process to pass the IPC auth token.
const EnvIPCToken = "LIFELOG_IPC_TOKEN"

func runDaemon(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var socketPathFlag string
	fs.StringVar(&socketPathFlag, "socket", "", "path to unix domain socket (defaults to standard application support directory)")

	if err := fs.Parse(args); err != nil {
		return ExitUsageError
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "Error: unexpected positional argument %q. Usage: %s daemon [--socket=<path>]\n", fs.Arg(0), AppName)
		return ExitUsageError
	}

	db, p, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error opening storage: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	// Resolve target socket path
	socketPath := p.Socket
	if socketPathFlag != "" {
		absPath, err := filepath.Abs(filepath.Clean(socketPathFlag))
		if err != nil {
			fmt.Fprintf(stderr, "Error resolving socket path %q: %v\n", socketPathFlag, err)
			return ExitUsageError
		}
		socketPath = absPath
	}

	// Retrieve or generate authentication token
	token := strings.TrimSpace(os.Getenv(EnvIPCToken))
	generatedToken := false
	if token == "" {
		newToken, err := ipc.GenerateAuthToken()
		if err != nil {
			fmt.Fprintf(stderr, "Error generating authentication token: %v\n", err)
			return ExitGeneralErr
		}
		token = newToken
		generatedToken = true
	}

	// Verify parent directory
	dir := filepath.Dir(socketPath)
	if err := paths.EnsureDir(dir); err != nil {
		fmt.Fprintf(stderr, "Error creating socket directory %s: %v\n", dir, err)
		return ExitGeneralErr
	}

	// Initialize App Observer Supervisor
	appSup, err := apps.NewSupervisor(apps.SupervisorConfig{
		DB:      db,
		Enabled: true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Warning: initializing app usage supervisor: %v\n", err)
	}

	// Initialize IPC Server
	server, err := ipc.NewServer(ipc.Config{
		SocketPath:    socketPath,
		AuthToken:     token,
		DB:            db,
		AppSupervisor: appSup,
		Version:       VersionString(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error initializing IPC server on %s: %v\n", socketPath, err)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "LIFELOG Desktop Daemon\n")
	fmt.Fprintf(stdout, "======================\n")
	fmt.Fprintf(stdout, "Version:   %s\n", VersionString())
	fmt.Fprintf(stdout, "Database:  %s\n", db.Path())
	fmt.Fprintf(stdout, "Socket:    %s\n", socketPath)

	if generatedToken {
		fmt.Fprintf(stdout, "Token:     %s\n", token)
		fmt.Fprintf(stdout, "Note:      Ephemeral token generated. Pass in 'auth' handshake.\n")
	} else {
		fmt.Fprintf(stdout, "Auth:      Configured via %s environment variable\n", EnvIPCToken)
	}
	fmt.Fprintf(stdout, "\nLIFELOG daemon is listening for IPC connections... (Press Ctrl+C to stop)\n")

	// Set up signal handler for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start app usage supervisor if enabled
	if appSup != nil {
		if err := appSup.Start(ctx); err != nil {
			fmt.Fprintf(stderr, "Warning: starting app supervisor: %v\n", err)
		}
		defer appSup.Stop()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ctx)
	}()

	select {
	case <-ctx.Done():
		fmt.Fprintf(stdout, "\nShutdown signal received. Stopping LIFELOG daemon...\n")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "Error during server shutdown: %v\n", err)
		}
	case err := <-errCh:
		if err != nil {
			fmt.Fprintf(stderr, "Daemon runtime error: %v\n", err)
			return ExitGeneralErr
		}
	}

	fmt.Fprintf(stdout, "LIFELOG daemon stopped cleanly.\n")
	return ExitSuccess
}
