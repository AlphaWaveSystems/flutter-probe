package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// commandRecorder is a host-command runner that records each command and runs nothing.
type commandRecorder struct{ cmds []string }

func (r *commandRecorder) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.cmds = append(r.cmds, name+" "+strings.Join(args, " "))
	return nil, nil
}

// TestSetAppearanceStepRunsItsValue checks the executor path of `set appearance`: the step runs the value it was
// written with, on the device the run targets. A dropped case or a fixed value fails one of the modes.
func TestSetAppearanceStepRunsItsValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		want string // the one host command the step must run
	}{
		{name: "light", mode: "light", want: "adb -s emulator-5554 shell cmd uimode night no"},
		{name: "dark", mode: "dark", want: "adb -s emulator-5554 shell cmd uimode night yes"},
		{name: "auto", mode: "auto", want: "adb -s emulator-5554 shell cmd uimode night auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &commandRecorder{}
			dc := &DeviceContext{Platform: device.PlatformAndroid, Serial: "emulator-5554", appearanceRun: rec}
			e := NewExecutor(nil, dc, nil, time.Second, false)
			step := parser.ActionStep{Verb: parser.VerbSetAppearance, Name: tc.mode}
			if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
				t.Fatalf("dispatchStep: %v", err)
			}
			if len(rec.cmds) != 1 || rec.cmds[0] != tc.want {
				t.Errorf("commands = %q, want [%q]", rec.cmds, tc.want)
			}
		})
	}
}

// TestSetAppearanceStepWithoutADeviceRunsNothing checks the cloud-mode path: with no device the step is skipped
// and nothing is run.
func TestSetAppearanceStepWithoutADeviceRunsNothing(t *testing.T) {
	e := NewExecutor(nil, nil, nil, time.Second, false)
	step := parser.ActionStep{Verb: parser.VerbSetAppearance, Name: "dark"}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("dispatchStep: %v", err)
	}
}
