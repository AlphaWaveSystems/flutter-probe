package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// "Clear data before each test" asks once for the run: a yes holds for every test, a no is the error of every test, and nobody to ask names --yes.
func TestClearBeforeTestAsksOncePerRun(t *testing.T) {
	asks, msgs := 0, []string{}
	dc := &DeviceContext{AppID: "com.example.app", Serial: "emulator-5554", Confirm: func(m string) bool { asks++; msgs = append(msgs, m); return true }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	exec := &Executor{} // no device context: the wipe itself is the cloud-mode no-op, so the question is what is under test
	for i := 0; i < 3; i++ {
		if err := r.clearBeforeTest(context.Background(), exec); err != nil {
			t.Fatalf("test %d: %v", i, err)
		}
	}
	if asks != 1 {
		t.Errorf("asked %d times for 3 tests, want once per run", asks)
	}
	for _, want := range []string{"com.example.app", "emulator-5554", "BEFORE EVERY TEST"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("the question lacks %q: %q", want, msgs[0])
		}
	}
}

func TestClearBeforeTestNoStopsEveryTest(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	for i := 0; i < 3; i++ {
		err := r.clearBeforeTest(context.Background(), &Executor{})
		if err == nil || !strings.Contains(err.Error(), "clear data before each test: cancelled by user") {
			t.Fatalf("test %d: %v", i, err)
		}
	}
	if asks != 1 {
		t.Errorf("asked %d times, want once", asks)
	}
}

func TestClearBeforeTestWithNobodyToAskNamesYes(t *testing.T) {
	r := &Runner{cfg: &config.Config{}, deviceCtx: &DeviceContext{AppID: "a.b", Serial: "s"}}
	err := r.clearBeforeTest(context.Background(), &Executor{})
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "clear data before each test") {
		t.Errorf("err = %v", err)
	}
}

func TestClearBeforeTestWithYesNeverAsks(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true, Confirm: func(string) bool { asks++; return false }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	if err := r.clearBeforeTest(context.Background(), &Executor{}); err != nil || asks != 0 {
		t.Errorf("err = %v, asked %d", err, asks)
	}
}

// The answer to the per-run question does not become permission for a `clear app data` step inside a test: that step still asks as it always has.
func TestTheRunQuestionDoesNotAnswerAClearAppDataStep(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return true }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	if err := r.clearBeforeTest(context.Background(), &Executor{}); err != nil {
		t.Fatal(err)
	}
	if dc.AllowClearData {
		t.Fatal("the run question turned on AllowClearData")
	}
	if err := dc.AskClearData(false); err != nil || asks != 2 {
		t.Errorf("a clear app data step after it: err %v, asked %d (want a second question)", err, asks)
	}
}

// Through the real attempt: when the run's question is answered no, the test body does not run at all (nothing was tapped), the result names the
// setting, and with the setting off the same test runs and no question is asked.
func TestAnAttemptDoesNotRunItsBodyWhenTheClearWasRefused(t *testing.T) {
	asks := 0
	client := newSlowClient()
	r := newTimeoutRunner(client, 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	prog, tdef := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "clear data before each test: cancelled by user") {
		t.Fatalf("result = %+v", res)
	}
	client.mu.Lock()
	tapped := len(client.taps)
	client.mu.Unlock()
	if tapped != 0 {
		t.Error("the test body ran although the clear was refused")
	}
	// the same run: a second test is refused with the same answer, without a second question
	res2 := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res2.Passed || asks != 1 {
		t.Errorf("second test passed=%v, asks=%d (want a no and one question)", res2.Passed, asks)
	}

	off := newTimeoutRunner(newSlowClient(), 0)
	off.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { t.Error("asked with the setting off"); return false }}
	if r := off.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1); !r.Passed {
		t.Errorf("with the setting off the test did not run: %+v", r)
	}
}

// A before-each hook is part of the test's start: after a refused clear it does not run either.
func TestABeforeEachHookDoesNotRunAfterARefusedClear(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { return false }}
	prog, tdef := parseOneTest(t, "before each\n  tap \"Hook\"\n\ntest \"t\"\n  tap \"Body\"\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	client.mu.Lock()
	taps := append([]string(nil), client.taps...)
	client.mu.Unlock()
	if res.Passed || len(taps) != 0 {
		t.Errorf("passed=%v, taps=%v: neither the hook nor the body may run after a refused clear", res.Passed, taps)
	}
}

