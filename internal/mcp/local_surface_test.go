package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// premiumTerms are words that must not appear in the MCP's tool text, its bundle manifest or its docs page. Each one names
// a paid, hosted or licensed part of FlutterProbe, which the MCP does not offer. Matching ignores case.
var premiumTerms = []string{
	"probe_cloud", "probe cloud", "upload-report", "upload-local", "upload-dry-run",
	"probe license", "probe licence", "license key", "licence key",
	"hosted", "premium",
}

// TestMCPStaysOnTheLocalSurface fails when the MCP tool list, the bundle manifest or the MCP docs page names a paid,
// hosted or licensed feature.
func TestMCPStaysOnTheLocalSurface(t *testing.T) {
	toolJSON, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{"the MCP tool list": string(toolJSON)}
	paths := []string{"../../website/src/content/docs/tools/mcp.md"}
	for _, pattern := range []string{"../../mcpb/*.json", "../../mcpb/*/*.json"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		texts[p] = string(b)
	}
	for name, text := range texts {
		lower := strings.ToLower(text)
		for _, term := range premiumTerms {
			if strings.Contains(lower, term) {
				t.Errorf("%s names %q, which the MCP does not offer", name, term)
			}
		}
	}
}

// TestHostedUploadFlagsAreRefused checks the flags string: a hosted upload flag is refused, and other flags pass.
func TestHostedUploadFlagsAreRefused(t *testing.T) {
	for _, flags := range []string{"--upload-report", "--upload-local", "--upload-dry-run", "--timeout 10s --upload-local=true"} {
		err := refusedHostedFlag(flags)
		if err == nil || !strings.Contains(err.Error(), "hosted upload is not available through MCP") {
			t.Errorf("refusedHostedFlag(%q) = %v, want the hosted upload refusal", flags, err)
		}
	}
	for _, flags := range []string{"", "--timeout 10s", "--record --format junit", "--tag smoke"} {
		if err := refusedHostedFlag(flags); err != nil {
			t.Errorf("refusedHostedFlag(%q) = %v, want no refusal", flags, err)
		}
	}
}

// TestRefusedRunsNeverStartTheCLI checks the response a client gets from run_tests and run_script: an error that names
// the refusal. The refusal happens before the CLI is started, so no device or process is involved.
func TestRefusedRunsNeverStartTheCLI(t *testing.T) {
	s := &Server{}
	cases := map[string]*mcpResponse{
		"run_tests":  s.runTests(1, map[string]string{"flags": "--upload-report"}),
		"run_script": s.runScript(1, `test "x"`, "--upload-local", ""),
	}
	for name, resp := range cases {
		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("%s: result is %T, want a map", name, resp.Result)
		}
		if isError, _ := result["isError"].(bool); !isError {
			t.Errorf("%s: isError is not set for a hosted upload flag", name)
		}
		content, _ := result["content"].([]map[string]any)
		if len(content) == 0 {
			t.Fatalf("%s: no content in the refusal", name)
		}
		if text, _ := content[0]["text"].(string); !strings.Contains(text, "hosted upload is not available through MCP") {
			t.Errorf("%s: refusal text = %q, want the hosted upload message", name, text)
		}
	}
}
