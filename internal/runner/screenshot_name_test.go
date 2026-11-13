package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// shotRecorder records the name each screenshot call is made with.
type shotRecorder struct {
	fakeAIClient
	names []string
}

func (r *shotRecorder) Screenshot(ctx context.Context, name string) (string, error) {
	r.names = append(r.names, name)
	return "", nil
}

func shotExec(t *testing.T) (*Executor, *shotRecorder) {
	t.Helper()
	rec := &shotRecorder{}
	e := NewExecutor(rec, nil, nil, time.Second, false)
	old := shotNow
	shotNow = func() time.Time { return time.Date(2026, 11, 13, 14, 15, 2, 123_000_000, time.Local) }
	t.Cleanup(func() { shotNow = old })
	return e, rec
}

func takeShot(t *testing.T, e *Executor, name string) {
	t.Helper()
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbTakeShot, Name: name, Line: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestTakeScreenshotNameBuiltins(t *testing.T) {
	e, rec := shotExec(t)
	e.SetStepContext("f14.probe", "F14 alice/ios: form", 1)
	takeShot(t, e, "F14_s02_<timestamp>")
	takeShot(t, e, "<run>_<test>_s02")
	takeShot(t, e, "plain")
	takeShot(t, e, "<unknown>_<timestamp>_<timestamp>")
	want := []string{
		"F14_s02_20261113-141502-123",
		runStamp + "_F14_alice_ios__form_s02",
		"plain",
		"<unknown>_20261113-141502-123_20261113-141502-123",
	}
	for i, w := range want {
		if rec.names[i] != w {
			t.Errorf("name %d = %q, want %q", i, rec.names[i], w)
		}
	}
}

func TestTakeScreenshotNameSuccessiveStepsDiffer(t *testing.T) {
	e, rec := shotExec(t)
	n := 0
	shotNow = func() time.Time { n++; return time.Date(2026, 11, 13, 14, 15, 2, n*1_000_000, time.Local) }
	takeShot(t, e, "x_<timestamp>")
	takeShot(t, e, "x_<timestamp>")
	if rec.names[0] == rec.names[1] {
		t.Fatalf("two steps in the same second got the same name %q", rec.names[0])
	}
}

func TestTakeScreenshotNameResolvesVariables(t *testing.T) {
	e, rec := shotExec(t)
	e.vars = map[string]string{"device": "alice-ios"}
	t.Setenv("PROBE_TEST_SHOT_RUN", "r7")
	takeShot(t, e, "F14_<device>_${PROBE_TEST_SHOT_RUN}_<timestamp>")
	if got, want := rec.names[0], "F14_alice-ios_r7_20261113-141502-123"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	// a variable of the same name as a built-in wins: the script's own binding is never overridden
	e.vars = map[string]string{"run": "mine"}
	takeShot(t, e, "<run>")
	if rec.names[1] != "mine" {
		t.Fatalf("variable <run> = %q, want mine", rec.names[1])
	}
}

func TestTakeScreenshotNameWithoutTestName(t *testing.T) {
	e, rec := shotExec(t)
	takeShot(t, e, "a_<test>")
	if rec.names[0] != "a_test" {
		t.Fatalf("name = %q, want a_test", rec.names[0])
	}
}

func TestCompareScreenshotKeepsBuiltinsLiteral(t *testing.T) {
	e, rec := shotExec(t)
	e.vars = map[string]string{"device": "alice-ios"}
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbCompareShot, Name: "base_<device>_<timestamp>", Line: 1}); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.names[0], "base_alice-ios_<timestamp>"; got != want {
		t.Fatalf("compare name = %q, want %q (built-ins must not expand: the name is the baseline key)", got, want)
	}
}

func TestSafeShotPart(t *testing.T) {
	for in, want := range map[string]string{"": "test", "ok-1.2_x": "ok-1.2_x", "a b/c": "a_b_c", "é": "_", "../x": ".._x"} {
		if got := safeShotPart(in); got != want {
			t.Errorf("safeShotPart(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.Contains(safeShotPart("a/b\\c:d"), "/") {
		t.Error("path separator kept")
	}
}
