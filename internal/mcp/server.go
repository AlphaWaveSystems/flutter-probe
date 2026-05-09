package mcp

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/ios"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// Version is reported in the MCP `initialize` response. Override at startup
// (e.g. from cli.Version which is set via -ldflags) to surface the binary
// version to MCP clients.
var Version = "dev"

// Server implements a minimal MCP (Model Context Protocol) server over stdio.
// It exposes probe capabilities as tools callable by AI agents (e.g., Claude).
type Server struct {
	in  *bufio.Reader
	out io.Writer
}

// NewServer creates a Server reading from os.Stdin and writing to os.Stdout.
func NewServer() *Server {
	return &Server{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stdout,
	}
}

// Run reads JSON-RPC 2.0 messages from stdin and writes responses to stdout.
func (s *Server) Run() error {
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcp: read: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		resp := s.handle(line)
		if resp != nil {
			data, _ := json.Marshal(resp)
			fmt.Fprintf(s.out, "%s\n", data)
		}
	}
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type mcpTool struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	InputSchema mcpSchema `json:"inputSchema"`
}

type mcpSchema struct {
	Type       string             `json:"type"`
	Properties map[string]mcpProp `json:"properties,omitempty"`
	Required   []string           `json:"required,omitempty"`
}

type mcpProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

