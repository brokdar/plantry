'use strict';
// Run: node --test .claude/hooks/block-hook-bypass.test.js
const test = require('node:test');
const assert = require('node:assert');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const { decide } = require('./block-hook-bypass.js');

// a fake key built at run time (never a real credential, never in the source)
const FAKE = 'sk-ant-api03-' + 'Ab3dE6gH9jK2mN5pQ8sT1vW4yZ7Qa';

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
  // second review: aliases, env, interpreters, attributes, remote-tracking refs
  "git config alias.pn 'push --no-verify'",
  "git config --global alias.ci 'commit -n'",
  "git config --global alias.x '!git commit --no-verify'",
  'git config core.attributesFile /x',
  'git config --global core.attributesfile /x',
  'PATH=$PWD/fake:$PATH git commit -m x',
  'PATH=./fake:$PATH git push origin x',
  'PATH="$(pwd)/fake:$PATH" scripts/secret-guard.sh staged',
  'export PATH=./fake:$PATH',
  'export PATH=/tmp/fake:$PATH',
  'PREK_HOME=/tmp/x prek run',
  'GIT_ATTR_SOURCE=x git commit -m x',
  'GIT_CONFIG_GLOBAL=/tmp/g git commit -m x',
  'PYTHONPATH=/tmp/p bash scripts/secret-guard.sh range a b',
  'BASH_ENV=/tmp/e git commit -m x',
  "python3 -c \"import subprocess;subprocess.run(['git','push','--no-verify'])\"",
  "node -e \"require('child_process').execFileSync('git',['push','--no-verify'])\"",
  "python3 - <<'E'\nopen('prek.toml','w').write('')\nE",
  "python3 -c \"open('.git/info/attributes','w').write('*.dat -diff')\"",
  "echo '*.dat -diff' >> .git/info/attributes",
  "echo '[alias]' >> ~/.gitconfig",
  "sed -i 's/x/y/' .git/config",
  'git update-ref refs/remotes/origin/leak HEAD',
  'git update-ref -m x refs/remotes/origin/leak $(git rev-parse HEAD)',
  'git fetch . leak:refs/remotes/origin/x',
  'git fetch . +leak:refs/remotes/origin/x',
  // gh: labels, rulesets, admin, workflows
  'gh pr edit 5 --add-label secret-guard-change',
  'gh pr edit 5 --add-label "secret-guard-change"',
  'gh label create secret-guard-change',
  'gh label delete x',
  'gh api -X POST repos/o/r/labels -f name=x',
  'gh api repos/o/r/issues/5/labels -f labels[]=secret-guard-change',
  'gh api repos/o/r/rulesets/1 -X DELETE',
  'gh api --method PUT repos/o/r/branches/main/protection',
  'gh api -X PUT repos/o/r/actions/workflows/1/disable',
  'gh workflow disable secrets.yml',
  'gh pr merge 5 --admin',
  'gh pr merge 5 --squash --admin',
  // publish-time scan: the text of what is sent to GitHub
  `gh pr create --title t --body "token ${FAKE}"`,
  `gh pr create --title "x ${FAKE}" --body ok`,
  `gh pr edit 5 --body "key: ${FAKE}"`,
  `gh pr comment 3 --body "${FAKE}"`,
  `gh issue create -t t -b "${FAKE}"`,
  `gh issue comment 3 --body "${FAKE}"`,
  `gh release create v1 --notes "${FAKE}"`,
  `gh pr create --title t --body-file - <<'EOF'\nsecret ${FAKE}\nEOF`,
  `gh pr create --title t --body "$(cat <<'EOF'\n## Summary\nleaked ${FAKE}\nEOF\n)"`,
  `bash -c 'gh pr comment 3 --body "${FAKE}"'`,
  `gh pr review 3 --approve --body "${FAKE}"`,
  `gh pr review 3 -b "${FAKE}"`,
  `gh api repos/o/r/issues/3/comments -f body="${FAKE}"`,
  `gh api repos/o/r/pulls/3/reviews -F body="${FAKE}"`,
  `gh api repos/o/r/pulls/3/comments --raw-field body="${FAKE}"`,
  `gh api repos/o/r/pulls/3/comments --field=body="${FAKE}"`,
  `gh api graphql -f query="mutation { x(body: \\"${FAKE}\\") }"`,
  `gh api repos/o/r/releases -f body="${FAKE}"`,
  `gh api gists -f description="${FAKE}"`,
  `gh api repos/o/r/issues/3/comments -f body="$(date)"`,
  'gh pr comment 3 --body "$(some-tool)"',
  'gh pr create --title t --body "$(cat "$F")"',
  'gh pr comment 3 --body `ls`',
  'gh pr comment 3 --body "$(cat /nonexistent/body.md)"',
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
  'git config alias.st status',
  'git config --global alias.co checkout',
  'GIT_AUTHOR_NAME=x GIT_COMMITTER_NAME=x git commit -m x',
  'GIT_SEQUENCE_EDITOR=true git rebase -i HEAD~2',
  'PATH=$HOME/.local/bin:$PATH go build ./...',
  'export PATH="$HOME/.bun/bin:$PATH"',
  'python3 -c "print(1)"',
  'node -e "console.log(1)"',
  'git update-ref refs/heads/x HEAD',
  'git fetch origin',
  'git fetch origin main:refs/heads/main',
  'gh pr create --title "feat: x" --body "A normal body"',
  'gh pr comment 3 --body "sk-ant- prefixes are scanned"',
  'gh pr view 5',
  'gh pr list --label bug',
  'gh pr merge 5 --squash',
  'gh pr checks 5 --watch',
  'gh api repos/o/r/pulls/5/comments',
  'gh api -X GET repos/o/r/labels',
  'gh api repos/o/r/actions/runs',
  'gh run list --workflow secrets.yml',
  'gh release view v1',
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

