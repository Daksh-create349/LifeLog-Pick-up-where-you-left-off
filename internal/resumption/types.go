package resumption

import (
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/git"
	"github.com/lifelog/lifelog/internal/storage"
	"github.com/lifelog/lifelog/internal/terminal"
)

// ActivityItem represents a single atomic observed action across any memory domain.
type ActivityItem struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"` // "filesystem", "git", "terminal"
	Type      string    `json:"type"`   // "fs_created", "fs_modified", "git_commit", "terminal_exec", etc.
	Timestamp time.Time `json:"timestamp"`
	Summary   string    `json:"summary"`
	Details   string    `json:"details"`
	Path      string    `json:"path"`
}

// Session represents a derived contiguous cluster of activity separated by an inactivity gap.
type Session struct {
	ID            string         `json:"id"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	Duration      time.Duration  `json:"duration"`
	ActivityCount int            `json:"activity_count"`
	DomainCounts  map[string]int `json:"domain_counts"`
	Activities    []ActivityItem `json:"activities"`
	Summary       string         `json:"summary"`
}

// PathChange describes an aggregated file path modification within the project.
type PathChange struct {
	Path          string           `json:"path"`
	RelativePath  string           `json:"relative_path"`
	LastEventType events.EventType `json:"last_event_type"`
	EventCount    int              `json:"event_count"`
	LastModTime   time.Time        `json:"last_mod_time"`
}

// ObservedFilesystem captures directly stored filesystem facts.
type ObservedFilesystem struct {
	TotalEvents   int                      `json:"total_events"`
	TypeCounts    map[events.EventType]int `json:"type_counts"`
	RecentChanges []PathChange             `json:"recent_changes"`
}

// ObservedGit captures directly stored Git repository facts.
type ObservedGit struct {
	IsRegistered      bool                         `json:"is_registered"`
	IsGitOnDisk       bool                         `json:"is_git_on_disk"`
	RepoID            string                       `json:"repo_id"`
	RepoPath          string                       `json:"repo_path"`
	CurrentBranch     string                       `json:"current_branch"`
	IsDetached        bool                         `json:"is_detached"`
	HeadCommit        *storage.GitCommitRecord     `json:"head_commit,omitempty"`
	WorkingTree       []git.FileChange             `json:"working_tree"`
	RecentCommits     []storage.GitCommitRecord    `json:"recent_commits"`
	BranchTransitions []storage.GitBranchLogRecord `json:"branch_transitions"`
}

// ObservedTerminal captures directly stored terminal execution facts.
type ObservedTerminal struct {
	TotalCommands  int               `json:"total_commands"`
	RecentCommands []terminal.Record `json:"recent_commands"`
}

// ObservedContext encapsulates all facts directly retrieved from LIFELOG storage.
type ObservedContext struct {
	Filesystem ObservedFilesystem `json:"filesystem"`
	Git        ObservedGit        `json:"git"`
	Terminal   ObservedTerminal   `json:"terminal"`
}

// DerivedContext contains computed sessions, statistics, and evidence-backed inferences.
type DerivedContext struct {
	Sessions          []Session     `json:"sessions"`
	InactivityGap     time.Duration `json:"inactivity_gap"`
	ActivityInference string        `json:"activity_inference"`
}

// ResumptionReport is the unified project-resumption model.
type ResumptionReport struct {
	ProjectPath        string          `json:"project_path"`
	CanonicalPath      string          `json:"canonical_path"`
	PathExists         bool            `json:"path_exists"`
	QueryTime          time.Time       `json:"query_time"`
	TimeWindow         time.Duration   `json:"time_window"`
	Since              *time.Time      `json:"since,omitempty"`
	LatestActivityTime *time.Time      `json:"latest_activity_time,omitempty"`
	Observed           ObservedContext `json:"observed"`
	Derived            DerivedContext  `json:"derived"`
	Unavailable        []string        `json:"unavailable"`
}

// Options configures resumption analysis parameters.
type Options struct {
	Window        time.Duration
	Since         *time.Time
	Limit         int
	InactivityGap time.Duration
}
