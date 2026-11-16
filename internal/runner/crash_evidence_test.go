package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/shellquote"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

const (
	cePkg    = "dev.probe.webviewfixture"
	ceSerial = "emulator-5600"
)

// ceSample reads a real capture of the device package (testdata/crash).
func ceSample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "device", "testdata", "crash", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ceAdb answers the device questions of the collector from fixed text and records every call, in order, in a log shared with other fakes.
type ceAdb struct {
	mu            sync.Mutex
	log           *[]string
	calls         [][]string
	date          string
	dateErr       error
	pidof         []string // one answer per call of pidof, the last repeated; "" means no process (exit status 1)
	pidCalls      int
	pidAfterClose string   // when set, pidof answers this once the log shows the client was closed (a reconnect began)
	exit          []string // one answer per call of exit-info, the last repeated
	exitCalls     int
	exitErr       error
	buf           string
}

func (a *ceAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil { // like the real adb, which runs under the context: an expired one fails the call
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, append([]string(nil), args...))
	if a.log != nil {
		*a.log = append(*a.log, "adb "+strings.Join(args[2:], " "))
	}
	switch {
	case len(args) > 3 && args[3] == "date":
		return []byte(a.date + "\n"), a.dateErr
	case len(args) > 3 && args[3] == "pidof":
		if a.pidAfterClose != "" && a.log != nil {
			for _, l := range *a.log {
				if l == "client close" {
					return []byte(a.pidAfterClose + "\n"), nil
				}
			}
		}
		i := a.pidCalls
		a.pidCalls++
		if i >= len(a.pidof) {
			i = len(a.pidof) - 1
		}
		if i < 0 || a.pidof[i] == "" {
			return nil, errors.New("exit status 1")
		}
		return []byte(a.pidof[i] + "\n"), nil
	case len(args) > 2 && args[2] == "logcat":
		return []byte(a.buf), nil
	case len(args) > 5 && args[5] == "exit-info":
		i := a.exitCalls
		a.exitCalls++
		if i >= len(a.exit) {
			i = len(a.exit) - 1
		}
		if a.exitErr != nil {
			return nil, a.exitErr
		}
		return []byte(a.exit[i]), nil
	}
	return nil, errors.New("unexpected adb call")
}

func (a *ceAdb) words(cmd string) int {
	n := 0
	for _, c := range a.calls {
		if len(c) > 3 && (c[3] == cmd || (len(c) > 5 && c[5] == cmd) || (c[2] == "logcat" && cmd == "logcat")) {
			n++
		}
	}
	return n
}

func ceEntry(pid, ts, reason string) string {
	return "        ApplicationExitInfo #0:\n          timestamp=" + ts + " pid=" + pid + " realUid=10207 packageUid=10207 definingUid=10207 user=0\n          process=" + cePkg + " reason=" + reason + " (x) status=0\n          importance=100 description=d state=empty\n"
}

func ceProbe(a *ceAdb) crashProbe { return crashProbe{runner: a, serial: ceSerial, app: cePkg} }

func TestJudgeDeath(t *testing.T) {
	now := time.Date(2026, 11, 15, 19, 31, 12, 0, time.UTC)
	at := func(sec int, kind device.ExitKind) device.ExitRecord {
		return device.ExitRecord{WallClock: now.Add(-time.Duration(sec) * time.Second), PID: 100 + sec, Kind: kind}
	}
	window := time.Minute
	for name, c := range map[string]struct {
		alive   bool
		exits   []device.ExitRecord
		now     time.Time
		dead    bool
		wantPID int
	}{
		"pid changed: alive, a recent crash":  {true, []device.ExitRecord{at(3, device.ExitCrash)}, now, true, 103},
		"pid gone, a recent native crash":     {false, []device.ExitRecord{at(3, device.ExitNativeCrash)}, now, true, 103},
		"a recent ANR kill":                   {true, []device.ExitRecord{at(3, device.ExitANR)}, now, true, 103},
		"killed by a signal":                  {true, []device.ExitRecord{at(3, device.ExitSignaled)}, now, true, 103},
		"low memory":                          {true, []device.ExitRecord{at(3, device.ExitLowMemory)}, now, true, 103},
		"the app exited itself":               {true, []device.ExitRecord{at(3, device.ExitSelf)}, now, true, 103},
		"slow: alive and nothing recent":      {true, nil, now, false, 0},
		"slow: alive, only an old crash":      {true, []device.ExitRecord{at(600, device.ExitCrash)}, now, false, 0},
		"gone, only an old crash":             {false, []device.ExitRecord{at(600, device.ExitCrash)}, now, true, 0},
		"gone, nothing listed":                {false, nil, now, true, 0},
		"a force-stop is not a death (alive)": {true, []device.ExitRecord{at(3, device.ExitUserRequested)}, now, false, 0},
		"a recent force-stop entry with nothing running (the test did not stop it)": {false, []device.ExitRecord{at(3, device.ExitUserRequested)}, now, true, 0},
		"an old force-stop with nothing running":                                    {false, []device.ExitRecord{at(600, device.ExitUserRequested)}, now, true, 0},
		"a force-stop and then a crash":                                             {false, []device.ExitRecord{at(1, device.ExitUserRequested), at(3, device.ExitCrash)}, now, true, 103},
		"an unknown reason is not a death":                                          {true, []device.ExitRecord{at(3, device.ExitUnknown)}, now, false, 0},
		"the newest entry is a force-stop":                                          {true, []device.ExitRecord{at(1, device.ExitUserRequested), at(3, device.ExitCrash)}, now, true, 103},
		"the window edge is inside":                                                 {true, []device.ExitRecord{at(60, device.ExitCrash)}, now, true, 160},
		"just past the window":                                                      {true, []device.ExitRecord{at(61, device.ExitCrash)}, now, false, 0},
		"no device clock: nothing can be said":                                      {false, []device.ExitRecord{at(3, device.ExitCrash)}, time.Time{}, false, 0},
	} {
		dead, rec := judgeDeath(c.alive, c.exits, c.now, window, false)
		if dead != c.dead || (c.wantPID != 0) != (rec != nil) || (rec != nil && rec.PID != c.wantPID) {
			t.Errorf("%s: dead=%v rec=%v, want dead=%v pid=%d", name, dead, rec, c.dead, c.wantPID)
		}
	}
}

