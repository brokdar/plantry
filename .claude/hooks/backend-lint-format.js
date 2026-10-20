#!/usr/bin/env node
/**
 * PostToolUse hook: gofmt + go vet on edited Go files.
 *
 * Exit codes:
 *   0 = allow
 *   1 = go vet failed (shown to Claude)
 */

'use strict';

const path = require('path');
const { spawnSync } = require('child_process');

const backendDir = path.join(__dirname, '..', '..', 'backend');

let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => { data += chunk; });
process.stdin.on('end', () => {
  let input = {};
  try { input = JSON.parse(data); } catch {}

  const filePath = (input && input.tool_input && input.tool_input.file_path) || '';
  if (!filePath.endsWith('.go')) process.exit(0);

  spawnSync('gofmt', ['-w', filePath], { stdio: 'ignore' });

  const result = spawnSync('go', ['vet', './...'], {
    cwd: backendDir,
    encoding: 'utf8',
  });

  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout || '');
    process.exit(1);
  }

  process.exit(0);
});
