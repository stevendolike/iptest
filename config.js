// 该脚本用于读取 config.json 文件，提取其中的 ip 和 port 信息，并将其保存到 ip.txt 文件中
import fs from "fs";

// 读取 config.json 文件
fs.readFile("config.json", "utf8", (err, data) => {
  if (err) {
    console.error("读取文件失败:", err);
    return;
  }

  try {
    // 解析 JSON 数据并提取 ip 和 port
    const uniqueEntries = new Set(
      data
        .split("\n")
        .filter((line) => line.trim() !== "") // 过滤空行
        .map(JSON.parse) // 解析 JSON
        .map(({ ip, port }) => `${ip} ${port}`), // 格式化为 "ip port"
    );
    // 将数据转换为文本格式
    const textData = Array.from(uniqueEntries).join("\n");

    // 将提取后的数据保存到新文件
    fs.writeFile("ip.txt", textData, (err) => {
      if (err) {
        console.error("写入文件失败:", err);
        return;
      }
      console.log("文件已成功保存为 ip.txt");
    });
  } catch (parseErr) {
    console.error("解析 JSON 失败:", parseErr);
  }
});
