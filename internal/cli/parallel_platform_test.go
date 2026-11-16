package cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/pflag"
)

const (
	androidSerial   = "emulator-7001"
	simUDID         = "0B1C2D3E-4F5A-6B7C-8D9E-0F1A2B3C4D5E"
	physicalIPhone  = "00008030-001A2B3C4D5E002E"
	wirelessAndroid = "adb-R58M12ABCDE-x1y2._adb-tls-connect._tcp"
)

// connectedFixture is the set of devices a run sees: an emulator, a simulator, a physical iPhone and a wireless-debugging Android device.
var connectedFixture = []device.Device{
	{ID: androidSerial, Name: "Pixel", Platform: device.PlatformAndroid},
	{ID: simUDID, Name: "iPhone 15", Platform: device.PlatformIOS},
	{ID: physicalIPhone, Name: "Jo's iPhone", Platform: device.PlatformIOS},
	{ID: wirelessAndroid, Name: "Tablet", Platform: device.PlatformAndroid},
}

// A --devices serial takes the platform the device reports, not one guessed from its shape: a physical iPhone has one dash in its id, and a
// wireless-debugging Android serial has more.
func TestDevicesFlagTakesThePlatformEachDeviceReports(t *testing.T) {
	runs, err := devicesFlagRuns(strings.Join([]string{physicalIPhone, simUDID, wirelessAndroid, androidSerial}, ","), "com.example.app", connectedFixture)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]device.Platform{
		physicalIPhone:  device.PlatformIOS,
		simUDID:         device.PlatformIOS,
		wirelessAndroid: device.PlatformAndroid,
		androidSerial:   device.PlatformAndroid,
	}
	if len(runs) != len(want) {
		t.Fatalf("want one run per serial, got %+v", runs)
	}
	for _, r := range runs {
		if r.Platform != want[r.DeviceID] {
			t.Errorf("%s is %s, the device reports %s", r.DeviceID, r.Platform, want[r.DeviceID])
		}
	}
}

// A serial that is not connected is refused, not guessed at.
func TestDevicesFlagRefusesADeviceNotConnected(t *testing.T) {
	_, err := devicesFlagRuns("emulator-9999", "com.example.app", connectedFixture)
	if err == nil || !strings.Contains(err.Error(), `"emulator-9999" is not connected`) {
		t.Errorf("an unknown serial must be refused, got %v", err)
	}
}

// Devices of two platforms are refused, and the --devices message names the flag's remedy.
func TestOnePlatformErrorRefusesAMixedList(t *testing.T) {
	mixed := []runner.DeviceRun{{DeviceID: androidSerial, Platform: device.PlatformAndroid}, {DeviceID: simUDID, Platform: device.PlatformIOS}}
	err := onePlatformError(mixed, true)
	if err == nil {
		t.Fatal("a --devices list of Android and iOS devices must be refused")
	}
	if want := "--devices must list devices of one platform; run one probe test per platform"; err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

// Discovered devices of two platforms are refused with a message that says which platforms, and how to choose one.
func TestOnePlatformErrorNamesThePlatformsOfADiscoveredMix(t *testing.T) {
	mixed := []runner.DeviceRun{{DeviceID: androidSerial, Platform: device.PlatformAndroid}, {DeviceID: simUDID, Platform: device.PlatformIOS}}
	err := onePlatformError(mixed, false)
	want := "parallel mode found devices of Android and iOS; use --devices with the serials of one platform, or run one probe test per platform"
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}

// One platform, or no devices, is not refused.
func TestOnePlatformErrorAllowsOnePlatform(t *testing.T) {
	android := []runner.DeviceRun{{DeviceID: androidSerial, Platform: device.PlatformAndroid}, {DeviceID: wirelessAndroid, Platform: device.PlatformAndroid}}
	ios := []runner.DeviceRun{{DeviceID: simUDID, Platform: device.PlatformIOS}, {DeviceID: physicalIPhone, Platform: device.PlatformIOS}}
	for name, runs := range map[string][]runner.DeviceRun{"android": android, "ios": ios, "none": nil} {
		for _, fromFlag := range []bool{true, false} {
			if err := onePlatformError(runs, fromFlag); err != nil {
				t.Errorf("%s (flag %v) must not be refused, got %v", name, fromFlag, err)
			}
		}
	}
}

// A run without --device uses the first connected device, whatever its platform, as before. The refusal does not touch that choice: it is
// only in the parallel path.
func TestDefaultDeviceIsTheFirstConnected(t *testing.T) {
	first, ok := defaultDevice(connectedFixture)
	if !ok || first.ID != androidSerial {
		t.Errorf("the default device must be the first connected one, got %+v", first)
	}
	if _, ok := defaultDevice(nil); ok {
		t.Error("with no device there is no default")
	}
}

// runTestsWithDevices runs the test command in a project with one test, with devices as the connected devices, and returns its error. The
// tests replace the device listing, so no device or adb is touched.
func runTestsWithDevices(t *testing.T, devices []device.Device, parallel bool, devicesFlag string) error {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("project:\n  app: com.example.app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "a.probe"), []byte("test \"a\"\n  tap \"A\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := listDevices
	listDevices = func(ctx context.Context, dm *device.Manager) ([]device.Device, error) { return devices, nil }
	t.Cleanup(func() { listDevices = prev })
	cmd := testCmd
	// A test of --property leaves an empty entry in the flag; clear it so this run does not inherit it.
	if sv, ok := cmd.Flags().Lookup("property").Value.(pflag.SliceValue); ok {
		if err := sv.Replace(nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Flags().Set("parallel", strconv.FormatBool(parallel)); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("devices", devicesFlag); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Flags().Set("parallel", "false")
		cmd.Flags().Set("devices", "")
	})
	return runTests(cmd, []string{"tests"})
}

// The command refuses a --devices list of two platforms before it connects to anything: the error is the refusal, not a device failure.
func TestRunTestsRefusesAMixedDevicesList(t *testing.T) {
	err := runTestsWithDevices(t, connectedFixture, true, androidSerial+","+simUDID)
	if err == nil || err.Error() != "--devices must list devices of one platform; run one probe test per platform" {
		t.Errorf("a mixed --devices list must be refused at start, got %v", err)
	}
}

// The command refuses an unknown serial with the not-connected message.
func TestRunTestsRefusesAnUnknownDevice(t *testing.T) {
	err := runTestsWithDevices(t, connectedFixture, true, androidSerial+",emulator-9999")
	if err == nil || !strings.Contains(err.Error(), `"emulator-9999" is not connected`) {
		t.Errorf("an unknown serial must be refused, got %v", err)
	}
}

// Discovery with --parallel and devices of two platforms attached is refused with the discovery message.
func TestRunTestsRefusesADiscoveredMix(t *testing.T) {
	err := runTestsWithDevices(t, connectedFixture, true, "")
	if err == nil || !strings.HasPrefix(err.Error(), "parallel mode found devices of Android and iOS") {
		t.Errorf("a discovered mix must be refused, got %v", err)
	}
}
