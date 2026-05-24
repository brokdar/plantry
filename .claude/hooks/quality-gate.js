#!/usr/bin/env node
/**
 * Quality Gate Hook — Plantry
 *
 * Runs lightweight formatting after file edits.
 *   .go              -> gofmt -w <file>
 *   .ts/.tsx/.svelte -> bunx prettier --write <file>
 *
 * Env vars:
 *   ECC_QUALITY_GATE_FIX=true    (default behaviour — always writes fixes)
 *   ECC_QUALITY_GATE_STRICT=true (log stderr on non-zero exit)
 *
 * Always exits 0 (async, non-blocking).
 */

'use strict';

const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const MAX_STDIN = 1024 * 1024;

function exec(command, args, cwd) {
  return spawnSync(command, args, {
    cwd: cwd || process.cwd(),
    encoding: 'utf8',
    env: process.env,
    timeout: 15000
  });
}

function log(msg) {
  process.stderr.write('[quality-gate] ' + msg + '\n');
}

function maybeRunQualityGate(filePath) {
  if (!filePath || !fs.existsSync(filePath)) {
    return;
  }

  filePath = path.resolve(filePath);

  var ext = path.extname(filePath).toLowerCase();
  var fix = String(process.env.ECC_QUALITY_GATE_FIX || 'true').toLowerCase() !== 'false';
  var strict = String(process.env.ECC_QUALITY_GATE_STRICT || '').toLowerCase() === 'true';

  if (ext === '.go') {
    if (fix) {
      var r = exec('gofmt', ['-w', filePath]);
      if (r.status !== 0 && strict) {
        log('gofmt failed for ' + filePath + ': ' + r.stderr);
      }
    } else if (strict) {
      var r2 = exec('gofmt', ['-l', filePath]);
      if (r2.status !== 0) {
        log('gofmt failed for ' + filePath + ': ' + r2.stderr);
      } else if (r2.stdout && r2.stdout.trim()) {
        log('gofmt check: unformatted file ' + filePath);
      }
    }
    return;
  }

  if (ext === '.ts' || ext === '.tsx' || ext === '.svelte') {
    if (fix) {
      var r3 = exec('bunx', ['prettier', '--write', filePath]);
      if (r3.status !== 0 && strict) {
        log('prettier failed for ' + filePath + ': ' + r3.stderr);
      }
    } else if (strict) {
      var r4 = exec('bunx', ['prettier', '--check', filePath]);
      if (r4.status !== 0) {
        log('prettier check failed for ' + filePath);
      }
    }
    return;
  }
}

function run(rawInput) {
  try {
    var input = JSON.parse(rawInput);
    var filePath = String((input.tool_input && input.tool_input.file_path) || '');
    maybeRunQualityGate(filePath);
  } catch (e) {
    // Ignore parse errors — never block Claude Code.
  }
  return rawInput;
}

if (require.main === module) {
  var raw = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', function(chunk) {
    if (raw.length < MAX_STDIN) {
      raw += chunk.substring(0, MAX_STDIN - raw.length);
    }
  });

  process.stdin.on('end', function() {
    var result = run(raw);
    process.stdout.write(result);
    process.exit(0);
  });
}

module.exports = { run: run };
