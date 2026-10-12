//go:build studio_e2e

// Studio E2E suite — drives the real FlutterProbe Studio app on macOS
// through the accessibility tree and screenshots. This is the smoke set: the
// launch and the recorder, the two checks that need the real window. Every
// other UI check runs in the headless browser suite (studio/e2e-web) or in the
// device tests on a named simulator. Run with:
//
//	make studio-e2e-native     (from the repo root; builds everything first)
//	go test -tags studio_e2e ./studio/e2e/ -run . -v -count=1   (prebuilt)
//
// Environment (all have defaults for the bundled fixture app, except the display):
//
//	STUDIO_E2E_DISPLAY    REQUIRED: the second display the windows open on (2 or more); see display.go
//	STUDIO_E2E_APP        path to .../Contents/MacOS/flutter-probe-studio
//	STUDIO_E2E_WORKSPACE  workspace folder Studio opens (must hold probe.yaml)
//	STUDIO_E2E_APP_BUNDLE path to the fixture .app built for the simulator ("" = already installed)
//	STUDIO_E2E_BUNDLE_ID  bundle id of the app under test
//	STUDIO_E2E_SIM_NAME   named simulator to use/create (default StudioE2E-iPhone17)
//	STUDIO_E2E_SIM_TYPE   simctl device type (default iPhone 17)
//	STUDIO_E2E_SIM_RUNTIME simctl runtime (default newest iOS)
//	STUDIO_E2E_OUT        report directory (default reports/studio-e2e)
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	studio   *Studio
	sim      *Simulator
	recorder *Recorder
	cfg      suiteConfig
	ctx      = context.Background()
)

type suiteConfig struct {
	App, Workspace                                         string
	AppBundle, BundleID, SimName, SimType, SimRuntime, Out string
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func repoRoot() string {
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func TestMain(m *testing.M) {
	// Refuse before anything is created or launched: the suite runs only on a
	// second display that STUDIO_E2E_DISPLAY names (display.go).
	if _, _, _, err := suiteDisplay(); err != nil {
		fmt.Fprintln(os.Stderr, "studio-e2e: refusing to run:", err)
		os.Exit(2)
	}
	root := repoRoot()
	fixture := filepath.Join(root, "native-test-apps", "studio-fixture")
	cfg = suiteConfig{
		App:        envOr("STUDIO_E2E_APP", filepath.Join(root, "studio", "build", "bin", "flutter-probe-studio.app", "Contents", "MacOS", "flutter-probe-studio")),
		Workspace:  envOr("STUDIO_E2E_WORKSPACE", fixture),
		AppBundle:  envOr("STUDIO_E2E_APP_BUNDLE", filepath.Join(fixture, "build", "ios", "iphonesimulator", "Runner.app")),
		BundleID:   envOr("STUDIO_E2E_BUNDLE_ID", "com.alphawavesystems.studioFixture"),
		SimName:    envOr("STUDIO_E2E_SIM_NAME", "StudioE2E-iPhone17"),
		SimType:    envOr("STUDIO_E2E_SIM_TYPE", "com.apple.CoreSimulator.SimDeviceType.iPhone-17"),
		SimRuntime: envOr("STUDIO_E2E_SIM_RUNTIME", ""),
		Out:        envOr("STUDIO_E2E_OUT", filepath.Join(root, "reports", "studio-e2e")),
	}
	recorder = &Recorder{OutDir: cfg.Out}

	var err error
	sim, err = EnsureSimulator(ctx, cfg.SimName, cfg.SimType, cfg.SimRuntime)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simulator:", err)
		os.Exit(2)
	}
	// Keep the simulator's own window on the suite's display too.
	if err := sim.MoveToTargetDisplay(); err != nil {
		fmt.Fprintln(os.Stderr, "simulator window:", err)
	}
	if cfg.AppBundle != "" {
		if _, statErr := os.Stat(cfg.AppBundle); statErr == nil {
			if err := sim.InstallAndLaunch(ctx, cfg.AppBundle, cfg.BundleID); err != nil {
				fmt.Fprintln(os.Stderr, "fixture app:", err)
				os.Exit(2)
			}
		} else {
			fmt.Fprintln(os.Stderr, "app bundle not found, assuming the app is installed and running:", cfg.AppBundle)
		}
	}
	code := m.Run()
	if studio != nil {
		studio.Close()
	}
	if err := recorder.Write(); err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
	}
	fmt.Fprintln(os.Stderr, "report:", filepath.Join(cfg.Out, "report.html"))
	os.Exit(code)
}

