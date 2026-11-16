package runner

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/hosted"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/shellquote"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// Crash evidence in a run (docs/proposals/crash-anr-capture.md, slice S2): when a step fails with a lost connection, the runner asks whether the app under test
// died, and if it did, collects the evidence once, before anything relaunches the app. It is off unless the run asks for it (defaults.crash_evidence or
// --crash-evidence), Android only, and it adds no adb call to a run in which no step lost its connection. The evidence is held in memory and printed as one line
// when the test fails; writing it into the reports and files is slice S3.

const (
	crashMaxCollections = 3                      // collections tried in one test attempt (a slow app that later dies is looked at again)
	crashPolls          = 4                      // looks at a device whose app has no process yet and no exit entry (the entry appears within about 300 ms)
	crashPollEvery      = 300 * time.Millisecond // between two such looks
	crashMaxLineRunes   = 240                    // the printed line
	crashWindowGrace    = 10 * time.Second       // an exit entry may be this much older than the start of the test and still belong to it
	crashMaxWindow      = 6 * time.Hour
)

// crashTotalTimeout bounds everything one collection does. It is a variable only so a test can shorten it; nothing else changes it.
var crashTotalTimeout = 20 * time.Second

// seenCrashes remembers which exit entries a run has already looked at, so that the crash of one test is never blamed on the next: an entry is marked when a
// collection chooses it, whether or not that test went on to fail. The zero value is ready; a nil value remembers nothing.
type seenCrashes struct {
	mu   sync.Mutex
	seen map[string]bool
}

func exitKey(r device.ExitRecord) string {
	return strconv.Itoa(r.PID) + "@" + r.WallClock.Format("2006-01-02 15:04:05.000")
}

func (l *seenCrashes) has(r device.ExitRecord) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seen[exitKey(r)]
}

func (l *seenCrashes) mark(r device.ExitRecord) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	l.seen[exitKey(r)] = true
}

// unseen returns the entries that are not marked yet, in their order.
func (l *seenCrashes) unseen(exits []device.ExitRecord) []device.ExitRecord {
	var out []device.ExitRecord
	for _, r := range exits {
		if !l.has(r) {
			out = append(out, r)
		}
	}
	return out
}

// CrashReport is what a failed test carries when its app died during it.
type CrashReport struct {
	// Gone is true when the app had no process at the time it was looked at.
	Gone bool
	// Kind says what the exit entry means (crash, native_crash, anr, ...); device.ExitUnknown when the process was gone but the device listed no entry for this test.
	Kind device.ExitKind
	// Evidence is the cleaned and masked evidence of the chosen process: the exit entry, the crash text and a one-line summary.
	Evidence device.CrashEvidence
	// Files are the files written for this report (absolute paths), set by the runner when the test failed; empty when none were.
	Files []string
}

// Line is the one line printed for a failed test. It holds only masked text and is cut by the collector.
func (c *CrashReport) Line() string {
	if c == nil {
		return ""
	}
	what := "the app process was gone and the device listed no exit for it yet"
	if c.Evidence.Exit != nil {
		what = fmt.Sprintf("%s (pid %d)", c.Kind, c.Evidence.Exit.PID)
	} else if !c.Gone {
		what = "the app exited"
	}
	if c.Evidence.Summary != "" {
		what += ": " + c.Evidence.Summary
	}
	// one line of plain text whatever the evidence holds: nothing from the device can move the cursor, clear the screen or start a second line
	return device.CleanOneLine("the app died during this test: "+what, crashMaxLineRunes)
}

// deathKinds are the exit reasons that mean the app ended without being asked to and not because of a change the system made around it. A force-stop (user
// requested), a change of permission or package or freezing (system_change), "other kills by system" and an unknown reason are not.
var deathKinds = map[device.ExitKind]bool{
	device.ExitSelf: true, device.ExitSignaled: true, device.ExitLowMemory: true, device.ExitCrash: true, device.ExitNativeCrash: true, device.ExitANR: true,
	device.ExitInitFailure: true, device.ExitResource: true, device.ExitDependency: true,
}

