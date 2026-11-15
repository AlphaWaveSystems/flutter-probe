package runner

import (
	"context"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// TestSetAirplaneModeStepRunsItsState checks the executor path of `set airplane mode`: on and off each run their own
// command, on the device the run targets. A dropped case or a fixed state fails one of the two.
func TestSetAirplaneModeStepRunsItsState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state string
		want  string // the one host command the step must run
	}{
		{name: "on", state: "on", want: "adb -s emulator-5554 shell cmd connectivity airplane-mode enable"},
		{name: "off", state: "off", want: "adb -s emulator-5554 shell cmd connectivity airplane-mode disable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &commandRecorder{}
			dc := &DeviceContext{Platform: device.PlatformAndroid, Serial: "emulator-5554", settingsRun: rec}
			e := NewExecutor(nil, dc, nil, time.Second, false)
			step := parser.ActionStep{Verb: parser.VerbSetAirplaneMode, Name: tc.state}
			if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
				t.Fatalf("dispatchStep: %v", err)
			}
			if len(rec.cmds) != 1 || rec.cmds[0] != tc.want {
				t.Errorf("commands = %q, want [%q]", rec.cmds, tc.want)
			}
		})
	}
}

// TestSetAirplaneModeRefusesWhatItCannotDo checks that a physical device, an iOS simulator and a network adb address are
// refused before any command runs: the step must not cut the connection it depends on, or fake a change it cannot make.
func TestSetAirplaneModeRefusesWhatItCannotDo(t *testing.T) {
	for _, tc := range []struct {
		name string
		dc   *DeviceContext
	}{
		{name: "a physical device", dc: &DeviceContext{Platform: device.PlatformAndroid, Serial: "R58M123ABC", IsPhysical: true}},
		{name: "an iOS simulator", dc: &DeviceContext{Platform: device.PlatformIOS, Serial: "AAAAAAAA-0000-0000-0000-000000000000"}},
		{name: "a network adb address", dc: &DeviceContext{Platform: device.PlatformAndroid, Serial: "192.168.1.20:5555"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &commandRecorder{}
			tc.dc.settingsRun = rec
			e := NewExecutor(nil, tc.dc, nil, time.Second, false)
			step := parser.ActionStep{Verb: parser.VerbSetAirplaneMode, Name: "on"}
			if err := e.dispatchStep(context.Background(), context.Background(), step); err == nil {
				t.Error("the step ran, want a refusal")
			}
			if len(rec.cmds) != 0 {
				t.Errorf("commands = %q, want none", rec.cmds)
			}
		})
	}
}

// TestSetAirplaneModeWithoutADeviceRunsNothing checks the cloud-mode path: with no device the step is skipped.
func TestSetAirplaneModeWithoutADeviceRunsNothing(t *testing.T) {
	e := NewExecutor(nil, nil, nil, time.Second, false)
	step := parser.ActionStep{Verb: parser.VerbSetAirplaneMode, Name: "on"}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("dispatchStep: %v", err)
	}
}
