export interface HealthResult {
  protocol_version: string;
  daemon_status: string;
  version: string;
  schema_version: number;
  uptime_seconds: number;
  db_path: string;
  pid: number;
}

export interface ProjectOverview {
  canonical_path: string;
  display_name: string;
  exists_on_disk: boolean;
  is_git_registered: boolean;
  git_repo_id?: string;
  git_branch?: string;
  last_fs_activity?: string;
  last_terminal_activity?: string;
  last_git_activity?: string;
  latest_activity?: string;
  domain_counts: {
    filesystem: number;
    git: number;
    terminal: number;
  };
  total_events: number;
  checkpoint_count: number;
  has_checkpoint: boolean;
}

export interface GetProjectsOptions {
  sort?: 'recent' | 'name' | 'activity';
  limit?: number;
  window?: string;
  since?: string;
}

export interface GetProjectsResult {
  projects: ProjectOverview[];
  count: number;
}

export interface TimelineItem {
  domain: string;
  record_id: string;
  title: string;
  description?: string;
  working_path?: string;
  timestamp: string;
  score?: number;
  metadata?: Record<string, string>;
}

export interface GetTimelineOptions {
  domain?: 'all' | 'filesystem' | 'git' | 'terminal';
  limit?: number;
  offset?: number;
  since?: string;
  until?: string;
}

export interface GetTimelineResult {
  items: TimelineItem[];
  total_count: number;
  duration_ms: number;
}

export interface ResumptionSession {
  id: string;
  start_time: string;
  end_time: string;
  duration: number; // nanoseconds or duration
  activity_count: number;
  domain_counts: Record<string, number>;
  summary: string;
}

export interface PathChange {
  path: string;
  relative_path: string;
  last_event_type: string;
  event_count: number;
  last_mod_time: string;
}

export interface ResumptionReport {
  project_path: string;
  canonical_path: string;
  path_exists: boolean;
  query_time: string;
  time_window: number;
  since?: string;
  latest_activity_time?: string;
  observed: {
    filesystem: {
      total_events: number;
      type_counts: Record<string, number>;
      recent_changes: PathChange[];
    };
    git: {
      is_registered: boolean;
      is_git_on_disk: boolean;
      repo_id: string;
      repo_path: string;
      current_branch: string;
      is_detached: boolean;
      head_commit?: {
        commit_hash: string;
        author_name: string;
        author_time: string;
        subject: string;
      };
      working_tree: Array<{ path: string; status: string }>;
      recent_commits: Array<{
        commit_hash: string;
        author_name: string;
        author_time: string;
        subject: string;
      }>;
      branch_transitions: Array<{
        branch_name: string;
        old_commit_hash?: string;
        new_commit_hash: string;
        observed_at: string;
      }>;
    };
    terminal: {
      total_commands: number;
      recent_commands: Array<{
        id: string;
        sanitized_command: string;
        executable: string;
        working_dir: string;
        exit_code: number;
        start_time: string;
        duration_ms: number;
      }>;
    };
  };
  derived: {
    sessions: ResumptionSession[];
    inactivity_gap: number;
    activity_inference: string;
  };
  unavailable: string[];
}

export interface GetResumptionOptions {
  limit?: number;
  window?: string;
  since?: string;
  gap?: string;
}

export interface GetResumptionResult {
  report: ResumptionReport;
}

export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

export interface ConnectionStatus {
  state: ConnectionState;
  error?: string;
  health?: HealthResult;
  socketPath?: string;
  pid?: number;
}

export interface AppUsageStat {
  app_bundle_id: string;
  app_name: string;
  total_duration_ms: number;
  session_count: number;
  percentage: number;
}

export interface GetAppUsageResult {
  total_duration_ms: number;
  total_sessions: number;
  period_start: string;
  period_end: string;
  apps: AppUsageStat[];
}

export interface GetAppUsageOptions {
  window?: string;
  since?: string;
  until?: string;
  limit?: number;
}

export interface AppSession {
  id: string;
  app_bundle_id: string;
  app_name: string;
  start_time: string;
  end_time: string;
  duration_ms: number;
  is_active: boolean;
  created_at?: string;
}

export interface GetAppSessionsOptions {
  window?: string;
  since?: string;
  until?: string;
  bundle_id?: string;
  limit?: number;
  offset?: number;
}

export interface GetAppSessionsResult {
  sessions: AppSession[];
  total_count: number;
}

export interface TrackingStatus {
  app_tracking_enabled: boolean;
  observer_status: 'active' | 'paused' | 'crashed' | 'unavailable';
  active_app?: string;
  active_bundle?: string;
  active_since?: string;
  active_duration_ms?: number;
  system_tracking_enabled: boolean;
  browser_tracking_enabled: boolean;
  browser_tracking_status: string;
}

export interface SetAppTrackingResult {
  status: TrackingStatus;
}

export interface SystemEvent {
  id: string;
  event_type: 'sleep' | 'wake' | 'lock' | 'unlock';
  timestamp: string;
  created_at?: string;
}

export interface GetSystemEventsOptions {
  event_type?: string;
  since?: string;
  until?: string;
  limit?: number;
  offset?: number;
}

export interface GetSystemEventsResult {
  events: SystemEvent[];
  count: number;
}

export interface SystemActivitySummary {
  total_events: number;
  sleep_count: number;
  wake_count: number;
  lock_count: number;
  unlock_count: number;
  last_event?: SystemEvent;
  since: string;
  until: string;
}

export interface BrowserTrackingStatus {
  enabled: boolean;
  status: 'disabled' | 'unavailable' | 'active';
  reason: string;
}

export interface DataStats {
  db_size_bytes: number;
  app_sessions_count: number;
  system_events_count: number;
  filesystem_events_count: number;
  git_commits_count: number;
  terminal_sessions_count: number;
  checkpoints_count: number;
  browser_domains_count: number;
  retention_days: number;
  last_retention_cleanup?: string;
}

export interface RetentionPolicy {
  retention_days: number;
  last_retention_cleanup?: string;
}

export interface PurgeStats {
  app_sessions_purged: number;
  system_events_purged: number;
  filesystem_events_purged: number;
  terminal_sessions_purged: number;
  browser_domains_purged: number;
  total_purged: number;
}

export interface DeleteActivityOptions {
  source: 'all' | 'apps' | 'system' | 'browser' | 'filesystem' | 'terminal' | 'git';
  since?: string;
  until?: string;
}

export interface DeleteActivityResult {
  success: boolean;
  source: string;
  deleted_count: number;
}

export interface UserDataExport {
  export_version: string;
  exported_at: string;
  app_sessions?: AppSession[];
  system_events?: SystemEvent[];
  browser_domains?: Array<{ id: string; domain: string; visit_count: number; last_visited: string }>;
  filesystem_events?: Array<{ id: string; event_type: string; path: string; size: number; mod_time: string; created_at: string }>;
  terminal_sessions?: Array<{ id: string; sanitized_command: string; exit_code: number; duration_ms: number; started_at: string }>;
}

