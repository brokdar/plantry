#!/usr/bin/env node
/**
 * PreToolUse hook: block manual edits to go.sum and bun.lock.
 *
 * Exit codes:
 *   0 = allow
 *   2 = block (lockfile modification attempted)
 */

'use strict';

const path = require('path');

const MESSAGES = {
  'go.sum': "Blocked: go.sum is managed by Go modules. Run 'go mod tidy' instead.",
  'bun.lock': "Blocked: bun.lock is managed by bun. Run 'bun install' instead.",
};

let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => { data += chunk; });
process.stdin.on('end', () => {
  let input = {};
  try { input = JSON.parse(data); } catch {}

  const filePath = (input && input.tool_input && input.tool_input.file_path) || '';
  const basename = path.basename(filePath);

  if (MESSAGES[basename]) {
    process.stderr.write(MESSAGES[basename] + '\n');
    process.exit(2);
  }

  process.exit(0);
});