var tools = []mcpTool{
	// ---- Device lifecycle ----
	{
		Name:        "list_devices",
		Description: "List all connected/booted simulators, emulators, and physical devices. Returns JSON array with id, name, platform (android|ios), state, and OS version. Use this before run_tests to discover which device id to target.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "list_simulators",
		Description: "List all iOS simulators (booted AND shutdown). Use this to discover a UDID you can pass to start_device. Returns JSON array with udid, name, runtime, and state.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "list_avds",
		Description: "List Android Virtual Device (AVD) names available on the host. Use the returned name with start_device to boot an Android emulator.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "start_device",
		Description: "Boot an Android emulator (by AVD name) or iOS simulator (by UDID). Blocks until the device is online. Returns the booted device's id, name, platform, and state.",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"platform"},
			Properties: map[string]mcpProp{
				"platform": {Type: "string", Description: "Target platform: android or ios"},
				"avd":      {Type: "string", Description: "Android AVD name (required for android; use list_avds to discover)"},
				"udid":     {Type: "string", Description: "iOS simulator UDID (optional for ios; auto-selects if omitted)"},
				"timeout":  {Type: "string", Description: "Boot timeout as a Go duration, e.g. 90s (default 120s)"},
			},
		},
	},
	{
		Name:        "shutdown_device",
		Description: "Shut down an iOS simulator (provide udid) or an Android emulator (provide serial). Provide exactly one of udid or serial.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"udid":   {Type: "string", Description: "iOS simulator UDID to shut down"},
				"serial": {Type: "string", Description: "Android emulator serial to shut down (e.g. emulator-5554)"},
			},
		},
	},
	// ---- Widget tree & file I/O ----
	{
		Name:        "get_widget_tree",
		Description: "Dump the live widget tree from the running Flutter app. Use this to understand the UI structure and discover correct selectors before writing tests.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"device": {Type: "string", Description: "Target device id (serial or UDID; default: first available)"},
			},
		},
	},
	{
		Name:        "read_test",
		Description: "Read the contents of a .probe test file.",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Path to the .probe file"},
			},
		},
	},
	{
		Name: "write_test",
		Description: `Create or overwrite a .probe test file. Content is validated (parsed) before writing — a syntax error is returned without creating the file.

Supports the full ProbeScript syntax:
  Regular tests:   test "name" with tap, see, wait, swipe, type, scroll, etc.
  Composite tests: composite test "name" for multi-device coordination:
                     devices
                       A: iPhone 15 Simulator
                       B: Pixel 9 Emulator
                     A:
                       tap "Login"
                     sync "both logged in"
                     B:
                       see "Welcome"
  Recipes:         recipe "name"(params) — reusable step sequences
  Hooks:           before each / after each / on failure / before all / after all
  Data-driven:     with examples | col1 | col2 |`,
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"path", "content"},
			Properties: map[string]mcpProp{
				"path":    {Type: "string", Description: "Destination path (must end in .probe)"},
				"content": {Type: "string", Description: "Full ProbeScript content to write"},
			},
		},
	},
	{
		Name:        "run_script",
		Description: "Execute an ad-hoc inline ProbeScript without creating a file. Use this to interactively probe the app, check widget visibility, or execute one-off steps.",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"script"},
			Properties: map[string]mcpProp{
				"script": {Type: "string", Description: "Full ProbeScript content to run (e.g. 'test \"check\"\n  see \"Welcome\"')"},
				"flags":  {Type: "string", Description: "Extra probe test flags (e.g. --timeout 10s)"},
				"device": {Type: "string", Description: "Target device id (serial or UDID; default: first available)"},
			},
		},
	},
	// ---- Execution & reporting ----
	{
		Name: "run_tests",
		Description: `Run FlutterProbe .probe test files against the connected Flutter app.

Named parameters cover the most common options. Any probe test flag can be passed via flags.

Key flags for the flags parameter:
  --timeout 60s               per-step timeout (default 30s)
  --format json|junit         structured output (default: terminal)
  --dry-run                   validate syntax without connecting to a device
  --parallel                  distribute tests across all connected devices
  --shard 1/3                 run 1/3 of test files (for CI matrix builds)
  --host <ip> --token <tok>   WiFi mode for physical devices
  --disable-animations        set timeDilation=0 to speed up Flutter animations
  -y                          auto-approve destructive operations (CI/CD mode)
  --video                     record device screen during the run
  --stream                    emit one ndjson line per test as it completes (requires --format json)

For composite multi-device tests use the composite_devices parameter.`,
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"paths":             {Type: "string", Description: "Space-separated .probe file paths or directories (default: tests/)"},
				"tag":               {Type: "string", Description: "Only run tests with this tag (e.g. smoke)"},
				"flags":             {Type: "string", Description: "Additional probe test flags (see description for key options)"},
				"device":            {Type: "string", Description: "Primary device id (serial or UDID; default: first available)"},
				"composite_devices": {Type: "string", Description: "Space-separated ALIAS=SPEC pairs for composite multi-device tests. Formats: host:port/token (WiFi), iOS UDID, or Android serial. Example: \"A=192.168.1.10:48686/tok B=emulator-5556\""},
			},
		},
	},
	{
		Name:        "list_files",
		Description: "List all .probe test files in a directory.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Directory to search (default: tests/)"},
			},
		},
	},
	{
		Name:        "lint",
		Description: "Validate .probe files for syntax errors without running them against a device.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"paths": {Type: "string", Description: "Space-separated paths to lint (default: tests/)"},
			},
		},
	},
	{
		Name:        "take_screenshot",
		Description: "Capture the current screen of the running Flutter app and return the image.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"name":   {Type: "string", Description: "Screenshot name (default: mcp_capture)"},
				"device": {Type: "string", Description: "Target device id (serial or UDID; default: first available)"},
			},
		},
	},
	{
		Name:        "get_report",
		Description: "Read the most recently modified JSON test run report. Use generate_report to convert it to HTML.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Explicit report file path (default: most recently modified *.json in reports/)"},
			},
		},
	},
	{
		Name:        "generate_test",
		Description: "Use AI to generate a .probe test file from a natural language description.",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"prompt"},
			Properties: map[string]mcpProp{
				"prompt": {Type: "string", Description: "Natural language description of the test to generate"},
				"output": {Type: "string", Description: "Output file path (default: tests/<generated_name>.probe)"},
			},
		},
	},
	// ---- Project management ----
	{
		Name:        "init_project",
		Description: "Initialize a new FlutterProbe project in the current working directory. Creates probe.yaml with sensible defaults and a tests/ directory scaffold. Run once when setting up a new Flutter app for testing.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "generate_report",
		Description: "Generate a standalone HTML test report from a JSON results file. Pass --open in flags to open it in the browser immediately.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"input":  {Type: "string", Description: "JSON report path (default: most recently modified *.json in reports/)"},
				"output": {Type: "string", Description: "HTML output path (default: reports/report.html)"},
				"flags":  {Type: "string", Description: "Extra probe report flags (e.g. --open)"},
			},
		},
	},
	{
		Name:        "record",
		Description: "Record user interactions with the running Flutter app and generate a .probe test file. Records for the specified duration (default 30s), then returns the generated file content. Requires a WebSocket-connected device (simulators/emulators); physical-device WiFi connections are not supported for recording.",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"output":  {Type: "string", Description: "Output .probe file path (default: tests/recorded.probe)"},
				"device":  {Type: "string", Description: "Target device id (default: first available)"},
				"timeout": {Type: "string", Description: "Max recording duration as a Go duration (default: 30s)"},
			},
		},
	},
}

