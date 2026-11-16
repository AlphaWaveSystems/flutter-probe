package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// testTimeoutGrace is the total budget of the cleanup after a test ran past its limit: the failure screenshot, the on-failure and
// after-each hooks, the video stop and the perf close run within it, and it holds even when one of those calls never returns. A
// variable so the tests can shorten it.
var testTimeoutGrace = 30 * time.Second

// ValidateTestTimeout checks defaults.test_timeout (or --test-timeout): zero turns the limit off, and a positive limit must be at
// least one second. A negative value is an error, not a silent off.
func ValidateTestTimeout(d time.Duration) error {
	if d < 0 {
		return fmt.Errorf("test_timeout cannot be negative (got %s); 0 turns the limit off", d)
	}
	if d > 0 && d < time.Second {
		return fmt.Errorf("test_timeout must be at least 1s (got %s); 0 turns the limit off", d)
	}
	return nil
}

// testTimeoutError is the failure of an attempt that ran past its limit. Error names the limit and the step the attempt was on,
// and keeps the text of the error the attempt ended with when that error was not the limit's own cancellation. cause is kept for
// Unwrap. restart is set when the restart that follows the timeout did not happen or failed.
type testTimeoutError struct {
	limit   time.Duration
	step    string // the step that was running when the limit ended the attempt, e.g. `line 3: tap "Save"`; empty when none
	cause   error
	restart error
}

func (e *testTimeoutError) Error() string {
	msg := fmt.Sprintf("test exceeded %s", formatLimit(e.limit))
	if e.step != "" {
		msg += fmt.Sprintf(" (%s)", e.step)
	}
	if e.cause != nil && !isCancellation(e.cause) {
		msg += ": " + e.cause.Error()
	}
	if e.restart != nil {
		msg += fmt.Sprintf(" (restart after the timeout failed: %v)", e.restart)
	}
	return msg
}

func (e *testTimeoutError) Unwrap() error { return e.cause }

// isCancellation reports whether err is a context ending (the limit's, or the run's). The limit's own failure is named by the
// timeout error; a cancellation is not worth repeating in its text.
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// stepText is the step a timeout names: its line and its description, as the step events give them.
func stepText(ev StepEvent) string {
	if ev.Description == "" {
		return ""
	}
	if ev.Line > 0 {
		return fmt.Sprintf("line %d: %s", ev.Line, ev.Description)
	}
	return ev.Description
}

// formatLimit writes a limit as the config writes it: 90s, not 1m30s.
func formatLimit(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s"
}

// attemptContext returns the context an attempt runs under: ctx itself without a limit, or ctx with the limit as its deadline.
func attemptContext(ctx context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	if limit > 0 {
		return context.WithTimeout(ctx, limit)
	}
	return ctx, func() {}
}

// cleanupGuard drops the writes of an attempt's reconnects once the attempt is released. A grace phase that was abandoned can
// still be running a reconnect when the next test starts; its client must not replace the runner's.
type cleanupGuard struct {
	mu       sync.Mutex
	released bool
}

func newCleanupGuard() *cleanupGuard { return &cleanupGuard{} }

// write runs fn unless the guard was released, and reports whether it ran. The check and the write hold the same lock, so a
// release never overlaps a write.
func (g *cleanupGuard) write(fn func()) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.released {
		return false
	}
	fn()
	return true
}

func (g *cleanupGuard) release() {
	g.mu.Lock()
	g.released = true
	g.mu.Unlock()
}

// testTimeoutCrashBudget bounds the crash-evidence look after an attempt ran past its limit. A variable so the tests can shorten it.
var testTimeoutCrashBudget = 10 * time.Second

// collectCrashAfterTimeout gives an attempt that ran past its limit its one look at the device's crash evidence. The context is fresh and
// bounded by testTimeoutCrashBudget, derived from the run's context, so a cancelled run still stops the look at once. The look is once per
// attempt (collectCrashOnce keeps that), and it runs before the cleanup starts: the body has returned, so no step of this executor is
// running while the look reads it. It does nothing unless the run asked for crash evidence.
func (e *Executor) collectCrashAfterTimeout(runCtx context.Context) {
	ctx, cancel := context.WithTimeout(runCtx, testTimeoutCrashBudget)
	defer cancel()
	e.collectCrashOnce(ctx)
}

// cleanupOut is where the failure screenshot's lines are printed. It is os.Stdout; a test replaces it to see what a cleanup that was
// abandoned still prints after the budget.
var cleanupOut io.Writer = os.Stdout

