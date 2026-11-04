package main

// Every address and response below is an explicit local regression fixture.
// These are not Cloudflare measurements or production GeoLite records.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func readFixtureCSV(t *testing.T, dir string) ([]string, []map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "out.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("custom UTF-8 BOM must survive the upstream integration")
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))).ReadAll()
	if err != nil || len(records) == 0 {
		t.Fatalf("invalid fixture CSV: %v", err)
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for _, fields := range records[1:] {
		row := map[string]string{}
		for i, name := range records[0] {
			row[name] = fields[i]
		}
		rows = append(rows, row)
	}
	return records[0], rows
}

func TestUpstreamTraceMetadataRetainsCustomCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=2001:db8::10\nvisit_scheme=https\ntls=TLSv1.3\nsni=plaintext\nhttp=h2\nwarp=off\ngateway=off\nrbi=off\nkex=X25519=LOCAL_FIXTURE\nts=1234567890.125\n")
	}))
	defer server.Close()
	dir := fixtureDir(t, server, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if err != nil || timedOut {
		t.Fatalf("local trace failed: %v; %s", err, out)
	}
	headers, rows := readFixtureCSV(t, dir)
	if len(rows) != 1 {
		t.Fatalf("valid trace must remain one row: %v; %s", rows, out)
	}
	// The established column names and localized fields remain compatible.
	if headers[1] != "端口" || rows[0]["源IP位置"] != "US" || rows[0]["城市(中文)"] != "測試城市" {
		t.Fatalf("custom CSV columns were lost: %v", rows[0])
	}
	want := map[string]string{
		"出站IP": "2001:db8::10", "出站IP类型": "IPv6",
		"出站IP位置": "", "IPS类型": "N/A", "ASN号码": "0", "ASN组织": "",
		"访问协议": "https", "TLS版本": "TLSv1.3", "SNI": "plaintext", "HTTP版本": "h2",
		"WARP": "off", "Gateway": "off", "RBI": "off", "密钥交换": "X25519=LOCAL_FIXTURE", "时间戳": "1234567890.125",
	}
	for name, value := range want {
		if !contains(headers, name) || rows[0][name] != value {
			t.Errorf("upstream column %q = %q, want local fixture %q", name, rows[0][name], value)
		}
	}
}

func TestDatabaseDownloadsRequireOptInAndValidHTTPDatabase(t *testing.T) {
	valid, err := os.ReadFile(filepath.Join("test", "fixtures", "GeoLite2-ASN-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	wrongKind, err := os.ReadFile(filepath.Join("test", "fixtures", "GeoLite2-Country-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		enabled bool
		status  int
		data    []byte
		good    bool
	}{
		{"valid", true, 200, valid, true}, {"http-error", true, 403, valid, false},
		{"invalid-mmdb", true, 200, []byte("LOCAL NOT A DATABASE"), false}, {"wrong-kind", true, 200, wrongKind, false},
		{"disabled", false, 200, valid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests int32
			download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&requests, 1)
				w.WriteHeader(tc.status)
				w.Write(tc.data)
			}))
			defer download.Close()
			trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=89.160.20.112\n")
			}))
			defer trace.Close()
			dir := fixtureDir(t, trace, 1)
			previous := []byte("LOCAL PREVIOUS CSV\n")
			if err := os.WriteFile(filepath.Join(dir, "out.csv"), previous, 0600); err != nil {
				t.Fatal(err)
			}
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-download-db="+fmt.Sprint(tc.enabled), "-asn-url="+download.URL, "-country-db=")
			if timedOut || (err == nil) != tc.good {
				t.Fatalf("database fixture: %v; %s", err, out)
			}
			cache, cacheErr := os.ReadFile(filepath.Join(dir, "GeoLite2-ASN.mmdb"))
			if tc.enabled && tc.good {
				if cacheErr != nil || !bytes.Equal(cache, valid) {
					t.Fatalf("valid cache was not atomically saved: %v", cacheErr)
				}
				_, rows := readFixtureCSV(t, dir)
				if len(rows) != 1 || rows[0]["ASN号码"] != "29518" {
					t.Fatalf("downloaded TEST database was not used: %v", rows)
				}
			} else if !os.IsNotExist(cacheErr) {
				t.Fatalf("invalid or disabled download produced a cache: %v", cacheErr)
			}
			if !tc.good {
				got, _ := os.ReadFile(filepath.Join(dir, "out.csv"))
				if !bytes.Equal(got, previous) {
					t.Fatal("database failure replaced the previous CSV")
				}
			}
			wantCalls := int32(0)
			if tc.enabled {
				wantCalls = 1
			}
			if atomic.LoadInt32(&requests) != wantCalls {
				t.Fatalf("requests=%d, want %d", requests, wantCalls)
			}
		})
	}
}

