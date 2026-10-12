package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/config"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/storage"
)

func runGit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printGitUsage(stdout)
		return ExitSuccess
	}

	switch args[0] {
	case "scan":
		return runGitScan(args[1:], stdout, stderr)
	case "status":
		return runGitStatus(args[1:], stdout, stderr)
	case "log":
		return runGitLog(args[1:], stdout, stderr)
	case "repos":
		return runRepos(stdout, stderr)
	case "branches":
		return runGitBranches(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printGitUsage(stdout)
		return ExitSuccess
	default:
		fmt.Fprintf(stderr, "Unknown git subcommand: %s\nRun '%s git help' for usage details.\n", args[0], AppName)
		return ExitUsageError
	}
}

func printGitUsage(w io.Writer) {
	fmt.Fprintf(w, "LIFELOG Git Memory Engine\n\n")
	fmt.Fprintf(w, "Usage:\n  %s git <command> [arguments]\n\n", AppName)
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  scan [path]             Scan and persist Git repository state (defaults to current dir)\n")
	fmt.Fprintf(w, "  status [path_or_id]     Display latest recorded repository state\n")
	fmt.Fprintf(w, "  log [path_or_id] [flag] Query recorded Git commit history\n")
	fmt.Fprintf(w, "  branches [path_or_id]   Display recorded branches and branch tip history\n")
	fmt.Fprintf(w, "  repos                   List all registered repositories\n")
	fmt.Fprintf(w, "  help                    Show this help information\n")
}

func openDB() (*storage.DB, *paths.Paths, error) {
	p, err := paths.Resolve()
	if err != nil {
		return nil, nil, fmt.Errorf("resolving paths: %w", err)
	}

	cfg, err := config.Load(p)
	if err != nil {
		return nil, nil, fmt.Errorf("loading configuration: %w", err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}

	return db, p, nil
}

func runGitScan(args []string, stdout, stderr io.Writer) int {
	targetPath := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetPath = args[0]
	}

	db, p, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := git.NewClient()
	if err := client.CheckInstalled(ctx); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}

	canonicalRoot, isBare, err := client.ValidateAndDiscoverRoot(ctx, targetPath, p.DataDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitUsageError
	}

	repoID, err := db.RegisterOrUpdateGitRepo(ctx, canonicalRoot, isBare)
	if err != nil {
		fmt.Fprintf(stderr, "Error registering repository: %v\n", err)
		return ExitGeneralErr
	}

	state, err := client.Inspect(ctx, canonicalRoot)
	if err != nil {
		fmt.Fprintf(stderr, "Error inspecting repository: %v\n", err)
		return ExitGeneralErr
	}

	summary, err := db.RecordGitState(ctx, repoID, state)
	if err != nil {
		fmt.Fprintf(stderr, "Error recording git state: %v\n", err)
		return ExitGeneralErr
	}

	headDesc := "None (empty repository)"
	if state.HeadCommit != nil {
		headDesc = fmt.Sprintf("%s - %s (%s)", state.HeadCommit.Hash[:min(8, len(state.HeadCommit.Hash))], state.HeadCommit.Subject, state.HeadCommit.AuthorName)
	}

	wtSummary := "clean"
	if len(state.WorkingTree) > 0 {
		staged, modified, untracked, deleted := 0, 0, 0, 0
		for _, c := range state.WorkingTree {
			switch c.Status {
			case git.StatusStaged:
				staged++
			case git.StatusModified:
				modified++
			case git.StatusUntracked:
				untracked++
			case git.StatusDeleted:
				deleted++
			}
		}
		var parts []string
		if staged > 0 {
			parts = append(parts, fmt.Sprintf("%d staged", staged))
		}
		if modified > 0 {
			parts = append(parts, fmt.Sprintf("%d modified", modified))
		}
		if untracked > 0 {
			parts = append(parts, fmt.Sprintf("%d untracked", untracked))
		}
		if deleted > 0 {
			parts = append(parts, fmt.Sprintf("%d deleted", deleted))
		}
		wtSummary = fmt.Sprintf("%d uncommitted change(s) [%s]", len(state.WorkingTree), strings.Join(parts, ", "))
	}

	repoType := "normal"
	if isBare {
		repoType = "bare"
	}

	fmt.Fprintf(stdout, "LIFELOG Git Scan\n")
	fmt.Fprintf(stdout, "================\n")
	fmt.Fprintf(stdout, "Repository ID:  %s\n", repoID)
	fmt.Fprintf(stdout, "Canonical Path: %s\n", canonicalRoot)
	fmt.Fprintf(stdout, "Type:           %s\n", repoType)
	fmt.Fprintf(stdout, "Branch:         %s\n", state.HeadBranch)
	fmt.Fprintf(stdout, "Head Commit:    %s\n", headDesc)
	fmt.Fprintf(stdout, "New Commits:    %d\n", summary.NewCommitsCount)
	fmt.Fprintf(stdout, "Branch Updates: %d\n", summary.BranchChanges)
	fmt.Fprintf(stdout, "Working Tree:   %s\n", wtSummary)

	return ExitSuccess
}

