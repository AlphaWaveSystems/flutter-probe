package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const crashPkg = "dev.probe.webviewfixture"

// sample reads one capture of a real device (see testdata/crash/README.md).
func sample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "crash", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// crashAdb is an AdbRunner that records every call and answers the two commands from fixed text, or fails.
type crashAdb struct {
	calls   [][]string
	exit    string
	buf     string
	exitErr error
	bufErr  error
	block   bool // wait for the context instead of answering
}

func (c *crashAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	c.calls = append(c.calls, append([]string(nil), args...))
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if len(args) > 2 && args[2] == "logcat" {
		return []byte(c.buf), c.bufErr
	}
	return []byte(c.exit), c.exitErr
}

func maskToken(s string) string { return strings.ReplaceAll(s, "abc123FIXTUREvalue", "[masked]") }

func TestParseExitInfoRealJavaCrash(t *testing.T) {
	recs := ParseExitInfo(sample(t, "exit-java.txt"), crashPkg)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3: %+v", len(recs), recs)
	}
	r := recs[0]
	if r.PID != 3567 || r.Reason != 4 || r.Kind != ExitCrash || r.Status != 0 || r.Process != crashPkg || r.Description != "crash" {
		t.Errorf("newest record = %+v", r)
	}
	if got := r.WallClock.Format("2006-01-02 15:04:05.000"); got != "2026-11-15 19:31:09.870" {
		t.Errorf("time = %s", got)
	}
	if recs[1].PID != 2939 || recs[1].Kind != ExitNativeCrash || recs[1].Status != 5 {
		t.Errorf("second record = %+v", recs[1])
	}
}

func TestParseExitInfoRealNativeCrash(t *testing.T) {
	recs := ParseExitInfo(sample(t, "exit-segv.txt"), crashPkg)
	if len(recs) == 0 || recs[0].PID != 3644 || recs[0].Kind != ExitNativeCrash || recs[0].Status != 11 {
		t.Fatalf("records = %+v", recs)
	}
}

// Entries of other processes under the same uid (the WebView sandbox) and of another uid are not the app's.
func TestParseExitInfoKeepsOnlyTheAppsOwnProcesses(t *testing.T) {
	recs := ParseExitInfo(sample(t, "exit-webview-and-stop.txt"), crashPkg)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(recs), recs)
	}
	if recs[0].PID != 2939 || recs[1].PID != 2990 || recs[1].Kind != ExitUserRequested || !strings.HasPrefix(recs[1].Description, "stop ") {
		t.Errorf("records = %+v", recs)
	}
	for _, r := range recs {
		if r.Process != crashPkg {
			t.Errorf("process %q is not the app", r.Process)
		}
	}
	if got := ParseExitInfo(sample(t, "exit-webview-and-stop.txt"), "dev.probe.other"); len(got) != 0 {
		t.Errorf("another package got %d records", len(got))
	}
	// a service process of the app is the app's
	svc := strings.Replace(sample(t, "exit-java.txt"), "process="+crashPkg+" reason=4", "process="+crashPkg+":sync reason=4", 1)
	if got := ParseExitInfo(svc, crashPkg); len(got) != 3 || got[0].Process != crashPkg+":sync" {
		t.Errorf("service process: %+v", got)
	}
	// a package that only starts with the name is not the app
	if got := ParseExitInfo(strings.ReplaceAll(sample(t, "exit-java.txt"), "process="+crashPkg, "process="+crashPkg+"x"), crashPkg); len(got) != 0 {
		t.Errorf("a longer package name matched: %+v", got)
	}
}

func TestExitKindsComeFromTheNumberNotTheWords(t *testing.T) {
	for reason, want := range map[int]ExitKind{0: ExitUnknown, 1: ExitSelf, 2: ExitSignaled, 3: ExitLowMemory, 4: ExitCrash, 5: ExitNativeCrash, 6: ExitANR, 10: ExitUserRequested, 11: ExitUserRequested, 13: ExitOther, 99: ExitOther} {
		text := "  package: " + crashPkg + "\n        ApplicationExitInfo #0:\n          timestamp=2026-01-02 03:04:05.006 pid=7 realUid=1\n          process=" + crashPkg + " reason=" + strconv.Itoa(reason) + " (APP CRASH(EXCEPTION)) subreason=0 status=0\n"
		recs := ParseExitInfo(text, crashPkg)
		if len(recs) != 1 || recs[0].Kind != want || recs[0].Reason != reason {
			t.Errorf("reason %d: %+v, want kind %s", reason, recs, want)
		}
	}
}

