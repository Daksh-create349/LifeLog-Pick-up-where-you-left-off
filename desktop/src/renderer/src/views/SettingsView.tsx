import React, { useState, useEffect, useCallback } from 'react';
import {
  Cpu,
  Database,
  Trash2,
  Download,
  RefreshCw,
  Sliders,
  Laptop,
  AlertCircle,
  FileText,
  GitBranch,
  Terminal,
  Globe,
  AppWindow,
} from 'lucide-react';
import {
  HealthResult,
  ConnectionStatus,
  TrackingStatus,
  DataStats,
  UserDataExport,
} from '../types';

interface SettingsViewProps {
  health?: HealthResult;
  connectionStatus: ConnectionStatus;
}

export const SettingsView: React.FC<SettingsViewProps> = ({
  health,
  connectionStatus,
}) => {
  const [trackingStatus, setTrackingStatus] = useState<TrackingStatus | null>(null);
  const [dataStats, setDataStats] = useState<DataStats | null>(null);
  const [retentionDays, setRetentionDays] = useState<number>(30);
  const [isLoadingStats, setIsLoadingStats] = useState<boolean>(true);
  const [isCleaning, setIsCleaning] = useState<boolean>(false);
  const [cleanupResult, setCleanupResult] = useState<string | null>(null);
  const [isExporting, setIsExporting] = useState<boolean>(false);
  const [exportNotice, setExportNotice] = useState<string | null>(null);

  // Deletion modal state
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState<boolean>(false);
  const [deleteSource, setDeleteSource] = useState<'all' | 'apps' | 'system' | 'filesystem' | 'terminal' | 'git'>('all');
  const [isDeleting, setIsDeleting] = useState<boolean>(false);
  const [deleteResult, setDeleteResult] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    if (!window.lifelog) return;
    setIsLoadingStats(true);
    try {
      const [tStatus, stats, ret] = await Promise.all([
        window.lifelog.getTrackingStatus().catch(() => null),
        window.lifelog.getDataStats().catch(() => null),
        window.lifelog.getRetentionPolicy().catch(() => null),
      ]);
      if (tStatus) setTrackingStatus(tStatus);
      if (stats) setDataStats(stats);
      if (ret) setRetentionDays(ret.retention_days);
    } catch (err) {
      console.error('Failed loading settings data:', err);
    } finally {
      setIsLoadingStats(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleToggleAppTracking = async () => {
    if (!window.lifelog || !trackingStatus) return;
    const nextState = !trackingStatus.app_tracking_enabled;
    try {
      const res = await window.lifelog.setAppTracking(nextState);
      setTrackingStatus(res.status);
    } catch (err) {
      console.error('Failed toggling app tracking:', err);
    }
  };

  const handleRetentionChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
    const days = parseInt(e.target.value, 10);
    setRetentionDays(days);
    if (!window.lifelog) return;
    try {
      await window.lifelog.setRetentionPolicy(days);
      await loadData();
    } catch (err) {
      console.error('Failed saving retention policy:', err);
    }
  };

  const handleRunCleanup = async () => {
    if (!window.lifelog || isCleaning) return;
    setIsCleaning(true);
    setCleanupResult(null);
    try {
      const purge = await window.lifelog.runRetentionCleanup();
      setCleanupResult(
        `Purged ${purge.total_purged} expired records (${purge.app_sessions_purged} apps, ${purge.system_events_purged} system, ${purge.filesystem_events_purged} files, ${purge.terminal_sessions_purged} commands).`
      );
      await loadData();
    } catch (err: any) {
      setCleanupResult(`Cleanup failed: ${err?.message || 'internal error'}`);
    } finally {
      setIsCleaning(false);
    }
  };

  const handleExportData = async () => {
    if (!window.lifelog || isExporting) return;
    setIsExporting(true);
    setExportNotice(null);
    try {
      const exportPayload: UserDataExport = await window.lifelog.exportUserData();
      const jsonStr = JSON.stringify(exportPayload, null, 2);
      const blob = new Blob([jsonStr], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `lifelog-export-${new Date().toISOString().slice(0, 10)}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      setExportNotice('Export downloaded successfully. Strictly sanitized with 0 secrets or tokens.');
    } catch (err: any) {
      setExportNotice(`Export failed: ${err?.message || 'internal error'}`);
    } finally {
      setIsExporting(false);
    }
  };

  const handleDeleteActivity = async () => {
    if (!window.lifelog || isDeleting) return;
    setIsDeleting(true);
    setDeleteResult(null);
    try {
      const res = await window.lifelog.deleteActivity({ source: deleteSource });
      setDeleteResult(`Successfully deleted ${res.deleted_count} records from source "${res.source}".`);
      setIsDeleteModalOpen(false);
      await loadData();
    } catch (err: any) {
      setDeleteResult(`Deletion failed: ${err?.message || 'internal error'}`);
    } finally {
      setIsDeleting(false);
    }
  };

  const formatBytes = (bytes?: number) => {
    if (!bytes && bytes !== 0) return 'Unknown';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const formatUptime = (seconds?: number) => {
    if (!seconds && seconds !== 0) return 'Unknown';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    if (hrs > 0) return `${hrs}h ${mins}m ${secs}s`;
    if (mins > 0) return `${mins}m ${secs}s`;
    return `${secs}s`;
  };

  return (
    <div className="p-8 space-y-8 max-w-4xl mx-auto pb-16">
      {/* Header */}
      <div>
        <h1 className="text-xl font-bold text-primary-light dark:text-primary-dark tracking-tight">
          Settings & Privacy Center
        </h1>
        <p className="text-xs text-secondary-light dark:text-secondary-dark mt-1">
          Granular privacy boundaries, tracking source controls, retention policies, and data management.
        </p>
      </div>

      {/* Section 1: Central Privacy & Tracking Sources */}
      <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-5">
        <div className="border-b border-border-light dark:border-border-dark pb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Sliders size={16} className="text-forest dark:text-forest-dark" />
            <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
              Tracking Sources & Permissions
            </h2>
          </div>
          <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark">
            Zero telemetry • Local-first
          </span>
        </div>

        <div className="divide-y divide-border-light dark:divide-border-dark space-y-4 pt-1">
          {/* Source 1: App Usage Tracking */}
          <div className="pt-3 first:pt-0 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <AppWindow size={15} className="text-forest" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  Foreground Application Tracking
                </span>
                <span
                  className={`px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider ${
                    trackingStatus?.app_tracking_enabled
                      ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                      : 'bg-zinc-500/10 text-zinc-500 border border-zinc-500/20'
                  }`}
                >
                  {trackingStatus?.app_tracking_enabled ? 'Active' : 'Paused'}
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Records frontmost active application name and bundle identifier via standard macOS workspace notifications. Never captures keystrokes, window titles, URLs, document content, or screens. Requires no Accessibility permissions.
              </p>
            </div>
            <button
              onClick={handleToggleAppTracking}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors shrink-0 ${
                trackingStatus?.app_tracking_enabled
                  ? 'border-border-light dark:border-border-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark text-primary-light dark:text-primary-dark'
                  : 'bg-forest text-white border-transparent hover:bg-forest/90'
              }`}
            >
              {trackingStatus?.app_tracking_enabled ? 'Pause' : 'Resume'}
            </button>
          </div>

          {/* Source 2: System Activity Tracking */}
          <div className="pt-4 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Laptop size={15} className="text-forest" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  System Activity & Power Transitions
                </span>
                <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                  Active
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Records system sleep, wake, screen lock, and screen unlock events via public notification centers. Prevents accumulating inactive hours during sleep or lock. Zero user content collected.
              </p>
            </div>
            <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark shrink-0 pt-1">
              Subprocess Engine
            </span>
          </div>

          {/* Source 3: Browser Domain Tracking */}
          <div className="pt-4 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Globe size={15} className="text-secondary-light dark:text-secondary-dark" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  Browser Domain Tracking
                </span>
                <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-zinc-500/10 text-zinc-500 border border-zinc-500/20">
                  Disabled
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Strictly opt-in and disabled by default. When enabled via optional local extension, records only top-level domains (e.g. "example.com"). Window-title scraping and URL path/query collection are strictly prohibited for privacy.
              </p>
            </div>
            <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark shrink-0 pt-1">
              Opt-In Only
            </span>
          </div>

          {/* Source 4: Filesystem Metadata */}
          <div className="pt-4 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <FileText size={15} className="text-forest" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  Filesystem Metadata Observation
                </span>
                <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                  Active
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Monitors registered directories for created, modified, and deleted files. Collects file paths, timestamps, and sizes only. File content is strictly never read or transmitted.
              </p>
            </div>
            <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark shrink-0 pt-1">
              V1 Poller
            </span>
          </div>

          {/* Source 5: Git Activity */}
          <div className="pt-4 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <GitBranch size={15} className="text-forest" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  Git Repository Scanning
                </span>
                <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                  Active
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Reads local Git commit hashes, author timestamps, and branch transitions across tracked repositories. Diffs and code contents are never exported or uploaded.
              </p>
            </div>
            <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark shrink-0 pt-1">
              V1 Git Engine
            </span>
          </div>

          {/* Source 6: Terminal Recording */}
          <div className="pt-4 flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Terminal size={15} className="text-forest" />
                <span className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                  Explicit Terminal Command Recording
                </span>
                <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                  Active
                </span>
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark leading-relaxed max-w-xl">
                Records commands invoked explicitly through "lifelog run &lt;cmd&gt;". General shell background sessions, keystrokes, and shell history are never intercepted or logged.
              </p>
            </div>
            <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark shrink-0 pt-1">
              Explicit Opt-In
            </span>
          </div>
        </div>
      </div>

      {/* Section 2: Data Management, Metrics, and Retention */}
      <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-5">
        <div className="border-b border-border-light dark:border-border-dark pb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Database size={16} className="text-forest dark:text-forest-dark" />
            <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
              Local Storage Metrics & Retention Policy
            </h2>
          </div>
          <button
            onClick={loadData}
            disabled={isLoadingStats}
            className="flex items-center gap-1.5 text-xs text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark transition-colors"
          >
            <RefreshCw size={12} className={isLoadingStats ? 'animate-spin' : ''} />
            Refresh
          </button>
        </div>

        {/* Real Metrics Grid */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark">
            <div className="text-[10px] uppercase font-mono text-secondary-light dark:text-secondary-dark">Database Size</div>
            <div className="text-base font-bold font-mono text-primary-light dark:text-primary-dark mt-1">
              {formatBytes(dataStats?.db_size_bytes)}
            </div>
          </div>
          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark">
            <div className="text-[10px] uppercase font-mono text-secondary-light dark:text-secondary-dark">App Sessions</div>
            <div className="text-base font-bold font-mono text-primary-light dark:text-primary-dark mt-1">
              {dataStats?.app_sessions_count ?? 0}
            </div>
          </div>
          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark">
            <div className="text-[10px] uppercase font-mono text-secondary-light dark:text-secondary-dark">System Events</div>
            <div className="text-base font-bold font-mono text-primary-light dark:text-primary-dark mt-1">
              {dataStats?.system_events_count ?? 0}
            </div>
          </div>
          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark">
            <div className="text-[10px] uppercase font-mono text-secondary-light dark:text-secondary-dark">FS Events</div>
            <div className="text-base font-bold font-mono text-primary-light dark:text-primary-dark mt-1">
              {dataStats?.filesystem_events_count ?? 0}
            </div>
          </div>
        </div>

        {/* Retention Policy Selector */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark space-y-3">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <div className="text-xs font-semibold text-primary-light dark:text-primary-dark">
                Automatic Retention Purge Window
              </div>
              <p className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5">
                Automatically purges activity records older than the selected threshold. Configuration and migrations are never removed.
              </p>
            </div>
            <select
              value={retentionDays}
              onChange={handleRetentionChange}
              className="px-3 py-1.5 rounded-lg border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-xs font-mono font-medium text-primary-light dark:text-primary-dark shadow-subtle shrink-0"
            >
              <option value={7}>7 Days</option>
              <option value={30}>30 Days (Default)</option>
              <option value={90}>90 Days</option>
              <option value={365}>1 Year (365 Days)</option>
              <option value={0}>Keep Forever</option>
            </select>
          </div>

          <div className="pt-2 border-t border-border-light dark:border-border-dark flex items-center justify-between text-xs">
            <span className="text-[11px] text-secondary-light dark:text-secondary-dark">
              Last cleanup: {dataStats?.last_retention_cleanup ? new Date(dataStats.last_retention_cleanup).toLocaleString() : 'Never run'}
            </span>
            <button
              onClick={handleRunCleanup}
              disabled={isCleaning || retentionDays === 0}
              className="px-3 py-1 rounded bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark text-[11px] font-medium text-primary-light dark:text-primary-dark hover:border-forest transition-colors flex items-center gap-1.5"
            >
              <RefreshCw size={11} className={isCleaning ? 'animate-spin' : ''} />
              {isCleaning ? 'Cleaning...' : 'Run Purge Now'}
            </button>
          </div>

          {cleanupResult && (
            <div className="p-2.5 rounded bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-xs font-mono">
              {cleanupResult}
            </div>
          )}
        </div>

        {/* Data Export & Danger Zone Controls */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
          {/* Export Box */}
          <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark space-y-2">
            <h3 className="text-xs font-semibold text-primary-light dark:text-primary-dark flex items-center gap-1.5">
              <Download size={14} className="text-forest" />
              Full User Data Export
            </h3>
            <p className="text-[11px] text-secondary-light dark:text-secondary-dark">
              Export all local activity records into a portable, structured JSON document. Tokens and internal secrets are excluded.
            </p>
            <button
              onClick={handleExportData}
              disabled={isExporting}
              className="w-full mt-2 px-3 py-1.5 rounded-lg border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-xs font-medium text-primary-light dark:text-primary-dark hover:border-forest transition-colors flex items-center justify-center gap-1.5"
            >
              <Download size={12} />
              {isExporting ? 'Exporting...' : 'Export JSON Data'}
            </button>
            {exportNotice && (
              <p className="text-[10px] text-emerald-600 dark:text-emerald-400 font-mono mt-1">
                {exportNotice}
              </p>
            )}
          </div>

          {/* Clear Activity Box */}
          <div className="p-4 rounded-xl border border-rose-200 dark:border-rose-950 bg-rose-50/30 dark:bg-rose-950/20 space-y-2">
            <h3 className="text-xs font-semibold text-rose-700 dark:text-rose-300 flex items-center gap-1.5">
              <Trash2 size={14} />
              Clear Activity Records
            </h3>
            <p className="text-[11px] text-secondary-light dark:text-secondary-dark">
              Selectively purge stored activity records by category. Schema migrations and database structure remain intact.
            </p>
            <button
              onClick={() => setIsDeleteModalOpen(true)}
              className="w-full mt-2 px-3 py-1.5 rounded-lg border border-rose-300 dark:border-rose-800 bg-rose-50 dark:bg-rose-900/30 text-rose-700 dark:text-rose-300 text-xs font-medium hover:bg-rose-100 dark:hover:bg-rose-900/50 transition-colors flex items-center justify-center gap-1.5"
            >
              <Trash2 size={12} />
              Clear Records...
            </button>
            {deleteResult && (
              <p className="text-[10px] text-primary-light dark:text-primary-dark font-mono mt-1">
                {deleteResult}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Section 3: Engine Diagnostics */}
      <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-4">
        <div className="border-b border-border-light dark:border-border-dark pb-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Cpu size={16} className="text-forest dark:text-forest-dark" />
            <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
              Engine & IPC Diagnostics
            </h2>
          </div>
          <span className="text-[11px] font-mono text-secondary-light dark:text-secondary-dark">
            PID: {connectionStatus.pid || health?.pid || 'N/A'}
          </span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs font-mono">
          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark space-y-0.5">
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark">Daemon Status</div>
            <div className="font-semibold text-forest dark:text-forest-dark capitalize">
              {health?.daemon_status || connectionStatus.state}
            </div>
          </div>

          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark space-y-0.5">
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark">Schema Version</div>
            <div className="font-semibold text-primary-light dark:text-primary-dark">
              Version {health?.schema_version || 9} (SQLite WAL)
            </div>
          </div>

          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark space-y-0.5">
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark">Protocol Version</div>
            <div className="font-semibold text-primary-light dark:text-primary-dark">
              JSON-RPC {health?.protocol_version || '2.0'}
            </div>
          </div>

          <div className="p-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark space-y-0.5">
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark">Daemon Uptime</div>
            <div className="font-semibold text-primary-light dark:text-primary-dark">
              {formatUptime(health?.uptime_seconds)}
            </div>
          </div>
        </div>

        <div className="space-y-2 pt-1 text-xs font-mono">
          <div>
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark mb-0.5">Database File Path</div>
            <div className="p-2 rounded bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-primary-light dark:text-primary-dark truncate text-[11px]">
              {health?.db_path || 'Standby'}
            </div>
          </div>

          <div>
            <div className="text-[10px] text-secondary-light dark:text-secondary-dark mb-0.5">Unix Domain Socket</div>
            <div className="p-2 rounded bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-primary-light dark:text-primary-dark truncate text-[11px]">
              {connectionStatus.socketPath || 'Standby'}
            </div>
          </div>
        </div>
      </div>

      {/* Confirmation Modal for Clearing Data */}
      {isDeleteModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-xs p-4">
          <div className="bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark rounded-xl max-w-md w-full p-6 shadow-xl space-y-4">
            <div className="flex items-center gap-2 text-rose-600 dark:text-rose-400">
              <AlertCircle size={20} />
              <h3 className="text-sm font-bold">Confirm Activity Data Deletion</h3>
            </div>

            <p className="text-xs text-secondary-light dark:text-secondary-dark leading-relaxed">
              This action permanently deletes selected activity records from your local SQLite database. This cannot be undone.
            </p>

            <div className="space-y-1.5 text-xs">
              <label className="font-semibold text-primary-light dark:text-primary-dark">Select Target Source:</label>
              <select
                value={deleteSource}
                onChange={(e) => setDeleteSource(e.target.value as any)}
                className="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark text-primary-light dark:text-primary-dark font-medium"
              >
                <option value="all">All Recorded Activity (Full Reset)</option>
                <option value="apps">Application Usage Sessions Only</option>
                <option value="system">System Activity Events Only</option>
                <option value="filesystem">Filesystem Events Only</option>
                <option value="terminal">Terminal Sessions Only</option>
                <option value="git">Git Commits Only</option>
              </select>
            </div>

            <div className="flex items-center justify-end gap-3 pt-3 border-t border-border-light dark:border-border-dark">
              <button
                onClick={() => setIsDeleteModalOpen(false)}
                className="px-3 py-1.5 rounded-lg border border-border-light dark:border-border-dark text-xs font-medium text-secondary-light dark:text-secondary-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleDeleteActivity}
                disabled={isDeleting}
                className="px-4 py-1.5 rounded-lg bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold shadow-subtle transition-colors flex items-center gap-1.5"
              >
                <Trash2 size={13} />
                {isDeleting ? 'Deleting...' : 'Delete Permanently'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
