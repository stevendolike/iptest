package main

// Regression tests for the actual CLI and download/filtering implementation.
// All addresses, locations and response payloads below are LOCAL TEST FIXTURES,
// not measurements or responses obtained from Cloudflare or any other provider.

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fixtureLocations = `[{"iata":"TST","cca2":"US","region":"Fixture","city":"Fixture City","region_zh":"測試地區","country":"測試國家","city_zh":"測試城市","emoji":"TEST"}]`

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "iptest-suite-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, item := range []struct {
		env, name string
		race      bool
	}{
		{"IPTEST_BIN", "iptest", false}, {"IPTEST_RACE_BIN", "iptest-race", true},
	} {
		if os.Getenv(item.env) != "" {
			continue
		}
		name := item.name
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		file := filepath.Join(dir, name)
		args := []string{"build", "-o", file}
		if item.race {
			args = append(args, "-race")
		}
		args = append(args, ".")
		if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "building test CLI: %v\n%s", err, out)
			os.RemoveAll(dir)
			os.Exit(1)
		}
		os.Setenv(item.env, file)
	}
	if os.Getenv("REVIEW_ARTIFACTS") == "" {
		os.Setenv("REVIEW_ARTIFACTS", dir)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func fixtureServer(t *testing.T, colo string, validTrace bool) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cdn-cgi/trace" {
			if validTrace {
				fmt.Fprintf(w, "uag=Mozilla/5.0\ncolo=%s\nloc=US\n", colo)
			} else {
				fmt.Fprint(w, "LOCAL FIXTURE: no valid trace")
			}
			return
		}
		w.Write(bytes.Repeat([]byte("x"), 8192))
	}))
	t.Cleanup(s.Close)
	return s
}

func fixtureAddress(t *testing.T, s *httptest.Server) (string, int) {
	t.Helper()
	host, p, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func fixtureDir(t *testing.T, s *httptest.Server, n int) string {
	t.Helper()
	dir := t.TempDir()
	host, port := fixtureAddress(t, s)
	if err := os.WriteFile(filepath.Join(dir, "locations.json"), []byte(fixtureLocations), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ip.txt"), []byte(strings.Repeat(fmt.Sprintf("%s %d\n", host, port), n)), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runFixture(t *testing.T, binary, dir string, limit time.Duration, args ...string) (string, error, bool) {
	t.Helper()
	if binary == "" {
		t.Fatal("IPTEST_BIN / IPTEST_RACE_BIN must name the actually built original binary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	base := []string{"-file=ip.txt", "-outfile=out.csv", "-tls=false", "-max=20", "-speedtest=0", "-url=fixture.local/download"}
	cmd := exec.CommandContext(ctx, binary, append(base, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GORACE=halt_on_error=0 exitcode=66")
	out, err := cmd.CombinedOutput()
	return string(out), err, ctx.Err() == context.DeadlineExceeded
}

func fixtureRows(t *testing.T, dir string) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "out.csv"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[1:]
}

func downloadFixture(t *testing.T, s *httptest.Server, fastMode bool, testURL string) float64 {
	t.Helper()
	oldTLS, oldFast, oldURL := *enableTLS, *fast, *speedTestURL
	*enableTLS, *fast, *speedTestURL = false, fastMode, testURL
	defer func() { *enableTLS, *fast, *speedTestURL = oldTLS, oldFast, oldURL }()
	host, port := fixtureAddress(t, s)
	return getDownloadSpeed(host, port)
}

func TestReviewBaselineValidTraceAndBOM(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if err != nil || timedOut {
		t.Fatalf("valid fixture failed: %v; %s", err, out)
	}
	rows := fixtureRows(t, dir)
	if len(rows) != 1 {
		t.Fatalf("expected one valid fixture row, got %d; %s", len(rows), out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("CSV lacks promised UTF-8 BOM")
	}
}

func TestTraceHTTPStatusControlsExportedEndpoints(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\n")
			}))
			defer s.Close()
			dir := fixtureDir(t, s, 1)
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
			if err != nil || timedOut {
				t.Fatalf("local trace fixture failed: %v; %s", err, out)
			}
			want := 0
			if status >= 200 && status < 300 {
				want = 1
			}
			if rows := fixtureRows(t, dir); len(rows) != want {
				t.Fatalf("HTTP %d trace exported %d endpoints, want %d; %s", status, len(rows), want, out)
			}
		})
	}
}

