package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func runEvents(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var limit int
	var pathFilter string
	fs.IntVar(&limit, "limit", 20, "maximum number of events to display (1-1000)")
	fs.StringVar(&pathFilter, "path", "", "optional path prefix to filter events")

	if err := fs.Parse(args); err != nil {
		return ExitUsageError
	}

	if limit <= 0 || limit > 1000 {
		fmt.Fprintf(stderr, "Error: limit must be between 1 and 1000 (got %d)\n", limit)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evts, err := db.QueryEvents(ctx, storage.EventFilter{
		PathPrefix: pathFilter,
		Limit:      limit,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error querying events: %v\n", err)
		return ExitGeneralErr
	}

	if len(evts) == 0 {
		fmt.Fprintf(stdout, "No filesystem events recorded yet.\n")
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "LIFELOG Filesystem Events (showing %d)\n", len(evts))
	fmt.Fprintf(stdout, "========================================\n")
	fmt.Fprintf(stdout, "%-24s  %-9s  %-10s  %s\n", "TIMESTAMP (UTC)", "TYPE", "SIZE (B)", "PATH")
	for _, e := range evts {
		fmt.Fprintf(stdout, "%-24s  %-9s  %-10d  %s\n",
			e.CreatedAt.Format("2006-01-02 15:04:05 UTC"),
			e.Type,
			e.Size,
			e.Path,
		)
	}

	return ExitSuccess
}
