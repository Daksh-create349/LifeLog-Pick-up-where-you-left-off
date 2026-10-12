import React, { useState, useEffect } from 'react';
import {
  Search,
  LayoutDashboard,
  Clock,
  FolderGit2,
  AppWindow,
  PlayCircle,
  Settings,
  X,
  ArrowRight,
  Database,
} from 'lucide-react';
import { NavView, ProjectOverview } from '../types';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onNavigate: (view: NavView) => void;
  projects: ProjectOverview[];
  onSelectProject: (projectPath: string) => void;
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({
  isOpen,
  onClose,
  onNavigate,
  projects,
  onSelectProject,
}) => {
  const [query, setQuery] = useState('');

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    if (isOpen) {
      window.addEventListener('keydown', handleKeyDown);
    }
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const navActions: Array<{ id: NavView; label: string; icon: any; category: string }> = [
    { id: 'overview', label: 'Go to Overview Dashboard', icon: LayoutDashboard, category: 'Navigation' },
    { id: 'timeline', label: 'Go to Activity Timeline', icon: Clock, category: 'Navigation' },
    { id: 'projects', label: 'Go to Known Projects', icon: FolderGit2, category: 'Navigation' },
    { id: 'apps', label: 'Go to Application Usage', icon: AppWindow, category: 'Navigation' },
    { id: 'resumption', label: 'Go to Project Resumption', icon: PlayCircle, category: 'Navigation' },
    { id: 'settings', label: 'Go to Settings & Diagnostics', icon: Settings, category: 'Navigation' },
  ];

  const filteredNav = navActions.filter((a) =>
    a.label.toLowerCase().includes(query.toLowerCase())
  );

  const filteredProjects = projects.filter(
    (p) =>
      p.display_name.toLowerCase().includes(query.toLowerCase()) ||
      p.canonical_path.toLowerCase().includes(query.toLowerCase())
  );

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-24 bg-black/40 backdrop-blur-sm animate-in fade-in duration-100">
      <div className="w-full max-w-xl mx-4 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-dropdown overflow-hidden flex flex-col max-h-[460px]">
        {/* Search Input Bar */}
        <div className="flex items-center px-4 border-b border-border-light dark:border-border-dark bg-surfaceSubtle-light/30 dark:bg-surfaceSubtle-dark/30">
          <Search size={16} className="text-secondary-light dark:text-secondary-dark mr-3" />
          <input
            type="text"
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command, view, or project name..."
            className="w-full py-3.5 text-sm bg-transparent border-none outline-none text-primary-light dark:text-primary-dark placeholder:text-secondary-light/70 dark:placeholder:text-secondary-dark/70"
          />
          <button
            onClick={onClose}
            className="p-1 text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark rounded"
          >
            <X size={15} />
          </button>
        </div>

        {/* Scrollable Results List */}
        <div className="overflow-y-auto p-2 space-y-3 flex-1 text-xs">
          {/* Navigation Items */}
          {filteredNav.length > 0 && (
            <div>
              <div className="px-2 py-1 text-[10px] font-semibold tracking-wider text-secondary-light dark:text-secondary-dark uppercase font-mono">
                Views
              </div>
              <div className="space-y-0.5">
                {filteredNav.map((item) => {
                  const Icon = item.icon;
                  return (
                    <button
                      key={item.id}
                      onClick={() => {
                        onNavigate(item.id);
                        onClose();
                      }}
                      className="w-full flex items-center justify-between px-3 py-2 rounded-lg text-primary-light dark:text-primary-dark hover:bg-forest-lightTint dark:hover:bg-forest-darkTint hover:text-forest dark:hover:text-forest-dark transition-colors group"
                    >
                      <span className="flex items-center gap-2.5">
                        <Icon size={14} />
                        <span>{item.label}</span>
                      </span>
                      <ArrowRight size={13} className="opacity-0 group-hover:opacity-100 transition-opacity" />
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {/* Known Projects */}
          {filteredProjects.length > 0 && (
            <div>
              <div className="px-2 py-1 text-[10px] font-semibold tracking-wider text-secondary-light dark:text-secondary-dark uppercase font-mono">
                Projects
              </div>
              <div className="space-y-0.5">
                {filteredProjects.map((p) => (
                  <button
                    key={p.canonical_path}
                    onClick={() => {
                      onSelectProject(p.canonical_path);
                      onClose();
                    }}
                    className="w-full flex items-center justify-between px-3 py-2 rounded-lg text-primary-light dark:text-primary-dark hover:bg-forest-lightTint dark:hover:bg-forest-darkTint hover:text-forest dark:hover:text-forest-dark transition-colors group text-left"
                  >
                    <div className="min-w-0 pr-2">
                      <div className="font-medium text-xs truncate">{p.display_name}</div>
                      <div className="text-[10px] text-secondary-light dark:text-secondary-dark truncate font-mono">
                        {p.canonical_path}
                      </div>
                    </div>
                    <span className="shrink-0 text-[10px] bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark px-1.5 py-0.5 rounded border border-border-light dark:border-border-dark text-secondary-light dark:text-secondary-dark">
                      Resume
                    </span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {filteredNav.length === 0 && filteredProjects.length === 0 && (
            <div className="py-8 text-center text-secondary-light dark:text-secondary-dark">
              No matching commands or projects found for &quot;{query}&quot;
            </div>
          )}
        </div>

        {/* Footer Note */}
        <div className="px-4 py-2 border-t border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark flex items-center justify-between text-[10px] text-secondary-light dark:text-secondary-dark">
          <span className="flex items-center gap-1.5">
            <Database size={11} />
            <span>Cross-domain FTS5 search query RPC scheduled for V2 Search Milestone</span>
          </span>
          <span className="font-mono">ESC to close</span>
        </div>
      </div>
    </div>
  );
};