func TestReviewKnownCountryFilterControl(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-countries=CN")
	if err != nil || timedOut {
		t.Fatalf("control failed: %v; %s", err, out)
	}
	if len(fixtureRows(t, dir)) != 0 {
		t.Fatalf("known US location bypassed CN filter; %s", out)
	}
}

func TestReviewUnknownCountryMustNotBypassFilter(t *testing.T) {
	s := fixtureServer(t, "ZZZ", true)
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-countries=CN")
	if err != nil || timedOut {
		t.Fatalf("fixture run failed: %v; %s", err, out)
	}
	if rows := fixtureRows(t, dir); len(rows) != 0 {
		t.Fatalf("unknown data centre bypassed explicit CN filter: got %d row(s); %s", len(rows), out)
	}
}

func TestReviewNormalDownloadControl(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	got := downloadFixture(t, s, false, "fixture.local/download")
	t.Logf("local fixture normal-mode speed: %.3f kB/s", got)
	if got <= 0 {
		t.Fatalf("valid local 200 response should have positive speed, got %f", got)
	}
}

func TestReviewHTTPErrorMustNotBecomeDownloadSpeed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write(bytes.Repeat([]byte("LOCAL ERROR PAGE "), 512))
	}))
	defer s.Close()
	got := downloadFixture(t, s, false, "fixture.local/forbidden")
	if got != 0 {
		t.Fatalf("HTTP 403 fixture was accepted as speed %.3f kB/s; expected invalid result 0", got)
	}
}

func TestReviewFastMustNotRejectSustainedHighSpeed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)
		chunk := bytes.Repeat([]byte("x"), 32768)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if _, err := w.Write(chunk); err != nil {
					return
				}
				f.Flush()
			}
		}
	}))
	defer s.Close()
	control := downloadFixture(t, s, false, "fixture.local/stream")
	t.Logf("same local sustained-stream normal control: %.3f kB/s", control)
	if control < 100 {
		t.Fatalf("fixture cannot demonstrate bug: normal control below 100 kB/s: %.3f", control)
	}
	got := downloadFixture(t, s, true, "fixture.local/stream")
	if got < 100 {
		t.Fatalf("high-speed stream rejected by -fast: normal=%.3f, fast=%.3f kB/s", control, got)
	}
}

func TestReviewMalformedDownloadURLMustNotPanic(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("malformed -url caused panic instead of validation error: %v", p)
		}
	}()
	downloadFixture(t, s, false, "fixture.local/%")
}

func TestReviewInvalidMaxMustFailGracefully(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			s := fixtureServer(t, "TST", true)
			dir := fixtureDir(t, s, 1)
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, time.Second, "-max="+value)
			if timedOut {
				t.Fatalf("-max=%s hangs instead of rejecting input; %s", value, out)
			}
			if strings.Contains(out, "panic:") || strings.Contains(out, "fatal error:") {
				t.Fatalf("-max=%s crashes rather than validating input: %s", value, out)
			}
			if err == nil {
				t.Fatalf("-max=%s should return a nonzero validation error; %s", value, out)
			}
		})
	}
}

func TestReviewNoResultMustNotReportSuccessWithStaleCSV(t *testing.T) {
	s := fixtureServer(t, "TST", false)
	dir := fixtureDir(t, s, 1)
	stale := []byte("LOCAL_FIXTURE_PREVIOUS_RUN_RESULT\n")
	if err := os.WriteFile(filepath.Join(dir, "out.csv"), stale, 0600); err != nil {
		t.Fatal(err)
	}
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if timedOut {
		t.Fatalf("fixture hung; %s", out)
	}
	now, readErr := os.ReadFile(filepath.Join(dir, "out.csv"))
	if err == nil && readErr == nil && bytes.Equal(now, stale) {
		t.Fatalf("zero valid IPs returned exit 0 and retained prior output unchanged; pipeline can consume stale data; %s", out)
	}
}

func TestReviewMissingInputMustReturnNonzero(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-file=missing.txt")
	if timedOut {
		t.Fatalf("fixture hung; %s", out)
	}
	if err == nil {
		t.Fatalf("missing input returned exit 0; shell treats this as success; %s", out)
	}
}

