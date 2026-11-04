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
const perCountryCount = 5; // 上游的国家最小数量及 top N。
let outboundType = 'all';
let targetASNs = [];
const dir = path.dirname(fileURLToPath(import.meta.url));

function outputPaths(source) {
  if (!source.endsWith('.csv')) throw new Error('输入文件扩展名必须为 .csv');
  const prefix = source.slice(0, -4);
  const groupedPrefix = path.basename(prefix) === 'ip_tq' ? path.join(path.dirname(prefix), 'ip') : prefix;
  return [`${prefix}.txt`, `${groupedPrefix}_all.txt`, `${groupedPrefix}_top${perCountryCount}.txt`];
}

async function extract(file) {
  const source = path.resolve(dir, file);
  const { headers, rows } = await readCSV(source, ['IP地址', '数据中心']);
  const portColumn = ['端口', '端口号'].find(name => headers.includes(name));
  if (!portColumn) throw new Error('CSV 文件缺少 端口／端口号 列');
  const locations = JSON.parse(await fs.readFile(path.join(dir, 'locations.json'), 'utf8'));
  const groups = new Map();
  const asnEndpoints = [];
  for (const [index, row] of rows.entries()) {
    const address = parseIPPort(row['IP地址'], row[portColumn]);
    if (!address) throw new Error(`CSV 第 ${index + 1} 条数据的 IP 或端口无效`);
    if (outboundType !== 'all' && row['出站IP类型'] !== (outboundType === 'ipv4' ? 'IPv4' : 'IPv6')) continue;
    if (speed && headers.includes('下载速度')) {
      const measured = Number.parseFloat(row['下载速度'].replace(/\s*kB\/s\s*$/, ''));
      if (!Number.isFinite(measured) || measured <= test) continue;
    }
    if (targetASNs.includes(row['ASN号码']) && row['出站IP类型'] === 'IPv4') asnEndpoints.push(formatIPPort(address.ip, address.port));
    const sourceCountry = row['IP位置'] ?? row['源IP位置'];
    const loc = sourceCountry
      ? locations.find(item => item.iata === row['数据中心'] && item.cca2 === sourceCountry) || locations.find(item => item.cca2 === sourceCountry)
      : locations.find(item => item.iata === row['数据中心']);
    const country = loc ? loc.emoji + loc.country : 'Unknown';
    if (!groups.has(country)) groups.set(country, []);
    const entries = groups.get(country);
    if (shu === 0 || entries.length < shu) entries.push(`${formatIPPort(address.ip, address.port)}#${country}`);
  }
  const output = [...groups.keys()].sort().map(country => groups.get(country).map((entry, i) => `${entry}${i + 1}`).join('\n')).join('\n');
  const [destination, allDestination, topDestination] = outputPaths(source);
  await writeTextAtomic(destination, output);
  const eligible = [...groups.keys()].sort().filter(country => groups.get(country).length >= perCountryCount);
  const format = limit => eligible.map(country => groups.get(country).slice(0, limit).map((entry, i) => `${entry}${i + 1}`).join('\n')).join('\n');
  await writeTextAtomic(allDestination, format(undefined));
  await writeTextAtomic(topDestination, format(perCountryCount));
  if (targetASNs.length) console.log(`目标 ASN IPv4 列表: ${JSON.stringify(asnEndpoints)}`);
  console.log(`已成功提取到 ${destination}`);
}

try {
  const requested = [];
  for (const arg of process.argv.slice(2)) {
    if (arg.startsWith('--outbound=')) {
      const value = arg.slice('--outbound='.length);
      if (!['ipv4', 'ipv6', 'all'].includes(value)) throw new Error('--outbound 必须为 ipv4、ipv6 或 all');
      outboundType = value;
    } else if (arg.startsWith('--asn=')) {
      const value = arg.slice('--asn='.length);
      targetASNs = value ? value.split(',').map(item => item.trim()) : [];
      if (targetASNs.some(item => !/^[1-9][0-9]*$/.test(item) || Number(item) > 2 ** 32 - 1)) throw new Error('--asn 必须为十进制 ASN 整数列表');
    } else if (arg.startsWith('-')) {
      throw new Error(`不支持的转换参数: ${arg}`);
    } else requested.push(arg);
  }
  const files = requested.length ? requested : (await fs.readdir(dir)).filter(file => file.endsWith('.csv') && file !== 'init.csv');
  if (!files.length) throw new Error('未找到待处理的 CSV 文件');
  const planned = files.flatMap(file => outputPaths(path.resolve(dir, file))).map(file => file.toLowerCase());
  if (new Set(planned).size !== planned.length) throw new Error('多个 CSV 的输出路径冲突，未开始转换');
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
