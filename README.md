# Cloudflare IP 测试工具

一个用于测试 Cloudflare IP 地址延迟和下载速度的 Go 语言工具。支持并发检测、延迟过滤、国家过滤、下载速度过滤、快速模式，并将结果导出为 UTF-8-BOM 编码的 CSV 文件。

## 更新事项

### 正确性与管线修正

- 同步并发结果及进度，避免漏写 IP。
- 下载最多采样 10 秒；快速模式在第 5 秒检查速度，低于 100 kB/s 时提前停止并保留已测速度。需要剔除低速结果时同时设置 `-speedLimit`。
- HTTP 错误响应不参与测速；有国家 allowlist 时不接受未知国家。
- 参数、IP、端口及输入格式会验证；真实读写错误返回非零退出码。
- 成功的零结果运行输出当次空 CSV（保留表头）／空 TXT；写入失败保留上一份完整文件，批量管线立即停止。
- CSV 支持引号、逗号、多行字段与 UTF-8 BOM；JSON 转换支持数组、单个对象及 JSON Lines。

- **2025-08-28**：
  - 输出文件编码变更为 UTF-8-BOM，确保 Windows 环境下（如 Excel）中文显示无乱码。
  - 恢复 CSV 输出字段：`地区(中文)`、`国家`、`城市(中文)`、`国旗`，与早期版本一致。
  - 优化快速模式（`-fast`），确保测速稳定运行，前 5 秒速度未达 100 kB/s 时终止测速。
  - 明确国家过滤参数（`-countries`）在 Windows 环境下建议使用引号，如 `-countries="CN,US"`。

## 功能特性

- **并发测试**：支持自定义最大并发线程数。
- **延迟检测**：可设置延迟阈值过滤高延迟 IP。
- **国家过滤**：可指定国家列表，仅测试和记录特定国家的 IP。
- **下载测速**：支持多线程下载速度测试，可设置速度阈值过滤低速 IP。
- **快速模式**：测速时若前 5 秒速度未达 100 kB/s，则提前终止测速。
- **地理位置信息**：自动获取数据中心和地理位置信息（支持中英文）。
- **结果导出**：生成 UTF-8-BOM 编码的 CSV 文件，包含 IP、端口、延迟、下载速度等详细信息。
- **TLS 支持**：可切换 HTTP/HTTPS 协议进行测试。

---

## 安装

### 编译步骤

1. 克隆仓库或下载代码：

```bash
git clone https://github.com/stevendolike/iptest.git
cd iptest
```

2. 编译程序

普通编译需要 Go；批量脚本还需要 Node.js。CI 使用 Go 1.20.14 和 Node.js 24。发布构建固定 Go 1.20.14，以保留现有 35 个平台组合（包括旧版 Windows ARM）。

```bash
go build -o iptest iptest.go
```

## 使用方法