func (s *Server) handle(line string) *mcpResponse {
	var req mcpRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return &mcpResponse{
			JSONRPC: "2.0",
			Error:   map[string]any{"code": -32700, "message": "Parse error: " + err.Error()},
		}
	}

	switch req.Method {
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "probe-mcp", "version": Version},
			},
		}

	case "tools/list":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"tools": tools},
		}

	case "tools/call":
		return s.callTool(req)

	case "notifications/initialized":
		return nil

	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32601, "message": "Method not found: " + req.Method},
		}
	}
}

func (s *Server) callTool(req mcpRequest) *mcpResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "Invalid params: "+err.Error())
	}

	var args map[string]string
	_ = json.Unmarshal(params.Arguments, &args)
	if args == nil {
		args = map[string]string{}
	}

	switch params.Name {
	case "list_devices":
		return s.listDevices(req.ID)
	case "list_simulators":
		return s.listSimulators(req.ID)
	case "list_avds":
		return s.listAVDs(req.ID)
	case "start_device":
		return s.startDevice(req.ID, params.Arguments)
	case "shutdown_device":
		return s.shutdownDevice(req.ID, args["udid"], args["serial"])
	case "get_widget_tree":
		return s.getWidgetTree(req.ID, args["device"])
	case "read_test":
		return s.readTest(req.ID, args["path"])
	case "write_test":
		return s.writeTest(req.ID, args["path"], args["content"])
	case "run_script":
		return s.runScript(req.ID, args["script"], args["flags"], args["device"])
	case "run_tests":
		return s.runTests(req.ID, args)
	case "list_files":
		out, err := s.runProbe("lint", args["path"], "", "--list", "")
		return textResp(req.ID, out, err)
	case "lint":
		out, err := s.runProbe("lint", args["paths"], "", "", "")
		return textResp(req.ID, out, err)
	case "take_screenshot":
		return s.takeScreenshot(req.ID, args["name"], args["device"])
	case "get_report":
		return s.getReport(req.ID, args["path"])
	case "generate_test":
		return s.generateTest(req.ID, args["prompt"], args["output"])
	case "init_project":
		return s.initProject(req.ID)
	case "generate_report":
		return s.generateReport(req.ID, args["input"], args["output"], args["flags"])
	case "record":
		return s.record(req.ID, args["output"], args["device"], args["timeout"])
	default:
		return errResp(req.ID, -32602, "Unknown tool: "+params.Name)
	}
}

// ---- Tool implementations ----

func (s *Server) getWidgetTree(id any, deviceID string) *mcpResponse {
	script := "test \"mcp_widget_tree\"\n  dump widget tree\n"
	out, err := s.runInlineScript(script, "", deviceID)
	if err != nil {
		return textResp(id, out, err)
	}
	tree := extractBlock(out, "[widget_tree]", "[/widget_tree]")
	if tree == "" {
		return textResp(id, "Widget tree not available. Ensure the app is running and connected.\n"+out, fmt.Errorf("no tree"))
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": tree}},
	}}
}

func (s *Server) readTest(id any, path string) *mcpResponse {
	if path == "" {
		return errResp(id, -32602, "path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return textResp(id, "Cannot read file: "+err.Error(), err)
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(data)}},
	}}
}

