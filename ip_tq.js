// 从 Go 输出的 CSV 提取 IP，并按国家分组。
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readCSV } from './lib/csv.js';
import { writeTextAtomic } from './lib/io.js';
import { parseIPPort, formatIPPort } from './lib/address.js';

const shu = 0; // 每个国家的数量，0 表示全部。
const speed = true;
const test = 0; // kB/s。
const dir = path.dirname(fileURLToPath(import.meta.url));

async function extract(file) {
  const source = path.resolve(dir, file);
  const { headers, rows } = await readCSV(source, ['IP地址', '端口', '数据中心']);
  const locations = JSON.parse(await fs.readFile(path.join(dir, 'locations.json'), 'utf8'));
  const groups = new Map();
  for (const [index, row] of rows.entries()) {
    const address = parseIPPort(row['IP地址'], row['端口']);
    if (!address) throw new Error(`CSV 第 ${index + 1} 条数据的 IP 或端口无效`);
    if (speed && headers.includes('下载速度')) {
      const measured = Number.parseFloat(row['下载速度'].replace(/\s*kB\/s\s*$/, ''));
      if (!Number.isFinite(measured) || measured <= test) continue;
    }
    const loc = locations.find(item => item.iata === row['数据中心']);
    const country = loc ? loc.emoji + loc.country : 'Unknown';
    if (!groups.has(country)) groups.set(country, []);
    const entries = groups.get(country);
    if (shu === 0 || entries.length < shu) entries.push(`${formatIPPort(address.ip, address.port)}#${country}`);
  }
  const output = [...groups.keys()].sort().map(country => groups.get(country).map((entry, i) => `${entry}${i + 1}`).join('\n')).join('\n');
  const destination = source.replace(/\.csv$/, '.txt');
  if (destination === source) throw new Error('输入文件扩展名必须为 .csv');
  await writeTextAtomic(destination, output);
  console.log(`已成功提取到 ${destination}`);
}

try {
  const requested = process.argv.slice(2);
  const files = requested.length ? requested : (await fs.readdir(dir)).filter(file => file.endsWith('.csv') && file !== 'init.csv');
  if (!files.length) throw new Error('未找到待处理的 CSV 文件');
  for (const file of files) {
    try {
      await extract(file);
    } catch (error) {
      console.error('处理文件时发生错误:', error.message);
      process.exitCode = 1;
    }
  }
} catch (error) {
  console.error('处理文件时发生错误:', error.message);
  process.exitCode = 1;
}
