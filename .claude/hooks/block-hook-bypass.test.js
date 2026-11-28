'use strict';
// Run: node --test .claude/hooks/block-hook-bypass.test.js
const test = require('node:test');
const assert = require('node:assert');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const { decide } = require('./block-hook-bypass.js');

const BLOCK = [
  // plain bypasses
  'git commit --no-verify -m "x"',
  'git commit -n -m x',
  'git commit -nm x',
  'git commit -anm x',
  'git commit -m x --no-verify',
  'git -C /tmp/r commit --no-verify -m x',
  'git push --no-verify origin main',
  'git push -n',
  'git merge --no-verify topic',
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
  // review finding 5: quoted text, shells, eval
  "bash -c 'git commit --no-verify -m x'",
  'sh -c "git commit --no-verify -m x"',
  "eval 'git commit --no-verify -m x'",
  "sudo bash -lc 'git push --no-verify'",
  'bash <<EOF\ngit commit --no-verify -m x\nEOF',
  "bash <<< 'git commit --no-verify -m x'",
  "ssh host 'git commit --no-verify -m x'",
  'echo "$(git commit --no-verify -m x)"',
  'x=$(git commit --no-verify -m x)',
  '`git commit --no-verify -m x`',
  // abbreviated long options
  'git commit --no-verif -m x',
  'git commit --no-v -m x',
  'git push --no-verif',
  // variable / substitution indirection, continuations, alias
  'x=--no-verify; git commit $x',
  'git commit "$x" -m y',
  'git commit --no-$(echo verify) -m x',
  'git commit --no-ver""ify -m x',
  'git commit --no-\\\nverify -m x',
  "alias gc='git commit --no-verify'; gc",
  '$g commit -m x',
  // hook removal from any cwd, scripting languages
  'cd .git/hooks && rm pre-push',
  'cd .git && rm hooks/pre-push',
  '(cd .git/hooks; rm -f pre-commit)',
  'cd .git; cd hooks; truncate -s0 pre-push',
  'cd /tmp && cd - && rm hooks/pre-push',
  'rm -f "$(git rev-parse --git-common-dir)/hooks/pre-push"',
  'rm .git/hooks/pre-*',
  "python3 -c \"import os; os.remove('.git/hooks/pre-push')\"",
  "python3 -c \"import os; os.remove(os.path.join('.git','hooks','pre-push'))\"",
  "node -e \"require('fs').unlinkSync('.git/hooks/pre-push')\"",
  "python3 - <<'PY'\nimport os\nos.remove('.git/hooks/pre-push')\nPY",
  'find .git/hooks -type f -delete',
  'rm -rf .git',
  // guard files cannot be written from Bash
  "sed -i 's/fail=1/fail=0/' scripts/secret-guard.sh",
  "sed -i.bak 's/x/y/' prek.toml",
  'sed -ni p .gitleaks.toml',
  'echo > scripts/secret-guard.sh',
  'echo "paths = [\'.*\']" >> .gitleaks.toml',
  'printf x | tee .gitleaksignore',
  'tee -a scripts/secret-patterns.txt < /dev/null',
  'mv /tmp/x scripts/secret-guard.sh',
  'cp /tmp/weak.toml .gitleaks.toml',
  'cp /tmp/weak.yml .github/workflows/secrets.yml',
  'rm scripts/secret-guard.sh',
  'rm -rf scripts',
  'rm -rf .github',
  'rm .claude/hooks/block-hook-bypass.js',
  'cd scripts && sed -i s/a/b/ secret-guard.sh',
  'cd scripts && echo x > secret-patterns.txt',
  'perl -pi -e s/a/b/ .gitleaksignore',
  'dd if=/dev/null of=prek.toml',
  'install -m 755 /tmp/x scripts/secret-guard.sh',
  'git rm scripts/secret-guard.sh',
  'git checkout HEAD~3 -- scripts/secret-guard.sh',
  "python3 -c \"open('.gitleaks.toml','w').write('')\"",
  // other bypasses from the review
  'git update-index --assume-unchanged scripts/x',
  'git update-index --skip-worktree .env',
  'prek install --hook-type pre-push -f',
  'prek install -c /tmp/weak.toml',
  'GIT_DIR=/tmp/other git commit -m x',
  'git --git-dir=/tmp/other commit -m x',
  'git --git-dir /tmp/other commit -m x',
  'env -i SKIP=x git commit -m x',
  'GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=/x git commit -m x',
  'git -c alias.ci="!git commit --no-verify" ci',
  'HOME=/tmp/h git commit -m x',
  'export GIT_DIR=/tmp/other',
  // fail closed on unparseable input that mentions a bypass
  'git commit -m "unterminated --no-verify',
];

const ALLOW = [
  'git commit -m "document that --no-verify is forbidden"',
  "git commit -m 'never SKIP=gitleaks'",
  'git commit -m "$(cat <<\'EOF\'\nuse --no-verify never\nEOF\n)"',
  'git commit -m "$(cat <<\'EOF\'\ndon\'t use --no-verify, it\'s (bad)\nEOF\n)"',
  'git commit -m "fix: x" -m "second paragraph -n"',
  'git commit -am "a message"',
  'git commit --message="x"',
  'git commit -m "$msg"',
  'git commit -F -',
  'git commit -m x',
  'git commit --amend --no-edit',
  'git commit -S -m x',
  'git push origin fix/fixture-secrets',
  'git push -u origin fix/fixture-secrets',
  'git push --force-with-lease origin x',
  'git push --force-with-lease=x origin x',
  'git add -A',
  'git add backend/foo.go',
  'git status',
  'git log --oneline -5',
  'git diff --no-index a b',
  'git checkout -b feature/x',
  'git checkout main -- backend/foo.go',
  'git rm --cached backend/foo.go',
  'git config user.name x',
  'git -c user.name=x -c user.email=y commit -m x',
  'ls .git/hooks',
  'cat .git/hooks/pre-commit',
  'cat scripts/secret-guard.sh',
  'sed -n 1,20p scripts/secret-guard.sh',
  'grep -n fail scripts/secret-guard.sh',
  'cp scripts/secret-guard.sh /tmp/copy.sh',
  'diff prek.toml /tmp/other.toml',
  'node --test .claude/hooks/block-hook-bypass.test.js',
  'python3 -I scripts/check-db-secrets.py --tracked',
  'scripts/secret-guard.sh range origin/main HEAD > /tmp/out.txt 2>&1',
  'prek install --hook-type pre-commit --hook-type pre-push',
  'prek run --all-files',
  'echo hello # git commit --no-verify',
  'cd backend && go test ./... && cd ..',
  'cd frontend && rm -rf node_modules/.cache',
  'rm -rf /tmp/scratch',
  'sed -i s/a/b/ backend/main.go',
  "gh pr create --title x --body 'mention git commit --no-verify here'",
  'FOO=bar make test',
  'export PATH="$HOME/bin:$PATH"',
  'HOME=/tmp/x go build ./...',
  'echo "git commit --no-verify"',
];

for (const c of BLOCK) {
  test(`blocks: ${c.split('\n')[0]}`, () => assert.ok(decide(c), c));
}
for (const c of ALLOW) {
  test(`allows: ${c.split('\n')[0]}`, () => assert.strictEqual(decide(c), null, `${c} -> ${decide(c)}`));
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