func TestReviewConcurrentResultsMustBeRaceFree(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 128)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_RACE_BIN"), dir, 20*time.Second, "-speedtest=5")
	logPath := filepath.Join(os.Getenv("REVIEW_ARTIFACTS"), "race-output.log")
	if err := os.WriteFile(logPath, []byte(out), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("full observed original-binary race output: %s", logPath)
	if timedOut {
		t.Fatal("race fixture timed out")
	}
	if strings.Contains(out, "WARNING: DATA RACE") {
		start := strings.Index(out, "WARNING: DATA RACE")
		end := start + 2200
		if end > len(out) {
			end = len(out)
		}
		t.Errorf("race detector reported real unsynchronised production access (exit=%v):\n%s", err, out[start:end])
		return
	}
	if err != nil {
		t.Fatalf("race fixture failed unexpectedly: %v; %s", err, out)
	}
	if rows := fixtureRows(t, dir); len(rows) != 128 {
		t.Fatalf("concurrent append lost output rows: expected 128, got %d", len(rows))
	}
}

func TestInvalidLocationsMustFailAndPreserveOutput(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	stale := []byte("LOCAL_PREVIOUS_RUN\n")
	if err := os.WriteFile(filepath.Join(dir, "locations.json"), []byte("{invalid JSON}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.csv"), stale, 0600); err != nil {
		t.Fatal(err)
	}
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if timedOut || err == nil {
		t.Fatalf("invalid locations must return a nonzero error: %v; %s", err, out)
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "out.csv"))
	if readErr != nil || !bytes.Equal(got, stale) {
		t.Fatalf("failure destroyed earlier output: %v; %q", readErr, got)
	}
}

func TestOutputWriteErrorMustReturnNonzero(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	if err := os.Mkdir(filepath.Join(dir, "out.csv"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if timedOut || err == nil {
		t.Fatalf("output directory target must fail: %v; %s", err, out)
	}
}

func TestReadIPsRejectsInvalidAddressesAndPorts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.txt")
	input := "192.0.2.1 443\nnot-an-ip 443\n192.0.2.2 -1\n192.0.2.3 65536\n192.0.2.4 443.5\n::1 8443\n"
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	ips, err := readIPs(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 || ips[0] != "192.0.2.1 443" || ips[1] != "::1 8443" {
		t.Fatalf("invalid dial inputs admitted: %q", ips)
	}
}

func TestInvalidDownloadURLMustReturnNonzero(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-speedtest=1", "-url=fixture.local/%")
	if timedOut || err == nil || strings.Contains(out, "panic:") {
		t.Fatalf("URL must be rejected gracefully before measurement: %v; %s", err, out)
	}
}

func TestHTTPDownloadFailureMustNotBeCSVResult(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cdn-cgi/trace" {
			fmt.Fprint(w, "uag=Mozilla/5.0\ncolo=TST\nloc=US\n")
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "LOCAL HTTP ERROR")
	}))
	defer s.Close()
	dir := fixtureDir(t, s, 1)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-speedtest=1")
	if timedOut || err != nil {
		t.Fatalf("fixture failed: %v; %s", err, out)
	}
	if rows := fixtureRows(t, dir); len(rows) != 0 {
		t.Fatalf("failed download exported as valid row: %v; %s", rows, out)
	}
}

func TestSpeedFilterStillCompletesProgress(t *testing.T) {
	s := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, s, 3)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, "-speedtest=2", "-speedLimit=1000000000")
	if timedOut || err != nil {
		t.Fatalf("fixture failed: %v; %s", err, out)
	}
	start := strings.Index(out, "开始测速")
	if start < 0 || !strings.Contains(out[start:], "100.00%") {
		t.Fatalf("filtered measurements did not complete progress: %s", out)
	}
}

