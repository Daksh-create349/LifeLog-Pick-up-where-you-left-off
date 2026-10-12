package ipc

import (
	"encoding/json"
	"time"

	"github.com/lifelog/lifelog/internal/resumption"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

// ProtocolVersion is the active JSON-RPC 2.0 protocol version for LIFELOG IPC.
const ProtocolVersion = "2.0"

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// Custom server error codes (-32000 to -32099)
	CodeUnauthorized       = -32001
	CodeResourceExceeded   = -32002
	CodeServerShuttingDown = -32003
)

// Request defines a JSON-RPC 2.0 request payload.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response defines a JSON-RPC 2.0 response payload.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a structured JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the standard Go error interface for RPCError.
func (e *RPCError) Error() string {
	return e.Message
}

// AuthParams contains the handshake authentication parameters.
type AuthParams struct {
	Token  string `json:"token"`
	Client string `json:"client,omitempty"`
}

// AuthResult represents the response for a successful auth handshake.
type AuthResult struct {
	Authenticated bool   `json:"authenticated"`
	SessionID     string `json:"session_id"`
	Version       string `json:"version"`
}

// HealthResult represents the daemon and database health check report.
type HealthResult struct {
	ProtocolVersion string `json:"protocol_version"`
	DaemonStatus    string `json:"daemon_status"`
	Version         string `json:"version"`
	SchemaVersion   int    `json:"schema_version"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	DBPath          string `json:"db_path"`
	PID             int    `json:"pid"`
}

// GetProjectsParams configures project discovery options.
type GetProjectsParams struct {
	Sort   string `json:"sort,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Window string `json:"window,omitempty"`
	Since  string `json:"since,omitempty"`
}

// GetProjectsResult returns discovered projects.
type GetProjectsResult struct {
	Projects []storage.ProjectOverview `json:"projects"`
	Count    int                       `json:"count"`
}

// GetTimelineParams configures activity timeline query parameters.
type GetTimelineParams struct {
	Domain string `json:"domain,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Since  string `json:"since,omitempty"`
	Until  string `json:"until,omitempty"`
}

// GetTimelineResult returns queried timeline records.
type GetTimelineResult struct {
	Items      []search.ResultItem `json:"items"`
	TotalCount int                 `json:"total_count"`
	DurationMs int64               `json:"duration_ms"`
}

// GetResumptionParams configures project resumption synthesis.
type GetResumptionParams struct {
	ProjectPath string `json:"project_path"`
	Limit       int    `json:"limit,omitempty"`
	Window      string `json:"window,omitempty"`
	Since       string `json:"since,omitempty"`
	Gap         string `json:"gap,omitempty"`
}

// GetResumptionResult returns the synthesized resumption report.
type GetResumptionResult struct {
	Report *resumption.ResumptionReport `json:"report"`
}

// GetAppUsageParams configures app usage aggregation query.
type GetAppUsageParams struct {
	Window string `json:"window,omitempty"`
	Since  string `json:"since,omitempty"`
	Until  string `json:"until,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// GetAppUsageResult returns aggregated application usage.
type GetAppUsageResult struct {
	TotalDurationMs int64                  `json:"total_duration_ms"`
	TotalSessions   int                    `json:"total_sessions"`
	PeriodStart     string                 `json:"period_start"`
	PeriodEnd       string                 `json:"period_end"`
	Apps            []storage.AppUsageStat `json:"apps"`
}

// GetAppSessionsParams configures paginated individual session querying.
type GetAppSessionsParams struct {
	Window   string `json:"window,omitempty"`
	Since    string `json:"since,omitempty"`
	Until    string `json:"until,omitempty"`
	BundleID string `json:"bundle_id,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// GetAppSessionsResult returns individual application sessions.
type GetAppSessionsResult struct {
	Sessions   []storage.AppSession `json:"sessions"`
	TotalCount int                  `json:"total_count"`
}

// SetAppTrackingParams configures app tracking state.
type SetAppTrackingParams struct {
	Enabled bool `json:"enabled"`
}

// TrackingStatusResult returns per-source observer state.
type TrackingStatusResult struct {
	AppTrackingEnabled     bool    `json:"app_tracking_enabled"`
	ObserverStatus         string  `json:"observer_status"` // "active", "paused", "crashed", "unavailable"
	ActiveApp              string  `json:"active_app,omitempty"`
	ActiveBundle           string  `json:"active_bundle,omitempty"`
	ActiveSince            *string `json:"active_since,omitempty"`
	ActiveDurationMs       int64   `json:"active_duration_ms,omitempty"`
	SystemTrackingEnabled  bool    `json:"system_tracking_enabled"`
	BrowserTrackingEnabled bool    `json:"browser_tracking_enabled"`
	BrowserTrackingStatus  string  `json:"browser_tracking_status"`
}

// SetAppTrackingResult returns updated tracking status.
type SetAppTrackingResult struct {
	Status TrackingStatusResult `json:"status"`
}

// GetSystemEventsParams configures system activity event querying.
type GetSystemEventsParams struct {
	EventType string `json:"event_type,omitempty"`
	Since     string `json:"since,omitempty"`
	Until     string `json:"until,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// GetSystemEventsResult returns system activity events.
type GetSystemEventsResult struct {
	Events []*storage.SystemEvent `json:"events"`
	Count  int                    `json:"count"`
}

// GetSystemActivitySummaryParams configures system activity summary query.
type GetSystemActivitySummaryParams struct {
	Since string `json:"since,omitempty"`
	Until string `json:"until,omitempty"`
}

// GetBrowserTrackingStatusResult returns browser tracking privacy and operational state.
type GetBrowserTrackingStatusResult struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"` // "disabled", "unavailable"
	Reason  string `json:"reason"`
}

// SetBrowserTrackingParams configures browser tracking state.
type SetBrowserTrackingParams struct {
	Enabled bool `json:"enabled"`
}

// SetBrowserTrackingResult returns updated browser tracking state.
type SetBrowserTrackingResult struct {
	Status GetBrowserTrackingStatusResult `json:"status"`
}

// GetDataStatsResult returns local database metrics and record counts.
type GetDataStatsResult struct {
	Stats *storage.DataStats `json:"stats"`
}

// GetRetentionPolicyResult returns the active data retention policy.
type GetRetentionPolicyResult struct {
	RetentionDays        int        `json:"retention_days"`
	LastRetentionCleanup *time.Time `json:"last_retention_cleanup,omitempty"`
}

// SetRetentionPolicyParams configures data retention period.
type SetRetentionPolicyParams struct {
	RetentionDays int `json:"retention_days"`
}

// SetRetentionPolicyResult returns the updated retention policy.
type SetRetentionPolicyResult struct {
	Success       bool `json:"success"`
	RetentionDays int  `json:"retention_days"`
}

// RunRetentionCleanupResult returns counts of purged records.
type RunRetentionCleanupResult struct {
	PurgeStats *storage.PurgeStats `json:"purge_stats"`
}

// DeleteActivityParams configures selective or full activity deletion.
type DeleteActivityParams struct {
	Source string `json:"source"` // "all", "apps", "system", "browser", "filesystem", "terminal", "git"
	Since  string `json:"since,omitempty"`
	Until  string `json:"until,omitempty"`
}

// DeleteActivityResult returns deletion confirmation and count.
type DeleteActivityResult struct {
	Success      bool   `json:"success"`
	Source       string `json:"source"`
	DeletedCount int64  `json:"deleted_count"`
}

// ExportUserDataResult returns full structured JSON user data.
type ExportUserDataResult struct {
	Export *storage.UserDataExport `json:"export"`
}
