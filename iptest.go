package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oschwald/geoip2-golang"
)

const (
	requestURL  = "speed.cloudflare.com/cdn-cgi/trace" // 请求trace URL
	timeout     = 1 * time.Second                      // 超时时间
	maxDuration = 2 * time.Second                      // 最大持续时间
)

var (
	File          = flag.String("file", "ip.txt", "IP地址文件名称,格式为 ip port ,就是IP和端口之间用空格隔开")       // IP地址文件名称
	outFile       = flag.String("outfile", "ip.csv", "输出文件名称")                                  // 输出文件名称
	maxThreads    = flag.Int("max", 100, "并发请求最大协程数")                                           // 最大协程数
	speedTest     = flag.Int("speedtest", 5, "下载测速协程数量,设为0禁用测速")                                // 下载测速协程数量
	speedTestURL  = flag.String("url", "speed.cloudflare.com/__down?bytes=500000000", "测速文件地址") // 测速文件地址
	enableTLS     = flag.Bool("tls", true, "是否启用TLS")                                           // TLS是否启用
	delay         = flag.Int("delay", 0, "延迟阈值(ms)，默认为0禁用延迟过滤")                                 // 默认0，禁用过滤
	countries     = flag.String("countries", "", "国家过滤列表（逗号分隔，例如：CN,US,JP），默认空（不过滤）")           // 国家过滤参数
	speedLimit    = flag.Int("speedLimit", 0, "下载速度阈值(kB/s)，默认为0（不过滤）")                         // 速度阈值参数
	fast          = flag.Bool("fast", false, "启用快速模式，测速时若前5秒速度未超过100kB/s则终止")                   // 快速模式参数
	enableIPAPI   = flag.Bool("ips", false, "启用 IPS 类型检测；按候选 IP 调用 ipapi.is，默认关闭")
	ipAPIURL      = flag.String("ipapi-url", "https://api.ipapi.is/", "IPS API 地址，可指定自己的 HTTP/HTTPS 代理")
	asnDBFile     = flag.String("asn-db", "GeoLite2-ASN.mmdb", "本地 ASN 数据库路径，空值禁用")
	countryDBFile = flag.String("country-db", "GeoLite2-Country.mmdb", "本地出站国家数据库路径，空值禁用")
	downloadDB    = flag.Bool("download-db", false, "明确允许下载缺失的 GeoLite2 数据库，默认关闭")
	asnDBURL      = flag.String("asn-url", "https://jsd.onmicrosoft.cn/gh/seketiti/GeoLiet2@release/GeoLite2-ASN.mmdb", "ASN 数据库下载地址，可指定自家镜像")
	countryDBURL  = flag.String("country-url", "https://jsd.onmicrosoft.cn/gh/seketiti/GeoLiet2@release/GeoLite2-Country.mmdb", "Country 数据库下载地址，可指定自家镜像")
)

type result struct {
	ip                           string        // IP地址
	port                         int           // 端口
	dataCenter                   string        // 数据中心
	locCode                      string        // 源IP位置
	region                       string        // 地区
	city                         string        // 城市
	region_zh                    string        // 地区(中文)
	country                      string        // 国家
	city_zh                      string        // 城市(中文)
	emoji                        string        // 国旗
	latency                      string        // 延迟
	tcpDuration                  time.Duration // TCP请求延迟
	outboundIP                   string
	ipType                       string
	countryName                  string
	visitScheme                  string
	tlsVersion                   string
	sni                          string
	httpVersion                  string
	warp                         string
	gateway                      string
	rbi                          string
	kex                          string
	timestamp                    string
	autonomousSystemNumber       uint
	autonomousSystemOrganization string
	ipsType                      string
}

type speedtestresult struct {
	result
	downloadSpeed float64 // 下载速度
}

