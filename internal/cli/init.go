package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func runInit(stdout, stderr io.Writer) int {
	p, err := paths.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "Error resolving application paths: %v\n", err)
		return ExitGeneralErr
	}

	if err := paths.EnsureDir(p.DataDir); err != nil {
		fmt.Fprintf(stderr, "Error creating data directory %s: %v\n", p.DataDir, err)
		return ExitGeneralErr
	}

	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return ExitGeneralErr
	}

	dbExisted := false
	if _, err := os.Stat(cfg.DatabasePath); err == nil {
		dbExisted = true
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening database: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	v, err := db.SchemaVersion(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading schema version: %v\n", err)
		return ExitGeneralErr
	}

	if dbExisted {
		fmt.Fprintf(stdout, "LIFELOG storage already initialized.\nData Directory: %s\nDatabase:       %s (schema v%d)\n",
			p.DataDir, cfg.DatabasePath, v)
	} else {
		fmt.Fprintf(stdout, "LIFELOG storage initialized successfully.\nData Directory: %s\nDatabase:       %s (schema v%d)\n",
			p.DataDir, cfg.DatabasePath, v)
	}

	return ExitSuccess
}
