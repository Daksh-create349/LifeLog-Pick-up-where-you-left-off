package ipc

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lifelog/lifelog/internal/paths"
	"github.com/lifelog/lifelog/internal/resumption"
	"github.com/lifelog/lifelog/internal/search"
	"github.com/lifelog/lifelog/internal/storage"
)

func handleAuth(sess *session, server *Server, params json.RawMessage) (any, *RPCError) {
	if sess.authenticated {
		return AuthResult{
			Authenticated: true,
			SessionID:     sess.id,
			Version:       server.cfg.Version,
		}, nil
	}

	var authParams AuthParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &authParams); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed auth parameters: %v", err),
			}
		}
	}

	if authParams.Token == "" {
		return nil, &RPCError{
			Code:    CodeUnauthorized,
			Message: "authentication token required",
		}
	}

	// Constant-time token verification
	if subtle.ConstantTimeCompare([]byte(authParams.Token), []byte(server.cfg.AuthToken)) != 1 {
		return nil, &RPCError{
			Code:    CodeUnauthorized,
			Message: "invalid authentication token",
		}
	}

	sess.authenticated = true
	return AuthResult{
		Authenticated: true,
		SessionID:     sess.id,
		Version:       server.cfg.Version,
	}, nil
}

func handleHealth(ctx context.Context, server *Server) (any, *RPCError) {
	if err := server.cfg.DB.HealthCheck(ctx); err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("database health check failed: %v", err),
		}
	}

	schemaVer, err := server.cfg.DB.SchemaVersion(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("querying schema version: %v", err),
		}
	}

	uptime := int64(time.Since(server.startTime).Seconds())
	return HealthResult{
		ProtocolVersion: ProtocolVersion,
		DaemonStatus:    "healthy",
		Version:         server.cfg.Version,
		SchemaVersion:   schemaVer,
		UptimeSeconds:   uptime,
		DBPath:          server.cfg.DB.Path(),
		PID:             os.Getpid(),
	}, nil
}

func handleGetProjects(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetProjectsParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getProjects parameters: %v", err),
			}
		}
	}

	// 1. Sort validation
	sortMode := storage.SortRecent
	if p.Sort != "" {
		switch strings.ToLower(strings.TrimSpace(p.Sort)) {
		case "recent":
			sortMode = storage.SortRecent
		case "name":
			sortMode = storage.SortName
		case "activity":
			sortMode = storage.SortActivity
		default:
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid sort %q (must be recent, name, activity)", p.Sort),
			}
		}
	}

	// 2. Limit validation
	limit := 50
	if p.Limit != 0 {
		if p.Limit < 1 || p.Limit > 500 {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("limit must be between 1 and 500 (got %d)", p.Limit),
			}
		}
		limit = p.Limit
	}

	// 3. Since / Window parsing
	var sinceTime *time.Time
	var windowDuration time.Duration

	if p.Since != "" {
		t, err := parseIPCTime(p.Since, true)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid since timestamp %q: %v", p.Since, err),
			}
		}
		sinceTime = t
	} else if p.Window != "" && p.Window != "0" && strings.ToLower(p.Window) != "all" {
		w, err := parseWindowDuration(p.Window)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid window %q: %v", p.Window, err),
			}
		}
		windowDuration = w
	}

	opts := storage.ProjectOverviewOptions{
		SortMode: sortMode,
		Limit:    limit,
		Window:   windowDuration,
		Since:    sinceTime,
	}

	projects, err := server.cfg.DB.DiscoverProjects(ctx, opts)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("discovering projects: %v", err),
		}
	}

	if projects == nil {
		projects = []storage.ProjectOverview{}
	}

	return GetProjectsResult{
		Projects: projects,
		Count:    len(projects),
	}, nil
}

func handleGetTimeline(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetTimelineParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getTimeline parameters: %v", err),
			}
		}
	}

	// 1. Domain validation
	dom := search.DomainAll
	if p.Domain != "" {
		d := search.Domain(strings.ToLower(strings.TrimSpace(p.Domain)))
		if !d.Valid() {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid domain %q (must be all, filesystem, git, terminal)", p.Domain),
			}
		}
		dom = d
	}

	// 2. Limit validation
	limit := 20
	if p.Limit != 0 {
		if p.Limit < 1 || p.Limit > 500 {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("limit must be between 1 and 500 (got %d)", p.Limit),
			}
		}
		limit = p.Limit
	}

	// 3. Offset validation
	if p.Offset < 0 {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("offset must be non-negative (got %d)", p.Offset),
		}
	}

	// 4. Since / Until timestamps
	var startTime, endTime *time.Time
	if p.Since != "" {
		t, err := parseIPCTime(p.Since, true)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid since parameter %q: %v", p.Since, err),
			}
		}
		startTime = t
	}
	if p.Until != "" {
		t, err := parseIPCTime(p.Until, false)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid until parameter %q: %v", p.Until, err),
			}
		}
		endTime = t
	}

	query := search.Query{
		Text:          "",
		Domain:        dom,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         limit,
		Offset:        p.Offset,
		Chronological: true,
	}

	result, err := server.cfg.DB.Search(ctx, query)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("retrieving timeline: %v", err),
		}
	}

	items := result.Items
	if items == nil {
		items = []search.ResultItem{}
	}

	return GetTimelineResult{
		Items:      items,
		TotalCount: result.TotalCount,
		DurationMs: result.DurationMs,
	}, nil
}

