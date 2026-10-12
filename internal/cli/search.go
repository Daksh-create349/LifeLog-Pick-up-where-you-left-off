package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/search"
)

func runSearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var domainStr string
	var limit int
	var sinceStr, untilStr string
	var chronological bool

	fs.StringVar(&domainStr, "domain", "all", "search domain: all, filesystem, git, terminal")
	fs.IntVar(&limit, "limit", 20, "maximum number of results (1-500)")
	fs.StringVar(&sinceStr, "since", "", "filter events on or after timestamp (RFC3339, YYYY-MM-DD, or duration e.g. 24h, 7d)")
	fs.StringVar(&sinceStr, "start", "", "alias for --since")
	fs.StringVar(&untilStr, "until", "", "filter events on or before timestamp (RFC3339, YYYY-MM-DD)")
	fs.StringVar(&untilStr, "end", "", "alias for --until")
	fs.BoolVar(&chronological, "chronological", false, "sort results in chronological order (oldest first)")

	var flagArgs []string
	var queryTokens []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			flagName := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			// If flag expects a value and didn't use '=', consume next arg if available
			if !strings.Contains(arg, "=") && (flagName == "domain" || flagName == "limit" || flagName == "since" || flagName == "start" || flagName == "until" || flagName == "end") {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		} else {
			queryTokens = append(queryTokens, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return ExitUsageError
	}

	queryText := strings.TrimSpace(strings.Join(queryTokens, " "))

	dom := search.Domain(strings.ToLower(domainStr))
	if !dom.Valid() {
		fmt.Fprintf(stderr, "Error: Invalid domain %q. Supported domains: all, filesystem, git, terminal\n", domainStr)
		return ExitUsageError
	}

	var startTime, endTime *time.Time
	if sinceStr != "" {
		t, err := parseSearchTime(sinceStr, true)
		if err != nil {
			fmt.Fprintf(stderr, "Error parsing --since/--start: %v\n", err)
			return ExitUsageError
		}
		startTime = t
	}

	if untilStr != "" {
		t, err := parseSearchTime(untilStr, false)
		if err != nil {
			fmt.Fprintf(stderr, "Error parsing --until/--end: %v\n", err)
			return ExitUsageError
		}
		endTime = t
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := db.Search(ctx, search.Query{
		Text:          queryText,
		Domain:        dom,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         limit,
		Chronological: chronological,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error executing search: %v\n", err)
		return ExitGeneralErr
	}

	if len(result.Items) == 0 {
		if queryText != "" {
			fmt.Fprintf(stdout, "No matching records found for query %q in domain %s.\n", queryText, dom)
		} else {
			fmt.Fprintf(stdout, "No recorded events found in domain %s.\n", dom)
		}
		return ExitSuccess
	}

	header := fmt.Sprintf("LIFELOG Search Results (showing %d of %d)", len(result.Items), result.TotalCount)
	if queryText != "" {
		header = fmt.Sprintf("LIFELOG Search Results for: %q (showing %d of %d)", queryText, len(result.Items), result.TotalCount)
	}
	fmt.Fprintf(stdout, "%s\n", header)
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("=", 88))
	fmt.Fprintf(stdout, "%-12s %-16s %-24s %s\n", "DOMAIN", "RECORD ID", "TIMESTAMP (UTC)", "TITLE / DETAILS")

	for _, item := range result.Items {
		timeStr := item.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC")
		displayTitle := item.Title
		if len(displayTitle) > 55 {
			displayTitle = displayTitle[:52] + "..."
		}
		recID := item.RecordID
		if len(recID) > 15 {
			recID = recID[:12] + "..."
		}
		fmt.Fprintf(stdout, "%-12s %-16s %-24s %s\n", item.Domain, recID, timeStr, displayTitle)
	}

	return ExitSuccess
}

func runTimeline(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("timeline", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var domainStr string
	var limit int
	var sinceStr, untilStr string

	fs.StringVar(&domainStr, "domain", "all", "timeline domain: all, filesystem, git, terminal")
	fs.IntVar(&limit, "limit", 20, "maximum number of entries (1-500)")
	fs.StringVar(&sinceStr, "since", "", "filter events on or after timestamp (RFC3339, YYYY-MM-DD, or duration e.g. 24h, 7d)")
	fs.StringVar(&untilStr, "until", "", "filter events on or before timestamp (RFC3339, YYYY-MM-DD)")

	if err := fs.Parse(args); err != nil {
		return ExitUsageError
	}

	dom := search.Domain(strings.ToLower(domainStr))
	if !dom.Valid() {
		fmt.Fprintf(stderr, "Error: Invalid domain %q. Supported domains: all, filesystem, git, terminal\n", domainStr)
		return ExitUsageError
	}

	var startTime, endTime *time.Time
	if sinceStr != "" {
		t, err := parseSearchTime(sinceStr, true)
		if err != nil {
			fmt.Fprintf(stderr, "Error parsing --since: %v\n", err)
			return ExitUsageError
		}
		startTime = t
	}

	if untilStr != "" {
		t, err := parseSearchTime(untilStr, false)
		if err != nil {
			fmt.Fprintf(stderr, "Error parsing --until: %v\n", err)
			return ExitUsageError
		}
		endTime = t
	}

	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := db.Search(ctx, search.Query{
		Text:          "",
		Domain:        dom,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         limit,
		Chronological: true, // Timeline is chronological (oldest to newest)
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error retrieving timeline: %v\n", err)
		return ExitGeneralErr
	}

	if len(result.Items) == 0 {
		fmt.Fprintf(stdout, "No timeline activity found in domain %s.\n", dom)
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "LIFELOG Unified Activity Timeline (showing %d)\n", len(result.Items))
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("=", 88))
	fmt.Fprintf(stdout, "%-24s %-12s %-16s %s\n", "TIMESTAMP (UTC)", "DOMAIN", "RECORD ID", "SUMMARY / DETAILS")

	for _, item := range result.Items {
		timeStr := item.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC")
		displayTitle := item.Title
		if len(displayTitle) > 55 {
			displayTitle = displayTitle[:52] + "..."
		}
		recID := item.RecordID
		if len(recID) > 15 {
			recID = recID[:12] + "..."
		}
		fmt.Fprintf(stdout, "%-24s %-12s %-16s %s\n", timeStr, item.Domain, recID, displayTitle)
	}

	return ExitSuccess
}

func parseSearchTime(s string, isSince bool) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	// 1. Check relative duration (e.g., 24h, 30m, 7d)
	if isSince {
		lower := strings.ToLower(s)
		if strings.HasSuffix(lower, "d") {
			daysStr := lower[:len(lower)-1]
			var days int
			if _, err := fmt.Sscanf(daysStr, "%d", &days); err == nil && days > 0 {
				t := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
				return &t, nil
			}
		}

		if d, err := time.ParseDuration(lower); err == nil && d > 0 {
			t := time.Now().UTC().Add(-d)
			return &t, nil
		}
	}

	// 2. Standard timestamp formats
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, s); err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}

	return nil, fmt.Errorf("unrecognized time format %q (use RFC3339, YYYY-MM-DD, or duration like 24h, 7d)", s)
}
