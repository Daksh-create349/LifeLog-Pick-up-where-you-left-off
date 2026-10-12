import React from 'react';
import { RefreshCw, Search } from 'lucide-react';

interface HeaderProps {
  title: string;
  subtitle?: string;
  onRefresh?: () => void;
  isLoading?: boolean;
  onOpenSearch?: () => void;
  actions?: React.ReactNode;
}

export const Header: React.FC<HeaderProps> = ({
  title,
  subtitle,
  onRefresh,
  isLoading,
  onOpenSearch,
  actions,
}) => {
  return (
    <header className="h-16 px-8 border-b border-border-light dark:border-border-dark flex items-center justify-between bg-surface-light/60 dark:bg-surface-dark/60 backdrop-blur-md sticky top-0 z-10 titlebar-drag">
      <div className="titlebar-nodrag">
        <h1 className="text-lg font-semibold tracking-tight text-primary-light dark:text-primary-dark leading-tight">
          {title}
        </h1>
        {subtitle && (
          <p className="text-xs text-secondary-light dark:text-secondary-dark mt-0.5">
            {subtitle}
          </p>
        )}
      </div>

      <div className="flex items-center gap-2 titlebar-nodrag">
        {actions}

        {onOpenSearch && (
          <button
            onClick={onOpenSearch}
            className="p-1.5 text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark rounded-md border border-border-light dark:border-border-dark transition-colors"
            title="Global Search (⌘K)"
          >
            <Search size={14} />
          </button>
        )}

        {onRefresh && (
          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="p-1.5 text-secondary-light dark:text-secondary-dark hover:text-primary-light dark:hover:text-primary-dark hover:bg-surfaceSubtle-light dark:hover:bg-surfaceSubtle-dark rounded-md border border-border-light dark:border-border-dark transition-colors disabled:opacity-50"
            title="Refresh Data"
          >
            <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
          </button>
        )}
      </div>
    </header>
  );
};
