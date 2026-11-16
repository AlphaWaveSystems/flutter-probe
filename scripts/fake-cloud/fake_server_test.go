package fakecloud

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// start runs the fake server on a free port with its log in dir and returns its address; the process is stopped with the test.
func start(t *testing.T, dir string, extra ...string) (addr, log string) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	port := freePort(t)
	log = filepath.Join(dir, "fake.log")
	args := append([]string{"fake_server.py", "--port", fmt.Sprint(port), "--log-dir", dir, "--log", log}, extra...)
	cmd := exec.Command(py, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr = fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 50; i++ {
		if c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond); err == nil {
			c.Close()
			return addr, log
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the server did not start: %s", stderr.String())
	return
}

func TestTheFakeServerRecordsAnUploadWithoutTheKey(t *testing.T) {
	dir := t.TempDir()
	addr, log := start(t, dir)
	req, _ := http.NewRequest("POST", "http://"+addr+"/v1/reports", strings.NewReader(`{"a":1}`))
	req.Header.Set("Authorization", "Bearer SECRET-LICENSE-VALUE")
	req.Header.Set("X-Probe-Attestation", "SECRET-TOKEN-VALUE")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 202 {
		t.Fatalf("%v %v", resp, err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), `"auth_scheme": "Bearer"`) || !strings.Contains(string(b), `"attestation_present": true`) {
		t.Errorf("log = %s", b)
	}
	if strings.Contains(string(b), "SECRET") {
		t.Errorf("a secret value reached the log: %s", b)
	}
	if body, _ := os.ReadFile(log + ".body.json"); string(body) != `{"a":1}` {
		t.Errorf("body file = %q", body)
	}
	// the issuer plays its part and records the scheme only
	r2, _ := http.NewRequest("GET", "http://"+addr+"/oidc?audience=probe-cloud", nil)
	r2.Header.Set("Authorization", "bearer SECRET-REQUEST-CREDENTIAL")
	if resp, err := http.DefaultClient.Do(r2); err != nil || resp.StatusCode != 200 {
		t.Fatalf("oidc: %v %v", resp, err)
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "SECRET") || !strings.Contains(string(b), "probe-cloud") {
		t.Errorf("log = %s", b)
	}
}

// A body is never waited for without a length, and a big one is refused unread.
func TestTheFakeServerRefusesWhatItCannotBound(t *testing.T) {
	addr, log := start(t, t.TempDir())
	chunked, _ := http.NewRequest("POST", "http://"+addr+"/v1/reports", io.NopCloser(strings.NewReader("abc")))
	chunked.ContentLength = -1 // unknown length: sent chunked
	if resp, err := http.DefaultClient.Do(chunked); err != nil || resp.StatusCode != 411 {
		t.Errorf("chunked: %v %v", resp, err)
	}
	big, _ := http.NewRequest("POST", "http://"+addr+"/v1/reports", strings.NewReader(strings.Repeat("x", 1<<20+1)))
	if resp, err := http.DefaultClient.Do(big); err != nil || resp.StatusCode != 413 {
		t.Errorf("1 MiB + 1: %v %v", resp, err)
	}
	ok, _ := http.NewRequest("POST", "http://"+addr+"/v1/reports", strings.NewReader(strings.Repeat("x", 1<<20)))
	if resp, err := http.DefaultClient.Do(ok); err != nil || resp.StatusCode != 202 {
		t.Errorf("exactly 1 MiB: %v %v", resp, err)
	}
	// a raw request with a negative and a non-numeric length
	for _, cl := range []string{"-5", "abc", ""} {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		hdr := "POST /v1/reports HTTP/1.1\r\nHost: x\r\n"
		if cl != "" {
			hdr += "Content-Length: " + cl + "\r\n"
		}
		_, _ = c.Write([]byte(hdr + "Connection: close\r\n\r\n"))
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, _ := io.ReadAll(c)
		c.Close()
		if !strings.HasPrefix(string(line), "HTTP/1.0 411") && !strings.HasPrefix(string(line), "HTTP/1.1 411") {
			t.Errorf("Content-Length %q: %q", cl, line)
		}
	}
	// a length together with a chunked encoding is the shape request smuggling uses: refused as well
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("POST /v1/reports HTTP/1.1\r\nHost: x\r\nContent-Length: 3\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n0\r\n\r\n"))
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, _ := io.ReadAll(c)
	c.Close()
	if !strings.Contains(string(resp), " 411 ") {
		t.Errorf("length with chunked: %q", resp)
	}
	if b, _ := os.ReadFile(log); strings.Count(string(b), `"event": "upload"`) != 1 {
		t.Errorf("only the 1 MiB upload belongs in the log: %s", b)
	}
}

// The log goes only inside the folder it is confined to, links included.
func TestTheFakeServerConfinesItsLog(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	dir, other := t.TempDir(), t.TempDir()
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(other, link); err != nil {
		t.Skip("no symbolic links here")
	}
	for name, logPath := range map[string]string{
		"outside":           filepath.Join(other, "x.log"),
		"dot-dot":           filepath.Join(dir, "..", "x.log"),
		"through a link":    filepath.Join(link, "x.log"),
		"a sibling by name": dir + "-sibling/x.log",
	} {
		out, err := exec.Command(py, "fake_server.py", "--port", fmt.Sprint(freePort(t)), "--log-dir", dir, "--log", logPath).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "refusing to write") || !strings.Contains(string(out), "--log-dir") {
			t.Errorf("%s: err %v, output %q", name, err, out)
		}
	}
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Errorf("something was written outside: %v", entries)
	}
}
