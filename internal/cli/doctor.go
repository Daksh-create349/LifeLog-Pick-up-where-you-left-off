package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/paths"
	_ "modernc.org/sqlite"
)

type checkStatus string

const (
	statusPass checkStatus = "PASS"
	statusWarn checkStatus = "WARN"
	statusFail checkStatus = "FAIL"
)

type diagnosticItem struct {
	Name    string
	Status  checkStatus
	Message string
}

func runDoctor(stdout, stderr io.Writer) int {
	var items []diagnosticItem

	addCheck := func(name string, status checkStatus, msg string) {
		items = append(items, diagnosticItem{Name: name, Status: status, Message: msg})
	}

	// 1. Platform compatibility
	if runtime.GOOS == "darwin" {
		addCheck("Platform Compatibility", statusPass, fmt.Sprintf("macOS (%s)", runtime.GOARCH))
	} else {
		addCheck("Platform Compatibility", statusWarn, fmt.Sprintf("Non-macOS platform: %s (%s)", runtime.GOOS, runtime.GOARCH))
	}

	// 2. Paths resolution check
	p, err := paths.Resolve()
	if err != nil {
		addCheck("Path Resolution", statusFail, fmt.Sprintf("Failed to resolve paths: %v", err))
		printDoctorReport(stdout, items)
		return ExitGeneralErr
	}
	addCheck("Path Resolution", statusPass, fmt.Sprintf("Data directory: %s", p.DataDir))

	// 3. Data directory check (read-only)
	dirInfo, err := os.Stat(p.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			addCheck("Data Directory", statusWarn, "Directory does not exist yet (will be initialized on first run)")
		} else {
			addCheck("Data Directory", statusFail, fmt.Sprintf("Cannot access directory: %v", err))
		}
	} else {
		if !dirInfo.IsDir() {
			addCheck("Data Directory", statusFail, "Target path is a file, not a directory")
		} else {
			perm := dirInfo.Mode().Perm()
			if perm&0077 != 0 {
				addCheck("Data Directory", statusWarn, fmt.Sprintf("Permissions %04o are broader than recommended 0700", perm))
			} else {
				addCheck("Data Directory", statusPass, fmt.Sprintf("Exists with secure permissions (%04o)", perm))
			}
		}
	}

	// 4. Config check (read-only)
	cfgData, err := os.ReadFile(p.Config)
	if err != nil {
		if os.IsNotExist(err) {
			addCheck("Configuration File", statusWarn, "Config file does not exist yet (default will be created on first run)")
		} else {
			addCheck("Configuration File", statusFail, fmt.Sprintf("Cannot read config: %v", err))
		}
	} else {
		var cfg config.Config
		if err := json.Unmarshal(cfgData, &cfg); err != nil {
			addCheck("Configuration File", statusFail, fmt.Sprintf("Malformed JSON in config: %v", err))
		} else if err := cfg.Validate(); err != nil {
			addCheck("Configuration File", statusFail, fmt.Sprintf("Invalid configuration: %v", err))
		} else {
			addCheck("Configuration File", statusPass, fmt.Sprintf("Valid (version %d)", cfg.Version))
		}
	}

	// 5. Database check (read-only)
	dbInfo, err := os.Stat(p.DB)
	if err != nil {
		if os.IsNotExist(err) {
			addCheck("Database File", statusWarn, "Database does not exist yet (will be initialized on first run)")
		} else {
			addCheck("Database File", statusFail, fmt.Sprintf("Cannot access database file: %v", err))
		}
	} else {
		perm := dbInfo.Mode().Perm()
		if perm&0077 != 0 {
			addCheck("Database File Permissions", statusWarn, fmt.Sprintf("Permissions %04o are broader than recommended 0600", perm))
		} else {
			addCheck("Database File Permissions", statusPass, fmt.Sprintf("Secure permissions (%04o)", perm))
		}

		// Read-only database connection check using query_only pragma
		roDSN := fmt.Sprintf("%s?_pragma=query_only(ON)&_pragma=busy_timeout(3000)", p.DB)
		sqlDB, err := sql.Open("sqlite", roDSN)
		if err != nil {
			addCheck("Database Integrity", statusFail, fmt.Sprintf("Cannot open database in read-only mode: %v", err))
		} else {
			defer sqlDB.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			var integrity string
			if err := sqlDB.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&integrity); err != nil {
				addCheck("Database Integrity", statusFail, fmt.Sprintf("Integrity check query failed: %v", err))
			} else if integrity != "ok" {
				addCheck("Database Integrity", statusFail, fmt.Sprintf("Integrity check failed: %s", integrity))
			} else {
				var version int
				_ = sqlDB.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
				addCheck("Database Integrity", statusPass, fmt.Sprintf("Integrity OK (schema version %d)", version))
			}
		}
	}

	printDoctorReport(stdout, items)

	for _, item := range items {
		if item.Status == statusFail {
			return ExitGeneralErr
		}
	}
	return ExitSuccess
}

func printDoctorReport(w io.Writer, items []diagnosticItem) {
	fmt.Fprintf(w, "LIFELOG Doctor Diagnostics\n")
	fmt.Fprintf(w, "==========================\n")
	passCount, warnCount, failCount := 0, 0, 0
	for _, it := range items {
		fmt.Fprintf(w, "[%-4s] %-28s %s\n", it.Status, it.Name, it.Message)
		switch it.Status {
		case statusPass:
			passCount++
		case statusWarn:
			warnCount++
		case statusFail:
			failCount++
		}
	}
	fmt.Fprintf(w, "\nSummary: %d passed, %d warning(s), %d failure(s)\n", passCount, warnCount, failCount)
}
