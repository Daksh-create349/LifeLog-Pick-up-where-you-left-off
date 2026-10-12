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

func runCheckpoint(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printCheckpointUsage(stdout)
		return ExitSuccess
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "create":
		return runCheckpointCreate(subargs, stdout, stderr)
	case "list":
		return runCheckpointList(subargs, stdout, stderr)
	case "show":
		return runCheckpointShow(subargs, stdout, stderr)
	case "delete":
		return runCheckpointDelete(subargs, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown checkpoint subcommand: %s\n\n", subcmd)
		printCheckpointUsage(stderr)
		return ExitUsageError
	}
}

func printCheckpointUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s checkpoint <subcommand> [arguments]\n\n", AppName)
	fmt.Fprintf(w, "Subcommands:\n")
	fmt.Fprintf(w, "  create <path> --label <text>   Capture an explicit project resumption checkpoint\n")
	fmt.Fprintf(w, "  list [<path>]                  List saved resumption checkpoints\n")
	fmt.Fprintf(w, "  show <checkpoint-id>           Display historical snapshot for a checkpoint\n")
	fmt.Fprintf(w, "  delete <checkpoint-id> --confirm Delete a saved resumption checkpoint\n")
	fmt.Fprintf(w, "  help                           Show this checkpoint usage information\n")
}

func runCheckpointCreate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("checkpoint create", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var label string
	var limit int
	var windowStr string
	var sinceStr string
	var gapStr string

	fs.StringVar(&label, "label", "", "descriptive label for the checkpoint (required)")
	fs.IntVar(&limit, "limit", 20, "maximum number of items to capture in snapshot (1-500)")
	fs.StringVar(&windowStr, "window", "7d", "lookback time window (e.g. 24h, 7d, 30d, 0/all for unbounded)")
	fs.StringVar(&sinceStr, "since", "", "filter activity on or after timestamp (RFC3339, YYYY-MM-DD)")
	fs.StringVar(&gapStr, "gap", "30m", "inactivity gap threshold for session clustering (e.g. 15m, 30m, 1h)")

	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			if !strings.Contains(arg, "=") && (flagName == "label" || flagName == "limit" || flagName == "window" || flagName == "since" || flagName == "gap") {
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

	if strings.TrimSpace(label) == "" {
		fmt.Fprintf(stderr, "Error: --label is required when creating a checkpoint\n")
		return ExitUsageError
	}

	projectPath := "."
	if len(posArgs) > 0 {
		projectPath = posArgs[0]
	}

	// Validate limit
	if limit < 1 || limit > 500 {
		fmt.Fprintf(stderr, "Error: --limit must be between 1 and 500 (got %d)\n", limit)
		return ExitUsageError
	}

	// Validate gap
	gapDuration := 30 * time.Minute
	if gapStr != "" {
		d, err := time.ParseDuration(gapStr)
		if err != nil || d <= 0 {
			fmt.Fprintf(stderr, "Error: invalid --gap duration %q\n", gapStr)
			return ExitUsageError
		}
		gapDuration = d
	}

	// Parse window / since
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
				fmt.Fprintf(stderr, "Error parsing --window duration %q\n", windowStr)
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

	chk, err := resumption.CreateCheckpoint(ctx, db, projectPath, label, opts)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating checkpoint: %v\n", err)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "Checkpoint created successfully.\n")
	fmt.Fprintf(stdout, "ID:           %s\n", chk.ID)
	fmt.Fprintf(stdout, "Label:        %s\n", chk.Label)
	fmt.Fprintf(stdout, "Project Path: %s\n", chk.ProjectPath)
	fmt.Fprintf(stdout, "Captured At:  %s\n", chk.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	return ExitSuccess
}

func runCheckpointList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("checkpoint list", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var limit int
	fs.IntVar(&limit, "limit", 20, "maximum number of checkpoints to display")

	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			if !strings.Contains(arg, "=") && flagName == "limit" {
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

	projectPath := ""
	if len(posArgs) > 0 {
		projectPath = posArgs[0]
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	list, err := db.ListCheckpoints(ctx, projectPath, limit)
	if err != nil {
		fmt.Fprintf(stderr, "Error listing checkpoints: %v\n", err)
		return ExitGeneralErr
	}

	if len(list) == 0 {
		if projectPath != "" {
			fmt.Fprintf(stdout, "No checkpoints recorded for project %s.\n", projectPath)
		} else {
			fmt.Fprintf(stdout, "No checkpoints recorded.\n")
		}
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "%-32s %-20s %-30s %s\n", "CHECKPOINT ID", "CAPTURED (UTC)", "PROJECT PATH", "LABEL")
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("-", 100))

	for _, chk := range list {
		fmt.Fprintf(stdout, "%-32s %-20s %-30s %s\n",
			chk.ID,
			chk.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
			truncate(chk.ProjectPath, 30),
			chk.Label,
		)
	}

	return ExitSuccess
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runCheckpointShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("checkpoint show", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var format string
	fs.StringVar(&format, "format", "text", "output format: text, markdown, json")

	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			if !strings.Contains(arg, "=") && flagName == "format" {
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

	if len(posArgs) == 0 {
		fmt.Fprintf(stderr, "Error: checkpoint ID is required. Usage: %s checkpoint show <checkpoint-id>\n", AppName)
		return ExitUsageError
	}

	checkpointID := posArgs[0]

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	chk, err := db.GetCheckpoint(ctx, checkpointID)
	if err != nil {
		fmt.Fprintf(stderr, "Error: checkpoint %q not found\n", checkpointID)
		return ExitGeneralErr
	}

	rendered, err := resumption.RenderCheckpoint(chk, format)
	if err != nil {
		fmt.Fprintf(stderr, "Error rendering checkpoint: %v\n", err)
		return ExitGeneralErr
	}

	stdout.Write(rendered)
	if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
		fmt.Fprintln(stdout)
	}

	return ExitSuccess
}

func runCheckpointDelete(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("checkpoint delete", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var confirm bool
	fs.BoolVar(&confirm, "confirm", false, "confirm checkpoint deletion")

	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return ExitUsageError
	}

	if len(posArgs) == 0 {
		fmt.Fprintf(stderr, "Error: checkpoint ID is required. Usage: %s checkpoint delete <checkpoint-id> --confirm\n", AppName)
		return ExitUsageError
	}

	if !confirm {
		fmt.Fprintf(stderr, "Error: checkpoint deletion requires --confirm flag\n")
		return ExitUsageError
	}

	checkpointID := posArgs[0]

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.DeleteCheckpoint(ctx, checkpointID); err != nil {
		fmt.Fprintf(stderr, "Error: checkpoint %q not found\n", checkpointID)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "Checkpoint %s deleted successfully.\n", checkpointID)
	return ExitSuccess
}