// finishAttempt runs the failure screenshot, the on-failure hooks and the after-each hooks of an attempt that ended on its own
// terms. It is the code the attempt always ran after its body; runErr says whether the attempt failed.
//
// client is the client the screenshot uses, read by the caller. guard, when set, decides whether the prints and the artifact
// writes still happen: a grace phase that was abandoned must not write to the output or to the attempt after the next test began.
func (r *Runner) finishAttempt(ctx context.Context, exec *Executor, prog *parser.Program, t parser.TestDef, runErr error, client probelink.ProbeClient, guard *cleanupGuard) {
	emit := func(f func()) {
		if guard == nil {
			f()
			return
		}
		guard.write(f)
	}

	// Auto-screenshot on failure
	if runErr != nil && client != nil {
		shotCtx, shotCancel := context.WithTimeout(ctx, 10*time.Second)
		shotName := fmt.Sprintf("failure_%s", sanitizeName(t.Name))
		path, shotErr := client.Screenshot(shotCtx, shotName)
		shotCancel()
		if shotErr != nil {
			emit(func() { fmt.Fprintf(cleanupOut, "    \033[33m⚠\033[0m  failure screenshot: %v\n", shotErr) })
		} else if path != "" {
			emit(func() {
				exec.AddArtifact(path)
				fmt.Fprintf(cleanupOut, "    \033[36m📸\033[0m  failure screenshot saved: %s\n", path)
			})
		}
	}

	// Run on-failure hooks. On a grace phase (guard set) no hook starts once the budget is spent: its step would print after the
	// phase was abandoned.
	if runErr != nil {
		for _, hook := range prog.Hooks {
			if hook.Kind == parser.HookOnFailure {
				if guard != nil && ctx.Err() != nil {
					break
				}
				_ = exec.RunBody(ctx, hook.Body) // best-effort
			}
		}
	}

	// Run after-each hooks (always)
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookAfterEach {
			if guard != nil && ctx.Err() != nil {
				break
			}
			_ = exec.RunBody(ctx, hook.Body) // best-effort
		}
	}
}

// graceOutcome is what the cleanup after a timed-out attempt left behind.
type graceOutcome struct {
	abandoned bool     // the grace budget ran out before the cleanup returned: nothing the cleanup wrote may be read
	artifacts []string // the attempt's artifacts: all of them when the cleanup finished, else a snapshot taken before it
	video     string   // the path the video stop returned, when the cleanup finished
	videoErr  error
	crash     *CrashReport // the crash evidence the look found before the cleanup: kept even when the cleanup is abandoned
}

// finishTimedOutAttempt is the cleanup after an attempt that ran past its limit. Its order is the order the app needs:
//
//  1. crash evidence (a seam until S2b), while the app is still the one that ran the test;
//  2. the failure screenshot, then the on-failure hooks and the after-each hooks, which may still read the app's screen and state;
//  3. the video stop and the perf close, which end the recording and the measurement of this attempt;
//  4. the restart, last, because it relaunches the app and clears what the steps above read.
//
// Steps 1 to 3 run on their own goroutine under the grace budget, so a call that never returns is given up at the budget and the
// attempt goes on. The writes they make go through guard, which is released when the attempt ends. A grace phase that was abandoned
// reports its artifacts as they were before it began; its video and perf window are dropped, and nothing it wrote is read after.
//
// The restart runs on ctx, the run's context, not on the attempt's, which the limit has expired. The restart step is bounded by
// agent.launch_timeout. It is skipped when the run itself was cancelled.
func (r *Runner) finishTimedOutAttempt(ctx context.Context, exec *Executor, guard *cleanupGuard, prog *parser.Program, t parser.TestDef, te *testTimeoutError, stopVideo func(context.Context) (string, error)) graceOutcome {
	// The crash look comes first, while the app is still the one that failed. The body has returned, so no step of this executor is running.
	exec.collectCrashAfterTimeout(ctx)
	crash := exec.CrashReport()
	before := exec.Artifacts()
	client := r.client // read here, on the runner's goroutine: the grace goroutine may outlive this call
	graceCtx, cancelGrace := context.WithTimeout(ctx, testTimeoutGrace)
	var video string
	var videoErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.finishAttempt(graceCtx, exec, prog, t, te, client, guard)
		video, videoErr = stopVideo(graceCtx)
		exec.closePerf(graceCtx)
	}()
	abandoned := false
	select {
	case <-done:
	case <-graceCtx.Done():
		abandoned = true
		guard.release()
	}
	cancelGrace()

	// The restart happens whether or not the cleanup finished: the next test needs the app relaunched either way.
	r.restartAfterTimeout(ctx, te)
	guard.release()
	if abandoned {
		return graceOutcome{abandoned: true, artifacts: before, crash: crash}
	}
	return graceOutcome{artifacts: exec.Artifacts(), video: video, videoErr: videoErr, crash: crash}
}

// restartAfterTimeout relaunches the app so that the next test starts clean. A failure, or a restart that could not happen because
// the run was cancelled, is recorded on te: the run goes on with the next test either way.
func (r *Runner) restartAfterTimeout(ctx context.Context, te *testTimeoutError) {
	if ctx.Err() != nil {
		te.restart = errors.New("the run was cancelled, so the app was not restarted")
		return
	}
	if r.restartHook != nil {
		te.restart = r.restartHook(ctx)
		return
	}
	restartGuard := newCleanupGuard()
	restart := r.newAttemptExecutor(restartGuard)
	if err := restart.RunStep(ctx, parser.ActionStep{Verb: parser.VerbRestart}); err != nil {
		te.restart = err
	}
	restartGuard.release()
}
