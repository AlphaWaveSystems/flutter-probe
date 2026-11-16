package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// stagedApp is an App whose last run left these results, with a save dialog that returns path.
func stagedApp(t *testing.T, results []runner.TestResult, path string) *App {
	t.Helper()
	a := &App{ctx: context.Background()}
	a.export.keep(results, runner.RunMetadata{DeviceName: "Pixel Test", DeviceID: "emu-1", Platform: "android", AppID: "com.example.app", ProbeVersion: "9.9.9"}, "/work/space")
	a.savePathFn = func(defaultName, label, pattern string) (string, error) { return path, nil }
	return a
}

func sampleResults() []runner.TestResult {
	return []runner.TestResult{
		{TestName: "passes", File: "/work/space/a.probe", Passed: true, Duration: 1500 * time.Millisecond, Attempts: 1},
		{TestName: "flaky", File: "/work/space/a.probe", Passed: true, Duration: 2 * time.Second, Attempts: 2},
		{TestName: "slow", File: "/work/space/a.probe", Passed: false, TimedOut: true, TimeLimit: 90 * time.Second, Error: errors.New("test exceeded 90s"), Duration: 90 * time.Second},
		{TestName: "fails", File: "/work/space/a.probe", Passed: false, Error: errors.New("element not found"), Duration: time.Second},
	}
}

// The file is the engine's own report of the run's results, byte for byte: the same bytes `probe test --format json` writes through the
// same reporter, with the attempt, timeout and error fields the engine writes.
func TestExportWritesTheEnginesJSONReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.json")
	a := stagedApp(t, sampleResults(), out)
	o, err := a.ExportRunResults("json")
	if err != nil || !o.Saved || o.Name != "r.json" || o.Tests != 4 {
		t.Fatalf("export = %+v, %v", o, err)
	}
	got, _ := os.ReadFile(out)
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJSON, &buf, false)
	rep.SetMetadata(runner.RunMetadata{DeviceName: "Pixel Test", DeviceID: "emu-1", Platform: "android", AppID: "com.example.app", ProbeVersion: "9.9.9"})
	if err := rep.Report(sampleResults()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, buf.Bytes()) {
		t.Errorf("the file is not the engine's report:\n%s\nvs\n%s", got, buf.Bytes())
	}
	var doc map[string]any
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"attempts", "timed_out", "time_limit_seconds", "test exceeded 90s", "element not found", "Pixel Test"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the report lacks %q", want)
		}
	}
}

func TestExportWritesTheEnginesJUnitReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.xml")
	nested := sampleResults()
	for i := range nested {
		nested[i].File = "/work/space/sub/a.probe"
	}
	a := stagedApp(t, nested, out)
	if o, err := a.ExportRunResults("junit"); err != nil || !o.Saved {
		t.Fatalf("export = %+v, %v", o, err)
	}
	got, _ := os.ReadFile(out)
	var doc struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Errors   int `xml:"errors,attr"`
	}
	if err := xml.Unmarshal(got, &doc); err != nil {
		t.Fatalf("not XML: %v\n%s", err, got)
	}
	if doc.Tests != 4 || doc.Failures+doc.Errors != 2 {
		t.Errorf("tests %d failures %d errors %d", doc.Tests, doc.Failures, doc.Errors)
	}
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJUnit, &buf, false)
	rep.SetMetadata(runner.RunMetadata{DeviceName: "Pixel Test", DeviceID: "emu-1", Platform: "android", AppID: "com.example.app", ProbeVersion: "9.9.9"})
	rep.SetJUnitOptions(runner.JUnitOptions{Base: "/work/space"})
	_ = rep.Report(nested)
	if !bytes.Equal(got, buf.Bytes()) {
		t.Error("the file is not the engine's JUnit report")
	}
	if strings.Contains(string(got), "/work/space") || !strings.Contains(string(got), "sub/a.probe") {
		t.Errorf("the JUnit report must name the file relative to the workspace (sub/a.probe), never its folder:\n%s", got)
	}
}

// The window cannot name a path: the method takes the format and nothing else.
func TestExportTakesNoPathFromTheWindow(t *testing.T) {
	m, _ := reflect.TypeOf(&App{}).MethodByName("ExportRunResults")
	if m.Type.NumIn() != 2 || m.Type.In(1).Kind() != reflect.String {
		t.Fatalf("ExportRunResults takes %v", m.Type)
	}
	a := stagedApp(t, sampleResults(), filepath.Join(t.TempDir(), "r.json"))
	for _, bad := range []string{"", "JSON", "json/../../x", "/tmp/evil.json", "csv"} {
		if _, err := a.ExportRunResults(bad); err == nil || !strings.Contains(err.Error(), "unknown export format") {
			t.Errorf("format %q: %v", bad, err)
		}
	}
}

