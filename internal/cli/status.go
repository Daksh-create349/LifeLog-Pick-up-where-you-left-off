package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func runStatus(stdout, stderr io.Writer) int {
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

	dbPath := cfg.DatabasePath
	var storageHealth string
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			storageHealth = "Uninitialized (run lifelog doctor or start system)"
		} else {
			storageHealth = fmt.Sprintf("Inaccessible (%v)", err)
		}
	} else {
		db, err := storage.Open(dbPath)
		if err != nil {
			storageHealth = fmt.Sprintf("Error opening (%v)", err)
		} else {
			defer db.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := db.HealthCheck(ctx); err != nil {
				storageHealth = fmt.Sprintf("Unhealthy (%v)", err)
			} else {
				v, _ := db.SchemaVersion(ctx)
				storageHealth = fmt.Sprintf("Healthy (connected, schema v%d)", v)
			}
		}
	}

	fmt.Fprintf(stdout, "LIFELOG Status\n")
	fmt.Fprintf(stdout, "==============\n")
	fmt.Fprintf(stdout, "Version:        %s\n", VersionString())
	fmt.Fprintf(stdout, "OS / Arch:      %s / %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(stdout, "Data Directory: %s\n", p.DataDir)
	fmt.Fprintf(stdout, "Config File:    %s (version %d)\n", p.Config, cfg.Version)
	fmt.Fprintf(stdout, "Database Path:  %s\n", dbPath)
	fmt.Fprintf(stdout, "Storage Health: %s\n", storageHealth)

	return ExitSuccess
}
