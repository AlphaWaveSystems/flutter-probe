package runner

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	for in, want := range map[string]string{
		"": "test", "ok-1.2_x": "ok-1.2_x", "a b/c": "a_b_c", "../x": ".._x",
		"Prüfung Größe": "Prüfung_Größe", "支付流程": "支付流程", "Привет мир": "Привет_мир", "قائمة": "قائمة", "नमस्ते": "नमस्ते", // letters and marks of other scripts stay
		"e\u0301": "e\u0301", "٣٤": "٣٤", // a combining mark and non-ASCII digits stay
		"pay 💳 ok": "pay___ok", "a\u200bb": "a_b", "a\u202eb": "a_b", "a\tb\nc": "a_b_c", "a:b?c*d": "a_b_c_d", // emoji, invisible and control characters, punctuation do not
		"bad\xffbyte": "bad_byte", "\xc3": "_", "\ufffd": "_", // invalid UTF-8 and the replacement character
	} {
		if got := safeShotPart(in); got != want {
			t.Errorf("safeShotPart(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.ContainsAny(safeShotPart("a/b\\c:d"), "/\\") {
		t.Error("path separator kept")
	}
}

func TestScreenshotNamesCarryUTF8Unchanged(t *testing.T) {
	e, rec := shotExec(t)
	e.SetStepContext("f.probe", "Zahlung Größe 支付", 1)
	e.vars = map[string]string{"gerät": "Käse-Ü"}
	takeShot(t, e, "Übersicht_<gerät>_支付_🙂_<test>")
	if got, want := rec.names[0], "Übersicht_Käse-Ü_支付_🙂_Zahlung_Größe_支付"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if !utf8.ValidString(rec.names[0]) {
		t.Fatal("name is not valid UTF-8")
	}
	// names with no placeholder pass through byte for byte, including a decomposed é (not normalised)
	takeShot(t, e, "caf\u00e9 cafe\u0301")
	if rec.names[1] != "caf\u00e9 cafe\u0301" {
		t.Fatalf("name changed: %q", rec.names[1])
	}
}

func TestCleanShotName(t *testing.T) {
	long := strings.Repeat("é", 100) // 200 bytes
	cases := []struct{ name, in, want string }{
		{"plain is unchanged", "Login page 2.png-ok_v1", "Login page 2.png-ok_v1"},
		{"UTF-8 is unchanged", "Prüfung Größe 支付 ✓", "Prüfung Größe 支付 ✓"},
		{"forward slash", "a/b", "a_b"},
		{"backslash", `a\b`, "a_b"},
		{"parent directory", "a/../../x", "a___x"},
		{"absolute path", "/etc/cron.d/x", "_etc_cron.d_x"},
		{"dots only", "....", "screenshot"},
		{"dots rebuilt after a removal", "...", "."},
		{"dots split by an invisible character", "a.​.b", "ab"},
		{"NUL", "a\x00b", "ab"},
		{"control characters", "a\tb\nc\x7fd", "abcd"},
		{"bidi override", "evil‮gnp.exe", "evilgnp.exe"},
		{"zero width joiner", "a‍b", "ab"},
		{"line separator", "a b c", "abc"},
		{"invalid UTF-8", "a\xffb", "a_b"},
		{"nothing left", "‮\x00", "screenshot"},
		{"empty", "", "screenshot"},
		{"cut at a whole character", long, strings.Repeat("é", 60)},
		{"cut inside a character backs up", "a" + long, "a" + strings.Repeat("é", 59)},
		{"ASCII cut", strings.Repeat("a", 300), strings.Repeat("a", 120)},
	}
	for _, c := range cases {
		got := cleanShotName(c.in)
		if got != c.want {
			t.Errorf("%s: cleanShotName(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
		if !utf8.ValidString(got) || len(got) > maxShotName {
			t.Errorf("%s: result %q is not valid UTF-8 within %d bytes", c.name, got, maxShotName)
		}
	}
}

func TestScreenshotNameFromValuesIsClean(t *testing.T) {
	e, rec := shotExec(t)
	e.SetStepContext("f.probe", "t", 1)
	e.vars = map[string]string{
		"v":    "../../x",
		"abs":  "/etc/cron.d/x",
		"bs":   `a\b`,
		"nul":  "a\x00b",
		"bidi": "n‮me",
		"self": "<test>",
		"half": "<tim",
	}
	t.Setenv("SHOT_ENV_PATH", "../../env")
	takeShot(t, e, "a/<v>")
	takeShot(t, e, "<abs>")
	takeShot(t, e, "<bs>")
	takeShot(t, e, "<nul>")
	takeShot(t, e, "<bidi>")
	takeShot(t, e, "${SHOT_ENV_PATH}")
	want := []string{"a___x", "_etc_cron.d_x", "a_b", "ab", "nme", "__env"}
	for i, w := range want {
		if rec.names[i] != w {
			t.Errorf("name %d = %q, want %q", i, rec.names[i], w)
		}
	}
}

func TestScreenshotVariableValueIsNotExpandedAsBuiltin(t *testing.T) {
	e, rec := shotExec(t)
	e.SetStepContext("f.probe", "My test", 1)
	e.vars = map[string]string{"self": "<test>", "half": "<tim"}
	takeShot(t, e, "<self>")
	takeShot(t, e, "<half>estamp>")
	takeShot(t, e, "<half><test>estamp>")
	// the value stays text: "<test>" is not replaced, and the pieces do not join into <timestamp>
	want := []string{"<test>", "<timestamp>", "<tim" + "My_test" + "estamp>"}
	for i, w := range want {
		if rec.names[i] != w {
			t.Errorf("name %d = %q, want %q", i, rec.names[i], w)
		}
	}
}

func TestCompareScreenshotNameIsClean(t *testing.T) {
	e, _ := shotExec(t)
	e.vars = map[string]string{"v": "../../x"}
	if got := e.shotName("a/<v>", false); got != "a___x" {
		t.Errorf("compare name = %q, want a___x", got)
	}
}

func TestTwoStepsInTheSameMillisecondShareATimestamp(t *testing.T) {
	// documented behaviour: <timestamp> has millisecond resolution; the app still keeps both files apart with its own stamp
	e, rec := shotExec(t)
	takeShot(t, e, "s_<timestamp>")
	takeShot(t, e, "s_<timestamp>")
	if rec.names[0] != rec.names[1] {
		t.Errorf("names differ: %q vs %q", rec.names[0], rec.names[1])
	}
}
