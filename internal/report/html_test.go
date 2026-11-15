package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/report"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func TestNewHTMLReport(t *testing.T) {
	r := report.NewHTMLReport("/tmp/report.html", "TestProject")
	if r.OutputPath != "/tmp/report.html" {
		t.Errorf("output path: %q", r.OutputPath)
	}
	if r.ProjectName != "TestProject" {
		t.Errorf("project name: %q", r.ProjectName)
	}
}

func TestHTMLReport_Write_Empty(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	r := report.NewHTMLReport(outPath, "Empty Test")
	err := r.Write(nil, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "Empty Test") {
		t.Error("report should contain project name")
	}
	if !strings.Contains(html, "FlutterProbe") {
		t.Error("report should contain FlutterProbe branding")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("report should be valid HTML")
	}
}

func TestHTMLReport_Write_WithResults(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	results := []runner.TestResult{
		{TestName: "login test", File: "tests/login.probe", Passed: true, Duration: 150 * time.Millisecond},
		{TestName: "checkout test", File: "tests/checkout.probe", Passed: false, Duration: 200 * time.Millisecond, Error: testErr("button not found")},
		{TestName: "skipped test", File: "tests/skip.probe", Skipped: true},
	}

	r := report.NewHTMLReport(outPath, "My App")
	err := r.Write(results, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	// Check counters
	if !strings.Contains(html, ">1<") {
		t.Error("should show 1 passed")
	}
	if !strings.Contains(html, "My App") {
		t.Error("should contain project name")
	}
}

func TestHTMLReport_Write_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	r := report.NewHTMLReport(outPath, "Meta Test")
	r.Metadata = &report.ReportMetadata{
		DeviceName: "Pixel 7",
		DeviceID:   "emulator-5554",
		Platform:   "android",
		OSVersion:  "Android 14",
		AppID:      "com.example.app",
		AppVersion: "1.2.3",
	}

	err := r.Write([]runner.TestResult{{TestName: "t", Passed: true}}, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "Pixel 7") {
		t.Error("should contain device name")
	}
	if !strings.Contains(html, "Android 14") {
		t.Error("should contain OS version")
	}
	if !strings.Contains(html, "com.example.app") {
		t.Error("should contain app ID")
	}
}

func TestHTMLReport_Write_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "sub", "dir", "report.html")

	r := report.NewHTMLReport(outPath, "Nested")
	err := r.Write(nil, nil)
	if err != nil {
		t.Fatalf("write should create parent dirs: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestHTMLReport_Write_WithArtifacts(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	results := []runner.TestResult{
		{TestName: "screenshot test", File: "tests/a.probe", Passed: true},
	}
	artifacts := map[string][]string{
		"screenshot test": {"screenshots/step1.png", "screenshots/step2.png"},
	}

	r := report.NewHTMLReport(outPath, "Artifact Test")
	err := r.Write(results, artifacts)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "step1.png") {
		t.Error("should contain artifact reference")
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

// bs is a backslash, so the JSON escapes below read plainly
const bs = "\\"

func writeHTML(t *testing.T, results []runner.TestResult, props map[string]string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "report.html")
	r := report.NewHTMLReport(out, "P")
	r.Properties = props
	if err := r.Write(results, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The tags and properties reach the page as JSON data and are put on it as text nodes: the markup in a value is never markup.
func TestHTMLReport_TagsAndPropertiesAreDataNotMarkup(t *testing.T) {
	payload := `<img src=x onerror=alert(1)></script><script>alert(2)</script>`
	page := writeHTML(t,
		[]runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1, Tags: []string{"smoke", payload}}},
		map[string]string{"build": payload})
	if strings.Contains(page, "<img src=x") || strings.Contains(page, "<script>alert") || strings.Contains(page, "</script><script") {
		t.Fatalf("a tag or property value was written as markup")
	}
	if !strings.Contains(page, bs+"u003cimg src=x onerror=alert(1)"+bs+"u003e") {
		t.Fatalf("the value is not in the page data in escaped form")
	}
	// the code that puts them on the page uses text nodes; innerHTML never receives a tag or a property
	for _, fn := range []string{"function renderProps()", "// Tags are filled as text nodes"} {
		i := strings.Index(page, fn)
		if i < 0 {
			t.Fatalf("%q missing from the page script", fn)
		}
		body := page[i:]
		if j := strings.Index(body[len(fn):], "\n}\n"); j >= 0 {
			body = body[:len(fn)+j]
		} else if j := strings.Index(body, "// Pass bar"); j >= 0 {
			body = body[:j]
		}
		if strings.Contains(body, "innerHTML") || !strings.Contains(body, "textContent") {
			t.Errorf("%q: must use textContent only:\n%s", fn, body)
		}
	}
}

func TestHTMLReport_NoTagsNoProperties(t *testing.T) {
	page := writeHTML(t, []runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}, nil)
	if !strings.Contains(page, "const PROPS = [];") || strings.Contains(page, `"tags":`) {
		t.Fatalf("empty report carries tags or properties data")
	}
}

func TestHTMLReport_PropertiesAreBoundedAndCleaned(t *testing.T) {
	props := map[string]string{"ok": "v\x00\x07w", "bad key": "x", "run_label": "reserved"}
	page := writeHTML(t, []runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1}}, props)
	if strings.Contains(page, "bad key") || strings.Contains(page, "reserved") || strings.Contains(page, bs+"u0000") || strings.Contains(page, bs+"u0007") {
		t.Fatalf("hand-made properties were not cleaned: %s", page[strings.Index(page, "const PROPS"):][:200])
	}
	if !strings.Contains(page, `{"name":"ok","value":"v  w"}`) {
		t.Fatalf("clean property missing")
	}
}

// A tag alone (no property) must reach the page data.
func TestHTMLReport_TagReachesThePageData(t *testing.T) {
	page := writeHTML(t, []runner.TestResult{{TestName: "t", File: "a.probe", Passed: true, Row: -1, Tags: []string{"smoke"}}}, nil)
	if !strings.Contains(page, `"tags":["smoke"]`) {
		t.Fatalf("tag missing from the page data")
	}
}
