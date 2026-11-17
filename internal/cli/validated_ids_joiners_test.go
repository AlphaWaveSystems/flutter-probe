package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

// oneline.Clean drops the joiners, so a<ZWJ>b prints as ab. An id that went through validation can never hold one: the --device serial
// (config.ValidateDeviceSerial), the app id of probe.yaml (config load), a package name read from the device (device.ValidPackageName) and a
// UDID (ios.ValidSimulatorUDID, ios.ValidDeviceUDID) refuse every joiner. A device name, and a serial picked from the device list without
// --device, are not validated: for them a joiner is dropped from the printed text.
func TestValidatedIDsNeverHoldAJoiner(t *testing.T) {
	for _, j := range []rune{0x200C, 0x200D, 0xFE00, 0xFE0F, 0xE0100, 0xE01EF} {
		js := string(j)
		if err := config.ValidateDeviceSerial("emulator" + js + "-5554"); err == nil {
			t.Errorf("ValidateDeviceSerial accepted %U", j)
		}
		if device.ValidPackageName("com.ex" + js + "ample") {
			t.Errorf("ValidPackageName accepted %U", j)
		}
		if ios.ValidSimulatorUDID("6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5" + js + "B") {
			t.Errorf("ValidSimulatorUDID accepted %U", j)
		}
		if ios.ValidDeviceUDID("00008110-001A2C3E0E12801" + js + "E") {
			t.Errorf("ValidDeviceUDID accepted %U", j)
		}
		cfgPath := filepath.Join(t.TempDir(), "probe.yaml")
		if err := os.WriteFile(cfgPath, []byte("project:\n  app: com.ex"+js+"ample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := config.LoadFile(cfgPath); err == nil {
			t.Errorf("a probe.yaml app id with %U was accepted", j)
		}
	}
}