// judgeDeath decides dead or slow from what was seen. exits are newest first, deviceNow is the device's clock and window how long ago the test began, in the
// device's time. An entry of an unexpected ending that is not older than the window belongs to this test: the app died, even if it has been started again since
// (its pid changed). With no such entry, an app that has no process is gone, unless the test stopped it on purpose (appStopped), and an app that has one is only slow. Without the device's clock nothing can be said.
func judgeDeath(alive bool, exits []device.ExitRecord, deviceNow time.Time, window time.Duration, appStopped bool) (dead bool, rec *device.ExitRecord) {
	if deviceNow.IsZero() {
		return false, nil
	}
	for i := range exits {
		if deathKinds[exits[i].Kind] && !exits[i].WallClock.Before(deviceNow.Add(-window)) {
			return true, &exits[i]
		}
	}
	// no process: gone, unless the test itself stopped the app (a kill step that no later step has launched again). A force-stop entry says nothing here: a restart
	// leaves one too, and an app that is launched again and then dies has no process and no entry yet.
	return !alive && !appStopped, nil
}

var (
	pidListRE  = regexp.MustCompile(`^\d{1,10}( \d{1,10}){0,15}$`)
	deviceDate = "+%Y-%m-%d %H:%M:%S"
)

// crashProbe is one device's questions: its clock and whether the app has a process. Both go through the AdbRunner, with -s <serial>.
type crashProbe struct {
	runner device.AdbRunner
	serial string
	app    string
}

func (p crashProbe) deviceNow(ctx context.Context) (time.Time, error) {
	out, err := p.runner.RunArgs(ctx, "-s", p.serial, "shell", "date", shellquote.Quote(deviceDate))
	if err != nil || len(out) > 64 {
		return time.Time{}, fmt.Errorf("device clock unreadable")
	}
	t, perr := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(string(out)), time.UTC)
	if perr != nil {
		return time.Time{}, fmt.Errorf("device clock unreadable")
	}
	return t, nil
}

// alive reports whether the app has a process. A failing pidof (exit status 1 when there is none) means it has not; the caller has already read the device, so a
// transport failure is not mistaken for this.
func (p crashProbe) alive(ctx context.Context) bool {
	out, err := p.runner.RunArgs(ctx, "-s", p.serial, "shell", "pidof", shellquote.Arg(p.app))
	return err == nil && len(out) < 256 && pidListRE.MatchString(strings.TrimSpace(string(out)))
}

// collectCrash looks at the device after a lost connection and returns a report when the app died, nil when it did not or when nothing could be read. since is how
// long ago the test attempt began.
func collectCrash(ctx context.Context, p crashProbe, since time.Duration, poll time.Duration, seen *seenCrashes, appStopped bool) *CrashReport {
	window := since + crashWindowGrace
	if window > crashMaxWindow {
		window = crashMaxWindow
	}
	scr := hosted.NewScrubber() // one per collection: it counts and is not safe for two goroutines
	redact := func(s string) string { return scr.ScrubText(sysdialog.ScrubKnown(s)) }
	var gone *CrashReport
	for attempt := 1; attempt <= crashPolls; attempt++ {
		now, err := p.deviceNow(ctx)
		if err != nil {
			return nil
		}
		opts := device.CrashOptions{Redact: redact, DeviceNow: now}
		ev, err := device.CollectCrashEvidence(ctx, p.runner, p.serial, p.app, opts)
		if err != nil {
			return nil
		}
		alive := p.alive(ctx)
		dead, rec := judgeDeath(alive, seen.unseen(ev.Exits), now, window, appStopped)
		switch {
		case dead && rec != nil:
			seen.mark(*rec)
			if len(ev.Exits) > 0 && exitKey(*rec) != exitKey(ev.Exits[0]) { // the newest entry is not the death (a later force-stop, say): read the crash text of the process that died
				opts.PID = rec.PID
				if again, err := device.CollectCrashEvidence(ctx, p.runner, p.serial, p.app, opts); err == nil {
					ev = again
				}
			}
			ev.Summary = device.CleanOneLine(ev.Summary, 160)
			return &CrashReport{Gone: !alive, Kind: rec.Kind, Evidence: ev}
		case dead:
			gone = &CrashReport{Gone: true, Kind: device.ExitUnknown, Evidence: ev}
		default:
			return nil // the app has a process and no unexpected exit is recent: slow, not dead
		}
		if attempt < crashPolls {
			select {
			case <-time.After(poll):
			case <-ctx.Done():
				return gone
			}
		}
	}
	return gone
}

