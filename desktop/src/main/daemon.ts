import { spawn, ChildProcess, execSync } from 'child_process';
import crypto from 'crypto';
import fs from 'fs';
import net from 'net';
import os from 'os';
import path from 'path';
import { IpcClient } from './ipcClient';
import { ConnectionStatus, HealthResult } from './types';

export class DaemonSupervisor {
  private child: ChildProcess | null = null;
  private client: IpcClient | null = null;
  private token: string;
  private socketPath: string;
  private binPath: string | null = null;
  private status: ConnectionStatus = { state: 'disconnected' };
  private onStatusChangeCallbacks: Array<(status: ConnectionStatus) => void> = [];
  private crashTimestamps: number[] = [];
  private isStopping = false;
  private restartTimer: NodeJS.Timeout | null = null;

  constructor() {
    this.token = crypto.randomBytes(32).toString('hex');
    this.socketPath = this.resolveSocketPath();
  }

  private resolveSocketPath(): string {
    const home = os.homedir();
    const appSupportDir = path.join(home, 'Library', 'Application Support', 'LifeLog');
    try {
      if (!fs.existsSync(appSupportDir)) {
        fs.mkdirSync(appSupportDir, { recursive: true, mode: 0o700 });
      }
    } catch (e) {
      // Fallback to temp directory if App Support is inaccessible
      const tmpDir = path.join(os.tmpdir(), 'lifelog');
      if (!fs.existsSync(tmpDir)) {
        fs.mkdirSync(tmpDir, { recursive: true, mode: 0o700 });
      }
      return path.join(tmpDir, 'lifelog.sock');
    }
    return path.join(appSupportDir, 'lifelog.sock');
  }

  private resolveBinary(): string {
    if (process.env.LIFELOG_DAEMON_BIN && fs.existsSync(process.env.LIFELOG_DAEMON_BIN)) {
      return process.env.LIFELOG_DAEMON_BIN;
    }

    // Check packaged Electron resourcesPath
    if ((process as any).resourcesPath) {
      const packagedCandidates = [
        path.join((process as any).resourcesPath, 'bin', 'lifelog'),
        path.join((process as any).resourcesPath, 'lifelog'),
      ];
      for (const cand of packagedCandidates) {
        if (fs.existsSync(cand)) {
          return cand;
        }
      }
    }

    // Traverse upwards from __dirname and process.cwd() to find lifelog or go.mod
    const searchRoots = [__dirname, process.cwd()];
    for (const start of searchRoots) {
      let cur = start;
      for (let i = 0; i < 5; i++) {
        const candidateBin = path.join(cur, 'lifelog');
        if (fs.existsSync(candidateBin)) {
          return candidateBin;
        }

        const candidateGoMod = path.join(cur, 'go.mod');
        if (fs.existsSync(candidateGoMod)) {
          try {
            console.log('Compiling Go daemon binary in', cur);
            execSync('go build -o lifelog .', { cwd: cur, stdio: 'ignore' });
            if (fs.existsSync(candidateBin)) {
              return candidateBin;
            }
          } catch (buildErr) {
            console.warn('Auto-compilation of Go binary failed:', buildErr);
          }
        }

        const parent = path.dirname(cur);
        if (parent === cur) break;
        cur = parent;
      }
    }

    throw new Error(
      'Could not locate lifelog Go binary. Please run "go build -o lifelog ." in the repository root.'
    );
  }

  public getStatus(): ConnectionStatus {
    return { ...this.status };
  }

  public getClient(): IpcClient | null {
    return this.client;
  }

  public onStatusChange(cb: (status: ConnectionStatus) => void): () => void {
    this.onStatusChangeCallbacks.push(cb);
    return () => {
      this.onStatusChangeCallbacks = this.onStatusChangeCallbacks.filter((c) => c !== cb);
    };
  }

  private updateStatus(newStatus: ConnectionStatus): void {
    this.status = newStatus;
    for (const cb of this.onStatusChangeCallbacks) {
      cb({ ...newStatus });
    }
  }

  public async start(): Promise<void> {
    this.isStopping = false;
    this.updateStatus({ state: 'connecting', socketPath: this.socketPath });

    try {
      this.binPath = this.resolveBinary();
    } catch (err: any) {
      this.updateStatus({
        state: 'error',
        error: err?.message || 'Binary resolution failed',
      });
      return;
    }

    this.spawnDaemonProcess();
  }