// Cancelling the dialog writes nothing and is not an error.
func TestExportCancelledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	a := stagedApp(t, sampleResults(), "")
	o, err := a.ExportRunResults("json")
	if err != nil || o.Saved || o.Name != "" {
		t.Errorf("cancel = %+v, %v", o, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the folder holds %v", entries)
	}
}

// What the dialog returned is checked: a name of the wrong kind, a folder, a link and a missing folder are refused, and nothing is written.
func TestExportRefusesAPathItCannotUse(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link.json")
	target := filepath.Join(dir, "target.json")
	_ = os.WriteFile(target, []byte("keep"), 0o644)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symlinks")
	}
	folder := filepath.Join(dir, "folder.json")
	_ = os.Mkdir(folder, 0o755)
	for name, path := range map[string]string{
		"a relative path":   "r.json",
		"another extension": filepath.Join(dir, "r.txt"),
		"no extension":      filepath.Join(dir, "report"),
		"a folder":          folder,
		"a link":            link,
		"a missing folder":  filepath.Join(dir, "nope", "r.json"),
		"the xml for json":  filepath.Join(dir, "r.xml"),
		"a path with a NUL": filepath.Join(dir, "r\x00.json"),
	} {
		a := stagedApp(t, sampleResults(), path)
		if _, err := a.ExportRunResults("json"); err == nil || !strings.Contains(err.Error(), "nothing was written") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Error("a linked file was written")
	}
	if _, err := os.Stat(filepath.Join(dir, "r.txt")); err == nil {
		t.Error("a file with the wrong extension was written")
	}
}

// A file the person chose to replace is replaced whole and keeps its permissions; no temp file is left.
func TestExportReplacesAnExistingFileAtomically(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.json")
	_ = os.WriteFile(out, []byte("old"), 0o640)
	a := stagedApp(t, sampleResults(), out)
	if _, err := a.ExportRunResults("json"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), "passes") {
		t.Errorf("not replaced: %q", b)
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder holds %v", entries)
	}
}

// Nothing to export before a run has finished, none while a run is going, and none over the headless bridge or for an automation client.
func TestExportRefusals(t *testing.T) {
	a := &App{ctx: context.Background()}
	if _, err := a.ExportRunResults("json"); err == nil || !strings.Contains(err.Error(), "no results") {
		t.Errorf("before any run: %v", err)
	}
	a = stagedApp(t, sampleResults(), filepath.Join(t.TempDir(), "r.json"))
	_, cancel, err := a.claimRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExportRunResults("json"); err == nil || !strings.Contains(err.Error(), "run is going") {
		t.Errorf("during a run: %v", err)
	}
	a.releaseRun(cancel)
	if !methodRefusedOverHTTP("ExportRunResults") {
		t.Error("the headless bridge does not refuse the export: there is no dialog there")
	}
	b := &App{ctx: context.Background(), savePathFn: nil}
	b.export.keep(sampleResults(), runner.RunMetadata{}, "")
	if _, err := b.ExportRunResults("json"); err == nil || !strings.Contains(err.Error(), "needs the Studio window") {
		t.Errorf("without a window: %v", err)
	}
}

