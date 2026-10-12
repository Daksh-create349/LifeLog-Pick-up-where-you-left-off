import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import net from 'net';
import fs from 'fs';
import path from 'path';
import os from 'os';
import { IpcClient } from '../src/main/ipcClient';

describe('IpcClient (Unix Domain Socket JSON-RPC 2.0)', () => {
  let server: net.Server;
  let socketPath: string;

  beforeEach(async () => {
    const tmpDir = path.join(os.tmpdir(), `ll_test_${Date.now()}_${Math.random().toString(36).substring(2, 7)}`);
    fs.mkdirSync(tmpDir, { recursive: true, mode: 0o700 });
    socketPath = path.join(tmpDir, 'test.sock');
  });

  afterEach(async () => {
    if (server) {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
    const dir = path.dirname(socketPath);
    if (fs.existsSync(dir)) {
      try {
        fs.rmSync(dir, { recursive: true, force: true });
      } catch {}
    }
  });

  it('connects, sends JSON-RPC 2.0 requests, and parses responses', async () => {
    server = net.createServer((conn) => {
      conn.on('data', (data) => {
        const line = data.toString('utf8').trim();
        const req = JSON.parse(line);
        if (req.method === 'auth') {
          conn.write(JSON.stringify({
            jsonrpc: '2.0',
            id: req.id,
            result: { authenticated: true, session_id: 'sess_123', version: '0.1.0' },
          }) + '\n');
        } else if (req.method === 'health') {
          conn.write(JSON.stringify({
            jsonrpc: '2.0',
            id: req.id,
            result: {
              protocol_version: '2.0',
              daemon_status: 'healthy',
              version: '0.1.0',
              schema_version: 7,
              uptime_seconds: 42,
              db_path: '/path/to/lifelog.db',
              pid: 1234,
            },
          }) + '\n');
        }
      });
    });

    await new Promise<void>((resolve) => server.listen(socketPath, resolve));

    const client = new IpcClient(socketPath);
    await client.connect();

    const authRes = await client.auth('test_token');
    expect(authRes.authenticated).toBe(true);
    expect(authRes.session_id).toBe('sess_123');

    const healthRes = await client.health();
    expect(healthRes.daemon_status).toBe('healthy');
    expect(healthRes.schema_version).toBe(7);

    client.close();
  });

  it('handles JSON-RPC error responses cleanly', async () => {
    server = net.createServer((conn) => {
      conn.on('data', (data) => {
        const req = JSON.parse(data.toString('utf8').trim());
        conn.write(JSON.stringify({
          jsonrpc: '2.0',
          id: req.id,
          error: { code: -32001, message: 'unauthorized: session not authenticated' },
        }) + '\n');
      });
    });

    await new Promise<void>((resolve) => server.listen(socketPath, resolve));

    const client = new IpcClient(socketPath);
    await client.connect();

    await expect(client.health()).rejects.toThrow('RPC error [-32001]: unauthorized: session not authenticated');

    client.close();
  });

  it('rejects pending requests when socket closes unexpectedly', async () => {
    server = net.createServer((conn) => {
      conn.on('data', () => {
        // Close connection immediately without replying
        conn.destroy();
      });
    });

    await new Promise<void>((resolve) => server.listen(socketPath, resolve));

    const client = new IpcClient(socketPath);
    await client.connect();

    await expect(client.call('testMethod')).rejects.toThrow();

    client.close();
  });

  it('enforces request timeout', async () => {
    server = net.createServer((conn) => {
      conn.on('data', () => {
        // Never reply
      });
    });

    await new Promise<void>((resolve) => server.listen(socketPath, resolve));

    const client = new IpcClient(socketPath);
    await client.connect();

    // Set 100ms timeout
    await expect(client.call('delayedMethod', null, 100)).rejects.toThrow('timed out after 100ms');

    client.close();
  });
});
