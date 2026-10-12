import { contextBridge, ipcRenderer } from 'electron';

export interface LifelogApi {
  getHealth: () => Promise<any>;
  getProjects: (options?: any) => Promise<any>;
  getTimeline: (options?: any) => Promise<any>;
  getResumption: (projectPath: string, options?: any) => Promise<any>;
  getAppUsage: (options?: any) => Promise<any>;
  getAppSessions: (options?: any) => Promise<any>;
  getTrackingStatus: () => Promise<any>;
  setAppTracking: (enabled: boolean) => Promise<any>;
  getSystemEvents: (options?: any) => Promise<any>;
  getSystemActivitySummary: (options?: any) => Promise<any>;
  getBrowserTrackingStatus: () => Promise<any>;
  setBrowserTracking: (enabled: boolean) => Promise<any>;
  getDataStats: () => Promise<any>;
  getRetentionPolicy: () => Promise<any>;
  setRetentionPolicy: (retention_days: number) => Promise<any>;
  runRetentionCleanup: () => Promise<any>;
  deleteActivity: (options: any) => Promise<any>;
  exportUserData: () => Promise<any>;
  getConnectionStatus: () => Promise<any>;
  onConnectionStatusChange: (callback: (status: any) => void) => () => void;
}

const api: LifelogApi = {
  getHealth: () => ipcRenderer.invoke('lifelog:getHealth'),
  getProjects: (options) => ipcRenderer.invoke('lifelog:getProjects', options),
  getTimeline: (options) => ipcRenderer.invoke('lifelog:getTimeline', options),
  getResumption: (projectPath, options) =>
    ipcRenderer.invoke('lifelog:getResumption', projectPath, options),
  getAppUsage: (options) => ipcRenderer.invoke('lifelog:getAppUsage', options),
  getAppSessions: (options) => ipcRenderer.invoke('lifelog:getAppSessions', options),
  getTrackingStatus: () => ipcRenderer.invoke('lifelog:getTrackingStatus'),
  setAppTracking: (enabled) => ipcRenderer.invoke('lifelog:setAppTracking', enabled),
  getSystemEvents: (options) => ipcRenderer.invoke('lifelog:getSystemEvents', options),
  getSystemActivitySummary: (options) => ipcRenderer.invoke('lifelog:getSystemActivitySummary', options),
  getBrowserTrackingStatus: () => ipcRenderer.invoke('lifelog:getBrowserTrackingStatus'),
  setBrowserTracking: (enabled) => ipcRenderer.invoke('lifelog:setBrowserTracking', enabled),
  getDataStats: () => ipcRenderer.invoke('lifelog:getDataStats'),
  getRetentionPolicy: () => ipcRenderer.invoke('lifelog:getRetentionPolicy'),
  setRetentionPolicy: (retention_days) => ipcRenderer.invoke('lifelog:setRetentionPolicy', retention_days),
  runRetentionCleanup: () => ipcRenderer.invoke('lifelog:runRetentionCleanup'),
  deleteActivity: (options) => ipcRenderer.invoke('lifelog:deleteActivity', options),
  exportUserData: () => ipcRenderer.invoke('lifelog:exportUserData'),
  getConnectionStatus: () => ipcRenderer.invoke('lifelog:getConnectionStatus'),
  onConnectionStatusChange: (callback) => {
    const handler = (_event: Electron.IpcRendererEvent, status: any) => callback(status);
    ipcRenderer.on('lifelog:connectionStatusChanged', handler);
    return () => {
      ipcRenderer.removeListener('lifelog:connectionStatusChanged', handler);
    };
  },
};

contextBridge.exposeInMainWorld('lifelog', api);
