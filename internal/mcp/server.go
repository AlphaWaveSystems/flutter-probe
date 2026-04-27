package mcp

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// ---- High value ----
	{
		Name:        "get_widget_tree",
		Description: "Dump the live widget tree from the running Flutter app. Use this to understand the UI structure and discover correct selectors before writing tests.",
		InputSchema: mcpSchema{Type: "object"},
	},
	{
		Name:        "read_test",
		Description: "Read the contents of a .probe test file",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Path to the .probe file"},
			},
		},
	},
	{
		Name:        "write_test",
		Description: "Create or overwrite a .probe test file with the given content",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"path", "content"},
			Properties: map[string]mcpProp{
				"path":    {Type: "string", Description: "Path to write (must end in .probe)"},
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
			},
		},
	},
	// ---- Medium value ----
	{
		Name:        "get_report",
		Description: "Read the most recent test run report in JSON format",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Explicit report file path (default: auto-detect latest in reports/)"},
			},
		},
	},
	{
		Name:        "generate_test",
		Description: "Use AI to generate a .probe test file from a natural language description",
		InputSchema: mcpSchema{
			Type:     "object",
			Required: []string{"prompt"},
			Properties: map[string]mcpProp{
				"prompt": {Type: "string", Description: "Natural language description of the test to generate"},
				"output": {Type: "string", Description: "Output file path (default: tests/<generated_name>.probe)"},
			},
		},
	},
	// ---- Existing tools ----
	{
		Name:        "run_tests",
		Description: "Run FlutterProbe .probe test files against the connected Flutter app",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"paths": {Type: "string", Description: "Space-separated list of .probe file paths or directories (default: tests/)"},
				"tag":   {Type: "string", Description: "Only run tests with this tag (e.g. smoke)"},
				"flags": {Type: "string", Description: "Additional probe test flags (e.g. --dry-run --format json)"},
			},
		},
	},
	{
		Name:        "list_files",
		Description: "List all .probe test files in a directory",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"path": {Type: "string", Description: "Directory to search (default: tests/)"},
			},
		},
	},
	{
		Name:        "lint",
		Description: "Validate .probe files for syntax errors without running them",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"paths": {Type: "string", Description: "Space-separated paths to lint (default: tests/)"},
			},
		},
	},
	{
		Name:        "take_screenshot",
		Description: "Capture the current screen of the running Flutter app and return the image",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"name": {Type: "string", Description: "Screenshot name (default: mcp_capture)"},
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
	case "get_widget_tree":
		return s.getWidgetTree(req.ID)

	case "read_test":
		return s.readTest(req.ID, args["path"])

	case "write_test":
		return s.writeTest(req.ID, args["path"], args["content"])

	case "run_script":
		return s.runScript(req.ID, args["script"], args["flags"])

	case "get_report":
		return s.getReport(req.ID, args["path"])

	case "generate_test":
		return s.generateTest(req.ID, args["prompt"], args["output"])

	case "run_tests":
		out, err := s.runProbe("test", args["paths"], args["tag"], args["flags"])
		return textResp(req.ID, out, err)

	case "list_files":
		out, err := s.runProbe("lint", args["path"], "", "--list")
		return textResp(req.ID, out, err)

	case "lint":
		out, err := s.runProbe("lint", args["paths"], "", "")
		return textResp(req.ID, out, err)

	case "take_screenshot":
		return s.takeScreenshot(req.ID, args["name"])

	default:
		return errResp(req.ID, -32602, "Unknown tool: "+params.Name)
	}
}

// ---- Tool implementations ----

func (s *Server) getWidgetTree(id any) *mcpResponse {
	script := "test \"mcp_widget_tree\"\n  dump widget tree\n"
	out, err := s.runInlineScript(script, "")
	if err != nil {
		return textResp(id, out, err)
	}
	// Extract [widget_tree]...[/widget_tree] block from output
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

func (s *Server) runScript(id any, script, flags string) *mcpResponse {
	if script == "" {
		return errResp(id, -32602, "script is required")
	}
	out, err := s.runInlineScript(script, flags)
	return textResp(id, out, err)
}

func (s *Server) getReport(id any, path string) *mcpResponse {
	if path == "" {
		// Auto-detect latest JSON report
		matches, _ := filepath.Glob(filepath.Join("reports", "*.json"))
		if len(matches) == 0 {
			return textResp(id, "No JSON reports found in reports/. Run tests first with --format json.", fmt.Errorf("no reports"))
		}
		path = matches[len(matches)-1]
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
	probePath := probeBin()
	cmdArgs := []string{"generate", "prompt", "--prompt", prompt}
	if output != "" {
		cmdArgs = append(cmdArgs, "--output", output)
	}
	cmd := exec.Command(probePath, cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return textResp(id, string(out), err)
}

func (s *Server) takeScreenshot(id any, name string) *mcpResponse {
	if name == "" {
		name = "mcp_capture"
	}
	script := fmt.Sprintf("test \"mcp screenshot\"\n  take screenshot \"%s\"\n", name)
	cmdOut, _ := s.runInlineScript(script, "")

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

// ---- Helpers ----

func (s *Server) runProbe(subcommand, paths, tag, extra string) (string, error) {
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
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *Server) runInlineScript(script, flags string) (string, error) {
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