// hook process cases that need files or git config

const fs = require('node:fs');
const os = require('node:os');

test('blocks a --body-file whose content carries a key (publish-time scan)', () => {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), 'hookbody-'));
  const f = path.join(d, 'body.md');
  fs.writeFileSync(f, `## Summary\nhere is the token ${FAKE}\n`);
  for (const cmd of [`gh pr create --title t --body-file ${f}`, `gh pr comment 3 -F ${f}`, `gh release create v1 --notes-file ${f}`, `gh pr edit 3 --body-file=${f}`]) {
    assert.match(decide(cmd) || '', /credential pattern/, cmd);
  }
  fs.writeFileSync(f, '## Summary\nnothing secret here\n');
  assert.strictEqual(decide(`gh pr create --title t --body-file ${f}`), null);
  fs.rmSync(d, { recursive: true });
});

test('scans files behind gh api, gist and $(cat file) bodies', () => {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), 'hookbody2-'));
  const f = path.join(d, 'b.txt');
  fs.writeFileSync(f, `token ${FAKE}\n`);
  const cmds = [
    `gh api repos/o/r/issues/3/comments -F body=@${f}`,
    `gh api repos/o/r/issues/3/comments --input ${f}`,
    `gh api repos/o/r/issues/3/comments --input=${f}`,
    `gh gist create ${f}`,
    `gh gist edit abc ${f}`,
    `gh pr comment 3 --body "$(cat ${f})"`,
    `gh pr review 3 --comment --body "$(cat '${f}')"`,
    'gh pr comment 3 --body `cat ' + f + '`',
    `gh pr review 3 --comment --body-file ${f}`,
  ];
  for (const c of cmds) assert.match(decide(c) || '', /credential pattern/, c);
  fs.writeFileSync(f, 'nothing secret\n');
  for (const c of cmds) assert.strictEqual(decide(c), null, c + ' -> ' + decide(c));
  assert.match(decide('gh pr comment 3 --body "$(some-tool)"') || '', /--body-file/);
  fs.rmSync(d, { recursive: true });
});

test('the block message names the pattern, never the secret', () => {
  const r = decide(`gh pr create --title t --body "${FAKE}"`);
  assert.ok(r);
  assert.ok(!r.includes(FAKE), 'the message must not echo the key');
});

test('blocks invoking a configured alias that runs --no-verify', () => {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), 'hookalias-'));
  const cfg = path.join(d, 'gitconfig');
  fs.writeFileSync(cfg, '[alias]\n\tpn = push --no-verify\n\tst = status\n');
  const old = process.env.GIT_CONFIG_GLOBAL;
  process.env.GIT_CONFIG_GLOBAL = cfg;
  try {
    assert.match(decide('git pn origin leak') || '', /alias pn/);
    assert.strictEqual(decide('git st'), null);
  } finally {
    if (old === undefined) delete process.env.GIT_CONFIG_GLOBAL; else process.env.GIT_CONFIG_GLOBAL = old;
    fs.rmSync(d, { recursive: true });
  }
});