func TestCollectCrashFindsARealJavaCrash(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || !r.Gone || r.Kind != device.ExitCrash || r.Evidence.Exit == nil || r.Evidence.Exit.PID != 3567 {
		t.Fatalf("report = %+v", r)
	}
	if !strings.HasPrefix(r.Evidence.Summary, "java.lang.RuntimeException: Unable to start activity") || len(r.Evidence.CrashLines) < 20 {
		t.Errorf("summary %q, %d lines", r.Evidence.Summary, len(r.Evidence.CrashLines))
	}
	if !strings.HasPrefix(r.Line(), "the app died during this test: crash (pid 3567): java.lang.RuntimeException") {
		t.Errorf("line = %q", r.Line())
	}
	// the commands: the device named on every call, read-only, in this order
	want := []string{"date", "exit-info", "logcat", "pidof"}
	got := []string{}
	for _, c := range a.calls {
		if c[0] != "-s" || c[1] != ceSerial {
			t.Errorf("a call without the device: %q", c)
		}
		for _, w := range c {
			if w == "forward" || w == "--remove-all" || w == "-c" || w == "clear" || w == "kill" || w == "force-stop" || w == "start" || w == "am" {
				t.Errorf("not read-only: %q", c)
			}
		}
		switch {
		case c[3] == "date":
			got = append(got, "date")
		case len(c) > 5 && c[5] == "exit-info":
			got = append(got, "exit-info")
		case c[3] == "pidof":
			got = append(got, "pidof")
		case c[2] == "logcat":
			got = append(got, "logcat")
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if a.calls[0][4] != shellquote.Quote("+%Y-%m-%d %H:%M:%S") {
		t.Errorf("the date format reaches the shell as %q", a.calls[0][4])
	}
}

func TestCollectCrashNativeCrashReadsTheTombstone(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:20", pidof: []string{""}, exit: []string{ceSample(t, "exit-segv.txt")}, buf: ceSample(t, "crashbuf-segv.txt")}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || r.Kind != device.ExitNativeCrash || r.Evidence.Summary != "Fatal signal 11 (SIGSEGV)" || len(r.Evidence.CrashLines) < 50 {
		t.Fatalf("report = %+v", r)
	}
}

// A restarted app has a new pid: it is alive, and the recent exit entry still says the first one died.
func TestCollectCrashPidChanged(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"4242"}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || r.Gone || r.Kind != device.ExitCrash || r.Evidence.Exit.PID != 3567 {
		t.Fatalf("report = %+v", r)
	}
	if !strings.Contains(r.Line(), "crash (pid 3567)") {
		t.Errorf("line = %q", r.Line())
	}
}

func TestCollectCrashSlowAppIsNotReported(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3567 3570"}, exit: []string{"ACTIVITY MANAGER PROCESS EXIT INFO\n"}}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil {
		t.Errorf("a slow app was reported: %+v", r)
	}
	if len(a.calls) != 3 || a.words("logcat") != 0 {
		t.Errorf("a slow app costs date, exit-info and pidof only, got %d calls", len(a.calls))
	}
	// an old crash is not this test's
	a = &ceAdb{date: "2026-11-15 21:00:00", pidof: []string{"3567"}, exit: []string{ceSample(t, "exit-java.txt")}}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil {
		t.Errorf("an old crash was reported: %+v", r)
	}
}

