package runner_test

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files")

func junitFixture() ([]runner.TestResult, runner.RunMetadata) {
	t0 := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	flakyAttempts := []runner.AttemptOutcome{
		{Passed: false, Error: fmt.Errorf("expected to see \"Pay\""), Duration: 1500 * time.Millisecond, StartedAt: t0.Add(2 * time.Second)},
		{Passed: true, Duration: 900 * time.Millisecond, StartedAt: t0.Add(4 * time.Second)},
	}
	results := []runner.TestResult{
		// Absolute path inside the project, a data-driven row, a retry that finally passed.
		{TestName: "checkout [row 2]", File: "/work/tests/b.probe", Passed: true, Row: 1, Duration: 900 * time.Millisecond, Attempts: 2, StartedAt: t0.Add(4 * time.Second), AttemptLog: flakyAttempts, DeviceName: "Pixel 7"},
		{TestName: "login", File: "tests/a.probe", Passed: true, Row: -1, Duration: 1200 * time.Millisecond, Attempts: 1, StartedAt: t0,
			AttemptLog: []runner.AttemptOutcome{{Passed: true, Duration: 1200 * time.Millisecond, StartedAt: t0}}},
		{TestName: "assertion fails", File: "tests/a.probe", Passed: false, Row: -1, Duration: 2 * time.Second, Error: fmt.Errorf("expected \"Done\" to appear"), StartedAt: t0.Add(time.Second)},
		{TestName: "agent died", File: "tests/a.probe", Passed: false, Row: -1, Duration: 500 * time.Millisecond, Error: fmt.Errorf("probelink: write: use of closed network connection"), StartedAt: t0.Add(3 * time.Second)},
		{TestName: "later", File: "tests/a.probe", Skipped: true, Row: -1},
		// Outside the project directory: only the file name is kept (no home directory in a report).
		{TestName: "elsewhere", File: "/Users/jane/other/c.probe", Passed: true, Row: -1, Duration: time.Second, StartedAt: t0.Add(5 * time.Second)},
		{TestName: "no error recorded", File: "tests/a.probe", Passed: false, Row: -1},
	}
	meta := runner.RunMetadata{DeviceName: "Pixel 7", Platform: "android", OSVersion: "Android 15", AppID: "dev.example.app", AppVersion: "1.2.3", ProbeVersion: "0.25.0", Provider: "local", DeviceClass: "emulator"}
	return results, meta
}

func renderJUnit(t *testing.T, o runner.JUnitOptions) []byte {
	t.Helper()
	results, meta := junitFixture()
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJUnit, &buf, false)
	rep.SetMetadata(meta)
	o.Base = "/work"
	rep.SetJUnitOptions(o)
	if err := rep.Report(results); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "junit", name)
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs (run go test ./internal/runner -run JUnit -update to refresh):\n--- got\n%s", name, got)
	}
}

func TestJUnitDefaultOutputGolden(t *testing.T) {
	got := renderJUnit(t, runner.JUnitOptions{})
	golden(t, "default.golden.xml", got)
	// A retry that finally passed is still ONE case by default; the retry shows only as a property.
	if n := strings.Count(string(got), `name="checkout [row 2]"`); n != 1 {
		t.Errorf("default output must keep one case per test, got %d", n)
	}
}

func TestJUnitAttemptsOutputGolden(t *testing.T) {
	got := renderJUnit(t, runner.JUnitOptions{Attempts: true, ClassSuffix: "-android", Label: "shard-1"})
	golden(t, "attempts.golden.xml", got)
	if n := strings.Count(string(got), `name="checkout [row 2]"`); n != 2 {
		t.Errorf("--junit-attempts writes one case per attempt, got %d", n)
	}
}

// The structure every consumer relies on, checked on the parsed XML rather than the text.
func TestJUnitStructure(t *testing.T) {
	type prop struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type msg struct {
		Message string `xml:"message,attr"`
	}
	var doc struct {
		Tests    int    `xml:"tests,attr"`
		Failures int    `xml:"failures,attr"`
		Errors   int    `xml:"errors,attr"`
		Skipped  int    `xml:"skipped,attr"`
		Time     string `xml:"time,attr"`
		Suites   []struct {
			Name      string `xml:"name,attr"`
			File      string `xml:"file,attr"`
			Time      string `xml:"time,attr"`
			Timestamp string `xml:"timestamp,attr"`
			Tests     int    `xml:"tests,attr"`
			Props     []prop `xml:"properties>property"`
			Cases     []struct {
				Name      string `xml:"name,attr"`
				Classname string `xml:"classname,attr"`
				File      string `xml:"file,attr"`
				Time      string `xml:"time,attr"`
				Failure   *msg   `xml:"failure"`
				Error     *msg   `xml:"error"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	raw := renderJUnit(t, runner.JUnitOptions{})
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("not valid XML: %v\n%s", err, raw)
	}
	if len(doc.Suites) != 3 {
		t.Fatalf("suites = %+v", doc.Suites)
	}
	// Deterministic order: sorted by name.
	for i := 1; i < len(doc.Suites); i++ {
		if doc.Suites[i-1].Name > doc.Suites[i].Name {
			t.Errorf("suites are not sorted: %s, %s", doc.Suites[i-1].Name, doc.Suites[i].Name)
		}
	}
	sum := 0
	for _, s := range doc.Suites {
		if s.Time == "" || strings.Contains(s.File, "\\") || strings.HasPrefix(s.File, "/") {
			t.Errorf("suite %s: time=%q file=%q", s.Name, s.Time, s.File)
		}
		sum += s.Tests
		for _, c := range s.Cases {
			if c.File != s.File || strings.HasPrefix(c.Classname, "/") || strings.Contains(c.Classname, "Users") {
				t.Errorf("case %q: file=%q classname=%q", c.Name, c.File, c.Classname)
			}
		}
	}
	if doc.Tests != sum || doc.Failures != 2 || doc.Errors != 1 || doc.Skipped != 1 {
		t.Errorf("totals tests=%d failures=%d errors=%d skipped=%d", doc.Tests, doc.Failures, doc.Errors, doc.Skipped)
	}
	if strings.Contains(string(raw), "/Users/") || strings.Contains(string(raw), "/work/") {
		t.Errorf("an absolute path leaked into the report:\n%s", raw)
	}
}