func resolveRepo(ctx context.Context, db *storage.DB, client *git.Client, arg, dataDir string) (*storage.GitRepoRecord, error) {
	if arg != "" {
		// 1. Try direct lookup by ID or path
		r, err := db.GetGitRepo(ctx, arg)
		if err == nil {
			return r, nil
		}
		// 2. Try discovering canonical root for arg
		canonical, _, err := client.ValidateAndDiscoverRoot(ctx, arg, dataDir)
		if err == nil {
			if r, err := db.GetGitRepo(ctx, canonical); err == nil {
				return r, nil
			}
		}
		return nil, fmt.Errorf("repository %q is not registered with LIFELOG; run 'lifelog git scan %s' first", arg, arg)
	}

	// Default to current working directory
	canonical, _, err := client.ValidateAndDiscoverRoot(ctx, ".", dataDir)
	if err != nil {
		return nil, fmt.Errorf("current directory is not a git repository; specify a repository path or ID")
	}
	r, err := db.GetGitRepo(ctx, canonical)
	if err != nil {
		return nil, fmt.Errorf("repository at %s is not registered; run 'lifelog git scan .' first", canonical)
	}
	return r, nil
}

func runGitStatus(args []string, stdout, stderr io.Writer) int {
	var targetArg string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetArg = args[0]
	}

	db, p, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := git.NewClient()
	repo, err := resolveRepo(ctx, db, client, targetArg, p.DataDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitUsageError
	}

	snap, changes, err := db.GetLatestGitSnapshot(ctx, repo.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Error querying snapshot: %v\n", err)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "LIFELOG Git Repository Status\n")
	fmt.Fprintf(stdout, "=============================\n")
	fmt.Fprintf(stdout, "Repository:     %s (%s)\n", repo.Path, repo.ID)
	fmt.Fprintf(stdout, "Last Observed:  %s\n", repo.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))

	if snap == nil {
		fmt.Fprintf(stdout, "\nNo state recorded yet. Run 'lifelog git scan %s'\n", repo.Path)
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "Branch:         %s\n", snap.Branch)
	headStr := "None"
	if snap.HeadCommitHash != "" {
		headStr = snap.HeadCommitHash
		// Query commit subject if available
		commits, _ := db.QueryGitCommits(ctx, repo.ID, 1)
		if len(commits) > 0 && commits[0].CommitHash == snap.HeadCommitHash {
			headStr = fmt.Sprintf("%s - %s (%s)", snap.HeadCommitHash[:min(8, len(snap.HeadCommitHash))], commits[0].Subject, commits[0].AuthorName)
		}
	}
	fmt.Fprintf(stdout, "Head Commit:    %s\n", headStr)

	if len(changes) == 0 {
		fmt.Fprintf(stdout, "Working Tree:   Clean\n")
	} else {
		fmt.Fprintf(stdout, "Working Tree:   %d uncommitted change(s)\n", len(changes))
		for _, c := range changes {
			fmt.Fprintf(stdout, "  [%-9s] %s\n", c.Status, c.Path)
		}
	}

	return ExitSuccess
}

