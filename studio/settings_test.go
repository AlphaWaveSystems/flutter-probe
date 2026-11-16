package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func readYAML(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSaveWorkspaceSettingsKeepsEverythingElseInProbeYAML(t *testing.T) {
	dir := t.TempDir()
	original := "project:\n  name: Demo\n  app: com.example.demo\nrecipes_folder: tests/recipes\ncloud:\n  provider: browserstack\nagent:\n  port: 48686\n  dial_timeout: 45s\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800, DefaultsTimeout: "20s", IOSDeviceID: "SIM-UDID", AndroidDeviceID: "emulator-5554"}); err != nil {
		t.Fatal(err)
	}
	m := readYAML(t, dir)
	if m["recipes_folder"] != "tests/recipes" || m["project"].(map[string]any)["app"] != "com.example.demo" || m["cloud"].(map[string]any)["provider"] != "browserstack" {
		t.Errorf("unmanaged keys were lost: %v", m)
	}
	agent := m["agent"].(map[string]any)
	if agent["port"] != 48800 || agent["dial_timeout"] != "45s" {
		t.Errorf("agent = %v; the port changes and dial_timeout stays", agent)
	}
	if m["defaults"].(map[string]any)["timeout"] != "20s" || m["device"].(map[string]any)["ios_device_id"] != "SIM-UDID" || m["device"].(map[string]any)["android_device_id"] != "emulator-5554" {
		t.Errorf("managed keys not written: %v", m)
	}

	got, err := a.LoadWorkspaceSettings(dir)
	if err != nil || got.AgentPort != 48800 || got.DefaultsTimeout != "20s" || got.IOSDeviceID != "SIM-UDID" || got.AndroidDeviceID != "emulator-5554" {
		t.Errorf("load = %+v, err %v", got, err)
	}
}

func TestSaveWorkspaceSettingsRemovesClearedKeys(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800, IOSDeviceID: "X"}); err != nil {
		t.Fatal(err)
	}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800}); err != nil {
		t.Fatal(err)
	}
	if dev, ok := readYAML(t, dir)["device"].(map[string]any); ok && dev["ios_device_id"] != nil {
		t.Errorf("a cleared field must leave probe.yaml: %v", dev)
	}
}

func TestSaveWorkspaceSettingsRefusesBadInputBeforeTouchingTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	if err := os.WriteFile(path, []byte("agent:\n  port: 48686\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	for _, bad := range []WorkspaceSettings{{DefaultsTimeout: "soon"}, {AgentPort: 70000}, {AgentPort: -1}} {
		if err := inWS(a, dir).SaveWorkspaceSettings(dir, bad); err == nil {
			t.Errorf("%+v must be refused", bad)
		}
	}
	if err := a.SaveWorkspaceSettings("", WorkspaceSettings{}); err == nil {
		t.Error("an empty workspace must be refused")
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "48686") || len(b) != len("agent:\n  port: 48686\n") {
		t.Errorf("the file changed after a refused save: %q", b)
	}
	// A probe.yaml that is not valid YAML is reported, not overwritten.
	if err := os.WriteFile(path, []byte("agent: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 1}); err == nil {
		t.Error("an unparsable probe.yaml must be reported")
	}
	if b, _ := os.ReadFile(path); string(b) != "agent: [unclosed\n" {
		t.Error("the unparsable file must be left as it was")
	}
}

// The three run knobs are written under defaults, read back, and removed when cleared; everything else in probe.yaml stays.
func TestSaveWorkspaceSettingsWritesTheRunKnobs(t *testing.T) {
	dir := t.TempDir()
	original := "project:\n  app: com.example.demo\ndefaults:\n  platform: ios\n  screenshots: on_failure\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{TestTimeout: "90s", RetryFailedTests: 2, ImplicitWait: "2s"}); err != nil {
		t.Fatal(err)
	}
	d := readYAML(t, dir)["defaults"].(map[string]any)
	if d["test_timeout"] != "90s" || d["retry_failed_tests"] != 2 || d["implicit_wait"] != "2s" || d["platform"] != "ios" || d["screenshots"] != "on_failure" {
		t.Errorf("defaults = %v", d)
	}
	got, err := a.LoadWorkspaceSettings(dir)
	if err != nil || got.TestTimeout != "1m30s" || got.RetryFailedTests != 2 || got.ImplicitWait != "2s" {
		t.Errorf("load = %+v, %v", got, err)
	}
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{}); err != nil {
		t.Fatal(err)
	}
	d2, _ := readYAML(t, dir)["defaults"].(map[string]any)
	for _, k := range []string{"test_timeout", "retry_failed_tests", "implicit_wait"} {
		if _, ok := d2[k]; ok {
			t.Errorf("%s was not removed when cleared: %v", k, d2)
		}
	}
	if d2["platform"] != "ios" {
		t.Errorf("a sibling key was lost: %v", d2)
	}
}

// What the form refuses is refused before anything is written, with the CLI's own rules for the time limit.
func TestSaveWorkspaceSettingsRefusesBadRunKnobs(t *testing.T) {
	for name, tc := range map[string]struct {
		s    WorkspaceSettings
		want string
	}{
		"a limit that is not a duration": {WorkspaceSettings{TestTimeout: "ninety"}, "not a valid duration"},
		"a negative limit":               {WorkspaceSettings{TestTimeout: "-5s"}, "cannot be negative"},
		"a limit under a second":         {WorkspaceSettings{TestTimeout: "500ms"}, "at least 1s"},
		"negative retries":               {WorkspaceSettings{RetryFailedTests: -1}, "outside 0 to 10"},
		"too many retries":               {WorkspaceSettings{RetryFailedTests: 11}, "outside 0 to 10"},
		"a wait that is not a duration":  {WorkspaceSettings{ImplicitWait: "soon"}, "not a valid duration"},
		"a negative wait":                {WorkspaceSettings{ImplicitWait: "-1s"}, "outside 0 to"},
		"a wait past five minutes":       {WorkspaceSettings{ImplicitWait: "6m"}, "outside 0 to"},
	} {
		dir := t.TempDir()
		err := inWS(&App{}, dir).SaveWorkspaceSettings(dir, tc.s)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "probe.yaml")); statErr == nil {
			t.Errorf("%s: probe.yaml was written", name)
		}
	}
}

// A probe.yaml set by hand beyond the form's bounds still saves other settings; only changing the value is held to the bounds.
func TestSaveWorkspaceSettingsKeepsAValueBeyondTheFormsBounds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("defaults:\n  retry_failed_tests: 15\n  implicit_wait: 10m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	loaded, err := a.LoadWorkspaceSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded.AgentPort = 48800
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, loaded); err != nil {
		t.Fatalf("saving an unrelated change was refused: %v", err)
	}
	d := readYAML(t, dir)["defaults"].(map[string]any)
	if d["retry_failed_tests"] != 15 || d["implicit_wait"] != "10m" {
		t.Errorf("defaults = %v", d)
	}
	loaded.RetryFailedTests = 16
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, loaded); err == nil {
		t.Error("a changed value beyond the bounds was accepted")
	}
}