func TestParseExitInfoHostileText(t *testing.T) {
	if got := ParseExitInfo(sample(t, "exit-java.txt"), "x;rm -rf /"); got != nil {
		t.Errorf("an invalid package name read records: %+v", got)
	}
	// a description with hidden characters is cleaned
	text := "        ApplicationExitInfo #0:\n          timestamp=2026-01-02 03:04:05.006 pid=7 realUid=1\n          process=" + crashPkg + " reason=4 (x) status=0\n          importance=1 description=a\u202eb\x1b[31mc\u200bd state=empty\n"
	recs := ParseExitInfo(text, crashPkg)
	if len(recs) != 1 || recs[0].Description != "ab[31mcd" {
		t.Errorf("description = %q", recs[0].Description)
	}
	// a flood of entries is cut at the limit; an enormous input is cut before it is read
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("        ApplicationExitInfo #0:\n          timestamp=2026-01-02 03:04:05.006 pid=7 realUid=1\n          process=" + crashPkg + " reason=4 (x) status=0\n")
	}
	if got := ParseExitInfo(b.String(), crashPkg); len(got) != maxExitRecords {
		t.Errorf("a flood kept %d records, want %d", len(got), maxExitRecords)
	}
	huge := strings.Repeat("x", maxCrashInputBytes+10) + "\n" + sample(t, "exit-java.txt")
	if got := ParseExitInfo(huge, crashPkg); len(got) != 0 {
		t.Errorf("text past the input limit was read: %d records", len(got))
	}
}

func TestParseCrashBufferRealJavaCrash(t *testing.T) {
	ct := ParseCrashBuffer(sample(t, "crashbuf-java.txt"), 3567, crashPkg, 0, 0)
	if !strings.HasPrefix(ct.Summary, "java.lang.RuntimeException: Unable to start activity") {
		t.Errorf("summary = %q", ct.Summary)
	}
	if len(ct.Lines) < 20 || ct.Truncated {
		t.Errorf("kept %d lines (truncated %v)", len(ct.Lines), ct.Truncated)
	}
	if ct.Lines[0] != "FATAL EXCEPTION: main" {
		t.Errorf("first line = %q", ct.Lines[0])
	}
	if got := ParseCrashBuffer(sample(t, "crashbuf-java.txt"), 1, crashPkg, 0, 0); len(got.Lines) != 0 || got.Summary != "" {
		t.Errorf("another pid kept %d lines", len(got.Lines))
	}
}

// A native crash is written by the system's tombstone writer, a different process: its lines are kept because they name the crashed process.
func TestParseCrashBufferRealNativeCrash(t *testing.T) {
	ct := ParseCrashBuffer(sample(t, "crashbuf-segv.txt"), 3644, crashPkg, 0, 0)
	if ct.Summary != "Fatal signal 11 (SIGSEGV)" {
		t.Errorf("summary = %q", ct.Summary)
	}
	var tomb, libc int
	for _, l := range ct.Lines {
		if strings.HasPrefix(l, "pid: 3644,") {
			tomb++
		}
		if strings.HasPrefix(l, "Fatal signal 11") {
			libc++
		}
	}
	if tomb != 1 || libc != 1 {
		t.Errorf("tombstone lines %d, libc lines %d", tomb, libc)
	}
	// the tombstone of another process is not kept
	if got := ParseCrashBuffer(sample(t, "crashbuf-segv.txt"), 12345, "dev.probe.other", 0, 0); len(got.Lines) != 0 {
		t.Errorf("another process kept %d lines", len(got.Lines))
	}
	// the package name in the tombstone is enough when the pid is not known to match
	if got := ParseCrashBuffer(sample(t, "crashbuf-segv.txt"), 99999, crashPkg, 0, 0); len(got.Lines) < 50 {
		t.Errorf("the package marker kept %d lines", len(got.Lines))
	}
}