func (s *Server) writeTest(id any, path, content string) *mcpResponse {
	if path == "" {
		return errResp(id, -32602, "path is required")
	}
	if !strings.HasSuffix(path, ".probe") {
		return errResp(id, -32602, "path must end in .probe")
	}
	if content == "" {
		return errResp(id, -32602, "content is required")
	}
	// Validate ProbeScript syntax before touching the filesystem. A parse error
	// is returned without creating the file so agents get immediate feedback.
	if _, err := parser.ParseFile(content); err != nil {
		return textResp(id, fmt.Sprintf("ProbeScript syntax error (file not written): %v", err), err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return textResp(id, "Cannot create directory: "+err.Error(), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return textResp(id, "Cannot write file: "+err.Error(), err)
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Written: %s (%d bytes)", path, len(content))}},
	}}
}

func (s *Server) runScript(id any, script, flags, deviceID string) *mcpResponse {
	if script == "" {
		return errResp(id, -32602, "script is required")
	}
	out, err := s.runInlineScript(script, flags, deviceID)
	return textResp(id, out, err)
}

func (s *Server) runTests(id any, args map[string]string) *mcpResponse {
	cmdArgs := []string{"test"}
	if args["paths"] != "" {
		cmdArgs = append(cmdArgs, strings.Fields(args["paths"])...)
	}
	if args["tag"] != "" {
		cmdArgs = append(cmdArgs, "--tag", args["tag"])
	}
	if args["flags"] != "" {
		cmdArgs = append(cmdArgs, strings.Fields(args["flags"])...)
	}
	if args["device"] != "" {
		cmdArgs = append(cmdArgs, "--device", args["device"])
	}
	// Each space-separated ALIAS=SPEC in composite_devices becomes its own
	// --composite-device flag so the runner can coordinate multi-device tests.
	for _, spec := range strings.Fields(args["composite_devices"]) {
		cmdArgs = append(cmdArgs, "--composite-device", spec)
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return textResp(id, string(out), err)
}

func (s *Server) getReport(id any, path string) *mcpResponse {
	if path == "" {
		// Use modification time, not alphabetical order, to find the latest report.
		matches, _ := filepath.Glob(filepath.Join("reports", "*.json"))
		if len(matches) == 0 {
			return textResp(id, "No JSON reports found in reports/. Run tests first with --format json.", fmt.Errorf("no reports"))
		}
		var newest string
		var newestMod time.Time
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			if newest == "" || info.ModTime().After(newestMod) {
				newest = m
				newestMod = info.ModTime()
			}
		}
		path = newest
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return textResp(id, "Cannot read report: "+err.Error(), err)
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(data)}},
	}}
}

func (s *Server) generateTest(id any, prompt, output string) *mcpResponse {
	if prompt == "" {
		return errResp(id, -32602, "prompt is required")
	}
	cmdArgs := []string{"generate", "prompt", "--prompt", prompt}
	if output != "" {
		cmdArgs = append(cmdArgs, "--output", output)
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return textResp(id, string(out), err)
}

func (s *Server) takeScreenshot(id any, name, deviceID string) *mcpResponse {
	if name == "" {
		name = "mcp_capture"
	}
	script := fmt.Sprintf("test \"mcp screenshot\"\n  take screenshot \"%s\"\n", name)
	cmdOut, _ := s.runInlineScript(script, "", deviceID)

	pattern := filepath.Join("reports", "screenshots", name+"_*.png")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": "Screenshot not found.\n" + cmdOut}},
			"isError": true,
		}}
	}
	latestPath := matches[len(matches)-1]
	imgData, err := os.ReadFile(latestPath)
	if err != nil {
		return errResp(id, -32603, "read screenshot: "+err.Error())
	}
	b64 := base64.StdEncoding.EncodeToString(imgData)
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Screenshot captured: %s", filepath.Base(latestPath))},
				{"type": "image", "data": b64, "mimeType": "image/png"},
			},
		},
	}
}

func (s *Server) initProject(id any) *mcpResponse {
	cmd := exec.Command(probeBin(), "init")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return textResp(id, string(out), err)
}

