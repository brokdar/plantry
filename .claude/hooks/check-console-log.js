#!/usr/bin/env node
/**
 * Stop Hook: Check for console.log statements in modified files
 *
 * Runs after each response. Checks if any modified JavaScript/TypeScript/Svelte
 * files contain console.log statements. Warns to remind removal before commit.
 *
 * Exclusions: test files, config files, __tests__/, __mocks__/ directories.
 *
 * Exit 0 always (non-blocking).
 */

'use strict';

const fs = require('fs');
const { spawnSync } = require('child_process');

// Files where console.log is expected and should not trigger warnings
const EXCLUDED_PATTERNS = [
  /\.test\.[jt]sx?$/,
  /\.spec\.[jt]sx?$/,
  /\.config\.[jt]s$/,
  /__tests__\//,
  /__mocks__\//,
];

const MAX_STDIN = 1024 * 1024; // 1MB limit
let data = '';
process.stdin.setEncoding('utf8');

process.stdin.on('data', function(chunk) {
  if (data.length < MAX_STDIN) {
    const remaining = MAX_STDIN - data.length;
    data += chunk.substring(0, remaining);
  }
});

process.stdin.on('end', function() {
  try {
    // Verify we're in a git repo
    const check = spawnSync('git', ['rev-parse', '--git-dir'], { stdio: 'pipe' });
    if (check.status !== 0) {
      process.stdout.write(data);
      process.exit(0);
    }

    // Get modified files from git (no-shell, argument array — injection-safe)
    const result = spawnSync('git', ['diff', '--name-only', 'HEAD'], {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe']
    });

    if (result.status !== 0 || !result.stdout) {
      process.stdout.write(data);
      process.exit(0);
    }

    const files = result.stdout
      .trim()
      .split('\n')
      .filter(Boolean)
      .filter(function(f) { return /\.(tsx?|jsx?|svelte)$/.test(f); })
      .filter(function(f) { return fs.existsSync(f); })
      .filter(function(f) { return !EXCLUDED_PATTERNS.some(function(p) { return p.test(f); }); });

    let hasConsole = false;

    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      let content;
      try {
        content = fs.readFileSync(file, 'utf8');
      } catch (_e) {
        continue;
      }
      if (content && content.includes('console.log')) {
        process.stderr.write('[Hook] WARNING: console.log found in ' + file + '\n');
        hasConsole = true;
      }
    }

    if (hasConsole) {
      process.stderr.write('[Hook] Remove console.log statements before committing\n');
    }
  } catch (err) {
    process.stderr.write('[Hook] check-console-log error: ' + err.message + '\n');
  }

  process.stdout.write(data);
  process.exit(0);
});
