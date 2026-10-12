import React from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';

interface ErrorStateProps {
  title?: string;
  message: string;
  onRetry?: () => void;
}

export const ErrorState: React.FC<ErrorStateProps> = ({
  title = 'Failed to load data',
  message,
  onRetry,
}) => {
  return (
    <div className="flex flex-col items-center justify-center p-8 text-center border border-red-200 dark:border-red-950/60 rounded-xl bg-red-50/40 dark:bg-red-950/20 my-6">
      <div className="w-10 h-10 rounded-full bg-red-100 dark:bg-red-900/40 flex items-center justify-center text-red-600 dark:text-red-400 mb-3">
        <AlertCircle size={20} strokeWidth={2} />
      </div>
      <h3 className="text-sm font-semibold text-red-900 dark:text-red-300 mb-1">
        {title}
      </h3>
      <p className="text-xs text-red-700 dark:text-red-400/90 max-w-md leading-relaxed mb-4 font-mono">
        {message}
      </p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-red-800 dark:text-red-200 bg-red-100 hover:bg-red-200 dark:bg-red-900/60 dark:hover:bg-red-900 rounded-md transition-colors"
        >
          <RefreshCw size={13} />
          Retry Request
        </button>
      )}
    </div>
  );
};
