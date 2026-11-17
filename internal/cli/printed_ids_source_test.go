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
	if n := strings.Count(src, "uploadAndStartCloudSession(ctx, provider, cloudApp, targetDevice, statusW)"); n != 2 {
		t.Errorf("both cloud paths must upload and start through uploadAndStartCloudSession: %d of 2", n)
	}
	if !strings.Contains(src, `"Connected to ProbeAgent via relay on %s (%s)", device.CleanOneLine(targetDevice, 80)`) {
		t.Error("the relay connect line must print the device cleaned")
	}
	for _, raw := range []string{`"Connected to ProbeAgent on %s", deviceSerial)`, `"App uploaded: %s", appID)`, `reachable", deviceSerial)`, `"Starting cloud session on %s (%s)...", targetDevice,`, `via relay on %s (%s)", targetDevice,`} {
		if strings.Contains(src, raw) {
			t.Errorf("a raw print is back: %s", raw)
		}
	}
}