// The exit entry appears a few hundred milliseconds after the death: a gone app with no entry is looked at again.
func TestCollectCrashWaitsBrieflyForTheEntry(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{"ACTIVITY MANAGER PROCESS EXIT INFO\n", "ACTIVITY MANAGER PROCESS EXIT INFO\n", ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || r.Kind != device.ExitCrash || a.exitCalls != 3 {
		t.Fatalf("report = %+v after %d looks", r, a.exitCalls)
	}
	// no entry ever: a report that the process is gone, after exactly crashPolls looks
	a = &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{"ACTIVITY MANAGER PROCESS EXIT INFO\n"}}
	r = collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || !r.Gone || r.Kind != device.ExitUnknown || r.Evidence.Exit != nil || a.exitCalls != crashPolls {
		t.Fatalf("report = %+v after %d looks", r, a.exitCalls)
	}
	if r.Line() != "the app died during this test: the app process was gone and the device listed no exit for it yet" {
		t.Errorf("line = %q", r.Line())
	}
	if a.words("logcat") != 0 {
		t.Error("the crash buffer was read with no process to read it for")
	}
	// a cancelled context stops the waiting
	ctx, cancel := context.WithCancel(context.Background())
	a = &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{"x\n"}}
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	collectCrash(ctx, ceProbe(a), time.Minute, time.Hour, nil, false)
	if time.Since(start) > 5*time.Second {
		t.Errorf("the wait did not stop on cancel: %v", time.Since(start))
	}
}

// The newest entry may be a later force-stop: the crash text read is the one of the process that died.
func TestCollectCrashChoosesTheDeadProcessNotTheNewestEntry(t *testing.T) {
	exit := ceEntry("3600", "2026-11-15 19:31:11.000", "10") + strings.Replace(ceEntry("3567", "2026-11-15 19:31:09.870", "4"), "ApplicationExitInfo #0", "ApplicationExitInfo #1", 1)
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3700"}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || r.Kind != device.ExitCrash || r.Evidence.Exit == nil || r.Evidence.Exit.PID != 3567 || !strings.HasPrefix(r.Evidence.Summary, "java.lang.RuntimeException") {
		t.Fatalf("report = %+v", r)
	}
	if a.exitCalls != 2 {
		t.Errorf("exit-info was read %d times, want 2 (the second for the process that died)", a.exitCalls)
	}
}

func TestCollectCrashGivesUpQuietlyWhenTheDeviceCannotBeRead(t *testing.T) {
	a := &ceAdb{date: "", dateErr: errors.New("device offline"), exit: []string{ceSample(t, "exit-java.txt")}}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil || len(a.calls) != 1 {
		t.Errorf("a failing clock: %+v after %d calls", r, len(a.calls))
	}
	a = &ceAdb{date: "not a date", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil || len(a.calls) != 1 {
		t.Errorf("an unreadable clock: %+v after %d calls", r, len(a.calls))
	}
	a = &ceAdb{date: strings.Repeat("9", 100), pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil {
		t.Errorf("a long clock answer was believed: %+v", r)
	}
	// a transport failure on exit-info is not "the app is gone"
	a = &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{""}, exitErr: errors.New("adb: device offline")}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false); r != nil || a.words("pidof") != 0 {
		t.Errorf("exit-info failed: %+v, pidof calls %d", r, a.words("pidof"))
	}
	// text that is not a pid list is not a running app
	for _, out := range []string{"abc", "12;rm", strings.Repeat("1 ", 40), "1\n2"} {
		a = &ceAdb{pidof: []string{out}}
		if ceProbe(a).alive(context.Background()) && out != "1\n2" {
			t.Errorf("pidof output %q was taken for a running app", out)
		}
	}
}

// The evidence is masked with the run's known secrets and the credential shapes, and home paths are shortened.
func TestCollectCrashMasksWhatTheDeviceSaid(t *testing.T) {
	sysdialog.NoteSecret("ce-known-secret-9f3a")
	buf := "11-15 19:03:48.050  3567  3567 E AndroidRuntime: java.lang.IllegalStateException: boom ce-known-secret-9f3a\n" +
		"11-15 19:03:48.051  3567  3567 E AndroidRuntime: \tat x.Y(Z.java:1) password=hunter2hunter2 /Users/someone/app/file.dart\n" +
		"11-15 19:03:48.052  3567  3567 E AndroidRuntime: Authorization: Bearer abcdefghijklmnop1234\n"
	exit := strings.Replace(ceSample(t, "exit-java.txt"), "description=crash state", "description=ce-known-secret-9f3a state", 1)
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{exit}, buf: buf}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil {
		t.Fatal("no report")
	}
	all := r.Line() + "\n" + strings.Join(r.Evidence.CrashLines, "\n") + "\n" + r.Evidence.Exit.Description
	for _, secret := range []string{"ce-known-secret-9f3a", "hunter2hunter2", "abcdefghijklmnop1234", "/Users/someone"} {
		if strings.Contains(all, secret) {
			t.Errorf("%q survived in:\n%s", secret, all)
		}
	}
	if !strings.Contains(all, "~/app/file.dart") {
		t.Errorf("the home path was not shortened:\n%s", all)
	}
}

