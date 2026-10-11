package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// studioTools drive a running Probe Studio through its opt-in automation endpoint: start Studio with
// PROBE_STUDIO_AUTOMATION=1 and these tools find it through the discovery file Studio writes.
var studioTools = []mcpTool{
	{
		Name: "studio_open_workspace",
		Description: `Open a folder as the workspace of a running Probe Studio (the one started with PROBE_STUDIO_AUTOMATION=1) and list it. The folder's probe.yaml decides the agent port, timeout and device ids used when connecting.
Studio automation is off by default: if no Studio answers, start it with PROBE_STUDIO_AUTOMATION=1 (the tools find it through the discovery file it writes; PROBE_STUDIO_AUTOMATION_FILE overrides its location).`,
		InputSchema: mcpSchema{Type: "object", Required: []string{"path"}, Properties: map[string]mcpProp{
			"path": {Type: "string", Description: "Absolute path of the workspace folder"},
		}},
	},
	{
		Name:        "studio_list_devices",
		Description: "List the simulators/emulators Studio can connect to (booted/online ones), as Studio's device picker shows them.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "studio_connect",
		Description: "Connect Studio to a device (id from studio_list_devices): sets up the port forward, reads the agent token and opens the live connection. Required before studio_run_file.",
		InputSchema: mcpSchema{Type: "object", Required: []string{"device_id"}, Properties: map[string]mcpProp{
			"device_id": {Type: "string", Description: "Device serial or simulator UDID"},
		}},
	},
	{
		Name:        "studio_run_file",
		Description: "Start running a .probe file in Studio. Returns at once; poll studio_run_state for step-by-step progress and studio_results for the verdicts. Fails if Studio is not connected to a device or a run is already in progress.",
		InputSchema: mcpSchema{Type: "object", Required: []string{"path"}, Properties: map[string]mcpProp{
			"path": {Type: "string", Description: "Absolute path of the .probe file"},
		}},
	},
	{
		Name:        "studio_cancel",
		Description: "Cancel the run Studio is executing. The current step is interrupted and the run ends as cancelled.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "studio_run_state",
		Description: "The state of the current or last run in Studio: running, the plan (tests and step counts), the step being executed now (line, description), the finished steps with their verdicts and the test results so far.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "studio_results",
		Description: "The test results of the current or last run in Studio (name, passed, error, duration, performance measurements), plus whether the run is still going.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "studio_screenshot",
		Description: "A screenshot of the device Studio is connected to (what Studio shows in its device view).",
		InputSchema: mcpSchema{Type: "object"},
	},
}

type studioDiscovery struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func studioDiscoveryPath() string {
	if p := os.Getenv("PROBE_STUDIO_AUTOMATION_FILE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "flutter-probe-studio", "automation.json")
}

func loadStudioDiscovery() (studioDiscovery, error) {
	var d studioDiscovery
	raw, err := os.ReadFile(studioDiscoveryPath())
	if err != nil {
		return d, fmt.Errorf("no running Studio found (%s). Start Probe Studio with PROBE_STUDIO_AUTOMATION=1", studioDiscoveryPath())
	}
	if err := json.Unmarshal(raw, &d); err != nil || d.URL == "" || d.Token == "" {
		return d, fmt.Errorf("unreadable Studio discovery file %s", studioDiscoveryPath())
	}
	return d, nil
}

// studioCall sends one automation request and returns the raw result.
func studioCall(method string, params any) (json.RawMessage, error) {
	d, err := loadStudioDiscovery()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{"method": method, "params": params})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(d.URL, "/")+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("studio is not answering at %s (is it still running? remove %s if it is stale): %w", d.URL, studioDiscoveryPath(), err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected answer from Studio (HTTP %d)", resp.StatusCode)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	return out.Result, nil
}

func (s *Server) studioTool(id any, name string, args map[string]string) *mcpResponse {
	method, params := "", map[string]any{}
	switch name {
	case "studio_open_workspace":
		method, params = "open_workspace", map[string]any{"path": args["path"]}
	case "studio_list_devices":
		method = "list_devices"
	case "studio_connect":
		method, params = "connect", map[string]any{"deviceId": args["device_id"]}
	case "studio_run_file":
		method, params = "run_file", map[string]any{"path": args["path"]}
	case "studio_cancel":
		method = "cancel"
	case "studio_run_state":
		method = "run_state"
	case "studio_results":
		method = "results"
	case "studio_screenshot":
		method = "screenshot"
	default:
		return errResp(id, -32601, "unknown tool: "+name)
	}
	res, err := studioCall(method, params)
	if err != nil {
		return textResp(id, err.Error(), err)
	}
	if name == "studio_screenshot" {
		var b64 string
		if json.Unmarshal(res, &b64) != nil || b64 == "" {
			return textResp(id, "Studio returned no screenshot", fmt.Errorf("empty"))
		}
		return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]any{
			{"type": "text", "text": "Device screenshot from Studio"},
			{"type": "image", "data": b64, "mimeType": "image/png"},
		}}}
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]any{{"type": "text", "text": string(res)}}}}
}

func init() { tools = append(tools, studioTools...) }
