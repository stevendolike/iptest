// Explicit local CSV fixtures; no external API or IP measurement is used.
import test from 'node:test';
import assert from 'node:assert/strict';
import { runCLI, csvHeader, localLocations } from './helpers.mjs';

// The upstream input script accepts its localized result-column names.
test('upstream input column names retain quoted parsing and deduplication', t => {
  const r = runCLI(t, 'ip_init.js', {
    'init.csv': '\ufeffIP地址,端口号,说明\n"192.0.2.1","443","LOCAL, FIXTURE"\n192.0.2.1,443,duplicate\n::1,8443,local\n',
  });
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip.txt'), '192.0.2.1 443\n::1 8443');
});

test('upstream result aliases group by trace IP location and keep bracketed IPv6', t => {
  const locations = JSON.stringify([
    { iata: 'TST', cca2: 'US', emoji: 'U', country: 'Local US' },
    { iata: 'JST', cca2: 'JP', emoji: 'J', country: 'Local Japan' },
  ]);
  const r = runCLI(t, 'ip_tq.js', {
    'result.csv': '\ufeffIP地址,端口号,数据中心,IP位置,下载速度\n"::1","08443",TST,JP,100 kB/s\n',
    'locations.json': locations,
  }, {}, ['result.csv']);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt'), '[::1]:8443#JLocal Japan1');
});

function localRow(ip, country = 'US', outbound = 'IPv4', asn = '64500', speed = '100 kB/s') {
  return [ip, '443', 'false', 'TST', country, 'LOCAL', 'LOCAL', 'LOCAL', 'LOCAL', 'LOCAL', 'TEST', '1 ms', speed, outbound, asn, 'LOCAL ORG'].join(',');
}
const richHeader = csvHeader + ',出站IP类型,ASN号码,ASN组织';

test('upstream all/top5 outputs coexist with the complete legacy result', t => {
  const us = Array.from({ length: 6 }, (_, i) => localRow(`192.0.2.${i + 1}`));
  const jp = [localRow('192.0.2.20', 'JP'), localRow('192.0.2.21', 'JP')];
  const r = runCLI(t, 'ip_tq.js', {
    'ip_tq.csv': [richHeader, ...us, ...jp].join('\n') + '\n',
    'locations.json': JSON.stringify([
      { iata: 'TST', cca2: 'US', emoji: 'U', country: 'Local US' },
      { iata: 'JST', cca2: 'JP', emoji: 'J', country: 'Local Japan' },
    ]),
  }, {}, ['ip_tq.csv']);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('ip_tq.txt').split('\n').length, us.length + jp.length);
  const expected = us.map((_, i) => `192.0.2.${i + 1}:443#ULocal US${i + 1}`);
  assert.equal(r.read('ip_all.txt'), expected.join('\n'));
  assert.equal(r.read('ip_top5.txt'), expected.slice(0, 5).join('\n'));
});

test('upstream outbound filter selects only explicit matching output types', async t => {
  for (const mode of ['ipv4', 'ipv6', 'all']) {
    await t.test(mode, t => {
      const r = runCLI(t, 'ip_tq.js', {
        'result.csv': [richHeader, localRow('192.0.2.1'), localRow('::1', 'US', 'IPv6'), localRow('192.0.2.2', 'US', '')].join('\n') + '\n',
        'locations.json': localLocations,
      }, {}, ['result.csv', `--outbound=${mode}`]);
      assert.equal(r.status, 0, r.stderr);
      const entries = r.read('result.txt').split('\n');
      const expected = mode === 'ipv4' ? ['192.0.2.1:443'] : mode === 'ipv6' ? ['[::1]:443'] : ['192.0.2.1:443', '[::1]:443', '192.0.2.2:443'];
      assert.deepEqual(entries.map(line => line.split('#')[0]), expected);
    });
  }
});

test('invalid outbound option fails before replacing existing output', t => {
  const previous = 'LOCAL PREVIOUS RESULT\n';
  const r = runCLI(t, 'ip_tq.js', {
    'result.csv': `${richHeader}\n${localRow('192.0.2.1')}\n`, 'locations.json': localLocations,
    'result.txt': previous,
  }, {}, ['result.csv', '--outbound=invalid']);
  assert.notEqual(r.status, 0);
  assert.equal(r.read('result.txt'), previous);
});

test('upstream ASN selection changes only the IPv4 log list, not saved records', t => {
  const r = runCLI(t, 'ip_tq.js', {
    'result.csv': [richHeader, localRow('192.0.2.1'), localRow('::1', 'US', 'IPv6'), localRow('192.0.2.2', 'US', 'IPv4', '64501')].join('\n') + '\n',
    'locations.json': localLocations,
  }, {}, ['result.csv', '--asn=64500']);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(r.read('result.txt').split('\n').length, 3);
  const logged = r.stdout.match(/目标 ASN IPv4 列表: (\[[^\n]*\])/);
  assert.ok(logged, r.stdout);
  assert.deepEqual(JSON.parse(logged[1]), ['192.0.2.1:443']);
});

test('invalid ASN token is rejected literally before output changes', t => {
  const previous = 'LOCAL PREVIOUS RESULT\n';
  const r = runCLI(t, 'ip_tq.js', {
    'result.csv': `${richHeader}\n${localRow('192.0.2.1')}\n`, 'locations.json': localLocations, 'result.txt': previous,
  }, {}, ['result.csv', '--asn=AS64500']);
  assert.notEqual(r.status, 0);
  assert.equal(r.read('result.txt'), previous);
});

test('new grouped outputs cannot overwrite another selected CSV result', t => {
  const csv = `${richHeader}\n${localRow('192.0.2.1')}\n`;
  const r = runCLI(t, 'ip_tq.js', {
    'ip_tq.csv': csv, 'ip_all.csv': csv, 'locations.json': localLocations,
    'ip_tq.txt': 'LOCAL TQ PREVIOUS', 'ip_all.txt': 'LOCAL ALL PREVIOUS',
  }, {}, ['ip_tq.csv', 'ip_all.csv']);
  assert.notEqual(r.status, 0);
  assert.equal(r.read('ip_tq.txt'), 'LOCAL TQ PREVIOUS');
  assert.equal(r.read('ip_all.txt'), 'LOCAL ALL PREVIOUS');
});