func handleGetResumption(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetResumptionParams
	if len(params) == 0 || string(params) == "null" {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: "getResumption requires parameters with project_path",
		}
	}

	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("malformed getResumption parameters: %v", err),
		}
	}

	if strings.TrimSpace(p.ProjectPath) == "" {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: "project_path cannot be empty",
		}
	}

	// Validate path canonicalization
	canonPath, err := paths.Canonical(p.ProjectPath)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("invalid project_path %q: %v", p.ProjectPath, err),
		}
	}

	// Limit validation
	limit := 20
	if p.Limit != 0 {
		if p.Limit < 1 || p.Limit > 500 {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("limit must be between 1 and 500 (got %d)", p.Limit),
			}
		}
		limit = p.Limit
	}

	// Gap validation
	gap := 30 * time.Minute
	if p.Gap != "" {
		d, err := time.ParseDuration(p.Gap)
		if err != nil || d <= 0 {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid gap duration %q: must be positive duration (e.g. 15m, 30m, 1h)", p.Gap),
			}
		}
		gap = d
	}

	// Since / Window parsing
	var sinceTime *time.Time
	var windowDuration time.Duration

	if p.Since != "" {
		t, err := parseIPCTime(p.Since, true)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid since parameter %q: %v", p.Since, err),
			}
		}
		sinceTime = t
	} else if p.Window != "" && p.Window != "0" && strings.ToLower(p.Window) != "all" {
		w, err := parseWindowDuration(p.Window)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid window %q: %v", p.Window, err),
			}
		}
		windowDuration = w
	}

	opts := resumption.Options{
		Window:        windowDuration,
		Since:         sinceTime,
		Limit:         limit,
		InactivityGap: gap,
	}

	report, err := resumption.BuildResumptionReport(ctx, server.cfg.DB, canonPath, opts)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("building resumption report: %v", err),
		}
	}

	return GetResumptionResult{
		Report: report,
	}, nil
}

func parseWindowDuration(windowStr string) (time.Duration, error) {
	lower := strings.ToLower(strings.TrimSpace(windowStr))
	if strings.HasSuffix(lower, "d") {
		var days int
		if _, err := fmt.Sscanf(lower[:len(lower)-1], "%d", &days); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
		return 0, fmt.Errorf("invalid day duration %q", windowStr)
	}
	d, err := time.ParseDuration(lower)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q (use 24h, 7d, 30d, or 0)", windowStr)
	}
	return d, nil
}

func parseIPCTime(s string, isSince bool) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	// Check relative duration for since queries (e.g. 24h, 7d)
	if isSince {
		lower := strings.ToLower(s)
		if strings.HasSuffix(lower, "d") {
			daysStr := lower[:len(lower)-1]
			var days int
			if _, err := fmt.Sscanf(daysStr, "%d", &days); err == nil && days > 0 {
				t := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
				return &t, nil
			}
		}

		if d, err := time.ParseDuration(lower); err == nil && d > 0 {
			t := time.Now().UTC().Add(-d)
			return &t, nil
		}
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, s); err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}

	return nil, fmt.Errorf("unrecognized time format (supported: RFC3339, YYYY-MM-DD, or duration like 24h, 7d)")
}

func handleGetAppUsage(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetAppUsageParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getAppUsage parameters: %v", err),
			}
		}
	}

	var sinceTime *time.Time
	if p.Window != "" {
		w, err := parseWindowDuration(p.Window)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid window %q: %v", p.Window, err),
			}
		}
		t := time.Now().UTC().Add(-w)
		sinceTime = &t
	} else if p.Since != "" {
		st, err := parseIPCTime(p.Since, true)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid since %q: %v", p.Since, err),
			}
		}
		sinceTime = st
	}

	var untilTime *time.Time
	if p.Until != "" {
		ut, err := parseIPCTime(p.Until, false)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid until %q: %v", p.Until, err),
			}
		}
		untilTime = ut
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	filter := storage.AppUsageFilter{
		Since: sinceTime,
		Until: untilTime,
		Limit: limit,
	}

	summary, err := server.cfg.DB.GetAppUsage(ctx, filter)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("calculating app usage: %v", err),
		}
	}

	appsList := summary.Apps
	if appsList == nil {
		appsList = []storage.AppUsageStat{}
	}

	return GetAppUsageResult{
		TotalDurationMs: summary.TotalDurationMs,
		TotalSessions:   summary.TotalSessions,
		PeriodStart:     summary.Since.Format(time.RFC3339),
		PeriodEnd:       summary.Until.Format(time.RFC3339),
		Apps:            appsList,
	}, nil
}