// A parallel lane takes its consent from --yes alone (the shared gate): a lane with no --yes (no AllowClearData) and nobody to ask refuses
// "clear data before each test" and nothing runs, not a hook, not the body; a lane with --yes answers it, with one line in the log naming the
// package, the device and "BEFORE EVERY TEST".
func TestALaneAnswersClearBeforeEachTestOnlyWithYes(t *testing.T) {
	noYes := &DeviceContext{AppID: "com.example.app", Serial: "R58M123ABC", NoPrompt: true} // what a lane is without --yes
	r := &Runner{cfg: &config.Config{}, deviceCtx: noYes}
	err := r.clearBeforeTest(context.Background(), &Executor{})
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "clear data before each test") {
		t.Fatalf("a lane without --yes was allowed to clear before each test: %v", err)
	}
	client := newSlowClient()
	run := newTimeoutRunner(client, 0)
	run.cfg.Defaults.ClearDataBeforeEachTest = true
	run.deviceCtx = noYes
	prog, tdef := parseOneTest(t, "before each\n  tap \"Hook\"\n\ntest \"t\"\n  tap \"Body\"\n")
	res := run.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	client.mu.Lock()
	taps := append([]string(nil), client.taps...)
	client.mu.Unlock()
	if res.Passed || len(taps) != 0 {
		t.Errorf("passed=%v taps=%v: a lane without --yes wiped or ran after a refusal", res.Passed, taps)
	}
	// with --yes the lane is answered, and the log says once what it is for
	withYes := &DeviceContext{AppID: "com.example.app", Serial: "R58M123ABC", NoPrompt: true, AllowClearData: true}
	rr := &Runner{cfg: &config.Config{}, deviceCtx: withYes}
	out := captureStdout(t, func() {
		for i := 0; i < 3; i++ {
			if err := rr.clearBeforeTest(context.Background(), &Executor{}); err != nil {
				t.Errorf("a lane with --yes was refused: %v", err)
			}
		}
	})
	if !strings.Contains(out, "BEFORE EVERY TEST on this device") || strings.Contains(out, "of this run") { // it is a line per device (a lane prints its own), not per run
		t.Errorf("the line should say the wipe is per device, not per run: %q", out)
	}
	if strings.Count(out, "BEFORE EVERY TEST") != 1 || !strings.Contains(out, "com.example.app") || !strings.Contains(out, "R58M123ABC") {
		t.Errorf("the log line should name the package and device once for 3 tests: %q", out)
	}
}

// A `clear app data` step in a test asks as it always has, through the real executor and device context (the run question's answer, tested above at
// the device context, is not permission for it).
func TestAClearAppDataStepStillAsksThroughTheExecutor(t *testing.T) {
	asks := 0
	r := newTimeoutRunner(newSlowClient(), 0)
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	prog, tdef := parseOneTest(t, "test \"t\"\n  clear app data\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "clear app data: cancelled by user") || asks != 1 {
		t.Errorf("the step did not ask: passed=%v err=%v asks=%d", res.Passed, res.Error, asks)
	}
	// --yes (AllowClearData) answers the step's own question as it always did
	lane := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true, NoPrompt: true}
	if err := lane.AskClearData(false); err != nil {
		t.Errorf("a clear app data step on a lane with --yes: %v", err)
	}
}

// The question text is one line each, whatever the id or serial hold.
func TestTheRunQuestionIsCleanedToOneLine(t *testing.T) {
	var msg string
	dc := &DeviceContext{AppID: "a.b\nFAKE\x1b[2J", Serial: "s\u202e\nX", Confirm: func(m string) bool { msg = m; return false }}
	_ = dc.AskClearData(true)
	if strings.Contains(msg, "\x1b") || strings.Contains(msg, "\u202e") || strings.Contains(msg, "a.b\nFAKE") {
		t.Errorf("the question carries a control character or a split: %q", msg)
	}
}

// With the setting on and no local device (cloud, dry run) the run says once that it does nothing, and runs the test.
func TestTheSettingWithNoDeviceSaysSoOnce(t *testing.T) {
	r := newTimeoutRunner(newSlowClient(), 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = nil
	prog, tdef := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n")
	out := captureStdout(t, func() {
		for i := 0; i < 3; i++ {
			if res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1); !res.Passed {
				t.Errorf("the test did not run: %+v", res)
			}
		}
	})
	if strings.Count(out, "clear-before-each-test has no effect here") != 1 {
		t.Errorf("the notice appeared %d times: %q", strings.Count(out, "clear-before-each-test has no effect here"), out)
	}
}

