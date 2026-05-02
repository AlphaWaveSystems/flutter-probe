package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

// roundTrip writes a single JSON-RPC request to a fresh server and returns
// the parsed response. EOF after one line cleanly terminates Run().
func roundTrip(t *testing.T, req map[string]any) map[string]any {
	t.Helper()
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	var out bytes.Buffer
	s := &Server{
		in:  bufio.NewReader(bytes.NewReader(append(in, '\n'))),
		out: &out,
	}
	if err := s.Run(); err != nil {
		t.Fatalf("server.Run: %v", err)
	}
	if out.Len() == 0 {
		return nil // notification, no response expected
	}
	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v\nraw: %s", err, out.String())
	}
	return resp
}

func TestInitialize(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	})
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %+v", resp)
	}
	info, _ := result["serverInfo"].(map[string]any)
	if name, _ := info["name"].(string); name != "probe-mcp" {
		t.Errorf("serverInfo.name = %q, want probe-mcp", name)
	}
}

func TestToolsList(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	result, _ := resp["result"].(map[string]any)
	rawTools, _ := result["tools"].([]any)

	got := map[string]bool{}
	for _, tool := range rawTools {
		entry, _ := tool.(map[string]any)
		name, _ := entry["name"].(string)
		got[name] = true
	}

	want := []string{
		// device lifecycle (new in this PR)
		"list_devices", "list_simulators", "list_avds", "start_device", "shutdown_device",
		// existing
		"get_widget_tree", "read_test", "write_test", "run_script", "get_report",
		"generate_test", "run_tests", "list_files", "lint", "take_screenshot",
	}
	if len(got) != len(want) {
		t.Errorf("tool count = %d, want %d (got names: %v)", len(got), len(want), keys(got))
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool: %q", name)
		}
	}
}

func TestExistingToolsExposeDeviceArg(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/list",
	})
	result, _ := resp["result"].(map[string]any)
	rawTools, _ := result["tools"].([]any)

	wantDeviceArg := map[string]bool{
		"get_widget_tree": false,
		"run_script":      false,
		"run_tests":       false,
		"take_screenshot": false,
	}
	for _, tool := range rawTools {
		entry, _ := tool.(map[string]any)
		name, _ := entry["name"].(string)
		if _, expected := wantDeviceArg[name]; !expected {
			continue
		}
		schema, _ := entry["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		if _, ok := props["device"]; !ok {
			t.Errorf("tool %q missing device property", name)
		}
		wantDeviceArg[name] = true
	}
	for name, found := range wantDeviceArg {
		if !found {
			t.Errorf("tool %q not present in tools/list", name)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "no/such/method",
	})
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error, got %+v", resp)
	}
	if code, _ := errObj["code"].(float64); int(code) != -32601 {
		t.Errorf("error.code = %v, want -32601", code)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	if resp != nil {
		t.Errorf("expected nil response for notification, got %+v", resp)
	}
}

func TestStartDeviceArgValidation(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string // substring of error message
	}{
		{"missing platform", map[string]any{}, "platform must be"},
		{"unknown platform", map[string]any{"platform": "blackberry"}, "platform must be"},
		{"android without avd", map[string]any{"platform": "android"}, "avd is required"},
		{"invalid timeout", map[string]any{"platform": "ios", "timeout": "5 weeks"}, "invalid timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := roundTrip(t, map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params":  map[string]any{"name": "start_device", "arguments": tc.args},
			})
			errObj, ok := resp["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected error, got %+v", resp)
			}
			if msg, _ := errObj["message"].(string); !strings.Contains(msg, tc.want) {
				t.Errorf("error message %q does not contain %q", msg, tc.want)
			}
		})
	}
}

func TestShutdownDeviceRequiresUDID(t *testing.T) {
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "shutdown_device", "arguments": map[string]any{}},
	})
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error, got %+v", resp)
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "udid is required") {
		t.Errorf("error message = %q, want udid is required", msg)
	}
}

// fakeManager implements devManager and lets list_devices be tested without
// shelling out to adb/simctl. Methods that aren't exercised in this test
// return zero values so the interface is satisfied.
type fakeManager struct {
	devices []device.Device
	listErr error
}

func (f *fakeManager) List(ctx context.Context) ([]device.Device, error) {
	return f.devices, f.listErr
}
func (f *fakeManager) Start(ctx context.Context, avd string, _, _ time.Duration) (*device.Device, error) {
	return &device.Device{ID: "emulator-5554", Name: avd, Platform: device.PlatformAndroid, State: "online"}, nil
}
func (f *fakeManager) StartIOS(ctx context.Context, udid string) (*device.Device, error) {
	if udid == "" {
		udid = "AUTO-UDID"
	}
	return &device.Device{ID: udid, Name: "iPhone 15", Platform: device.PlatformIOS, State: "booted"}, nil
}
func (f *fakeManager) SimCtl() *ios.SimCtl { return ios.New() }
func (f *fakeManager) ADB() *device.ADB    { return device.NewADB() }

func TestListDevicesEncodesJSON(t *testing.T) {
	prev := deviceManager
	defer func() { deviceManager = prev }()
	deviceManager = func() devManager {
		return &fakeManager{devices: []device.Device{
			{ID: "emulator-5554", Name: "Pixel 7", Platform: device.PlatformAndroid, State: "online", OSVersion: "Android 14"},
			{ID: "ABCD-UDID", Name: "iPhone 15", Platform: device.PlatformIOS, State: "booted", OSVersion: "iOS 18.6"},
		}}
	}

	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "list_devices", "arguments": map[string]any{}},
	})

	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)

	var got []map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("payload is not JSON: %v\nraw: %s", err, text)
	}
	if len(got) != 2 {
		t.Fatalf("got %d devices, want 2", len(got))
	}
	if got[0]["id"] != "emulator-5554" || got[0]["platform"] != "android" {
		t.Errorf("first entry wrong: %+v", got[0])
	}
	if got[1]["id"] != "ABCD-UDID" || got[1]["platform"] != "ios" {
		t.Errorf("second entry wrong: %+v", got[1])
	}
}

func TestStartDeviceIOSAutoSelect(t *testing.T) {
	prev := deviceManager
	defer func() { deviceManager = prev }()
	deviceManager = func() devManager { return &fakeManager{} }

	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "start_device",
			"arguments": map[string]any{"platform": "ios"},
		},
	})

	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)

	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if got["platform"] != "ios" || got["state"] != "booted" {
		t.Errorf("unexpected payload: %+v", got)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