func TestParseCrashBufferDropsCrashpadNoiseAndOtherProcesses(t *testing.T) {
	text := sample(t, "crashbuf-webview-crashpad-noise.txt")
	if !strings.Contains(text, "crashpad") {
		t.Fatal("the sample lost its crashpad lines")
	}
	// the exception belongs to the sandbox process 2965; the app's own pid has nothing here
	if got := ParseCrashBuffer(text, 2939, crashPkg, 0, 0); len(got.Lines) != 0 {
		t.Errorf("the app's pid kept %d lines", len(got.Lines))
	}
	got := ParseCrashBuffer(text, 2965, crashPkg, 0, 0)
	if len(got.Lines) == 0 {
		t.Fatal("the sandbox process's lines were dropped")
	}
	for _, l := range got.Lines {
		if strings.Contains(l, "MINIDUMP") || strings.Contains(l, "crashpad") {
			t.Errorf("crashpad text kept: %q", l)
		}
	}
	// a crashpad line of the wanted pid is still noise
	both := "11-15 19:03:48.050  4000  4000 F crashpad: -----BEGIN CRASHPAD MINIDUMP-----\n11-15 19:03:48.051  4000  4000 E AndroidRuntime: java.lang.Error: boom\n"
	if got := ParseCrashBuffer(both, 4000, crashPkg, 0, 0); len(got.Lines) != 1 || got.Summary != "java.lang.Error: boom" {
		t.Errorf("crashpad of the wanted pid: %+v", got)
	}
}

func TestParseCrashBufferLimits(t *testing.T) {
	text := sample(t, "crashbuf-java.txt")
	ct := ParseCrashBuffer(text, 3567, crashPkg, 5, 0)
	if len(ct.Lines) != 5 || !ct.Truncated {
		t.Errorf("max lines: kept %d, truncated %v", len(ct.Lines), ct.Truncated)
	}
	if ct.Summary == "" {
		t.Error("the summary was lost with the lines past the limit")
	}
	ct = ParseCrashBuffer(text, 3567, crashPkg, 0, 300)
	size := 0
	for _, l := range ct.Lines {
		size += len(l) + 1
	}
	if size > 300 || !ct.Truncated || len(ct.Lines) == 0 {
		t.Errorf("max bytes: kept %d bytes in %d lines, truncated %v", size, len(ct.Lines), ct.Truncated)
	}
	// one enormous line counts at its cut length, so it neither exhausts the budget nor is read without a limit
	big := "11-15 19:03:48.050  4000  4000 E AndroidRuntime: java.lang.Error: " + strings.Repeat("x", 600000) + "\n"
	ct = ParseCrashBuffer(big+big, 4000, crashPkg, 0, 0)
	if len(ct.Lines) == 0 || len(ct.Lines) > 2 {
		t.Errorf("enormous lines: kept %d", len(ct.Lines))
	}
}

func TestParseCrashBufferCleansHostileText(t *testing.T) {
	line := func(msg string) string { return "11-15 19:03:48.050  4000  4000 E AndroidRuntime: " + msg + "\n" }
	text := line("java.lang.Error: a\u202eb\u200bc\x1b[2Jd\u2028e\x00f\xffg") + line("\tat x.Y(Z.java:1)")
	ct := ParseCrashBuffer(text, 4000, crashPkg, 0, 0)
	if len(ct.Lines) != 2 || ct.Lines[0] != "java.lang.Error: abc[2Jdefg" || ct.Lines[1] != " at x.Y(Z.java:1)" {
		t.Errorf("lines = %q", ct.Lines)
	}
	if ct.Summary != "java.lang.Error: abc[2Jdefg" {
		t.Errorf("summary = %q", ct.Summary)
	}
	for _, l := range ct.Lines {
		if !utf8.ValidString(l) {
			t.Errorf("invalid UTF-8 kept: %q", l)
		}
	}
	// a line of another process that claims to be a tombstone of the wanted pid is not one: only the DEBUG tag counts
	fake := "11-15 19:03:48.050  5000  5000 E SomeApp: pid: 4000, tid: 4000 >>> x <<<\n"
	if got := ParseCrashBuffer(fake, 4000, crashPkg, 0, 0); len(got.Lines) != 0 {
		t.Errorf("a fake tombstone line of another tag was kept: %q", got.Lines)
	}
}

