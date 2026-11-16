package runner

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// testTimeoutGrace is the total budget of the grace phase after a test ran past its limit: the failure screenshot, the on-failure
// hooks and the after-each hooks run within it, and it holds even when a call never returns. A variable so the tests can shorten it.
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

// testTimeoutError is the failure of an attempt that ran past its limit. Error names the limit; cause is the step error the
// cancellation produced, kept for Unwrap; restart is set when the restart that follows the timeout failed.
type testTimeoutError struct {
	limit   time.Duration
	cause   error
	restart error
}

func (e *testTimeoutError) Error() string {
	msg := fmt.Sprintf("test exceeded %s", formatLimit(e.limit))
	if e.restart != nil {
		msg += fmt.Sprintf(" (restart after the timeout failed: %v)", e.restart)
	}
	return msg
}

func (e *testTimeoutError) Unwrap() error { return e.cause }

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

// collectCrashAfterTimeout is the seam for crash evidence after a test that ran past its limit. It does nothing yet. probe-admin's
// S2b puts the evidence collector on this path (collectCrashOnce, which needs a live, fresh context), and that call replaces the
// body of this function.
func (e *Executor) collectCrashAfterTimeout(ctx context.Context) {}

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
			emit(func() { fmt.Printf("    \033[33m⚠\033[0m  failure screenshot: %v\n", shotErr) })
		} else if path != "" {
			emit(func() {
				exec.AddArtifact(path)
				fmt.Printf("    \033[36m📸\033[0m  failure screenshot saved: %s\n", path)
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

// finishTimedOutAttempt is the cleanup after an attempt that ran past its limit. The run is not cancelled, so the failure
// screenshot and the hooks run on a context with the grace budget as their deadline. They run on their own goroutine: if a call
// never returns, the grace phase is abandoned at the budget, and the goroutine can still write only through exec and the guard,
// which are not read or released for it. Then the app is restarted through the existing restart step, on a fresh executor.
// It reports the artifacts the attempt has, and whether the grace phase was abandoned.
func (r *Runner) finishTimedOutAttempt(ctx context.Context, exec *Executor, guard *cleanupGuard, prog *parser.Program, t parser.TestDef, te *testTimeoutError) ([]string, bool) {
	before := exec.Artifacts()
	client := r.client // read here, on the runner's goroutine: the grace goroutine may outlive this call
	graceCtx, cancelGrace := context.WithTimeout(context.WithoutCancel(ctx), testTimeoutGrace)
	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.collectCrashAfterTimeout(graceCtx)
		r.finishAttempt(graceCtx, exec, prog, t, te, client, guard)
	}()
	abandoned := false
	select {
	case <-done:
	case <-graceCtx.Done():
		abandoned = true
		guard.release()
	}
	cancelGrace()

	artifacts := before
	if !abandoned {
		artifacts = exec.Artifacts()
	}

	restartGuard := newCleanupGuard()
	restart := r.newAttemptExecutor(restartGuard)
	if err := restart.RunStep(context.WithoutCancel(ctx), parser.ActionStep{Verb: parser.VerbRestart}); err != nil {
		te.restart = err
	}
	restartGuard.release()
	guard.release()
	return artifacts, abandoned
}
