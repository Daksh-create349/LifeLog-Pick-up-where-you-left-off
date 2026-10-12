package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/lifelog/lifelog/internal/git"
)

var (
	ErrRepoNotFound  = errors.New("repository not found")
	snapSeqCounter   uint64
	branchLogCounter uint64
)

// GitRepoRecord represents a registered Git repository.
type GitRepoRecord struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	IsBare    bool      `json:"is_bare"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GitCommitRecord represents an immutable Git commit record.
type GitCommitRecord struct {
	RepositoryID string    `json:"repository_id"`
	CommitHash   string    `json:"commit_hash"`
	AuthorName   string    `json:"author_name"`
	AuthorTime   time.Time `json:"author_time"`
	Subject      string    `json:"subject"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// GitSnapshotRecord represents an instantaneous working-tree observation.
type GitSnapshotRecord struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	Branch         string    `json:"branch"`
	IsDetached     bool      `json:"is_detached"`
	HeadCommitHash string    `json:"head_commit_hash"`
	ObservedAt     time.Time `json:"observed_at"`
}

// GitBranchLogRecord captures branch tip movement over time.
type GitBranchLogRecord struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	BranchName    string    `json:"branch_name"`
	OldCommitHash string    `json:"old_commit_hash"`
	NewCommitHash string    `json:"new_commit_hash"`
	ObservedAt    time.Time `json:"observed_at"`
}

// GitScanSummary summarizes changes captured during a repository scan.
type GitScanSummary struct {
	RepoID          string
	NewCommitsCount int
	BranchChanges   int
	NewSnapshot     bool
}

func generateRepoID(canonicalPath string) string {
	sum := sha256.Sum256([]byte(canonicalPath))
	return fmt.Sprintf("repo_%s", hex.EncodeToString(sum[:8]))
}

func nextSnapshotID() string {
	seq := atomic.AddUint64(&snapSeqCounter, 1)
	return fmt.Sprintf("snap_%d_%d", time.Now().UTC().UnixNano(), seq)
}

func nextBranchLogID() string {
	seq := atomic.AddUint64(&branchLogCounter, 1)
	return fmt.Sprintf("blog_%d_%d", time.Now().UTC().UnixNano(), seq)
}

// RegisterOrUpdateGitRepo registers a repository canonical path or updates its timestamp if already known.
func (db *DB) RegisterOrUpdateGitRepo(ctx context.Context, canonicalPath string, isBare bool) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	repoID := generateRepoID(canonicalPath)

	query := `
		INSERT INTO git_repositories (id, path, is_bare, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET updated_at = excluded.updated_at
	`
	_, err := db.sql.ExecContext(ctx, query, repoID, canonicalPath, isBare, now, now)
	if err != nil {
		return "", fmt.Errorf("registering repository %s: %w", canonicalPath, err)
	}

	// Retrieve stored ID (handles cases where a different ID existed previously)
	var storedID string
	err = db.sql.QueryRowContext(ctx, "SELECT id FROM git_repositories WHERE path = ?", canonicalPath).Scan(&storedID)
	if err != nil {
		return repoID, nil
	}
	return storedID, nil
}

// GetGitRepo retrieves a repository record by its ID or canonical path.
func (db *DB) GetGitRepo(ctx context.Context, pathOrID string) (*GitRepoRecord, error) {
	query := `
		SELECT id, path, is_bare, created_at, updated_at
		FROM git_repositories
		WHERE id = ? OR path = ?
		LIMIT 1
	`
	var r GitRepoRecord
	var createdStr, updatedStr string
	err := db.sql.QueryRowContext(ctx, query, pathOrID, pathOrID).Scan(&r.ID, &r.Path, &r.IsBare, &createdStr, &updatedStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRepoNotFound
		}
		return nil, fmt.Errorf("retrieving repository %q: %w", pathOrID, err)
	}

	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if r.CreatedAt.IsZero() {
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	}
	return &r, nil
}

