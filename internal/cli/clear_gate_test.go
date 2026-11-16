package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// clearGateADB is an adb that lists one emulator, answers ro.kernel.qemu with 1, and logs every call to the returned file. A lane that
// checks its device shows up in the log; a lane that does not, does not.
func clearGateADB(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "adb")
	log := filepath.Join(dir, "adb.log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + log + "'\n" +
		"case \"$*\" in\n" +
		"  *devices*) printf 'List of devices attached\\nemulator-5590\\tdevice product:sdk model:sdk device:emu64a transport_id:1\\n' ;;\n" +
		"  *getprop*qemu*) printf '1\\n' ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

// TestClearGateLanes runs the real gate of a parallel lane, built from --yes and the person's setting, over the lanes a run can have. It
// checks what wipes, and that a lane looks at its device only when it has to answer the question: never with --yes, never with the
// setting off.
func TestClearGateLanes(t *testing.T) {
	bin, log := clearGateADB(t)
	dm := device.NewManagerWithPaths(device.ToolPaths{ADB: bin})
	ctx := context.Background()
	for _, tc := range []struct {
		name         string
		yes          bool
		withoutAsk   bool // the person's setting for simulators and emulators
		platform     device.Platform
		serial       string
		wantWipe     bool
		wantAdbCalls bool // the lane had to check its device to answer
	}{
		{"--yes wipes a physical phone", true, false, device.PlatformAndroid, "R58M12ABCDE", true, false},
		{"--yes wipes an emulator without looking at it", true, true, device.PlatformAndroid, "emulator-5590", true, false},
		{"no --yes, setting off: an emulator is refused without a look", false, false, device.PlatformAndroid, "emulator-5590", false, false},
		{"no --yes, setting on: a verified emulator is wiped without a question", false, true, device.PlatformAndroid, "emulator-5590", true, true},
		{"no --yes, setting on: a physical phone is refused", false, true, device.PlatformAndroid, "R58M12ABCDE", false, true},
		{"no --yes, setting on: a serial the list does not know is refused", false, true, device.PlatformAndroid, "emulator-9999", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := logLength(t, log)
			gate := clearGateFor(ctx, dm, tc.yes, tc.withoutAsk, false, "com.example.app")
			dc := &runner.DeviceContext{Platform: tc.platform, Serial: tc.serial, AppID: "com.example.app"}
			dc.ApplyClearGate(gate)
			err := dc.AskClearData(false)
			wiped := err == nil
			if wiped != tc.wantWipe {
				t.Fatalf("wipe = %v (err %v), want %v", wiped, err, tc.wantWipe)
			}
			if !wiped && !strings.Contains(err.Error(), "pass --yes (-y)") {
				t.Errorf("a refused lane must fail with the hint to pass --yes, got %v", err)
			}
			if looked := logLength(t, log) > before; looked != tc.wantAdbCalls {
				t.Errorf("the lane checked its device = %v, want %v", looked, tc.wantAdbCalls)
			}
		})
	}
}

// TestClearGateLaneDoesNotCheckUntilAsked checks that the gate's device check is lazy: building the lane's confirmation costs no adb call,
// and the first call that asks makes it.
func TestClearGateLaneDoesNotCheckUntilAsked(t *testing.T) {
	bin, log := clearGateADB(t)
	dm := device.NewManagerWithPaths(device.ToolPaths{ADB: bin})
	before := logLength(t, log)
	gate := clearGateFor(context.Background(), dm, false, true, false, "com.example.app")
	confirm := gate.Lane(device.PlatformAndroid, "emulator-5590")
	if logLength(t, log) != before {
		t.Fatal("building a lane's confirmation must not check the device")
	}
	if confirm == nil || !confirm("clear?") {
		t.Fatal("a verified emulator with the setting on must be allowed")
	}
	if logLength(t, log) == before {
		t.Fatal("the first question must check the device")
	}
}

// TestCompositeContextTakesTheClearGate checks that a composite Android device gets the run's gate: --yes lets it wipe, and no gate refuses.
func TestCompositeContextTakesTheClearGate(t *testing.T) {
	bin, _ := clearGateADB(t)
	dm := device.NewManagerWithPaths(device.ToolPaths{ADB: bin})
	cfg := &config.Config{}
	yes := compositeAndroidContext(context.Background(), cfg, dm, "emulator-5590", runner.ClearGate{Yes: true})
	if err := yes.AskClearData(false); err != nil {
		t.Errorf("composite with --yes must wipe, got %v", err)
	}
	none := compositeAndroidContext(context.Background(), cfg, dm, "emulator-5590", runner.ClearGate{})
	if err := none.AskClearData(false); err == nil || !strings.Contains(err.Error(), "pass --yes (-y)") {
		t.Errorf("composite without --yes or a lane confirmation must refuse with the hint, got %v", err)
	}
}

func logLength(t *testing.T, log string) int {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return len(b)
}

// TestAutoLineSaysWhatWasAnswered: a clear the setting answers without a question says what it answered, the keychain reset of an iOS
// simulator included, so the line never reads as a clear of app data alone.
func TestAutoLineSaysWhatWasAnswered(t *testing.T) {
	var out bytes.Buffer
	question := "This will permanently delete ALL app data for com.example.app on SIM-UDID-1.\n" +
		"  SharedPreferences, databases, and files will be wiped. It also resets the keychain of ALL apps on this simulator. Continue?"
	if !confirmWithPrefs(nil, true, func() bool { return true }, &out, "com.example.app", "SIM-UDID-1")(question) {
		t.Fatal("a verified simulator with the setting on must answer yes")
	}
	line := out.String()
	if !strings.Contains(line, "also resets the keychain of ALL apps on this simulator") {
		t.Errorf("the automatic line must name the keychain reset, got %q", line)
	}
	if strings.Contains(line, "Continue?") {
		t.Errorf("the automatic line states what was answered; it must not read as a question, got %q", line)
	}
}
