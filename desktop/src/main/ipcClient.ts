import net from 'net';
import {
  HealthResult,
  GetProjectsOptions,
  GetProjectsResult,
  GetTimelineOptions,
  GetTimelineResult,
  GetResumptionOptions,
  GetResumptionResult,
} from './types';

export interface RPCError {
  code: number;
  message: string;
  data?: unknown;
}

interface RPCResponse<T = unknown> {
  jsonrpc: string;
  id: number | null;
  result?: T;
  error?: RPCError;
}

const MAX_PAYLOAD_BYTES = 1024 * 1024; // 1 MiB
const DEFAULT_REQUEST_TIMEOUT_MS = 10000;

export class IpcClient {
  private socket: net.Socket | null = null;
  private socketPath: string;
  private nextId = 1;
  private pendingRequests = new Map<
    number,
    {
      resolve: (val: any) => void;
      reject: (err: Error) => void;
      timer: NodeJS.Timeout;
    }
  >();
  private buffer = '';
  private isDestroyed = false;

  constructor(socketPath: string) {
    this.socketPath = socketPath;
  }

  public connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.isDestroyed) {
        return reject(new Error('IPC client has been destroyed'));
      }

      const socket = net.createConnection({ path: this.socketPath });
      this.socket = socket;

      const connectTimeout = setTimeout(() => {
        socket.destroy();
        reject(new Error(`Connection to socket ${this.socketPath} timed out`));
      }, 3000);

      socket.once('connect', () => {
        clearTimeout(connectTimeout);
        resolve();
      });

      socket.on('data', (chunk) => {
        this.handleData(chunk);
      });

      socket.once('error', (err) => {
        clearTimeout(connectTimeout);
        this.cleanupPending(err);
        if (!socket.destroyed) {
          socket.destroy();
        }
        reject(err);
      });

      socket.on('close', () => {
        this.cleanupPending(new Error('Socket connection closed'));
        this.socket = null;
      });
    });
  }

  private handleData(chunk: Buffer): void {
    if (this.buffer.length + chunk.length > MAX_PAYLOAD_BYTES) {
      const err = new Error(`Received payload exceeding limit of ${MAX_PAYLOAD_BYTES} bytes`);
      this.cleanupPending(err);
      this.close();
      return;
    }

    this.buffer += chunk.toString('utf8');

    let newlineIndex: number;
    while ((newlineIndex = this.buffer.indexOf('\n')) !== -1) {
      const line = this.buffer.slice(0, newlineIndex).trim();
      this.buffer = this.buffer.slice(newlineIndex + 1);

      if (line.length === 0) continue;

      try {
        const parsed = JSON.parse(line) as RPCResponse;
        if (parsed.id !== null && typeof parsed.id === 'number') {
          const pending = this.pendingRequests.get(parsed.id);
          if (pending) {
            clearTimeout(pending.timer);
            this.pendingRequests.delete(parsed.id);

            if (parsed.error) {
              pending.reject(
                new Error(`RPC error [${parsed.error.code}]: ${parsed.error.message}`)
              );
            } else {
              pending.resolve(parsed.result);
            }
          }
        }
      } catch (parseErr) {
        // Malformed line
        console.error('Failed to parse incoming JSON-RPC response line:', parseErr);
      }
    }
  }

  private cleanupPending(err: Error): void {
    for (const [id, pending] of this.pendingRequests.entries()) {
      clearTimeout(pending.timer);
      pending.reject(err);
      this.pendingRequests.delete(id);
    }
  }

  public call<T>(method: string, params?: unknown, timeoutMs = DEFAULT_REQUEST_TIMEOUT_MS): Promise<T> {
    return new Promise((resolve, reject) => {
      if (!this.socket || this.socket.destroyed) {
        return reject(new Error('Socket is not connected'));
      }

      const id = this.nextId++;
      const payload = {
        jsonrpc: '2.0',
        id,
        method,
        params: params === undefined ? null : params,
      };

      const timer = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error(`RPC request '${method}' [id=${id}] timed out after ${timeoutMs}ms`));
      }, timeoutMs);

      this.pendingRequests.set(id, { resolve, reject, timer });

      const line = JSON.stringify(payload) + '\n';
      this.socket.write(line, (writeErr) => {
        if (writeErr) {
          clearTimeout(timer);
          this.pendingRequests.delete(id);
          reject(writeErr);
        }
      });
    });
  }

  public async auth(token: string): Promise<{ authenticated: boolean; session_id: string; version: string }> {
    return this.call('auth', { token, client: 'lifelog-desktop' });
  }

  public async health(): Promise<HealthResult> {
    return this.call<HealthResult>('health');
  }

  public async getProjects(options?: GetProjectsOptions): Promise<GetProjectsResult> {
    return this.call<GetProjectsResult>('getProjects', options);
  }

  public async getTimeline(options?: GetTimelineOptions): Promise<GetTimelineResult> {
    return this.call<GetTimelineResult>('getTimeline', options);
  }

  public async getResumption(
    projectPath: string,
    options?: GetResumptionOptions
  ): Promise<GetResumptionResult> {
    return this.call<GetResumptionResult>('getResumption', {
      project_path: projectPath,
      ...(options || {}),
    });
  }

  public async getAppUsage(options?: import('./types').GetAppUsageOptions): Promise<import('./types').GetAppUsageResult> {
    return this.call<import('./types').GetAppUsageResult>('getAppUsage', options);
  }

  public async getAppSessions(options?: import('./types').GetAppSessionsOptions): Promise<import('./types').GetAppSessionsResult> {
    return this.call<import('./types').GetAppSessionsResult>('getAppSessions', options);
  }

  public async getTrackingStatus(): Promise<import('./types').TrackingStatus> {
    return this.call<import('./types').TrackingStatus>('getTrackingStatus');
  }

  public async setAppTracking(enabled: boolean): Promise<import('./types').SetAppTrackingResult> {
    return this.call<import('./types').SetAppTrackingResult>('setAppTracking', { enabled });
  }

  public async getSystemEvents(options?: import('./types').GetSystemEventsOptions): Promise<import('./types').GetSystemEventsResult> {
    return this.call<import('./types').GetSystemEventsResult>('getSystemEvents', options);
  }

  public async getSystemActivitySummary(options?: { since?: string; until?: string }): Promise<import('./types').SystemActivitySummary> {
    return this.call<import('./types').SystemActivitySummary>('getSystemActivitySummary', options);
  }

  public async getBrowserTrackingStatus(): Promise<import('./types').BrowserTrackingStatus> {
    return this.call<import('./types').BrowserTrackingStatus>('getBrowserTrackingStatus');
  }

  public async setBrowserTracking(enabled: boolean): Promise<import('./types').BrowserTrackingStatus> {
    const res = await this.call<{ status: import('./types').BrowserTrackingStatus }>('setBrowserTracking', { enabled });
    return res.status;
  }

  public async getDataStats(): Promise<import('./types').DataStats> {
    const res = await this.call<{ stats: import('./types').DataStats }>('getDataStats');
    return res.stats;
  }

  public async getRetentionPolicy(): Promise<import('./types').RetentionPolicy> {
    return this.call<import('./types').RetentionPolicy>('getRetentionPolicy');
  }

  public async setRetentionPolicy(retention_days: number): Promise<{ success: boolean; retention_days: number }> {
    return this.call<{ success: boolean; retention_days: number }>('setRetentionPolicy', { retention_days });
  }

  public async runRetentionCleanup(): Promise<import('./types').PurgeStats> {
    const res = await this.call<{ purge_stats: import('./types').PurgeStats }>('runRetentionCleanup');
    return res.purge_stats;
  }

  public async deleteActivity(options: import('./types').DeleteActivityOptions): Promise<import('./types').DeleteActivityResult> {
    return this.call<import('./types').DeleteActivityResult>('deleteActivity', options);
  }

  public async exportUserData(): Promise<import('./types').UserDataExport> {
    const res = await this.call<{ export: import('./types').UserDataExport }>('exportUserData');
    return res.export;
  }

  public close(): void {
    this.isDestroyed = true;
    if (this.socket) {
      this.socket.destroy();
      this.socket = null;
    }
    this.cleanupPending(new Error('IPC client closed'));
  }
}
