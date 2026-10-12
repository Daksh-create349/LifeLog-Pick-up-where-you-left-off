import React, { useState, useEffect, useCallback } from 'react';
import {
  AppWindow,
  Clock,
  Play,
  Pause,
  RefreshCw,
  Layers,
  ShieldCheck,
  Activity,
  AlertCircle,
  Moon,
  Sun,
  Lock,
  Unlock,
  Laptop,
} from 'lucide-react';
import {
  GetAppUsageResult,
  GetAppSessionsResult,
  TrackingStatus,
  AppUsageStat,
  AppSession,
  SystemEvent,
  SystemActivitySummary,
} from '../types';
import { EmptyState } from '../components/EmptyState';
import { ErrorState } from '../components/ErrorState';

type TimeWindow = '24h' | '7d' | '30d';

export const AppsView: React.FC = () => {
  const [timeWindow, setTimeWindow] = useState<TimeWindow>('24h');
  const [usageData, setUsageData] = useState<GetAppUsageResult | null>(null);
  const [sessionsData, setSessionsData] = useState<GetAppSessionsResult | null>(null);
  const [systemEvents, setSystemEvents] = useState<SystemEvent[]>([]);
  const [systemSummary, setSystemSummary] = useState<SystemActivitySummary | null>(null);
  const [trackingStatus, setTrackingStatus] = useState<TrackingStatus | null>(null);
  const [activeTab, setActiveTab] = useState<'sessions' | 'system'>('sessions');
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [isToggling, setIsToggling] = useState<boolean>(false);

  const fetchData = useCallback(async () => {
    setIsLoading(true);
    setError(null);

    try {
      const [usage, sessions, status, sysEvts, sysSum] = await Promise.all([
        window.lifelog.getAppUsage({ window: timeWindow, limit: 50 }),
        window.lifelog.getAppSessions({ window: timeWindow, limit: 100 }),
        window.lifelog.getTrackingStatus(),
        window.lifelog.getSystemEvents({ limit: 50 }).catch(() => ({ events: [] })),
        window.lifelog.getSystemActivitySummary({}).catch(() => null),
      ]);

      setUsageData(usage);
      setSessionsData(sessions);
      setTrackingStatus(status);
      setSystemEvents(sysEvts?.events || []);
      setSystemSummary(sysSum);
    } catch (err: any) {
      setError(err?.message || 'Failed to load application usage data');
    } finally {
      setIsLoading(false);
    }
  }, [timeWindow]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Periodic refresh every 15 seconds to pick up new sessions
  useEffect(() => {
    const timer = setInterval(() => {
      fetchData();
    }, 15000);
    return () => clearInterval(timer);
  }, [fetchData]);

  const handleToggleTracking = async () => {
    if (!trackingStatus || isToggling) return;
    setIsToggling(true);
    try {
      const newEnabled = !trackingStatus.app_tracking_enabled;
      const res = await window.lifelog.setAppTracking(newEnabled);
      setTrackingStatus(res.status);
      await fetchData();
    } catch (err: any) {
      setError(err?.message || 'Failed to toggle app tracking');
    } finally {
      setIsToggling(false);
    }
  };

  const formatDuration = (ms: number): string => {
    if (ms <= 0) return '0s';
    const totalSecs = Math.floor(ms / 1000);
    const hours = Math.floor(totalSecs / 3600);
    const mins = Math.floor((totalSecs % 3600) / 60);
    const secs = totalSecs % 60;

    if (hours > 0) {
      return `${hours}h ${mins}m`;
    }
    if (mins > 0) {
      return `${mins}m ${secs}s`;
    }
    return `${secs}s`;
  };

  const formatTime = (isoStr: string): string => {
    try {
      const d = new Date(isoStr);
      return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    } catch {
      return isoStr;
    }
  };

  const formatDate = (isoStr: string): string => {
    try {
      const d = new Date(isoStr);
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
    } catch {
      return isoStr;
    }
  };

  if (isLoading && !usageData) {
    return (
      <div className="flex-1 flex items-center justify-center p-8">
        <div className="flex flex-col items-center gap-3">
          <RefreshCw className="animate-spin text-forest" size={24} />
          <p className="text-sm text-secondary-light dark:text-secondary-dark font-medium">
            Loading application memory...
          </p>
        </div>
      </div>
    );
  }

  if (error && !usageData) {
    return (
      <div className="flex-1 p-8">
        <ErrorState message={error} onRetry={fetchData} />
      </div>
    );
  }

  const isTrackingActive = trackingStatus?.app_tracking_enabled && trackingStatus?.observer_status === 'active';
  const isTrackingPaused = !trackingStatus?.app_tracking_enabled || trackingStatus?.observer_status === 'paused';

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-primary-light dark:text-primary-dark">
            Application Usage
          </h1>
          <p className="text-xs text-secondary-light dark:text-secondary-dark mt-1">
            Privacy-conscious foreground application tracking with zero content or keystroke inspection
          </p>
        </div>

        {/* Controls: Time Window Filter & Tracking Toggle */}
        <div className="flex items-center gap-3">
          {/* Time Window Tabs */}
          <div className="flex items-center bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark p-0.5 rounded-lg border border-border-light dark:border-border-dark text-xs font-medium">
            <button
              onClick={() => setTimeWindow('24h')}
              className={`px-3 py-1 rounded-md transition-colors ${
                timeWindow === '24h'
                  ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-xs font-semibold'
                  : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
              }`}
            >
              24 Hours
            </button>
            <button
              onClick={() => setTimeWindow('7d')}
              className={`px-3 py-1 rounded-md transition-colors ${
                timeWindow === '7d'
                  ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-xs font-semibold'
                  : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
              }`}
            >
              7 Days
            </button>
            <button
              onClick={() => setTimeWindow('30d')}
              className={`px-3 py-1 rounded-md transition-colors ${
                timeWindow === '30d'
                  ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-xs font-semibold'
                  : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
              }`}
            >
              30 Days
            </button>
          </div>

          {/* Tracking Toggle Button */}
          {trackingStatus && (
            <button
              onClick={handleToggleTracking}
              disabled={isToggling}
              className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-lg text-xs font-medium border transition-colors shadow-xs ${
                isTrackingActive
                  ? 'bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border-border-light dark:border-border-dark text-primary-light dark:text-primary-dark hover:bg-surface-light dark:hover:bg-surface-dark'
                  : 'bg-forest text-white border-forest hover:bg-forestDark'
              }`}
            >
              {isTrackingActive ? (
                <>
                  <Pause size={13} strokeWidth={2.2} />
                  <span>Pause Tracking</span>
                </>
              ) : (
                <>
                  <Play size={13} strokeWidth={2.2} />
                  <span>Resume Tracking</span>
                </>
              )}
            </button>
          )}

          <button
            onClick={fetchData}
            title="Refresh application data"
            className="p-1.5 rounded-lg border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark"
          >
            <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      {/* Observer Status Banner */}
      {isTrackingPaused && (
        <div className="flex items-center justify-between px-4 py-2.5 rounded-xl border border-amber-300/40 dark:border-amber-700/40 bg-amber-500/10 text-xs text-amber-800 dark:text-amber-200">
          <div className="flex items-center gap-2">
            <AlertCircle size={15} />
            <span>App tracking is currently paused. No new application sessions are being recorded.</span>
          </div>
          <button
            onClick={handleToggleTracking}
            className="font-semibold underline hover:no-underline"
          >
            Resume
          </button>
        </div>
      )}

      {/* Focus Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Total Foreground Time */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle flex flex-col justify-between">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark">
            <span className="text-xs font-medium">Observed Active Time</span>
            <Clock size={16} />
          </div>
          <div className="mt-2">
            <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark font-mono">
              {formatDuration(usageData?.total_duration_ms || 0)}
            </div>
            <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5">
              Across reporting window
            </div>
          </div>
        </div>

        {/* Distinct Applications */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle flex flex-col justify-between">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark">
            <span className="text-xs font-medium">Active Applications</span>
            <AppWindow size={16} />
          </div>
          <div className="mt-2">
            <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark font-mono">
              {usageData?.apps?.length || 0}
            </div>
            <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5">
              Unique bundle identifiers
            </div>
          </div>
        </div>

        {/* Recorded Sessions */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle flex flex-col justify-between">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark">
            <span className="text-xs font-medium">Recorded Sessions</span>
            <Layers size={16} />
          </div>
          <div className="mt-2">
            <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark font-mono">
              {usageData?.total_sessions || 0}
            </div>
            <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5">
              Foreground transitions
            </div>
          </div>
        </div>

        {/* Observer Runtime Status */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle flex flex-col justify-between">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark">
            <span className="text-xs font-medium">Observer Engine</span>
            <Activity size={16} />
          </div>
          <div className="mt-2">
            <div className="flex items-center gap-2">
              <span
                className={`w-2.5 h-2.5 rounded-full ${
                  isTrackingActive
                    ? 'bg-emerald-500 animate-pulse'
                    : 'bg-amber-500'
                }`}
              />
              <span className="text-sm font-semibold capitalize text-primary-light dark:text-primary-dark">
                {trackingStatus?.observer_status || 'Offline'}
              </span>
            </div>
            <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-1 truncate">
              {trackingStatus?.active_app ? (
                <>Foreground: <span className="font-mono text-primary-light dark:text-primary-dark">{trackingStatus.active_app}</span></>
              ) : (
                'Event-driven AppKit notifications'
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Main 2-Column Content: Top Apps & Session Timeline */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* Left Column: Top Applications Breakdown (7 cols) */}
        <div className="lg:col-span-7 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold uppercase tracking-wider text-secondary-light dark:text-secondary-dark font-mono">
              Top Applications
            </h2>
            <span className="text-xs text-secondary-light dark:text-secondary-dark">
              Sorted by duration
            </span>
          </div>

          {!usageData || usageData.apps.length === 0 ? (
            <div className="p-8 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-center">
              <EmptyState
                icon={AppWindow}
                title="No application activity recorded"
                description="Foreground applications will appear here as you interact with your Mac."
              />
            </div>
          ) : (
            <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-4">
              {usageData.apps.map((app: AppUsageStat) => (
                <div key={app.app_bundle_id} className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="font-semibold text-primary-light dark:text-primary-dark truncate">
                        {app.app_name}
                      </span>
                      <span className="text-[10px] text-secondary-light dark:text-secondary-dark font-mono truncate max-w-[160px]">
                        {app.app_bundle_id}
                      </span>
                    </div>
                    <div className="flex items-center gap-3 text-right shrink-0">
                      <span className="text-[11px] text-secondary-light dark:text-secondary-dark font-mono">
                        {app.session_count} {app.session_count === 1 ? 'session' : 'sessions'}
                      </span>
                      <span className="font-mono font-bold text-primary-light dark:text-primary-dark">
                        {formatDuration(app.total_duration_ms)}
                      </span>
                    </div>
                  </div>

                  {/* Proportional Duration Bar */}
                  <div className="w-full h-1.5 rounded-full bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark overflow-hidden flex">
                    <div
                      className="h-full bg-forest rounded-full transition-all duration-300"
                      style={{ width: `${Math.max(app.percentage, 1.5)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Right Column: Recent Activity Stream (5 cols) */}
        <div className="lg:col-span-5 space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1.5 p-0.5 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-xs">
              <button
                onClick={() => setActiveTab('sessions')}
                className={`px-2.5 py-1 rounded-md text-xs font-semibold transition-colors ${
                  activeTab === 'sessions'
                    ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-subtle'
                    : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
                }`}
              >
                App Sessions
              </button>
              <button
                onClick={() => setActiveTab('system')}
                className={`px-2.5 py-1 rounded-md text-xs font-semibold transition-colors ${
                  activeTab === 'system'
                    ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-subtle'
                    : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
                }`}
              >
                System Events
              </button>
            </div>
            <span className="text-xs text-secondary-light dark:text-secondary-dark">
              {activeTab === 'sessions'
                ? 'Latest transitions'
                : systemSummary
                ? `${systemSummary.total_events} events (${systemSummary.lock_count} lock, ${systemSummary.sleep_count} sleep)`
                : 'Sleep, wake & lock'}
            </span>
          </div>

          {activeTab === 'sessions' ? (
            !sessionsData || sessionsData.sessions.length === 0 ? (
              <div className="p-8 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-center">
                <EmptyState
                  icon={Clock}
                  title="No recent transitions"
                  description="Session history updates automatically when switching windows."
                />
              </div>
            ) : (
              <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle divide-y divide-border-light dark:divide-border-dark max-h-[500px] overflow-y-auto">
                {sessionsData.sessions.slice(0, 30).map((sess: AppSession) => (
                  <div key={sess.id} className="py-2.5 first:pt-0 last:pb-0 flex items-center justify-between text-xs">
                    <div className="min-w-0 pr-3">
                      <div className="flex items-center gap-1.5">
                        <span className="font-semibold text-primary-light dark:text-primary-dark truncate">
                          {sess.app_name}
                        </span>
                        {sess.is_active && (
                          <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                            Active
                          </span>
                        )}
                      </div>
                      <div className="text-[10px] text-secondary-light dark:text-secondary-dark mt-0.5">
                        {formatDate(sess.start_time)} at {formatTime(sess.start_time)}
                      </div>
                    </div>

                    <div className="shrink-0 text-right font-mono text-[11px] text-primary-light dark:text-primary-dark font-medium">
                      {sess.is_active ? 'In progress' : formatDuration(sess.duration_ms)}
                    </div>
                  </div>
                ))}
              </div>
            )
          ) : (
            /* System Activity Events Tab */
            systemEvents.length === 0 ? (
              <div className="p-8 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark text-center">
                <EmptyState
                  icon={Laptop}
                  title="No system transitions"
                  description="Sleep, wake, lock, and unlock transitions will appear here."
                />
              </div>
            ) : (
              <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle divide-y divide-border-light dark:divide-border-dark max-h-[500px] overflow-y-auto">
                {systemEvents.map((evt: SystemEvent) => {
                  const getEventConfig = (type: string) => {
                    switch (type) {
                      case 'sleep':
                        return { label: 'System Sleep', icon: Moon, color: 'text-indigo-400' };
                      case 'wake':
                        return { label: 'System Wake', icon: Sun, color: 'text-amber-400' };
                      case 'lock':
                        return { label: 'Screen Locked', icon: Lock, color: 'text-rose-400' };
                      case 'unlock':
                        return { label: 'Screen Unlocked', icon: Unlock, color: 'text-emerald-400' };
                      default:
                        return { label: `System ${type}`, icon: Laptop, color: 'text-secondary-light' };
                    }
                  };
                  const cfg = getEventConfig(evt.event_type);
                  const IconComponent = cfg.icon;

                  return (
                    <div key={evt.id} className="py-2.5 first:pt-0 last:pb-0 flex items-center justify-between text-xs">
                      <div className="flex items-center gap-2.5 min-w-0 pr-3">
                        <div className={`p-1.5 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark ${cfg.color} shrink-0`}>
                          <IconComponent size={13} />
                        </div>
                        <div className="min-w-0">
                          <span className="font-semibold text-primary-light dark:text-primary-dark truncate block">
                            {cfg.label}
                          </span>
                          <span className="text-[10px] text-secondary-light dark:text-secondary-dark font-mono">
                            {formatDate(evt.timestamp)} at {formatTime(evt.timestamp)}
                          </span>
                        </div>
                      </div>

                      <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-semibold uppercase tracking-wider bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-secondary-light dark:text-secondary-dark">
                        {evt.event_type}
                      </span>
                    </div>
                  );
                })}
              </div>
            )
          )}
        </div>
      </div>

      {/* Privacy Guarantee Card */}
      <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark">
        <div className="flex items-start gap-3">
          <div className="p-1.5 rounded-lg bg-surface-light dark:bg-surface-dark text-forest shrink-0">
            <ShieldCheck size={18} />
          </div>
          <div>
            <h3 className="text-xs font-semibold text-primary-light dark:text-primary-dark">
              Local-First Application Tracking Boundaries
            </h3>
            <p className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5 leading-relaxed">
              LIFELOG observes only the active foreground application name and bundle identifier via standard macOS workspace notifications. Window titles, document names, URLs, typed keystrokes, clipboard contents, and application screens are strictly never recorded.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};
