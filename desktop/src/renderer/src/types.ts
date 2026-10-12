import {
  HealthResult,
  ProjectOverview,
  GetProjectsOptions,
  GetProjectsResult,
  TimelineItem,
  GetTimelineOptions,
  GetTimelineResult,
  ResumptionReport,
  GetResumptionOptions,
  GetResumptionResult,
  ConnectionStatus,
  AppUsageStat,
  GetAppUsageResult,
  GetAppUsageOptions,
  AppSession,
  GetAppSessionsOptions,
  GetAppSessionsResult,
  TrackingStatus,
  SetAppTrackingResult,
  SystemEvent,
  GetSystemEventsOptions,
  GetSystemEventsResult,
  SystemActivitySummary,
  BrowserTrackingStatus,
  DataStats,
  RetentionPolicy,
  PurgeStats,
  DeleteActivityOptions,
  DeleteActivityResult,
  UserDataExport,
} from '../../main/types';

export type {
  HealthResult,
  ProjectOverview,
  GetProjectsOptions,
  GetProjectsResult,
  TimelineItem,
  GetTimelineOptions,
  GetTimelineResult,
  ResumptionReport,
  GetResumptionOptions,
  GetResumptionResult,
  ConnectionStatus,
  AppUsageStat,
  GetAppUsageResult,
  GetAppUsageOptions,
  AppSession,
  GetAppSessionsOptions,
  GetAppSessionsResult,
  TrackingStatus,
  SetAppTrackingResult,
  SystemEvent,
  GetSystemEventsOptions,
  GetSystemEventsResult,
  SystemActivitySummary,
  BrowserTrackingStatus,
  DataStats,
  RetentionPolicy,
  PurgeStats,
  DeleteActivityOptions,
  DeleteActivityResult,
  UserDataExport,
};

export type NavView = 'overview' | 'timeline' | 'projects' | 'apps' | 'resumption' | 'settings';

declare global {
  interface Window {
    lifelog: {
      getHealth: () => Promise<HealthResult>;
      getProjects: (options?: GetProjectsOptions) => Promise<GetProjectsResult>;
      getTimeline: (options?: GetTimelineOptions) => Promise<GetTimelineResult>;
      getResumption: (projectPath: string, options?: GetResumptionOptions) => Promise<GetResumptionResult>;
      getAppUsage: (options?: GetAppUsageOptions) => Promise<GetAppUsageResult>;
      getAppSessions: (options?: GetAppSessionsOptions) => Promise<GetAppSessionsResult>;
      getTrackingStatus: () => Promise<TrackingStatus>;
      setAppTracking: (enabled: boolean) => Promise<SetAppTrackingResult>;
      getSystemEvents: (options?: GetSystemEventsOptions) => Promise<GetSystemEventsResult>;
      getSystemActivitySummary: (options?: { since?: string; until?: string }) => Promise<SystemActivitySummary>;
      getBrowserTrackingStatus: () => Promise<BrowserTrackingStatus>;
      setBrowserTracking: (enabled: boolean) => Promise<BrowserTrackingStatus>;
      getDataStats: () => Promise<DataStats>;
      getRetentionPolicy: () => Promise<RetentionPolicy>;
      setRetentionPolicy: (retention_days: number) => Promise<{ success: boolean; retention_days: number }>;
      runRetentionCleanup: () => Promise<PurgeStats>;
      deleteActivity: (options: DeleteActivityOptions) => Promise<DeleteActivityResult>;
      exportUserData: () => Promise<UserDataExport>;
      getConnectionStatus: () => Promise<ConnectionStatus>;
      onConnectionStatusChange: (callback: (status: ConnectionStatus) => void) => () => void;
    };
  }
}

