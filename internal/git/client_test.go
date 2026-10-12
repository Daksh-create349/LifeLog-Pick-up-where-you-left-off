package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lifelog/lifelog/internal/git"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git command failed: %s %v: %s", dir, args, string(out))
	}
	return strings.TrimSpace(string(out))
}

func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.name", "LifeLog Tester")
	runGitCmd(t, dir, "config", "user.email", "tester@lifelog.local")
}

func TestClient_CheckInstalled(t *testing.T) {
	client := git.NewClient()
	if err := client.CheckInstalled(context.Background()); err != nil {
		t.Fatalf("expected git to be installed, got: %v", err)
	}
}

func TestClient_ValidateAndDiscoverRoot(t *testing.T) {
	client := git.NewClient()
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Invalid / non-existent paths
	if _, _, err := client.ValidateAndDiscoverRoot(ctx, "", ""); err == nil {
		t.Error("expected error on empty path, got nil")
	}
	if _, _, err := client.ValidateAndDiscoverRoot(ctx, filepath.Join(tempDir, "non_existent"), ""); err == nil {
		t.Error("expected error on non-existent path, got nil")
	}

	// 2. Non-git directory
	nonGit := filepath.Join(tempDir, "plain_dir")
	if err := os.MkdirAll(nonGit, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.ValidateAndDiscoverRoot(ctx, nonGit, ""); !errors.Is(err, git.ErrNotRepository) {
		t.Errorf("expected ErrNotRepository, got: %v", err)
	}

	// 3. Normal git repo with Unicode and spaces
	unicodeRepo := filepath.Join(tempDir, "Project 🚀 Spaces")
	if err := os.MkdirAll(unicodeRepo, 0700); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, unicodeRepo)

	evalPath := func(p string) string {
		if ep, err := filepath.EvalSymlinks(p); err == nil {
			return ep
		}
		return p
	}

	expectedUnicode := evalPath(unicodeRepo)
	root, isBare, err := client.ValidateAndDiscoverRoot(ctx, unicodeRepo, "")
	if err != nil {
		t.Fatalf("discovery from root failed: %v", err)
	}
	if root != expectedUnicode || isBare {
		t.Errorf("got root=%q isBare=%v, want %q false", root, isBare, expectedUnicode)
	}

	// Discover from nested subdirectory
	subDir := filepath.Join(unicodeRepo, "src", "nested")
	if err := os.MkdirAll(subDir, 0700); err != nil {
		t.Fatal(err)
	}
	rootSub, _, err := client.ValidateAndDiscoverRoot(ctx, subDir, "")
	if err != nil {
		t.Fatalf("discovery from subDir failed: %v", err)
	}
	if rootSub != expectedUnicode {
		t.Errorf("got root=%q, want %q", rootSub, expectedUnicode)
	}

	// 4. Bare repository discovery
	bareRepo := filepath.Join(tempDir, "bare_repo.git")
	runGitCmd(t, tempDir, "clone", "--bare", unicodeRepo, bareRepo)
	rootBare, isBare, err := client.ValidateAndDiscoverRoot(ctx, bareRepo, "")
	if err != nil {
		t.Fatalf("discovery for bare repo failed: %v", err)
	}
	if !isBare {
		t.Error("expected isBare to be true")
	}
	expectedBare := evalPath(bareRepo)
	if rootBare != expectedBare {
		t.Errorf("expected bare repo path %q, got: %q", expectedBare, rootBare)
	}

	// 5. Worktree (.git file) discovery
	wtDir := filepath.Join(tempDir, "worktree_dir")
	runGitCmd(t, unicodeRepo, "worktree", "add", "-b", "wt-branch", wtDir)
	rootWT, _, err := client.ValidateAndDiscoverRoot(ctx, wtDir, "")
	if err != nil {
		t.Fatalf("worktree discovery failed: %v", err)
	}
	expectedWT := evalPath(wtDir)
	if rootWT != expectedWT {
		t.Errorf("got worktree root=%q, want %q", rootWT, expectedWT)
	}
}

func TestClient_Inspect_EmptyRepo(t *testing.T) {
	client := git.NewClient()
	tempDir := t.TempDir()
	initTestRepo(t, tempDir)

	state, err := client.Inspect(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("inspect empty repo failed: %v", err)
	}

	if !state.IsEmpty {
		t.Error("expected IsEmpty to be true")
	}
	if state.HeadCommit != nil {
		t.Errorf("expected nil HeadCommit on empty repo, got %+v", state.HeadCommit)
	}
}

func TestClient_Inspect_FullLifecycle(t *testing.T) {
	client := git.NewClient()
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo_lifecycle")
	if err := os.MkdirAll(repoDir, 0700); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, repoDir)

	// 1. Make first commit
	f1 := filepath.Join(repoDir, "hello.txt")
	if err := os.WriteFile(f1, []byte("version 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repoDir, "add", "hello.txt")
	runGitCmd(t, repoDir, "commit", "-m", "initial commit")

	// Make second commit with Unicode file
	f2 := filepath.Join(repoDir, "doc 📝.txt")
	if err := os.WriteFile(f2, []byte("unicode document\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repoDir, "add", "doc 📝.txt")
	runGitCmd(t, repoDir, "commit", "-m", "add unicode doc")

	ctx := context.Background()
	state1, err := client.Inspect(ctx, repoDir)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if state1.IsEmpty {
		t.Error("expected non-empty repo")
	}
	if len(state1.RecentCommits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(state1.RecentCommits))
	}
	if state1.HeadCommit.Subject != "add unicode doc" {
		t.Errorf("head commit subject: got %q", state1.HeadCommit.Subject)
	}
	if state1.HeadCommit.AuthorName != "LifeLog Tester" {
		t.Errorf("head author: got %q", state1.HeadCommit.AuthorName)
	}

	// 2. Working tree states: staged, modified, untracked, deleted
	// Create staged file
	stagedFile := filepath.Join(repoDir, "staged.txt")
	if err := os.WriteFile(stagedFile, []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repoDir, "add", "staged.txt")

	// Unstaged modification
	if err := os.WriteFile(f1, []byte("version 2 (modified)\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Untracked file
	untrackedFile := filepath.Join(repoDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}

	// Deleted file
	if err := os.Remove(f2); err != nil {
		t.Fatal(err)
	}

	state2, err := client.Inspect(ctx, repoDir)
	if err != nil {
		t.Fatalf("inspect with working tree changes failed: %v", err)
	}

	statuses := make(map[string]git.ChangeStatus)
	for _, c := range state2.WorkingTree {
		statuses[c.Path] = c.Status
	}

	if statuses["staged.txt"] != git.StatusStaged {
		t.Errorf("expected staged.txt to be staged, got %v", statuses["staged.txt"])
	}
	if statuses["hello.txt"] != git.StatusModified {
		t.Errorf("expected hello.txt to be modified, got %v", statuses["hello.txt"])
	}
	if statuses["untracked.txt"] != git.StatusUntracked {
		t.Errorf("expected untracked.txt to be untracked, got %v", statuses["untracked.txt"])
	}
	if statuses["doc 📝.txt"] != git.StatusDeleted {
		t.Errorf("expected doc 📝.txt to be deleted, got %v", statuses["doc 📝.txt"])
	}

	// 3. Detached HEAD detection
	runGitCmd(t, repoDir, "checkout", state1.RecentCommits[1].Hash)
	state3, err := client.Inspect(ctx, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !state3.IsDetached {
		t.Error("expected IsDetached to be true")
	}
	if !strings.Contains(state3.HeadBranch, "detached") {
		t.Errorf("expected detached branch string, got %q", state3.HeadBranch)
	}
}