type location struct {
	Iata      string  `json:"iata"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Cca2      string  `json:"cca2"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Region_zh string  `json:"region_zh"`
	Country   string  `json:"country"`
	City_zh   string  `json:"city_zh"`
	Emoji     string  `json:"emoji"`
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if flag.NArg() != 0 {
		return fmt.Errorf("不支持位置参数: %s；请使用命名参数", strings.Join(flag.Args(), " "))
	}
	if *maxThreads <= 0 {
		return fmt.Errorf("-max 必须大于 0")
	}
	if *speedTest < 0 {
		return fmt.Errorf("-speedtest 不能小于 0")
	}
	if *delay < 0 || *speedLimit < 0 {
		return fmt.Errorf("延迟和速度阈值不能小于 0")
	}
	if *File == "" || *outFile == "" {
		return fmt.Errorf("输入及输出文件名不能为空")
	}
	if _, err := downloadURL(*speedTestURL, *enableTLS); err != nil {
		return err
	}
	if *enableIPAPI {
		if _, err := metadataURL(*ipAPIURL); err != nil {
			return fmt.Errorf("-ipapi-url: %w", err)
		}
	}
	if *countries != "" {
		for _, code := range strings.Split(*countries, ",") {
			if !regexp.MustCompile(`^[A-Za-z]{2}$`).MatchString(strings.TrimSpace(code)) {
				return fmt.Errorf("国家代码必须为两个英文字母")
			}
		}
	}
	var validCount int32 // 有效IP计数器

	startTime := time.Now()
	if runtime.GOOS == "linux" {
		fmt.Println("Linux 文件描述符上限沿用父进程设置；高并发前请在启动 shell 设置 ulimit -n 10000（受硬上限限制）。")
	}

	// 解析国家过滤列表
	var countryFilter []string
	if *countries != "" {
		countryFilter = strings.Split(*countries, ",")
		for i, country := range countryFilter {
			countryFilter[i] = strings.TrimSpace(strings.ToUpper(country)) // 转换为大写以统一格式
		}
	}

	locations, err := loadLocations("locations.json")
	if err != nil {
		return err
	}

	locationMap := make(map[string]location)
	for _, loc := range locations {
		locationMap[loc.Iata] = loc
	}

	ips, err := readIPs(*File)
	if err != nil {
		return fmt.Errorf("无法从文件中读取 IP: %w", err)
	}
	databases := geoDatabases{}
	if len(ips) > 0 {
		databases.asn, err = loadGeoDatabase(*asnDBFile, "ASN", *asnDBURL)
		if err != nil {
			return fmt.Errorf("ASN 数据库: %w", err)
		}
		if databases.asn != nil {
			defer databases.asn.Close()
		}
		databases.country, err = loadGeoDatabase(*countryDBFile, "Country", *countryDBURL)
		if err != nil {
			return fmt.Errorf("Country 数据库: %w", err)
		}
		if databases.country != nil {
			defer databases.country.Close()
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(ips))
	resultChan := make(chan result, len(ips))
	concurrency := *maxThreads
	if concurrency > len(ips) {
		concurrency = len(ips)
	}
	thread := make(chan struct{}, concurrency)

	var count int
	var resultMu sync.Mutex // Protect the shared results and progress counters.
	total := len(ips)

	for _, ip := range ips {
		thread <- struct{}{}
		go func(ip string) {
			defer func() {
				<-thread
				resultMu.Lock()
				count++
				percentage := float64(count) / float64(total) * 100
				fmt.Printf("已完成: %d 总数: %d 已完成: %.2f%%\r", count, total, percentage)
				if count == total {
					fmt.Printf("已完成: %d 总数: %d 已完成: %.2f%%\n", count, total, percentage)
				}
				resultMu.Unlock()
				wg.Done()
			}()

			parts := strings.Fields(ip)
			if len(parts) != 2 {
				fmt.Printf("IP地址格式错误: %s\n", ip)
				return
			}
			ipAddr := parts[0]
			portStr := parts[1]

			port, err := strconv.Atoi(portStr)
			if err != nil {
				fmt.Printf("端口格式错误: %s\n", portStr)
				return
			}

			dialer := &net.Dialer{
				Timeout:   timeout,
				KeepAlive: 0,
			}
			start := time.Now()
			conn, err := dialer.Dial("tcp", net.JoinHostPort(ipAddr, strconv.Itoa(port)))
			if err != nil {
				return
			}
			defer conn.Close()

			tcpDuration := time.Since(start)
			if *delay > 0 && tcpDuration.Milliseconds() > int64(*delay) {
				return // 超过延迟阈值直接返回
			}

			start = time.Now()
			client := http.Client{
				Transport: &http.Transport{
					Dial: func(network, addr string) (net.Conn, error) {
						return conn, nil
					},
				},
				Timeout: timeout,
			}

			var protocol string
			if *enableTLS {
				protocol = "https://"
			} else {
				protocol = "http://"
			}
			requestURL := protocol + requestURL

			req, _ := http.NewRequest("GET", requestURL, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0")
			req.Close = true
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return // Error/interception bodies must not qualify as valid trace data.
			}

			duration := time.Since(start)
			if duration > maxDuration {
				return
			}

			buf := &bytes.Buffer{}
			timeout := time.After(maxDuration)
			done := make(chan bool)
			errChan := make(chan error)
			go func() {
				_, err := io.Copy(buf, resp.Body)
				done <- true
				errChan <- err
				if err != nil {
					return
				}
			}()
			select {
			case <-done:
			case <-timeout:
				return
			}

			body := buf
			err = <-errChan
			if err != nil {
				return
			}
			if strings.Contains(body.String(), "uag=Mozilla/5.0") {
				if matches := regexp.MustCompile(`colo=([A-Z]+)[\s\S]*?loc=([A-Z]+)`).FindStringSubmatch(body.String()); len(matches) > 2 {
					dataCenter := matches[1]
					locCode := matches[2]
					loc, ok := locationMap[dataCenter]
					if !ok {
						if len(countryFilter) > 0 {
							return // Unknown country cannot satisfy an explicit allowlist.
						}
						atomic.AddInt32(&validCount, 1)
						fmt.Printf("发现有效IP %s 端口 %d 位置信息未知 延迟 %d 毫秒\n", ipAddr, port, tcpDuration.Milliseconds())
						resultChan <- makeTraceResult(ipAddr, port, dataCenter, locCode, tcpDuration, location{}, body.String(), databases)
						return
					}

					// 国家过滤逻辑
					if len(countryFilter) > 0 {
						// 如果国家不在过滤列表中，跳过
						if !contains(countryFilter, strings.ToUpper(loc.Cca2)) {
							fmt.Printf("IP %s 端口 %d 国家 %s 不在过滤列表中，跳过\n", ipAddr, port, loc.Cca2)
							return
						}
					}

					// 记录通过延迟检查和国家过滤的有效IP
					atomic.AddInt32(&validCount, 1)
					fmt.Printf("发现有效IP %s 端口 %d 位置信息 %s 国家 %s 延迟 %d 毫秒\n", ipAddr, port, loc.City_zh, loc.Cca2, tcpDuration.Milliseconds())
					resultChan <- makeTraceResult(ipAddr, port, dataCenter, locCode, tcpDuration, loc, body.String(), databases)
				}
			}
		}(ip)
	}

	wg.Wait()
	close(resultChan)

	if len(resultChan) == 0 {
		fmt.Print("\033[2J")
		fmt.Println("没有发现有效的IP")
	}

	var results []speedtestresult
	if *speedTest > 0 {
		fmt.Printf("找到符合条件的ip 共%d个\n", atomic.LoadInt32(&validCount))
		fmt.Printf("开始测速\n")
		var wg2 sync.WaitGroup
		workers := *speedTest
		if workers > *maxThreads {
			workers = *maxThreads
		}
		if workers > len(resultChan) {
			workers = len(resultChan)
		}
		wg2.Add(workers)
		count = 0
		total := len(resultChan)
		results = []speedtestresult{}
		for i := 0; i < workers; i++ {
			thread <- struct{}{}
			go func() {
				defer func() {
					<-thread
					wg2.Done()
				}()
				for res := range resultChan {
					downloadSpeed := getDownloadSpeed(res.ip, res.port)
					keep := downloadSpeed > 0 && (*speedLimit == 0 || downloadSpeed >= float64(*speedLimit))
					if !keep {
						fmt.Printf("IP %s 端口 %d 测速无效或低于速度阈值，跳过\n", res.ip, res.port)
					}
					resultMu.Lock()
					if keep {
						results = append(results, speedtestresult{result: res, downloadSpeed: downloadSpeed})
					}

					count++
					percentage := float64(count) / float64(total) * 100
					fmt.Printf("已完成: %.2f%%\r", percentage)
					if count == total {
						fmt.Printf("已完成: %.2f%%\n", percentage)
					}
					resultMu.Unlock()
				}
			}()
		}
		wg2.Wait()
	} else {
		for res := range resultChan {
			results = append(results, speedtestresult{result: res})
		}
	}

	if *speedTest > 0 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].downloadSpeed > results[j].downloadSpeed
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].result.tcpDuration < results[j].result.tcpDuration
		})
	}

	file, err := os.CreateTemp(filepath.Dir(*outFile), ".iptest-*.csv")
	if err != nil {
		return fmt.Errorf("无法创建输出临时文件: %w", err)
	}
	defer file.Close()
	defer os.Remove(file.Name())

	// 添加 UTF-8 BOM 头
	_, err = file.Write([]byte{0xEF, 0xBB, 0xBF})
	if err != nil {
		return fmt.Errorf("写入 UTF-8 BOM 失败: %w", err)
	}

	writer := csv.NewWriter(file)
	header := []string{"IP地址", "端口", "TLS", "数据中心", "源IP位置", "地区", "城市", "地区(中文)", "国家", "城市(中文)", "国旗", "网络延迟"}
	if *speedTest > 0 {
		header = append(header, "下载速度")
	}
	header = append(header, "出站IP", "出站IP类型", "出站IP位置", "IPS类型", "ASN号码", "ASN组织", "访问协议", "TLS版本", "SNI", "HTTP版本", "WARP", "Gateway", "RBI", "密钥交换", "时间戳")
	writer.Write(header)
	for _, res := range results {
		row := []string{res.ip, strconv.Itoa(res.port), strconv.FormatBool(*enableTLS), res.dataCenter, res.locCode, res.region, res.city, res.region_zh, res.country, res.city_zh, res.emoji, res.latency}
		if *speedTest > 0 {
			row = append(row, fmt.Sprintf("%.0f kB/s", res.downloadSpeed))
		}
		row = append(row, res.outboundIP, res.ipType, res.countryName, res.ipsType, strconv.FormatUint(uint64(res.autonomousSystemNumber), 10), res.autonomousSystemOrganization, res.visitScheme, res.tlsVersion, res.sni, res.httpVersion, res.warp, res.gateway, res.rbi, res.kex, res.timestamp)
		writer.Write(row)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("写入 CSV 失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭输出文件失败: %w", err)
	}
	if err := os.Rename(file.Name(), *outFile); err != nil {
		return fmt.Errorf("替换输出文件失败: %w", err)
	}
	fmt.Print("\033[2J")
	fmt.Printf("有效IP数量: %d | 成功将结果写入文件 %s，耗时 %d秒\n", len(results), *outFile, time.Since(startTime)/time.Second)
	return nil
}

