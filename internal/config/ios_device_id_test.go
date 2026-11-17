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