// ListGitRepos returns all registered repositories ordered by most recently updated.
func (db *DB) ListGitRepos(ctx context.Context) ([]GitRepoRecord, error) {
	query := `
		SELECT id, path, is_bare, created_at, updated_at
		FROM git_repositories
		ORDER BY updated_at DESC, id ASC
	`
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing git repositories: %w", err)
	}
	defer rows.Close()

	var repos []GitRepoRecord
	for rows.Next() {
		var r GitRepoRecord
		var createdStr, updatedStr string
		if err := rows.Scan(&r.ID, &r.Path, &r.IsBare, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scanning repository row: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if r.CreatedAt.IsZero() {
			r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		if r.UpdatedAt.IsZero() {
			r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

// RecordGitState transactionally records commits, branch updates, and working-tree snapshots.
func (db *DB) RecordGitState(ctx context.Context, repoID string, state *git.RepoState) (*GitScanSummary, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting git state transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	summary := &GitScanSummary{RepoID: repoID}
	observedStr := state.ObservedAt.UTC().Format(time.RFC3339Nano)

	// 1. Update repo updated_at timestamp
	_, err = tx.ExecContext(ctx, "UPDATE git_repositories SET updated_at = ? WHERE id = ?", observedStr, repoID)
	if err != nil {
		return nil, fmt.Errorf("updating repo timestamp: %w", err)
	}

	// 2. Persist commits with deduplication (INSERT OR IGNORE)
	stmtCommit, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO git_commits (repository_id, commit_hash, author_name, author_time, subject, discovered_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing commit insert: %w", err)
	}
	defer stmtCommit.Close()

	for _, c := range state.RecentCommits {
		authorStr := c.AuthorTime.UTC().Format(time.RFC3339Nano)
		res, err := stmtCommit.ExecContext(ctx, repoID, c.Hash, c.AuthorName, authorStr, c.Subject, observedStr)
		if err != nil {
			return nil, fmt.Errorf("inserting commit %s: %w", c.Hash, err)
		}
		if rows, err := res.RowsAffected(); err == nil && rows > 0 {
			summary.NewCommitsCount++
		}
	}

	// 3. Persist branches and branch log
	stmtBranchLog, err := tx.PrepareContext(ctx, `
		INSERT INTO git_branch_log (id, repository_id, branch_name, old_commit_hash, new_commit_hash, observed_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing branch log insert: %w", err)
	}
	defer stmtBranchLog.Close()

	stmtBranchUpsert, err := tx.PrepareContext(ctx, `
		INSERT INTO git_branches (repository_id, name, commit_hash, observed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(repository_id, name) DO UPDATE SET commit_hash = excluded.commit_hash, observed_at = excluded.observed_at
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing branch upsert: %w", err)
	}
	defer stmtBranchUpsert.Close()

	for _, b := range state.Branches {
		var existingHash string
		err := tx.QueryRowContext(ctx, "SELECT commit_hash FROM git_branches WHERE repository_id = ? AND name = ?", repoID, b.Name).Scan(&existingHash)
		if err == nil {
			if existingHash != b.CommitHash {
				logID := nextBranchLogID()
				if _, err := stmtBranchLog.ExecContext(ctx, logID, repoID, b.Name, existingHash, b.CommitHash, observedStr); err != nil {
					return nil, fmt.Errorf("logging branch update: %w", err)
				}
				if _, err := stmtBranchUpsert.ExecContext(ctx, repoID, b.Name, b.CommitHash, observedStr); err != nil {
					return nil, fmt.Errorf("updating branch: %w", err)
				}
				summary.BranchChanges++
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			logID := nextBranchLogID()
			if _, err := stmtBranchLog.ExecContext(ctx, logID, repoID, b.Name, "", b.CommitHash, observedStr); err != nil {
				return nil, fmt.Errorf("logging new branch: %w", err)
			}
			if _, err := stmtBranchUpsert.ExecContext(ctx, repoID, b.Name, b.CommitHash, observedStr); err != nil {
				return nil, fmt.Errorf("inserting branch: %w", err)
			}
			summary.BranchChanges++
		}
	}

	// 4. Snapshot deduplication check
	var headCommitHash string
	if state.HeadCommit != nil {
		headCommitHash = state.HeadCommit.Hash
	}

	shouldCreateSnapshot := true
	var prevSnapID string
	var prevBranch string
	var prevDetached bool
	var prevHeadHash string
	err = tx.QueryRowContext(ctx, `
		SELECT id, branch, is_detached, COALESCE(head_commit_hash, '')
		FROM git_snapshots
		WHERE repository_id = ?
		ORDER BY observed_at DESC, id DESC
		LIMIT 1
	`, repoID).Scan(&prevSnapID, &prevBranch, &prevDetached, &prevHeadHash)
	if err == nil {
		if prevBranch == state.HeadBranch && prevDetached == state.IsDetached && prevHeadHash == headCommitHash {
			// Compare working tree changes with previous snapshot
			prevChanges, err := fetchSnapshotChanges(ctx, tx, prevSnapID)
			if err == nil && fileChangesEqual(prevChanges, state.WorkingTree) {
				shouldCreateSnapshot = false
			}
		}
	}

	if shouldCreateSnapshot {
		snapID := nextSnapshotID()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO git_snapshots (id, repository_id, branch, is_detached, head_commit_hash, observed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, snapID, repoID, state.HeadBranch, state.IsDetached, headCommitHash, observedStr)
		if err != nil {
			return nil, fmt.Errorf("inserting snapshot: %w", err)
		}

		if len(state.WorkingTree) > 0 {
			stmtWT, err := tx.PrepareContext(ctx, `
				INSERT OR IGNORE INTO git_working_tree_changes (snapshot_id, path, status)
				VALUES (?, ?, ?)
			`)
			if err != nil {
				return nil, fmt.Errorf("preparing working tree insert: %w", err)
			}
			defer stmtWT.Close()

			for _, change := range state.WorkingTree {
				if _, err := stmtWT.ExecContext(ctx, snapID, change.Path, string(change.Status)); err != nil {
					return nil, fmt.Errorf("inserting working tree change %s: %w", change.Path, err)
				}
			}
		}
		summary.NewSnapshot = true
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing git state: %w", err)
	}
	return summary, nil
}

func fetchSnapshotChanges(ctx context.Context, tx *sql.Tx, snapshotID string) ([]git.FileChange, error) {
	rows, err := tx.QueryContext(ctx, "SELECT path, status FROM git_working_tree_changes WHERE snapshot_id = ? ORDER BY path ASC", snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []git.FileChange
	for rows.Next() {
		var c git.FileChange
		var statusStr string
		if err := rows.Scan(&c.Path, &statusStr); err != nil {
			return nil, err
		}
		c.Status = git.ChangeStatus(statusStr)
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

func fileChangesEqual(a, b []git.FileChange) bool {
	if len(a) != len(b) {
		return false
	}
	sortChanges := func(src []git.FileChange) []git.FileChange {
		res := make([]git.FileChange, len(src))
		copy(res, src)
		sort.Slice(res, func(i, j int) bool {
			if res[i].Path == res[j].Path {
				return res[i].Status < res[j].Status
			}
			return res[i].Path < res[j].Path
		})
		return res
	}
	sa := sortChanges(a)
	sb := sortChanges(b)
	for i := range sa {
		if sa[i].Path != sb[i].Path || sa[i].Status != sb[i].Status {
			return false
		}
	}
	return true
}

// QueryGitCommits returns commits recorded for a repository in reverse-chronological order.
func (db *DB) QueryGitCommits(ctx context.Context, repoID string, limit int) ([]GitCommitRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT repository_id, commit_hash, author_name, author_time, subject, discovered_at
		FROM git_commits
		WHERE repository_id = ?
		ORDER BY author_time DESC, commit_hash DESC
		LIMIT ?
	`
	rows, err := db.sql.QueryContext(ctx, query, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying git commits: %w", err)
	}
	defer rows.Close()

	var commits []GitCommitRecord
	for rows.Next() {
		var c GitCommitRecord
		var authorStr, discoveredStr string
		if err := rows.Scan(&c.RepositoryID, &c.CommitHash, &c.AuthorName, &authorStr, &c.Subject, &discoveredStr); err != nil {
			return nil, fmt.Errorf("scanning commit row: %w", err)
		}
		c.AuthorTime, _ = time.Parse(time.RFC3339Nano, authorStr)
		if c.AuthorTime.IsZero() {
			c.AuthorTime, _ = time.Parse(time.RFC3339, authorStr)
		}
		c.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredStr)
		if c.DiscoveredAt.IsZero() {
			c.DiscoveredAt, _ = time.Parse(time.RFC3339, discoveredStr)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// GetLatestGitSnapshot returns the most recent snapshot and its working tree changes.
func (db *DB) GetLatestGitSnapshot(ctx context.Context, repoID string) (*GitSnapshotRecord, []git.FileChange, error) {
	var s GitSnapshotRecord
	var observedStr string
	query := `
		SELECT id, repository_id, branch, is_detached, COALESCE(head_commit_hash, ''), observed_at
		FROM git_snapshots
		WHERE repository_id = ?
		ORDER BY observed_at DESC, id DESC
		LIMIT 1
	`
	err := db.sql.QueryRowContext(ctx, query, repoID).Scan(&s.ID, &s.RepositoryID, &s.Branch, &s.IsDetached, &s.HeadCommitHash, &observedStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("retrieving latest snapshot: %w", err)
	}
	s.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedStr)
	if s.ObservedAt.IsZero() {
		s.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)
	}

	rows, err := db.sql.QueryContext(ctx, "SELECT path, status FROM git_working_tree_changes WHERE snapshot_id = ? ORDER BY path ASC", s.ID)
	if err != nil {
		return &s, nil, fmt.Errorf("retrieving working tree changes: %w", err)
	}
	defer rows.Close()

	var changes []git.FileChange
	for rows.Next() {
		var c git.FileChange
		var statusStr string
		if err := rows.Scan(&c.Path, &statusStr); err != nil {
			return &s, nil, err
		}
		c.Status = git.ChangeStatus(statusStr)
		changes = append(changes, c)
	}
	return &s, changes, rows.Err()
}

// QueryGitBranches returns all currently discovered branches for a repository.
func (db *DB) QueryGitBranches(ctx context.Context, repoID string) ([]git.BranchInfo, error) {
	rows, err := db.sql.QueryContext(ctx, "SELECT name, commit_hash FROM git_branches WHERE repository_id = ? ORDER BY name ASC", repoID)
	if err != nil {
		return nil, fmt.Errorf("querying git branches: %w", err)
	}
	defer rows.Close()

	var branches []git.BranchInfo
	for rows.Next() {
		var b git.BranchInfo
		if err := rows.Scan(&b.Name, &b.CommitHash); err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// QueryGitBranchLog returns historical branch tip movements in reverse-chronological order.
func (db *DB) QueryGitBranchLog(ctx context.Context, repoID string, limit int) ([]GitBranchLogRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, repository_id, branch_name, old_commit_hash, new_commit_hash, observed_at
		FROM git_branch_log
		WHERE repository_id = ?
		ORDER BY observed_at DESC, id DESC
		LIMIT ?
	`
	rows, err := db.sql.QueryContext(ctx, query, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying branch log: %w", err)
	}
	defer rows.Close()

	var logs []GitBranchLogRecord
	for rows.Next() {
		var l GitBranchLogRecord
		var observedStr string
		if err := rows.Scan(&l.ID, &l.RepositoryID, &l.BranchName, &l.OldCommitHash, &l.NewCommitHash, &observedStr); err != nil {
			return nil, err
		}
		l.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedStr)
		if l.ObservedAt.IsZero() {
			l.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
