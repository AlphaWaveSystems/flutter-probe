package runner

import (
	"context"
	"errors"
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
	"github.com/gorilla/websocket"
)

// reconnectRig is a DeviceContext whose adb is a script: it logs every call and answers from files, so Reconnect runs for real (the token read, the follower, the dial).
type reconnectRig struct {
	dir, log string
	dc       *DeviceContext
}

func newReconnectRig(t *testing.T, hostPort int, tcp6, pm string, runAsToken string) *reconnectRig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for adb")
	}
	r := &reconnectRig{dir: t.TempDir()}
	r.log = filepath.Join(r.dir, "adb.log")
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	header := "  sl local_address rem_address st\n"
	write("tcp6", header+tcp6)
	write("tcp", header)
	write("pm", pm)
	write("tmptoken", "tmp-token-0123456789abcdef0123456789\n") // the shared file: a token that proves nothing
	if runAsToken != "" {
		write("runas", runAsToken+"\n")
	}
	script := "#!/bin/sh\necho \"$*\" >> " + r.log + "\ncase \"$*\" in\n" +
		"  *forward*) exit 0;;\n" +
		"  *\"cat /proc/net/tcp6\"*) cat " + r.dir + "/tcp6;;\n" +
		"  *\"cat /proc/net/tcp\"*) cat " + r.dir + "/tcp;;\n" +
		"  *\"pm list packages -U\"*) cat " + r.dir + "/pm;;\n" +
		"  *run-as*\" rm \"*) exit 0;;\n" +
		"  *run-as*) if [ -f " + r.dir + "/runas ]; then cat " + r.dir + "/runas; else exit 1; fi;;\n" +
		"  *\"cat /data/local/tmp/probe/token\"*) cat " + r.dir + "/tmptoken;;\n" +
		"  *) exit 0;;\nesac\n"
	adb := filepath.Join(r.dir, "adb")
	if err := os.WriteFile(adb, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r.dc = &DeviceContext{
		Manager: device.NewManagerWithPaths(device.ToolPaths{ADB: adb}), Platform: device.PlatformAndroid, Serial: "emulator-5554",
		AppID: "com.second", Port: hostPort, DevicePort: 48911,
		ReconnectDelay: 10 * time.Millisecond, TokenReadTimeout: 3 * time.Second, DialTimeout: 1500 * time.Millisecond,
	}
	return r
}

func (r *reconnectRig) calls(t *testing.T) []string {
	t.Helper()
	b, _ := os.ReadFile(r.log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (r *reconnectRig) forwardsTo() []string {
	b, _ := os.ReadFile(r.log)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if i := strings.Index(l, "forward tcp:"); i >= 0 {
			out = append(out, l[i:])
		}
	}
	return out
}

// listener counts the connections that reach the host side of the forward.
func countingListener(t *testing.T) (port int, conns *atomic.Int32) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	conns = &atomic.Int32{}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port, conns
}

// A shared token, another app holds the configured port and the app has none: the reconnect is refused, naming the holder, and nothing is dialed.
func TestReconnectRefusesAPortAnotherAppHoldsAndDoesNotDial(t *testing.T) {
	hostPort, conns := countingListener(t)
	r := newReconnectRig(t, hostPort, listenRow(48911, 10100), "package:com.first uid:10100\npackage:com.second uid:10200\n", "")
	start := time.Now()
	client, err := r.dc.Reconnect(context.Background())
	if client != nil {
		client.Close()
		t.Fatalf("a reconnect that must be refused returned a client")
	}
	var held *device.PortHeldError
	if !errors.As(err, &held) || held.Holders[0] != "com.first" || held.App != "com.second" || held.Port != 48911 {
		t.Fatalf("err = %v, want a *PortHeldError naming com.first, com.second and 48911", err)
	}
	if !strings.Contains(err.Error(), "reconnect:") {
		t.Errorf("the error should say it is a reconnect: %v", err)
	}
	if conns.Load() != 0 {
		t.Errorf("%d connection(s) reached the forwarded port: the holder was dialed", conns.Load())
	}
	if took := time.Since(start); took < r.dc.DialTimeout || took > r.dc.DialTimeout+4*time.Second {
		t.Errorf("refused after %s, want about the dial window (%s)", took, r.dc.DialTimeout)
	}
	for _, f := range r.forwardsTo() {
		if !strings.HasSuffix(f, "tcp:48911") {
			t.Errorf("the forward was moved to %q: it must stay on the configured port when nothing of the app is found", f)
		}
	}
}