func (s *Server) generateReport(id any, input, output, flags string) *mcpResponse {
	if input == "" {
		// Same mod-time logic as getReport.
		matches, _ := filepath.Glob(filepath.Join("reports", "*.json"))
		if len(matches) == 0 {
			return textResp(id, "No JSON reports found in reports/. Run tests with --format json first.", fmt.Errorf("no reports"))
		}
		var newest string
		var newestMod time.Time
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			if newest == "" || info.ModTime().After(newestMod) {
				newest = m
				newestMod = info.ModTime()
			}
		}
		input = newest
	}
	if output == "" {
		output = filepath.Join("reports", "report.html")
	}
	cmdArgs := []string{"report", "--input", input, "-o", output}
	if flags != "" {
		cmdArgs = append(cmdArgs, strings.Fields(flags)...)
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return textResp(id, string(out), err)
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Report generated: %s\n%s", output, string(out))}},
	}}
}

func (s *Server) record(id any, output, deviceID, timeout string) *mcpResponse {
	d := 30 * time.Second
	if timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			return errResp(id, -32602, "invalid timeout: "+err.Error())
		}
		d = parsed
	}
	if output == "" {
		output = filepath.Join("tests", "recorded.probe")
	}
	cmdArgs := []string{"record", "--output", output}
	if deviceID != "" {
		cmdArgs = append(cmdArgs, "--device", deviceID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, _ := cmd.CombinedOutput()

	// Whether the CLI exited cleanly or was killed by the timeout, return the
	// generated file if it was written.
	if data, err := os.ReadFile(output); err == nil && len(data) > 0 {
		return &mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf(
				"Recording complete (%s) → %s\n\n%s", d, output, string(data),
			)}},
		}}
	}
	return textResp(id, string(out), fmt.Errorf("recording produced no output file"))
}

// ---- Device lifecycle ----

// deviceManager is the function used to construct the device manager. Tests
// override it to inject fakes.
var deviceManager func() devManager = func() devManager { return device.NewManager() }

// devManager is the subset of device.Manager that the MCP server uses.
// Defined as an interface so tests can substitute fakes without spinning
// up real adb/simctl.
type devManager interface {
	List(ctx context.Context) ([]device.Device, error)
	Start(ctx context.Context, avdName string, bootTimeout, pollInterval time.Duration) (*device.Device, error)
	StartIOS(ctx context.Context, udid string) (*device.Device, error)
	SimCtl() *ios.SimCtl
	ADB() *device.ADB
}

func (s *Server) listDevices(id any) *mcpResponse {
	ctx := context.Background()
	dm := deviceManager()
	devices, err := dm.List(ctx)
	if err != nil {
		return textResp(id, "list devices: "+err.Error(), err)
	}
	type entry struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Platform  string `json:"platform"`
		State     string `json:"state"`
		OSVersion string `json:"osVersion,omitempty"`
	}
	out := make([]entry, 0, len(devices))
	for _, d := range devices {
		out = append(out, entry{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  string(d.Platform),
			State:     d.State,
			OSVersion: d.OSVersion,
		})
	}
	return jsonResp(id, out)
}

func (s *Server) listSimulators(id any) *mcpResponse {
	ctx := context.Background()
	sims, err := deviceManager().SimCtl().List(ctx)
	if err != nil {
		return textResp(id, "list simulators: "+err.Error(), err)
	}
	type entry struct {
		UDID    string `json:"udid"`
		Name    string `json:"name"`
		Runtime string `json:"runtime"`
		State   string `json:"state"`
	}
	out := make([]entry, 0, len(sims))
	for _, sim := range sims {
		out = append(out, entry{
			UDID:    sim.UDID,
			Name:    sim.Name,
			Runtime: sim.HumanRuntime(),
			State:   sim.State,
		})
	}
	return jsonResp(id, out)
}

func (s *Server) listAVDs(id any) *mcpResponse {
	ctx := context.Background()
	avds, err := deviceManager().ADB().ListAVDs(ctx)
	if err != nil {
		return textResp(id, "list avds: "+err.Error(), err)
	}
	if avds == nil {
		avds = []string{}
	}
	return jsonResp(id, avds)
}

