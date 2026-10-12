import React, { useState } from 'react';
import {
  Clock,
  GitBranch,
  Terminal,
  FileCode,
  Filter,
  Layers,
} from 'lucide-react';
import { TimelineItem } from '../types';
import { EmptyState } from '../components/EmptyState';

interface TimelineViewProps {
  items: TimelineItem[];
  isLoading: boolean;
  onFilterChange: (filters: { domain?: string; window?: string; limit?: number }) => void;
  activeDomain: string;
  activeWindow: string;
}

export const TimelineView: React.FC<TimelineViewProps> = ({
  items,
  isLoading,
  onFilterChange,
  activeDomain,
  activeWindow,
}) => {
  const [limit, setLimit] = useState(50);

  const domains = [
    { id: 'all', label: 'All Domains', icon: Layers },
    { id: 'filesystem', label: 'Filesystem', icon: FileCode },
    { id: 'git', label: 'Git Repositories', icon: GitBranch },
    { id: 'terminal', label: 'Terminal Commands', icon: Terminal },
  ] as const;

  const windows = [
    { id: 'all', label: 'All Time' },
    { id: '24h', label: 'Last 24 Hours' },
    { id: '7d', label: 'Last 7 Days' },
    { id: '30d', label: 'Last 30 Days' },
  ] as const;

  const handleDomainSelect = (domId: string) => {
    onFilterChange({ domain: domId, window: activeWindow, limit });
  };

  const handleWindowSelect = (winId: string) => {
    onFilterChange({ domain: activeDomain, window: winId, limit });
  };

  const handleLimitSelect = (newLimit: number) => {
    setLimit(newLimit);
    onFilterChange({ domain: activeDomain, window: activeWindow, limit: newLimit });
  };

  const getDomainBadge = (domain: string) => {
    switch (domain.toLowerCase()) {
      case 'git':
        return (
          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono font-medium bg-forest-lightTint dark:bg-forest-darkTint text-forest dark:text-forest-dark border border-forest/20">
            <GitBranch size={10} />
            <span>git</span>
          </span>
        );
      case 'terminal':
        return (
          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono font-medium bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-400 border border-amber-200 dark:border-amber-900/40">
            <Terminal size={10} />
            <span>terminal</span>
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono font-medium bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark text-secondary-light dark:text-secondary-dark border border-border-light dark:border-border-dark">
            <FileCode size={10} />
            <span>fs</span>
          </span>
        );
    }
  };

  const formatDate = (ts: string) => {
    try {
      const d = new Date(ts);
      return {
        date: d.toLocaleDateString([], { month: 'short', day: 'numeric' }),
        time: d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      };
    } catch {
      return { date: '', time: ts };
    }
  };

  return (
    <div className="p-8 space-y-6 max-w-6xl mx-auto">
      {/* Filters Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-3 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle">
        {/* Domain Filter Pills */}
        <div className="flex items-center gap-1.5 overflow-x-auto">
          {domains.map((dom) => {
            const Icon = dom.icon;
            const isSelected = activeDomain === dom.id;
            return (
              <button
                key={dom.id}
                onClick={() => handleDomainSelect(dom.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                  isSelected
                    ? 'bg-forest text-white shadow-subtle dark:bg-forest'
                    : 'text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark'
                }`}
              >
                <Icon size={12} />
                <span>{dom.label}</span>
              </button>
            );
          })}
        </div>

        {/* Window & Limit Controls */}
        <div className="flex items-center gap-3">
          {/* Time Window Dropdown */}
          <div className="flex items-center gap-1.5 text-xs text-secondary-light dark:text-secondary-dark">
            <Filter size={12} />
            <select
              value={activeWindow}
              onChange={(e) => handleWindowSelect(e.target.value)}
              className="bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark rounded-md px-2 py-1 text-xs text-primary-light dark:text-primary-dark outline-none cursor-pointer"
            >
              {windows.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.label}
                </option>
              ))}
            </select>
          </div>

          {/* Limit Dropdown */}
          <div className="flex items-center gap-1 text-xs text-secondary-light dark:text-secondary-dark">
            <span>Show:</span>
            <select
              value={limit}
              onChange={(e) => handleLimitSelect(Number(e.target.value))}
              className="bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark rounded-md px-2 py-1 text-xs text-primary-light dark:text-primary-dark outline-none cursor-pointer"
            >
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </div>
        </div>
      </div>

      {/* Timeline Stream */}
      <div className="rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-subtle overflow-hidden">
        <div className="p-4 border-b border-border-light dark:border-border-dark flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold text-primary-light dark:text-primary-dark">
              Chronological Events
            </h2>
            <span className="px-2 py-0.5 rounded text-[11px] font-mono bg-surfaceSubtle-light dark:bg-surfaceSubtle-dark border border-border-light dark:border-border-dark text-secondary-light dark:text-secondary-dark">
              {isLoading ? '...' : `${items.length} records`}
            </span>
          </div>
        </div>

        {items.length === 0 ? (
          <div className="p-8">
            <EmptyState
              icon={Clock}
              title="No events found for this filter"
              description="Try adjusting your memory domain or lookback window above."
            />
          </div>
        ) : (
          <div className="divide-y divide-border-light/60 dark:divide-border-dark/60">
            {items.map((item) => {
              const { date, time } = formatDate(item.timestamp);
              return (
                <div
                  key={item.record_id}
                  className="p-3.5 hover:bg-surfaceSubtle-light/60 dark:hover:bg-surfaceSubtle-dark/60 transition-colors flex items-start justify-between gap-4 text-xs"
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <div className="shrink-0 mt-0.5">{getDomainBadge(item.domain)}</div>
                    <div className="min-w-0 space-y-1">
                      <div className="font-semibold text-primary-light dark:text-primary-dark break-words">
                        {item.title}
                      </div>
                      {item.working_path && (
                        <div className="text-[11px] text-secondary-light dark:text-secondary-dark font-mono truncate">
                          {item.working_path}
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="shrink-0 text-right space-y-0.5 font-mono text-[11px] text-secondary-light dark:text-secondary-dark">
                    <div>{date}</div>
                    <div className="text-[10px] opacity-75">{time}</div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
