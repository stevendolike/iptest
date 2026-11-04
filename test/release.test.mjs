import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync, execFileSync } from 'node:child_process';
import { repo } from './helpers.mjs';

function gitFixture(t) {
  const parent = process.env.IPTEST_TEST_TMP || os.tmpdir();
  fs.mkdirSync(parent, { recursive: true });
  const dir = fs.mkdtempSync(path.join(parent, 'release-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, 'empty-hooks'));
  const git = (...args) => execFileSync('git', ['-C', dir, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init', '--quiet');
  const commit = message => git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', '-c', `core.hooksPath=${path.join(dir, 'empty-hooks')}`, 'commit', '--allow-empty', '--quiet', '-m', message);
  commit('LOCAL FIXTURE A');
  const first = git('rev-parse', 'HEAD');
  git('tag', 'v0.1.0', first);
  commit('LOCAL FIXTURE B');
  const second = git('rev-parse', 'HEAD');
  const bash = process.env.IPTEST_TEST_BASH || (process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : '/bin/bash');
  return { dir, first, second, run(tag, sha = second) { return spawnSync(bash, [path.join(repo, 'tools', 'resolve-release.sh').replaceAll('\\', '/'), tag, sha], { cwd: dir, encoding: 'utf8', timeout: 10000 }); } };
}

test('existing release tag resolves to its own commit instead of selected branch', t => {
  const f = gitFixture(t);
  const r = f.run('v0.1.0');
  assert.equal(r.status, 0, r.stderr);
  assert.match(r.stdout, new RegExp(`commit=${f.first}(?:\\r?\\n|$)`));
});

test('new release tag is bound to the explicitly selected commit', t => {
  const f = gitFixture(t);
  const r = f.run('v0.2.0-rc.1');
  assert.equal(r.status, 0, r.stderr);
  assert.match(r.stdout, new RegExp(`commit=${f.second}(?:\\r?\\n|$)`));
  assert.match(r.stdout, /tag=v0\.2\.0-rc\.1/);
});

test('invalid release tag is rejected before any successful resolution', t => {
  const f = gitFixture(t);
  const r = f.run('main');
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /invalid release tag/i);
  assert.equal(r.stdout, '');
});

test('release build checkout and release target use the resolved commit', () => {
  const workflow = fs.readFileSync(path.join(repo, '.github', 'workflows', 'main.yml'), 'utf8');
  assert.match(workflow, /ref:\s*\$\{\{ needs\.resolve\.outputs\.commit \}\}/);
  assert.match(workflow, /target_commitish:\s*\$\{\{ needs\.resolve\.outputs\.commit \}\}/);
  assert.doesNotMatch(workflow, /Kwisma\/iptest/);
});
