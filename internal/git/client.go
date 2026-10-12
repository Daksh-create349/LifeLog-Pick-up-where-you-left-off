package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCmdTimeout = 8 * time.Second
)

// Client provides safe, non-destructive Git operations via os/exec.
type Client struct{}

// NewClient returns a new Git client.
func NewClient() *Client {
	return &Client{}
}

// CheckInstalled verifies that the git binary is available and executable.
func (c *Client) CheckInstalled(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", ErrNotInstalled, err)
	}
	return nil
}

// ValidateAndDiscoverRoot resolves a path to its canonical repository root and verifies safety constraints.
func (c *Client) ValidateAndDiscoverRoot(ctx context.Context, inputPath, dataDir string) (string, bool, error) {
	if inputPath == "" {
		return "", false, fmt.Errorf("%w: path cannot be empty", ErrInvalidPath)
	}

	absPath, err := filepath.Abs(inputPath)
	if err != nil {
		return "", false, fmt.Errorf("%w: cannot resolve absolute path: %v", ErrInvalidPath, err)
	}
	absPath = filepath.Clean(absPath)

	if err := checkRestrictedPath(absPath, dataDir); err != nil {
		return "", false, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, fmt.Errorf("%w: path does not exist: %s", ErrInvalidPath, absPath)
		}
		return "", false, fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	if !info.IsDir() {
		// If path is a file, check if it's a git worktree file (.git)
		if filepath.Base(absPath) == ".git" {
			absPath = filepath.Dir(absPath)
		} else {
			return "", false, fmt.Errorf("%w: path is not a directory: %s", ErrInvalidPath, absPath)
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, defaultCmdTimeout)
	defer cancel()

	// 1. Check if bare repository
	cmdBare := exec.CommandContext(cmdCtx, "git", "-C", absPath, "rev-parse", "--is-bare-repository")
	bareOut, err := cmdBare.Output()
	if err != nil {
		return "", false, fmt.Errorf("%w: %s is not inside a git repository", ErrNotRepository, absPath)
	}
	isBare := strings.TrimSpace(string(bareOut)) == "true"

	var canonicalRoot string
	if isBare {
		cmdDir := exec.CommandContext(cmdCtx, "git", "-C", absPath, "rev-parse", "--git-dir")
		gitDirOut, err := cmdDir.Output()
		if err != nil {
			return "", true, fmt.Errorf("%w: failed resolving bare git dir: %v", ErrNotRepository, err)
		}
		canonicalRoot = strings.TrimSpace(string(gitDirOut))
		if !filepath.IsAbs(canonicalRoot) {
			canonicalRoot = filepath.Join(absPath, canonicalRoot)
		}
	} else {
		cmdTop := exec.CommandContext(cmdCtx, "git", "-C", absPath, "rev-parse", "--show-toplevel")
		topOut, err := cmdTop.Output()
		if err != nil {
			return "", false, fmt.Errorf("%w: failed resolving git root: %v", ErrNotRepository, err)
		}
		canonicalRoot = strings.TrimSpace(string(topOut))
	}

	canonicalRoot = filepath.Clean(canonicalRoot)
	if evalRoot, err := filepath.EvalSymlinks(canonicalRoot); err == nil {
		canonicalRoot = evalRoot
	}

	// Verify resolved canonical root against safety boundaries
	if err := checkRestrictedPath(canonicalRoot, dataDir); err != nil {
		return "", isBare, err
	}

	return canonicalRoot, isBare, nil
}

func checkRestrictedPath(path, dataDir string) error {
	if path == "/" || path == filepath.VolumeName(path)+"\\" {
		return fmt.Errorf("%w: scanning filesystem root %q is not permitted", ErrRestrictedPath, path)
	}

	home, err := os.UserHomeDir()
	if err == nil && filepath.Clean(home) == path {
		return fmt.Errorf("%w: scanning entire user home directory is not permitted", ErrRestrictedPath)
	}

	if dataDir != "" {
		cleanData := filepath.Clean(dataDir)
		if path == cleanData || strings.HasPrefix(path, cleanData+string(filepath.Separator)) {
			return fmt.Errorf("%w: cannot monitor LIFELOG internal data directory %q", ErrRestrictedPath, path)
		}
	}

	return nil
}

// Inspect reads the instantaneous Git state of the repository without modifying it.
func (c *Client) Inspect(ctx context.Context, root string) (*RepoState, error) {
	state := &RepoState{
		Root:       root,
		ObservedAt: time.Now().UTC(),
	}

	cmdCtx, cancel := context.WithTimeout(ctx, defaultCmdTimeout)
	defer cancel()

	// 1. Check bare status
	cmdBare := exec.CommandContext(cmdCtx, "git", "-C", root, "rev-parse", "--is-bare-repository")
	bareOut, err := cmdBare.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotRepository, err)
	}
	state.IsBare = strings.TrimSpace(string(bareOut)) == "true"

	// 2. Check if repository is empty (has no commits)
	cmdHead := exec.CommandContext(cmdCtx, "git", "-C", root, "rev-parse", "--verify", "HEAD")
	if err := cmdHead.Run(); err != nil {
		state.IsEmpty = true
		// In empty repo, discover current initial branch name (e.g. main/master)
		cmdSym := exec.CommandContext(cmdCtx, "git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD")
		if symOut, err := cmdSym.Output(); err == nil {
			state.HeadBranch = strings.TrimSpace(string(symOut))
		} else {
			state.HeadBranch = "main"
		}
		if !state.IsBare {
			state.WorkingTree = c.inspectWorkingTree(cmdCtx, root)
		}
		return state, nil
	}

	// 3. Current branch and detached HEAD detection
	cmdSym := exec.CommandContext(cmdCtx, "git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD")
	if symOut, err := cmdSym.Output(); err == nil {
		state.HeadBranch = strings.TrimSpace(string(symOut))
		state.IsDetached = false
	} else {
		state.IsDetached = true
		cmdRev := exec.CommandContext(cmdCtx, "git", "-C", root, "rev-parse", "--short", "HEAD")
		if revOut, err := cmdRev.Output(); err == nil {
			state.HeadBranch = fmt.Sprintf("detached at %s", strings.TrimSpace(string(revOut)))
		} else {
			state.HeadBranch = "detached"
		}
	}

	// 4. Collect recent commits
	commits, err := c.collectCommits(cmdCtx, root, 100)
	if err == nil && len(commits) > 0 {
		state.RecentCommits = commits
		state.HeadCommit = &commits[0]
	}

	// 5. Collect branches
	state.Branches = c.collectBranches(cmdCtx, root)

	// 6. Collect working-tree state (if not bare)
	if !state.IsBare {
		state.WorkingTree = c.inspectWorkingTree(cmdCtx, root)
	}

	return state, nil
}

