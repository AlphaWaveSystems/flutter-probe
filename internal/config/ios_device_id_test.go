package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// device.ios_device_id must be a simulator or device UDID: anything else stops the config load with a message that names the key.
func TestIOSDeviceIDMustBeAUDID(t *testing.T) {
	load := func(id string) error {
		p := filepath.Join(t.TempDir(), "probe.yaml")
		if err := os.WriteFile(p, []byte("device:\n  ios_device_id: \""+id+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadFile(p)
		return err
	}
	for _, ok := range []string{"", "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B", "00008110-001A2C3E0E12801E", "0123456789abcdef0123456789abcdef01234567"} {
		if err := load(ok); err != nil {
			t.Errorf("%q must load: %v", ok, err)
		}
	}
	for _, bad := range []string{"../../x", "/etc", "iPhone 16", "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B/../x", "*"} {
		if err := load(bad); err == nil || !strings.Contains(err.Error(), "device.ios_device_id") {
			t.Errorf("%q must be refused naming the key, got %v", bad, err)
		}
	}
}

// The load error points at `probe device list`, and LoadForRepair keeps the value and reports it on its own.
func TestIOSDeviceIDErrorAndRepairLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("device:\n  ios_device_id: \"iPhone 16\"\nagent:\n  port: 4444\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "probe device list") {
		t.Errorf("Load must point at probe device list: %v", err)
	}
	cfg, iosErr, err := LoadForRepair(dir)
	if err != nil || iosErr == nil || iosErr.ID != "iPhone 16" || cfg.Device.IOSDeviceID != "iPhone 16" || cfg.Agent.Port != 4444 {
		t.Errorf("LoadForRepair = %+v, %v, %v; want the config with the value kept and the iOS error on its own", cfg, iosErr, err)
	}
	// Every other rule still fails the repair load.
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("project:\n  app: \"bad app\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadForRepair(dir); err == nil {
		t.Error("LoadForRepair must still refuse an invalid project.app")
	}
}
