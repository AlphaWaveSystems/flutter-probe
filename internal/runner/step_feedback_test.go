package runner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---- isTTY ----

func TestIsTTY_FalseInTestProcess(t *testing.T) {
	// go test always runs with stdout connected to a pipe, not a TTY.
	if isTTY() {
		t.Error("isTTY() = true inside go test; expected false (stdout is a pipe)")
	}
}

// ---- printStepBeforeW ----

func TestPrintStepBeforeW_EmitsArrow(t *testing.T) {
	var buf bytes.Buffer
	printStepBeforeW(&buf, false, 0, `tap "Login"`)
	got := buf.String()
	if !strings.Contains(got, "→") {
		t.Errorf("expected → in output, got: %q", got)
	}
	if !strings.Contains(got, `tap "Login"`) {
		t.Errorf("expected description in output, got: %q", got)
	}
}

func TestPrintStepBeforeW_EmptyDesc_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	printStepBeforeW(&buf, false, 0, "")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty desc, got: %q", buf.String())
	}
}

func TestPrintStepBeforeW_IndentsWithDepth(t *testing.T) {
	var buf bytes.Buffer
	printStepBeforeW(&buf, false, 2, "tap")
	if !strings.Contains(buf.String(), "    ") { // 4 spaces base + 2*2 indent
		t.Errorf("expected indentation for depth=2, got: %q", buf.String())
	}
}

// ---- printStepAfterNW ----

func TestPrintStepAfterNW_SuccessIcon(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, false, 0, "tap", 100*time.Millisecond, nil, 0)
	got := buf.String()
	if !strings.Contains(got, "✓") {
		t.Errorf("expected ✓ for success, got: %q", got)
	}
}

func TestPrintStepAfterNW_FailureIcon(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, false, 0, "see", time.Second, errors.New("not found"), 0)
	got := buf.String()
	if !strings.Contains(got, "✗") {
		t.Errorf("expected ✗ for failure, got: %q", got)
	}
}

func TestPrintStepAfterNW_TTY_NoTicks_OverwritesLine(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, true, 0, "tap", 100*time.Millisecond, nil, 0)
	got := buf.String()
	// Must contain the cursor-up + carriage-return + erase sequence.
	if !strings.Contains(got, "\033[1A\r\033[K") {
		t.Errorf("expected ANSI cursor-up sequence on TTY with no ticks, got: %q", got)
	}
}

func TestPrintStepAfterNW_TTY_WithTicks_Appends(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, true, 0, "tap", time.Second, nil, 3)
	got := buf.String()
	// When tick lines were printed, must NOT overwrite (no cursor-up).
	if strings.Contains(got, "\033[1A") {
		t.Errorf("should not use cursor-up when tickLines > 0, got: %q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("expected ✓ in appended output, got: %q", got)
	}
}

func TestPrintStepAfterNW_NonTTY_Appends(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, false, 0, "tap", 100*time.Millisecond, nil, 0)
	got := buf.String()
	if strings.Contains(got, "\033[1A") {
		t.Errorf("non-TTY must not use cursor-up, got: %q", got)
	}
}

func TestPrintStepAfterNW_EmptyDesc_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	printStepAfterNW(&buf, false, 0, "", time.Second, nil, 0)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty desc, got: %q", buf.String())
	}
}

// ---- printStepTickW ----

func TestPrintStepTickW_ContainsTimer(t *testing.T) {
	var buf bytes.Buffer
	printStepTickW(&buf, 0, `wait until "X" appears`, 7*time.Second)
	got := buf.String()
	if !strings.Contains(got, "7s") && !strings.Contains(got, "(7)") {
		t.Errorf("expected elapsed seconds in tick output, got: %q", got)
	}
	if !strings.Contains(got, "⏱") {
		t.Errorf("expected ⏱ in tick output, got: %q", got)
	}
}

// ---- printStepWarningW ----

func TestPrintStepWarningW_ContainsWarningSymbol(t *testing.T) {
	var buf bytes.Buffer
	printStepWarningW(&buf, 0, "see", 24*time.Second, 30*time.Second)
	got := buf.String()
	if !strings.Contains(got, "⚠") {
		t.Errorf("expected ⚠ in warning output, got: %q", got)
	}
	if !strings.Contains(got, "24s") && !strings.Contains(got, "24") {
		t.Errorf("expected elapsed seconds in warning, got: %q", got)
	}
	if !strings.Contains(got, "30s") && !strings.Contains(got, "30") {
		t.Errorf("expected timeout seconds in warning, got: %q", got)
	}
}

// ---- printCurrentStepW ----