func TestCollectCrashEvidenceRunsTwoReadOnlyCommandsAndMasks(t *testing.T) {
	adb := &crashAdb{exit: sample(t, "exit-java.txt"), buf: sample(t, "crashbuf-java.txt")}
	ev, err := CollectCrashEvidence(context.Background(), adb, "emulator-5600", crashPkg, CrashOptions{Redact: maskToken, PID: 3567})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"-s", "emulator-5600", "shell", "dumpsys", "activity", "exit-info", crashPkg},
		{"-s", "emulator-5600", "logcat", "-b", "crash", "-d", "-v", "threadtime"},
	}
	if len(adb.calls) != 2 {
		t.Fatalf("calls = %q", adb.calls)
	}
	for i, c := range adb.calls {
		if strings.Join(c, "\x00") != strings.Join(want[i], "\x00") {
			t.Errorf("call %d = %q, want %q", i, c, want[i])
		}
		for _, w := range c {
			if w == "-c" || w == "forward" || w == "--remove-all" || w == "clear" || w == "kill" || w == "force-stop" {
				t.Errorf("call %d is not read-only: %q", i, c)
			}
		}
	}
	if ev.Exit == nil || ev.Exit.PID != 3567 || ev.Exit.Kind != ExitCrash || len(ev.Exits) != 3 {
		t.Errorf("exit = %+v (%d exits)", ev.Exit, len(ev.Exits))
	}
	all := ev.Summary + "\n" + strings.Join(ev.CrashLines, "\n")
	if !strings.Contains(sample(t, "crashbuf-java.txt"), "abc123FIXTUREvalue") {
		t.Fatal("the sample no longer holds the token the test masks")
	}
	if strings.Contains(all, "abc123FIXTUREvalue") || !strings.Contains(all, "[masked]") {
		t.Errorf("the token was not masked in the evidence:\n%s", all)
	}
	if !strings.HasPrefix(ev.Summary, "java.lang.RuntimeException: Unable to start activity") {
		t.Errorf("summary = %q", ev.Summary)
	}
}

func TestCollectCrashEvidenceMasksTheExitDescription(t *testing.T) {
	exit := strings.Replace(sample(t, "exit-java.txt"), "description=crash state", "description=died with abc123FIXTUREvalue inside state", 1)
	ev, err := CollectCrashEvidence(context.Background(), &crashAdb{exit: exit}, "emulator-5600", crashPkg, CrashOptions{Redact: maskToken})
	if err != nil || ev.Exit == nil {
		t.Fatalf("%v %+v", err, ev)
	}
	if strings.Contains(ev.Exit.Description, "abc123") || ev.Exit.Description != "died with [masked] inside" {
		t.Errorf("description = %q", ev.Exit.Description)
	}
	for _, r := range ev.Exits {
		if strings.Contains(r.Description, "abc123") {
			t.Errorf("an exit kept the token: %+v", r)
		}
	}
}

// A secret that straddles the cut must not leave its readable start: the text is masked, then cut.
func TestCollectCrashEvidenceMasksBeforeItCuts(t *testing.T) {
	secret := "abc123FIXTUREvalue"
	line := "java.lang.Error: " + strings.Repeat("x", maxCrashLineRunes-17-8) + secret + " tail"
	buf := "11-15 19:03:48.050  4000  4000 E AndroidRuntime: " + line + "\n"
	exit := "        ApplicationExitInfo #0:\n          timestamp=2026-01-02 03:04:05.006 pid=4000 realUid=1\n          process=" + crashPkg + " reason=4 (x) status=0\n"
	ev, err := CollectCrashEvidence(context.Background(), &crashAdb{exit: exit, buf: buf}, "s", crashPkg, CrashOptions{Redact: maskToken})
	if err != nil || len(ev.CrashLines) != 1 {
		t.Fatalf("%v %+v", err, ev)
	}
	if strings.Contains(ev.CrashLines[0], "abc12") || strings.Contains(ev.Summary, "abc12") {
		t.Errorf("a part of the secret survived the cut: %q", ev.CrashLines[0][len(ev.CrashLines[0])-40:])
	}
	if utf8.RuneCountInString(ev.CrashLines[0]) > maxCrashLineRunes || utf8.RuneCountInString(ev.Summary) > maxCrashSummary {
		t.Errorf("a line or the summary is past its limit: %d, %d", utf8.RuneCountInString(ev.CrashLines[0]), utf8.RuneCountInString(ev.Summary))
	}
}

