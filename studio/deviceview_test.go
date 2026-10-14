package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// viewFakeClient answers the agent calls the device view hub makes. The nil embedded interface makes any call
// outside this list panic, which would show Studio calling something the view should not.
type viewFakeClient struct {
	probelink.ProbeClient
	mu          sync.Mutex
	screenshots atomic.Int32
	closed      atomic.Int32
	methods     []string
}

// 1x1 transparent PNG.
var tinyPNG = base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0\x00\x00\x00\x00IEND\xaeB`\x82"))

func (f *viewFakeClient) Call(_ context.Context, method string, _ any) (json.RawMessage, error) {
	f.mu.Lock()
	f.methods = append(f.methods, method)
	f.mu.Unlock()
	switch method {
	case probelink.MethodScreenshot:
		f.screenshots.Add(1)
		return json.RawMessage(`{"path":"/tmp/x.png","data":"` + tinyPNG + `","format":"png","width":1,"height":1,"logicalWidth":400,"logicalHeight":800,"dpr":2}`), nil
	case "probe.type_text", "probe.hit_test":
		return json.RawMessage(`{"ok":true,"node":null}`), nil
	case probelink.MethodDumpTree:
		return json.RawMessage(`{"nodes":[]}`), nil
	}
	return nil, errors.New("method not found")
}
func (f *viewFakeClient) Close() error { f.closed.Add(1); return nil }
func (f *viewFakeClient) Screenshot(context.Context, string) (string, error) {
	f.screenshots.Add(1)
	return "", errors.New("unused")
}
func (f *viewFakeClient) DumpWidgetTree(context.Context) (string, error) { return "", nil }

func connectedApp(f *viewFakeClient) *App {
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.conn = &connection{client: f, deviceID: "dev-1", deviceName: "Test Phone", platform: device.PlatformAndroid}
	return a
}

func TestDeviceViewIsOffByDefaultAndStartsNothing(t *testing.T) {
	t.Setenv(deviceViewEnv, "")
	f := &viewFakeClient{}
	a := connectedApp(f)
	info, err := a.GetDeviceView()
	if err != nil || info.Enabled || info.URL != "" || info.Token != "" {
		t.Fatalf("info %+v, err %v; want disabled and empty", info, err)
	}
	if a.conn.view != nil {
		t.Fatal("no hub may be started while the feature is off")
	}
	// With the feature off the old frame loop is the frame source.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	old := streamErrorBackoff
	streamErrorBackoff = time.Millisecond
	defer func() { streamErrorBackoff = old }()
	a.startFrameSource(ctx, done, f)
	for f.screenshots.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the old loop must stop with its context")
	}
}

func TestDeviceViewNeedsAConnection(t *testing.T) {
	t.Setenv(deviceViewEnv, "1")
	a := &App{ctx: context.Background()}
	if _, err := a.GetDeviceView(); !errors.Is(err, errNotConnected) {
		t.Fatalf("err = %v, want errNotConnected", err)
	}
}

func TestDeviceViewServesFramesAndStopsWithTheConnection(t *testing.T) {
	t.Setenv(deviceViewEnv, "1")
	f := &viewFakeClient{}
	a := connectedApp(f)

	// With the view on, Studio's own capture loop does not run.
	done := make(chan struct{})
	a.startFrameSource(context.Background(), done, f)
	select {
	case <-done:
	default:
		t.Fatal("the old loop's done channel must be closed at once when the view is on")
	}
	if f.screenshots.Load() != 0 {
		t.Fatal("no capture may happen before the view is asked for")
	}

	info, err := a.GetDeviceView()
	if err != nil || !info.Enabled || !strings.HasPrefix(info.URL, "ws://127.0.0.1:") || len(info.Token) < 32 {
		t.Fatalf("info %+v, err %v", info, err)
	}
	again, _ := a.GetDeviceView()
	if again.URL != info.URL || again.Token != info.Token {
		t.Error("one hub per connection: a second call returns the same address")
	}

	// The loopback WebSocket answers with a hello for this device and then frames.
	ws, resp, err := websocket.DefaultDialer.Dial(info.URL+"?token="+info.Token, nil)
	if err != nil {
		t.Fatalf("dial: %v (resp %v)", err, resp)
	}
	var hello struct {
		T      string `json:"t"`
		Device struct{ ID, Platform, Name string }
	}
	gotFrame := false
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for !(hello.T == "hello" && gotFrame) {
		mt, b, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (hello %+v, frame %v)", err, hello, gotFrame)
		}
		if mt == websocket.BinaryMessage && len(b) > 5 && b[0] == 1 {
			gotFrame = true
			continue
		}
		var m struct {
			T      string `json:"t"`
			Device struct{ ID, Platform, Name string }
		}
		if json.Unmarshal(b, &m) == nil && m.T == "hello" {
			hello = m
		}
	}
	if hello.Device.ID != "dev-1" || hello.Device.Name != "Test Phone" || hello.Device.Platform != "android" {
		t.Errorf("hello device = %+v", hello.Device)
	}
	if _, resp, err := websocket.DefaultDialer.Dial(info.URL+"?token=wrong", nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a wrong token must get 401, got err %v resp %v", err, resp)
	}
	ws.Close()

	// Disconnecting stops the hub, and the connection's client is closed once, by Studio, not by the hub.
	a.Disconnect()
	if f.closed.Load() != 1 {
		t.Errorf("client closed %d times, want 1", f.closed.Load())
	}
	addr := strings.TrimPrefix(strings.TrimSuffix(info.URL, "/device/v1"), "ws://")
	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("the hub still listens after Disconnect")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
