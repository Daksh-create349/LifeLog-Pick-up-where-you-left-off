package git

import (
	"errors"
	"time"
)

var (
	ErrNotInstalled   = errors.New("git executable not found in PATH")
	ErrNotRepository  = errors.New("not a git repository")
	ErrRestrictedPath = errors.New("cannot monitor restricted path")
	ErrInvalidPath    = errors.New("invalid repository path")
)

// ChangeStatus describes the working-tree classification of a file.
type ChangeStatus string

const (
	StatusStaged    ChangeStatus = "staged"
	StatusModified  ChangeStatus = "modified"
	StatusUntracked ChangeStatus = "untracked"
	StatusDeleted   ChangeStatus = "deleted"
)

// FileChange represents an observed working-tree modification.
type FileChange struct {
	Path   string       `json:"path"`
	Status ChangeStatus `json:"status"`
}

// Commit holds immutable commit metadata.
type Commit struct {
	Hash       string    `json:"hash"`
	AuthorName string    `json:"author_name"`
	AuthorTime time.Time `json:"author_time"`
	Subject    string    `json:"subject"`
}

// BranchInfo stores branch discovery information.
type BranchInfo struct {
	Name       string `json:"name"`
	CommitHash string `json:"commit_hash"`
}

// RepoState captures an instantaneous snapshot of a Git repository.
type RepoState struct {
	Root          string       `json:"root"`
	IsBare        bool         `json:"is_bare"`
	IsEmpty       bool         `json:"is_empty"`
	HeadBranch    string       `json:"head_branch"`
	IsDetached    bool         `json:"is_detached"`
	HeadCommit    *Commit      `json:"head_commit,omitempty"`
	Branches      []BranchInfo `json:"branches"`
	RecentCommits []Commit     `json:"recent_commits"`
	WorkingTree   []FileChange `json:"working_tree"`
	ObservedAt    time.Time    `json:"observed_at"`
}