// collectCrashOnce looks at the device for the evidence of a dead app. Its contract, which other callers rely on and which must stay stable:
//   - Signature: a method of *Executor taking the step's context, returning nothing. The result is read with CrashReport().
//   - It is called when a step has failed with a lost connection (before the reconnect attempts) and when any other step has failed (after it failed, before it is
//     reported); a caller that ends a test attempt for another reason, such as a per-test timeout, may call it once at that moment.
//   - It does nothing unless the run asked for crash evidence (DeviceContext.CrashEvidence) on an Android device with a valid app id, a serial and an adb.
//   - It is once per test attempt in effect: it stops at the first report, and a slow app is looked at again at most crashMaxCollections times in all.
//   - It is read-only, bounded (crashTotalTimeout, and the caller's context ends it at once) and never changes the step's error or any later behaviour; a failure
//     to read the device leaves the report nil.
//   - The context must be live. Every adb call honours it, so a caller whose context has already expired (a per-test timeout, say) silently gets nothing and
//     uses up one of the looks: pass a fresh context with its own bound, derived from the run's context so that a cancelled run still stops it at once.
func (e *Executor) collectCrashOnce(ctx context.Context) {
	dc := e.deviceCtx
	if dc == nil || !dc.CrashEvidence || dc.Platform != device.PlatformAndroid || !device.ValidPackageName(dc.AppID) || e.crashReport != nil || e.crashTries >= crashMaxCollections {
		return
	}
	var runner device.AdbRunner = dc.CrashADB
	if runner == nil && dc.Manager != nil {
		runner = dc.Manager.ADB()
	}
	if runner == nil || strings.TrimSpace(dc.Serial) == "" {
		return
	}
	e.crashTries++
	poll := dc.CrashPoll
	if poll <= 0 {
		poll = crashPollEvery
	}
	cctx, cancel := context.WithTimeout(ctx, crashTotalTimeout)
	defer cancel()
	since := time.Duration(0)
	if !e.startedAt.IsZero() {
		since = time.Since(e.startedAt)
	}
	e.crashReport = collectCrash(cctx, crashProbe{runner: runner, serial: dc.Serial, app: dc.AppID}, since, poll, &dc.seenCrashes, e.appStopped)
}

// CrashReport is the evidence collected during this test attempt, or nil.
func (e *Executor) CrashReport() *CrashReport { return e.crashReport }

// failedTestCrash is what a finished test attempt does with the evidence: only a failed test carries it, and only a failed test prints its line. A test that
// passed after a lost connection (the reconnect worked) keeps nothing and says nothing.
func failedTestCrash(e *Executor, runErr error) *CrashReport {
	if runErr == nil {
		return nil
	}
	c := e.CrashReport()
	if c != nil {
		fmt.Printf("    \033[31m💥\033[0m  %s\n", c.Line())
	}
	return c
}

// noteAppState remembers whether the test has stopped the app on purpose: a kill step that succeeded does, a restart, clear or open step that succeeded launches it
// again. With no process and no exit entry, the crash look takes the first for the test's own doing and the second for a death.
func (e *Executor) noteAppState(step parser.Step, err error) {
	a, ok := step.(parser.ActionStep)
	if !ok || err != nil {
		return
	}
	switch a.Verb {
	case parser.VerbKill:
		e.appStopped = true
	case parser.VerbRestart, parser.VerbClearAppData, parser.VerbOpen:
		e.appStopped = false
	}
}