func TestUpstreamASNAndCountryAreReadFromLocalDatabases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This IP is a record in the vendored MaxMind TEST databases only.
		// The actual TCP/HTTP request still goes solely to the loopback server.
		fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=89.160.20.112\n")
	}))
	defer server.Close()
	dir := fixtureDir(t, server, 1)
	for _, name := range []string{"ASN", "Country"} {
		data, err := os.ReadFile(filepath.Join("test", "fixtures", "GeoLite2-"+name+"-Test.mmdb"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "GeoLite2-"+name+".mmdb"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if err != nil || timedOut {
		t.Fatalf("local database fixture failed: %v; %s", err, out)
	}
	_, rows := readFixtureCSV(t, dir)
	if len(rows) != 1 || rows[0]["ASN号码"] != "29518" || rows[0]["ASN组织"] != "Bredband2 AB" || rows[0]["出站IP位置"] != "瑞典" {
		t.Fatalf("database-derived TEST values missing: %v; %s", rows, out)
	}
	if rows[0]["国家"] != "測試國家" || rows[0]["源IP位置"] != "US" {
		t.Fatalf("outbound metadata replaced the custom data-center fields: %v", rows[0])
	}
}

func TestIPSAPIStatusAndExplicitFieldsControlClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"education", 200, `{"company":{"type":"education"}}`, "🎓教育网"},
		{"government", 200, `{"asn":{"type":"government"}}`, "🏛政府"},
		{"banking", 200, `{"company":{"type":"banking"}}`, "💰金融"},
		{"business", 200, `{"company":{"type":"business"}}`, "🏢企业"},
		{"datacenter", 200, `{"is_datacenter":true,"company":{"type":"isp"},"asn":{"type":"isp"}}`, "🟥机房"},
		{"datacenter-flag-only", 200, `{"is_datacenter":true}`, "🟥机房"},
		{"residential-explicit-false", 200, `{"is_datacenter":false,"company":{"type":"isp"},"asn":{"type":"isp"}}`, "✅家宽"},
		{"legacy-isp-inference", 200, `{"company":{"type":"isp"},"asn":{"type":"isp"}}`, "✅家宽"},
		{"http-error", 403, `{"company":{"type":"isp"},"asn":{"type":"isp"}}`, "N/A"},
		{"quota", 200, `{"error":"LOCAL TEST QUOTA"}`, "N/A"},
		{"empty", 200, `{}`, "N/A"}, {"malformed", 200, `not JSON`, "N/A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer api.Close()
			old := *ipAPIURL
			*ipAPIURL = api.URL
			defer func() { *ipAPIURL = old }()
			if got := queryIPAPI("127.0.0.1"); got != tc.want {
				t.Fatalf("local API classification=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestExplicitDatacenterIPSClassificationReachesCSV(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"is_datacenter":true,"company":{"type":"isp"},"asn":{"type":"isp"}}`)
	}))
	defer api.Close()
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=198.51.100.10\n")
	}))
	defer trace.Close()
	dir := fixtureDir(t, trace, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-ips=true", "-ipapi-url="+api.URL)
	if err != nil || timedOut {
		t.Fatalf("local IPS fixture failed: %v; %s", err, out)
	}
	_, rows := readFixtureCSV(t, dir)
	if len(rows) != 1 || rows[0]["IPS类型"] != "🟥机房" {
		t.Fatalf("explicit datacenter was misclassified in CSV: %v", rows)
	}
}

func TestBatchRejectsMissingFlagValuesBeforeAnyWrites(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Bash integration is verified on Linux")
	}
	for _, args := range [][]string{
		{"-ipapi-url", "-ips"}, {"-countries", "-fast"},
		{"-asn-db", "--download-db"}, {"-country-url", "--ips=true"},
		{"-ipapi-url"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			server := fixtureServer(t, "TST", true)
			dir := batchFixture(t, server)
			previous := map[string][]byte{}
			for _, name := range []string{"ip.txt", "ip_tq.csv", "ip_tq.txt", "ip_all.txt", "ip_top5.txt"} {
				previous[name] = []byte("LOCAL PREVIOUS " + name + "\n")
				if err := os.WriteFile(filepath.Join(dir, name), previous[name], 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runBatch(t, dir, args...)
			if err == nil || !bytes.Contains([]byte(out), []byte("缺少值")) || bytes.Contains([]byte(out), []byte("开始执行")) {
				t.Errorf("missing-value batch must fail before running: %v; %s", err, out)
			}
			for name, want := range previous {
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("missing-value invocation changed %s: %v; %q", name, err, got)
				}
			}
		})
	}
}

