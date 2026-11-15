package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// fakeADBManager returns a device manager whose adb answers `getprop ro.hardware` with hardware, so the physical check
// runs without a device. The stand-in is a shell script in a temporary directory.
func fakeADBManager(t *testing.T, hardware string) *device.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\ncase \"$*\" in\n  *getprop*) echo " + hardware + " ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return device.NewManagerWithPaths(device.ToolPaths{ADB: path})
}

// parallelDeviceCases are the serials a parallel run can name, with what the device context must say and whether the
// airplane step must refuse the device. Only the emulator may take the step.
var parallelDeviceCases = []struct {
	name     string
	serial   string
	physical bool
	refused  bool
}{
	{name: "emulator", serial: "emulator-5554", physical: false, refused: false},
	{name: "USB device", serial: "R58M123ABC", physical: true, refused: true},
	{name: "wireless-debugging device", serial: "adb-R58-x._adb-tls-connect._tcp", physical: true, refused: true},
	{name: "network adb address", serial: "192.168.1.20:5555", physical: true, refused: true},
}

// TestParallelDeviceContextAsksTheDevice checks the device context of each device of a parallel run: IsPhysical comes
// from the device, and `set airplane mode` runs on an emulator only, whichever serial kind the run was given.
func TestParallelDeviceContextAsksTheDevice(t *testing.T) {
	po := NewParallelOrchestrator(&config.Config{}, fakeADBManager(t, "qcom"), nil, RunOptions{}, 0)
	for _, tc := range parallelDeviceCases {
		t.Run(tc.name, func(t *testing.T) {
			dr := &DeviceRun{DeviceID: tc.serial, Platform: device.PlatformAndroid, Port: 48790}
			dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
			if dc.IsPhysical != tc.physical {
				t.Errorf("IsPhysical = %v, want %v", dc.IsPhysical, tc.physical)
			}
			rec := &commandRecorder{}
			dc.SettingsRun = rec
			err := dc.SetAirplaneMode(context.Background(), true)
			if tc.refused {
				if err == nil {
					t.Error("the step ran on a device that is not an emulator, want a refusal")
				}
				if len(rec.cmds) != 0 {
					t.Errorf("commands = %q, want none", rec.cmds)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetAirplaneMode: %v", err)
			}
			if len(rec.cmds) != 1 || rec.cmds[0] != "adb -s emulator-5554 shell cmd connectivity airplane-mode enable" {
				t.Errorf("commands = %q, want the emulator's enable command", rec.cmds)
			}
		})
	}
}

// fakeXcrun puts an xcrun stand-in first on PATH. It answers `simctl list devices --json` with listJSON, and every other
// simctl command succeeds without output, so no simulator is needed.
func fakeXcrun(t *testing.T, listJSON string) {
	t.Helper()
	dir := t.TempDir()
	listPath := filepath.Join(dir, "list.json")
	if err := os.WriteFile(listPath, []byte(listJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n  *\"list devices\"*) cat '" + listPath + "' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestParallelIOSDeviceContextAsksSimctl checks the iOS device of a parallel run: a device that simctl does not list is
// physical, and `set language` refuses it, where a simulator takes the change. The simulator is the control.
func TestParallelIOSDeviceContextAsksSimctl(t *testing.T) {
	const simUDID = "AAAAAAAA-1111-2222-3333-444444444444"
	const physUDID = "BBBBBBBB-1111-2222-3333-444444444444"
	simList := `{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-26-4":[{"udid":"` + simUDID +
		`","name":"sim","state":"Shutdown","deviceTypeIdentifier":"x"}]}}`
	for _, tc := range []struct {
		name     string
		udid     string
		list     string
		physical bool
	}{
		{name: "a physical iPhone", udid: physUDID, list: `{"devices":{}}`, physical: true},
		{name: "a simulator", udid: simUDID, list: simList, physical: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeXcrun(t, tc.list)
			po := NewParallelOrchestrator(&config.Config{}, device.NewManager(), nil, RunOptions{}, 0)
			dr := &DeviceRun{DeviceID: tc.udid, Platform: device.PlatformIOS, Port: 48790}
			dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
			if dc.IsPhysical != tc.physical {
				t.Errorf("IsPhysical = %v, want %v", dc.IsPhysical, tc.physical)
			}
			err := dc.SetLanguage(context.Background(), "de")
			if tc.physical {
				if err == nil || !strings.Contains(err.Error(), "physical iOS") {
					t.Errorf("SetLanguage on a physical iPhone: err = %v, want the physical iOS refusal", err)
				}
				return
			}
			if err != nil {
				t.Errorf("SetLanguage on a simulator: %v", err)
			}
		})
	}
}

// TestParallelStepRunsOnTheDeviceItBelongsTo checks the dispatch of the step through a parallel run's device context:
// the step is refused on a physical device before any command runs, through the executor path a run uses.
func TestParallelStepRunsOnTheDeviceItBelongsTo(t *testing.T) {
	po := NewParallelOrchestrator(&config.Config{}, fakeADBManager(t, "qcom"), nil, RunOptions{}, 0)
	dr := &DeviceRun{DeviceID: "R58M123ABC", Platform: device.PlatformAndroid, Port: 48790}
	dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second)
	rec := &commandRecorder{}
	dc.SettingsRun = rec
	e := NewExecutor(nil, dc, nil, time.Second, false)
	step := parser.ActionStep{Verb: parser.VerbSetAirplaneMode, Name: "on"}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err == nil {
		t.Error("the step ran on a physical device, want a refusal")
	}
	if len(rec.cmds) != 0 {
		t.Errorf("commands = %q, want none", rec.cmds)
	}
}
