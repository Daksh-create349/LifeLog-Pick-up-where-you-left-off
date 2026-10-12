package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lifelog/lifelog/internal/git"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=TestAuthor",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=TestAuthor",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v (output: %s)", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestGitClient_EmptyRepository(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "empty_repo")
	_ = os.MkdirAll(repoDir, 0755)

	runGit(t, repoDir, "init", "-b", "main")

	client := git.NewClient()
	ctx := context.Background()

	canon, isBare, err := client.ValidateAndDiscoverRoot(ctx, repoDir, "")
	if err != nil {
		t.Fatalf("ValidateAndDiscoverRoot failed: %v", err)
	}
	if isBare {
		t.Errorf("expected isBare false, got true")
	}
	if canon == "" {
		t.Errorf("empty canonical root")
	}

	state, err := client.Inspect(ctx, canon)
	if err != nil {
		t.Fatalf("Inspect on empty repository failed: %v", err)
	}

	if !state.IsEmpty {
		t.Errorf("expected state.IsEmpty to be true on empty repo")
	}
	if state.HeadBranch != "main" {
		t.Errorf("expected HeadBranch 'main', got %q", state.HeadBranch)
	}
	if state.HeadCommit != nil {
		t.Errorf("expected nil HeadCommit on empty repo")
	}
}

func TestGitClient_DetachedHEAD(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "detached_repo")
	_ = os.MkdirAll(repoDir, 0755)

	runGit(t, repoDir, "init", "-b", "main")
	_ = os.WriteFile(filepath.Join(repoDir, "a.txt"), []byte("initial"), 0644)
	runGit(t, repoDir, "add", "a.txt")
	commitHash := runGit(t, repoDir, "commit", "-m", "first commit")
	_ = commitHash

	// Get full commit hash
	headHash := runGit(t, repoDir, "rev-parse", "HEAD")

	// Checkout commit hash directly -> detached HEAD
	runGit(t, repoDir, "checkout", headHash)

	client := git.NewClient()
	ctx := context.Background()

	state, err := client.Inspect(ctx, repoDir)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !state.IsDetached {
		t.Errorf("expected state.IsDetached to be true")
	}
	if !strings.Contains(state.HeadBranch, "detached at") {
		t.Errorf("expected HeadBranch to contain 'detached at', got %q", state.HeadBranch)
	}
}

func TestGitClient_WorktreeSupport(t *testing.T) {
	tempDir := t.TempDir()
	mainRepo := filepath.Join(tempDir, "main_repo")
	_ = os.MkdirAll(mainRepo, 0755)

	runGit(t, mainRepo, "init", "-b", "main")
	_ = os.WriteFile(filepath.Join(mainRepo, "init.txt"), []byte("data"), 0644)
	runGit(t, mainRepo, "add", "init.txt")
	runGit(t, mainRepo, "commit", "-m", "main commit")

	// Create linked worktree
	worktreeDir := filepath.Join(tempDir, "linked_worktree")
	runGit(t, mainRepo, "worktree", "add", "-b", "feature-worktree", worktreeDir)

	client := git.NewClient()
	ctx := context.Background()

	canonRoot, isBare, err := client.ValidateAndDiscoverRoot(ctx, worktreeDir, "")
	if err != nil {
		t.Fatalf("ValidateAndDiscoverRoot on worktree failed: %v", err)
	}
	if isBare {
		t.Errorf("worktree should not be bare")
	}

	state, err := client.Inspect(ctx, canonRoot)
	if err != nil {
		t.Fatalf("Inspect on worktree failed: %v", err)
	}

	if state.HeadBranch != "feature-worktree" {
		t.Errorf("expected worktree branch 'feature-worktree', got %q", state.HeadBranch)
	}
	if state.HeadCommit == nil || state.HeadCommit.Subject != "main commit" {
		t.Errorf("expected HeadCommit 'main commit', got %v", state.HeadCommit)
	}
}
