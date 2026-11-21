#!/usr/bin/env node
/**
 * PreToolUse hook: keep tests fast (see "Test speed contract" in CLAUDE.md).
 *
 * Bash:
 *   - `go test -race` outside scripts/gate.sh without -run: the race detector
 *     costs 4-8x; the edit loop runs targeted tests without it.
 *   - `go test -count=1` without -run: it switches off the test cache, which
 *     is what runs only the affected packages.
 * Edit/Write/MultiEdit:
 *   - raising a timeout in CI workflows, the gate, Playwright or Vitest config,
 *     or a budget in test-budget.json. A timeout is a symptom: profile the slow
 *     test and fix it. A deliberate raise goes into test-budget.json with a
 *     reason, and that file's raises are checked against origin/main in CI.
 *
 * Exit codes: 0 = allow, 2 = block (reason on stderr, shown to the agent).
 */

'use strict';

const fs = require('fs');
const path = require('path');

const PROFILE =
  'Profile instead: `go test -run X -cpuprofile cpu.out ./pkg` + `go tool pprof -top`, ' +
  'or the per-test times in the gate log. Fix the cause (fixture rebuilt per test, ' +
  'a sweep in a unit test, a query or evaluation inside a loop).';

const TIMEOUT_FILES = [
  /\.github\/workflows\/[^/]+\.ya?ml$/,
  /scripts\/gate\.sh$/,
  /frontend\/playwright\.config\.ts$/,
  /frontend\/vite(st)?\.config\.ts$/,
];

// Timeout-like values in a text, in seconds, keyed by position.
function timeouts(text) {
  const out = [];
  const re = /(timeout-minutes:\s*(\d+))|(-timeout[ =](\d+)([smh]))|(\b(?:timeout|testTimeout|hookTimeout|navigationTimeout|actionTimeout):\s*([\d_]+))/g;
  let m;
  while ((m = re.exec(text))) {
    if (m[2]) out.push(Number(m[2]) * 60);
    else if (m[4]) out.push(Number(m[4]) * { s: 1, m: 60, h: 3600 }[m[5]]);
    else if (m[7]) out.push(Number(m[7].replace(/_/g, '')) / 1000);
  }
  return out;
}

// Budget numbers in test-budget.json text: every "cpu_s"/"wall_s" value and
// every timeouts entry, by key path.
function budgets(text) {
  let j;
  try { j = JSON.parse(text); } catch { return null; }
  const out = {};
  const walk = (v, p) => {
    if (v && typeof v === 'object') {
      for (const [k, x] of Object.entries(v)) walk(x, p ? `${p}.${k}` : k);
    } else if (/\.(cpu_s|wall_s)$/.test(p) || /^timeouts\./.test(p) || /^single_test_max_s/.test(p) || /^tolerance/.test(p)) {
      out[p] = typeof v === 'number' ? v : parseFloat(String(v).replace(/_/g, '')) || 0;
    }
  };
  walk(j, '');
  return out;
}

function block(msg) {
  process.stderr.write(msg + '\n');
  process.exit(2);
}

function checkBash(raw) {
  const cmd = raw
    .replace(/<<-?\s*['"]?(\w+)['"]?[^\n]*\n[\s\S]*?\n\s*\1\b/g, '')
    .replace(/'[^']*'/g, "''")
    .replace(/"(?:[^"\\]|\\.)*"/g, '""');
  for (const part of cmd.split(/&&|\|\||;|\n/)) {
    if (!/\bgo\s+test\b/.test(part)) continue;
    const narrowed = /\s-run[ =]/.test(part);
    if (/\s-race\b/.test(part) && !narrowed) {
      block('Blocked: `go test -race` without -run. The race tier runs once per PR in scripts/gate.sh ' +
        '(-race -short). In the edit loop run the affected test without -race: ' +
        '`go test ./pkg -run TestX`. Use -race with -run only to chase a data race.');
    }
    if (/\s-count[ =]1\b/.test(part) && !narrowed) {
      block('Blocked: `go test -count=1` without -run. It disables the test cache, which is what ' +
        'runs only the packages your change affects. Drop -count=1 (or narrow with -run to chase a flake).');
    }
  }
}

function checkEdit(input) {
  const file = input.file_path || '';
  const isBudget = /(^|\/)test-budget\.json$/.test(file);
  const isTimeoutFile = TIMEOUT_FILES.some(r => r.test(file));
  if (!isBudget && !isTimeoutFile) return;

  let before = '';
  try { before = fs.readFileSync(file, 'utf8'); } catch { return; } // a new file is fine
  let after = before;
  if (typeof input.content === 'string') after = input.content;
  else if (Array.isArray(input.edits)) {
    for (const e of input.edits) after = after.split(e.old_string).join(e.new_string);
  } else if (typeof input.old_string === 'string') {
    after = input.replace_all
      ? after.split(input.old_string).join(input.new_string)
      : after.replace(input.old_string, input.new_string);
  }

  if (isTimeoutFile) {
    const a = timeouts(before), b = timeouts(after);
    const raised = b.some((v, i) => i >= a.length ? false : v > a[i]) || b.length > a.length;
    if (raised) {
      block(`Blocked: this edit raises a timeout in ${path.basename(file)}. A timeout is a tripwire for hangs, ` +
        'never the fix for slow tests. ' + PROFILE +
        ' If a raise is truly needed, record it in test-budget.json "timeouts" with reasons["timeouts:<file>"] ' +
        'saying why it is slow and why it cannot be faster, and tell the user.');
    }
  }
  if (isBudget) {
    const a = budgets(before), b = budgets(after);
    if (!a || !b) return;
    const ra = (() => { try { return JSON.parse(before).reasons || {}; } catch { return {}; } })();
    const rb = (() => { try { return JSON.parse(after).reasons || {}; } catch { return {}; } })();
    const newReason = Object.keys(rb).some(k => (rb[k] || '').trim() && rb[k] !== ra[k]);
    const raised = Object.keys(b).filter(k => k in a && b[k] > a[k]);
    if (raised.length && !newReason) {
      block(`Blocked: this edit raises ${raised.join(', ')} in test-budget.json without a new reason. ` +
        'Tests may get slower only in proportion to the tests added. ' + PROFILE +
        ' A justified raise adds reasons["<section>:<name>"]: why it costs this much and why it cannot be faster.');
    }
  }
}

let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', c => { data += c; });
process.stdin.on('end', () => {
  let input = {};
  try { input = JSON.parse(data); } catch {}
  const ti = (input && input.tool_input) || {};
  if (input.tool_name === 'Bash') checkBash(ti.command || '');
  else checkEdit(ti);
  process.exit(0);
});