func TestBatchExplicitDashPrefixedDatabaseValueStillWorks(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Bash integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", true)
	dir := batchFixture(t, server)
	out, err := runBatch(t, dir, "-asn-db=-LOCAL_FIXTURE_CACHE.mmdb", "-country-db=", "-download-db=false")
	if err != nil {
		t.Fatalf("explicit equals value must remain usable: %v; %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	if err != nil || len(data) == 0 {
		t.Fatalf("valid batch must produce its tested endpoint: %v; %q; %s", err, data, out)
	}
}

func TestBatchForwardsUpstreamIPSAndDatabaseFlags(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Bash integration is verified on Linux")
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"company":{"type":"isp"},"asn":{"type":"isp"}}`)
	}))
	defer api.Close()
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=198.51.100.10\n")
	}))
	defer trace.Close()
	dir := batchFixture(t, trace)
	out, err := runBatch(t, dir, "-ips=true", "-ipapi-url="+api.URL, "-download-db=false", "-asn-db=", "-country-db=")
	if err != nil {
		t.Fatalf("batch failed to accept upstream flags: %v; %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ip_tq.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.csv"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_, rows := readFixtureCSV(t, dir)
	if len(rows) != 1 || rows[0]["IPS类型"] != "✅家宽" {
		t.Fatalf("batch lost upstream lookup: %v", rows)
	}
	text, err := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	if err != nil || len(text) == 0 {
		t.Fatalf("batch lost the custom TXT output: %v; %s", err, out)
	}
}

func TestIPSLookupDoesNotDependOnOutboundIP(t *testing.T) {
	for _, tc := range []struct {
		name, outbound string
		enabled        bool
	}{
		{"missing", "", true},
		{"invalid", "not-an-ip", true},
		{"valid-control", "89.160.20.112", true},
		{"disabled-missing", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&requests, 1)
				if got := r.URL.Query().Get("q"); got != "127.0.0.1" {
					t.Errorf("candidate IPS q=%q, want loopback candidate", got)
				}
				fmt.Fprint(w, `{"is_datacenter":true}`)
			}))
			defer api.Close()
			trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\n")
				if tc.outbound != "" {
					fmt.Fprintf(w, "ip=%s\n", tc.outbound)
				}
			}))
			defer trace.Close()
			dir := fixtureDir(t, trace, 1)
			for _, kind := range []string{"ASN", "Country"} {
				data, err := os.ReadFile(filepath.Join("test", "fixtures", "GeoLite2-"+kind+"-Test.mmdb"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "GeoLite2-"+kind+".mmdb"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-ips="+fmt.Sprint(tc.enabled), "-ipapi-url="+api.URL)
			if err != nil || timedOut {
				t.Fatalf("local trace fixture failed: %v; %s", err, out)
			}
			_, rows := readFixtureCSV(t, dir)
			if len(rows) != 1 {
				t.Fatalf("trace fixture must produce one row: %v; %s", rows, out)
			}
			want, calls := "N/A", int32(0)
			if tc.enabled {
				want, calls = "🟥机房", 1
			}
			if got := atomic.LoadInt32(&requests); got != calls || rows[0]["IPS类型"] != want {
				t.Errorf("IPS requests=%d, CSV label=%q; want %d, %q", got, rows[0]["IPS类型"], calls, want)
			}
			wantASN, wantCountry := "0", ""
			if tc.name == "valid-control" {
				wantASN, wantCountry = "29518", "瑞典"
			}
			if rows[0]["ASN号码"] != wantASN || rows[0]["出站IP位置"] != wantCountry {
				t.Errorf("outbound database metadata=%q/%q, want %q/%q", rows[0]["ASN号码"], rows[0]["出站IP位置"], wantASN, wantCountry)
			}
		})
	}
}

func TestUpstreamIPSLookupIsExplicitAndUsesCandidateIP(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			var requests int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&requests, 1)
				if r.URL.Query().Get("q") != "127.0.0.1" {
					t.Errorf("IPS query must use the tested candidate, got %q", r.URL.Query().Get("q"))
				}
				fmt.Fprint(w, `{"company":{"type":"isp"},"asn":{"type":"isp"}}`)
			}))
			defer api.Close()
			trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\nip=198.51.100.10\n")
			}))
			defer trace.Close()
			dir := fixtureDir(t, trace, 1)
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-ips="+fmt.Sprint(enabled), "-ipapi-url="+api.URL)
			if err != nil || timedOut {
				t.Fatalf("explicit local IPS fixture failed: %v; %s", err, out)
			}
			_, rows := readFixtureCSV(t, dir)
			if len(rows) != 1 {
				t.Fatalf("IPS fixture lost its valid trace: %v", rows)
			}
			want, calls := "N/A", int32(0)
			if enabled {
				want, calls = "✅家宽", 1
			}
			if rows[0]["IPS类型"] != want || atomic.LoadInt32(&requests) != calls {
				t.Fatalf("IPS=%q, requests=%d, want %q and %d", rows[0]["IPS类型"], atomic.LoadInt32(&requests), want, calls)
			}
		})
	}
}
