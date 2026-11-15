package devsettings

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPlanAirplaneMode checks the one command the step runs, and that every other device is refused before anything runs.
func TestPlanAirplaneMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		p      Platform
		device string
		on     bool
		want   string // the command; "" when the plan must be refused
	}{
		{name: "android on", p: Android, device: "emulator-5554", on: true,
			want: "adb -s emulator-5554 shell cmd connectivity airplane-mode enable"},
		{name: "android off", p: Android, device: "emulator-5554", on: false,
			want: "adb -s emulator-5554 shell cmd connectivity airplane-mode disable"},
		{name: "a network adb address is refused", p: Android, device: "192.168.1.20:5555", on: true},
		{name: "booted is not a device", p: Android, device: "booted", on: true},
		{name: "no device", p: Android, device: "", on: false},
		{name: "device with shell characters", p: Android, device: "emulator-5554; reboot", on: true},
		{name: "an iOS simulator is refused", p: IOS, device: "AAAAAAAA-0000-0000-0000-000000000000", on: true},
		{name: "unknown platform", p: "windows", device: "x1", on: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmds, err := PlanAirplaneMode(tc.p, tc.device, tc.on)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("planned %v, want a refusal", cmds)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if len(cmds) != 1 || cmds[0].String() != tc.want {
				t.Errorf("commands = %v, want [%q]", cmds, tc.want)
			}
		})
	}
}

// airplaneRunner answers every host command with the same output and error, and records the commands.
type airplaneRunner struct {
	out  string
	err  error
	cmds []string
}

func (r *airplaneRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.cmds = append(r.cmds, name+" "+strings.Join(args, " "))
	return []byte(r.out), r.err
}

// TestApplyCommandsReportsFailures checks that a command the device or the host refuses is an error, even when adb
// itself exits 0, as it does when the on-device command prints an error.
func TestApplyCommandsReportsFailures(t *testing.T) {
	cmds, err := PlanAirplaneMode(Android, "emulator-5554", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		run     *airplaneRunner
		wantErr string // a substring of the error; "" when the change must succeed
	}{
		{name: "the command works", run: &airplaneRunner{out: ""}},
		{name: "the device prints an error", run: &airplaneRunner{out: "Error: unknown command: airplane-mode"}, wantErr: "the device refused the change"},
		{name: "the host command fails", run: &airplaneRunner{out: "no devices", err: errors.New("exit status 1")}, wantErr: "adb failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ApplyCommands(context.Background(), tc.run, Android, cmds)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ApplyCommands: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}
