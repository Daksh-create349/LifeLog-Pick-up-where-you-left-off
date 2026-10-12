package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/resumption"
)

func runResume(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var limit int
	var windowStr string
	var sinceStr string
	var gapStr string

	fs.IntVar(&limit, "limit", 20, "maximum number of recent records and sessions to display (1-500)")
	fs.StringVar(&windowStr, "window", "7d", "lookback time window (e.g. 24h, 7d, 30d, 0/all for unbounded)")
	fs.StringVar(&sinceStr, "since", "", "filter activity on or after timestamp (RFC3339, YYYY-MM-DD)")
	fs.StringVar(&gapStr, "gap", "30m", "inactivity gap threshold for session clustering (e.g. 15m, 30m, 1h)")

	// Parse flags and positional arguments allowing flexible ordering
	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			if !strings.Contains(arg, "=") && (flagName == "limit" || flagName == "window" || flagName == "since" || flagName == "gap") {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return ExitUsageError
	}

	if len(posArgs) > 1 {
		fmt.Fprintf(stderr, "Error: too many arguments. Usage: %s resume <project-path> [flags]\n", AppName)
		return ExitUsageError
	}

	projectPath := "."
	if len(posArgs) == 1 {
		projectPath = posArgs[0]
	}

	// Validate and parse limit
	if limit < 1 || limit > 500 {
		fmt.Fprintf(stderr, "Error: --limit must be between 1 and 500 (got %d)\n", limit)
		return ExitUsageError
	}

	// Validate and parse inactivity gap
	gapDuration := 30 * time.Minute
	if gapStr != "" {
		d, err := time.ParseDuration(gapStr)
		if err != nil || d <= 0 {
			fmt.Fprintf(stderr, "Error: invalid --gap duration %q (must be positive duration e.g. 15m, 30m, 1h)\n", gapStr)
			return ExitUsageError
		}
		gapDuration = d
	}

	// Parse time window / since
	var sinceTime *time.Time
	var windowDuration time.Duration

	if sinceStr != "" {
		t, err := parseSearchTime(sinceStr, true)
		if err != nil {
			fmt.Fprintf(stderr, "Error parsing --since: %v\n", err)
			return ExitUsageError
		}
		sinceTime = t
	} else if windowStr != "" && windowStr != "0" && strings.ToLower(windowStr) != "all" {
		lower := strings.ToLower(windowStr)
		if strings.HasSuffix(lower, "d") {
			var days int
			if _, err := fmt.Sscanf(lower[:len(lower)-1], "%d", &days); err == nil && days > 0 {
				windowDuration = time.Duration(days) * 24 * time.Hour
			} else {
				fmt.Fprintf(stderr, "Error parsing --window duration %q\n", windowStr)
				return ExitUsageError
			}
		} else {
			d, err := time.ParseDuration(lower)
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "Error parsing --window duration %q (use 24h, 7d, or 0 for all)\n", windowStr)
				return ExitUsageError
			}
			windowDuration = d
		}
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opts := resumption.Options{
		Window:        windowDuration,
		Since:         sinceTime,
		Limit:         limit,
		InactivityGap: gapDuration,
	}

	report, err := resumption.BuildResumptionReport(ctx, db, projectPath, opts)
	if err != nil {
		fmt.Fprintf(stderr, "Error building resumption context: %v\n", err)
		return ExitGeneralErr
	}

	resumption.FormatReport(stdout, report)
	return ExitSuccess
}
