// 从 init.csv 提取唯一的 IP、端口组合。

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readCSV } from './lib/csv.js';
import { writeTextAtomic } from './lib/io.js';
import { parseIPPort } from './lib/address.js';

const dir = path.dirname(fileURLToPath(import.meta.url));
try {
  const { headers, rows } = await readCSV(path.join(dir, 'init.csv'));
  const ipColumn = ['ip', 'IP', 'IP地址'].find(name => headers.includes(name));
  const portColumn = ['port', 'Port', '端口', '端口号'].find(name => headers.includes(name));
  if (!ipColumn || !portColumn) throw new Error('CSV 文件缺少 ip／IP地址 或 port／端口号 列');
  const entries = new Set();
  for (const row of rows) {
    const ip = row[ipColumn], port = row[portColumn];
    const address = parseIPPort(ip, port);
    if (!address) {
      console.error(`无效 IP 或端口: ${ip} ${port}`);
      continue;
    }
    entries.add(`${address.ip} ${address.port}`);
  }
  await writeTextAtomic(path.join(dir, 'ip.txt'), [...entries].join('\n'));
  console.log('已成功提取到 ip.txt');
} catch (error) {
  console.error('处理文件时发生错误:', error.message);
  process.exitCode = 1;
}
