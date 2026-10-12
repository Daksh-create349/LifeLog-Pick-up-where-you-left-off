import React from 'react';
import {
  FolderGit2,
  Clock,
  HardDrive,
  Activity,
  ArrowRight,
  GitBranch,
  Terminal,
  FileCode,
  ShieldCheck,
} from 'lucide-react';
import { ProjectOverview, TimelineItem, HealthResult, NavView } from '../types';
import { EmptyState } from '../components/EmptyState';

interface OverviewViewProps {
  projects: ProjectOverview[];
  timelineItems: TimelineItem[];
  health?: HealthResult;
  isLoading: boolean;
  onNavigate: (view: NavView) => void;
  onSelectProject: (path: string) => void;
}

export const OverviewView: React.FC<OverviewViewProps> = ({
  projects,
  timelineItems,
  health,
  isLoading,
  onNavigate,
  onSelectProject,
}) => {
  const topProjects = projects.slice(0, 4);
  const recentEvents = timelineItems.slice(0, 6);

  const getDomainIcon = (domain: string) => {
    switch (domain.toLowerCase()) {
      case 'git':
        return <GitBranch size={13} className="text-forest dark:text-forest-dark" />;
      case 'terminal':
        return <Terminal size={13} className="text-amber-600 dark:text-amber-400" />;
      default:
        return <FileCode size={13} className="text-secondary-light dark:text-secondary-dark" />;
    }
  };

  const formatTimestamp = (ts: string) => {
    try {
      const d = new Date(ts);
      return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    } catch {
      return ts;
    }
  };

  return (
    <div className="p-8 space-y-8 max-w-6xl mx-auto">
      {/* Metrics Row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Metric 1 */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark mb-2">
            <span className="text-xs font-medium">Discovered Projects</span>
            <FolderGit2 size={16} />
          </div>
          <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark">
            {isLoading ? '...' : projects.length}
          </div>
          <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-1">
            Tracked in local storage
          </div>
        </div>

        {/* Metric 2 */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark mb-2">
            <span className="text-xs font-medium">Timeline Events</span>
            <Clock size={16} />
          </div>
          <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark">
            {isLoading ? '...' : timelineItems.length}
          </div>
          <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-1">
            Recorded observations
          </div>
        </div>

        {/* Metric 3 */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark mb-2">
            <span className="text-xs font-medium">Storage Engine</span>
            <HardDrive size={16} />
          </div>
          <div className="text-2xl font-bold tracking-tight text-forest dark:text-forest-dark">
            Schema v{health?.schema_version || 7}
          </div>
          <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-1 truncate">
            SQLite WAL Mode
          </div>
        </div>

        {/* Metric 4 */}
        <div className="p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
          <div className="flex items-center justify-between text-secondary-light dark:text-secondary-dark mb-2">
            <span className="text-xs font-medium">IPC Protocol</span>
            <Activity size={16} />
          </div>
          <div className="text-2xl font-bold tracking-tight text-primary-light dark:text-primary-dark">
            v{health?.protocol_version || '2.0'}
          </div>
          <div className="text-[11px] text-secondary-light dark:text-secondary-dark mt-1">
            Authenticated Unix Socket
          </div>
        </div>
      </div>

      {/* Two Column Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Active Projects */}
        <div className="rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle overflow-hidden flex flex-col">
          <div className="p-4 border-b border-border-light dark:border-border-dark flex items-center justify-between">
            <div>
              <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
                Discovered Projects
              </h2>
              <p className="text-xs text-secondary-light dark:text-secondary-dark">
                Workspaces identified from recorded computer activity
              </p>
            </div>
            <button
              onClick={() => onNavigate('projects')}
              className="text-xs font-medium text-forest dark:text-forest-dark hover:underline flex items-center gap-1"
            >
              <span>View all</span>
              <ArrowRight size={12} />
            </button>
          </div>

          <div className="p-3 flex-1 flex flex-col justify-center">
            {projects.length === 0 ? (
              <EmptyState
                icon={FolderGit2}
                title="No projects discovered yet"
                description="Run 'lifelog watch <dir>' or 'lifelog git scan <dir>' in the terminal to register project activity."
              />
            ) : (
              <div className="divide-y divide-border-light/60 dark:divide-border-dark/60">
                {topProjects.map((p) => (
                  <div
                    key={p.canonical_path}
                    className="py-3 px-2 flex items-center justify-between hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark rounded-lg transition-colors group"
                  >
                    <div className="min-w-0 pr-4">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-semibold text-primary-light dark:text-primary-dark truncate">
                          {p.display_name}
                        </span>
                        {p.git_branch && (
                          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono bg-forest-lightTint dark:bg-forest-darkTint text-forest dark:text-forest-dark border border-forest/20">
                            <GitBranch size={9} />
                            <span>{p.git_branch}</span>
                          </span>
                        )}
                      </div>
                      <div className="text-[11px] text-secondary-light dark:text-secondary-dark font-mono truncate mt-0.5">
                        {p.canonical_path}
                      </div>
                    </div>
                    <button
                      onClick={() => onSelectProject(p.canonical_path)}
                      className="shrink-0 px-2.5 py-1 text-xs font-medium text-forest dark:text-forest-dark bg-forest-lightTint dark:bg-forest-darkTint hover:bg-forest hover:text-white dark:hover:bg-forest dark:hover:text-white rounded-md transition-colors"
                    >
                      Resume
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Recent Activity Feed */}
        <div className="rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle overflow-hidden flex flex-col">
          <div className="p-4 border-b border-border-light dark:border-border-dark flex items-center justify-between">
            <div>
              <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
                Recent Recorded Activity
              </h2>
              <p className="text-xs text-secondary-light dark:text-secondary-dark">
                Latest unified chronological memory entries
              </p>
            </div>
            <button
              onClick={() => onNavigate('timeline')}
              className="text-xs font-medium text-forest dark:text-forest-dark hover:underline flex items-center gap-1"
            >
              <span>View timeline</span>
              <ArrowRight size={12} />
            </button>
          </div>

          <div className="p-3 flex-1 flex flex-col justify-center">
            {timelineItems.length === 0 ? (
              <EmptyState
                icon={Clock}
                title="No timeline events recorded"
                description="Filesystem events, Git commits, and terminal commands will stream here automatically."
              />
            ) : (
              <div className="divide-y divide-border-light/60 dark:divide-border-dark/60">
                {recentEvents.map((ev) => (
                  <div
                    key={ev.record_id}
                    className="py-2.5 px-2 flex items-center justify-between text-xs hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark rounded-lg transition-colors"
                  >
                    <div className="flex items-center gap-2.5 min-w-0 pr-3">
                      <div className="shrink-0 w-6 h-6 rounded bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark flex items-center justify-center">
                        {getDomainIcon(ev.domain)}
                      </div>
                      <div className="min-w-0">
                        <div className="font-medium text-primary-light dark:text-primary-dark truncate">
                          {ev.title}
                        </div>
                        <div className="text-[10px] text-secondary-light dark:text-secondary-dark truncate font-mono">
                          {ev.working_path || ev.record_id}
                        </div>
                      </div>
                    </div>
                    <div className="text-[11px] text-secondary-light dark:text-secondary-dark shrink-0 font-mono">
                      {formatTimestamp(ev.timestamp)}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Privacy Guarantee Card */}
      <div className="p-4 rounded-xl border border-forest/20 bg-forest-lightTint/50 dark:bg-forest-darkTint/30 flex items-start gap-3.5">
        <div className="w-8 h-8 rounded-lg bg-forest text-white shrink-0 flex items-center justify-center">
          <ShieldCheck size={18} />
        </div>
        <div>
          <h3 className="text-xs font-semibold text-forest dark:text-forest-dark">
            Zero Telemetry & Local-First Boundaries
          </h3>
          <p className="text-[11px] text-secondary-light dark:text-secondary-dark mt-0.5 leading-relaxed">
            All activity metadata is stored locally on this Mac in SQLite WAL mode. No network calls,
            no telemetry analytics, and no remote cloud synchronization. File contents and terminal stdout are never collected.
          </p>
        </div>
      </div>
    </div>
  );
};
