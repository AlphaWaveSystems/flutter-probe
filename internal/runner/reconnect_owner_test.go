package runner

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// The app under test (cePkg) runs, no forward blames another device, and another app (uid 10300) listens on the agent port of the device: the hint names it.
const ownerTable = "  sl  local_address rem_address st\n   0: 0000000000000000FFFF00000100007F:BF0C 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 10300 0 1 1 0 100 0 0 10 0\n"

func ownerAdb(forward string) *ceAdb {
	a := liveAdb(forward)
	a.procTCP6 = ownerTable
	a.pmList = "package:" + cePkg + " uid:10207\npackage:com.example.other uid:10300\n"
	return a
}

func ownerExecutor(a *ceAdb) *Executor {
	e := hintExecutor(a, true)
	e.deviceCtx.Port = 48908 // 0xBF0C: the host port the CLI forwards
	e.deviceCtx.DevicePort = 48908
	return e
}

func TestTheHintNamesAnotherAppThatHoldsTheDevicePort(t *testing.T) {
	h := ownerExecutor(ownerAdb("")).reconnectHint(context.Background())
	if !strings.Contains(h, "agent port 48908 on the device is held by com.example.other (another app on this device)") {
		t.Errorf("owner hint: %q", h)
	}
	// the device port, not the host port, is what is looked up: a different device port finds nobody
	e := ownerExecutor(ownerAdb(""))
	e.deviceCtx.DevicePort = 48909
	if h := e.reconnectHint(context.Background()); h != "" {
		t.Errorf("the lookup used the wrong port: %q", h)
	}
	// with no device port set, the host port is used
	e = ownerExecutor(ownerAdb(""))
	e.deviceCtx.DevicePort = 0
	if h := e.reconnectHint(context.Background()); !strings.Contains(h, "held by com.example.other") {
		t.Errorf("no device port set: %q", h)
	}
	// a forward to another device is named first and hides the owner; a dead app hides both; this device's own forward hides nothing
	if h := ownerExecutor(ownerAdb("emulator-5554 tcp:48908 tcp:48908\n")).reconnectHint(context.Background()); !strings.Contains(h, "forwarded to another device") || strings.Contains(h, "held by") {
		t.Errorf("forward first: %q", h)
	}
	dead := ownerAdb("")
	dead.pidof = []string{""}
	if h := ownerExecutor(dead).reconnectHint(context.Background()); !strings.Contains(h, "is not running") || strings.Contains(h, "held by") {
		t.Errorf("dead app first: %q", h)
	}
	if h := ownerExecutor(ownerAdb("emulator-5600 tcp:48908 tcp:48908\n")).reconnectHint(context.Background()); !strings.Contains(h, "held by com.example.other") {
		t.Errorf("own forward: %q", h)
	}
	// only reads: nothing that changes the device
	a := ownerAdb("")
	ownerExecutor(a).reconnectHint(context.Background())
	for _, c := range a.calls {
		for _, w := range c {
			if w == "kill" || w == "force-stop" || w == "start" || w == "--remove" || w == "-c" {
				t.Errorf("a changing call: %v", c)
			}
		}
	}
	// the clock and pidof of the earlier looks, the forward list, and the three reads of the owner lookup: six calls, once each, in this order
	var got []string
	for _, c := range a.calls {
		got = append(got, strings.TrimPrefix(strings.Join(c, " "), "-s "+ceSerial+" "))
	}
	want := "shell date '+%Y-%m-%d %H:%M:%S'|shell pidof " + cePkg + "|forward --list|shell cat /proc/net/tcp6|shell cat /proc/net/tcp|shell pm list packages -U"
	if strings.Join(got, "|") != want {
		t.Errorf("calls = %q\nwant   %q", strings.Join(got, "|"), want)
	}
}

func TestTokenReadHint(t *testing.T) {
	ctx := context.Background()
	readErr := errors.New("android: probe token not found within 30s")
	// owner readable: named
	if h := ownerExecutor(ownerAdb("")).deviceCtx.tokenReadHint(ctx, readErr); !strings.Contains(h, "held by com.example.other") || strings.Contains(h, "did not answer") {
		t.Errorf("owner: %q", h)
	}
	// nothing readable or nobody holds the port: the hedged line, no names
	free := ownerAdb("")
	free.procTCP6 = ""
	if h := ownerExecutor(free).deviceCtx.tokenReadHint(ctx, readErr); h != device.NeutralPortHint || !strings.Contains(h, "if another app") {
		t.Errorf("fallback: %q", h)
	}
	// the app is not running: that error says so already
	if h := ownerExecutor(ownerAdb("")).deviceCtx.tokenReadHint(ctx, device.TokenAppNotRunningError(30*time.Second, "x")); h != "" {
		t.Errorf("dead app: %q", h)
	}
	// not Android, no error, cancelled
	ios := ownerExecutor(ownerAdb(""))
	ios.deviceCtx.Platform = device.PlatformIOS
	if h := ios.deviceCtx.tokenReadHint(ctx, readErr); h != "" {
		t.Errorf("ios: %q", h)
	}
	if h := ownerExecutor(ownerAdb("")).deviceCtx.tokenReadHint(ctx, nil); h != "" {
		t.Errorf("no error: %q", h)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	b := ownerAdb("")
	if h := ownerExecutor(b).deviceCtx.tokenReadHint(cancelled, readErr); h != "" || b.attempted != 0 {
		t.Errorf("cancelled: %q after %d calls", h, b.attempted)
	}
}

// The failed token read of a reconnect carries the hint, and the error it wraps stays in the chain.
func TestTheReconnectTokenReadFailureUsesTheTokenReadHint(t *testing.T) {
	b, err := os.ReadFile("device_context.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, `"reconnect: read token: %w%s", err, dc.tokenReadHint(ctx, err)`)
	if i < 0 {
		t.Fatal("the failed token read of a reconnect does not carry tokenReadHint")
	}
	if strings.Count(src, "reconnect: read token") != 1 {
		t.Error("another place reports a failed token read of a reconnect without the hint")
	}
}
