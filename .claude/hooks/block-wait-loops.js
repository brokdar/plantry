#!/usr/bin/env node
/**
 * PreToolUse hook (Bash): block wait loops that can run forever.
 *
 * 1. while/until loops that poll processes (pgrep, pidof, ps|grep).
 *    `pgrep -f X` matches the loop's own `bash -c "...X..."`, so it never ends.
 * 2. Busy loops (`do :; done`, `do true; done`).
 * 3. Unbounded while/until + sleep polling not wrapped in `timeout`.
 *
 * Exit codes: 0 = allow, 2 = block (reason on stderr, shown to the agent).
 */

'use strict';

const ALT =
  'Instead: start the job with run_in_background (you are notified when it exits), ' +
  'or `wait $PID` on a PID you started, or wrap a bounded poll in `timeout <secs>`.';

const RULES = [
  {
    test: c => /\b(while|until)\b[\s\S]*\b(pgrep|pidof)\b/.test(c) ||
      /\b(while|until)\b[\s\S]*\bps\b[^;|]*\|\s*grep\b/.test(c),
    msg: 'Blocked: process-polling wait loop. `pgrep -f`/`ps | grep` matches the loop\'s own command line, so it never exits.',
  },
  {
    test: c => /\bdo\s+(:|true)\s*;?\s*done\b/.test(c),
    msg: 'Blocked: busy loop with no sleep (pins a CPU core).',
  },
  {
    test: c => /\b(while|until)\b[\s\S]*\bsleep\b/.test(c) && !/\btimeout\s+\d/.test(c),
    msg: 'Blocked: unbounded while/until polling loop.',
  },
];

let data = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => { data += chunk; });
process.stdin.on('end', () => {
  let input = {};
  try { input = JSON.parse(data); } catch {}
  const raw = (input && input.tool_input && input.tool_input.command) || '';
  // Check only executable shell code: drop heredoc bodies and quoted strings
  // (commit messages, PR bodies, grep patterns) to avoid false positives.
  const cmd = raw
    .replace(/<<-?\s*['"]?(\w+)['"]?[^\n]*\n[\s\S]*?\n\s*\1\b/g, '')
    .replace(/'[^']*'/g, "''")
    .replace(/"(?:[^"\\]|\\.)*"/g, '""');

  for (const r of RULES) {
    if (r.test(cmd)) {
      process.stderr.write(r.msg + ' ' + ALT + '\n');
      process.exit(2);
    }
  }
  process.exit(0);
});
