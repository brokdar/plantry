'use strict';
// Run: node --test .claude/hooks/block-hook-bypass.test.js
const test = require('node:test');
const assert = require('node:assert');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const { decide } = require('./block-hook-bypass.js');

const BLOCK = [
  'git commit --no-verify -m "x"',
  'git commit -n -m x',
  'git commit -nm x',
  'git commit -m x --no-verify',
  'git -C /tmp/r commit --no-verify -m x',
  'git push --no-verify origin main',
  'git push -n',
  'git add . && git commit --no-verify -m x',
  'SKIP=gitleaks git commit -m x',
  'PREK_SKIP=gitleaks git commit -m x',
  'export PREK_ALLOW_NO_CONFIG=1; git commit -m x',
  'env SKIP="gitleaks" git commit -m x',
  'git -c core.hooksPath=/dev/null commit -m x',
  'git -c "core.hooksPath=/dev/null" commit -m x',
  'git config core.hooksPath /tmp/none',
  'rm .git/hooks/pre-commit',
  'rm -f /repo/.git/hooks/pre-push',
  'chmod -x .git/hooks/pre-commit',
  'mv .git/hooks/pre-push /tmp/x',
  'echo exit 0 > .git/hooks/pre-commit',
  'prek uninstall',
  'git add -f .env',
  'git add --force backend/secret.key',
  'git add -fA',
];

const ALLOW = [
  'git commit -m "document that --no-verify is forbidden"',
  "git commit -m 'never SKIP=gitleaks'",
  'git commit -m "$(cat <<\'EOF\'\nuse --no-verify never\nEOF\n)"',
  'git commit -m x',
  'git commit --amend --no-edit',
  'git push origin fix/fixture-secrets',
  'git push --force-with-lease origin x',
  'git add -A',
  'git add backend/foo.go',
  'git status',
  'ls .git/hooks',
  'cat .git/hooks/pre-commit',
  'prek install --hook-type pre-commit --hook-type pre-push',
  'git config user.name x',
  'git diff --no-index a b',
];

for (const c of BLOCK) {
  test(`blocks: ${c.split('\n')[0]}`, () => assert.ok(decide(c), c));
}
for (const c of ALLOW) {
  test(`allows: ${c.split('\n')[0]}`, () => assert.strictEqual(decide(c), null, c));
}

test('hook process exits 2 with a reason on git commit --no-verify', () => {
  const r = spawnSync('node', [path.join(__dirname, 'block-hook-bypass.js')], {
    input: JSON.stringify({ tool_input: { command: 'git commit --no-verify -m "x"' } }),
    encoding: 'utf8',
  });
  assert.strictEqual(r.status, 2);
  assert.match(r.stderr, /--no-verify/);
});

test('hook process exits 0 on a normal commit', () => {
  const r = spawnSync('node', [path.join(__dirname, 'block-hook-bypass.js')], {
    input: JSON.stringify({ tool_input: { command: 'git commit -m "x"' } }),
    encoding: 'utf8',
  });
  assert.strictEqual(r.status, 0);
});
