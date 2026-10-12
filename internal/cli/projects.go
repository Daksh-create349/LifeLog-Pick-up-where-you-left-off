package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/storage"
)

func runProjects(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var sortStr string
	var limit int
	var windowStr string
	var sinceStr string
	var asJSON bool

	fs.StringVar(&sortStr, "sort", "recent", "sort order: recent, name, activity")
	fs.IntVar(&limit, "limit", 50, "maximum number of projects to display (1-500)")
	fs.StringVar(&windowStr, "window", "7d", "activity lookback window (e.g. 24h, 7d, 30d, 0/all for lifetime)")
	fs.StringVar(&sinceStr, "since", "", "filter activity on or after timestamp (RFC3339, YYYY-MM-DD)")
	fs.BoolVar(&asJSON, "json", false, "output projects overview as JSON")

	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			if !strings.Contains(arg, "=") && (flagName == "sort" || flagName == "limit" || flagName == "window" || flagName == "since") {
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

	if len(posArgs) > 0 {
		fmt.Fprintf(stderr, "Error: unexpected positional argument %q. Usage: %s projects [flags]\n", posArgs[0], AppName)
		return ExitUsageError
	}

	// Validate sort mode
	var sortMode storage.ProjectSortMode
	switch strings.ToLower(strings.TrimSpace(sortStr)) {
	case "recent":
		sortMode = storage.SortRecent
	case "name":
		sortMode = storage.SortName
	case "activity":
		sortMode = storage.SortActivity
	default:
		fmt.Fprintf(stderr, "Error: invalid --sort mode %q (supported: recent, name, activity)\n", sortStr)
		return ExitUsageError
	}

	// Validate limit
	if limit < 1 || limit > 500 {
		fmt.Fprintf(stderr, "Error: --limit must be between 1 and 500 (got %d)\n", limit)
		return ExitUsageError
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
				fmt.Fprintf(stderr, "Error parsing --window duration %q (use 24h, 7d, or 0 for all)\n", windowStr)
				return ExitUsageError
			}
			windowDuration = d
		}
	}

	if windowDuration > 0 && sinceTime == nil {
		t := time.Now().UTC().Add(-windowDuration)
		sinceTime = &t
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opts := storage.ProjectOverviewOptions{
		SortMode: sortMode,
		Limit:    limit,
		Window:   windowDuration,
		Since:    sinceTime,
	}

	projects, err := db.DiscoverProjects(ctx, opts)
	if err != nil {
		fmt.Fprintf(stderr, "Error discovering projects: %v\n", err)
		return ExitGeneralErr
	}

	if asJSON {
		if projects == nil {
			projects = []storage.ProjectOverview{}
		}
		data, err := json.MarshalIndent(projects, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "Error marshaling projects to JSON: %v\n", err)
			return ExitGeneralErr
		}
		fmt.Fprintln(stdout, string(data))
		return ExitSuccess
	}

	// Human-readable overview output
	if len(projects) == 0 {
		fmt.Fprintf(stdout, "No projects currently discovered in LIFELOG memory.\n")
		fmt.Fprintf(stdout, "Record terminal activity, register Git repositories, or watch project directories to track activity.\n")
		return ExitSuccess
	}

	windowDesc := "lifetime"
	if windowDuration > 0 {
		windowDesc = windowStr
	} else if sinceTime != nil {
		windowDesc = fmt.Sprintf("since %s", sinceTime.Format("2006-01-02"))
	}

	fmt.Fprintf(stdout, "LIFELOG Multi-Project Overview\n")
	fmt.Fprintf(stdout, "Discovered Projects: %d | Sort: %s | Window: %s\n", len(projects), sortStr, windowDesc)
	fmt.Fprintf(stdout, "================================================================================\n\n")

	for i, p := range projects {
		gitInfo := "not git"
		if p.IsGitRegistered {
			branch := p.GitBranch
			if branch == "" {
				branch = "unknown"
			}
			gitInfo = fmt.Sprintf("git (%s)", branch)
		}

		lastActive := "no activity"
		if p.LatestActivity != nil {
			lastActive = p.LatestActivity.UTC().Format("2006-01-02 15:04:05 UTC")
		}

		diskStatus := "exists"
		if !p.ExistsOnDisk {
			diskStatus = "missing on disk"
		}

		chkStatus := "none"
		if p.HasCheckpoint {
			chkStatus = fmt.Sprintf("%d saved", p.CheckpointCount)
		}

		fmt.Fprintf(stdout, "[%d] %s (%s)\n", i+1, p.DisplayName, diskStatus)
		fmt.Fprintf(stdout, "    Path:         %s\n", p.CanonicalPath)
		fmt.Fprintf(stdout, "    Git State:    %s\n", gitInfo)
		fmt.Fprintf(stdout, "    Checkpoints:  %s\n", chkStatus)
		fmt.Fprintf(stdout, "    Activity:     %d total events (fs: %d, git: %d, term: %d)\n",
			p.TotalEvents,
			p.DomainCounts["filesystem"],
			p.DomainCounts["git"],
			p.DomainCounts["terminal"])
		fmt.Fprintf(stdout, "    Last Active:  %s\n", lastActive)
		fmt.Fprintln(stdout)
	}

	return ExitSuccess
}