func ceExecutor(a *ceAdb, on bool) *Executor {
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: on, CrashADB: a, CrashPoll: time.Millisecond}
	e := NewExecutor(nil, dc, nil, time.Second, false)
	e.startedAt = time.Now()
	return e
}

func TestCollectCrashOnceGuards(t *testing.T) {
	dead := func() *ceAdb {
		return &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	}
	// off: no adb call at all
	a := dead()
	e := ceExecutor(a, false)
	e.collectCrashOnce(context.Background())
	if len(a.calls) != 0 || e.CrashReport() != nil {
		t.Errorf("off: %d calls, report %v", len(a.calls), e.CrashReport())
	}
	// iOS, no app id, no serial, cloud: none either
	for name, mod := range map[string]func(*DeviceContext){
		"ios":       func(dc *DeviceContext) { dc.Platform = device.PlatformIOS },
		"no app id": func(dc *DeviceContext) { dc.AppID = "" },
		"no serial": func(dc *DeviceContext) { dc.Serial = " " },
		"no adb":    func(dc *DeviceContext) { dc.CrashADB = nil },
	} {
		a = dead()
		e = ceExecutor(a, true)
		mod(e.deviceCtx)
		e.collectCrashOnce(context.Background())
		if len(a.calls) != 0 || e.CrashReport() != nil {
			t.Errorf("%s: %d calls", name, len(a.calls))
		}
	}
	e = NewExecutor(nil, nil, nil, time.Second, false)
	e.collectCrashOnce(context.Background())
	if e.CrashReport() != nil {
		t.Error("a cloud run collected")
	}
	// on: one collection, and a second call after a report adds nothing
	a = dead()
	e = ceExecutor(a, true)
	e.collectCrashOnce(context.Background())
	n := len(a.calls)
	if e.CrashReport() == nil || n == 0 {
		t.Fatalf("report %v after %d calls", e.CrashReport(), n)
	}
	e.collectCrashOnce(context.Background())
	if len(a.calls) != n {
		t.Errorf("a second collection ran: %d calls, was %d", len(a.calls), n)
	}
	// a slow app is looked at again, at most crashMaxCollections times
	a = &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3567"}, exit: []string{"x\n"}}
	e = ceExecutor(a, true)
	for i := 0; i < 10; i++ {
		e.collectCrashOnce(context.Background())
	}
	if e.CrashReport() != nil || a.words("date") != crashMaxCollections {
		t.Errorf("slow app: %d collections, want %d", a.words("date"), crashMaxCollections)
	}
}

// ceClient fails a device action with a lost connection and logs when it is closed, which is the first thing a reconnect attempt does.
type ceClient struct {
	fakeAIClient
	log *[]string
}

func (c *ceClient) DeviceAction(ctx context.Context, action, value string) error {
	return errors.New("probelink: write: broken pipe")
}
func (c *ceClient) Close() error {
	*c.log = append(*c.log, "client close")
	return nil
}

// The evidence is read before the reconnect attempt touches the connection, and a run without the opt-in makes no adb call.
func TestALostConnectionCollectsBeforeAnyReconnect(t *testing.T) {
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	run := func(on bool) *Executor {
		e := ceExecutor(a, on)
		e.client = &ceClient{log: &log}
		e.SetReconnectPolicy(1, time.Hour)
		e.timeout = 50 * time.Millisecond
		_ = e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
		return e
	}
	e := run(true)
	if e.CrashReport() == nil || e.CrashReport().Kind != device.ExitCrash {
		t.Fatalf("report = %+v", e.CrashReport())
	}
	closeAt, lastAdb := -1, -1
	for i, l := range log {
		if l == "client close" && closeAt < 0 {
			closeAt = i
		}
		if strings.HasPrefix(l, "adb ") {
			lastAdb = i
		}
	}
	if closeAt < 0 || lastAdb < 0 || lastAdb > closeAt {
		t.Errorf("the adb calls must all come before the first reconnect step: %v", log)
	}
	// opt-in off: the same failure makes no adb call
	log = nil
	a2 := &ceAdb{log: &log, date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
	a = a2
	e = run(false)
	if len(a2.calls) != 0 || e.CrashReport() != nil {
		t.Errorf("off: %d adb calls", len(a2.calls))
	}
}

// A run whose steps all pass makes no adb call, and a failure that is not a lost connection is looked at only after the step has failed (see the timeout tests).
func TestNoCollectionWithoutAFailure(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
	e := ceExecutor(a, true)
	var log []string
	e.client = &ceFailingClient{log: &log}
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter}); err != nil || len(a.calls) != 0 {
		t.Errorf("a passing step: err %v, %d adb calls", err, len(a.calls))
	}
	// a failure that is not a lost connection is looked at once, after the step, and never before a reconnect: nothing was closed or retried
	a2 := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3567"}, exit: []string{"x\n"}}
	e2 := ceExecutor(a2, true)
	e2.client = &ceFailingClient{log: &log, err: errors.New("rpc error -32001: Widget not found")}
	_ = e2.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	if a2.words("date") != 1 {
		t.Errorf("%d looks after a missing widget, want 1", a2.words("date"))
	}
}

