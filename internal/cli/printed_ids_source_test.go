package cli

import (
	"os"
	"strings"
	"testing"
)

// probe test prints the device it connected to, the id a cloud provider returned for the upload and the device it checks with -v: each is
// text from outside the program, so each is printed through device.CleanOneLine. The prints sit deep in the command, so the source is checked.
func TestProbeTestPrintsDeviceAndUploadIDsCleaned(t *testing.T) {
	b, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		`statusOK(statusW, "Connected to ProbeAgent on %s", device.CleanOneLine(deviceSerial, 80))`,
		`statusOK(statusW, "Connected to ProbeAgent on %s (%s)", device.CleanOneLine(targetDevice, 80), provider.Name())`,
		`trace("android: ensuring adb + device %s reachable", device.CleanOneLine(deviceSerial, 80))`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("probe test must print cleaned: %s", want)
		}
	}
	if n := strings.Count(src, `statusOK(statusW, "App uploaded: %s", device.CleanOneLine(appID, 80))`); n != 2 {
		t.Errorf("both cloud uploads must print the provider's id cleaned: %d of 2", n)
	}
	for _, raw := range []string{`"Connected to ProbeAgent on %s", deviceSerial)`, `"App uploaded: %s", appID)`, `reachable", deviceSerial)`} {
		if strings.Contains(src, raw) {
			t.Errorf("a raw print is back: %s", raw)
		}
	}
}