// Keep upstream trace metadata while retaining the fork's established CSV names.
func makeTraceResult(ip string, port int, colo, locCode string, latency time.Duration, loc location, body string, databases geoDatabases) result {
	trace := parseTraceResponse(body)
	res := result{
		ip: ip, port: port, dataCenter: colo, locCode: locCode,
		region: loc.Region, city: loc.City, region_zh: loc.Region_zh, country: loc.Country, city_zh: loc.City_zh, emoji: loc.Emoji,
		latency: fmt.Sprintf("%d ms", latency.Milliseconds()), tcpDuration: latency,
		outboundIP: trace["ip"], ipType: getIPType(trace["ip"]), ipsType: "N/A",
		visitScheme: trace["visit_scheme"], tlsVersion: trace["tls"], sni: trace["sni"], httpVersion: trace["http"],
		warp: trace["warp"], gateway: trace["gateway"], rbi: trace["rbi"], kex: trace["kex"], timestamp: trace["ts"],
	}
	if *enableIPAPI {
		res.ipsType = queryIPAPI(ip)
	}
	if outbound := net.ParseIP(res.outboundIP); outbound != nil {
		if databases.asn != nil {
			if record, err := databases.asn.ASN(outbound); err == nil {
				res.autonomousSystemNumber, res.autonomousSystemOrganization = record.AutonomousSystemNumber, record.AutonomousSystemOrganization
			}
		}
		if databases.country != nil {
			if record, err := databases.country.Country(outbound); err == nil {
				res.countryName = record.Country.Names["zh-CN"]
			}
		}
	}
	return res
}

