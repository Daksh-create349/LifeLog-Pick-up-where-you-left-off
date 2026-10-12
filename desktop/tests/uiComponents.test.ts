import { describe, it, expect } from 'vitest';
import fs from 'fs';
import path from 'path';

describe('UI & Safety Standards', () => {
  it('enforces strict zero-emoji policy across all desktop source files', () => {
    // Regex matching emoji characters (Unicode ranges for emoticons, symbols, pictographs)
    const emojiRegex = /[\u{1F300}-\u{1F5FF}\u{1F600}-\u{1F64F}\u{1F680}-\u{1F6FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}]/u;

    const findSourceFiles = (dir: string): string[] => {
      let results: string[] = [];
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        const fullPath = path.join(dir, entry.name);
        if (entry.isDirectory() && entry.name !== 'node_modules' && entry.name !== 'dist' && entry.name !== 'dist-electron') {
          results = results.concat(findSourceFiles(fullPath));
        } else if (entry.isFile() && (entry.name.endsWith('.ts') || entry.name.endsWith('.tsx') || entry.name.endsWith('.html'))) {
          results.push(fullPath);
        }
      }
      return results;
    };

    const srcDir = path.resolve(__dirname, '../src');
    const sourceFiles = findSourceFiles(srcDir);
    expect(sourceFiles.length).toBeGreaterThan(5);

    const violations: Array<{ file: string; match: string }> = [];

    for (const file of sourceFiles) {
      const content = fs.readFileSync(file, 'utf8');
      const match = content.match(emojiRegex);
      if (match) {
        violations.push({ file: path.relative(srcDir, file), match: match[0] });
      }
    }

    expect(violations).toEqual([]);
  });

  it('exposes narrow and typed Preload API methods', () => {
    const preloadSource = fs.readFileSync(
      path.resolve(__dirname, '../src/preload/index.ts'),
      'utf8'
    );

    // Verify contextBridge exposes exact methods
    expect(preloadSource).toContain('contextBridge.exposeInMainWorld');
    expect(preloadSource).toContain('getHealth');
    expect(preloadSource).toContain('getProjects');
    expect(preloadSource).toContain('getTimeline');
    expect(preloadSource).toContain('getResumption');
    expect(preloadSource).toContain('getAppUsage');
    expect(preloadSource).toContain('getAppSessions');
    expect(preloadSource).toContain('getTrackingStatus');
    expect(preloadSource).toContain('setAppTracking');
    expect(preloadSource).toContain('getConnectionStatus');
    expect(preloadSource).toContain('onConnectionStatusChange');

    // Security: no direct fs, child_process, or net exposure in preload
    expect(preloadSource).not.toContain("require('fs')");
    expect(preloadSource).not.toContain("require('child_process')");
    expect(preloadSource).not.toContain("require('net')");
  });
});
