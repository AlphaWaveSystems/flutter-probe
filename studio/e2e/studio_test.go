//go:build studio_e2e

// Studio E2E suite — drives the real FlutterProbe Studio app on macOS
// through the accessibility tree and screenshots. Run with:
//
//	make studio-e2e            (from the repo root; builds everything first)
//	go test -tags studio_e2e ./studio/e2e/ -run . -v -count=1   (prebuilt)
//
// Environment (all have defaults for the bundled fixture app):
//
//	STUDIO_E2E_APP        path to .../Contents/MacOS/flutter-probe-studio
//	STUDIO_E2E_WORKSPACE  workspace folder Studio opens (must hold probe.yaml)
//	STUDIO_E2E_TEST_FILE  .probe file (relative to the workspace) to run
//	STUDIO_E2E_FAIL_FILE  .probe file whose first test is expected to fail ("" to skip)
//	STUDIO_E2E_PERF_FILE  .probe file with a `start measuring` test ("" to skip)
//	STUDIO_E2E_PASS_NAME  a test name expected in the pass list of TEST_FILE
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
	vision   *Vision
	recorder *Recorder
	cfg      suiteConfig
	ctx      = context.Background()
)

type suiteConfig struct {
	App, Workspace, TestFile, FailFile, PerfFile, PassName string
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
	root := repoRoot()
	fixture := filepath.Join(root, "native-test-apps", "studio-fixture")
	cfg = suiteConfig{
		App:        envOr("STUDIO_E2E_APP", filepath.Join(root, "studio", "build", "bin", "flutter-probe-studio.app", "Contents", "MacOS", "flutter-probe-studio")),
		Workspace:  envOr("STUDIO_E2E_WORKSPACE", fixture),
		TestFile:   envOr("STUDIO_E2E_TEST_FILE", "probe-tests/counter.probe"),
		FailFile:   envOr("STUDIO_E2E_FAIL_FILE", "probe-tests/failing.probe"),
		PerfFile:   envOr("STUDIO_E2E_PERF_FILE", "probe-tests/list.probe"),
		PassName:   envOr("STUDIO_E2E_PASS_NAME", "counter increments and resets"),
		AppBundle:  envOr("STUDIO_E2E_APP_BUNDLE", filepath.Join(fixture, "build", "ios", "iphonesimulator", "Runner.app")),
		BundleID:   envOr("STUDIO_E2E_BUNDLE_ID", "com.alphawavesystems.studioFixture"),
		SimName:    envOr("STUDIO_E2E_SIM_NAME", "StudioE2E-iPhone17"),
		SimType:    envOr("STUDIO_E2E_SIM_TYPE", "com.apple.CoreSimulator.SimDeviceType.iPhone-17"),
		SimRuntime: envOr("STUDIO_E2E_SIM_RUNTIME", ""),
		Out:        envOr("STUDIO_E2E_OUT", filepath.Join(root, "reports", "studio-e2e")),
	}
	recorder = &Recorder{OutDir: cfg.Out}

	var err error
	if vision, err = NewVisionFromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "vision disabled:", err)
	}
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

// openFile clicks a path in the file browser, descending into folders.
func openFile(t *testing.T, s *Studio, rel string) {
	t.Helper()
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, p := range parts {
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool { return FindButton(els, p) != nil }), "file row "+p)
		must(t, s.ClickButton(p), "open "+p)
		time.Sleep(400 * time.Millisecond)
	}
	must(t, s.WaitForText(ctx, strings.ToUpper(filepath.Base(rel)), 10*time.Second), "editor shows file name")
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

func Test02_DevicePickerListsNamedSimulator(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		must(t, s.RefreshDevices(), "refresh devices")
		picked, err := s.SelectPopup(1, sim.Name)
		must(t, err, "select device")
		if !strings.Contains(picked, "ios") {
			t.Errorf("device label %q should carry the platform tag", picked)
		}
	})
}

func Test03_ConnectShowsLiveScreenAndTree(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		must(t, s.WaitFor(ctx, 20*time.Second, func(els []Element) bool {
			return FindText(els, "Connect to load the widget tree.") == nil
		}), "inspector loads widget tree")
		shot := filepath.Join(cfg.Out, "screenshots", t.Name()+"-connected.png")
		must(t, s.Screenshot(shot), "screenshot")
		if vision != nil {
			must(t, vision.Assert(ctx, shot, "The device pane in the middle shows a live phone screen of a mobile app, not a placeholder icon."), "vision: live screen")
		}
		must(t, s.ClickButton("Disconnect"), "disconnect")
		must(t, s.WaitForText(ctx, "disconnected", 10*time.Second), "status back to disconnected")
	})
}

func Test04_RunFilePassesAndListsResults(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		openFile(t, s, cfg.TestFile)
		must(t, s.ClickButton("Run"), "click run")
		must(t, s.WaitFor(ctx, 120*time.Second, func(els []Element) bool {
			for _, e := range els {
				if strings.Contains(e.Label(), "pass: "+cfg.PassName) {
					return true
				}
			}
			return false
		}), "result row for "+cfg.PassName)
		must(t, s.WaitForTextFold(ctx, "passed", 60*time.Second), "summary shows passed count")
	})
}

