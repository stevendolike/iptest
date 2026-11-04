// CLI fixtures only. The injected node-fetch module NEVER uses the network.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

export const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export function runCLI(t, script, inputs = {}, fetchResult = { proxyip: true }, args = []) {
  const parent = process.env.IPTEST_TEST_TMP || os.tmpdir();
  fs.mkdirSync(parent, { recursive: true });
  const dir = fs.mkdtempSync(path.join(parent, 'iptest-node-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.copyFileSync(path.join(repo, script), path.join(dir, script));
  fs.writeFileSync(path.join(dir, 'package.json'), '{"type":"module"}\n');
  if (fs.existsSync(path.join(repo, 'lib'))) fs.cpSync(path.join(repo, 'lib'), path.join(dir, 'lib'), { recursive: true });
  const modules = path.join(dir, 'node_modules');
  fs.mkdirSync(modules);
  for (const pkg of ['csv-parse']) {
    const source = path.join(repo, 'node_modules', pkg);
    if (fs.existsSync(source)) fs.cpSync(source, path.join(modules, pkg), { recursive: true });
  }
  const stub = path.join(modules, 'node-fetch');
  fs.mkdirSync(stub);
  fs.writeFileSync(path.join(stub, 'package.json'), '{"name":"node-fetch","version":"0.0.0-test","type":"module","exports":"./index.js"}\n');
  const fetchStatus = fetchResult?.__status ?? 200;
  fs.writeFileSync(path.join(stub, 'index.js'), `
import fs from 'node:fs';
export default async function fetch(url) {
  fs.appendFileSync('mock-requests.jsonl', JSON.stringify({ fixture: true, url }) + '\\n');
  return { ok: ${fetchStatus >= 200 && fetchStatus < 300}, status: ${fetchStatus}, async json() { return ${JSON.stringify(fetchResult)}; }, async text() { return 'LOCAL TEST RESPONSE'; } };
}
`);
  for (const [name, data] of Object.entries(inputs)) fs.writeFileSync(path.join(dir, name), data);
  const preload = Object.hasOwn(inputs, 'preload.mjs') ? ['--import', './preload.mjs'] : [];
  const proc = spawnSync(process.execPath, [...preload, script, ...args], { cwd: dir, encoding: 'utf8', timeout: 6000 });
  return { ...proc, dir, read(name) { const p = path.join(dir, name); return fs.existsSync(p) ? fs.readFileSync(p, 'utf8') : null; } };
}

export const csvHeader = 'IP地址,端口,TLS,数据中心,源IP位置,地区,城市,地区(中文),国家,城市(中文),国旗,网络延迟,下载速度';
export const localLocations = JSON.stringify([{ iata: 'TST', cca2: 'US', emoji: 'TEST', country: 'Fixture Country' }]);