type geoDatabases struct{ asn, country *geoip2.Reader }

// Read into immutable memory rather than depending on platform-specific mmap.
// Missing optional caches do not trigger hidden network downloads.
func loadGeoDatabase(name, kind, sourceURL string) (*geoip2.Reader, error) {
	if name == "" {
		return nil, nil
	}
	file, err := os.Open(name)
	if os.IsNotExist(err) {
		if *downloadDB {
			return downloadGeoDatabase(name, kind, sourceURL)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const maxDatabase = 64 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxDatabase+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDatabase {
		return nil, fmt.Errorf("数据库超过 64 MiB 上限")
	}
	return decodeGeoDatabase(data, kind)
}

func decodeGeoDatabase(data []byte, kind string) (*geoip2.Reader, error) {
	reader, err := geoip2.FromBytes(data)
	if err != nil {
		if reader != nil {
			reader.Close()
		}
		return nil, err
	}
	if kind == "ASN" {
		_, err = reader.ASN(net.IPv4zero)
	} else {
		_, err = reader.Country(net.IPv4zero)
	}
	if err != nil {
		reader.Close()
		return nil, err
	}
	return reader, nil
}

func downloadGeoDatabase(name, kind, rawURL string) (*geoip2.Reader, error) {
	address, err := metadataURL(rawURL)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(address.String())
	if err != nil {
		return nil, fmt.Errorf("数据库下载请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("数据库下载 HTTP %d", resp.StatusCode)
	}
	const maxDatabase = 64 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDatabase+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDatabase {
		return nil, fmt.Errorf("数据库超过 64 MiB 上限")
	}
	reader, err := decodeGeoDatabase(data, kind)
	if err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			reader.Close()
		}
	}()
	file, err := os.CreateTemp(filepath.Dir(name), ".geoip-*.mmdb")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(file.Name(), name); err != nil {
		return nil, err
	}
	published = true
	return reader, nil
}

type ipapiResponse struct {
	IsDatacenter bool `json:"is_datacenter"`
	Company      struct {
		Type string `json:"type"`
	} `json:"company"`
	ASN struct {
		Type string `json:"type"`
	} `json:"asn"`
	Error string `json:"error"`
}

func metadataURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("元数据 URL 必须为 HTTP/HTTPS、包含主机名且不得包含凭据或 fragment")
	}
	return parsed, nil
}

