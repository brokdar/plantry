#!/usr/bin/env node
/**
 * PreToolUse hook (Bash): block ways to skip the secret-prevention git hooks.
 *
 * The prek pre-commit and pre-push hooks (gitleaks, blocked files, fixture DB
 * check) are the local layers of "Secrets" in CLAUDE.md. This hook stops an
 * agent from stepping around them:
 *
 *  1. `git commit` / `git push` with --no-verify or -n
 *  2. SKIP= / PREK_SKIP= / PREK_ALLOW_NO_CONFIG= (and PRE_COMMIT_*) env prefixes
 *  3. git -c core.hooksPath=..., git config core.hooksPath
 *  4. deleting, moving, chmodding or overwriting anything in .git/hooks, or
 *     `prek uninstall`
 *  5. git add -f / --force (forcing an ignored file past .gitignore)
 *
 * If a check fires on real content, fix the content (never allowlist a real
 * secret, rotate it). Exit codes: 0 = allow, 2 = block (reason on stderr).
 */

'use strict';

const ALT =
  'Do not bypass the secret checks: fix what they flag. If a scan fired on a real ' +
  'secret, stop, remove it from the commit and rotate it (see "Secrets" in CLAUDE.md).';

const SEG = '[^;&|\\n]*';

// Executable shell text with heredoc bodies and quoted strings blanked, so a
// commit message that mentions --no-verify is not a flag.
function blankQuoted(raw) {
  return raw
    .replace(/<<-?\s*['"]?(\w+)['"]?[^\n]*\n[\s\S]*?\n\s*\1\b/g, '')
    .replace(/'[^']*'/g, "''")
    .replace(/"(?:[^"\\]|\\.)*"/g, '""');
}

// Same, but quotes only removed: `git -c "core.hooksPath=x"` stays visible.
function unquote(raw) {
  return raw
    .replace(/<<-?\s*['"]?(\w+)['"]?[^\n]*\n[\s\S]*?\n\s*\1\b/g, '')
    .replace(/['"]/g, '');
}

const GIT_SUB = sub => new RegExp(`\\bgit\\b${SEG}\\b${sub}\\b(${SEG})`, 'g');

// Does any `git <sub>` segment satisfy pred(args)?
function gitSegment(cmd, sub, pred) {
  for (const m of cmd.matchAll(GIT_SUB(sub))) {
    if (pred(m[1] || '')) return true;
  }
  return false;
}

const RULES = [
  {
    quoted: 'blank',
    test: c =>
      gitSegment(c, 'commit', a => /(^|\s)(--no-verify\b|-[a-zA-Z]*n[a-zA-Z]*(\s|$))/.test(a)) ||
      gitSegment(c, 'push', a => /(^|\s)(--no-verify\b|-[a-zA-Z]*n[a-zA-Z]*(\s|$))/.test(a)),
    msg: 'Blocked: git commit/push with --no-verify or -n skips the secret-scan hooks.',
  },
  {
    quoted: 'strip',
    // an env assignment at the start of a command (after env/export/other VAR=x),
    // not a word inside a message
    test: c => /(^|[;&|(\n])\s*((export|env)\s+)?([A-Za-z_][A-Za-z0-9_]*=\S*\s+)*(SKIP|PREK_SKIP|PRE_COMMIT_SKIP|PREK_ALLOW_NO_CONFIG|PRE_COMMIT_ALLOW_NO_CONFIG)=/.test(c),
    msg: 'Blocked: SKIP=/PREK_SKIP=/PREK_ALLOW_NO_CONFIG= disables hooks.',
  },
  {
    quoted: 'strip',
    test: c => /core\.hookspath/i.test(c) && /\bgit\b/.test(c),
    msg: 'Blocked: changing core.hooksPath redirects the git hooks away from prek.',
  },
  {
    quoted: 'strip',
    test: c =>
      new RegExp(
        `\\b(rm|mv|chmod|chown|unlink|truncate|ln|shred|rmdir|cp|install|tee|sed|perl|dd|find|rsync)\\b${SEG}(\\.git/hooks|--git-path\\s+hooks|git-common-dir\\S*/hooks)`
      ).test(c) ||
      />>?\s*\S*\.git\/hooks/.test(c) ||
      /\b(prek|pre-commit)\s+uninstall\b/.test(c),
    msg: 'Blocked: do not delete, chmod or rewrite .git/hooks, or uninstall prek.',
  },
  {
    quoted: 'blank',
    test: c =>
      gitSegment(c, 'add', a => /(^|\s)(--force\b|-[a-zA-Z]*f[a-zA-Z]*(\s|$))/.test(a)),
    msg: 'Blocked: git add -f/--force would add an ignored file (.env, secret.key, DBs ...) past .gitignore.',
  },
];

function decide(raw) {
  const blank = blankQuoted(raw);
  const strip = unquote(raw);
  for (const r of RULES) {
    if (r.test(r.quoted === 'blank' ? blank : strip)) return r.msg + ' ' + ALT;
  }
  return null;
}

module.exports = { decide };

if (require.main === module) {
  let data = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    let input = {};
    try { input = JSON.parse(data); } catch {}
    const raw = (input && input.tool_input && input.tool_input.command) || '';
    const reason = decide(raw);
    if (reason) {
      process.stderr.write(reason + '\n');
      process.exit(2);
    }
    process.exit(0);
  });
}