// A shared token, another app holds the configured port, the app is on the next one: the reconnect forwards to it.
func TestReconnectFollowsTheAppsPortWithASharedToken(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if c, err := upgrader.Upgrade(w, req, nil); err == nil {
			c.Close() // the handshake that follows fails: this test is about the forward
		}
	}))
	defer srv.Close()
	hostPort, _ := strconv.Atoi(srv.Listener.Addr().String()[strings.LastIndex(srv.Listener.Addr().String(), ":")+1:])
	r := newReconnectRig(t, hostPort, listenRow(48911, 10100)+listenRow(48912, 10200), "package:com.first uid:10100\npackage:com.second uid:10200\n", "")
	// The version handshake after the dial has nothing to talk to, so the run is bounded: the forward is made before the dial, and that is what this test is about.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	client, err := r.dc.Reconnect(ctx)
	if client != nil {
		client.Close()
	}
	var held *device.PortHeldError
	if errors.As(err, &held) {
		t.Fatalf("refused although the app listens on 48912: %v", err)
	}
	fw := r.forwardsTo()
	if len(fw) < 2 || !strings.HasSuffix(fw[0], "tcp:48911") || !strings.HasSuffix(fw[len(fw)-1], "tcp:48912") {
		t.Errorf("forwards = %q, want the first to the configured 48911 and the last to the app's 48912", fw)
	}
	if got := r.dc.devicePort(); got != 48912 {
		t.Errorf("the context's device port is %d after the move, want 48912", got)
	}
}

// With a run-as token and a follower already on the app's port, the reconnect forwards to that port, and makes no call that looks at the sockets or the packages.
func TestReconnectWithARunAsTokenStartsOnTheFollowedPortAndMakesNoExtraCall(t *testing.T) {
	hostPort, _ := countingListener(t)
	r := newReconnectRig(t, hostPort, listenRow(48911, 10100)+listenRow(48912, 10200), "package:com.first uid:10100\npackage:com.second uid:10200\n", "run-as-token-0123456789abcdef0123456789")
	// the follower uses the same adb script as the context, so every look at the sockets would be in the log
	f := device.NewPortFollower(r.dc.Manager.ADB(), r.dc.Manager, "emulator-5554", "com.second", hostPort, 48911, nil, nil)
	if !f.Suspect(context.Background()) || f.DevicePort() != 48912 {
		t.Fatalf("setup: the follower did not move to 48912")
	}
	if err := os.WriteFile(r.log, nil, 0o644); err != nil { // forget the calls of the setup: the reconnect is what is measured
		t.Fatal(err)
	}
	r.dc.Follower = f
	client, _ := r.dc.Reconnect(context.Background()) // the dial fails (the listener drops it): only the adb calls matter here
	if client != nil {
		client.Close()
	}
	for _, c := range r.calls(t) {
		if strings.Contains(c, "/proc/net") || strings.Contains(c, "pm list") {
			t.Errorf("a run-as token needs no look at the sockets or packages, but adb was asked: %q", c)
		}
	}
	fw := r.forwardsTo()
	if len(fw) == 0 || !strings.HasSuffix(fw[0], "tcp:48912") {
		t.Errorf("forwards = %q, want the first to the followed port 48912", fw)
	}
	for _, f := range fw {
		if strings.HasSuffix(f, "tcp:48911") {
			t.Errorf("a forward went back to the configured port: %q", f)
		}
	}
}
