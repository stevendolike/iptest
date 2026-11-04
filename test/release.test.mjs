import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, spawnSync, execFileSync } from 'node:child_process';
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
  return {
    dir, first, second, git,
    addRemote() {
      const remote = path.join(dir, 'remote.git');
      git('init', '--bare', '--quiet', remote);
      git('remote', 'add', 'origin', remote);
      git('push', '--quiet', 'origin', 'HEAD:refs/heads/main');
      return remote;
    },
    run(tag, sha = second, mode) {
      const args = [path.join(repo, 'tools', 'resolve-release.sh').replaceAll('\\', '/'), tag, sha];
      if (mode) args.push(mode);
      return spawnSync(bash, args, { cwd: dir, encoding: 'utf8', timeout: 10000 });
    },
    publishAsync(tag, sha) {
      return new Promise((resolve, reject) => {
        const script = path.join(repo, 'tools', 'resolve-release.sh').replaceAll('\\', '/');
        const child = spawn(bash, [script, tag, sha, 'publish'], { cwd: dir });
        let stdout = '', stderr = '';
        const timer = setTimeout(() => { child.kill(); reject(new Error('local publish fixture timed out')); }, 10000);
        child.stdout.on('data', data => { stdout += data; });
        child.stderr.on('data', data => { stderr += data; });
        child.on('error', error => { clearTimeout(timer); reject(error); });
        child.on('close', status => { clearTimeout(timer); resolve({ status, stdout, stderr }); });
      });
    },
  };
}

test('manual dispatch rejects an existing historical tag before emitting outputs', t => {
  const f = gitFixture(t);
  const r = f.run('v0.1.0');
  assert.equal(r.error, undefined);
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /existing release tag.*new tag/i);
  assert.equal(r.stdout, '');
  assert.equal(f.git('rev-parse', 'v0.1.0^{commit}'), f.first);
});

test('tag push is bound to the event commit rather than a different selected commit', t => {
  const f = gitFixture(t);
  const control = f.run('v0.1.0', f.first, 'push');
  assert.equal(control.status, 0, control.stderr);
  assert.match(control.stdout, new RegExp(`commit=${f.first}(?:\\r?\\n|$)`));
  const mismatch = f.run('v0.1.0', f.second, 'push');
  assert.equal(mismatch.error, undefined);
  assert.notEqual(mismatch.status, 0);
  assert.equal(mismatch.stdout, '');
  assert.match(mismatch.stderr, /commit mismatch/i);
});

