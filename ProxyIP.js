// 该脚本从 CSV 文件中提取代理 IP 和端口，并验证其有效性，将结果分类保存到不同的文件中。

import path from "node:path";
import url from "node:url";
import fetch from "node-fetch";
import { readCSV } from './lib/csv.js';
import { writeTextAtomic } from './lib/io.js';
import { parseIPPort, formatIPPort } from './lib/address.js';

// 获取当前脚本路径
const __filename = url.fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// 文件路径
const csvFilePath = path.resolve(__dirname, "init.csv");
const proxyFilePath = path.resolve(__dirname, "proxy_ips.txt");
const reverseFilePath = path.resolve(__dirname, "reverse_ips.txt");

// CSV列名
const ipcom = "ip";
const portcom = "port";

async function extractIpAndPort() {
  try {
    // Parse quoted fields before validation; a comma inside a field is not a separator.
    const { rows } = await readCSV(csvFilePath, [ipcom, portcom]);
    const resultSet = new Set();
    for (const { ip, port } of rows) {
      const address = parseIPPort(ip, port);
      if (!address) {
        console.log(`无效 IP 或端口: ${ip}:${port}`);
        continue;
      }
      resultSet.add(`${address.ip} ${address.port}`);
    }

    console.log(`提取到 ${resultSet.size} 个代理IP，开始验证...`);

    // 顺序验证 IP
    const proxyList = [];
    const reverseList = [];

    for (const entry of resultSet) {
      const result = await validateProxy(entry);
      if (result) {
        if (result.type === "proxyip") {
          proxyList.push(result.entry);
        } else if (result.type === "reverse") {
          reverseList.push(result.entry);
        }
      }
    }

    await writeTextAtomic(proxyFilePath, proxyList.join("\n"));
    await writeTextAtomic(reverseFilePath, reverseList.join("\n"));

    console.log(
      `已成功提取到 ${proxyList.length} 个有效 ProxyIP，结果已写入 ${proxyFilePath}`,
    );
    console.log(
      `已成功提取到 ${reverseList.length} 个有效 反代IP，结果已写入 ${reverseFilePath}`,
    );
  } catch (error) {
    console.error("处理文件时发生错误:", error.message);
    process.exitCode = 1;
  }
}

// 验证代理
async function validateProxy(entry) {
  const [ip, port] = entry.split(" ");
  const endpoint = formatIPPort(ip, port);
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 10000);
  try {
    const response = await fetch(
      `https://proxyip.edtunnel.best/api?host=speedtest.cloudflare.com&ip=${ip}&port=${port}&tls=true`,
      {
        signal: controller.signal,
        headers: {
          "User-Agent":
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
          Accept: "application/json, text/plain, */*",
          Referer: "https://proxyip.edtunnel.best/",
        },
      },
    );

    if (!response.ok) {
      response.body?.destroy?.();
      throw new Error(`HTTP ${response.status}`);
    }
    const resultData = await response.json();

    if (resultData && resultData.proxyip) {
      console.log(`发现有效 ProxyIP: ${endpoint}`);
      return { type: "proxyip", entry: endpoint };
    }

    if (resultData && resultData.reverse) {
      console.log(`发现有效 反代IP: ${endpoint}`);
      return { type: "reverse", entry: endpoint };
    }
  } catch (error) {
    throw new Error(`请求失败: ${endpoint} -> ${error.message}`);
  } finally {
    clearTimeout(timer);
  }
  return null;
}

await extractIpAndPort();
