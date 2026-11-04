# Cloudflare IP 测试工具

一个用于测试 Cloudflare IP 地址延迟和下载速度的 Go 语言工具。支持并发检测、延迟过滤、国家过滤、下载速度过滤、快速模式，并将结果导出为 UTF-8-BOM 编码的 CSV 文件。

## 更新事项

### 上游整合与自有功能

整合 `Kwisma/iptest@86b24ecfcb2e01e96a9692b69d2dbdbffda0fc11` 的出站信息、ASN／国家数据库、IPS 检测、出站类型过滤及 all／top5 输出。保留本仓库的国家／速度过滤、快速模式、中文地理字段、UTF-8 BOM、完整 `ip_tq.txt` 管线及已有正确性／发布修正。这是一次性整合，不新增 cron 或自动同步 workflow。

新增元数据查询默认关闭：`-ips` 明确启用 API 查询，`-download-db` 明确允许下载缺失数据库。可指定自己的代理／镜像；原有 `locations.json` 获取逻辑不变。

### 正确性与管线修正

- 同步并发结果及进度，避免漏写 IP。
- 下载最多采样 10 秒；快速模式在第 5 秒检查速度，低于 100 kB/s 时提前停止并保留已测速度。需要剔除低速结果时同时设置 `-speedLimit`。
- HTTP 错误响应不参与有效 IP 探测或测速；有国家 allowlist 时不接受未知国家。
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
go mod download
go build -o iptest iptest.go
```

请保留并使用仓库的 `go.mod`／`go.sum`；带 ASN／国家查询的版本不再是仅依赖标准库的单文件程序。35 个组合表示编译覆盖，不代表已在每个平台完成运行测试。

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
|`-ips`|`false`|按待测候选 IP 查询 IPS 类型；默认不调用该 API。|
|`-ipapi-url`|`https://api.ipapi.is/`|IPS API 地址，可指定自家 HTTP/HTTPS 代理。|
|`-asn-db`|`GeoLite2-ASN.mmdb`|本地 ASN 数据库；空值禁用此查询。|
|`-country-db`|`GeoLite2-Country.mmdb`|本地出站国家数据库；空值禁用此查询。|
|`-download-db`|`false`|明确允许下载缺失数据库；已有文件会验证，不会静默替换损坏文件。|
|`-asn-url`|`https://jsd.onmicrosoft.cn/gh/seketiti/GeoLiet2@release/GeoLite2-ASN.mmdb`|ASN 下载地址，可指定自家镜像。|
|`-country-url`|`https://jsd.onmicrosoft.cn/gh/seketiti/GeoLiet2@release/GeoLite2-Country.mmdb`|国家数据库下载地址，可指定自家镜像。|

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

另外保留上游新增字段：`出站IP`、`出站IP类型`、`出站IP位置`、`ASN号码`、`ASN组织`、`IPS类型`、`访问协议`、`TLS版本`、`SNI`、`HTTP版本`、`WARP`、`Gateway`、`RBI`、`密钥交换`、`时间戳`。

- `IP地址`／`端口` 是实际探测的候选端点；`出站IP` 来自该端点返回的 trace。
- ASN／出站国家按 trace 的出站 IP 查询本地数据库；IPS API 则查询候选 IP。两种身份不混用，也不替换原有数据中心地理字段。
- 缺少或无效的出站 IP 不会阻止已启用的候选 IP IPS 查询；ASN／出站国家仍保持缺失值，不改用候选 IP 冒充出站信息。
- IPS 标签是根据 API 字段推断的提示，不是家宽属性的独立证明。明确的 `is_datacenter=true` 会阻止 ISP／ISP 被误标为家宽；API 错误、非法或空回应为 `N/A`。
- 未取得 ASN 记录时 `ASN号码` 为 `0`，其他缺失元数据为空；不要把这些占位值当作真实查询结果。

## 注意事项

1. **文件权限**：首次运行会自动下载 `locations.json`，请确保有写入权限。
2. **网络连接**：需能访问 `https://proxy.api.030101.xyz/locations-adw.pages.dev/` 以下载地理位置数据。
3. **Linux 系统**：程序沿用父进程的文件描述符上限，不会在子 shell 中假装提升。高并发运行前请在同一个启动 shell 执行 `ulimit -n 10000`（受当前硬上限限制）；用 systemd 等服务启动时设置该服务的 `LimitNOFILE`。不需要为了测速默认使用 `sudo`。
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

