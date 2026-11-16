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

	"github.com/spf13/pflag"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// TestCompositeIOSContextTakesTheClearGate: an iOS device of a composite run gets the run's clear gate, as an Android one does.
func TestCompositeIOSContextTakesTheClearGate(t *testing.T) {
	yes := compositeIOSContext(context.Background(), nil, "SIM-UDID-1", runner.ClearGate{Yes: true})
	if err := yes.AskClearData(false); err != nil {
		t.Errorf("composite iOS with --yes must wipe, got %v", err)
	}
	none := compositeIOSContext(context.Background(), nil, "SIM-UDID-1", runner.ClearGate{})
	if err := none.AskClearData(false); err == nil || !strings.Contains(err.Error(), "pass --yes (-y)") {
		t.Errorf("composite iOS without --yes or a lane confirmation must refuse with the hint, got %v", err)
	}
	var lanePlatform device.Platform
	var laneSerial string
	asked := compositeIOSContext(context.Background(), nil, "00008030-PHONE", runner.ClearGate{Lane: func(p device.Platform, s string) runner.ConfirmFunc {
		lanePlatform, laneSerial = p, s
		return func(string) bool { return true }
	}})
	if err := asked.AskClearData(false); err != nil {
		t.Errorf("the lane's confirmation said yes, so the clear must go ahead, got %v", err)
	}
	if lanePlatform != device.PlatformIOS || laneSerial != "00008030-PHONE" {
		t.Errorf("the lane confirmation got (%v, %q), want (iOS, 00008030-PHONE)", lanePlatform, laneSerial)
	}
}

// compositeGateFor runs the test command with one composite device and --yes as asked, and returns the clear gate its composite build was
// given. The build is a stub that fails, so no composite device is driven. The run uses a fake adb that lists one verified emulator, and
// --dry-run, so no real device is touched; the run is guarded like the other command tests.
func compositeGateFor(t *testing.T, yes bool) runner.ClearGate {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // the setting is read from the person's prefs file: keep the test off the real one
	dir := t.TempDir()
	t.Chdir(dir)
	bin, _ := clearGateADB(t)
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("project:\n  app: com.example.app\ntools:\n  adb: "+bin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "a.probe"), []byte("test \"a\"\n  tap \"A\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevList := listDevices
	listDevices = listing([]device.Device{{ID: "emulator-5590", Name: "Pixel", Platform: device.PlatformAndroid}})
	t.Cleanup(func() { listDevices = prevList })

	var got runner.ClearGate
	built := false
	prevBuild := buildCompositeRunnerFn
	buildCompositeRunnerFn = func(_ context.Context, _ *config.Config, _ *device.Manager, _ []string, _ time.Duration, _ bool, g runner.ClearGate) (*runner.CompositeRunner, func(), error) {
		built = true
		got = g
		return nil, func() {}, errors.New("stub: no composite device in this test")
	}
	t.Cleanup(func() { buildCompositeRunnerFn = prevBuild })

	cmd := testCmd
	if sv, ok := cmd.Flags().Lookup("composite-device").Value.(pflag.SliceValue); ok {
		if err := sv.Replace([]string{"A=emulator-5590"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Flags().Set("yes", strconv.FormatBool(yes)); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sv, ok := cmd.Flags().Lookup("composite-device").Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		}
		_ = cmd.Flags().Set("yes", "false")
		_ = cmd.Flags().Set("dry-run", "false")
	})
	done := make(chan error, 1)
	go func() { done <- runTests(cmd, []string{"tests"}) }()
	select {
	case runErr := <-done:
		if !built {
			t.Fatalf("the command returned before the composite build: %v", runErr)
		}
	case <-time.After(testRunGuard):
		t.Fatalf("the test command did not return within %v", testRunGuard)
	}
	return got
}

// TestCompositeRunGetsTheYesFlag: the composite run's clear gate answers --yes the way a parallel run's does. The call site passes the flag.
func TestCompositeRunGetsTheYesFlag(t *testing.T) {
	if !compositeGateFor(t, true).Yes {
		t.Error("composite run with --yes: its clear gate must allow a clear, it does not")
	}
	if compositeGateFor(t, false).Yes {
		t.Error("composite run without --yes: its clear gate must not allow a clear")
	}
}