func runGitLog(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("git-log", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var limit int
	fs.IntVar(&limit, "limit", 20, "maximum number of commits to display (1-1000)")

	var targetArg string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && (arg == "-limit" || arg == "--limit") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if targetArg == "" {
			targetArg = arg
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return ExitUsageError
	}
	if limit <= 0 || limit > 1000 {
		fmt.Fprintf(stderr, "Error: limit must be between 1 and 1000 (got %d)\n", limit)
		return ExitUsageError
	}

	db, p, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := git.NewClient()
	repo, err := resolveRepo(ctx, db, client, targetArg, p.DataDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitUsageError
	}

	commits, err := db.QueryGitCommits(ctx, repo.ID, limit)
	if err != nil {
		fmt.Fprintf(stderr, "Error querying commits: %v\n", err)
		return ExitGeneralErr
	}

	if len(commits) == 0 {
		fmt.Fprintf(stdout, "No commits recorded for repository %s.\n", repo.Path)
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "LIFELOG Git Commits: %s (showing %d)\n", repo.Path, len(commits))
	fmt.Fprintf(stdout, "========================================================================================\n")
	fmt.Fprintf(stdout, "%-9s  %-20s  %-24s  %s\n", "COMMIT", "AUTHOR", "AUTHOR TIME (UTC)", "SUBJECT")
	for _, c := range commits {
		hashShort := c.CommitHash[:min(8, len(c.CommitHash))]
		author := c.AuthorName[:min(20, len(c.AuthorName))]
		fmt.Fprintf(stdout, "%-9s  %-20s  %-24s  %s\n",
			hashShort,
			author,
			c.AuthorTime.UTC().Format("2006-01-02 15:04:05 UTC"),
			c.Subject,
		)
	}

	return ExitSuccess
}

func runGitBranches(args []string, stdout, stderr io.Writer) int {
	var targetArg string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetArg = args[0]
	}

	db, p, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := git.NewClient()
	repo, err := resolveRepo(ctx, db, client, targetArg, p.DataDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitUsageError
	}

	branches, err := db.QueryGitBranches(ctx, repo.ID)
	if err != nil {
		fmt.Fprintf(stderr, "Error querying branches: %v\n", err)
		return ExitGeneralErr
	}

	branchLogs, err := db.QueryGitBranchLog(ctx, repo.ID, 20)
	if err != nil {
		fmt.Fprintf(stderr, "Error querying branch logs: %v\n", err)
		return ExitGeneralErr
	}

	fmt.Fprintf(stdout, "LIFELOG Git Branches: %s\n", repo.Path)
	fmt.Fprintf(stdout, "==================================================\n")
	if len(branches) == 0 {
		fmt.Fprintf(stdout, "No local branches discovered.\n")
	} else {
		for _, b := range branches {
			short := b.CommitHash[:min(8, len(b.CommitHash))]
			fmt.Fprintf(stdout, "  * %-20s -> %s\n", b.Name, short)
		}
	}

	if len(branchLogs) > 0 {
		fmt.Fprintf(stdout, "\nObserved Branch Movement (showing %d):\n", len(branchLogs))
		fmt.Fprintf(stdout, "--------------------------------------------------\n")
		for _, l := range branchLogs {
			oldStr := "initial discovery"
			if l.OldCommitHash != "" {
				oldStr = l.OldCommitHash[:min(8, len(l.OldCommitHash))]
			}
			newStr := l.NewCommitHash[:min(8, len(l.NewCommitHash))]
			fmt.Fprintf(stdout, "  [%s] %s: %s -> %s\n",
				l.ObservedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
				l.BranchName,
				oldStr,
				newStr,
			)
		}
	}

	return ExitSuccess
}

func runRepos(stdout, stderr io.Writer) int {
	db, _, err := openDB()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return ExitGeneralErr
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repos, err := db.ListGitRepos(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "Error listing repositories: %v\n", err)
		return ExitGeneralErr
	}

	if len(repos) == 0 {
		fmt.Fprintf(stdout, "No Git repositories registered yet. Use '%s git scan <path>' to register one.\n", AppName)
		return ExitSuccess
	}

	fmt.Fprintf(stdout, "LIFELOG Registered Repositories (showing %d)\n", len(repos))
	fmt.Fprintf(stdout, "========================================================================================\n")
	fmt.Fprintf(stdout, "%-16s  %-6s  %-24s  %s\n", "REPOSITORY ID", "BARE", "LAST OBSERVED (UTC)", "CANONICAL PATH")
	for _, r := range repos {
		fmt.Fprintf(stdout, "%-16s  %-6t  %-24s  %s\n",
			r.ID,
			r.IsBare,
			r.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
			r.Path,
		)
	}

	return ExitSuccess
}
