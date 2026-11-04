import test from 'node:test';
import assert from 'node:assert/strict';
import { runCLI } from './helpers.mjs';

test('invalid decimal and out-of-range ports are never emitted', t => {
  const r = runCLI(t, 'ip_init.js', { 'init.csv': 'ip,port\n192.0.2.1,443\n192.0.2.2,443.5\n192.0.2.3,65536\n192.0.2.4,-1\n192.0.2.5,0x1bb\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443');
});

test('invalid IPs are rejected before producing dial input', t => {
  const r = runCLI(t, 'ip_init.js', { 'init.csv': 'ip,port\nnot-an-ip,443\n999.0.0.1,443\n192.0.2.1,443\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443');
});

test('JSON arrays produce validated, deduplicated input', t => {
  const data = JSON.stringify([{ ip: '192.0.2.1', port: 443 }, { ip: '192.0.2.1', port: 443 }, { ip: '::1', port: 8443 }], null, 2);
  const r = runCLI(t, 'config.js', { 'config.json': data });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443\n::1 8443');
});

test('JSON Lines input control', t => {
  const r = runCLI(t, 'config.js', { 'config.json': '{"ip":"192.0.2.1","port":443}\n{"ip":"::1","port":8443}\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443\n::1 8443');
});

test('config refuses missing fields instead of undefined output', t => {
  const r = runCLI(t, 'config.js', { 'config.json': '{"ip":"192.0.2.1"}\n', 'ip.txt': 'LOCAL_PREVIOUS_RUN\n' });
  assert.notEqual(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), 'LOCAL_PREVIOUS_RUN\n');
});
