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
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800, DefaultsTimeout: "20s", IOSDeviceID: "SIM-UDID", AndroidDeviceID: "emulator-5554"}); err != nil {
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
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800, IOSDeviceID: "X"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800}); err != nil {
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
		if err := a.SaveWorkspaceSettings(dir, bad); err == nil {
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
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 1}); err == nil {
		t.Error("an unparsable probe.yaml must be reported")
	}
	if b, _ := os.ReadFile(path); string(b) != "agent: [unclosed\n" {
		t.Error("the unparsable file must be left as it was")
	}
}