type ceFailingClient struct {
	fakeAIClient
	log *[]string
	err error
}

func (c *ceFailingClient) DeviceAction(ctx context.Context, action, value string) error { return c.err }

// Only a failed test carries the evidence and prints its line; a test that passed after the reconnect says nothing.
func TestOnlyAFailedTestCarriesTheEvidence(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	e := ceExecutor(a, true)
	e.collectCrashOnce(context.Background())
	if e.CrashReport() == nil {
		t.Fatal("nothing collected")
	}
	if got := failedTestCrash(e, nil); got != nil {
		t.Errorf("a passed test carries evidence: %+v", got)
	}
	if got := failedTestCrash(e, errors.New("step failed")); got == nil || got != e.CrashReport() {
		t.Errorf("a failed test dropped the evidence: %+v", got)
	}
	if got := failedTestCrash(ceExecutor(&ceAdb{}, true), errors.New("step failed")); got != nil {
		t.Errorf("a failed test with nothing collected carries %+v", got)
	}
	var nilReport *CrashReport
	if nilReport.Line() != "" {
		t.Error("a nil report has a line")
	}
}

func TestTheOptInDefaultsToOffAndIsReadFromTheConfig(t *testing.T) {
	var cfg config.Config
	if cfg.Defaults.CrashEvidence {
		t.Error("crash evidence is on by default")
	}
	if err := yaml.Unmarshal([]byte("defaults:\n  crash_evidence: true\n"), &cfg); err != nil || !cfg.Defaults.CrashEvidence {
		t.Errorf("yaml: %v, %+v", err, cfg.Defaults)
	}
	if dc := (&DeviceContext{}); dc.CrashEvidence {
		t.Error("a device context starts with crash evidence on")
	}
}

// The window of "during this test" is the time since the attempt began plus a grace of ten seconds, never more than six hours.
func TestTheDeathWindow(t *testing.T) {
	const now = "2026-11-15 19:31:12"
	crashAt := func(ts string) string { return ceEntry("3567", ts, "4") }
	for name, c := range map[string]struct {
		since time.Duration
		ts    string
		want  bool
	}{
		"inside the elapsed time":               {100 * time.Second, "2026-11-15 19:30:00.000", true},
		"inside only because of the grace":      {100 * time.Second, "2026-11-15 19:29:27.000", true},  // 105 s old, window 110 s
		"past the grace":                        {100 * time.Second, "2026-11-15 19:29:20.000", false}, // 112 s old
		"a test that just began":                {2 * time.Second, "2026-11-15 19:31:02.000", true},    // 10 s old, window 12 s
		"a crash before a test that just began": {2 * time.Second, "2026-11-15 19:30:50.000", false},   // 22 s old: the test before this one
		"a long test, inside the maximum":       {24 * time.Hour, "2026-11-15 13:31:13.000", true},     // just under 6 h
		"a long test, past the maximum":         {24 * time.Hour, "2026-11-15 12:31:12.000", false},    // 7 h old
	} {
		a := &ceAdb{date: now, pidof: []string{"3600"}, exit: []string{crashAt(c.ts)}, buf: ceSample(t, "crashbuf-java.txt")}
		if got := collectCrash(context.Background(), ceProbe(a), c.since, time.Millisecond, nil, false) != nil; got != c.want {
			t.Errorf("%s: reported = %v, want %v", name, got, c.want)
		}
	}
}

// pidof is asked about the app by its quoted name, and a bad app id stops before any adb call.
func TestThePackageIsQuotedAndValidatedBeforeAnyCall(t *testing.T) {
	a := &ceAdb{pidof: []string{"1"}}
	p := crashProbe{runner: a, serial: ceSerial, app: "a.b;rm -rf /"}
	p.alive(context.Background())
	if got := a.calls[0][len(a.calls[0])-1]; got != shellquote.Quote("a.b;rm -rf /") {
		t.Errorf("pidof argument = %q", got)
	}
	for _, bad := range []string{"a.b;rm", "-a.b", "nodots", "a..b", "a.b'c"} {
		a = &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
		e := ceExecutor(a, true)
		e.deviceCtx.AppID = bad
		e.collectCrashOnce(context.Background())
		if len(a.calls) != 0 || e.CrashReport() != nil {
			t.Errorf("%q: %d adb calls", bad, len(a.calls))
		}
	}
}

// ---- review of #636 ----