  private spawnDaemonProcess(): void {
    if (!this.binPath || this.isStopping) return;

    console.log(`Starting Go daemon supervisor (socket: ${this.socketPath})...`);

    try {
      this.child = spawn(this.binPath, ['daemon', `--socket=${this.socketPath}`], {
        env: {
          ...process.env,
          LIFELOG_IPC_TOKEN: this.token,
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (err: any) {
      this.updateStatus({
        state: 'error',
        error: `Failed to spawn daemon process: ${err?.message}`,
      });
      return;
    }

    const pid = this.child.pid;

    this.child.stderr?.on('data', (chunk) => {
      const msg = chunk.toString('utf8').trim();
      if (msg) console.error(`[Go daemon stderr]: ${msg}`);
    });

    this.child.once('exit', (code, signal) => {
      console.log(`Daemon process exited (code=${code}, signal=${signal})`);
      this.child = null;
      if (this.client) {
        this.client.close();
        this.client = null;
      }

      if (this.isStopping) {
        this.updateStatus({ state: 'disconnected' });
        return;
      }

      // Track crash frequency
      const now = Date.now();
      this.crashTimestamps = this.crashTimestamps.filter((t) => now - t < 60000);
      this.crashTimestamps.push(now);

      if (this.crashTimestamps.length > 3) {
        this.updateStatus({
          state: 'error',
          error:
            'Daemon crashed repeatedly (more than 3 times in 60s). Please run "lifelog doctor" in terminal.',
        });
        return;
      }

      // Schedule backoff restart
      const backoffMs = Math.min(1000 * Math.pow(2, this.crashTimestamps.length - 1), 5000);
      this.updateStatus({
        state: 'connecting',
        error: `Daemon exited unexpectedly. Reconnecting in ${backoffMs / 1000}s...`,
      });

      this.restartTimer = setTimeout(() => {
        if (!this.isStopping) {
          this.spawnDaemonProcess();
        }
      }, backoffMs);
    });

    // Wait for socket to become active and perform handshake
    this.waitForSocketAndHandshake(pid);
  }

  private async waitForSocketAndHandshake(pid?: number): Promise<void> {
    const deadline = Date.now() + 6000;
    let socketReady = false;

    while (Date.now() < deadline && !this.isStopping) {
      try {
        await new Promise<void>((resolve, reject) => {
          const s = net.createConnection({ path: this.socketPath });
          s.once('connect', () => {
            s.destroy();
            resolve();
          });
          s.once('error', (err) => {
            s.destroy();
            reject(err);
          });
        });
        socketReady = true;
        break;
      } catch {
        await new Promise((r) => setTimeout(r, 100));
      }
    }

    if (!socketReady || this.isStopping) {
      if (!this.isStopping) {
        this.updateStatus({
          state: 'error',
          error: `Timed out waiting for daemon socket ${this.socketPath} to become ready`,
        });
      }
      return;
    }

    // Connect client and authenticate
    try {
      const client = new IpcClient(this.socketPath);
      await client.connect();

      const authRes = await client.auth(this.token);
      if (!authRes.authenticated) {
        throw new Error('Daemon rejected authentication handshake');
      }

      const healthRes = await client.health();
      this.client = client;

      this.updateStatus({
        state: 'connected',
        health: healthRes,
        socketPath: this.socketPath,
        pid: pid || healthRes.pid,
      });

      console.log('Successfully connected and authenticated with Go daemon');
    } catch (err: any) {
      console.error('Failed to establish authenticated bridge with Go daemon:', err);
      this.updateStatus({
        state: 'error',
        error: `Authentication failed: ${err?.message}`,
      });
    }
  }

  public async stop(): Promise<void> {
    this.isStopping = true;
    if (this.restartTimer) {
      clearTimeout(this.restartTimer);
      this.restartTimer = null;
    }

    if (this.client) {
      this.client.close();
      this.client = null;
    }

    if (this.child && !this.child.killed) {
      console.log('Sending SIGTERM to Go daemon...');
      this.child.kill('SIGTERM');

      await new Promise<void>((resolve) => {
        const killTimer = setTimeout(() => {
          if (this.child && !this.child.killed) {
            console.warn('Go daemon did not exit within 3s, sending SIGKILL...');
            this.child.kill('SIGKILL');
          }
          resolve();
        }, 3000);

        if (this.child) {
          this.child.once('exit', () => {
            clearTimeout(killTimer);
            resolve();
          });
        } else {
          clearTimeout(killTimer);
          resolve();
        }
      });
    }

    this.updateStatus({ state: 'disconnected' });
  }
}
