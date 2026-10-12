package terminal

import (
	"time"
)

// Record represents a single recorded terminal command execution.
type Record struct {
	ID               string    `json:"id"`
	WorkingDir       string    `json:"working_dir"`
	Executable       string    `json:"executable"`
	SanitizedArgs    []string  `json:"sanitized_args"`
	SanitizedCommand string    `json:"sanitized_command"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	DurationMs       int64     `json:"duration_ms"`
	ExitCode         *int      `json:"exit_code"`       // nil if terminated by signal or failed to launch
	CollectionMode   string    `json:"collection_mode"` // "direct" or "shell"
	IsRedacted       bool      `json:"is_redacted"`
	IsOmitted        bool      `json:"is_omitted"`
	OmissionReason   string    `json:"omission_reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RecordFilter provides filtering options for terminal history queries.
type RecordFilter struct {
	WorkingDir    string
	Limit         int
	Offset        int
	Chronological bool
}