// The printed line is one plain line whatever the evidence holds.
func TestThePrintedLineIsOneLineOfPlainText(t *testing.T) {
	for name, hostile := range map[string]string{
		"escape sequence": "boom \x1b[2J\x1b[31mred\x1b[0m",
		"carriage return": "visible\roverwritten",
		"new lines":       "line one\nline two\r\nline three",
		"NUL":             "a\x00b\x00c",
		"bidi":            "abc\u202edef\u2066ghi\u200fjkl",
		"separators":      "a\u2028b\u2029c",
		"tabs and runs":   "a\t\t   b",
		"hidden letters":  "a\u3164b\u2800c\ufe0fd\ufffce",
	} {
		c := &CrashReport{Gone: true, Kind: device.ExitCrash, Evidence: device.CrashEvidence{Summary: hostile, Exit: &device.ExitRecord{PID: 7}}}
		line := c.Line()
		for _, r := range line {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029 || r == 0x202e || r == 0x2066 || r == 0x200f || r == 0x3164 || r == 0x2800 || r == 0xfe0f || r == 0xfffc {
				t.Errorf("%s: control or hidden character U+%04X in %q", name, r, line)
			}
		}
		if strings.Contains(line, "  ") || strings.HasSuffix(line, " ") {
			t.Errorf("%s: spaces not collapsed in %q", name, line)
		}
		if !strings.HasPrefix(line, "the app died during this test: crash (pid 7): ") {
			t.Errorf("%s: line = %q", name, line)
		}
	}
	// what is written stays: the characters are replaced, not the words
	c := &CrashReport{Kind: device.ExitCrash, Evidence: device.CrashEvidence{Summary: "boom \x1b[2J \r tail", Exit: &device.ExitRecord{PID: 7}}}
	if c.Line() != "the app died during this test: crash (pid 7): boom [2J tail" {
		t.Errorf("line = %q", c.Line())
	}
	// the cut
	long := &CrashReport{Kind: device.ExitCrash, Evidence: device.CrashEvidence{Summary: strings.Repeat("x", 1000), Exit: &device.ExitRecord{PID: 7}}}
	if n := len([]rune(long.Line())); n != crashMaxLineRunes {
		t.Errorf("a long line has %d runes, want %d", n, crashMaxLineRunes)
	}
	// and the summary kept in the report, which the reports will carry, is one line too
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: "11-15 19:03:48.050  3567  3567 E AndroidRuntime: java.lang.Error: a\u202e   b  \tc\n"}
	r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
	if r == nil || r.Evidence.Summary != "java.lang.Error: a b c" {
		t.Errorf("summary = %+v", r)
	}
}

// Every exit reason number: the ones that mean the app ended unexpectedly are deaths, the ones that mean the system or the user changed something are not.
func TestEveryExitReasonIsOrIsNotADeath(t *testing.T) {
	deaths := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 9: true, 12: true}
	for reason := 0; reason <= 17; reason++ {
		exit := ceEntry("3567", "2026-11-15 19:31:09.870", strconv.Itoa(reason))
		a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3600"}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
		got := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false) != nil
		if got != deaths[reason] {
			t.Errorf("reason %d: reported = %v, want %v", reason, got, deaths[reason])
		}
	}
	for reason, want := range map[int]device.ExitKind{7: device.ExitInitFailure, 8: device.ExitSystemChange, 9: device.ExitResource, 12: device.ExitDependency, 13: device.ExitOther, 14: device.ExitSystemChange, 15: device.ExitSystemChange, 16: device.ExitSystemChange, 99: device.ExitOther} {
		recs := device.ParseExitInfo(ceEntry("3567", "2026-11-15 19:31:09.870", strconv.Itoa(reason)), cePkg)
		if len(recs) != 1 || recs[0].Kind != want {
			t.Errorf("reason %d: %+v, want kind %s", reason, recs, want)
		}
	}
}