func (s *Server) startDevice(id any, raw json.RawMessage) *mcpResponse {
	var args struct {
		Platform string `json:"platform"`
		AVD      string `json:"avd"`
		UDID     string `json:"udid"`
		Timeout  string `json:"timeout"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return errResp(id, -32602, "invalid arguments: "+err.Error())
		}
	}
	platform := strings.ToLower(args.Platform)
	if platform != "android" && platform != "ios" {
		return errResp(id, -32602, "platform must be \"android\" or \"ios\"")
	}

	timeout := 120 * time.Second
	if args.Timeout != "" {
		d, err := time.ParseDuration(args.Timeout)
		if err != nil {
			return errResp(id, -32602, "invalid timeout: "+err.Error())
		}
		timeout = d
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	dm := deviceManager()
	var booted *device.Device
	var err error
	switch platform {
	case "android":
		if args.AVD == "" {
			return errResp(id, -32602, "avd is required for android (use list_avds)")
		}
		booted, err = dm.Start(ctx, args.AVD, timeout, 0)
	case "ios":
		booted, err = dm.StartIOS(ctx, args.UDID)
	}
	if err != nil {
		return textResp(id, "start device: "+err.Error(), err)
	}
	return jsonResp(id, map[string]any{
		"id":       booted.ID,
		"name":     booted.Name,
		"platform": string(booted.Platform),
		"state":    booted.State,
	})
}

func (s *Server) shutdownDevice(id any, udid, serial string) *mcpResponse {
	if udid == "" && serial == "" {
		return errResp(id, -32602, "udid (iOS) or serial (Android) is required")
	}
	ctx := context.Background()
	if udid != "" {
		if err := deviceManager().SimCtl().Shutdown(ctx, udid); err != nil {
			return textResp(id, "shutdown simulator: "+err.Error(), err)
		}
		return jsonResp(id, map[string]any{"ok": true, "udid": udid})
	}
	// Android emulator: adb -s <serial> emu kill
	if _, err := deviceManager().ADB().Run(ctx, serial, "emu", "kill"); err != nil {
		return textResp(id, "shutdown emulator: "+err.Error(), err)
	}
	return jsonResp(id, map[string]any{"ok": true, "serial": serial})
}

// ---- Helpers ----

func (s *Server) runProbe(subcommand, paths, tag, extra, deviceID string) (string, error) {
	cmdArgs := []string{subcommand}
	if paths != "" {
		cmdArgs = append(cmdArgs, strings.Fields(paths)...)
	}
	if tag != "" {
		cmdArgs = append(cmdArgs, "--tag", tag)
	}
	if extra != "" {
		cmdArgs = append(cmdArgs, strings.Fields(extra)...)
	}
	if deviceID != "" {
		cmdArgs = append(cmdArgs, "--device", deviceID)
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *Server) runInlineScript(script, flags, deviceID string) (string, error) {
	tmp, err := os.CreateTemp("", "probe-mcp-*.probe")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(script); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	cmdArgs := []string{"test", tmp.Name()}
	if flags != "" {
		cmdArgs = append(cmdArgs, strings.Fields(flags)...)
	}
	if deviceID != "" {
		cmdArgs = append(cmdArgs, "--device", deviceID)
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func extractBlock(s, open, close string) string {
	start := strings.Index(s, open)
	if start == -1 {
		return ""
	}
	start += len(open)
	end := strings.Index(s[start:], close)
	if end == -1 {
		return strings.TrimSpace(s[start:])
	}
	return strings.TrimSpace(s[start : start+end])
}

func probeBin() string {
	if p, err := exec.LookPath("probe"); err == nil {
		return p
	}
	return "probe"
}

func textResp(id any, text string, err error) *mcpResponse {
	isError := err != nil
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isError,
		},
	}
}

func errResp(id any, code int, msg string) *mcpResponse {
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]any{"code": code, "message": msg},
	}
}

// jsonResp returns the value JSON-encoded inside a text content block. MCP
// clients parse the text themselves; this is the convention for structured
// data over a protocol that only ships text/image content blocks.
func jsonResp(id any, value any) *mcpResponse {
	data, err := json.Marshal(value)
	if err != nil {
		return errResp(id, -32603, "encode result: "+err.Error())
	}
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(data)}},
		},
	}
}
