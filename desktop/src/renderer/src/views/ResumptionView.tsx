import React, { useState } from 'react';
import {
  PlayCircle,
  Clock,
  GitBranch,
  Terminal,
  FileCode,
  ShieldAlert,
  Sparkles,
  CheckCircle2,
  Folder,
} from 'lucide-react';
import { ResumptionReport, ProjectOverview } from '../types';
import { EmptyState } from '../components/EmptyState';

interface ResumptionViewProps {
  currentPath: string;
  onPathChange: (path: string) => void;
  onSynthesize: (path: string, gap: string) => void;
  report: ResumptionReport | null;
  isLoading: boolean;
  projects: ProjectOverview[];
}

export const ResumptionView: React.FC<ResumptionViewProps> = ({
  currentPath,
  onPathChange,
  onSynthesize,
  report,
  isLoading,
  projects,
}) => {
  const [gap, setGap] = useState('30m');

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (currentPath.trim()) {
      onSynthesize(currentPath.trim(), gap);
    }
  };

  const formatDuration = (nanos: number) => {
    const seconds = Math.floor(nanos / 1_000_000_000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    if (hours > 0) return `${hours}h ${minutes % 60}m`;
    if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
    return `${seconds}s`;
  };

  return (
    <div className="p-8 space-y-6 max-w-5xl mx-auto">
      {/* Path Input & Synthesis Bar */}
      <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
        <form onSubmit={handleFormSubmit} className="flex flex-wrap items-center gap-3">
          <div className="flex-1 min-w-[260px] relative">
            <Folder size={14} className="absolute left-3 top-3 text-secondary-light dark:text-secondary-dark" />
            <input
              type="text"
              value={currentPath}
              onChange={(e) => onPathChange(e.target.value)}
              placeholder="Enter canonical project directory path..."
              className="w-full pl-9 pr-3 py-2 text-xs font-mono bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark rounded-lg text-primary-light dark:text-primary-dark outline-none focus:border-forest dark:focus:border-forest"
            />
          </div>

          {/* Quick Picker from Known Projects */}
          {projects.length > 0 && (
            <select
              value={currentPath}
              onChange={(e) => {
                onPathChange(e.target.value);
                if (e.target.value) onSynthesize(e.target.value, gap);
              }}
              className="px-2.5 py-2 text-xs bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark rounded-lg text-secondary-light dark:text-secondary-dark outline-none cursor-pointer max-w-[200px] truncate"
            >
              <option value="">Choose known project...</option>
              {projects.map((p) => (
                <option key={p.canonical_path} value={p.canonical_path}>
                  {p.display_name}
                </option>
              ))}
            </select>
          )}

          {/* Inactivity Gap Selector */}
          <div className="flex items-center gap-1.5 text-xs text-secondary-light dark:text-secondary-dark">
            <span className="text-[11px]">Gap:</span>
            <select
              value={gap}
              onChange={(e) => setGap(e.target.value)}
              className="px-2 py-2 text-xs bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark rounded-lg text-primary-light dark:text-primary-dark outline-none cursor-pointer"
            >
              <option value="15m">15m gap</option>
              <option value="30m">30m gap</option>
              <option value="1h">1h gap</option>
            </select>
          </div>

          <button
            type="submit"
            disabled={isLoading || !currentPath.trim()}
            className="px-4 py-2 text-xs font-medium text-white bg-forest hover:bg-forest-hover rounded-lg transition-colors shadow-subtle disabled:opacity-50 shrink-0 inline-flex items-center gap-1.5"
          >
            <PlayCircle size={13} />
            <span>{isLoading ? 'Synthesizing...' : 'Synthesize Context'}</span>
          </button>
        </form>
      </div>

      {/* Synthesis Report Content */}
      {!report ? (
        <EmptyState
          icon={PlayCircle}
          title="Select a project to synthesize resumption report"
          description="Synthesizes observed filesystem, Git, and terminal memories into contiguous activity sessions so you can pick up exactly where you left off."
        />
      ) : (
        <div className="space-y-6">
          {/* Project Summary Header */}
          <div className="p-5 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-base font-bold text-primary-light dark:text-primary-dark font-mono">
                    {report.project_path}
                  </h2>
                  {report.path_exists && (
                    <span className="flex items-center gap-1 text-[10px] text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-900/40 px-2 py-0.5 rounded font-mono font-medium">
                      <CheckCircle2 size={10} />
                      <span>exists on disk</span>
                    </span>
                  )}
                </div>
                <div className="text-xs text-secondary-light dark:text-secondary-dark mt-1 font-mono">
                  Canonical: {report.canonical_path}
                </div>
              </div>

              {/* Activity Inference Banner */}
              {report.derived.activity_inference && (
                <div className="p-3 rounded-lg bg-forest-lightTint dark:bg-forest-darkTint border border-forest/20 max-w-sm">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-forest dark:text-forest-dark mb-1">
                    <Sparkles size={12} />
                    <span>Activity Inference</span>
                  </div>
                  <p className="text-xs text-primary-light dark:text-primary-dark leading-relaxed">
                    {report.derived.activity_inference}
                  </p>
                </div>
              )}
            </div>
          </div>

          {/* Section 1: OBSERVED ACTIVITY */}
          <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-4">
            <div className="border-b border-border-light dark:border-border-dark pb-3">
              <h3 className="text-xs font-bold tracking-wider uppercase text-secondary-light dark:text-secondary-dark font-mono">
                1. Observed Activity Facts
              </h3>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {/* Filesystem Facts */}
              <div className="p-3.5 rounded-lg border border-border-light dark:border-border-dark bg-surfaceSubtle-light/40 dark:bg-surfaceSubtle-dark/40 space-y-2">
                <div className="flex items-center gap-2 text-xs font-semibold text-primary-light dark:text-primary-dark">
                  <FileCode size={13} />
                  <span>Filesystem Events ({report.observed.filesystem.total_events})</span>
                </div>
                {report.observed.filesystem.recent_changes.length === 0 ? (
                  <p className="text-[11px] text-secondary-light dark:text-secondary-dark">No file modifications recorded</p>
                ) : (
                  <ul className="text-[11px] font-mono space-y-1 overflow-x-hidden">
                    {report.observed.filesystem.recent_changes.slice(0, 5).map((change) => (
                      <li key={change.path} className="truncate text-secondary-light dark:text-secondary-dark">
                        <span className="font-semibold text-primary-light dark:text-primary-dark">
                          {change.last_event_type}
                        </span>{' '}
                        {change.relative_path || change.path}
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              {/* Git Facts */}
              <div className="p-3.5 rounded-lg border border-border-light dark:border-border-dark bg-surfaceSubtle-light/40 dark:bg-surfaceSubtle-dark/40 space-y-2">
                <div className="flex items-center gap-2 text-xs font-semibold text-primary-light dark:text-primary-dark">
                  <GitBranch size={13} />
                  <span>Git Repository State</span>
                </div>
                {report.observed.git.is_registered ? (
                  <div className="text-[11px] space-y-1.5 font-mono">
                    <div className="text-forest dark:text-forest-dark font-semibold">
                      Branch: {report.observed.git.current_branch || 'HEAD'}
                    </div>
                    {report.observed.git.head_commit && (
                      <div className="truncate text-secondary-light dark:text-secondary-dark">
                        HEAD: {report.observed.git.head_commit.subject}
                      </div>
                    )}
                    <div className="text-secondary-light dark:text-secondary-dark">
                      Working tree changes: {report.observed.git.working_tree.length}
                    </div>
                  </div>
                ) : (
                  <p className="text-[11px] text-secondary-light dark:text-secondary-dark">Not registered as Git repository</p>
                )}
              </div>

              {/* Terminal Facts */}
              <div className="p-3.5 rounded-lg border border-border-light dark:border-border-dark bg-surfaceSubtle-light/40 dark:bg-surfaceSubtle-dark/40 space-y-2">
                <div className="flex items-center gap-2 text-xs font-semibold text-primary-light dark:text-primary-dark">
                  <Terminal size={13} />
                  <span>Terminal Commands ({report.observed.terminal.total_commands})</span>
                </div>
                {report.observed.terminal.recent_commands.length === 0 ? (
                  <p className="text-[11px] text-secondary-light dark:text-secondary-dark">No explicit commands recorded</p>
                ) : (
                  <ul className="text-[11px] font-mono space-y-1 overflow-x-hidden">
                    {report.observed.terminal.recent_commands.slice(0, 5).map((cmd) => (
                      <li key={cmd.id} className="truncate text-secondary-light dark:text-secondary-dark">
                        $ {cmd.sanitized_command}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          </div>

          {/* Section 2: DERIVED ACTIVITY SESSIONS & SYNTHESIS */}
          <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-4">
            <div className="border-b border-border-light dark:border-border-dark pb-3 flex items-center justify-between">
              <h3 className="text-xs font-bold tracking-wider uppercase text-secondary-light dark:text-secondary-dark font-mono">
                2. Derived Activity Sessions ({report.derived.sessions.length})
              </h3>
              <span className="text-[11px] text-secondary-light dark:text-secondary-dark font-mono">
                Inactivity gap threshold: {gap}
              </span>
            </div>

            {report.derived.sessions.length === 0 ? (
              <p className="text-xs text-secondary-light dark:text-secondary-dark">
                No activity sessions clustered within the selected lookback window.
              </p>
            ) : (
              <div className="space-y-3">
                {report.derived.sessions.map((sess) => (
                  <div
                    key={sess.id}
                    className="p-4 rounded-lg border border-border-light dark:border-border-dark bg-surfaceSubtle-light/30 dark:bg-surfaceSubtle-dark/30 space-y-1.5"
                  >
                    <div className="flex items-center justify-between text-xs">
                      <div className="font-semibold text-primary-light dark:text-primary-dark font-mono">
                        {sess.id}
                      </div>
                      <div className="flex items-center gap-1.5 text-secondary-light dark:text-secondary-dark font-mono text-[11px]">
                        <Clock size={11} />
                        <span>Duration: {formatDuration(sess.duration)}</span>
                        <span>•</span>
                        <span>{sess.activity_count} actions</span>
                      </div>
                    </div>
                    <div className="text-xs text-primary-light dark:text-primary-dark font-medium">
                      {sess.summary}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Section 3: UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES */}
          <div className="p-6 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle space-y-3">
            <div className="border-b border-border-light dark:border-border-dark pb-3 flex items-center gap-2">
              <ShieldAlert size={14} className="text-secondary-light dark:text-secondary-dark" />
              <h3 className="text-xs font-bold tracking-wider uppercase text-secondary-light dark:text-secondary-dark font-mono">
                3. Unavailable Context & Privacy Boundaries
              </h3>
            </div>
            <p className="text-xs text-secondary-light dark:text-secondary-dark leading-relaxed">
              LIFELOG enforces strict zero-inspection privacy boundaries. The following information is never collected or inferred:
            </p>
            <ul className="text-xs text-secondary-light dark:text-secondary-dark space-y-1 list-disc list-inside">
              <li>File contents, code diffs, and AST modifications</li>
              <li>Terminal stdout/stderr command output and interactive shell sessions</li>
              <li>Browser cookies, private sessions, and unconsented web history</li>
              <li>Keystrokes, clipboard buffers, and screen recordings</li>
            </ul>
          </div>
        </div>
      )}
    </div>
  );
};
