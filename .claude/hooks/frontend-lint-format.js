#!/usr/bin/env node
/**
 * PostToolUse hook: prettier + eslint on edited TS/TSX files.
 *
 * Exit codes:
 *   0 = allow
 *   1 = eslint failed (shown to Claude)
 */

'use strict';

const path = require('path');
const { spawnSync } = require('child_process');

const frontendDir = path.join(__dirname, '..', '..', 'frontend');

let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => { data += chunk; });
process.stdin.on('end', () => {
  let input = {};
  try { input = JSON.parse(data); } catch {}

  const filePath = (input && input.tool_input && input.tool_input.file_path) || '';
  if (!/\.(ts|tsx)$/.test(filePath)) process.exit(0);

  spawnSync('bunx', ['prettier', '--write', filePath], {
    cwd: frontendDir,
    stdio: 'ignore',
  });

  const result = spawnSync('bunx', ['eslint', '--fix', filePath], {
    cwd: frontendDir,
    encoding: 'utf8',
  });

  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout || '');
    process.exit(1);
  }

  process.exit(0);
});