func batchFixture(t *testing.T, server *httptest.Server) string {
	t.Helper()
	dir := fixtureDir(t, server, 1)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.sh", "iptest.go", "ip_init.js", "ip_tq.js", "package.json", "lib/csv.js", "lib/io.js", "lib/address.js"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		// Windows autocrlf is a checkout conversion, not a committed shell defect.
		if strings.HasSuffix(name, ".sh") {
			data = bytes.ReplaceAll(data, []byte{13, 10}, []byte{10})
		}
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "node_modules"), filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	host, port := fixtureAddress(t, server)
	if err := os.WriteFile(filepath.Join(dir, "init.csv"), []byte(fmt.Sprintf("ip,port\n%s,%d\n", host, port)), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runBatch(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := append([]string{"main.sh", "-tls=false", "-speedtest=0"}, args...)
	cmd := exec.CommandContext(ctx, "bash", command...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("batch fixture timed out: %s", out)
	}
	return string(out), err
}

func TestLinuxFileLimitPolicyDoesNotSpawnChildShell(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux file descriptor policy")
	}
	server := fixtureServer(t, "TST", true)
	dir := fixtureDir(t, server, 1)
	bin := filepath.Join(dir, "fake-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "child-shell.marker")
	if err := os.WriteFile(filepath.Join(bin, "bash"), []byte("#!/bin/sh\nprintf '%s\\n' child-shell > \"$IPTEST_FD_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("IPTEST_FD_MARKER", marker)
	out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second)
	if err != nil || timedOut || len(fixtureRows(t, dir)) != 1 {
		t.Fatalf("valid fixture must still produce its endpoint: %v; %s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("CLI invoked a child bash rather than inheriting the parent file limit: %v; %s", err, out)
	}
	if strings.Contains(out, "上限已提升") || !strings.Contains(out, "父进程") || !strings.Contains(out, "ulimit -n") {
		t.Fatalf("file-limit policy must be honest and actionable: %s", out)
	}
}

func TestCLIRejectsPositionalArgumentsBeforeChangingOutput(t *testing.T) {
	for _, args := range [][]string{{"unexpected"}, {"unexpected", "-countries=CN"}, {"-countries=CN", "unexpected"}, {"--", "unexpected"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			server := fixtureServer(t, "TST", true)
			dir := fixtureDir(t, server, 1)
			previous := []byte("LOCAL_PREVIOUS_CSV\n")
			if err := os.WriteFile(filepath.Join(dir, "out.csv"), previous, 0600); err != nil {
				t.Fatal(err)
			}
			out, err, timedOut := runFixture(t, os.Getenv("IPTEST_BIN"), dir, 5*time.Second, args...)
			if timedOut || err == nil || !strings.Contains(out, "位置参数") {
				t.Fatalf("positional arguments must fail closed: %v; %s", err, out)
			}
			got, readErr := os.ReadFile(filepath.Join(dir, "out.csv"))
			if readErr != nil || !bytes.Equal(got, previous) {
				t.Fatalf("argument failure replaced prior output: %v; %q", readErr, got)
			}
		})
	}
}

func TestBatchRejectsPositionalArgumentsBeforeStartingPipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash batch integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", true)
	dir := batchFixture(t, server)
	previous := []byte("LOCAL_PREVIOUS_TXT\n")
	if err := os.WriteFile(filepath.Join(dir, "ip_tq.txt"), previous, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runBatch(t, dir, "unexpected", "-countries=CN")
	if err == nil || !strings.Contains(out, "位置参数") || strings.Contains(out, "开始执行") {
		t.Fatalf("batch accepted positional token or started work: %v; %s", err, out)
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	if readErr != nil || !bytes.Equal(got, previous) {
		t.Fatalf("failed batch replaced prior text: %v; %q", readErr, got)
	}
}

func TestBatchSpaceSeparatedCountryFlagStillWorks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash batch integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", true)
	dir := batchFixture(t, server)
	out, err := runBatch(t, dir, "-countries", "CN")
	if err != nil {
		t.Fatalf("valid separated flag rejected: %v; %s", err, out)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	if readErr != nil || len(data) != 0 {
		t.Fatalf("country filter did not produce a fresh empty result: %v; %q", readErr, data)
	}
}

func TestBatchPipelineUsesPassedTLSAndProducesFreshText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash batch integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", true)
	dir := batchFixture(t, server)
	out, err := runBatch(t, dir)
	if err != nil {
		t.Fatalf("batch failed: %v; %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	host, port := fixtureAddress(t, server)
	if err != nil || !strings.HasPrefix(string(data), net.JoinHostPort(host, strconv.Itoa(port))+"#") {
		t.Fatalf("batch did not produce current tested entry: %v; %q; %s", err, data, out)
	}
}

func TestBatchZeroResultsReplacesPreviousText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash batch integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", false)
	dir := batchFixture(t, server)
	if err := os.WriteFile(filepath.Join(dir, "ip_tq.txt"), []byte("LOCAL_PREVIOUS_RUN"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runBatch(t, dir)
	if err != nil {
		t.Fatalf("empty-result batch failed: %v; %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ip_tq.txt"))
	if err != nil || len(data) != 0 {
		t.Fatalf("old batch result survived: %v; %q", err, data)
	}
}

func TestBatchStopsWhenInitInputMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash batch integration is verified on Linux")
	}
	server := fixtureServer(t, "TST", true)
	dir := batchFixture(t, server)
	if err := os.Remove(filepath.Join(dir, "init.csv")); err != nil {
		t.Fatal(err)
	}
	out, err := runBatch(t, dir)
	if err == nil || !strings.Contains(out, "init.csv") || strings.Contains(out, "所有脚本执行完毕") {
		t.Fatalf("failed batch did not stop at the actual missing input: %v; %s", err, out)
	}
}