func queryIPAPI(ip string) string {
	if net.ParseIP(ip) == nil {
		return "N/A"
	}
	address, err := metadataURL(*ipAPIURL)
	if err != nil {
		return "N/A"
	}
	query := address.Query()
	query.Set("q", ip)
	address.RawQuery = query.Encode()
	client := http.Client{Timeout: 2 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(address.String())
	if err != nil {
		return "N/A"
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "N/A"
	}
	const maxResponse = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(body) > maxResponse {
		return "N/A"
	}
	var data ipapiResponse
	if json.Unmarshal(body, &data) != nil || data.Error != "" {
		return "N/A"
	}
	company, asn := strings.ToLower(strings.TrimSpace(data.Company.Type)), strings.ToLower(strings.TrimSpace(data.ASN.Type))
	switch {
	case company == "education" || asn == "education":
		return "🎓教育网"
	case company == "government" || asn == "government":
		return "🏛政府"
	case company == "banking" || asn == "banking":
		return "💰金融"
	case company == "business" || asn == "business":
		return "🏢企业"
	case company == "isp" && asn == "isp" && !data.IsDatacenter:
		return "✅家宽"
	case company != "" || asn != "" || data.IsDatacenter:
		return "🟥机房"
	default:
		return "N/A"
	}
}

func parseTraceResponse(body string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(body, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			fields[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return fields
}

func getIPType(ip string) string {
	if ip == "" {
		return "未知"
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "无效IP"
	}
	if parsed.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
}

// 从文件中读取IP地址和端口
func readIPs(File string) ([]string, error) {
	file, err := os.Open(File)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) != 2 {
			fmt.Printf("行格式错误: %s\n", line)
			continue
		}
		ipAddr := parts[0]
		portStr := parts[1]

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 || !regexp.MustCompile(`^[0-9]+$`).MatchString(portStr) || net.ParseIP(ipAddr) == nil {
			fmt.Printf("IP 或端口格式错误: %s\n", line)
			continue
		}

		ip := fmt.Sprintf("%s %d", ipAddr, port)
		ips = append(ips, ip)
	}
	return ips, scanner.Err()
}

// inc函数实现ip地址自增
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// 测速函数
func getDownloadSpeed(ip string, port int) float64 {
	address, err := downloadURL(*speedTestURL, *enableTLS)
	if err != nil {
		fmt.Println(err)
		return 0
	}
	req, err := http.NewRequest("GET", address, nil)
	if err != nil {
		fmt.Printf("创建测速请求失败: %v\n", err)
		return 0
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 0,
	}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		fmt.Printf("IP %s 端口 %d 连接失败: %v\n", ip, port, err)
		return 0
	}
	defer conn.Close()

	fmt.Printf("正在测试IP %s 端口 %d\n", ip, port)
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	client := http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return conn, nil
			},
		},
		Timeout: 10 * time.Second,
	}
	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("IP %s 端口 %d 测速无效: %v\n", ip, port, err)
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("IP %s 端口 %d 测速 HTTP 状态无效: %d\n", ip, port, resp.StatusCode)
		return 0
	}

	// Only one goroutine reads the body. Buffered completion cannot leak on an
	// early low-speed return, and the byte counter is safe to sample concurrently.
	reader := &countingReader{reader: resp.Body}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, reader)
		done <- err
	}()
	measuredSpeed := func() float64 {
		seconds := time.Since(startTime).Seconds()
		if seconds <= 0 {
			return 0
		}
		return float64(atomic.LoadInt64(&reader.written)) / seconds / 1024
	}
	var checkpoint <-chan time.Time
	if *fast {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		checkpoint = timer.C
	}
	for {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				fmt.Printf("IP %s 端口 %d 测速读取失败: %v\n", ip, port, err)
				return 0
			}
			speed := measuredSpeed()
			fmt.Printf("IP %s 端口 %d 下载速度 %.0f kB/s\n", ip, port, speed)
			return speed
		case <-checkpoint:
			checkpoint = nil // The fast checkpoint runs once, not the total timeout.
			speed := measuredSpeed()
			if speed < 100 {
				cancel()
				fmt.Printf("IP %s 端口 %d 前5秒速度 %.0f kB/s 未达100 kB/s，终止测速\n", ip, port, speed)
				return speed
			}
		case <-ctx.Done():
			// The sampling window ending is not a failed download. Keep its bytes.
			return measuredSpeed()
		}
	}
}

