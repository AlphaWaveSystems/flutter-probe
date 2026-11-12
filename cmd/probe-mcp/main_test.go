package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWantsFlags(t *testing.T) {
	for _, args := range [][]string{{"--http", "127.0.0.1:0"}, {"-http=127.0.0.1:0"}, {"--http=x"}, {"--dangerously-allow-non-loopback"}, {"-h"}, {"--help"}} {
		if !wantsFlags(args) {
			t.Errorf("wantsFlags(%v) = false", args)
		}
	}
	// Everything else starts stdio, as before the HTTP transport existed.
	for _, args := range [][]string{nil, {}, {"http"}, {"--stdio"}, {"--httpx"}, {"serve", "127.0.0.1:1"}, {"--verbose"}} {
		if wantsFlags(args) {
			t.Errorf("wantsFlags(%v) = true: stdio would not start", args)
		}
	}
}

func TestRunWithFlagsRefusals(t *testing.T) {
	cases := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--help"}, 0, "--dangerously-allow-non-loopback"},
		{[]string{"--dangerously-allow-non-loopback"}, 2, "--http needs an address"},
		// Addresses from the documentation ranges: no interface has them, so nothing can be opened
		// to the network even if the refusal were missing.
		{[]string{"--http", "192.0.2.1:1"}, 1, "not a loopback address"},
		{[]string{"--http=[2001:db8::1]:1"}, 1, "not a loopback address"},
		{[]string{"--http", "example.com:1"}, 1, "must be an IP address or localhost"},
	}
	for _, c := range cases {
		var stderr bytes.Buffer
		if code := runWithFlags(c.args, &stderr); code != c.code || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%v: exit %d, stderr %q; want exit %d and %q", c.args, code, stderr.String(), c.code, c.want)
		}
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestBinary builds probe-mcp and runs it: stdio answers the same bytes with and without stray
// arguments, and --http serves on loopback with a token that is in its file and never on stderr or
// stdout.
func TestBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses an interrupt signal and file modes")
	}
	bin := filepath.Join(t.TempDir(), "probe-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "PROBE_PROJECT_DIR=", "PROBE_MCP_HTTP_TOKEN=")

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"
	const want = `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"probe-mcp","version":"dev"}}}` + "\n"
	for _, args := range [][]string{nil, {"--stdio"}, {"ignored"}} {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Stdin = strings.NewReader(initialize)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("stdio %v: %v\n%s", args, err, stderr.String())
		}
		if stdout.String() != want || stderr.Len() != 0 {
			t.Errorf("stdio %v:\n stdout %q\n want   %q\n stderr %q", args, stdout.String(), want, stderr.String())
		}
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Errorf("a stdio run wrote under the home directory: %v", entries)
	}

	cmd := exec.Command(bin, "--http", "127.0.0.1:0")
	cmd.Env = env
	stdout, stderr := &lockedBuffer{}, &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	urlRE := regexp.MustCompile(`MCP over HTTP at (http://127\.0\.0\.1:\d+/mcp)`)
	fileRE := regexp.MustCompile(`bearer token written to (\S+)`)
	var url, file string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		u, f := urlRE.FindStringSubmatch(stderr.String()), fileRE.FindStringSubmatch(stderr.String())
		if u != nil && f != nil {
			url, file = u[1], f[1]
			break
		}
	}
	if url == "" {
		t.Fatalf("the server did not announce itself:\n%s", stderr.String())
	}
	// macOS puts the config dir under "Library/Application Support"; the path regexp stops at the space.
	matches, _ := filepath.Glob(filepath.Join(home, "*", "*", "flutter-probe-mcp", "http-token"))
	more, _ := filepath.Glob(filepath.Join(home, "*", "flutter-probe-mcp", "http-token"))
	matches = append(matches, more...)
	if len(matches) != 1 || !strings.HasPrefix(matches[0], file) {
		t.Fatalf("token file: found %v, stderr names %q", matches, file)
	}
	fi, err := os.Lstat(matches[0])
	if err != nil || fi.Mode() != 0o600 {
		t.Fatalf("token file mode = %v (%v), want -rw-------", fi.Mode(), err)
	}
	raw, _ := os.ReadFile(matches[0])
	token := strings.TrimSpace(string(raw))

	post := func(auth string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(initialize))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := post("Bearer " + token); code != http.StatusOK || body+"\n" != want {
		t.Errorf("with the token: %d %q, want 200 and the stdio answer", code, body)
	}
	if code, body := post(""); code != http.StatusUnauthorized || body != "" {
		t.Errorf("without a token: %d %q, want 401 and no body", code, body)
	}
	if code, _ := post("Bearer " + strings.Repeat("0", len(token))); code != http.StatusUnauthorized {
		t.Errorf("with a wrong token: %d, want 401", code)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("exit after an interrupt: %v\n%s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), token) || strings.Contains(stdout.String(), token) {
		t.Error("the token was printed")
	}
	if stdout.String() != "" {
		t.Errorf("--http wrote to stdout: %q", stdout.String())
	}
	if _, err := os.Stat(matches[0]); !os.IsNotExist(err) {
		t.Error("the token file is still there after the server stopped")
	}
}