// A run through Studio leaves the engine's results for the export, a new run clears them until it ends, and the AI paths never see the export.
func TestARunLeavesItsResultsForTheExport(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  log \"hi\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.SetWorkspace(dir)
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 2 * time.Second
	cfg.Project.App = "com.example.app"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	conn := &connection{client: &runFake{}, cfg: cfg, deviceID: "sim-1", deviceName: "Test Phone", platform: "ios", streamCancel: cancel, streamDone: done}
	a.conn = conn
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
	defer func() { cancel(); <-done }()
	if _, err := a.RunFile(file); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "run.json")
	a.savePathFn = func(string, string, string) (string, error) { return out, nil }
	o, err := a.ExportRunResults("json")
	if err != nil || !o.Saved || o.Tests != 1 {
		t.Fatalf("export = %+v, %v", o, err)
	}
	got, _ := os.ReadFile(out)
	for _, want := range []string{`"Test Phone"`, `"com.example.app"`, `"name": "t"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the report lacks %s:\n%s", want, got)
		}
	}
}

func TestTheExportIsNotOnAnAIPath(t *testing.T) {
	files, _ := filepath.Glob("ai_*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, name := range []string{"ExportRunResults", "exportSource", "a.export", "keepForExport"} {
			if strings.Contains(string(b), name) {
				t.Errorf("%s refers to %s: the exported report must not reach an AI provider", f, name)
			}
		}
	}
}

// Export is never reachable by an automation client: its category is one automation may not hold, and no automation method is the export.
func TestTheExportIsNeverAutomationCallable(t *testing.T) {
	cat := boundMethods["ExportRunResults"]
	if cat != catWritesOutside || !automationNever[cat] {
		t.Errorf("ExportRunResults is %q, automationNever = %v", cat, automationNever[cat])
	}
	for _, name := range append(append([]string(nil), automationBackendMethods...), automationRPC...) {
		if strings.Contains(strings.ToLower(name), "export") {
			t.Errorf("the automation endpoint reaches %s", name)
		}
	}
}

// blockFake is a device whose agent does not answer until the test lets it.
type blockFake struct {
	runFake
	started chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (b *blockFake) Call(ctx context.Context, m string, p any) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (b *blockFake) Tap(ctx context.Context, sel probelink.SelectorParam) error {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.gate:
	case <-ctx.Done():
	}
	return nil
}

// A run that starts drops the results of the last one from the export until it ends: while it goes there is nothing old to export.
func TestANewRunDropsTheOldResultsFromTheExport(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  tap \"Go\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.SetWorkspace(dir)
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	fake := &blockFake{started: make(chan struct{}), gate: make(chan struct{})}
	a.conn = &connection{client: fake, cfg: cfg, streamCancel: cancel, streamDone: done}
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
	defer func() { cancel(); <-done }()
	a.export.keep(sampleResults(), runner.RunMetadata{}, dir)
	ran := make(chan struct{})
	go func() { _, _ = a.RunFile(file); close(ran) }()
	select {
	case <-fake.started:
	case <-time.After(10 * time.Second):
		close(fake.gate)
		t.Fatal("the run never reached the device")
	}
	if left, _, _ := a.export.get(); len(left) != 0 {
		t.Errorf("while the new run goes, %d results of the old one are still there to export", len(left))
	}
	close(fake.gate)
	<-ran
}

// An upper-case extension is the same extension: the dialog's own name is used as it is.
func TestExportAcceptsAnUpperCaseExtension(t *testing.T) {
	out := filepath.Join(t.TempDir(), "REPORT.JSON")
	a := stagedApp(t, sampleResults(), out)
	if o, err := a.ExportRunResults("json"); err != nil || !o.Saved || o.Name != "REPORT.JSON" {
		t.Fatalf("export = %+v, %v", o, err)
	}
	if err := checkExportPath(filepath.Join(t.TempDir(), "r.JsOn"), ".json"); err != nil {
		t.Errorf("a mixed-case extension was refused: %v", err)
	}
}

// Opening another workspace drops the results of a run in the old one: they are not exportable there, and their JUnit names would be relative to
// the old folder.
func TestAnotherWorkspaceDropsTheResultsToExport(t *testing.T) {
	a := stagedApp(t, sampleResults(), filepath.Join(t.TempDir(), "r.json"))
	if !a.CanExportRunResults() {
		t.Fatal("staged results are not exportable")
	}
	a.SetWorkspace(t.TempDir())
	if a.CanExportRunResults() {
		t.Error("SetWorkspace left the old results exportable")
	}
	b := stagedApp(t, sampleResults(), filepath.Join(t.TempDir(), "r.json"))
	b.useWorkspace(t.TempDir())
	if _, err := b.ExportRunResults("json"); err == nil || !strings.Contains(err.Error(), "no results") {
		t.Errorf("useWorkspace left the old results exportable: %v", err)
	}
}

// What the buttons follow: results held and no run going.
func TestCanExportFollowsWhatIsHeld(t *testing.T) {
	a := &App{ctx: context.Background()}
	if a.CanExportRunResults() {
		t.Error("nothing is held, yet export is offered")
	}
	a = stagedApp(t, sampleResults(), filepath.Join(t.TempDir(), "r.json"))
	if !a.CanExportRunResults() {
		t.Error("results are held and no run is going, yet export is not offered")
	}
	_, cancel, err := a.claimRun()
	if err != nil {
		t.Fatal(err)
	}
	if a.CanExportRunResults() {
		t.Error("a run is going, yet export is offered")
	}
	a.releaseRun(cancel)
}

// A run that was stopped leaves nothing to export: its results are partial.
func TestAStoppedRunLeavesNothingToExport(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  tap \"Go\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.SetWorkspace(dir)
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	fake := &blockFake{started: make(chan struct{}), gate: make(chan struct{})}
	a.conn = &connection{client: fake, cfg: cfg, streamCancel: cancel, streamDone: done}
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
	defer func() { cancel(); <-done }()
	ran := make(chan struct{})
	go func() { _, _ = a.RunFile(file); close(ran) }()
	select {
	case <-fake.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached the device")
	}
	a.CancelRun()
	<-ran
	if a.CanExportRunResults() {
		t.Error("a stopped run left partial results to export")
	}
}