// The summary is masked and then cut, like the lines: a secret inside it, or straddling its limit, leaves nothing readable.
func TestCollectCrashEvidenceMasksTheSummaryBeforeItCuts(t *testing.T) {
	exit := "        ApplicationExitInfo #0:\n          timestamp=2026-01-02 03:04:05.006 pid=4000 realUid=1\n          process=" + crashPkg + " reason=4 (x) status=0\n"
	for name, msg := range map[string]string{
		"inside":    "java.lang.IllegalStateException: boom token=abc123FIXTUREvalue",
		"straddles": "java.lang.Error: " + strings.Repeat("x", maxCrashSummary-17-8) + "abc123FIXTUREvalue tail",
	} {
		buf := "11-15 19:03:48.050  4000  4000 E AndroidRuntime: " + msg + "\n"
		ev, err := CollectCrashEvidence(context.Background(), &crashAdb{exit: exit, buf: buf}, "s", crashPkg, CrashOptions{Redact: maskToken})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(ev.Summary, "abc12") || ev.Summary == "" || utf8.RuneCountInString(ev.Summary) > maxCrashSummary {
			t.Errorf("%s: summary = %q", name, ev.Summary)
		}
	}
}

// Text past the input limit is never read, whatever it says.
func TestParseCrashBufferIgnoresTextPastTheInputLimit(t *testing.T) {
	filler := strings.Repeat("junk line that is not a log line\n", maxCrashInputBytes/30)
	text := filler + "11-15 19:03:48.050  4000  4000 E AndroidRuntime: java.lang.Error: late\n"
	if got := ParseCrashBuffer(text, 4000, crashPkg, 0, 0); len(got.Lines) != 0 {
		t.Errorf("a line past the input limit was read: %q", got.Lines)
	}
	if got := ParseCrashBuffer("11-15 19:03:48.050  4000  4000 E AndroidRuntime: java.lang.Error: early\n"+filler, 4000, crashPkg, 0, 0); len(got.Lines) != 1 {
		t.Errorf("a line inside the limit was dropped: %q", got.Lines)
	}
}

func TestCollectCrashEvidenceRefusesBadInputWithoutCallingAdb(t *testing.T) {
	for name, c := range map[string]struct {
		runner AdbRunner
		serial string
		pkg    string
		opts   CrashOptions
	}{
		"no redaction":  {&crashAdb{}, "s", crashPkg, CrashOptions{}},
		"no serial":     {&crashAdb{}, " ", crashPkg, CrashOptions{Redact: maskToken}},
		"no runner":     {nil, "s", crashPkg, CrashOptions{Redact: maskToken}},
		"shell package": {&crashAdb{}, "s", "a.b;rm -rf /", CrashOptions{Redact: maskToken}},
		"one word":      {&crashAdb{}, "s", "nodots", CrashOptions{Redact: maskToken}},
		"quoted":        {&crashAdb{}, "s", "a.b'c", CrashOptions{Redact: maskToken}},
		"empty package": {&crashAdb{}, "s", "", CrashOptions{Redact: maskToken}},
	} {
		_, err := CollectCrashEvidence(context.Background(), c.runner, c.serial, c.pkg, c.opts)
		var ce *CrashCollectError
		if !errors.As(err, &ce) || ce.Step != "options" {
			t.Errorf("%s: err = %v", name, err)
		}
		if a, ok := c.runner.(*crashAdb); ok && len(a.calls) != 0 {
			t.Errorf("%s: adb was called: %q", name, a.calls)
		}
	}
}

// Errors have a fixed text: nothing the device or adb printed can reach a message the reconnect code reads.
func TestCollectCrashEvidenceErrorsNeverCarryDeviceText(t *testing.T) {
	leak := errors.New("adb: device offline: abc123FIXTUREvalue \x1b[31m reconnect now")
	for name, adb := range map[string]*crashAdb{
		"exit-info":    {exitErr: leak},
		"crash buffer": {exit: sample(t, "exit-java.txt"), bufErr: leak},
	} {
		_, err := CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken})
		var ce *CrashCollectError
		if !errors.As(err, &ce) || ce.Step != name {
			t.Fatalf("%s: err = %v", name, err)
		}
		if strings.Contains(err.Error(), "abc123") || strings.Contains(err.Error(), "offline") || strings.Contains(err.Error(), "reconnect") || errors.Is(err, leak) {
			t.Errorf("%s: the error carries adb text: %q", name, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			t.Errorf("%s: an adb failure looks like a timeout", name)
		}
	}
}