// countingReader 用于统计读取的字节数
type countingReader struct {
	written int64 // First field: 64-bit atomic alignment on 32-bit platforms.
	reader  io.Reader
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	atomic.AddInt64(&r.written, int64(n))
	return n, err
}

// 辅助函数：检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func loadLocations(name string) ([]location, error) {
	body, err := os.ReadFile(name)
	downloaded := false
	if os.IsNotExist(err) {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, requestErr := client.Get("https://proxy.api.030101.xyz/locations-adw.pages.dev/")
		if requestErr != nil {
			return nil, fmt.Errorf("下载 locations.json 失败: %w", requestErr)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("下载 locations.json: HTTP %d", resp.StatusCode)
		}
		body, err = ioutil.ReadAll(io.LimitReader(resp.Body, 4<<20))
		downloaded = true
	}
	if err != nil {
		return nil, fmt.Errorf("读取 locations.json 失败: %w", err)
	}
	var locations []location
	if err := json.Unmarshal(body, &locations); err != nil {
		return nil, fmt.Errorf("解析 locations.json 失败: %w", err)
	}
	if downloaded {
		file, err := os.CreateTemp(filepath.Dir(name), ".locations-*.json")
		if err != nil {
			return nil, err
		}
		defer file.Close()
		defer os.Remove(file.Name())
		if _, err := file.Write(body); err != nil {
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		if err := os.Rename(file.Name(), name); err != nil {
			return nil, err
		}
	}
	return locations, nil
}

func downloadURL(raw string, tls bool) (string, error) {
	protocol := "http"
	if tls {
		protocol = "https"
	}
	address := raw
	if !strings.Contains(address, "://") {
		address = protocol + "://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("测速 URL 无效: %w", err)
	}
	if parsed.Hostname() == "" || parsed.User != nil || parsed.Scheme != protocol {
		return "", fmt.Errorf("测速 URL 必须包含主机名、不得包含凭据，且协议必须与 -tls 一致")
	}
	return parsed.String(), nil
}