test('tag push rejects a missing event tag instead of treating it as a new release', t => {
  const f = gitFixture(t);
  const r = f.run('v0.2.0', f.second, 'push');
  assert.equal(r.error, undefined);
  assert.notEqual(r.status, 0);
  assert.equal(r.stdout, '');
  assert.match(r.stderr, /missing release tag/i);
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

test('manual release requires an explicit new tag and routes the event mode', () => {
  const workflow = fs.readFileSync(path.join(repo, '.github', 'workflows', 'main.yml'), 'utf8');
  assert.doesNotMatch(workflow, /default:\s*["']?v1\.0\.0/);
  assert.match(workflow, /description:[^\n]*new[^\n]*unused/i);
  assert.match(workflow, /"\$REQUESTED_TAG" "\$GITHUB_SHA" "\$GITHUB_EVENT_NAME"/);
});

test('release build checkout and release target use the resolved commit', () => {
  const workflow = fs.readFileSync(path.join(repo, '.github', 'workflows', 'main.yml'), 'utf8');
  assert.match(workflow, /ref:\s*\$\{\{ needs\.resolve\.outputs\.commit \}\}/);
  assert.match(workflow, /target_commitish:\s*\$\{\{ needs\.resolve\.outputs\.commit \}\}/);
  assert.doesNotMatch(workflow, /Kwisma\/iptest/);
});

// All remotes below are local bare Git fixtures, never GitHub or public IPs.
test('publish atomically creates the remote tag at the resolved commit', t => {
  const f = gitFixture(t);
  f.addRemote();
  const r = f.run('v0.2.0', f.second, 'publish');
  assert.equal(r.error, undefined);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v0.2.0'), `${f.second}\trefs/tags/v0.2.0`);
  assert.match(r.stdout, new RegExp(`commit=${f.second}(?:\\r?\\n|$)`));
});

test('publish rejects a competing tag at a different commit without moving it', t => {
  const f = gitFixture(t);
  f.addRemote();
  const resolved = f.run('v0.2.0', f.second);
  assert.equal(resolved.status, 0, resolved.stderr);
  f.git('push', '--quiet', 'origin', `${f.first}:refs/tags/v0.2.0`);
  const r = f.run('v0.2.0', f.second, 'publish');
  assert.equal(r.error, undefined);
  assert.notEqual(r.status, 0);
  assert.equal(r.stdout, '');
  assert.match(r.stderr, /commit mismatch/i);
  assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v0.2.0'), `${f.first}\trefs/tags/v0.2.0`);
});

test('simultaneous publishers at different commits have exactly one remote tag winner', async t => {
  const f = gitFixture(t);
  f.addRemote();
  const results = await Promise.all([f.publishAsync('v0.2.0', f.first), f.publishAsync('v0.2.0', f.second)]);
  const winners = results.filter(result => result.status === 0);
  assert.equal(winners.length, 1, JSON.stringify(results));
  const outputs = Object.fromEntries(winners[0].stdout.trim().split(/\r?\n/).map(line => line.split('=')));
  assert.ok([f.first, f.second].includes(outputs.commit));
  assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v0.2.0'), `${outputs.commit}\trefs/tags/v0.2.0`);
  assert.ok(results.filter(result => result.status !== 0).every(result => result.stdout === ''));
});

test('publish compares an annotated remote tag to its peeled commit', t => {
  const f = gitFixture(t);
  f.addRemote();
  f.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'tag.gpgsign=false', 'tag', '-a', 'v0.2.0', f.second, '-m', 'LOCAL ANNOTATED TAG');
  const tagObject = f.git('rev-parse', 'v0.2.0');
  assert.notEqual(tagObject, f.second);
  f.git('push', '--quiet', 'origin', 'refs/tags/v0.2.0');
  const control = f.run('v0.2.0', f.second, 'publish');
  assert.equal(control.status, 0, control.stderr);
  assert.match(control.stdout, new RegExp(`commit=${f.second}(?:\\r?\\n|$)`));
  const mismatch = f.run('v0.2.0', f.first, 'publish');
  assert.notEqual(mismatch.status, 0);
  assert.match(mismatch.stderr, /commit mismatch/i);
  assert.equal(mismatch.stdout, '');
  assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v0.2.0'), `${tagObject}\trefs/tags/v0.2.0`);
});

test('publish cannot report success when the remote cannot be read', t => {
  const f = gitFixture(t);
  f.git('remote', 'add', 'origin', path.join(f.dir, 'missing-remote.git'));
  const r = f.run('v0.2.0', f.second, 'publish');
  assert.equal(r.error, undefined);
  assert.notEqual(r.status, 0);
  assert.equal(r.stdout, '');
  assert.match(r.stderr, /remote/i);
});

test('release workflow guards the remote binding before publishing without overwrites', () => {
  const workflow = fs.readFileSync(path.join(repo, '.github', 'workflows', 'main.yml'), 'utf8');
  assert.match(workflow, /concurrency:[\s\S]*?group:[^\n]*release_tag[^\n]*github\.ref_name/);
  assert.match(workflow, /cancel-in-progress:\s*false/);
  const release = workflow.slice(workflow.indexOf('\n  release:'));
  assert.match(release, /actions\/checkout@/);
  const guard = release.indexOf('"$RELEASE_TAG" "$RELEASE_COMMIT" publish');
  assert.ok(guard >= 0 && guard < release.indexOf('softprops/action-gh-release@'), 'remote binding must be verified before the release action');
  assert.match(release, /overwrite_files:\s*false/);
});