func handleGetAppSessions(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetAppSessionsParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getAppSessions parameters: %v", err),
			}
		}
	}

	var sinceTime *time.Time
	if p.Window != "" {
		w, err := parseWindowDuration(p.Window)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid window %q: %v", p.Window, err),
			}
		}
		t := time.Now().UTC().Add(-w)
		sinceTime = &t
	} else if p.Since != "" {
		st, err := parseIPCTime(p.Since, true)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid since %q: %v", p.Since, err),
			}
		}
		sinceTime = st
	}

	var untilTime *time.Time
	if p.Until != "" {
		ut, err := parseIPCTime(p.Until, false)
		if err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("invalid until %q: %v", p.Until, err),
			}
		}
		untilTime = ut
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	filter := storage.AppSessionFilter{
		Since:    sinceTime,
		Until:    untilTime,
		BundleID: p.BundleID,
		Limit:    limit,
		Offset:   offset,
	}

	sessions, totalCount, err := server.cfg.DB.GetAppSessions(ctx, filter)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("querying app sessions: %v", err),
		}
	}

	if sessions == nil {
		sessions = []storage.AppSession{}
	}

	return GetAppSessionsResult{
		Sessions:   sessions,
		TotalCount: totalCount,
	}, nil
}

func handleGetTrackingStatus(ctx context.Context, server *Server) (any, *RPCError) {
	browserPref, _ := server.cfg.DB.GetPreference(ctx, "browser_tracking_enabled", "false")
	browserEnabled := browserPref == "true"

	if server.cfg.AppSupervisor == nil {
		return TrackingStatusResult{
			AppTrackingEnabled:     false,
			ObserverStatus:         "unavailable",
			SystemTrackingEnabled:  false,
			BrowserTrackingEnabled: browserEnabled,
			BrowserTrackingStatus:  "disabled",
		}, nil
	}

	st := server.cfg.AppSupervisor.Status()
	res := TrackingStatusResult{
		AppTrackingEnabled:     st.Enabled,
		ObserverStatus:         st.Status,
		ActiveApp:              st.ActiveApp,
		ActiveBundle:           st.ActiveBundle,
		ActiveDurationMs:       st.ActiveDurationMs,
		SystemTrackingEnabled:  st.Enabled,
		BrowserTrackingEnabled: browserEnabled,
		BrowserTrackingStatus:  "disabled",
	}
	if st.ActiveSince != nil {
		s := st.ActiveSince.Format(time.RFC3339)
		res.ActiveSince = &s
	}

	return res, nil
}

func handleSetAppTracking(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p SetAppTrackingParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed setAppTracking parameters: %v", err),
			}
		}
	}

	if server.cfg.AppSupervisor == nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: "app tracking observer supervisor is not configured",
		}
	}

	if err := server.cfg.AppSupervisor.SetEnabled(p.Enabled); err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("setting app tracking enabled state: %v", err),
		}
	}

	st, rpcErr := handleGetTrackingStatus(ctx, server)
	if rpcErr != nil {
		return nil, rpcErr
	}

	trackingStatus, ok := st.(TrackingStatusResult)
	if !ok {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: "failed casting tracking status result",
		}
	}

	return SetAppTrackingResult{
		Status: trackingStatus,
	}, nil
}

func handleGetSystemEvents(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetSystemEventsParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getSystemEvents parameters: %v", err),
			}
		}
	}

	filter := storage.SystemEventFilter{
		EventType: storage.SystemEventType(p.EventType),
		Limit:     p.Limit,
		Offset:    p.Offset,
	}
	if p.Since != "" {
		if t, err := time.Parse(time.RFC3339, p.Since); err == nil {
			filter.Since = &t
		}
	}
	if p.Until != "" {
		if t, err := time.Parse(time.RFC3339, p.Until); err == nil {
			filter.Until = &t
		}
	}

	events, err := server.cfg.DB.GetSystemEvents(ctx, filter)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("querying system events: %v", err),
		}
	}
	if events == nil {
		events = []*storage.SystemEvent{}
	}

	return GetSystemEventsResult{
		Events: events,
		Count:  len(events),
	}, nil
}

