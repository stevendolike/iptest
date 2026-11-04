import fs from 'node:fs/promises';
import { parse } from 'csv-parse/sync';

// Decode CSV records before selecting columns; quoted newlines/commas are data.
export async function readCSV(file, required = []) {
  const records = parse(await fs.readFile(file, 'utf8'), {
    bom: true,
    trim: true,
    skip_empty_lines: true,
  });
  if (records.length === 0) throw new Error(`CSV 文件为空: ${file}`);
  const headers = records[0].map(value => value.trim());
  if (new Set(headers).size !== headers.length) throw new Error('CSV 文件包含重复列名');
  for (const name of required) {
    if (!headers.includes(name)) throw new Error(`CSV 文件缺少 ${name} 列`);
  }
  const rows = records.slice(1).map(fields => Object.fromEntries(headers.map((name, index) => [name, fields[index]])));
  return { headers, rows };
}
