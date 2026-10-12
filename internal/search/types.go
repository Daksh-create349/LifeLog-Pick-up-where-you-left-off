package search

import (
	"time"
)

// Domain represents a searchable memory domain.
type Domain string

const (
	DomainAll        Domain = "all"
	DomainFilesystem Domain = "filesystem"
	DomainGit        Domain = "git"
	DomainTerminal   Domain = "terminal"
	DomainApp        Domain = "app"
	DomainSystem     Domain = "system"
	DomainBrowser    Domain = "browser"
)

// Valid returns true if the domain is supported.
func (d Domain) Valid() bool {
	switch d {
	case DomainAll, DomainFilesystem, DomainGit, DomainTerminal, DomainApp, DomainSystem, DomainBrowser:
		return true
	default:
		return false
	}
}

// Query defines parameters for a cross-domain search.
type Query struct {
	Text          string
	Domain        Domain
	StartTime     *time.Time
	EndTime       *time.Time
	Limit         int
	Offset        int
	Chronological bool
}

// ResultItem represents a single matched record across any memory domain.
type ResultItem struct {
	Domain      Domain            `json:"domain"`
	RecordID    string            `json:"record_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	WorkingPath string            `json:"working_path"`
	Timestamp   time.Time         `json:"timestamp"`
	Score       float64           `json:"score"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// SearchResult bundles result items, original query, and performance metrics.
type SearchResult struct {
	Query      Query        `json:"query"`
	Items      []ResultItem `json:"items"`
	TotalCount int          `json:"total_count"`
	DurationMs int64        `json:"duration_ms"`
}