func handleGetSystemActivitySummary(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p GetSystemActivitySummaryParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed getSystemActivitySummary parameters: %v", err),
			}
		}
	}

	var since, until time.Time
	if p.Until != "" {
		if t, err := time.Parse(time.RFC3339, p.Until); err == nil {
			until = t
		}
	}
	if until.IsZero() {
		until = time.Now().UTC()
	}

	if p.Since != "" {
		if t, err := time.Parse(time.RFC3339, p.Since); err == nil {
			since = t
		}
	}
	if since.IsZero() {
		since = until.Add(-24 * time.Hour)
	}

	summary, err := server.cfg.DB.GetSystemActivitySummary(ctx, since, until)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("getting system activity summary: %v", err),
		}
	}
	return summary, nil
}

func handleGetBrowserTrackingStatus(ctx context.Context, server *Server) (any, *RPCError) {
	pref, _ := server.cfg.DB.GetPreference(ctx, "browser_tracking_enabled", "false")
	enabled := pref == "true"
	return GetBrowserTrackingStatusResult{
		Enabled: enabled,
		Status:  "disabled",
		Reason:  "Browser domain tracking requires optional LIFELOG native extension. Window-title scraping is strictly prohibited for privacy.",
	}, nil
}

func handleSetBrowserTracking(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p SetBrowserTrackingParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed setBrowserTracking parameters: %v", err),
			}
		}
	}

	val := "false"
	if p.Enabled {
		val = "true"
	}
	_ = server.cfg.DB.SetPreference(ctx, "browser_tracking_enabled", val)

	return SetBrowserTrackingResult{
		Status: GetBrowserTrackingStatusResult{
			Enabled: p.Enabled,
			Status:  "disabled",
			Reason:  "Browser domain tracking requires optional LIFELOG native extension. Window-title scraping is strictly prohibited for privacy.",
		},
	}, nil
}

func handleGetDataStats(ctx context.Context, server *Server) (any, *RPCError) {
	stats, err := server.cfg.DB.GetDataStats(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("getting data stats: %v", err),
		}
	}
	return GetDataStatsResult{Stats: stats}, nil
}

func handleGetRetentionPolicy(ctx context.Context, server *Server) (any, *RPCError) {
	stats, err := server.cfg.DB.GetDataStats(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("getting retention policy: %v", err),
		}
	}
	return GetRetentionPolicyResult{
		RetentionDays:        stats.RetentionDays,
		LastRetentionCleanup: stats.LastRetentionCleanup,
	}, nil
}

func handleSetRetentionPolicy(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p SetRetentionPolicyParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed setRetentionPolicy parameters: %v", err),
			}
		}
	}

	if p.RetentionDays < 0 {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: "retention days must be 0 (forever) or positive integer",
		}
	}

	_ = server.cfg.DB.SetPreference(ctx, "retention_days", fmt.Sprintf("%d", p.RetentionDays))

	return SetRetentionPolicyResult{
		Success:       true,
		RetentionDays: p.RetentionDays,
	}, nil
}

func handleRunRetentionCleanup(ctx context.Context, server *Server) (any, *RPCError) {
	stats, err := server.cfg.DB.GetDataStats(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("fetching retention policy: %v", err),
		}
	}

	purgeStats, err := server.cfg.DB.PurgeRetention(ctx, stats.RetentionDays)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("running retention cleanup: %v", err),
		}
	}

	return RunRetentionCleanupResult{PurgeStats: purgeStats}, nil
}

func handleDeleteActivity(ctx context.Context, server *Server, params json.RawMessage) (any, *RPCError) {
	var p DeleteActivityParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("malformed deleteActivity parameters: %v", err),
			}
		}
	}

	if p.Source == "" {
		return nil, &RPCError{
			Code:    CodeInvalidParams,
			Message: "source parameter required (e.g. all, apps, system, browser, filesystem, terminal, git)",
		}
	}

	var sincePtr, untilPtr *time.Time
	if p.Since != "" {
		if t, err := time.Parse(time.RFC3339, p.Since); err == nil {
			sincePtr = &t
		}
	}
	if p.Until != "" {
		if t, err := time.Parse(time.RFC3339, p.Until); err == nil {
			untilPtr = &t
		}
	}

	deleted, err := server.cfg.DB.DeleteActivity(ctx, p.Source, sincePtr, untilPtr)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("deleting activity records: %v", err),
		}
	}

	return DeleteActivityResult{
		Success:      true,
		Source:       p.Source,
		DeletedCount: deleted,
	}, nil
}

func handleExportUserData(ctx context.Context, server *Server) (any, *RPCError) {
	export, err := server.cfg.DB.ExportUserData(ctx)
	if err != nil {
		return nil, &RPCError{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("exporting user data: %v", err),
		}
	}
	return ExportUserDataResult{Export: export}, nil
}
