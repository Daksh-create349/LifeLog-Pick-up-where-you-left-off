package resumption

import (
	"fmt"
	"io"
	"time"

	"github.com/lifelog/lifelog/internal/events"
)

// FormatReport writes a structured, human-readable resumption report to w.
func FormatReport(w io.Writer, r *ResumptionReport) {
	fmt.Fprintf(w, "================================================================================\n")
	fmt.Fprintf(w, "LIFELOG PROJECT RESUMPTION REPORT\n")
	fmt.Fprintf(w, "================================================================================\n")
	fmt.Fprintf(w, "Target Path:    %s\n", r.ProjectPath)
	fmt.Fprintf(w, "Canonical Path: %s\n", r.CanonicalPath)

	if !r.PathExists {
		fmt.Fprintf(w, "Notice:         Path does not currently exist on disk as a directory.\n")
	}

	if r.LatestActivityTime != nil {
		ago := r.QueryTime.Sub(*r.LatestActivityTime).Round(time.Minute)
		fmt.Fprintf(w, "Last Activity:  %s (%s ago)\n", r.LatestActivityTime.UTC().Format("2006-01-02 15:04:05 UTC"), ago)
	} else {
		fmt.Fprintf(w, "Last Activity:  No recorded activity\n")
	}

	if r.Since != nil {
		fmt.Fprintf(w, "Time Window:    Since %s (inactivity gap threshold: %v)\n", r.Since.UTC().Format("2006-01-02 15:04:05 UTC"), r.Derived.InactivityGap)
	} else {
		fmt.Fprintf(w, "Time Window:    All recorded history (inactivity gap threshold: %v)\n", r.Derived.InactivityGap)
	}
	fmt.Fprintln(w)

	// -------------------------------------------------------------------------
	// SECTION 1: OBSERVED ACTIVITY
	// -------------------------------------------------------------------------
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	fmt.Fprintf(w, "1. OBSERVED ACTIVITY (Persisted Local Facts)\n")
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n\n")

	// 1A. Filesystem Memory
	fmt.Fprintf(w, "[Filesystem Activity]\n")
	fs := r.Observed.Filesystem
	if fs.TotalEvents > 0 {
		fmt.Fprintf(w, "  Total Events: %d (Created: %d, Modified: %d, Deleted: %d)\n",
			fs.TotalEvents,
			fs.TypeCounts[events.EventCreated],
			fs.TypeCounts[events.EventModified],
			fs.TypeCounts[events.EventDeleted],
		)
		if len(fs.RecentChanges) > 0 {
			fmt.Fprintf(w, "  Recently Changed Paths:\n")
			for _, ch := range fs.RecentChanges {
				modStr := ch.LastModTime.UTC().Format("2006-01-02 15:04:05")
				fmt.Fprintf(w, "    %-8s  %-40s  (%d event(s), last: %s)\n",
					ch.LastEventType, truncate(ch.RelativePath, 40), ch.EventCount, modStr)
			}
		}
	} else {
		fmt.Fprintf(w, "  No filesystem events recorded for this path.\n")
	}
	fmt.Fprintln(w)

	// 1B. Git Memory
	fmt.Fprintf(w, "[Git Repository Memory]\n")
	g := r.Observed.Git
	if g.IsRegistered {
		branchState := g.CurrentBranch
		if g.IsDetached {
			branchState += " (detached HEAD)"
		}
		fmt.Fprintf(w, "  Repository:   %s [%s]\n", g.RepoPath, g.RepoID)
		fmt.Fprintf(w, "  Branch:       %s\n", branchState)

		if g.HeadCommit != nil {
			fmt.Fprintf(w, "  HEAD Commit:  %s by %s (%s)\n",
				truncate(g.HeadCommit.CommitHash, 8),
				g.HeadCommit.AuthorName,
				g.HeadCommit.AuthorTime.UTC().Format("2006-01-02 15:04:05"))
			fmt.Fprintf(w, "                %q\n", g.HeadCommit.Subject)
		}

		if len(g.WorkingTree) > 0 {
			fmt.Fprintf(w, "  Working Tree: %d uncommitted change(s):\n", len(g.WorkingTree))
			for i, ch := range g.WorkingTree {
				if i >= 10 {
					fmt.Fprintf(w, "    ... and %d more change(s)\n", len(g.WorkingTree)-10)
					break
				}
				fmt.Fprintf(w, "    %-10s  %s\n", ch.Status, ch.Path)
			}
		} else {
			fmt.Fprintf(w, "  Working Tree: Clean (0 uncommitted changes observed)\n")
		}

		if len(g.RecentCommits) > 0 {
			fmt.Fprintf(w, "  Recent Commits:\n")
			for i, c := range g.RecentCommits {
				if i >= 5 {
					fmt.Fprintf(w, "    ... and %d more commit(s)\n", len(g.RecentCommits)-5)
					break
				}
				fmt.Fprintf(w, "    %s  %s  %-20s  %q\n",
					truncate(c.CommitHash, 8),
					c.AuthorTime.UTC().Format("2006-01-02 15:04"),
					truncate(c.AuthorName, 20),
					truncate(c.Subject, 38))
			}
		}

		if len(g.BranchTransitions) > 0 {
			fmt.Fprintf(w, "  Branch Transitions:\n")
			for _, bt := range g.BranchTransitions {
				oldHash := truncate(bt.OldCommitHash, 8)
				if oldHash == "" {
					oldHash = "<initial>"
				}
				fmt.Fprintf(w, "    %s  branch %-15s %s -> %s\n",
					bt.ObservedAt.UTC().Format("2006-01-02 15:04"),
					bt.BranchName,
					oldHash,
					truncate(bt.NewCommitHash, 8))
			}
		}
	} else if g.IsGitOnDisk {
		fmt.Fprintf(w, "  Git repository detected on disk, but not yet registered/scanned into LIFELOG.\n")
	} else {
		fmt.Fprintf(w, "  No registered Git repository associated with this path.\n")
	}
	fmt.Fprintln(w)

	// 1C. Terminal Memory
	fmt.Fprintf(w, "[Terminal Execution Records]\n")
	term := r.Observed.Terminal
	if term.TotalCommands > 0 {
		fmt.Fprintf(w, "  Explicitly Recorded Commands: %d\n", term.TotalCommands)
		for _, rec := range term.RecentCommands {
			cmd := rec.SanitizedCommand
			if rec.IsOmitted {
				cmd = "[DETAILS OMITTED] (" + rec.OmissionReason + ")"
			}
			exitStr := "exit=?"
			if rec.ExitCode != nil {
				exitStr = fmt.Sprintf("exit=%d", *rec.ExitCode)
			}
			fmt.Fprintf(w, "    %s  %-8s  %-8s  %s\n",
				rec.StartTime.UTC().Format("2006-01-02 15:04:05"),
				exitStr,
				fmt.Sprintf("%dms", rec.DurationMs),
				truncate(cmd, 45))
		}
	} else {
		fmt.Fprintf(w, "  No terminal commands explicitly recorded for this directory.\n")
	}
	fmt.Fprintln(w)

	// -------------------------------------------------------------------------
	// SECTION 2: DERIVED ACTIVITY SESSIONS & INFERENCES
	// -------------------------------------------------------------------------
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	fmt.Fprintf(w, "2. DERIVED ACTIVITY SESSIONS & SYNTHESIS (Computed Context)\n")
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n\n")

	fmt.Fprintf(w, "[Context Inference]\n")
	fmt.Fprintf(w, "  %s\n\n", r.Derived.ActivityInference)

	fmt.Fprintf(w, "[Chronological Activity Sessions]\n")
	if len(r.Derived.Sessions) == 0 {
		fmt.Fprintf(w, "  No distinct activity sessions identified.\n")
	} else {
		for _, s := range r.Derived.Sessions {
			durStr := formatDuration(s.Duration)
			fmt.Fprintf(w, "  %s (%s -> %s, duration: %s, %d events)\n",
				s.ID,
				s.StartTime.UTC().Format("2006-01-02 15:04:05"),
				s.EndTime.UTC().Format("15:04:05"),
				durStr,
				s.ActivityCount)
			fmt.Fprintf(w, "    Summary: %s\n", s.Summary)
		}
	}
	fmt.Fprintln(w)

	// -------------------------------------------------------------------------
	// SECTION 3: UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES
	// -------------------------------------------------------------------------
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	fmt.Fprintf(w, "3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES\n")
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n\n")

	for _, n := range r.Unavailable {
		fmt.Fprintf(w, "  - %s\n", n)
	}
	fmt.Fprintf(w, "\n================================================================================\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, mins)
}
