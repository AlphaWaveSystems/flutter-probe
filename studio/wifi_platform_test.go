package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// A WiFi connection has no way to say what the device is (the agent does not advertise it): the status says nothing about the platform instead of
// "ios" for an Android phone. A connection made by Connect (a device Studio listed and classified) says what it is.
func TestAConnectionThatDoesNotKnowItsPlatformSaysNone(t *testing.T) {
	a := NewApp()
	a.conn = &connection{client: &runFake{}, deviceID: "192.0.2.7:48686", deviceName: "WiFi 192.0.2.7", platform: device.PlatformIOS, platformUnknown: true}
	if st := a.Status(); !st.Connected || st.Platform != "" {
		t.Errorf("a connection of unknown platform: %+v, want connected with no platform", st)
	}
	for _, p := range []device.Platform{device.PlatformAndroid, device.PlatformIOS} {
		a.conn = &connection{client: &runFake{}, deviceID: "x", deviceName: "Listed", platform: p}
		if st := a.Status(); st.Platform != string(p) {
			t.Errorf("a listed %s device reports %q", p, st.Platform)
		}
	}
	a.conn = &connection{client: &runFake{}, deviceID: "192.0.2.7:48686", platform: device.PlatformIOS, platformUnknown: true}
	if got := a.conn.platformName(); got != "" {
		t.Errorf("platformName = %q", got)
	}
}

// ConnectWiFi is the only place a connection of unknown platform is made, and it must say so: a real connect to a fake agent leaves no platform.
func TestConnectWiFiLeavesThePlatformUnknown(t *testing.T) {
	agent := newFakeAgent(t)
	a := NewApp()
	a.ctx = context.Background()
	t.Cleanup(func() { a.shutdown(context.Background()) })
	st, err := a.ConnectWiFi("127.0.0.1", agent.port, "abcdefgh1234")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected || st.Platform != "" || a.Status().Platform != "" {
		t.Errorf("connect = %+v, status = %+v: a WiFi connection must name no platform", st, a.Status())
	}
}

// The settings panel and the device view are told the platform the connection knows, and nothing for a WiFi one (never "ios").
func TestSettingsAndDeviceViewAreToldNoPlatformForWiFi(t *testing.T) {
	t.Setenv(deviceViewEnv, "1")
	for _, c := range []struct {
		unknown bool
		want    string
	}{{true, ""}, {false, "ios"}} {
		a := NewApp()
		a.ctx = context.Background()
		a.hub = newEventHub()
		a.conn = &connection{client: &viewFakeClient{}, deviceID: "dev-1", deviceName: "Phone", platform: device.PlatformIOS, platformUnknown: c.unknown}
		info, err := a.GetDeviceSettings()
		if err != nil {
			t.Skipf("device listing is not available here: %v", err)
		}
		if info.Platform != c.want {
			t.Errorf("unknown=%v: settings platform %q, want %q", c.unknown, info.Platform, c.want)
		}
		vi, err := a.GetDeviceView()
		if err != nil || !vi.Enabled {
			t.Fatalf("device view: %+v, %v", vi, err)
		}
		ws, _, err := websocket.DefaultDialer.Dial(vi.URL+"?token="+vi.Token, nil)
		if err != nil {
			t.Fatal(err)
		}
		ws.SetReadDeadline(time.Now().Add(10 * time.Second))
		got, seen := "?", false
		for !seen {
			_, b, err := ws.ReadMessage()
			if err != nil {
				t.Fatalf("no hello: %v", err)
			}
			var m struct {
				T      string `json:"t"`
				Device struct{ Platform string }
			}
			if json.Unmarshal(b, &m) == nil && m.T == "hello" {
				got, seen = m.Device.Platform, true
			}
		}
		ws.Close()
		if got != c.want {
			t.Errorf("unknown=%v: device view platform %q, want %q", c.unknown, got, c.want)
		}
		a.shutdown(context.Background())
	}
}

// The run's exported metadata names no platform for a WiFi connection (it used to say "ios" for an Android phone).
func TestTheExportedRunNamesNoPlatformForWiFi(t *testing.T) {
	for _, c := range []struct {
		unknown bool
		want    string
	}{{true, ""}, {false, "ios"}} {
		dir := t.TempDir()
		file := filepath.Join(dir, "a.probe")
		if err := os.WriteFile(file, []byte("test \"t\"\n  log \"hi\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		a := &App{ctx: context.Background(), hub: newEventHub()}
		a.SetWorkspace(dir)
		cfg := &config.Config{}
		cfg.Defaults.Timeout = 2 * time.Second
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		a.conn = &connection{client: &runFake{}, cfg: cfg, deviceID: "x", deviceName: "Phone", platform: "ios", platformUnknown: c.unknown, streamCancel: cancel, streamDone: done}
		go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
		if _, err := a.RunFile(file); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "run.json")
		a.savePathFn = func(string, string, string) (string, error) { return out, nil }
		if _, err := a.ExportRunResults("json"); err != nil {
			t.Fatal(err)
		}
		cancel()
		<-done
		raw, _ := os.ReadFile(out)
		var rep struct{ Metadata struct{ Platform string } }
		if err := json.Unmarshal(raw, &rep); err != nil {
			t.Fatalf("%v: %s", err, raw)
		}
		if got := rep.Metadata.Platform; got != c.want {
			t.Errorf("unknown=%v: exported platform %q, want %q\n%s", c.unknown, got, c.want, raw)
		}
	}
}
