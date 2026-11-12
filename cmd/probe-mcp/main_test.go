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

func TestRunArgsLeavesPlainStdioAlone(t *testing.T) {
	// No arguments, or words only, start stdio, as before the HTTP transport existed.
	for _, args := range [][]string{nil, {}, {"http"}, {"serve", "127.0.0.1:1"}, {"-"}, {"ignored"}} {
		var stderr bytes.Buffer
		if code, handled := runArgs(args, "", &stderr); handled || code != 0 || stderr.Len() != 0 {
			t.Errorf("runArgs(%v) = %d, handled %v, stderr %q: stdio would not start", args, code, handled, stderr.String())
		}
	}
}

// TestRunArgsRefusesUnknownOptions: a mistyped option is an error, not a silent stdio server.
func TestRunArgsRefusesUnknownOptions(t *testing.T) {
	for _, args := range [][]string{{"--htpp", "127.0.0.1:0"}, {"--httpx"}, {"--stdio"}, {"--verbose"}, {"-x"}, {"word", "--htpp"}, {"--http=127.0.0.1:0", "--nope"}} {
		var stderr bytes.Buffer
		code, handled := runArgs(args, "", &stderr)
		if !handled || code != 2 {
			t.Errorf("runArgs(%v) = %d, handled %v; want exit 2", args, code, handled)
		}
		if out := stderr.String(); !strings.Contains(out, "unknown option") || !strings.Contains(out, "usage: probe-mcp") {
			t.Errorf("runArgs(%v) stderr = %q, want the unknown option and a usage line", args, out)
		}
	}
}

func TestRunArgsRefusals(t *testing.T) {
	cases := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--help"}, 0, "--dangerously-allow-non-loopback"},
		{[]string{"-h"}, 0, "[::1]:7391"},
		{[]string{"--dangerously-allow-non-loopback"}, 2, "--http needs an address"},
		{[]string{"word", "--http", "127.0.0.1:0"}, 2, "--http needs an address"},
		// Addresses from the documentation ranges: no interface has them, so nothing can be opened
		// to the network even if the refusal were missing.
		{[]string{"--http", "192.0.2.1:1"}, 1, "not a loopback address"},
		{[]string{"-http=192.0.2.1:1"}, 1, "not a loopback address"},
		{[]string{"--http=[2001:db8::1]:1"}, 1, "not a loopback address"},
		{[]string{"--http", "example.com:1"}, 1, "must be an IP address or localhost"},
	}
	for _, c := range cases {
		var stderr bytes.Buffer
		code, handled := runArgs(c.args, "", &stderr)
		if !handled || code != c.code || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%v: exit %d (handled %v), stderr %q; want exit %d and %q", c.args, code, handled, stderr.String(), c.code, c.want)
		}
	}
}

// TestTakeTokenUnsetsTheVariable: after the start-up read the variable is gone from the process, so
// no child process (probe test, adb) inherits the token, on the stdio path as on the HTTP path.
func TestTakeTokenUnsetsTheVariable(t *testing.T) {
	t.Setenv("PROBE_MCP_HTTP_TOKEN", "value-for-this-test")
	if got := takeToken(); got != "value-for-this-test" {
		t.Errorf("takeToken() = %q", got)
	}
	if v, set := os.LookupEnv("PROBE_MCP_HTTP_TOKEN"); set {
		t.Errorf("the variable is still set (%d characters): a child process would inherit it", len(v))
	}
	for _, kv := range exec.Command("true").Environ() {
		if strings.HasPrefix(kv, "PROBE_MCP_HTTP_TOKEN=") {
			t.Error("a child process would be started with the variable")
		}
	}
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// main takes the token before it looks at the arguments, so the stdio path is covered too.
	take, args := strings.Index(string(src), "token := takeToken()"), strings.Index(string(src), "runArgs(os.Args[1:], token, os.Stderr)")
	if take < 0 || args < 0 || take > args {
		t.Error("main does not take the token before it handles the arguments")
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
	for _, args := range [][]string{nil, {"ignored"}, {"serve", "now"}} {
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

	// A mistyped option: exit 2, a usage line, nothing on stdout, no server.
	{
		cmd := exec.Command(bin, "--htpp", "127.0.0.1:0")
		cmd.Env = env
		cmd.Stdin = strings.NewReader(initialize)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unknown option --htpp") {
			t.Errorf("--htpp: err %v, stdout %q, stderr %q; want exit 2 and the unknown option on stderr", err, stdout.String(), stderr.String())
		}
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
