package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

const repairUDID = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"

func badIOSWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("agent:\n  port: 4444\ndevice:\n  ios_device_id: \"iPhone 16\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// An old probe.yaml with an iOS device that is not a UDID still opens in Settings: the value is shown with what is wrong and where to find
// the UDID, and saving a UDID repairs the file so a run loads it.
func TestSettingsShowAndRepairAnIOSDeviceThatIsNotAUDID(t *testing.T) {
	dir := badIOSWorkspace(t)
	a := inWS(&App{}, dir)
	loaded, err := a.LoadWorkspaceSettings(dir)
	if err != nil {
		t.Fatalf("Settings must open: %v", err)
	}
	if loaded.IOSDeviceID != "iPhone 16" || loaded.AgentPort != 4444 || !strings.Contains(loaded.IOSDeviceIDProblem, "probe device list") || !strings.Contains(loaded.IOSDeviceIDProblem, `"iPhone 16"`) {
		t.Fatalf("loaded = %+v", loaded)
	}
	form := loaded
	form.Base = &loaded
	form.IOSDeviceID = repairUDID
	if err := a.SaveWorkspaceSettings(dir, form); err != nil {
		t.Fatalf("the repairing save must land: %v", err)
	}
	cfg, err := config.Load(dir)
	if err != nil || cfg.Device.IOSDeviceID != repairUDID {
		t.Fatalf("after the repair, a run must load it: %+v, %v", cfg, err)
	}
	if again, _ := a.LoadWorkspaceSettings(dir); again.IOSDeviceIDProblem != "" {
		t.Errorf("a repaired file must show no problem: %q", again.IOSDeviceIDProblem)
	}
}

// Only a changed iOS device is held to the rule: with the old value left as it is, another field saves (no "changed on disk" either), and the
// old value stays for the person to fix.
func TestAnUnchangedBadIOSDeviceDoesNotBlockAnotherField(t *testing.T) {
	dir := badIOSWorkspace(t)
	a := inWS(&App{}, dir)
	loaded, err := a.LoadWorkspaceSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	form := loaded
	form.Base = &loaded
	form.AgentPort = 4555
	if err := a.SaveWorkspaceSettings(dir, form); err != nil {
		t.Fatalf("saving another field must land: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if !strings.Contains(string(data), "port: 4555") || !strings.Contains(string(data), "iPhone 16") {
		t.Errorf("probe.yaml = %q", data)
	}
}

// The value is trimmed before the rule: a UDID with spaces around it is a UDID, saved without them.
func TestAnIOSDeviceWithSpacesAroundIsTrimmed(t *testing.T) {
	dir := t.TempDir()
	if err := inWS(&App{}, dir).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48800, IOSDeviceID: "  " + repairUDID + " "}); err != nil {
		t.Fatalf("a UDID with spaces around must be accepted: %v", err)
	}
	if cfg, err := config.Load(dir); err != nil || cfg.Device.IOSDeviceID != repairUDID {
		t.Errorf("saved %+v, %v", cfg, err)
	}
}

// A probe.yaml the engine cannot read is refused with the real reason, not as "changed on disk since the form was loaded".
func TestAnUnreadableProbeYAMLIsNotCalledAChangeOnDisk(t *testing.T) {
	dir := t.TempDir()
	src := "agent:\n  port: 4444\ndefaults:\n  retry_failed_tests: many\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	base := WorkspaceSettings{AgentPort: 4444}
	form := base
	form.Base = &base
	form.AgentPort = 4555
	err := inWS(&App{}, dir).SaveWorkspaceSettings(dir, form)
	if err == nil || strings.Contains(err.Error(), "changed on disk") || !strings.Contains(err.Error(), "would not load") {
		t.Errorf("err = %v, want the real reason", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml")); string(data) != src {
		t.Error("the file changed")
	}
}
