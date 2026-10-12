import React from 'react';
import {
  LayoutDashboard,
  Clock,
  FolderGit2,
  AppWindow,
  PlayCircle,
  Settings,
  Search,
  Sun,
  Moon,
  Layers,
} from 'lucide-react';
import { NavView, ConnectionStatus } from '../types';

interface SidebarProps {
  currentView: NavView;
  onNavigate: (view: NavView) => void;
  onOpenSearch: () => void;
  connectionStatus: ConnectionStatus;
  isDark: boolean;
  onToggleTheme: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onNavigate,
  onOpenSearch,
  connectionStatus,
  isDark,
  onToggleTheme,
}) => {
  const navItems = [
    { id: 'overview', label: 'Overview', icon: LayoutDashboard },
    { id: 'timeline', label: 'Timeline', icon: Clock },
    { id: 'projects', label: 'Projects', icon: FolderGit2 },
    { id: 'apps', label: 'Apps', icon: AppWindow },
    { id: 'resumption', label: 'Resumption', icon: PlayCircle },
    { id: 'settings', label: 'Settings', icon: Settings },
  ] as const;

  const isConnected = connectionStatus.state === 'connected';
  const isConnecting = connectionStatus.state === 'connecting';

  return (
    <aside className="w-60 h-full flex flex-col justify-between border-r border-border-light dark:border-border-dark bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark select-none">
      {/* Top Section */}
      <div>
        {/* macOS Traffic Light Header Spacer */}
        <div className="h-10 titlebar-drag flex items-center px-4" />

        {/* Wordmark Branding */}
        <div className="px-5 py-3">
          <div className="flex items-center gap-2.5">
            <div className="w-7 h-7 rounded-lg bg-forest text-white flex items-center justify-center shadow-subtle">
              <Layers size={16} strokeWidth={2.25} />
            </div>
            <div>
              <div className="text-sm font-bold tracking-tight text-primary-light dark:text-primary-dark font-mono leading-none">
                LIFELOG
              </div>
              <div className="text-[10px] text-secondary-light dark:text-secondary-dark font-medium tracking-wide mt-1">
                Local Memory Engine
              </div>
            </div>
          </div>
        </div>

        {/* Quick Search Button */}
        <div className="px-3 pt-3 pb-2">
          <button
            onClick={onOpenSearch}
            className="w-full flex items-center justify-between px-3 py-1.5 text-xs text-secondary-light dark:text-secondary-dark bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark rounded-md hover:border-forest/40 dark:hover:border-forest/40 transition-colors shadow-subtle"
          >
            <span className="flex items-center gap-2">
              <Search size={13} />
              <span>Search activity...</span>
            </span>
            <kbd className="text-[10px] bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark px-1.5 py-0.5 rounded border border-border-light dark:border-border-dark font-mono text-secondary-light dark:text-secondary-dark">
              ⌘K
            </kbd>
          </button>
        </div>

        {/* Navigation Items */}
        <nav className="px-3 py-2 space-y-0.5">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive = currentView === item.id;
            return (
              <button
                key={item.id}
                onClick={() => onNavigate(item.id)}
                className={`w-full flex items-center gap-2.5 px-3 py-2 rounded-md text-xs font-medium transition-all ${
                  isActive
                    ? 'bg-forest text-white shadow-subtle dark:bg-forest'
                    : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark hover:bg-surface-light dark:hover:bg-surface-dark'
                }`}
              >
                <Icon size={15} strokeWidth={isActive ? 2.25 : 1.75} />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </div>

      {/* Bottom Section */}
      <div className="p-3 border-t border-border-light dark:border-border-dark space-y-2">
        {/* Status Indicator */}
        <div className="px-2.5 py-2 rounded-md bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0">
            <span
              className={`w-2 h-2 rounded-full shrink-0 ${
                isConnected
                  ? 'bg-emerald-500 ring-2 ring-emerald-500/20'
                  : isConnecting
                  ? 'bg-amber-500 animate-pulse'
                  : 'bg-red-500'
              }`}
            />
            <div className="truncate">
              <div className="text-[11px] font-medium text-primary-light dark:text-primary-dark truncate">
                {isConnected
                  ? 'Daemon Online'
                  : isConnecting
                  ? 'Connecting...'
                  : 'Daemon Offline'}
              </div>
              <div className="text-[10px] text-secondary-light dark:text-secondary-dark truncate">
                {isConnected && connectionStatus.health
                  ? `v${connectionStatus.health.version} (Schema ${connectionStatus.health.schema_version})`
                  : connectionStatus.error || 'UDS bridge standby'}
              </div>
            </div>
          </div>
          <button
            onClick={onToggleTheme}
            title={isDark ? 'Switch to Light Appearance' : 'Switch to Dark Appearance'}
            className="p-1.5 text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark rounded transition-colors"
          >
            {isDark ? <Sun size={13} /> : <Moon size={13} />}
          </button>
        </div>
      </div>
    </aside>
  );
};
