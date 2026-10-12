package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lifelog/lifelog/internal/collector"
	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func runWatch(args []string, stdout, stderr io.Writer) int {
	var flagArgs []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && (arg == "-interval" || arg == "--interval") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}

	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var intervalStr string
	fs.StringVar(&intervalStr, "interval", "1s", "polling interval (e.g. 500ms, 1s)")

	if err := fs.Parse(flagArgs); err != nil {
		return ExitUsageError
	}

	if len(positional) < 1 {
		fmt.Fprintf(stderr, "Error: watch requires an explicit directory path.\nUsage: %s watch <path> [--interval 1s]\n", AppName)
		return ExitUsageError
	}
	if len(positional) > 1 {
		fmt.Fprintf(stderr, "Error: unexpected additional arguments %v\nUsage: %s watch <path> [--interval 1s]\n", positional[1:], AppName)
		return ExitUsageError
	}

	targetPath := positional[0]

	interval, err := time.ParseDuration(intervalStr)
	if err != nil || interval <= 0 {
		fmt.Fprintf(stderr, "Error: invalid interval duration %q: %v\n", intervalStr, err)
		return ExitUsageError
	}

	p, err := paths.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "Error resolving application paths: %v\n", err)
		return ExitGeneralErr
	}

	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return ExitGeneralErr
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening database: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(stdout, "\nReceived shutdown signal. Stopping watcher gracefully...\n")
		cancel()
	}()

	poller, err := collector.NewFSPoller(collector.PollerConfig{
		Root:     targetPath,
		Interval: interval,
		DataDir:  p.DataDir,
		OnEvents: func(batch []events.Event) error {
			if err := db.RecordEvents(context.Background(), batch); err != nil {
				fmt.Fprintf(stderr, "Error persisting events: %v\n", err)
				return err
			}
			for _, e := range batch {
				fmt.Fprintf(stdout, "[%s] %-8s %s (%d bytes)\n",
					e.CreatedAt.Format("2006-01-02 15:04:05 UTC"),
					strings.ToUpper(string(e.Type)),
					e.Path,
					e.Size,
				)
			}
			return nil
		},
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error initializing watcher: %v\n", err)
		return ExitUsageError
	}

	fmt.Fprintf(stdout, "LIFELOG Filesystem Watcher\n")
	fmt.Fprintf(stdout, "==========================\n")
	fmt.Fprintf(stdout, "Monitored Root: %s\n", poller.MonitoredRoot())
	fmt.Fprintf(stdout, "Strategy:       %s\n", poller.Strategy())
	fmt.Fprintf(stdout, "Interval:       %v\n", interval)
	fmt.Fprintf(stdout, "Database:       %s\n\n", cfg.DatabasePath)
	fmt.Fprintf(stdout, "Watching for changes... (Press Ctrl+C to stop)\n")

	if err := poller.Start(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintf(stderr, "Watcher stopped with error: %v\n", err)
			return ExitGeneralErr
		}
	}

	fmt.Fprintf(stdout, "Watcher stopped.\n")
	return ExitSuccess
}
