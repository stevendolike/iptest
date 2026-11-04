// Convert JSON arrays, one JSON object, or JSON Lines into Go dial input.
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseIPPort } from './lib/address.js';
import { writeTextAtomic } from './lib/io.js';

const dir = path.dirname(fileURLToPath(import.meta.url));
try {
  const text = (await fs.readFile(path.join(dir, 'config.json'), 'utf8')).trim();
  let rows;
  if (!text) {
    rows = [];
  } else {
    try {
      const value = JSON.parse(text);
      rows = Array.isArray(value) ? value : [value];
    } catch {
      rows = text.split(String.fromCharCode(10)).filter(line => line.trim()).map(line => JSON.parse(line));
    }
  }
  const entries = new Set();
  for (const row of rows) {
    if (!row || typeof row !== 'object' || Array.isArray(row)) throw new Error('每条 JSON 记录必须为对象');
    const address = parseIPPort(row.ip, row.port);
    if (!address) throw new Error('JSON 记录缺少合法的 ip / port');
    entries.add(`${address.ip} ${address.port}`);
  }
  await writeTextAtomic(path.join(dir, 'ip.txt'), [...entries].join('\n'));
  console.log('已成功提取到 ip.txt');
} catch (error) {
  console.error('处理 JSON 时发生错误:', error.message);
  process.exitCode = 1;
}