// run wraps a test: fresh Studio per test (self-contained), screenshot on
// failure, result recorded for the report, explicit cleanup.
func run(t *testing.T, body func(t *testing.T, s *Studio)) {
	t.Helper()
	start := time.Now()
	res := Result{Name: t.Name(), Device: sim.Name, DeviceID: sim.UDID}
	s, err := Launch(ctx, LaunchOptions{
		AppBinary: cfg.App,
		Workspace: cfg.Workspace,
		LogPath:   filepath.Join(cfg.Out, "logs", t.Name()+".log"),
	})
	if err != nil {
		res.Error = err.Error()
		res.DurationMs = time.Since(start).Milliseconds()
		recorder.Add(res)
		t.Fatalf("launch studio: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
		shot := filepath.Join(cfg.Out, "screenshots", t.Name()+".png")
		if err := s.Screenshot(shot); err == nil {
			res.Shots = append(res.Shots, shot)
		}
		s.Close()
		sim.Terminate(ctx, cfg.BundleID)
		_ = sim.InstallAndLaunch(ctx, cfg.AppBundle, cfg.BundleID)
		hideSimulator()
		res.Passed = !t.Failed()
		res.Skipped = t.Skipped()
		res.DurationMs = time.Since(start).Milliseconds()
		recorder.Add(res)
	}()
	body(t, s)
}

// hideSimulator keeps Simulator.app from staying in front after a launch;
// keystrokes in later tests must reach Studio.
func hideSimulator() {
	_ = exec.Command("osascript", "-e", `tell application "System Events" to set visible of process "Simulator" to false`).Run()
}

func must(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// connect selects the named simulator in the device picker and connects.
func connect(t *testing.T, s *Studio) {
	t.Helper()
	must(t, s.RefreshDevices(), "refresh devices")
	picked, err := s.SelectPopup(1, sim.Name)
	must(t, err, "select device")
	if !strings.Contains(picked, sim.Name) {
		t.Fatalf("picked %q, want %q", picked, sim.Name)
	}
	must(t, s.ClickButton("Connect"), "click connect")
	must(t, s.WaitFor(ctx, 40*time.Second, func(els []Element) bool {
		return FindText(els, sim.Name) != nil && FindButton(els, "Disconnect") != nil
	}), "status shows connected device")
}

func Test01_LaunchShowsWorkspace(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		must(t, s.WaitForText(ctx, "disconnected", 10*time.Second), "initial status")
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool { return FindButton(els, "probe.yaml") != nil }), "workspace file list")
		els, _ := s.Elements()
		for _, want := range []string{"Run", "Record", "Save", "Connect"} {
			if FindButton(els, want) == nil {
				t.Errorf("toolbar button %q missing", want)
			}
		}
	})
}

func Test07_RecordFlowWritesProbeScript(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		must(t, s.WaitFor(ctx, 20*time.Second, func(els []Element) bool {
			p := FindDevicePane(els)
			return p != nil && p.W >= 50
		}), "live device frame")
		must(t, s.ClickButton("Record"), "start recording")
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool { return FindButton(els, "Stop") != nil }), "record button turns into Stop")
		must(t, s.WaitForText(ctx, "RECORDED.PROBE", 10*time.Second), "editor switches to recorded.probe")
		// Three taps on the device itself (the Simulator window): Studio's
		// device pane does not forward clicks yet, and the agent's recorder
		// captures gestures made in the app. The fixture's Counter screen has
		// "Tap Me" just below the centre; each tap becomes a recorded line.
		for i := 0; i < 3; i++ {
			must(t, sim.Tap(0.5, 0.52), "tap on the simulator")
			time.Sleep(700 * time.Millisecond)
		}
		must(t, s.Focus(), "refocus studio")
		must(t, s.ClickButton("Stop"), "stop recording")
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool {
			return FindButton(els, "Record") != nil
		}), "record button restored")
		// The recorder opens an unsaved "recorded.probe"; read the editor
		// through the clipboard (select all, copy) rather than saving a file.
		must(t, s.FocusEditor(), "focus editor")
		text, err := s.EditorText()
		must(t, err, "read recorded script")
		// Stop replaces the editor with the agent's assembled script (steps
		// only; the `test` header typed at Start is dropped — known gap).
		if n := strings.Count(text, "tap #counter_button"); n != 3 {
			t.Errorf("recorded script should hold 3 taps on #counter_button, got %d:\n%s", n, text)
		}
	})
}