func (c *Client) collectCommits(ctx context.Context, root string, limit int) ([]Commit, error) {
	args := []string{"-C", root, "log", fmt.Sprintf("-n%d", limit), "--format=%H%x00%an%x00%aI%x00%s"}
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []Commit
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 4 {
			continue
		}
		authorTime, err := time.Parse(time.RFC3339, parts[2])
		if err != nil {
			authorTime = time.Now().UTC()
		}
		commits = append(commits, Commit{
			Hash:       parts[0],
			AuthorName: parts[1],
			AuthorTime: authorTime,
			Subject:    parts[3],
		})
	}
	return commits, nil
}

func (c *Client) collectBranches(ctx context.Context, root string) []BranchInfo {
	args := []string{"-C", root, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads"}
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var branches []BranchInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			branches = append(branches, BranchInfo{
				Name:       parts[0],
				CommitHash: parts[1],
			})
		}
	}
	return branches
}

func (c *Client) inspectWorkingTree(ctx context.Context, root string) []FileChange {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain=v1", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var changes []FileChange
	entries := bytes.Split(out, []byte{0})
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 3 {
			continue
		}
		x := entry[0]
		y := entry[1]
		filePath := string(entry[3:])

		// Renames in porcelain -z format provide the target path in the subsequent entry
		if (x == 'R' || x == 'C') && i+1 < len(entries) {
			i++
			filePath = string(entries[i])
		}

		// Untracked
		if x == '?' && y == '?' {
			changes = append(changes, FileChange{Path: filePath, Status: StatusUntracked})
			continue
		}

		// Staged changes
		if x != ' ' && x != '?' {
			if x == 'D' {
				changes = append(changes, FileChange{Path: filePath, Status: StatusDeleted})
			} else {
				changes = append(changes, FileChange{Path: filePath, Status: StatusStaged})
			}
		}

		// Unstaged changes
		if y == 'M' {
			changes = append(changes, FileChange{Path: filePath, Status: StatusModified})
		} else if y == 'D' {
			changes = append(changes, FileChange{Path: filePath, Status: StatusDeleted})
		}
	}

	return changes
}
