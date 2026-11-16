package mcp

import (
	"strings"
	"testing"
)

// TestRefusedConsentFlag: a --yes that reaches the CLI through any argument is refused; every other flag passes.
func TestRefusedConsentFlag(t *testing.T) {
	for _, tc := range []struct {
		value  string
		refuse bool
	}{
		{"-y", true},
		{"--yes", true},
		{"--yes=true", true},
		{"-vy", true},
		{"-yv", true},
		{"--parallel -y", true},
		{"--timeout 10s", false},
		{"--parallel", false},
		{"--device emulator-5590", false},
		{"-v", false},
		{"--yesterday", false},
		{"--tag smoke", false},
	} {
		err := refusedConsentFlag(tc.value)
		if tc.refuse && err == nil {
			t.Errorf("%q: want a refusal", tc.value)
		}
		if !tc.refuse && err != nil {
			t.Errorf("%q: want it to pass, got %v", tc.value, err)
		}
		if tc.refuse && err != nil && !strings.Contains(err.Error(), "--yes is not available through MCP") {
			t.Errorf("%q: the refusal must say --yes is not available through MCP, got %v", tc.value, err)
		}
	}
}

// TestBuildTestArgsRefusesConsentInEveryField: a -y in any field that becomes a CLI argument is refused, and the same fields without one
// build the command as before.
func TestBuildTestArgsRefusesConsentInEveryField(t *testing.T) {
	for _, field := range []string{"paths", "tag", "flags", "device", "composite_devices"} {
		if _, err := buildTestArgs(map[string]string{field: "-y"}); err == nil {
			t.Errorf("%s=-y must be refused", field)
		}
	}
	args, err := buildTestArgs(map[string]string{"paths": "tests/", "flags": "--parallel --timeout 10s", "device": "emulator-5590"})
	if err != nil {
		t.Fatalf("an ordinary run must build, got %v", err)
	}
	for _, want := range []string{"test", "tests/", "--parallel", "--timeout", "10s", "--device", "emulator-5590"} {
		found := false
		for _, a := range args {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the command must carry %q, got %v", want, args)
		}
	}
	for _, a := range args {
		if isConsentFlag(a) {
			t.Errorf("an ordinary run must carry no consent flag, got %v", args)
		}
	}
}

// TestRunScriptRefusesConsentFlag: run_script takes the flags and the device of an inline script. A --yes in either is refused by the real
// handler, with the refusal as the text the client gets back. A run that was not refused would instead answer with the CLI's own output,
// so the text is what tells the two apart.
func TestRunScriptRefusesConsentFlag(t *testing.T) {
	s := &Server{}
	for _, tc := range []struct{ flags, device string }{
		{"--yes", ""},
		{"-y", ""},
		{"--timeout 10s -vy", ""},
		{"", "-y"},
	} {
		resp := s.runScript(1, `test "x"`, tc.flags, tc.device)
		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("flags %q device %q: result is %T, want a map", tc.flags, tc.device, resp.Result)
		}
		content, _ := result["content"].([]map[string]any)
		text := ""
		if len(content) > 0 {
			text, _ = content[0]["text"].(string)
		}
		if !strings.Contains(text, "--yes is not available through MCP") {
			t.Errorf("flags %q device %q: a --yes must be refused with the MCP refusal, got %q", tc.flags, tc.device, text)
		}
	}
}
