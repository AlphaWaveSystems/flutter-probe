package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
		Description: "Connect Studio to a device: either a device from studio_list_devices (device_id: sets up the port forward, reads the agent token and opens the live connection), or a phone on the network (host, port, token: the same as the manual entry in Studio's WiFi overlay; needs a profile build of the app made with PROBE_AGENT=true and PROBE_WIFI=true, running in the foreground). With test_only=true and host it only checks that the agent is reachable and accepts the token. Required before studio_run_file.",
		InputSchema: mcpSchema{Type: "object", Properties: map[string]mcpProp{
			"device_id": {Type: "string", Description: "Device serial or simulator UDID (not with host)"},
			"host":      {Type: "string", Description: "IPv4, IPv6 or hostname of a phone on the network (WiFi)"},
			"port":      {Type: "string", Description: "Agent port for host, as a number in text (default 48686)"},
			"token":     {Type: "string", Description: "Agent token for host: the value after PROBE_TOKEN= in the app's console"},
			"test_only": {Type: "string", Description: "\"true\" with host: only check reachability and the token, do not connect"},
		}},
	},
	{
		Name:        "studio_run_file",
		Description: "Start running a .probe file in Studio, or part of it: the test that contains a line (line), the tests with a tag (tag), or the tests that failed the last time the file ran in Studio (failed_only). Give at most one of line, tag and failed_only; with none, the whole file runs. The file must be a .probe file inside the open workspace or an automation folder; any other path is refused. Returns at once; poll studio_run_state for step-by-step progress and studio_results for the verdicts. Fails if Studio is not connected to a device, a run is already in progress, or no test matches the scope.",
		InputSchema: mcpSchema{Type: "object", Required: []string{"path"}, Properties: map[string]mcpProp{
			"path":        {Type: "string", Description: "Absolute path of the .probe file"},
			"line":        {Type: "integer", Description: "Run the test that contains this 1-based line of the file"},
			"tag":         {Type: "string", Description: "Run the tests that carry this tag, for example smoke (the @ is optional)"},
			"failed_only": {Type: "boolean", Description: "Re-run the tests that failed the last time this file ran in Studio"},
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
	{
		Name:        "studio_widget_tree",
		Description: "The widget tree of the app on the device Studio is connected to, as text (what Studio's inspector shows). Read-only; fails if Studio is not connected to a device. Like get_widget_tree, but through the open Studio session, so no second connection to the agent is made.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "studio_ui_state",
		Description: "What Studio shows right now, in one read-only call: the connection (connected or not, which device, platform, agent version) and the state of the current or last run (running, finished, the plan and the results so far). Use it to decide whether to call studio_connect or studio_run_file next.",
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

// requireLoopbackURL refuses to send the bearer token anywhere but this machine: the discovery file
// is only trusted for the address it names, and that must be loopback.
func requireLoopbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return fmt.Errorf("the Studio discovery file has an unusable address %q", raw)
	}
	h := u.Hostname()
	if strings.EqualFold(h, "localhost") {
		return nil
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing to send the Studio token to %q: the discovery file must name a loopback address", h)
}

// studioCall sends one automation request and returns the raw result.
func studioCall(method string, params any) (json.RawMessage, error) {
	d, err := loadStudioDiscovery()
	if err != nil {
		return nil, err
	}
	if err := requireLoopbackURL(d.URL); err != nil {
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

// studioRunParams reads the arguments of studio_run_file. Its line and failed_only fields are typed, so they are decoded
// from the raw arguments: the string map the other tools read would hold an empty string for a number or a boolean.
func studioRunParams(raw json.RawMessage) (map[string]any, error) {
	var a struct {
		Path       string `json:"path"`
		Line       int    `json:"line"`
		Tag        string `json:"tag"`
		FailedOnly bool   `json:"failed_only"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("studio_run_file takes path (text), line (number), tag (text) and failed_only (true or false): %w", err)
		}
	}
	params := map[string]any{"path": a.Path}
	if a.Line != 0 {
		params["line"] = a.Line
	}
	if a.Tag != "" {
		params["tag"] = a.Tag
	}
	if a.FailedOnly {
		params["failedOnly"] = true
	}
	return params, nil
}

func (s *Server) studioTool(id any, name string, args map[string]string, rawArgs json.RawMessage) *mcpResponse {
	method, params := "", map[string]any{}
	switch name {
	case "studio_open_workspace":
		method, params = "open_workspace", map[string]any{"path": args["path"]}
	case "studio_list_devices":
		method = "list_devices"
	case "studio_connect":
		switch {
		case args["host"] != "":
			method = "connect_wifi"
			if args["test_only"] == "true" {
				method = "test_wifi"
			}
			port := 0 // 0 = the agent default, 48686
			if p := strings.TrimSpace(args["port"]); p != "" {
				n, err := strconv.Atoi(p)
				if err != nil {
					return textResp(id, "port must be a number", err)
				}
				port = n
			}
			params = map[string]any{"host": args["host"], "port": port, "token": args["token"]}
		case args["device_id"] != "":
			method, params = "connect", map[string]any{"deviceId": args["device_id"]}
		default:
			return textResp(id, "studio_connect needs device_id, or host with port and token", fmt.Errorf("missing arguments"))
		}
	case "studio_run_file":
		runParams, err := studioRunParams(rawArgs)
		if err != nil {
			return textResp(id, err.Error(), err)
		}
		method, params = "run_file", runParams
	case "studio_cancel":
		method = "cancel"
	case "studio_run_state":
		method = "run_state"
	case "studio_results":
		method = "results"
	case "studio_screenshot":
		method = "screenshot"
	case "studio_widget_tree":
		method = "widget_tree"
	case "studio_ui_state":
		method = "ui_state"
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
	text := string(res)
	if name == "studio_widget_tree" {
		// Studio answers the tree as one JSON string: hand the client the tree itself, not a quoted, escaped copy.
		var tree string
		if json.Unmarshal(res, &tree) != nil || tree == "" {
			return textResp(id, "Studio returned no widget tree", fmt.Errorf("empty"))
		}
		text = tree
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}}
}

func init() { tools = append(tools, studioTools...) }