先准备 `init.csv`，支持本仓库的 `ip`／`port` 或上游的 `IP地址`／`端口`（也支持 `端口号`）表头。支持合法 quoted CSV；重复 IP／端口组合会去重。

```csv
ip,port
1.1.1.1,443
```

完整管线为 `init.csv → ip.txt → ip_tq.csv → ip_tq.txt`，并额外生成 `ip_all.txt`／`ip_top5.txt`。可一次运行：

```bash
bash main.sh -countries="CN,US" -speedLimit=1000 -fast
```

管线允许追加表中列出的 Go 测试参数，但保留 `-file`、`-outfile`，避免后续步骤误用旧文件。`-countries CN,US` 和 `-countries=CN,US` 均支持；布尔值使用 `-tls=false` 这类等号格式。位置参数、未知参数和缺少值会在管线开始前拒绝；单独运行 Go 也不接受位置参数，但可自由指定输入／输出。

值参数后紧跟另一个旗标会立即作为缺少值拒绝。若值本身以 `-` 开头（例如本地数据库文件名），请使用 `-asn-db=-cache.mmdb` 这类明确的等号格式。

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

`ip_tq.js` 可明确指定一个或多个 Go 结果 CSV；未指定时扫描当前脚本目录的 CSV，但跳过原始 `init.csv`。缺列、无效 CSV 或任一数据记录的 IP／端口无效都会使该文件转换失败，不会覆盖上一次完整 TXT；低速过滤不绕过地址验证。有效 IPv6 输出为 `[IPv6]:端口`，端口规范化为十进制整数。

结果表头兼容 `端口`／`端口号`、`源IP位置`／`IP位置`。有源国家代码时按该代码分组，缺少代码时才使用数据中心位置；未知代码不会假装成其他国家。

```bash
node ip_tq.js ip_tq.csv --outbound=ipv6
node ip_tq.js ip_tq.csv --outbound=all --asn=64500,64501
```

- `--outbound=ipv4|ipv6|all`：按明确的出站 IP 类型过滤所有保存结果，默认 `all`。记录未提供类型时不会被当成 IPv4／IPv6。
- `--asn=整数,整数`：只改变控制台的目标 ASN IPv4 出站列表，不额外过滤保存文件。这里的 IPv4 指出站类型，输出仍是经过验证的候选端点。上例 ASN 为文档保留编号，不是生产数据。
- 原有 `<输入名>.txt` 保留全部符合过滤条件的记录。`ip_tq.csv` 的附加输出为 `ip_all.txt`／`ip_top5.txt`；其他文件为 `<输入名>_all.txt`／`<输入名>_top5.txt`。
- 附加输出默认只收录至少 5 条记录的国家；all 保留该国家全部记录，top5 保留 CSV 顺序的前 5 条。可在 `ip_tq.js` 调整 `perCountryCount`；Go 开启测速时已按下载速度降序输出，否则按延迟升序输出。
- 保留自有默认：转换器 `shu=0` 不限制完整 TXT 的每国数量，`test=0` 过滤非正速度，而不是引入上游的 100 kB/s 默认门槛。
- 所有输出路径在转换前检查，冲突或非法选项不会开始写入。各文件独立原子替换，不是多文件事务；后续文件写入失败不会回滚已经成功替换的文件。

## 自动验证及发布

```bash
npm ci --ignore-scripts
bash tools/check.sh
```

测试需要 Go、Node.js、Bash 和支持 Go race detector 的 C 编译器（Linux 可用 GCC）。测试仅使用本地 HTTP 服务／明确 mock，不会扫描公网 IP。

`Verify` workflow 在 PR、`main` 和 `fix/**` 分支执行。Release workflow 先解析并验证 tag，再测试和构建。手动运行时必须填写尚未使用的新 tag；既有 tag 会在解析阶段明确拒绝，不尝试用新测试工具重建历史版本。推送 tag 时必须与本次事件的 commit 一致。所有平台构建和发布使用同一 commit；同名 tag 的 workflow 串行执行。发布前以不强制覆盖的 Git push 原子保留新 tag，并重新读取远端确认 commit，冲突或读写失败立即停止；不会移动既有 tag，也不覆盖同名 release 资产。

## js格式化

安装工具 
```bash
npm install --global prettier
```

格式化文件

```bash
prettier --write ip_init.js
```
