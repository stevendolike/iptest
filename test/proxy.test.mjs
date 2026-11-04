// Success payloads are injected by runCLI; no real ProxyIP API is called.
import test from 'node:test';
import assert from 'node:assert/strict';
import { runCLI } from './helpers.mjs';

for (const [kind, output, empty] of [
  ['proxyip', 'proxy_ips.txt', 'reverse_ips.txt'],
  ['reverse', 'reverse_ips.txt', 'proxy_ips.txt'],
]) {
  test(`${kind} output brackets IPv6 and preserves IPv4 endpoints`, t => {
    const r = runCLI(t, 'ProxyIP.js', {
      'init.csv': 'ip,port\n192.0.2.1,443\n::1,8443\n2001:db8::1,443\n',
    }, { [kind]: true });
    assert.equal(r.error, undefined);
    assert.equal(r.status, 0, r.stderr);
    assert.equal(r.read(output), '192.0.2.1:443\n[::1]:8443\n[2001:db8::1]:443');
    assert.equal(r.read(empty), '');
    const requests = r.read('mock-requests.jsonl').trim().split('\n').map(JSON.parse);
    assert.deepEqual(requests.map(item => new URL(item.url).searchParams.get('ip')), ['192.0.2.1', '::1', '2001:db8::1']);
    assert.ok(requests.every(item => item.fixture === true));
  });
}
