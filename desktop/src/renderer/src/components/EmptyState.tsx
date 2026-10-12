import React from 'react';
import { LucideIcon } from 'lucide-react';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description: string;
  action?: {
    label: string;
    onClick: () => void;
  };
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  icon: Icon,
  title,
  description,
  action,
}) => {
  return (
    <div className="flex flex-col items-center justify-center p-12 text-center border border-dashed border-border-light dark:border-border-dark rounded-xl bg-surfaceSubtle-light/40 dark:bg-surfaceSubtle-dark/40 my-6">
      <div className="w-12 h-12 rounded-full bg-forest-lightTint dark:bg-forest-darkTint flex items-center justify-center text-forest dark:text-forest-dark mb-4">
        <Icon size={24} strokeWidth={1.75} />
      </div>
      <h3 className="text-sm font-semibold text-primary-light dark:text-primary-dark tracking-tight mb-1">
        {title}
      </h3>
      <p className="text-xs text-secondary-light dark:text-secondary-dark max-w-sm leading-relaxed mb-4">
        {description}
      </p>
      {action && (
        <button
          onClick={action.onClick}
          className="px-3.5 py-1.5 text-xs font-medium text-white bg-forest hover:bg-forest-hover dark:bg-forest dark:hover:bg-forest-hover rounded-md transition-colors shadow-subtle"
        >
          {action.label}
        </button>
      )}
    </div>
  );
};
