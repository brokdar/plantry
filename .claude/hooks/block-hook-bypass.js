#!/usr/bin/env node
/**
 * PreToolUse hook (Bash): block ways to skip the secret-prevention git hooks.
 *
 * The prek pre-commit and pre-push hooks (gitleaks, blocked files, DB check)
 * are the local layers of "Secrets" in CLAUDE.md. This hook stops an agent from
 * stepping around them:
 *
 *  1. git commit/push/merge/... with --no-verify (or any abbreviation, --no-v...)
 *     or, for commit/push, -n (also inside a flag cluster such as -nm)
 *  2. SKIP= / PREK_SKIP= / PREK_ALLOW_NO_CONFIG= (and PRE_COMMIT_*), and
 *     GIT_DIR / GIT_WORK_TREE / GIT_CONFIG* / GIT_INDEX_FILE (HOME and
 *     XDG_CONFIG_HOME next to git), as prefix, `env`, `export` or `declare`
 *  3. git -c <anything but user.*-style keys>, --git-dir, --work-tree,
 *     core.hooksPath in any form, `git update-index --assume-unchanged|--skip-worktree`
 *  4. deleting, moving, chmodding or overwriting anything in .git/hooks from any
 *     cwd (tracks `cd`), `prek uninstall`, `prek install -f/-c`
 *  5. Bash writes (rm mv cp sed -i tee redirects ...) to the guard files listed
 *     in scripts/secret-guard-files.txt; Edit/Write tool changes are fine
 *  6. git add -f / --force
 *
 * It is a shell tokenizer, not a regex over text: quotes are removed, command
 * substitutions and `bash -c`/`sh -c`/`eval`/shell heredocs are scanned
 * recursively, and a command it cannot tokenize that mentions git, commit/push
 * and a bypass marker is blocked. A dynamic word ($x, $(...)) in a git commit
 * or push command line is blocked unless it is the argument of -m/-F: write the
 * branch name or flag literally. A bare `git` word anywhere in a command is
 * analyzed, so `echo git commit --no-verify` is blocked too (quote it).
 * Residual risk: a script file that runs the commands cannot be read; pre-push
 * and CI still scan every pushed commit.
 *
 * If a check fires on real content, fix the content (never allowlist a real
 * secret, rotate it). Exit codes: 0 = allow, 2 = block (reason on stderr).
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ALT =
  'Do not bypass the secret checks: fix what they flag. If a scan fired on a real ' +
  'secret, stop, remove it from the commit and rotate it (see "Secrets" in CLAUDE.md).';

const FALLBACK_GUARD = [
  'scripts/secret-guard.sh', 'scripts/secret-ci.sh', 'scripts/check-db-secrets.py',
  'scripts/secret-patterns.txt', 'scripts/binary-allowlist.txt', 'scripts/secret-guard-files.txt',
  'prek.toml', '.gitleaks.toml', '.gitleaksignore', '.github/workflows/secrets.yml',
  '.claude/hooks/block-hook-bypass.js', '.claude/settings.json',
];

function loadGuardFiles() {
  try {
    const f = path.join(__dirname, '..', '..', 'scripts', 'secret-guard-files.txt');
    const list = fs.readFileSync(f, 'utf8').split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'));
    if (list.length) return list;
  } catch {}
  return FALLBACK_GUARD;
}
const GUARD = loadGuardFiles();

const SHELLS = new Set(['bash', 'sh', 'zsh', 'dash', 'ksh', 'ash', 'fish']);
const INTERP_INLINE = { python: 'c', python3: 'c', node: 'e', nodejs: 'e', perl: 'e', ruby: 'e', php: 'r', deno: 'e', bun: 'e' };
// commands whose arguments are themselves commands (text)
const RUNNERS = new Set(['eval', 'su', 'ssh', 'xargs', 'sudo', 'env', 'time', 'nohup', 'watch', 'timeout', 'script',
  'alias', 'source', '.', 'command', 'exec', 'setsid', 'nice', 'flock', 'busybox', 'stdbuf', 'ionice', 'doas', 'parallel']);
const KEYWORDS = new Set(['if', 'then', 'else', 'elif', 'do', 'while', 'until', '!', '{', '}', 'time', 'coproc']);
const ENV_BLOCK = /^(SKIP|PREK_SKIP|PRE_COMMIT_SKIP|PREK_ALLOW_NO_CONFIG|PRE_COMMIT_ALLOW_NO_CONFIG|GIT_DIR|GIT_WORK_TREE|GIT_COMMON_DIR|GIT_INDEX_FILE|GIT_EXEC_PATH|GIT_TEMPLATE_DIR|GIT_CONFIG.*|GIT_NAMESPACE)$/;
const ENV_BLOCK_NEAR_GIT = /^(HOME|XDG_CONFIG_HOME)$/;
// commands that write/remove their path arguments
const DESTRUCT_ANY = new Set(['rm', 'mv', 'chmod', 'chown', 'chgrp', 'unlink', 'truncate', 'shred', 'rmdir', 'tee', 'touch', 'patch', 'sed', 'perl', 'dd', 'find']);
const DESTRUCT_DEST = new Set(['cp', 'install', 'rsync', 'ln', 'scp', 'curl', 'wget']);
const ANCESTOR_CMDS = new Set(['rm', 'mv', 'rmdir', 'shred', 'unlink']);

// ── tokenizer ───────────────────────────────────────────────────────────────
// returns { commands: [{words, redirects, heredocs}], subs: [string], ok }.
// A word is {t: text without quotes, dyn: contains an unresolved expansion,
// pre: literal text before the first expansion}.
function tokenize(src) {
  const text = src.replace(/\\\r?\n/g, '');
  const commands = [];
  const subs = [];
  let cmd = { words: [], redirects: [], heredocs: [] };
  let word = null;
  let ok = true;
  let i = 0;
  const pendingHere = [];

  const newWord = () => ({ t: '', dyn: false, pre: null });
  const addCh = ch => { if (!word) word = newWord(); word.t += ch; };
  const markDyn = () => { if (!word) word = newWord(); if (!word.dyn) { word.dyn = true; word.pre = word.t; } };
  const endWord = () => { if (word) { cmd.words.push(word); word = null; } };
  const endCmd = () => {
    endWord();
    if (cmd.words.length || cmd.redirects.length || cmd.heredocs.length || pendingHere.some(h => h.cmd === cmd)) commands.push(cmd);
    cmd = { words: [], redirects: [], heredocs: [] };
  };

  // position of the `)` closing a $( ... ) whose body starts at `from`; -1 if none
  function matchParen(from) {
    let depth = 1, j = from;
    while (j < text.length) {
      const c = text[j];
      if (c === '\\') { j += 2; continue; }
      if (c === "'") { const k = text.indexOf("'", j + 1); if (k < 0) return -1; j = k + 1; continue; }
      if (c === '"') { j++; while (j < text.length && text[j] !== '"') { if (text[j] === '\\') j++; j++; } j++; continue; }
      if (c === '<' && text[j + 1] === '<' && text[j + 2] !== '<') {
        // heredoc inside the substitution: skip its body so quotes in it do not matter
        let k = j + 2;
        if (text[k] === '-') k++;
        while (text[k] === ' ') k++;
        const d = readSimpleWord(text, k);
        let nl = text.indexOf('\n', d.end);
        if (nl < 0) return -1;
        let pos = nl + 1, found = false;
        while (pos <= text.length) {
          let e = text.indexOf('\n', pos); if (e < 0) e = text.length;
          if (text.slice(pos, e).trim() === d.t) { pos = e; found = true; break; }
          pos = e + 1;
          if (e >= text.length) break;
        }
        if (!found) return -1;
        j = pos; continue;
      }
      if (c === '(') depth++;
      else if (c === ')') { depth--; if (depth === 0) return j; }
      j++;
    }
    return -1;
  }
  function matchTick(from) {
    let j = from;
    while (j < text.length) { if (text[j] === '\\') { j += 2; continue; } if (text[j] === '`') return j; j++; }
    return -1;
  }
  const isExpansionStart = ch => /[A-Za-z_{@*#?0-9!-]/.test(ch || '');

  while (i < text.length) {
    const c = text[i];
    if (c === '\\') { if (i + 1 < text.length) addCh(text[i + 1]); i += 2; continue; }
    if (c === "'") {
      const k = text.indexOf("'", i + 1);
      if (k < 0) { ok = false; break; }
      if (!word) word = newWord();
      word.t += text.slice(i + 1, k); i = k + 1; continue;
    }
    if (c === '"') {
      if (!word) word = newWord();
      i++;
      let closed = false;
      while (i < text.length) {
        const d = text[i];
        if (d === '"') { closed = true; i++; break; }
        if (d === '\\' && i + 1 < text.length) { word.t += text[i + 1]; i += 2; continue; }
        if (d === '$' && text[i + 1] === '(') {
          const k = matchParen(i + 2); if (k < 0) { ok = false; i = text.length; break; }
          subs.push(text.slice(i + 2, k)); markDyn(); word.t += '$()'; i = k + 1; continue;
        }
        if (d === '`') {
          const k = matchTick(i + 1); if (k < 0) { ok = false; i = text.length; break; }
          subs.push(text.slice(i + 1, k)); markDyn(); word.t += '$()'; i = k + 1; continue;
        }
        if (d === '$' && isExpansionStart(text[i + 1])) markDyn();
        word.t += d; i++;
      }
      if (!closed) { ok = false; break; }
      continue;
    }
    if (c === '$' && text[i + 1] === '(') {
      const k = matchParen(i + 2); if (k < 0) { ok = false; break; }
      subs.push(text.slice(i + 2, k)); markDyn(); word.t += '$()'; i = k + 1; continue;
    }
    if (c === '`') {
      const k = matchTick(i + 1); if (k < 0) { ok = false; break; }
      subs.push(text.slice(i + 1, k)); markDyn(); word.t += '$()'; i = k + 1; continue;
    }
    if (c === '$' && isExpansionStart(text[i + 1])) { markDyn(); addCh(c); i++; continue; }
    if ((c === '<' || c === '>') && text[i + 1] === '(') {
      const k = matchParen(i + 2); if (k < 0) { ok = false; break; }
      subs.push(text.slice(i + 2, k)); i = k + 1; continue;
    }
    if (c === '#' && !word) { while (i < text.length && text[i] !== '\n') i++; continue; }
    if (c === '\n') {
      endCmd();
      i++;
      while (pendingHere.length) {
        const h = pendingHere.shift();
        const lines = [];
        while (i < text.length) {
          let e = text.indexOf('\n', i); if (e < 0) e = text.length;
          const line = text.slice(i, e);
          i = e + 1;
          if ((h.strip ? line.trim() : line) === h.delim) break;
          lines.push(line);
        }
        h.cmd.heredocs.push(lines.join('\n'));
      }
      continue;
    }
    if (c === ' ' || c === '\t' || c === '\r') { endWord(); i++; continue; }
    if (c === '&' && text[i + 1] === '>') { i++; continue; } // &> file: the > is read as a redirect
    if (c === ';' || c === '|' || c === '&' || c === '(' || c === ')') {
      endCmd();
      i++;
      while ((c === '&' || c === '|') && text[i] === c) i++;
      continue;
    }
    if (c === '<' || c === '>') {
      let op = c;
      let j = i + 1;
      if (text[j] === c) { op += c; j++; }
      if (op === '<<' && text[j] === '<') { op = '<<<'; j++; }
      if (word && /^\d+$/.test(word.t)) word = null; // fd number
      if (op === '<<' && text[j] === '-') { op += '-'; j++; }
      if (text[j] === '&') { j++; while (j < text.length && /[\d-]/.test(text[j])) j++; i = j; continue; }
      if (text[j] === '|') j++;
      while (text[j] === ' ' || text[j] === '\t') j++;
      const sub = readSimpleWord(text, j);
      i = sub.end;
      if (op === '<<' || op === '<<-') pendingHere.push({ delim: sub.t, strip: op === '<<-', cmd });
      else if (op === '<<<') cmd.heredocs.push(sub.t);
      else if (op !== '<') cmd.redirects.push({ t: sub.t, dyn: sub.dyn });
      continue;
    }
    addCh(c); i++;
  }
  if (!ok) return { commands, subs, ok: false };
  endCmd();
  return { commands, subs, ok };
}

function readSimpleWord(text, j) {
  let t = '', dyn = false;
  while (j < text.length) {
    const c = text[j];
    if (/[\s;|&<>()]/.test(c)) break;
    if (c === '\\') { t += text[j + 1] || ''; j += 2; continue; }
    if (c === "'") { const k = text.indexOf("'", j + 1); if (k < 0) { j = text.length; break; } t += text.slice(j + 1, k); j = k + 1; continue; }
    if (c === '"') {
      j++;
      while (j < text.length && text[j] !== '"') { if (text[j] === '\\') j++; if (text[j] === '$') dyn = true; t += text[j]; j++; }
      j++; continue;
    }
    if (c === '$' || c === '`') dyn = true;
    t += c; j++;
  }
  return { t, dyn, end: j };
}

// ── helpers ─────────────────────────────────────────────────────────────────
const base = w => path.posix.basename(w);

function resolveP(cwd, p) {
  if (cwd === null && !p.startsWith('/') && !p.startsWith('~')) return null;
  const q = p.startsWith('/') || p.startsWith('~') ? p : path.posix.join(cwd, p);
  return path.posix.normalize(q);
}

function globRe(p) {
  const esc = p.replace(/[.+^${}()|\\]/g, '\\$&').replace(/\*/g, '[^/]*').replace(/\?/g, '[^/]');
  try { return new RegExp('(^|/)' + esc + '$'); } catch { return null; }
}

