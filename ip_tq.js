// 该脚本用于从 CSV 文件中提取 IP 和端口信息，并根据国家分组输出到文本文件。
import fs from "node:fs";
import path from "node:path";
import url from "node:url";
//每个国家提前数量
const shu = 0;
// 是否过滤下载速度
const speed = true;
// 过滤下载速度下限，单位kb/s
const test = 0;
// 获取当前脚本路径
const __filename = url.fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// 提取列
const ip = "IP地址";
const port = "端口";
const speedtestresult = "下载速度";
const datacenter = "数据中心";

async function processCSVFiles() {
  try {
    // 获取所有 CSV 文件
    const files = fs
      .readdirSync(__dirname)
      .filter((file) => file.endsWith(".csv"));
    if (files.length === 0) {
      console.log("未找到 CSV 文件。");
      return;
    }

    console.log(`发现 ${files.length} 个 CSV 文件，开始处理...`);

    for (const file of files) {
      const csvFilePath = path.resolve(__dirname, file);
      const txtFilePath = path.resolve(__dirname, file.replace(".csv", ".txt"));

      console.log(`处理文件: ${file}`);
      await extractIpAndPort(csvFilePath, txtFilePath);
    }
  } catch (error) {
    console.error("处理文件时发生错误:", error.message);
  }
}

async function extractIpAndPort(csvFilePath, txtFilePath) {
  try {
    // 读取 CSV 文件内容
    console.log(`开始读取 CSV 文件...${csvFilePath}`);
    const data = await fs.promises.readFile(csvFilePath, "utf8");
    console.log("CSV 文件读取成功。");

    // 按行分割 CSV 内容
    const lines = data
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line); // 去掉空行
    if (lines.length < 2) {
      throw new Error("CSV 文件内容不足或格式不正确");
    }
    console.log("CSV 文件内容处理完成。");

    // 获取表头
    const headers = lines[0].split(",");
    const ipIndex = headers.indexOf(ip);
    const portIndex = headers.indexOf(port);
    const speedIndex = headers.indexOf(speedtestresult);
    const datacenterIndex = headers.indexOf(datacenter);

    if (ipIndex === -1 || portIndex === -1 || datacenterIndex === -1) {
      throw new Error(`CSV 文件缺少 ${ip}、${port} 或 ${datacenter} 列`);
    }
    console.log("CSV 文件列索引检查通过。");

    // 提取 IP 和端口
    console.log("开始提取 IP 和端口...");
    const jsonFilePath = path.resolve(__dirname, "locations.json");
    const jsonData = await fs.promises.readFile(jsonFilePath, "utf8");
    const locations = JSON.parse(jsonData);
    
    // 判断是否为IPv6地址
    const isIPv6 = (ip) => ip.includes(":");
    
    const ipEntries = lines
      .slice(1) // 去掉表头
      .map((line) => line.split(",")) // 按逗号分割每一行
      .filter(
        (fields) =>
          fields.length >
          Math.max(ipIndex, portIndex, speedIndex, datacenterIndex),
      ) // 确保有足够的列
      .filter((fields) => {
        if (speed) {
          const speedField = fields[speedIndex];
          if (speedField) {
            const speedfo = parseFloat(fields[speedIndex].replace(" kB/s", ""));
            return speedfo > test;
          }
        }
        return true;
      })
      .map((fields) => {
        let ip = fields[ipIndex];
        const port = fields[portIndex];
        const dc = fields[datacenterIndex];
        const location = locations.find((loc) => loc.iata === dc);
        const country = location
          ? location.emoji + location.country
          : "Unknown";
        
        // 如果是IPv6地址且没有括号，则添加括号
        if (isIPv6(ip) && !ip.startsWith("[")) {
          ip = `[${ip}]`;
        }
        
        console.log(`提取：${ip}:${port}#${country}`);
        return { entry: `${ip}:${port}#${country}`, country };
      });

    console.log(`IP 和端口提取完成。${ipEntries.length}`);

    const grouped = ipEntries.reduce((acc, { entry, country }) => {
      if (!acc[country]) {
        acc[country] = [];
      }
      if (shu === 0 || acc[country].length < shu) {
        acc[country].push(entry);
      }
      return acc;
    }, {});
    console.log("IP 和端口根据国家分组完成。");

    // 可选：对国家进行排序后拼接所有分组
    const result = Object.keys(grouped)
      .sort() // 对国家名称进行排序
      .map((country, index) => {
        return grouped[country]
          .map((entry, index) => `${entry}${index + 1}`) // 添加序号
          .join("\n");
      })
      .join("\n");
    const countries = Object.keys(grouped).sort().join("、");
    console.log(`提取国家: ${countries}`);
    await fs.promises.writeFile(txtFilePath, result, "utf8");
    console.log(`已成功提取到 ${txtFilePath}`);
  } catch (error) {
    console.error("处理文件时发生错误:", error.message);
  }
}
async function getDatacenterMap() {
  const FilePath = path.resolve(__dirname, "locations.json");
  const data = await fs.promises.readFile(FilePath, "utf8");
}
await processCSVFiles();