// The per-test question is built by the shared gate: on an iOS simulator it carries the keychain sentence of the clear question, on Android it does not,
// and in both it names the package and device and says it happens before every test.
func TestThePerTestQuestionCarriesTheSharedIOSKeychainSentence(t *testing.T) {
	for _, tc := range []struct {
		platform device.Platform
		keychain bool
	}{{device.PlatformIOS, true}, {device.PlatformAndroid, false}} {
		var msg string
		dc := &DeviceContext{AppID: "a.b", Serial: "s", Platform: tc.platform, Confirm: func(m string) bool { msg = m; return false }}
		_ = dc.AskClearData(true)
		if strings.Contains(msg, iosKeychainSentence) != tc.keychain || !strings.Contains(msg, "BEFORE EVERY TEST") || !strings.Contains(msg, "a.b") {
			t.Errorf("%s: question = %q", tc.platform, msg)
		}
		// the refusal hint says it too on iOS
		hint := (&DeviceContext{AppID: "a.b", Serial: "s", Platform: tc.platform}).AskClearData(true)
		if hint == nil || strings.Contains(hint.Error(), "keychain") != tc.keychain {
			t.Errorf("%s: hint = %v", tc.platform, hint)
		}
	}
}

// The refusal hint cleans the app id like the question does: no control or direction character and no unbounded length reaches the line.
func TestTheRefusalHintCleansTheAppID(t *testing.T) {
	dc := &DeviceContext{AppID: "a.b\nFAKE\x1b[2J\u202e" + strings.Repeat("x", 500), Serial: "s"}
	for _, perTest := range []bool{true, false} {
		err := dc.AskClearData(perTest)
		if err == nil {
			t.Fatal("no confirmation and no --yes was allowed")
		}
		msg := err.Error()
		if strings.ContainsAny(msg, "\n\x1b\u202e") || len([]rune(msg)) > 700 {
			t.Errorf("perTest=%v: the hint carries the raw app id: %q", perTest, msg)
		}
	}
}

// The question of the before-each-test wipe says when it happens, and IsBeforeEachTestQuestion tells it from a `clear app data` step's.
func TestTheBeforeEachQuestionIsToldFromAStepQuestion(t *testing.T) {
	var perTest, step string
	(&DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(m string) bool { perTest = m; return false }}).AskClearData(true)
	(&DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(m string) bool { step = m; return false }}).AskClearData(false)
	if !IsBeforeEachTestQuestion(perTest) || IsBeforeEachTestQuestion(step) {
		t.Errorf("per-test %v, step %v: %q / %q", IsBeforeEachTestQuestion(perTest), IsBeforeEachTestQuestion(step), perTest, step)
	}
}

// Every message of the wipe gates and the wipe itself shows the app id and the device as one clean bounded line: the step's question, the
// reinstall's question and hint, and the line that says what is being cleared.
func TestEveryWipeMessageCleansTheAppIDAndTheDevice(t *testing.T) {
	bad := "a.b\nFAKE\x1b[2J‮" + strings.Repeat("x", 300)
	check := func(label, s string) {
		t.Helper()
		// the questions have one fixed line break of their own; nothing else may break a line, and nothing is cut by the raw ids
		clean := strings.ReplaceAll(strings.ReplaceAll(s, "\n  SharedPreferences", " SharedPreferences"), "\n  This wipes", " This wipes")
		if strings.ContainsAny(clean, "\n\x1b‮") || strings.Contains(s, strings.Repeat("x", 100)) {
			t.Errorf("%s carries the raw app id or serial: %q", label, s)
		}
	}
	for _, perTest := range []bool{false, true} {
		var q string
		(&DeviceContext{AppID: bad, Serial: bad, Platform: "android", Confirm: func(m string) bool { q = m; return false }}).AskClearData(perTest)
		check(fmt.Sprintf("question perTest=%v", perTest), q)
		check("hint", (&DeviceContext{AppID: bad, Serial: bad}).AskClearData(perTest).Error())
	}
	var rq string
	(&DeviceContext{AppID: bad, Serial: bad, Confirm: func(m string) bool { rq = m; return false }}).AskReinstallForNotifications()
	check("reinstall question", rq)
	check("reinstall hint", (&DeviceContext{AppID: bad, Serial: bad}).AskReinstallForNotifications().Error())
	out := captureStdout(t, func() {
		_ = (&DeviceContext{AppID: bad, Serial: bad, Platform: "other"}).clearAppData(context.Background())
	})
	check("the clearing line", strings.TrimSuffix(regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out, ""), "\n")) // the line's own colours and line end are not the ids'
}
