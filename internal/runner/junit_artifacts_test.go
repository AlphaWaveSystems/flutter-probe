package runner_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func artifactResults(arts ...string) []runner.TestResult {
	return []runner.TestResult{{TestName: "shot", File: "tests/a.probe", Passed: true, Row: -1, Duration: time.Second, Artifacts: arts}}
}

func junitWith(t *testing.T, o runner.JUnitOptions, results []runner.TestResult) string {
	t.Helper()
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJUnit, &buf, false)
	o.Base = "/work"
	rep.SetJUnitOptions(o)
	if err := rep.Report(results); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

type junitOut struct {
	Suites []struct {
		Cases []struct {
			Name      string `xml:"name,attr"`
			SystemOut string `xml:"system-out"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

func parseJUnit(t *testing.T, x string) junitOut {
	t.Helper()
	var d junitOut
	if err := xml.Unmarshal([]byte(x), &d); err != nil {
		t.Fatalf("not well-formed XML: %v\n%s", err, x)
	}
	return d
}

func linesOf(t *testing.T, x string) []string {
	d := parseJUnit(t, x)
	if len(d.Suites) == 0 || len(d.Suites[0].Cases) == 0 {
		t.Fatal("no case")
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(d.Suites[0].Cases[0].SystemOut), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// With the option off, a test that has artifacts gives the same bytes as one that has none; with it on and no artifacts, no element is written.
func TestJUnitWithoutTheOptionOrWithoutArtifactsIsByteIdentical(t *testing.T) {
	plain := junitWith(t, runner.JUnitOptions{}, artifactResults())
	if got := junitWith(t, runner.JUnitOptions{}, artifactResults("/work/reports/a.png", "/work/reports/b.png")); got != plain {
		t.Errorf("artifacts changed the output with the option off:\n%s", got)
	}
	if got := junitWith(t, runner.JUnitOptions{Artifacts: true}, artifactResults()); got != plain || strings.Contains(got, "system-out") {
		t.Errorf("an empty artifact list wrote something:\n%s", got)
	}
	if got := junitWith(t, runner.JUnitOptions{Artifacts: true}, artifactResults("", "  ")); strings.Contains(got, "system-out") {
		t.Errorf("blank artifact names wrote something:\n%s", got)
	}
}

func TestJUnitArtifactPathsAreRelativeToTheReportDirectoryAndNeverAbsolute(t *testing.T) {
	o := runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}
	got := linesOf(t, junitWith(t, o, artifactResults(
		"/work/reports/screenshots/home.png",        // inside: relative
		"/work/reports/visual-diff/home_diff_1.png", // inside: relative
		"/Users/someone/Library/Caches/shot.png",    // outside: the file name only
		"/work/other/x.png",                         // a sibling directory: the file name only
		"../../escape/up.png",                       // climbs out: the file name only
		"rel/dir/ok.png",                            // already relative: kept
		`C:\Users\someone\AppData\shot.png`,         // a Windows path from another machine
		`\\server\share\shot.png`,
		"/work/reports/screenshots/home.png", // twice: once
	)))
	want := []string{
		"artifact: screenshots/home.png",
		"artifact: visual-diff/home_diff_1.png",
		"artifact: shot.png",
		"artifact: x.png",
		"artifact: up.png",
		"artifact: rel/dir/ok.png",
		"artifact: shot.png",
	}
	// the repeated "shot.png" entries (two different absolute paths with one name) are listed once
	want = []string{want[0], want[1], want[2], want[3], want[4], want[5]}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, l := range got {
		if strings.Contains(l, "/Users/") || strings.Contains(l, `\`) || strings.Contains(l, "someone") {
			t.Errorf("a path of another machine was written: %s", l)
		}
	}
}

func TestJUnitArtifactBaseDefaultsToTheJUnitBase(t *testing.T) {
	got := linesOf(t, junitWith(t, runner.JUnitOptions{Artifacts: true}, artifactResults("/work/reports/a.png", "/elsewhere/b.png")))
	if len(got) != 2 || got[0] != "artifact: reports/a.png" || got[1] != "artifact: b.png" {
		t.Fatalf("got %v", got)
	}
}

// Artifact names come from test and screenshot names, so they can hold anything: the file stays well-formed XML, control characters become spaces,
// markup stays text, and a secret the run knows is masked.
func TestJUnitArtifactNamesAreMaskedAndSafeToWrite(t *testing.T) {
	sysdialog.NoteSecret("artifact-secret-Hn37")
	hostile := []string{
		"/work/reports/a]]>b.png", "/work/reports/<img src=x onerror=alert(1)>.png", "/work/reports/new\nline.png", "/work/reports/nul\x00bell\x07.png",
		"/work/reports/sep\u2028x.png", "/work/reports/artifact-secret-Hn37.png", "/work/reports/bad\xff\xfeutf8.png", "/work/reports/</system-out><x/>.png",
		"/work/reports/nonchars\uFFFE\uFFFF.png",
	}
	x := junitWith(t, runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}, artifactResults(hostile...))
	got := linesOf(t, x)
	if len(got) != len(hostile) {
		t.Fatalf("%d lines for %d artifacts:\n%s", len(got), len(hostile), strings.Join(got, "\n"))
	}
	for _, l := range got {
		for _, bad := range []string{"\x00", "\x07", "\u2028", "\uFFFE", "\uFFFF", "artifact-secret-Hn37"} { // markup characters are fine in the decoded text; the raw file is checked below
			if strings.Contains(l, bad) {
				t.Errorf("%q still in %q", bad, l)
			}
		}
	}
	if strings.Contains(x, "<img") || strings.Contains(x, "</system-out><x/>") {
		t.Errorf("markup in an artifact name was written as markup")
	}
	if !strings.Contains(strings.Join(got, "\n"), "****") {
		t.Errorf("the known secret was not masked")
	}
}

func TestJUnitArtifactCountAndLengthAreCapped(t *testing.T) {
	var many []string
	for i := 0; i < 60; i++ {
		many = append(many, fmt.Sprintf("/work/reports/s%02d.png", i))
	}
	got := linesOf(t, junitWith(t, runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}, artifactResults(many...)))
	if len(got) != 51 || got[49] != "artifact: s49.png" || got[50] != "artifacts: 10 more not listed" {
		t.Fatalf("%d lines, last two: %q %q", len(got), got[len(got)-2], got[len(got)-1])
	}
	exact := linesOf(t, junitWith(t, runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}, artifactResults(many[:50]...)))
	if len(exact) != 50 || strings.Contains(strings.Join(exact, "\n"), "more not listed") {
		t.Errorf("50 artifacts: %d lines", len(exact))
	}
	long := "/work/reports/" + strings.Repeat("a", 400) + ".png"
	l := linesOf(t, junitWith(t, runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}, artifactResults(long)))
	if len([]rune(strings.TrimPrefix(l[0], "artifact: "))) != 300 {
		t.Errorf("a long path is %d characters", len([]rune(strings.TrimPrefix(l[0], "artifact: "))))
	}
	exact300 := "/work/reports/" + strings.Repeat("b", 296) + ".png"
	l = linesOf(t, junitWith(t, runner.JUnitOptions{Artifacts: true, ArtifactBase: "/work/reports"}, artifactResults(exact300)))
	if strings.TrimPrefix(l[0], "artifact: ") != strings.Repeat("b", 296)+".png" {
		t.Errorf("a path of exactly 300 characters was changed")
	}
}

// With --junit-attempts the artifacts of all attempts are listed on the case that carries the final outcome, once.
func TestJUnitArtifactsWithAttemptsGoOnTheFinalCase(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	res := []runner.TestResult{{TestName: "flaky", File: "tests/a.probe", Passed: true, Row: -1, Duration: time.Second, Attempts: 2, StartedAt: t0,
		Artifacts:  []string{"/work/reports/try1.png", "/work/reports/try2.png"},
		AttemptLog: []runner.AttemptOutcome{{Passed: false, Error: fmt.Errorf("first failed"), Duration: time.Second, StartedAt: t0}, {Passed: true, Duration: time.Second, StartedAt: t0.Add(2 * time.Second)}}}}
	d := parseJUnit(t, junitWith(t, runner.JUnitOptions{Artifacts: true, Attempts: true, ArtifactBase: "/work/reports"}, res))
	cases := d.Suites[0].Cases
	if len(cases) != 2 || strings.TrimSpace(cases[0].SystemOut) != "" || !strings.Contains(cases[1].SystemOut, "artifact: try1.png") || !strings.Contains(cases[1].SystemOut, "artifact: try2.png") {
		t.Fatalf("cases: %+v", cases)
	}
}