func TestCollectCrashEvidenceStopsAtTheTimeoutAndOnCancel(t *testing.T) {
	start := time.Now()
	_, err := CollectCrashEvidence(context.Background(), &crashAdb{block: true}, "s", crashPkg, CrashOptions{Redact: maskToken, Timeout: 30 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Errorf("timeout: err = %v after %v", err, time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CollectCrashEvidence(ctx, &crashAdb{block: true}, "s", crashPkg, CrashOptions{Redact: maskToken}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: err = %v", err)
	}
}

func TestCollectCrashEvidenceWithNothingToReadAsksForNoCrashBuffer(t *testing.T) {
	adb := &crashAdb{exit: "ACTIVITY MANAGER PROCESS EXIT INFO\n"}
	ev, err := CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken})
	if err != nil || ev.Exit != nil || len(adb.calls) != 1 {
		t.Errorf("err %v, exit %v, %d calls", err, ev.Exit, len(adb.calls))
	}
	// a pid that is given is read even when the exit list has no entry for it
	adb = &crashAdb{exit: sample(t, "exit-java.txt"), buf: sample(t, "crashbuf-segv.txt")}
	ev, err = CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken, PID: 3644})
	if err != nil || ev.Exit != nil || len(adb.calls) != 2 || ev.Summary != "Fatal signal 11 (SIGSEGV)" {
		t.Errorf("err %v, exit %v, %d calls, summary %q", err, ev.Exit, len(adb.calls), ev.Summary)
	}
	// without a pid the newest entry decides which process's crash text is read
	adb = &crashAdb{exit: sample(t, "exit-segv.txt"), buf: sample(t, "crashbuf-segv.txt")}
	ev, _ = CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken})
	if ev.Exit == nil || ev.Exit.PID != 3644 || ev.Summary != "Fatal signal 11 (SIGSEGV)" || len(ev.CrashLines) < 50 {
		t.Errorf("newest entry: %+v (%d lines)", ev.Exit, len(ev.CrashLines))
	}
}

func TestCollectCrashEvidenceLimitsTheKeptText(t *testing.T) {
	adb := &crashAdb{exit: sample(t, "exit-segv.txt"), buf: sample(t, "crashbuf-segv.txt")}
	ev, _ := CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken, MaxLines: 10})
	if len(ev.CrashLines) != 10 || !ev.Truncated {
		t.Errorf("kept %d lines, truncated %v", len(ev.CrashLines), ev.Truncated)
	}
	var many strings.Builder
	for i := 0; i < 300; i++ {
		many.WriteString("11-15 19:03:48.050  3644  3644 E AndroidRuntime: line\n")
	}
	adb.buf = many.String()
	ev, _ = CollectCrashEvidence(context.Background(), adb, "s", crashPkg, CrashOptions{Redact: maskToken})
	if len(ev.CrashLines) != defaultCrashLines || !ev.Truncated {
		t.Errorf("default: kept %d lines, truncated %v", len(ev.CrashLines), ev.Truncated)
	}
}

func TestCleanDeviceTextAndCuts(t *testing.T) {
	for in, want := range map[string]string{
		"plain text é 日本":         "plain text é 日本",
		"a\tb":                    "a b",
		"a\x00b\x07c":             "abc",
		"a\u202eb\u2066c\u200fd":  "abcd",
		"x\u2028y\u2029z":         "xyz",
		"\ufeffbom":               "bom",
		"bad\xc3(utf8":            "bad(utf8",
		"\x1b[31mred\x1b[0m":      "[31mred[0m",
		"nonchar\ufffe\U0010ffff": "nonchar",
		"private\ue000use":        "privateuse",
	} {
		if got := cleanDeviceText(in); got != want {
			t.Errorf("cleanDeviceText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cutRunes("日本語日本語", 4); got != "日本語…" {
		t.Errorf("cutRunes = %q", got)
	}
	if got := cutRunes("abc", 3); got != "abc" {
		t.Errorf("cutRunes at the limit = %q", got)
	}
	if got := capBytes("aé", 2); got != "a" || !utf8.ValidString(got) {
		t.Errorf("capBytes split a character: %q", got)
	}
	if got := capBytes("abc", 10); got != "abc" {
		t.Errorf("capBytes = %q", got)
	}
}
