#!/usr/bin/env node
/**
 * Config Protection Hook
 *
 * Blocks modifications to linter/formatter config files.
 * Agents frequently modify these to make checks pass instead of fixing
 * the actual code. This hook steers the agent back to fixing the source.
 *
 * Exit codes:
 *   0 = allow (not a config file, or first-time creation of one)
 *   2 = block (existing config file modification attempted)
 */

'use strict';

const fs = require('fs');
const path = require('path');

const MAX_STDIN = 1024 * 1024;

const PROTECTED_FILES = new Set([
  // ESLint (legacy + v9 flat config, JS/TS/MJS/CJS)
  '.eslintrc',
  '.eslintrc.js',
  '.eslintrc.cjs',
  '.eslintrc.json',
  '.eslintrc.yml',
  '.eslintrc.yaml',
  'eslint.config.js',
  'eslint.config.mjs',
  'eslint.config.cjs',
  'eslint.config.ts',
  'eslint.config.mts',
  'eslint.config.cts',
  // Prettier (all config variants including ESM)
  '.prettierrc',
  '.prettierrc.js',
  '.prettierrc.cjs',
  '.prettierrc.json',
  '.prettierrc.yml',
  '.prettierrc.yaml',
  'prettier.config.js',
  'prettier.config.cjs',
  'prettier.config.mjs',
  // Biome
  'biome.json',
  'biome.jsonc',
  // Ruff (Python)
  '.ruff.toml',
  'ruff.toml',
  // Note: pyproject.toml is intentionally NOT included here because it
  // contains project metadata alongside linter config. Blocking all edits
  // to pyproject.toml would prevent legitimate dependency changes.
  // Shell / Style / Markdown
  '.shellcheckrc',
  '.stylelintrc',
  '.stylelintrc.json',
  '.stylelintrc.yml',
  '.markdownlint.json',
  '.markdownlint.yaml',
  '.markdownlintrc'
]);

function parseInput(raw) {
  try {
    return raw.trim() ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function run(raw, truncated) {
  if (truncated) {
    process.stderr.write(
      `BLOCKED: Hook input exceeded ${MAX_STDIN} bytes. ` +
      'Refusing to bypass config-protection on a truncated payload. ' +
      'Retry with a smaller edit or disable the config-protection hook temporarily.\n'
    );
    process.exit(2);
  }

  const input = parseInput(raw);
  const filePath = input?.tool_input?.file_path || input?.tool_input?.file || '';
  if (!filePath) process.exit(0);

  const basename = path.basename(filePath);
  if (!PROTECTED_FILES.has(basename)) process.exit(0);

  // Allow first-time creation — there's no existing config to weaken.
  // The hook's purpose is blocking modifications; writing a brand-new
  // config file in a project that has none is a legitimate bootstrap
  // path (e.g. scaffolding ESLint into a fresh repo).
  //
  // Fail closed on any stat error other than ENOENT. Use lstatSync so a
  // symlink at the protected path is treated as present even if its target
  // is missing — a dangling symlink at e.g. .eslintrc.js still represents
  // an existing config entry that an agent should not silently replace.
  // fs.existsSync would swallow EACCES/EPERM as false; lstatSync exposes
  // the error code so we can treat only genuine "path not found" (ENOENT)
  // as absent.
  let exists = true;
  try {
    fs.lstatSync(filePath);
    // lstat succeeded — something (file, dir, or symlink) exists here.
  } catch (err) {
    if (err && err.code === 'ENOENT') {
      exists = false;
    }
    // Any other error (EACCES, EPERM, ELOOP, etc.) leaves exists=true
    // so the guard is never silently weakened.
  }

  if (!exists) process.exit(0);

  process.stderr.write(
    `BLOCKED: Modifying ${basename} is not allowed. ` +
    'Fix the source code to satisfy linter/formatter rules instead of ' +
    'weakening the config. If this is a legitimate config change, ' +
    'disable the config-protection hook temporarily.\n'
  );
  process.exit(2);
}

// Read stdin, then decide
let raw = '';
let truncated = false;

process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => {
  if (raw.length < MAX_STDIN) {
    const remaining = MAX_STDIN - raw.length;
    raw += chunk.substring(0, remaining);
    if (chunk.length > remaining) truncated = true;
  } else {
    truncated = true;
  }
});

process.stdin.on('end', () => {
  run(raw, truncated);
});
