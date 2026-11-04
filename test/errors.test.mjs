import test from 'node:test';
import assert from 'node:assert/strict';
import { runCLI, csvHeader, localLocations } from './helpers.mjs';

test('missing input returns failure without overwriting old input output', t => {
  const r = runCLI(t, 'ip_init.js', { 'ip.txt': 'LOCAL_PREVIOUS_RUN\n' });
  assert.notEqual(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), 'LOCAL_PREVIOUS_RUN\n');
});

test('malformed output CSV must return failure and preserve old text', t => {
  const r = runCLI(t, 'ip_tq.js', { 'result.csv': 'IP地址,端口\n192.0.2.1,443\n', 'result.txt': 'LOCAL_PREVIOUS_RUN\n', 'locations.json': localLocations });
  assert.notEqual(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt'), 'LOCAL_PREVIOUS_RUN\n');
});

test('an explicitly selected result does not parse unrelated input CSV', t => {
  const row = '192.0.2.1,443,true,TST,US,Fixture,Fixture,Fixture,Fixture,Fixture,TEST,0 ms,123 kB/s';
  const r = runCLI(t, 'ip_tq.js', { 'init.csv': 'ip,port\n192.0.2.1,443\n', 'result.csv': csvHeader + '\n' + row + '\n', 'locations.json': localLocations }, undefined, ['result.csv']);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.stderr, '');
  assert.equal(r.read('result.txt'), '192.0.2.1:443#TESTFixture Country1');
});

test('a fresh empty CSV result replaces prior text with empty text', t => {
  const r = runCLI(t, 'ip_tq.js', { 'result.csv': csvHeader + '\n', 'result.txt': 'LOCAL_PREVIOUS_RUN\n', 'locations.json': localLocations });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt'), '');
});

test('invalid config JSON must fail rather than claim successful conversion', t => {
  const r = runCLI(t, 'config.js', { 'config.json': '{not JSON}', 'ip.txt': 'LOCAL_PREVIOUS_RUN\n' });
  assert.notEqual(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), 'LOCAL_PREVIOUS_RUN\n');
});

test('ProxyIP HTTP service errors fail without replacing prior results', t => {
  const r = runCLI(t, 'ProxyIP.js', { 'init.csv': 'ip,port\n192.0.2.1,443\n', 'proxy_ips.txt': 'LOCAL_PREVIOUS_RUN\n' }, { __status: 503 });
  assert.notEqual(r.status, 0, r.stderr);
  assert.equal(r.read('proxy_ips.txt'), 'LOCAL_PREVIOUS_RUN\n');
});

test('partial output write does not destroy the last successful output', t => {
  const preload = `import fs from 'node:fs/promises';
const original = fs.writeFile.bind(fs);
fs.writeFile = async (file, data, options) => {
  if (String(data) === '192.0.2.1 443') {
    await original(file, 'LOCAL_PARTIAL_WRITE', options);
    throw Object.assign(new Error('LOCAL_FIXTURE_ENOSPC'), { code: 'ENOSPC' });
  }
  return original(file, data, options);
};`;
  const r = runCLI(t, 'ip_init.js', { 'init.csv': 'ip,port\n192.0.2.1,443\n', 'ip.txt': 'LOCAL_PREVIOUS_RUN\n', 'preload.mjs': preload });
  assert.equal(r.read('ip.txt'), 'LOCAL_PREVIOUS_RUN\n');
  assert.notEqual(r.status, 0, r.stderr);
});
