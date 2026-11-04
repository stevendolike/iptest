import test from 'node:test';
import assert from 'node:assert/strict';
import { runCLI, csvHeader, localLocations } from './helpers.mjs';

test('simple CSV input control', t => {
  const r = runCLI(t, 'ip_init.js', { 'init.csv': 'ip,port\n192.0.2.1,443\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443');
});

test('quoted IP and port decode to usable input', t => {
  const r = runCLI(t, 'ip_init.js', { 'init.csv': '"ip","port"\n"192.0.2.1","443"\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443');
});

test('quoted metadata comma does not shift input columns', t => {
  const r = runCLI(t, 'ip_init.js', { 'init.csv': 'meta,ip,port\n"comma, field",192.0.2.1,443\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443');
});

test('quoted city cannot turn latency into a positive download speed', t => {
  const row = '192.0.2.1,443,true,TST,US,Fixture,"City, State",Fixture,Fixture Country,Fixture City,TEST,10 ms,0 kB/s';
  const r = runCLI(t, 'ip_tq.js', { 'result.csv': '\ufeff' + csvHeader + '\n' + row + '\n', 'locations.json': localLocations });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt'), '');
});

test('quoted multiline city preserves speed and output entry', t => {
  const row = '192.0.2.1,443,true,TST,US,Fixture,"City,\nState",Fixture,Fixture Country,Fixture City,TEST,0 ms,123 kB/s';
  const r = runCLI(t, 'ip_tq.js', { 'result.csv': csvHeader + '\n' + row + '\n', 'locations.json': localLocations });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt'), '192.0.2.1:443#TESTFixture Country1');
});

test('ProxyIP accepts and decodes quoted CSV fields', t => {
  const r = runCLI(t, 'ProxyIP.js', { 'init.csv': 'ip,port\n"192.0.2.1","443"\n' });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('proxy_ips.txt'), '192.0.2.1:443');
  assert.match(r.read('mock-requests.jsonl'), /192\.0\.2\.1/);
});
