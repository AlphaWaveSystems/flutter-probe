package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// The simulator's token path is built from its serial and later removed (os.Remove before a reconnect). A serial that is not a
// simulator UDID gives no path, so it cannot point the removal at another file.
func TestIOSTokenPathOnlyForASimulatorUDID(t *testing.T) {
	for _, serial := range []string{"../../x", "/etc", "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B/../../x", "*", "", "00008110-001A2C3E0E12801E"} {
		dc := &DeviceContext{Manager: device.NewManager(), Serial: serial, Platform: device.PlatformIOS}
		if p := dc.iosTokenPath(); p != "" {
			t.Errorf("iosTokenPath for %q = %q, want none", serial, p)
		}
	}
	const sim = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"
	dc := &DeviceContext{Manager: device.NewManager(), Serial: sim, Platform: device.PlatformIOS}
	if p := dc.iosTokenPath(); !strings.HasSuffix(p, "/Library/Developer/CoreSimulator/Devices/"+sim+"/data/tmp/probe/token") {
		t.Errorf("iosTokenPath for a simulator = %q", p)
	}
}
