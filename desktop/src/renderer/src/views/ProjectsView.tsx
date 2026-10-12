import React from 'react';
import {
  FolderGit2,
  GitBranch,
  Terminal,
  FileCode,
  Bookmark,
  CheckCircle2,
  ArrowRight,
  ArrowDownUp,
} from 'lucide-react';
import { ProjectOverview } from '../types';
import { EmptyState } from '../components/EmptyState';

interface ProjectsViewProps {
  projects: ProjectOverview[];
  isLoading: boolean;
  onSelectProject: (path: string) => void;
  sortMode: 'recent' | 'name' | 'activity';
  onSortChange: (sort: 'recent' | 'name' | 'activity') => void;
}

export const ProjectsView: React.FC<ProjectsViewProps> = ({
  projects,
  isLoading,
  onSelectProject,
  sortMode,
  onSortChange,
}) => {
  const sortOptions = [
    { id: 'recent', label: 'Recent Activity' },
    { id: 'name', label: 'Alphabetical' },
    { id: 'activity', label: 'Activity Volume' },
  ] as const;

  const formatActivityTime = (ts?: string) => {
    if (!ts) return 'Never recorded';
    try {
      const d = new Date(ts);
      return d.toLocaleDateString([], {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return ts;
    }
  };

  return (
    <div className="p-8 space-y-6 max-w-6xl mx-auto">
      {/* Controls Bar */}
      <div className="flex items-center justify-between p-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
            Known Workspaces
          </h2>
          <span className="px-2 py-0.5 rounded text-[11px] font-mono bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-secondary-light dark:text-secondary-dark">
            {isLoading ? '...' : `${projects.length} discovered`}
          </span>
        </div>

        {/* Sort Selector */}
        <div className="flex items-center gap-1.5 text-xs text-secondary-light dark:text-secondary-dark">
          <ArrowDownUp size={13} />
          <span>Sort by:</span>
          <div className="inline-flex rounded-lg border border-border-light dark:border-border-dark p-0.5 bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark">
            {sortOptions.map((opt) => (
              <button
                key={opt.id}
                onClick={() => onSortChange(opt.id)}
                className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-all ${
                  sortMode === opt.id
                    ? 'bg-surface-light dark:bg-surface-dark text-primary-light dark:text-primary-dark shadow-subtle font-semibold'
                    : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark'
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Projects Grid */}
      {projects.length === 0 ? (
        <EmptyState
          icon={FolderGit2}
          title="No projects recorded yet"
          description="Projects are automatically inferred when filesystem events, Git scans, or terminal commands are observed inside a directory."
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {projects.map((proj) => (
            <div
              key={proj.canonical_path}
              className="p-5 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle flex flex-col justify-between hover:border-forest/50 dark:hover:border-forest/50 transition-all group"
            >
              <div>
                {/* Header */}
                <div className="flex items-start justify-between gap-3 mb-2">
                  <div className="min-w-0">
                    <h3 className="text-sm font-bold text-primary-light dark:text-primary-dark truncate tracking-tight">
                      {proj.display_name}
                    </h3>
                    <p className="text-[11px] text-secondary-light dark:text-secondary-dark font-mono truncate mt-0.5">
                      {proj.canonical_path}
                    </p>
                  </div>
                  {proj.exists_on_disk && (
                    <span className="shrink-0 flex items-center gap-1 text-[10px] text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-900/40 px-2 py-0.5 rounded font-mono font-medium">
                      <CheckCircle2 size={10} />
                      <span>on disk</span>
                    </span>
                  )}
                </div>

                {/* Git and Activity Badges */}
                <div className="flex flex-wrap items-center gap-2 my-3 text-[11px]">
                  {proj.git_branch ? (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded font-mono font-medium bg-forest-lightTint dark:bg-forest-darkTint text-forest dark:text-forest-dark border border-forest/20">
                      <GitBranch size={11} />
                      <span>{proj.git_branch}</span>
                    </span>
                  ) : proj.is_git_registered ? (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded font-mono bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark text-secondary-light dark:text-secondary-dark border border-border-light dark:border-border-dark">
                      <GitBranch size={11} />
                      <span>git registered</span>
                    </span>
                  ) : null}

                  {proj.has_checkpoint && (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded font-mono bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-400 border border-amber-200 dark:border-amber-900/40">
                      <Bookmark size={11} />
                      <span>{proj.checkpoint_count} checkpoint(s)</span>
                    </span>
                  )}
                </div>

                {/* Counts Breakdown */}
                <div className="grid grid-cols-3 gap-2 py-2 px-3 rounded-lg bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark text-center my-3 text-xs">
                  <div>
                    <div className="flex items-center justify-center gap-1 text-secondary-light dark:text-secondary-dark text-[10px]">
                      <FileCode size={11} />
                      <span>Files</span>
                    </div>
                    <div className="font-semibold text-primary-light dark:text-primary-dark mt-0.5">
                      {proj.domain_counts?.filesystem || 0}
                    </div>
                  </div>
                  <div>
                    <div className="flex items-center justify-center gap-1 text-secondary-light dark:text-secondary-dark text-[10px]">
                      <GitBranch size={11} />
                      <span>Git</span>
                    </div>
                    <div className="font-semibold text-primary-light dark:text-primary-dark mt-0.5">
                      {proj.domain_counts?.git || 0}
                    </div>
                  </div>
                  <div>
                    <div className="flex items-center justify-center gap-1 text-secondary-light dark:text-secondary-dark text-[10px]">
                      <Terminal size={11} />
                      <span>Terminal</span>
                    </div>
                    <div className="font-semibold text-primary-light dark:text-primary-dark mt-0.5">
                      {proj.domain_counts?.terminal || 0}
                    </div>
                  </div>
                </div>
              </div>

              {/* Footer Actions */}
              <div className="pt-3 border-t border-border-light/60 dark:border-border-dark/60 flex items-center justify-between text-xs">
                <span className="text-[11px] text-secondary-light dark:text-secondary-dark">
                  Last active: {formatActivityTime(proj.latest_activity)}
                </span>
                <button
                  onClick={() => onSelectProject(proj.canonical_path)}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md font-medium text-white bg-forest hover:bg-forest-hover transition-colors shadow-subtle"
                >
                  <span>Synthesize Context</span>
                  <ArrowRight size={12} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
