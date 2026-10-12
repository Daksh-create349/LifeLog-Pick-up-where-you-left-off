import { ipcMain, BrowserWindow } from 'electron';
import { DaemonSupervisor } from './daemon';
import { GetProjectsOptions, GetTimelineOptions, GetResumptionOptions } from './types';

export function setupIpcBridge(
  supervisor: DaemonSupervisor,
  getMainWindow: () => BrowserWindow | null
): void {
  // Broadcast connection status changes to renderer
  supervisor.onStatusChange((status) => {
    const win = getMainWindow();
    if (win && !win.isDestroyed()) {
      win.webContents.send('lifelog:connectionStatusChanged', status);
    }
  });

  ipcMain.handle('lifelog:getConnectionStatus', async () => {
    return supervisor.getStatus();
  });

  ipcMain.handle('lifelog:getHealth', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.health();
  });

  ipcMain.handle('lifelog:getProjects', async (_event, options?: GetProjectsOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getProjects(options);
  });

  ipcMain.handle('lifelog:getTimeline', async (_event, options?: GetTimelineOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getTimeline(options);
  });

  ipcMain.handle(
    'lifelog:getResumption',
    async (_event, projectPath: string, options?: GetResumptionOptions) => {
      const client = supervisor.getClient();
      if (!client) {
        throw new Error('Go daemon is not connected');
      }
      return client.getResumption(projectPath, options);
    }
  );

  ipcMain.handle('lifelog:getAppUsage', async (_event, options?: import('./types').GetAppUsageOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getAppUsage(options);
  });

  ipcMain.handle('lifelog:getAppSessions', async (_event, options?: import('./types').GetAppSessionsOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getAppSessions(options);
  });

  ipcMain.handle('lifelog:getTrackingStatus', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getTrackingStatus();
  });

  ipcMain.handle('lifelog:setAppTracking', async (_event, enabled: boolean) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.setAppTracking(enabled);
  });

  ipcMain.handle('lifelog:getSystemEvents', async (_event, options?: import('./types').GetSystemEventsOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getSystemEvents(options);
  });

  ipcMain.handle('lifelog:getSystemActivitySummary', async (_event, options?: { since?: string; until?: string }) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getSystemActivitySummary(options);
  });

  ipcMain.handle('lifelog:getBrowserTrackingStatus', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getBrowserTrackingStatus();
  });

  ipcMain.handle('lifelog:setBrowserTracking', async (_event, enabled: boolean) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.setBrowserTracking(enabled);
  });

  ipcMain.handle('lifelog:getDataStats', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getDataStats();
  });

  ipcMain.handle('lifelog:getRetentionPolicy', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.getRetentionPolicy();
  });

  ipcMain.handle('lifelog:setRetentionPolicy', async (_event, retention_days: number) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.setRetentionPolicy(retention_days);
  });

  ipcMain.handle('lifelog:runRetentionCleanup', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.runRetentionCleanup();
  });

  ipcMain.handle('lifelog:deleteActivity', async (_event, options: import('./types').DeleteActivityOptions) => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.deleteActivity(options);
  });

  ipcMain.handle('lifelog:exportUserData', async () => {
    const client = supervisor.getClient();
    if (!client) {
      throw new Error('Go daemon is not connected');
    }
    return client.exportUserData();
  });
}