// The crash of one test is not blamed on the next: an entry a collection has chosen is not chosen again.
func TestAnEarlierTestsCrashIsNotBlamedOnTheNextOne(t *testing.T) {
	var seen seenCrashes
	exit := ceSample(t, "exit-java.txt")
	// test A: its app died and the evidence was collected
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
	if r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, &seen, false); r == nil || r.Evidence.Exit.PID != 3567 {
		t.Fatalf("test A: %+v", r)
	}
	// test B starts five seconds later, loses its connection to a slow app: the same entry is still in the list and still inside its window
	b := &ceAdb{date: "2026-11-15 19:31:17", pidof: []string{"3700"}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
	if r := collectCrash(context.Background(), ceProbe(b), 6*time.Second, time.Millisecond, &seen, false); r != nil {
		t.Errorf("test B was blamed for test A's crash: %+v", r)
	}
	// without the seen the entry would be blamed: the test above is about the seen
	b2 := &ceAdb{date: "2026-11-15 19:31:17", pidof: []string{"3700"}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
	if r := collectCrash(context.Background(), ceProbe(b2), 6*time.Second, time.Millisecond, nil, false); r == nil {
		t.Error("the control case did not report: the test proves nothing")
	}
	// a new crash in test B is its own
	newer := ceEntry("3800", "2026-11-15 19:31:15.000", "5") + strings.Replace(exit, "ApplicationExitInfo #0", "ApplicationExitInfo #1", 1)
	c := &ceAdb{date: "2026-11-15 19:31:17", pidof: []string{""}, exit: []string{newer}, buf: ceSample(t, "crashbuf-segv.txt")}
	if r := collectCrash(context.Background(), ceProbe(c), 6*time.Second, time.Millisecond, &seen, false); r == nil || r.Evidence.Exit.PID != 3800 || r.Kind != device.ExitNativeCrash {
		t.Errorf("test B's own crash: %+v", r)
	}
	// the same through the executor: two attempts of one device share the seen, and an attempt that collected for a test that then passed still marks it
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: true, CrashPoll: time.Millisecond}
	first := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{exit}, buf: ceSample(t, "crashbuf-java.txt")}
	dc.CrashADB = first
	e1 := NewExecutor(nil, dc, nil, time.Second, false)
	e1.startedAt = time.Now()
	e1.collectCrashOnce(context.Background())
	if e1.CrashReport() == nil {
		t.Fatal("attempt 1 collected nothing")
	}
	second := &ceAdb{date: "2026-11-15 19:31:17", pidof: []string{"3700"}, exit: []string{exit}}
	dc.CrashADB = second
	e2 := NewExecutor(nil, dc, nil, time.Second, false)
	e2.startedAt = time.Now()
	e2.collectCrashOnce(context.Background())
	if e2.CrashReport() != nil {
		t.Errorf("attempt 2 was blamed: %+v", e2.CrashReport())
	}
}

