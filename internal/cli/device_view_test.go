package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/deviceview"
	"github.com/gorilla/websocket"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) Len() int { return len(s.String()) }

type stubAgent struct{ closed atomic.Bool }

func (s *stubAgent) Screenshot(context.Context, deviceview.ShotOptions) (deviceview.Shot, error) {
	return deviceview.Shot{Data: []byte{1}, Format: "jpeg", Width: 1, Height: 1}, nil
}
func (s *stubAgent) TapAt(context.Context, float64, float64) error            { return nil }
func (s *stubAgent) LongPressAt(context.Context, float64, float64, int) error { return nil }
func (s *stubAgent) SwipeAt(context.Context, float64, float64, float64, float64, int) error {
	return nil
}
func (s *stubAgent) TypeText(context.Context, string) error { return nil }
func (s *stubAgent) HitTest(context.Context, float64, float64) (*deviceview.Node, error) {
	return nil, nil
}
func (s *stubAgent) DumpTreeBounds(context.Context) ([]deviceview.Node, error) { return nil, nil }
func (s *stubAgent) Close() error                                              { s.closed.Store(true); return nil }

func TestDeviceViewCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"device", "view"})
	if err != nil || cmd != deviceViewCmd {
		t.Fatalf("device view not registered: %v", err)
	}
	for _, name := range []string{"device", "port", "json"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	if v, _ := cmd.Flags().GetBool("json"); !v {
		t.Error("--json must default to true")
	}
}

func TestServeDeviceViewStartupLines(t *testing.T) {
	for _, asJSON := range []bool{true, false} {
		var out, errOut syncBuf
		ctx, cancel := context.WithCancel(context.Background())
		agent := &stubAgent{}
		done := make(chan error, 1)
		go func() {
			done <- serveDeviceView(ctx, deviceview.Options{
				Agent:  agent,
				Device: deviceview.DeviceInfo{ID: "dev-1", Platform: "android", Name: "Test"},
			}, asJSON, &out, &errOut)
		}()

		var line struct{ URL, Token, Device string }
		deadline := time.Now().Add(3 * time.Second)
		for asJSON && time.Now().Before(deadline) {
			if strings.HasSuffix(out.String(), "\n") {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !agent.closed.Load() {
			t.Error("agent not closed on shutdown")
		}

		if asJSON {
			if err := json.Unmarshal(bytes.TrimSpace([]byte(out.String())), &line); err != nil {
				t.Fatalf("stdout is not one JSON line: %q (%v)", out.String(), err)
			}
			if !strings.HasPrefix(line.URL, "ws://127.0.0.1:") || !strings.HasSuffix(line.URL, "/device/v1") ||
				line.Token == "" || line.Device != "dev-1" {
				t.Fatalf("bad startup line: %+v", line)
			}
			if strings.Contains(errOut.String(), line.Token) {
				t.Error("token leaked to stderr although it was printed on stdout")
			}
			if strings.Contains(line.URL, "token") {
				t.Error("url must not carry the token")
			}
		} else {
			if out.Len() != 0 {
				t.Errorf("stdout must be empty with --json=false, got %q", out.String())
			}
			if !strings.Contains(errOut.String(), "?token=") {
				t.Errorf("stderr must carry the connection URL, got %q", errOut.String())
			}
		}
	}
}

func TestServeDeviceViewEndToEnd(t *testing.T) {
	var out, errOut syncBuf
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveDeviceView(ctx, deviceview.Options{Agent: &stubAgent{}}, true, &out, &errOut)
	}()
	defer func() { cancel(); <-done }()

	var line struct{ URL, Token string }
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.HasSuffix(out.String(), "\n") {
		time.Sleep(5 * time.Millisecond)
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out.String())), &line); err != nil {
		t.Fatal(err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(line.URL+"?token="+line.Token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(data), `"t":"hello"`) {
		t.Fatalf("first message = %s (%v)", data, err)
	}
}