点击 [Releases](https://github.com/stevendolike/iptest/releases) 下载执行文件

### 基本命令

```bash
./iptest -file=ip.txt -outfile=result.csv
```

### 参数说明

|参数|默认值|说明|
|------|------|------|
|`-file`|`ip.txt`|IP 地址文件路径，格式为每行 `IP 端口`（例如 `1.1.1.1 443`）。|
|`-outfile`|`ip.csv`|输出 CSV 文件路径（使用 UTF-8-BOM 编码）。|
|`-max`|`100`|最大并发协程数，必须大于 0。实际 worker 数不会超过候选数量。|
|`-speedtest`|`5`|下载测速协程数量，必须非负，设为 `0` 禁用测速。|
|`-url`|`speed.cloudflare.com/__down?bytes=500000000`|可填写主机／路径或完整 URL；协议须与 `-tls` 一致，不允许 URL 凭据。|
|`-tls`|`true`|是否启用 TLS（`true` 为 HTTPS，`false` 为 HTTP）。|
|`-delay`|`0`|延迟阈值（毫秒），超过此值的 IP 将被过滤（设为 `0` 禁用过滤）。|
|`-countries`|``|国家过滤列表（逗号分隔，例如：`CN,US,JP`），默认空（不过滤）。Windows 用户建议使用引号，如：`"CN,US"`。|
|`-speedLimit`|`0`|下载速度阈值（kB/s），低于此值的 IP 将被过滤（设为 `0` 禁用过滤）。|
|`-fast`|`false`|启用快速模式，测速时若前 5 秒速度未达 100 kB/s 则终止。|

### 示例

1. 基础测试（仅延迟）：

```bash
./iptest -file=ip.txt -outfile=fast-ips.csv -max=200 -speedtest=0
```

2. 启用下载测速并过滤国家：

```bash
./iptest -file=ip.txt -outfile=result.csv -speedtest=10 -tls=true -countries="CN,US"
```

3. 过滤高延迟和低速 IP，并启用快速模式：

```bash
./iptest -file=ip.txt -outfile=result.csv -delay=150 -speedLimit=1000 -fast
```

## 输入文件格式

- 文件需为纯文本格式，每行包含一个 IP 和端口，例如：

支持 IPv4／IPv6。端口必须是十进制整数 1–65535；无效行跳过并提示，不会传给 TCP 拨号。

```txt
1.1.1.1 443
2.2.2.2 2053
```

## 输出文件说明

CSV 文件使用 UTF-8-BOM 编码，包含以下字段：

- `IP地址`、`端口`、`TLS`、`数据中心`、`源IP位置`、`地区`、`城市`、`地区(中文)`、`国家`、`城市(中文)`、`国旗`、`网络延迟`、（若启用测速）`下载速度`。

## 注意事项

1. **文件权限**：首次运行会自动下载 `locations.json`，请确保有写入权限。
2. **网络连接**：需能访问 `https://proxy.api.030101.xyz/locations-adw.pages.dev/` 以下载地理位置数据。
3. **Linux 系统**：可能需要 `sudo` 权限提升文件描述符上限。
4. **测速文件**：默认请求 Cloudflare 的 500MB 文件，下载采样最多 10 秒；高速连接可能在此之前完成文件下载，请留意流量。
5. **国家过滤**：输入的国家代码需与 `locations.json` 中的 `cca2` 字段一致（如 `CN`、`US`）。Windows 用户建议使用引号，如 `-countries="CN,US"`。
6. **速度过滤**：仅在启用测速（`-speedtest > 0`）时生效，低于 `-speedLimit` 的 IP 不会记录到输出文件中。
7. **快速模式**：仅在启用测速（`-speedtest > 0`）时生效，第 5 秒速度低于 100 kB/s 时提前停止。高速连接继续采样至文件结束或 10 秒总期限，不会因第 5 秒到达而误记为 0。
8. **文件编码**：输出的 CSV 文件使用 UTF-8-BOM 编码，确保 Windows 应用（如 Excel）正确显示中文。

## 许可证

本项目采用 **GNU General Public License v3.0** 开源许可证。
您可以在以下链接查看完整协议内容：
[https://www.gnu.org/licenses/gpl-3.0.html](https://www.gnu.org/licenses/gpl-3.0.html)

### 主要条款

- **自由使用**：允许自由使用、修改和分发本软件。
- **开源要求**：修改后的衍生作品必须以相同许可证开源。
- **版权声明**：所有副本必须包含原始版权声明和许可证声明。
- **免责条款**：本软件不提供任何担保，作者不承担使用风险。

## 致谢

代码基于[白嫖哥](https://github.com/XIU2)源码修改：[https://t.me/CF_NAT/38811](https://t.me/CF_NAT/38811)

`delay` 添加参考: [https://github.com/yutian81/IP-SpeedTest](https://github.com/yutian81/IP-SpeedTest)

特别感谢 Mingyu 提供的代理服务：[https://proxy.api.030101.xyz/](https://proxy.api.030101.xyz/)

## 批量获取ip

更换初始数据即可开始

## 安装依赖

```bash
apt install golang-go
npm ci --ignore-scripts
```

先准备 `init.csv`，必须包含 `ip`、`port` 表头。支持合法 quoted CSV；重复 IP／端口组合会去重。

```csv
ip,port
1.1.1.1,443
```

完整管线为 `init.csv → ip.txt → ip_tq.csv → ip_tq.txt`。可一次运行：

```bash
bash main.sh -countries="CN,US" -speedLimit=1000 -fast
```

管线允许追加 Go 测试参数，但保留 `-file`、`-outfile`，避免后续步骤误用旧文件。单独运行 Go 时可自由指定输入／输出。

## 运行

整理并去重

```bash
node ip_init.js
```

测试有效ip

- 参数说明

- `file` 默认输入文件 `ip.txt` IP地址文件名称,格式为 ip port ,就是IP和端口之间用空格隔开
- `outfile` 默认输出文件命名称 `ip.csv`（使用 UTF-8-BOM 编码）
- `max` 默认并发 `100` 并发请求最大协程数
- `speedtest` 默认并发 `5` 下载测速协程数量,设为0禁用测速
- `url` 默认测速地址 `speed.cloudflare.com/__down?bytes=500000000` 测速文件地址
- `tls` 默认参数 `true` 是否启用TLS
- `countries` 默认空，国家过滤列表（逗号分隔，例如：`CN,US,JP`），不过滤时不设置，Windows 用户建议使用引号，如 `"CN,US"`
- `speedLimit` 默认 `0`，下载速度阈值（kB/s），低于此值的 IP 将被过滤（设为 `0` 禁用过滤）
- `fast` 默认 `false`，启用快速模式，测速时若前 5 秒速度未达 100 kB/s 则终止

```bash
go run iptest.go -file ip.txt -outfile ip_tq.csv -countries="CN,US" -speedLimit=1000 -fast
```

提取有速度的ip并整理为指定格式

```bash
node ip_tq.js ip_tq.csv
```

## 格式转换

本仓库不包含 XLSX 转换／CSV 合并工具。请自行准备上述 UTF-8 `init.csv`，不要调用未提供的 `csv_hb.js`。

若源数据为 JSON，可将对象数组、单个对象或 JSON Lines 存入 `config.json`，再运行 `node config.js` 产生经过验证的 `ip.txt`；不支持的结构会失败，不会输出 `undefined`。

```json
[
  {"ip": "1.1.1.1", "port": 443},
  {"ip": "::1", "port": 8443}
]
```

`ip_tq.js` 可明确指定一个或多个 Go 结果 CSV；未指定时扫描当前脚本目录的 CSV，但跳过原始 `init.csv`。缺列／无效 CSV 返回失败码，不会覆盖上一次完整 TXT。

## 自动验证及发布

```bash
npm ci --ignore-scripts
bash tools/check.sh
```

测试需要 Go、Node.js、Bash 和支持 Go race detector 的 C 编译器（Linux 可用 GCC）。测试仅使用本地 HTTP 服务／明确 mock，不会扫描公网 IP。

`Verify` workflow 在 PR、`main` 和 `fix/**` 分支执行。Release workflow 先解析并验证 tag，再测试和构建：既有 tag 使用其实际 commit；新 tag 显式绑定本次选择的 commit。所有平台构建及 release 的 `target_commitish` 使用同一解析结果，不会移动既有 tag。

## js格式化

安装工具 
```bash
npm install --global prettier
```

格式化文件

```bash
prettier --write ip_init.js
```