func TestPrintCurrentStepW_TTY_UsesCarriageReturn(t *testing.T) {
	var buf bytes.Buffer
	printCurrentStepW(&buf, true, "tap")
	got := buf.String()
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("expected \\r prefix on TTY, got: %q", got)
	}
}

func TestPrintCurrentStepW_NonTTY_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	printCurrentStepW(&buf, false, "tap")
	if buf.Len() != 0 {
		t.Errorf("expected no output on non-TTY, got: %q", buf.String())
	}
}

func TestPrintCurrentStepW_EmptyDesc_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	printCurrentStepW(&buf, true, "")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty desc, got: %q", buf.String())
	}
}

// ---- clearCurrentStepW ----

func TestClearCurrentStepW_TTY_WritesSpaces(t *testing.T) {
	var buf bytes.Buffer
	clearCurrentStepW(&buf, true)
	got := buf.String()
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("expected \\r prefix, got: %q", got)
	}
	// Should contain spaces to overwrite previous content.
	if !strings.Contains(got, " ") {
		t.Errorf("expected spaces to clear line, got: %q", got)
	}
}

func TestClearCurrentStepW_NonTTY_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	clearCurrentStepW(&buf, false)
	if buf.Len() != 0 {
		t.Errorf("expected no output on non-TTY, got: %q", buf.String())
	}
}

// ---- runStepTicker ----

func TestRunStepTicker_NoOutputBeforeFirstTick(t *testing.T) {
	// Close stopTicker immediately — ticker goroutine should exit before
	// the first tick fires and produce no output.
	ctx := context.Background()
	var extraLines atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go runStepTicker(ctx, "tap", 0, 1*time.Hour, stop, done, &extraLines)
	close(stop)
	<-done

	if extraLines.Load() != 0 {
		t.Errorf("expected 0 extra lines for fast step, got %d", extraLines.Load())
	}
}

func TestRunStepTicker_PrintsTickAfterInterval(t *testing.T) {
	// Use a 1ms tick interval so the test runs quickly.
	// Wait for at least one tick before stopping.
	ctx := context.Background()
	var extraLines atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go runStepTicker(ctx, "slow step", 0, 1*time.Millisecond, stop, done, &extraLines)

	// Wait for at least one tick to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if extraLines.Load() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-done

	if extraLines.Load() == 0 {
		t.Error("expected at least one tick line to be emitted")
	}
}

func TestRunStepTicker_WarningAt80Percent(t *testing.T) {
	// Context deadline = 50ms total. 80% threshold = 40ms.
	// Tick interval = 10ms — the warning should fire around the 40ms tick.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var extraLines atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go runStepTicker(ctx, "see", 0, 10*time.Millisecond, stop, done, &extraLines)

	// Wait for context to expire (ticker will exit via <-ctx.Done()).
	<-ctx.Done()
	close(stop)
	<-done

	// We expect at least: a few ⏱ ticks + 1 ⚠ warning line.
	if extraLines.Load() < 2 {
		t.Errorf("expected ⏱ ticks + ⚠ warning, got %d extra lines", extraLines.Load())
	}
}

func TestRunStepTicker_StopsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var extraLines atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go runStepTicker(ctx, "wait", 0, 1*time.Hour, stop, done, &extraLines)

	cancel() // cancel context — goroutine must exit via <-ctx.Done()
	select {
	case <-done:
		// Good — goroutine exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("runStepTicker did not exit after ctx cancel within 2s")
	}
	close(stop) // safe to close after done is closed
}

func TestRunStepTicker_WarningPrintedOnlyOnce(t *testing.T) {
	// Short deadline ensures multiple ticks cross the 80% threshold.
	// The warning should fire exactly once regardless.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	var extraLines atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	// 5ms ticks, 30ms deadline: ticks at ~5,10,15,20,25ms
	// 80% of 30ms = 24ms — warning fires at the 25ms tick, not again after.
	go runStepTicker(ctx, "see", 0, 5*time.Millisecond, stop, done, &extraLines)

	<-ctx.Done()
	close(stop)
	<-done

	// Count: say 5 ticks + 1 warning = 6. If warning fires twice, it'd be 7+.
	// We just verify it's bounded (not doubling): max reasonable = ticks + 1.
	ticks := int(extraLines.Load())
	// With 30ms deadline and 5ms ticks we get at most ~6 ticks, warning ≤ 1.
	// Anything ≤ 10 is sane; > 10 suggests a bug (warning loop).
	if ticks > 10 {
		t.Errorf("too many extra lines (%d) — warning may have fired more than once", ticks)
	}
}
