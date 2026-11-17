package cli

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
)

// followRig is a project run through the real `probe test` command, on a device whose adb is a script. The host side of the agent port is a local listener that counts the
// connections that reach it, so a test can say that the run did not dial.
type followRig struct {
	conns *atomic.Int32
}

// fakeAndroidOptions shapes the fake device: the run-as token (empty: run-as fails), whether run-as answers only once (a cold start: the token is read, rejected, and
// read again when run-as no longer answers), and whether the agent port answers 401 (the holder, which does not know the token) instead of dropping the connection.
type fakeAndroidOptions struct {
	runAsToken string
	runAsOnce  bool
	serve401   bool
	// serveAgent answers the handshake and every other call like an agent would, reporting agentAppID (empty: no app_id), instead of dropping the connection.
	serveAgent bool
	agentAppID string
}

func runTestWithFakeAndroid(t *testing.T, o fakeAndroidOptions) (*followRig, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for adb")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	rig := &followRig{conns: &atomic.Int32{}}
	var port int
	if o.serveAgent {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			rig.conns.Add(1)
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var req probelink.Request
				if json.Unmarshal(data, &req) != nil {
					return
				}
				body := map[string]any{"ok": true}
				if req.Method == probelink.MethodPing {
					body["agent_version"] = "0.27.0"
					if o.agentAppID != "" {
						body["app_id"] = o.agentAppID
					}
				}
				raw, _ := json.Marshal(body)
				resp, _ := json.Marshal(probelink.Response{JSONRPC: "2.0", ID: req.ID, Result: raw})
				if conn.WriteMessage(websocket.TextMessage, resp) != nil {
					return
				}
			}
		}))
		t.Cleanup(srv.Close)
		port = srv.Listener.Addr().(*net.TCPAddr).Port
	} else if o.serve401 {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rig.conns.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)
		port = srv.Listener.Addr().(*net.TCPAddr).Port
	} else {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				rig.conns.Add(1)
				c.Close()
			}
		}()
		port = l.Addr().(*net.TCPAddr).Port
	}

	state := t.TempDir()
	header := "  sl local_address rem_address st\n"
	holder := "   0: 0000000000000000FFFF00000100007F:BF0F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 10100        0 1 1 0000000000000000 100 0 0 10 0\n"
	files := map[string]string{
		"tcp6":     header + holder, // com.first (uid 10100) listens on 48911 = 0xBF0F
		"tcp":      header,
		"pm":       "package:com.first uid:10100\npackage:com.second uid:10200\n",
		"tmptoken": "shared-token-0123456789abcdef0123456789\n",
	}
	if o.runAsToken != "" {
		files["runas"] = o.runAsToken + "\n"
	}
	if o.runAsOnce {
		files["runas-once"] = ""
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(state, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	adb := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"  *devices*) printf 'List of devices attached\\nemulator-5554\\tdevice\\n';;\n" +
		"  *forward*) exit 0;;\n" +
		"  *\"cat /proc/net/tcp6\"*) cat " + state + "/tcp6;;\n" +
		"  *\"cat /proc/net/tcp\"*) cat " + state + "/tcp;;\n" +
		"  *\"pm list packages -U\"*) cat " + state + "/pm;;\n" +
		"  *run-as*) if [ -f " + state + "/runas ]; then if [ -f " + state + "/runas-once ] && [ -f " + state + "/runas-done ]; then exit 1; fi; touch " + state + "/runas-done; cat " + state + "/runas; else exit 1; fi;;\n" +
		"  *\"cat /data/local/tmp/probe/token\"*) cat " + state + "/tmptoken;;\n" +
		"  *) exit 0;;\nesac\n"
	if err := os.WriteFile(adb, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "project:\n  app: com.second\nagent:\n  port: " + strconv.Itoa(port) + "\n  device_port: 48911\n  dial_timeout: 1500ms\n  token_read_timeout: 3s\ntools:\n  adb: " + adb + "\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "a.probe"), []byte("test \"a\"\n  tap \"A\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := listDevices
	listDevices = listing([]device.Device{{ID: "emulator-5554", Name: "Pixel", Platform: device.PlatformAndroid}})
	t.Cleanup(func() { listDevices = prev })
	cmd := testCmd
	if sv, ok := cmd.Flags().Lookup("property").Value.(pflag.SliceValue); ok {
		if err := sv.Replace(nil); err != nil {
			t.Fatal(err)
		}
	}
	cmd.Flags().Set("parallel", "false")
	cmd.Flags().Set("devices", "")
	done := make(chan error, 1)
	go func() { done <- runTests(cmd, []string{"tests"}) }()
	select {
	case err := <-done:
		return rig, err
	case <-time.After(testRunGuard):
		t.Fatalf("the test command did not return within %v", testRunGuard)
		return rig, nil
	}
}

// probe test with a shared token, another app on the agent port and the app under test with none: the command refuses, names the holder and dials nothing.
func TestProbeTestRefusesAPortAnotherAppHoldsAndDoesNotDial(t *testing.T) {
	rig, err := runTestWithFakeAndroid(t, fakeAndroidOptions{})
	if err == nil {
		t.Fatal("the run must be refused")
	}
	for _, want := range []string{"agent port 48911 on the device is held by com.first", "com.second has not opened an agent port", "not connecting"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error lacks %q: %v", want, err)
		}
	}
	if rig.conns.Load() != 0 {
		t.Errorf("%d connection(s) reached the agent port: the holder was dialed", rig.conns.Load())
	}
}

// With a run-as token nothing is checked before the dial: the run goes on to dial (the listener drops the connection, so it fails on that, not on a refusal).
func TestProbeTestWithARunAsTokenIsNotRefusedBeforeTheDial(t *testing.T) {
	rig, err := runTestWithFakeAndroid(t, fakeAndroidOptions{runAsToken: "run-as-token-0123456789abcdef0123456789"})
	if err == nil {
		t.Fatal("the fake agent drops the connection, so the run must fail")
	}
	if strings.Contains(err.Error(), "has not opened an agent port") {
		t.Errorf("refused although the token proves the app: %v", err)
	}
	if rig.conns.Load() == 0 {
		t.Errorf("no connection reached the agent port: the run did not get as far as the dial (%v)", err)
	}
}

// The cold start of the review, through the real command: the run-as token is read and the holder rejects it (401), the token is read again, run-as no longer answers and
// the read falls back to the shared file, whose token may be the holder's. The holder check runs again and the redial does not happen: the holder was dialed once.
func TestProbeTestRefusesTheRedialWithASharedTokenAfterARunAsTokenWasRejected(t *testing.T) {
	rig, err := runTestWithFakeAndroid(t, fakeAndroidOptions{runAsToken: "run-as-token-0123456789abcdef0123456789", runAsOnce: true, serve401: true})
	if err == nil || !strings.Contains(err.Error(), "com.second has not opened an agent port") || !strings.Contains(err.Error(), "held by com.first") {
		t.Fatalf("err = %v, want the refusal naming com.first and com.second", err)
	}
	if rig.conns.Load() != 1 {
		t.Errorf("%d dial(s) reached the holder, want exactly the first one with the run-as token", rig.conns.Load())
	}
}

// stdoutOf runs fn with os.Stdout replaced by a pipe and returns what was written to it: probe test writes its status lines (the warning among them) to stdout.
func stdoutOf(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

// The `probe test` site of the app id warning, through the real command: an agent that says it is another app than project.app (com.second) is warned about, once; the
// same app, and an agent that says nothing, are not. The run still goes on to connect: a warning, not a refusal.
func TestProbeTestWarnsWhenTheAgentReportsAnotherApp(t *testing.T) {
	for _, tc := range []struct {
		name, reported string
		wantWarn       bool
	}{
		{"another app", "com.other.app", true},
		{"a difference of case alone is another app on Android", "com.Second", true},
		{"the same app", "com.second", false},
		{"the agent does not say", "", false},
	} {
		var out string
		var err error
		var rig *followRig
		out = stdoutOf(t, func() {
			rig, err = runTestWithFakeAndroid(t, fakeAndroidOptions{runAsToken: "run-as-token-0123456789abcdef0123456789", serveAgent: true, agentAppID: tc.reported})
		})
		_ = err
		if rig.conns.Load() == 0 {
			t.Fatalf("%s: the agent was never reached (%v)\n%s", tc.name, err, out)
		}
		has := strings.Contains(out, "reports app "+tc.reported+", but project.app is com.second")
		if has != tc.wantWarn {
			t.Errorf("%s: warning printed = %v, want %v\n%s", tc.name, has, tc.wantWarn, out)
		}
		if tc.wantWarn && strings.Count(out, "reports app") != 1 {
			t.Errorf("%s: the warning must be printed once:\n%s", tc.name, out)
		}
		if !strings.Contains(out, "Connected to ProbeAgent") {
			t.Errorf("%s: a warning is not a refusal, the run must connect:\n%s", tc.name, out)
		}
	}
}
