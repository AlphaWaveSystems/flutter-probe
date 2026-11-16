package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// testRunGuard is how long a test command may take to return. A command that should stop at start returns at once; one that starts a real run
// would hang on devices that do not exist, so the guard fails the test instead of letting it hang.
const testRunGuard = 20 * time.Second

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

// listing returns the device listing that always gives devices.
func listing(devices []device.Device) func(context.Context, *device.Manager) ([]device.Device, error) {
	return func(context.Context, *device.Manager) ([]device.Device, error) { return devices, nil }
}

// noDeviceADB is an adb with no device attached: `devices` lists none, and every other command fails as a device that is not online would.
func noDeviceADB(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *devices*) printf 'List of devices attached\\n\\n' ;;\n" +
		"  *) echo \"error: device not found or not online\" >&2; exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// runTestsWith runs the test command in a project with one test, with list as the device listing, and returns its error. The listing is
// replaced, so no device or adb is touched by the listing itself. The run is guarded: if the command has not returned within testRunGuard,
// the test fails instead of hanging, which a regression that starts a real run would otherwise do.
func runTestsWith(t *testing.T, list func(context.Context, *device.Manager) ([]device.Device, error), parallel bool, devicesFlag string) error {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	// The adb is a fake with no device attached, so the run reaches the device step and fails on the device, the same way on every machine.
	// Without it the step looks for adb on PATH, and a machine with no Android SDK fails on the tool instead of the device.
	adb := noDeviceADB(t)
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("project:\n  app: com.example.app\ntools:\n  adb: "+adb+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "a.probe"), []byte("test \"a\"\n  tap \"A\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := listDevices
	listDevices = list
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
	done := make(chan error, 1)
	go func() { done <- runTests(cmd, []string{"tests"}) }()
	select {
	case err := <-done:
		return err
	case <-time.After(testRunGuard):
		t.Fatalf("the test command did not return within %v: it started a run where it should have stopped", testRunGuard)
		return nil
	}
}

// runTestsWithDevices is runTestsWith with a fixed set of connected devices.
func runTestsWithDevices(t *testing.T, devices []device.Device, parallel bool, devicesFlag string) error {
	t.Helper()
	return runTestsWith(t, listing(devices), parallel, devicesFlag)
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

// A device listing that fails stops a parallel run with the listing's error, for a --devices list and for discovery alike. Before this
// check, a failed listing was taken for an empty one.
func TestRunTestsStopsOnAListingError(t *testing.T) {
	failing := func(context.Context, *device.Manager) ([]device.Device, error) {
		return nil, errors.New("adb not found")
	}
	for _, flag := range []string{androidSerial, ""} {
		err := runTestsWith(t, failing, true, flag)
		if err == nil || !strings.Contains(err.Error(), "list devices") || !strings.Contains(err.Error(), "adb not found") {
			t.Errorf("--devices %q: a failed listing must stop the run with its error, got %v", flag, err)
		}
	}
}

// Discovery with --parallel needs at least two connected devices: one device is refused with the count, before any run starts.
func TestRunTestsDiscoveryNeedsTwoDevices(t *testing.T) {
	err := runTestsWithDevices(t, connectedFixture[:1], true, "")
	if err == nil || err.Error() != "parallel mode requires 2+ connected devices (found 1)" {
		t.Errorf("one device must not start a parallel run, got %v", err)
	}
}

// A run without --parallel or --devices uses the first connected device, as before, even with two platforms attached: the refusal is only
// in the parallel path. The run then tries that device, so the error names it and not the second one.
func TestRunTestsDefaultRunUsesTheFirstConnectedDevice(t *testing.T) {
	err := runTestsWithDevices(t, connectedFixture, false, "")
	if err == nil {
		t.Fatal("the run has no device to reach, so it must fail, after choosing the first device")
	}
	msg := err.Error()
	if strings.Contains(msg, "one platform") || strings.Contains(msg, "parallel mode") {
		t.Errorf("a run without --parallel must not be refused for its devices, got %v", err)
	}
	if !strings.Contains(msg, androidSerial) || strings.Contains(msg, simUDID) {
		t.Errorf("the run must use the first connected device %s, got %v", androidSerial, err)
	}
}