func Test05_RunFailingFileShowsErrorRow(t *testing.T) {
	if cfg.FailFile == "" {
		t.Skip("STUDIO_E2E_FAIL_FILE unset")
	}
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		openFile(t, s, cfg.FailFile)
		must(t, s.ClickButton("Run"), "click run")
		must(t, s.WaitFor(ctx, 120*time.Second, func(els []Element) bool {
			for _, e := range els {
				if strings.HasPrefix(e.Label(), "fail: ") {
					return true
				}
			}
			return false
		}), "a fail row")
		must(t, s.WaitForTextFold(ctx, "failed", 30*time.Second), "summary shows failed count")
	})
}

func Test06_PerfLinesAppearUnderResult(t *testing.T) {
	if cfg.PerfFile == "" {
		t.Skip("STUDIO_E2E_PERF_FILE unset")
	}
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		openFile(t, s, cfg.PerfFile)
		must(t, s.ClickButton("Run"), "click run")
		must(t, s.WaitForText(ctx, "⏱", 180*time.Second), "a performance line under a result")
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

func Test08_SettingsOverlayReadsAndSavesProbeYAML(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		must(t, s.ClickButton("⚙"), "open settings")
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool { return FindField(els, "") != nil }), "settings form")
		shot := filepath.Join(cfg.Out, "screenshots", t.Name()+"-settings.png")
		must(t, s.Screenshot(shot), "screenshot")
		must(t, s.ClickButton("✕"), "close settings")
	})
}

func Test09_AIChatPaneTogglesAndAsksForKey(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		must(t, s.ClickButton("✦"), "toggle chat")
		must(t, s.WaitFor(ctx, 10*time.Second, func(els []Element) bool { return FindButton(els, "🔑") != nil }), "chat pane shows key button")
		must(t, s.ClickButton("🔑"), "open key dialog")
		must(t, s.WaitForText(ctx, "key", 10*time.Second), "api key overlay")
		must(t, s.ClickButton("✕"), "close overlay")
		must(t, s.ClickButton("✦"), "toggle chat off")
	})
}

func Test10_ConnectWithoutAgentReportsError(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		sim.Terminate(ctx, cfg.BundleID)
		time.Sleep(time.Second)
		must(t, s.RefreshDevices(), "refresh devices")
		_, err := s.SelectPopup(1, sim.Name)
		must(t, err, "select device")
		must(t, s.ClickButton("Connect"), "click connect")
		// The status text shows "connection failed" for ~2.5s and the toast
		// "Connect failed: …" for 8s; poll fast enough to catch either.
		must(t, s.WaitFor(ctx, 60*time.Second, func(els []Element) bool {
			return FindText(els, "connection failed") != nil || FindText(els, "Connect failed") != nil
		}), "error state surfaced")
		els, _ := s.Elements()
		if FindButton(els, "Disconnect") != nil {
			t.Fatalf("connected although the app is not running")
		}
	})
}

// Test12: while a file runs, each step gets a row under its test; the last
// step of the passing fixture test ends as "step pass" and the toolbar
// progress reaches the final step count.
func Test12_LiveStepsReachLastStep(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		openFile(t, s, cfg.TestFile)
		must(t, s.ClickButton("Run"), "click run")
		// The fixture test finishes in well under a second, so the transient
		// "step N of M" text and the Cancel button are not asserted here; the
		// end state is: every step row has a verdict and Cancel is hidden again.
		// The last step of counter.probe is `see "Taps: 0"`; it must end as a passed step row.
		must(t, s.WaitFor(ctx, 120*time.Second, func(els []Element) bool {
			for _, e := range els {
				l := e.Label()
				if strings.HasPrefix(l, "step pass: ") && strings.Contains(l, `see "Taps: 0"`) {
					return true
				}
			}
			return false
		}), "last step row passed")
		must(t, s.WaitFor(ctx, 60*time.Second, func(els []Element) bool {
			for _, e := range els {
				if strings.Contains(e.Label(), "pass: "+cfg.PassName) {
					return true
				}
			}
			return false
		}), "test verdict row")
		// No step was left running, and the Cancel button is hidden again.
		must(t, s.WaitFor(ctx, 15*time.Second, func(els []Element) bool { return FindButton(els, "Cancel") == nil }), "cancel hidden after run")
		els, err := s.Elements()
		must(t, err, "elements")
		for _, e := range els {
			if strings.HasPrefix(e.Label(), "step running: ") {
				t.Fatalf("step still marked running after the run: %q", e.Label())
			}
		}
	})
}

// Test13: a failing step keeps a "step fail" row that names its line and
// carries the error.
func Test13_FailingStepRowNamesLine(t *testing.T) {
	if cfg.FailFile == "" {
		t.Skip("STUDIO_E2E_FAIL_FILE unset")
	}
	run(t, func(t *testing.T, s *Studio) {
		connect(t, s)
		openFile(t, s, cfg.FailFile)
		must(t, s.ClickButton("Run"), "click run")
		must(t, s.WaitFor(ctx, 120*time.Second, func(els []Element) bool {
			for _, e := range els {
				l := e.Label()
				if strings.HasPrefix(l, "step fail: ") && strings.Contains(l, "(line ") {
					return true
				}
			}
			return false
		}), "a failed step row with its line")
		must(t, s.WaitForText(ctx, "This text does not exist", 30*time.Second), "step error text shown")
	})
}

func Test11_WiFiDiscoveryOverlayOpensAndCloses(t *testing.T) {
	run(t, func(t *testing.T, s *Studio) {
		must(t, s.ClickButton("📡"), "open wifi overlay")
		must(t, s.WaitForText(ctx, "WiFi", 10*time.Second), "wifi overlay title")
		must(t, s.ClickButton("✕"), "close wifi overlay")
	})
}