// Does resolved path q (null = unknown cwd) or glob rawWord touch a guard file
// or .git/hooks? Returns a reason or null. `ancestors`: also a directory that
// holds guard files (for rm/mv).
function touchesProtected(q, rawWord, { ancestors } = {}) {
  const hasGlob = /[*?[]/.test(rawWord);
  const segs = (q || rawWord).split('/').filter(Boolean);
  for (let k = 0; k + 1 < segs.length; k++) if (segs[k] === '.git' && segs[k + 1] === 'hooks') return 'git hooks';
  if (/(^|\/)\.git\/hooks(\/|$)/.test(rawWord)) return 'git hooks';
  if (ancestors && q && /(^|\/)\.git$/.test(q)) return 'the .git directory';
  const re = hasGlob && q ? globRe(q) : null;
  for (const g of GUARD) {
    if (q === null) {
      if (base(rawWord) === base(g) && !rawWord.includes('$')) return 'guard file ' + g;
      continue;
    }
    if (q === g || q.endsWith('/' + g)) return 'guard file ' + g;
    if (re && re.test(g)) return 'guard file ' + g;
    if (ancestors) {
      if (q === '.' || q === '..' || q === '/' || q === '~') return 'a directory holding guard files';
      for (let d = path.posix.dirname(g); d && d !== '.'; d = path.posix.dirname(d)) {
        if (q === d || q.endsWith('/' + d) || (re && re.test(d))) return 'directory of guard file ' + g;
      }
    }
  }
  if (re && (re.test('.git/hooks') || re.test('.git/hooks/pre-commit') || re.test('.git/hooks/pre-push'))) return 'git hooks';
  return null;
}

function mentionsProtected(code) {
  if (/\.git['"\s,\/\\+)\]\[]*hooks/.test(code)) return 'git hooks';
  for (const g of GUARD) {
    const b = base(g);
    if (code.includes(b) && (b !== 'settings.json' || code.includes('.claude'))) return 'guard file ' + g;
  }
  return null;
}

function patchTouchesGuard(args) {
  for (const a of args) {
    if (a.t.startsWith('-')) continue;
    try {
      const st = fs.statSync(a.t);
      if (!st.isFile() || st.size > 2 * 1024 * 1024) continue;
      const body = fs.readFileSync(a.t, 'utf8');
      for (const m of body.matchAll(/^(?:\+\+\+|---|diff --git)\s+(?:a\/|b\/)?(\S+)/gm)) {
        if (touchesProtected(path.posix.normalize(m[1]), m[1])) return true;
      }
    } catch {}
  }
  return false;
}

// ── analysis ────────────────────────────────────────────────────────────────
const VERIFY_SUBS = new Set(['commit', 'push', 'merge', 'am', 'cherry-pick', 'revert', 'rebase']);
// short options that take an argument, per subcommand
const SHORT_ARG = { commit: 'mFCct', push: 'o', merge: 'mFsX', rebase: 'sxXCS', 'cherry-pick': 'mXs', revert: 'mXs', am: 'p' };
const LONG_ARG = /^--(message|file|reuse-message|reedit-message|template|author|date|fixup|squash|cleanup|trailer|exec|onto|strategy|strategy-option|receive-pack|repo|push-option|mainline)$/;

function gitAnalyze(words, ctx) {
  let i = 0;
  while (i < words.length) {
    const w = words[i];
    if (w.dyn) return 'a dynamic word before the git subcommand cannot be checked';
    const t = w.t;
    if (t === '-C') { i += 2; continue; }
    if (t === '-c') {
      const kv = words[i + 1];
      if (kv && kv.dyn) return 'git -c with a dynamic value';
      const key = ((kv || {}).t || '').split('=')[0];
      if (!/^(user\.(name|email|signingkey)|color\.[a-z.]+|advice\.[a-z.]+|core\.pager|pager\.[a-z]+|init\.defaultbranch|commit\.gpgsign|http\.[a-z.]+)$/i.test(key)) return `git -c ${key} can redirect or disable hooks`;
      i += 2; continue;
    }
    if (/^(--git-dir|--work-tree|--namespace|--super-prefix|--config-env|--exec-path)(=|$)/.test(t)) return `git ${t.split('=')[0]} points git at another repository or config`;
    if (t.startsWith('-')) { i++; continue; }
    break;
  }
  if (i >= words.length) return null;
  const sub = words[i].t;
  const args = words.slice(i + 1);

  if (args.some(a => /core\.hookspath/i.test(a.t))) return 'core.hooksPath redirects the git hooks away from prek';
  if (sub === 'config') {
    if (args.some(a => /^(include|includeif)\b/i.test(a.t))) return 'git config include can redirect the hooks';
    for (const a of args) {
      if (/\s/.test(a.t)) { const r = decideText(a.t.replace(/^!/, ''), ctx, 1); if (r) return r; }
    }
  }
  if (sub === 'update-index' && args.some(a => /^--(assume-unchanged|skip-worktree)$/.test(a.t))) {
    return 'git update-index --assume-unchanged/--skip-worktree hides changes from the scans';
  }
  if (sub === 'add') {
    for (const a of args) {
      if (a.t === '--') break;
      if (a.t === '--force' || (/^-[a-zA-Z]+$/.test(a.t) && a.t.includes('f'))) return 'git add -f/--force would add an ignored file (.env, secret.key, DBs ...) past .gitignore';
    }
  }
  if (['rm', 'mv', 'restore', 'checkout'].includes(sub)) {
    for (const a of args) {
      if (a.t.startsWith('-')) continue;
      const q = resolveP(ctx.cwd, a.t);
      const r = touchesProtected(q, a.t, { ancestors: sub === 'rm' || sub === 'mv' });
      if (r && r !== 'git hooks') return `git ${sub} would change ${r}`;
    }
  }
  if ((sub === 'apply' || sub === 'am') && patchTouchesGuard(args)) return `git ${sub} patch changes a guard file`;
  if (!VERIFY_SUBS.has(sub)) return null;

  const shortArg = SHORT_ARG[sub] || '';
  for (let k = 0; k < args.length; k++) {
    const a = args[k];
    if (a.t === '--') break;
    if (a.dyn) {
      const pre = a.pre === null ? '' : a.pre;
      if (pre.startsWith('--') && pre.includes('=') && LONG_ARG.test(pre.split('=')[0])) continue;
      if (pre === '' || pre.startsWith('-')) return `a dynamic word (${a.t.replace(/\$\(\)/g, '$(...)')}) in git ${sub} could be --no-verify; write the flag or value literally`;
      continue;
    }
    const t = a.t;
    if (t.startsWith('--')) {
      const name = t.split('=')[0];
      if (/^--no-v/.test(name)) return `git ${sub} ${name} skips the secret-scan hooks`;
      if (LONG_ARG.test(name) && !t.includes('=')) k++;
      continue;
    }
    if (t.startsWith('-') && t.length > 1) {
      for (let c = 1; c < t.length; c++) {
        const ch = t[c];
        if (ch === 'n' && (sub === 'commit' || sub === 'push')) return `git ${sub} -n skips the secret-scan hooks`;
        if (shortArg.includes(ch)) { if (c === t.length - 1) k++; break; }
      }
    }
  }
  return null;
}

function analyzeCommand(cmd, ctx, depth) {
  const words = cmd.words;
  let i = 0;
  while (i < words.length && KEYWORDS.has(words[i].t)) i++;
  const assignNames = [];
  const gitNear = words.some(w => base(w.t) === 'git' || base(w.t) === 'prek' || w.t === 'pre-commit');
  while (i < words.length && /^[A-Za-z_][A-Za-z0-9_]*=/.test(words[i].t)) { assignNames.push(words[i].t.split('=')[0]); i++; }
  const first = i < words.length ? base(words[i].t) : '';
  if (['export', 'declare', 'typeset', 'readonly', 'local', 'env'].includes(first)) {
    for (let k = i + 1; k < words.length; k++) {
      const w = words[k].t;
      if (/^[A-Za-z_][A-Za-z0-9_]*(=|$)/.test(w)) assignNames.push(w.split('=')[0]);
      else if (first === 'env' && w === '-u') k++;
    }
  }
  for (const n of assignNames) {
    if (ENV_BLOCK.test(n)) return `${n}= disables hooks or redirects git away from this repository`;
    if (ENV_BLOCK_NEAR_GIT.test(n) && gitNear) return `${n}= next to git can load another git config (hooksPath)`;
  }

  // cd tracking
  if (words[i] && (words[i].t === 'cd' || words[i].t === 'pushd')) {
    const target = words.slice(i + 1).find(w => !w.t.startsWith('-') || w.t === '-');
    if (!target) ctx.cwd = '~';
    else if (target.dyn || target.t === '-') ctx.cwd = null;
    else ctx.cwd = resolveP(ctx.cwd, target.t);
    return null;
  }
  if (words[i] && words[i].t === 'popd') { ctx.cwd = null; return null; }

  for (const r of cmd.redirects) {
    if (r.t === '/dev/null' || r.t.startsWith('/dev/std') || r.t === '/dev/fd') continue;
    const p = touchesProtected(resolveP(ctx.cwd, r.t), r.t);
    if (p) return `a redirect writes into ${p}`;
  }
  if (i >= words.length) return null;

  // any bare `git` word anywhere (covers sudo/xargs/timeout/nohup wrappers)
  for (let k = i; k < words.length; k++) {
    if (base(words[k].t) === 'git' && !words[k].dyn) {
      const r = gitAnalyze(words.slice(k + 1), ctx);
      if (r) return r;
    }
  }
  if (words[i].dyn && words.some(w => /^(commit|push)$/.test(w.t))) return 'a dynamic command word next to commit/push cannot be checked';

  const name = first;
  if (name === 'prek' || name === 'pre-commit') {
    const rest = words.slice(i + 1).map(w => w.t);
    if (rest[0] === 'uninstall') return 'prek uninstall removes the secret-scan hooks';
    if (rest[0] === 'install' && rest.some(a => /^(-f|--overwrite|-c|--config(=.*)?|--allow-missing-config)$/.test(a))) return 'prek install with -f/-c can replace the hooks or point them at another config';
  }

  // shells anywhere in the command (covers `sudo bash -c ...`)
  for (let k = i; k < words.length; k++) {
    if (SHELLS.has(base(words[k].t))) {
      for (let m = k + 1; m < words.length; m++) {
        if (/^-[a-zA-Z]*c[a-zA-Z]*$/.test(words[m].t) && words[m + 1]) {
          const r = decideText(words[m + 1].t, ctx, depth + 1);
          if (r) return r;
        }
      }
      for (const h of cmd.heredocs) { const r = decideText(h, ctx, depth + 1); if (r) return r; }
    }
  }
  if (words.slice(i).some(w => w.t === 'eval' || w.t === 'source' || w.t === '.')) {
    const at = words.findIndex((w, k) => k >= i && (w.t === 'eval' || w.t === 'source' || w.t === '.'));
    const r = decideText(words.slice(at + 1).map(w => w.t).join(' '), ctx, depth + 1);
    if (r) return r;
    for (const h of cmd.heredocs) { const r2 = decideText(h, ctx, depth + 1); if (r2) return r2; }
  }
  if (words.slice(i).some(w => RUNNERS.has(base(w.t)))) {
    for (let k = i + 1; k < words.length; k++) {
      if (/\s/.test(words[k].t)) {
        const r = decideText(words[k].t.replace(/^[A-Za-z_][A-Za-z0-9_]*=/, ''), ctx, depth + 1);
        if (r) return r;
      }
    }
    if (name === 'xargs' || name === 'parallel') for (const h of cmd.heredocs) { const r = decideText(h, ctx, depth + 1); if (r) return r; }
  }

  // inline interpreter code and interpreter heredocs
  const interp = INTERP_INLINE[name] || INTERP_INLINE[name.replace(/[0-9.]+$/, '')];
  if (interp) {
    const rest = words.slice(i + 1);
    for (let k = 0; k < rest.length; k++) {
      const t = rest[k].t;
      if ((t.startsWith('-') && !t.startsWith('--') && t.includes(interp)) || t === '--eval' || t === '--print') {
        const r = mentionsProtected(rest.slice(k + 1).map(w => w.t).join(' '));
        if (r) return `inline ${name} code touches ${r}`;
      }
    }
    for (const h of cmd.heredocs) { const r = mentionsProtected(h); if (r) return `a ${name} script touches ${r}`; }
  }

  // writes to .git/hooks and guard files
  if (DESTRUCT_ANY.has(name) || DESTRUCT_DEST.has(name)) {
    const rest = words.slice(i + 1);
    let args = rest.filter(w => !w.t.startsWith('-') || w.t === '-');
    if (name === 'sed' || name === 'perl') {
      if (!rest.some(w => /^-[a-zA-Z]*i/.test(w.t) || /^--in-place/.test(w.t))) args = [];
    } else if (name === 'find') {
      args = rest.some(w => /^-(delete|exec|execdir|ok|okdir|fprint.*)$/.test(w.t)) ? args.filter(w => !/^[{}+;]$/.test(w.t)) : [];
    } else if (name === 'dd') {
      args = rest.filter(w => /^of=/.test(w.t)).map(w => ({ t: w.t.slice(3), dyn: w.dyn }));
    } else if (name === 'curl' || name === 'wget') {
      const o = rest.findIndex(w => w.t === '-o' || w.t === '--output' || w.t === '-O');
      args = name === 'curl' && o >= 0 && rest[o + 1] ? [rest[o + 1]] : (name === 'wget' && o >= 0 && rest[o + 1] ? [rest[o + 1]] : []);
    } else if (DESTRUCT_DEST.has(name)) {
      const dst = [];
      if (args.length) dst.push(args[args.length - 1]);
      const t = rest.findIndex(w => w.t === '-t');
      if (t >= 0 && rest[t + 1]) dst.push(rest[t + 1]);
      args = dst;
    } else if (name === 'patch' && patchTouchesGuard(args)) {
      return 'patch changes a guard file';
    }
    for (const a of args) {
      const r = touchesProtected(resolveP(ctx.cwd, a.t), a.t, { ancestors: ANCESTOR_CMDS.has(name) });
      if (r) return `${name} would change ${r}`;
      if (a.dyn && /hooks/.test(a.t)) return `${name} on a dynamic path that mentions hooks`;
      if (ctx.cwd === null && /(^|\/)hooks(\/|$)/.test(a.t)) return `${name} on hooks/ from an unknown directory`;
    }
  }
  return null;
}

function decideText(src, ctx, depth) {
  if (depth > 8) return 'command nesting too deep to check';
  const tk = tokenize(src);
  for (const c of tk.commands) {
    const r = analyzeCommand(c, ctx, depth);
    if (r) return r;
  }
  for (const s of tk.subs) {
    const r = decideText(s, ctx, depth + 1);
    if (r) return r;
  }
  if (!tk.ok && /\bgit\b/.test(src) && /\b(commit|push)\b/.test(src) && /(--no-v|(^|\s)-[a-z]*n\b|SKIP|hooksPath|\$\(|`)/i.test(src)) {
    return 'a command that mentions git commit/push and a bypass marker could not be parsed';
  }
  return null;
}

function decide(raw) {
  const reason = decideText(raw, { cwd: '.' }, 0);
  return reason ? 'Blocked: ' + reason + '. ' + ALT : null;
}

module.exports = { decide, tokenize };

if (require.main === module) {
  let data = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => {
    let input = {};
    try { input = JSON.parse(data); } catch {}
    const raw = (input && input.tool_input && input.tool_input.command) || '';
    let reason;
    try {
      reason = decide(raw);
    } catch (e) {
      reason = /\bgit\b/.test(raw) ? 'Blocked: the bypass check failed on this command (' + e.message + '). ' + ALT : null;
    }
    if (reason) {
      process.stderr.write(reason + '\n');
      process.exit(2);
    }
    process.exit(0);
  });
}