// A force-stop that comes after the death (the next test relaunching the app, say) does not hide it, whether the app is running again or not.
func TestALaterForceStopDoesNotHideTheDeath(t *testing.T) {
	exit := ceEntry("3600", "2026-11-15 19:31:11.000", "10") + strings.Replace(ceEntry("3567", "2026-11-15 19:31:09.870", "5"), "ApplicationExitInfo #0", "ApplicationExitInfo #1", 1)
	for name, alive := range map[string]string{"app running again": "3700", "app gone": ""} {
		a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{alive}, exit: []string{exit}, buf: ceSample(t, "crashbuf-segv.txt")}
		r := collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, nil, false)
		if r == nil || r.Kind != device.ExitNativeCrash || r.Evidence.Exit.PID != 3567 || r.Gone != (alive == "") {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// and a force-stop alone is not one
	only := ceEntry("3600", "2026-11-15 19:31:11.000", "10")
	if r := collectCrash(context.Background(), ceProbe(&ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3700"}, exit: []string{only}}), time.Minute, time.Millisecond, nil, false); r != nil {
		t.Errorf("a force-stop alone was reported: %+v", r)
	}
}

// adb that never answers: the collection ends at its own bound, and at once when the run is cancelled.
func TestTheCollectionIsBoundedAndCancellable(t *testing.T) {
	if crashTotalTimeout != 20*time.Second {
		t.Errorf("the bound is %v, documented as 20 s", crashTotalTimeout)
	}
	saved := crashTotalTimeout
	defer func() { crashTotalTimeout = saved }()
	crashTotalTimeout = 60 * time.Millisecond
	hang := func() *ceHangAdb { return &ceHangAdb{} }
	a := hang()
	e := ceExecutorWith(a)
	start := time.Now()
	e.collectCrashOnce(context.Background())
	if d := time.Since(start); d > 5*time.Second || e.CrashReport() != nil || a.calls.Load() == 0 {
		t.Errorf("bound: took %v, report %v, %d calls", d, e.CrashReport(), a.calls.Load())
	}
	// the parent's cancel stops it long before the bound
	crashTotalTimeout = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	a = hang()
	e = ceExecutorWith(a)
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	start = time.Now()
	e.collectCrashOnce(ctx)
	if d := time.Since(start); d > 5*time.Second || e.CrashReport() != nil {
		t.Errorf("cancel: took %v, report %v", d, e.CrashReport())
	}
}

type ceHangAdb struct{ calls atomic.Int32 }

func (h *ceHangAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	h.calls.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

func ceExecutorWith(a device.AdbRunner) *Executor {
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: true, CrashADB: a, CrashPoll: time.Millisecond}
	e := NewExecutor(nil, dc, nil, time.Second, false)
	e.startedAt = time.Now()
	return e
}

// A pid can come back: a later crash of a process with the same number is a different entry.
func TestAReusedPidIsANewEntry(t *testing.T) {
	var seen seenCrashes
	first := ceEntry("3567", "2026-11-15 19:31:09.870", "4")
	a := &ceAdb{date: "2026-11-15 19:31:12", pidof: []string{"3700"}, exit: []string{first}, buf: ceSample(t, "crashbuf-java.txt")}
	if collectCrash(context.Background(), ceProbe(a), time.Minute, time.Millisecond, &seen, false) == nil {
		t.Fatal("the first crash was not reported")
	}
	again := ceEntry("3567", "2026-11-15 19:40:00.000", "5")
	b := &ceAdb{date: "2026-11-15 19:40:03", pidof: []string{"3700"}, exit: []string{again}, buf: ceSample(t, "crashbuf-java.txt")}
	if r := collectCrash(context.Background(), ceProbe(b), time.Minute, time.Millisecond, &seen, false); r == nil || r.Kind != device.ExitNativeCrash {
		t.Errorf("a new crash with a reused pid was taken for the old one: %+v", r)
	}
}

// With a kill step in the test and no launch since, no process is the test's own doing; every other case with no process is a death, a force-stop entry or not.
func TestJudgeDeathWhenTheTestStoppedTheApp(t *testing.T) {
	now := time.Date(2026, 11, 15, 19, 31, 12, 0, time.UTC)
	at := func(sec int, kind device.ExitKind) device.ExitRecord {
		return device.ExitRecord{WallClock: now.Add(-time.Duration(sec) * time.Second), PID: 100 + sec, Kind: kind}
	}
	window := time.Minute
	for name, c := range map[string]struct {
		alive, stopped bool
		exits          []device.ExitRecord
		dead           bool
	}{
		"killed by the test, nothing running":            {false, true, []device.ExitRecord{at(3, device.ExitUserRequested)}, false},
		"killed by the test, no entry at all":            {false, true, nil, false},
		"not killed by the test, nothing running":        {false, false, []device.ExitRecord{at(3, device.ExitUserRequested)}, true},
		"restarted by the test and dead since, no entry": {false, false, []device.ExitRecord{at(7, device.ExitUserRequested)}, true},
		"killed by the test and a crash entry before it": {false, true, []device.ExitRecord{at(1, device.ExitUserRequested), at(3, device.ExitCrash)}, true},
		"killed by the test, an app that is running":     {true, true, nil, false},
	} {
		if dead, _ := judgeDeath(c.alive, c.exits, now, window, c.stopped); dead != c.dead {
			t.Errorf("%s: dead = %v, want %v", name, dead, c.dead)
		}
	}
}

// What the test did to the app is remembered from its steps: a kill that worked stops it, a restart, clear or open that worked launches it, and a step that failed
// or another step changes nothing.
func TestTheTestsOwnStopIsRememberedFromItsSteps(t *testing.T) {
	e := &Executor{}
	step := func(v parser.ActionVerb) parser.Step { return parser.ActionStep{Verb: v} }
	apply := func(v parser.ActionVerb, err error, want bool, label string) {
		t.Helper()
		e.noteAppState(step(v), err)
		if e.appStopped != want {
			t.Errorf("%s: stopped = %v, want %v", label, e.appStopped, want)
		}
	}
	apply(parser.VerbPressEnter, nil, false, "an unrelated step")
	apply(parser.VerbKill, errors.New("failed"), false, "a kill that failed")
	apply(parser.VerbKill, nil, true, "a kill")
	apply(parser.VerbPressEnter, nil, true, "an unrelated step after a kill")
	apply(parser.VerbRestart, errors.New("failed"), true, "a restart that failed")
	apply(parser.VerbRestart, nil, false, "a restart")
	apply(parser.VerbKill, nil, true, "a second kill")
	apply(parser.VerbOpen, nil, false, "open the app")
	apply(parser.VerbKill, nil, true, "a third kill")
	apply(parser.VerbClearAppData, nil, false, "clear app data")
	e.noteAppState(parser.AssertStep{}, nil) // a step that is not an action
	if e.appStopped {
		t.Error("an assert step changed the state")
	}
}

// Through the executor: after a kill step the app having no process is not a death; after a restart it is.
func TestAKillStepMakesNoProcessExpectedAndARestartDoesNot(t *testing.T) {
	deadAdb := func() *ceAdb {
		return &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:08.000", "10")}}
	}
	a := deadAdb()
	e := ceExecutor(a, true)
	e.appStopped = true
	e.collectCrashOnce(context.Background())
	if e.CrashReport() != nil {
		t.Errorf("after a kill: %+v", e.CrashReport())
	}
	a = deadAdb()
	e = ceExecutor(a, true)
	e.appStopped = false // a restart launched it again, and a force-stop entry from that restart is in the list
	e.collectCrashOnce(context.Background())
	if e.CrashReport() == nil || !e.CrashReport().Gone {
		t.Errorf("after a restart: %+v", e.CrashReport())
	}
}

// The state is kept by the executor itself as steps run, not only by the helper: an open step that works after a kill leaves the app expected to run.
func TestRunStepKeepsTheStopStateOfTheApp(t *testing.T) {
	e := ceExecutor(&ceAdb{}, true)
	e.client = &ceFailingClient{log: new([]string)}
	e.appStopped = true
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbOpen}); err != nil {
		t.Fatal(err)
	}
	if e.appStopped {
		t.Error("an open step that worked left the app marked as stopped")
	}
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter}); err != nil || e.appStopped {
		t.Errorf("an unrelated step: err %v, stopped %v", err, e.appStopped)
	}
}
