package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// compositeFakeADB returns a device manager whose adb answers `getprop ro.hardware` with hardware, so the physical check
// runs without a device. The stand-in is a shell script in a temporary directory.
func compositeFakeADB(t *testing.T, hardware string) *device.Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\ncase \"$*\" in\n  *getprop*) echo " + hardware + " ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return device.NewManagerWithPaths(device.ToolPaths{ADB: path})
}

// compositeRecorder records the host commands and runs nothing.
type compositeRecorder struct{ cmds []string }

func (r *compositeRecorder) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.cmds = append(r.cmds, name+" "+strings.Join(args, " "))
	return nil, nil
}

// TestCompositeDeviceContextAsksTheDevice checks the device context of an Android device in a composite run: IsPhysical
// is asked of the device, and `set airplane mode` runs on an emulator only, whichever serial kind the run was given.
func TestCompositeDeviceContextAsksTheDevice(t *testing.T) {
	ctx := context.Background()
	dm := compositeFakeADB(t, "qcom")
	cfg := &config.Config{}
	for _, tc := range []struct {
		name     string
		serial   string
		physical bool
		refused  bool
	}{
		{name: "emulator", serial: "emulator-5554", physical: false, refused: false},
		{name: "USB device", serial: "R58M123ABC", physical: true, refused: true},
		{name: "wireless-debugging device", serial: "adb-R58-x._adb-tls-connect._tcp", physical: true, refused: true},
		{name: "network adb address", serial: "192.168.1.20:5555", physical: true, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := compositeAndroidContext(ctx, cfg, dm, tc.serial, runner.ClearGate{})
			if dc.IsPhysical != tc.physical {
				t.Errorf("IsPhysical = %v, want %v", dc.IsPhysical, tc.physical)
			}
			rec := &compositeRecorder{}
			dc.SettingsRun = rec
			err := dc.SetAirplaneMode(ctx, true)
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
