#!/usr/bin/env node
/**
 * PostToolUse Hook: Warn about console.log statements after edits
 *
 * Runs after Edit tool use. If the edited JS/TS/Svelte file contains
 * console.log statements, warns with line numbers to help remove debug
 * statements before committing.
 *
 * Exit 0 always (non-blocking).
 */

'use strict';

const fs = require('fs');

const MAX_STDIN = 1024 * 1024; // 1MB limit
let data = '';
process.stdin.setEncoding('utf8');

process.stdin.on('data', chunk => {
  if (data.length < MAX_STDIN) {
    const remaining = MAX_STDIN - data.length;
    data += chunk.substring(0, remaining);
  }
});

process.stdin.on('end', () => {
  try {
    const input = JSON.parse(data);
    const filePath = input.tool_input && input.tool_input.file_path;

    if (filePath && /\.(ts|tsx|js|jsx|svelte)$/.test(filePath)) {
      let content;
      try {
        content = fs.readFileSync(filePath, 'utf8');
      } catch (_e) {
        // File unreadable — pass through
        process.stdout.write(data);
        process.exit(0);
      }

      const lines = content.split('\n');
      const matches = [];

      lines.forEach(function(line, idx) {
        if (/console\.log/.test(line)) {
          matches.push((idx + 1) + ': ' + line.trim());
        }
      });

      if (matches.length > 0) {
        process.stderr.write('[Hook] WARNING: console.log found in ' + filePath + '\n');
        matches.slice(0, 5).forEach(function(m) {
          process.stderr.write(m + '\n');
        });
        process.stderr.write('[Hook] Remove console.log before committing\n');
      }
    }
  } catch (_e) {
    // Invalid input — pass through
  }

  process.stdout.write(data);
  process.exit(0);
});
