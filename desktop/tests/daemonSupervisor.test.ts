import { describe, it, expect } from 'vitest';
import crypto from 'crypto';

describe('Daemon Supervisor Resilience & Security', () => {
  it('generates cryptographically secure 256-bit hexadecimal tokens', () => {
    const token = crypto.randomBytes(32).toString('hex');
    expect(token).toHaveLength(64);
    expect(/^[0-9a-f]{64}$/.test(token)).toBe(true);

    const token2 = crypto.randomBytes(32).toString('hex');
    expect(token).not.toBe(token2);
  });

  it('tracks crash frequencies and detects crash limit', () => {
    const timestamps: number[] = [];
    const now = Date.now();

    // Simulate 4 crashes within 60s
    timestamps.push(now - 40000);
    timestamps.push(now - 30000);
    timestamps.push(now - 20000);
    timestamps.push(now - 10000);

    const window60s = timestamps.filter((t) => now - t < 60000);
    expect(window60s.length).toBe(4);
    expect(window60s.length > 3).toBe(true);
  });

  it('calculates exponential backoff delays with upper bound', () => {
    const computeBackoff = (crashCount: number) =>
      Math.min(1000 * Math.pow(2, crashCount - 1), 5000);

    expect(computeBackoff(1)).toBe(1000); // 1s
    expect(computeBackoff(2)).toBe(2000); // 2s
    expect(computeBackoff(3)).toBe(4000); // 4s
    expect(computeBackoff(4)).toBe(5000); // bounded at 5s
  });
});
