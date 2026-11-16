package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/hosted"
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
	crashTotalTimeout   = 20 * time.Second       // for everything one collection does
	crashWindowGrace    = 10 * time.Second       // an exit entry may be this much older than the start of the test and still belong to it
	crashMaxWindow      = 6 * time.Hour
)

// CrashReport is what a failed test carries when its app died during it.
type CrashReport struct {
	// Gone is true when the app had no process at the time it was looked at.
	Gone bool
	// Kind says what the exit entry means (crash, native_crash, anr, ...); device.ExitUnknown when the process was gone but the device listed no entry for this test.
	Kind device.ExitKind
	// Evidence is the cleaned and masked evidence of the chosen process: the exit entry, the crash text and a one-line summary.
	Evidence device.CrashEvidence
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
	return "the app died during this test: " + what
}

// deathKinds are the exit reasons that mean the app ended without being asked to. A force-stop (user requested) and an unknown reason are not.
var deathKinds = map[device.ExitKind]bool{
	device.ExitSelf: true, device.ExitSignaled: true, device.ExitLowMemory: true, device.ExitCrash: true, device.ExitNativeCrash: true, device.ExitANR: true, device.ExitOther: true,
}

// judgeDeath decides dead or slow from what was seen. exits are newest first, deviceNow is the device's clock and window how long ago the test began, in the
// device's time. An entry of an unexpected ending that is not older than the window belongs to this test: the app died, even if it has been started again since
// (its pid changed). With no such entry, an app that has no process is gone, and an app that has one is only slow. Without the device's clock nothing can be said.
func judgeDeath(alive bool, exits []device.ExitRecord, deviceNow time.Time, window time.Duration) (dead bool, rec *device.ExitRecord) {
	if deviceNow.IsZero() {
		return false, nil
	}
	for i := range exits {
		if deathKinds[exits[i].Kind] && !exits[i].WallClock.Before(deviceNow.Add(-window)) {
			return true, &exits[i]
		}
	}
	return !alive, nil
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
func collectCrash(ctx context.Context, p crashProbe, since time.Duration, poll time.Duration) *CrashReport {
	window := since + crashWindowGrace
	if window < 30*time.Second {
		window = 30 * time.Second
	}
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
		dead, rec := judgeDeath(alive, ev.Exits, now, window)
		switch {
		case dead && rec != nil:
			if len(ev.Exits) > 0 && rec != &ev.Exits[0] { // the newest entry is not the death (a later force-stop, say): read the crash text of the process that died
				opts.PID = rec.PID
				if again, err := device.CollectCrashEvidence(ctx, p.runner, p.serial, p.app, opts); err == nil {
					ev = again
				}
			}
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

// collectCrashOnce is called when a step has failed with a lost connection, before the reconnect attempts. It does nothing unless the run asked for crash
// evidence on an Android device, collects at most crashMaxCollections times per test attempt, and stops at the first death found.
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
	e.crashReport = collectCrash(cctx, crashProbe{runner: runner, serial: dc.Serial, app: dc.AppID}, since, poll)
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
