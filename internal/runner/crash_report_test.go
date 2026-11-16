package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/hosted"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func goldenResults() []TestResult {
	return []TestResult{
		{TestName: "passes", File: "a.probe", Passed: true, Duration: 1500 * time.Millisecond, Row: -1, Tags: []string{"smoke"}, Artifacts: []string{"/work/reports/shot.png", "rel.png"}, DeviceID: "emulator-5600", DeviceName: "Pixel", Attempts: 2},
		{TestName: "fails", File: "b.probe", Passed: false, Duration: 2 * time.Second, Row: 1, Error: errors.New("step failed"), DeviceID: "emulator-5600"},
		{TestName: "skipped", File: "c.probe", Skipped: true, Row: -1},
	}
}

// With no crash evidence the report and the stream are byte for byte what they were before the "crash" object existed (recorded from the unchanged reporter).
func TestReportsWithoutCrashEvidenceAreUnchanged(t *testing.T) {
	r := &Reporter{outputDir: "/work/reports"}
	got, _ := json.MarshalIndent(r.buildJSONReport(goldenResults()).Results, "", "  ")
	want, err := os.ReadFile(filepath.Join("testdata", "report_results_no_crash.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the JSON results changed:\n%s", got)
	}
	var stream []byte
	for _, res := range goldenResults() {
		var buf bytes.Buffer
		sr := &Reporter{outputDir: "/work/reports", streaming: true, out: &buf}
		sr.StreamResult(res)
		stream = append(stream, buf.Bytes()...)
	}
	wantStream, _ := os.ReadFile(filepath.Join("testdata", "report_stream_no_crash.golden.jsonl"))
	if !bytes.Equal(stream, wantStream) {
		t.Errorf("the stream changed:\n%s", stream)
	}
	if strings.Contains(string(got), "crash") || strings.Contains(string(stream), "crash") {
		t.Error("the word crash appears in a report without evidence")
	}
}

func hostileCrash() *CrashReport {
	return &CrashReport{
		Gone: true,
		Kind: device.ExitNativeCrash,
		Evidence: device.CrashEvidence{
			Package: "dev.probe.app",
			Exit:    &device.ExitRecord{PID: 3277, Process: "dev.probe.app:\x1b[2Jsync", Description: "boom\r\x00\u202eevil", Kind: device.ExitNativeCrash, Status: 6, WallClock: time.Date(2026, 11, 15, 19, 31, 9, 870e6, time.UTC)},
			Exits: []device.ExitRecord{
				{PID: 3277, Process: "dev.probe.app", Description: "a\u2028b\x1b[31m", Kind: device.ExitNativeCrash, Status: 6, WallClock: time.Date(2026, 11, 15, 19, 31, 9, 870e6, time.UTC)},
				{PID: 3000, Process: "dev.probe.app", Description: "stop", Kind: device.ExitUserRequested, WallClock: time.Date(2026, 11, 15, 19, 0, 0, 0, time.UTC)},
			},
			Summary:    "Fatal signal 6 (SIGABRT)\x1b[2J",
			CrashLines: []string{"Fatal signal 6 (SIGABRT)", "\tat x.Y(Z.java:1)\r", "evil\x1b[2Jline\u202e", "bad\x00nul"},
			Truncated:  true,
		},
	}
}

func hasHidden(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029 || r == 0x202e || r == 0x2066 || r == 0x200b {
			return true
		}
	}
	return false
}

func TestTheCrashObjectInTheJSONReport(t *testing.T) {
	c := hostileCrash()
	c.Files = []string{"/work/reports/crash/t/exit.txt", "/elsewhere/crash.txt", "/work/reports/../up.txt"}
	res := goldenResults()
	res[1].Crash = c
	r := &Reporter{outputDir: "/work/reports"}
	b, err := json.Marshal(r.buildJSONReport(res))
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Results []struct {
			Name  string     `json:"name"`
			Crash *jsonCrash `json:"crash"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Results[0].Crash != nil || back.Results[2].Crash != nil || back.Results[1].Crash == nil {
		t.Fatalf("the object is on the wrong results: %s", b)
	}
	j := back.Results[1].Crash
	if j.Kind != "native_crash" || !j.Gone || j.PID != 3277 || j.ExitTime != "2026-11-15 19:31:09.870" || !j.Truncated {
		t.Errorf("crash object = %+v", j)
	}
	if j.Process != "dev.probe.app: [2Jsync" || j.Description != "boom evil" || j.Summary != "Fatal signal 6 (SIGABRT) [2J" {
		t.Errorf("strings not cleaned: %q %q %q", j.Process, j.Description, j.Summary)
	}
	for _, s := range []string{j.Process, j.Description, j.Summary, j.Kind} {
		if hasHidden(s) {
			t.Errorf("hidden character left in %q", s)
		}
	}
	if len(j.Files) != 3 || j.Files[0] != "crash/t/exit.txt" || j.Files[1] != "/elsewhere/crash.txt" || j.Files[2] != "/work/reports/../up.txt" {
		t.Errorf("files = %q (inside the report folder: relative; anything else as given, never a path that climbs out)", j.Files)
	}
	// the stream carries the same object
	var buf bytes.Buffer
	sr := &Reporter{outputDir: "/work/reports", streaming: true, out: &buf}
	sr.StreamResult(res[1])
	if !strings.Contains(buf.String(), `"crash":{"kind":"native_crash","gone":true,"pid":3277`) {
		t.Errorf("stream = %s", buf.String())
	}
	// a nil report has no object
	if crashJSON(nil, "/x") != nil {
		t.Error("a nil report has an object")
	}
	// a report with no exit entry still has its kind
	bare := crashJSON(&CrashReport{Gone: true, Kind: device.ExitUnknown}, "")
	if bare == nil || bare.Kind != "unknown" || bare.PID != 0 || bare.ExitTime != "" {
		t.Errorf("bare = %+v", bare)
	}
}

func TestWritingTheCrashFiles(t *testing.T) {
	dir := t.TempDir()
	files, err := writeCrashFiles(dir, "a.probe", "checkout / the \"buy\" step", 1, hostileCrash())
	if err != nil || len(files) != 2 {
		t.Fatalf("files %v, err %v", files, err)
	}
	for _, f := range files {
		if !strings.HasPrefix(f, dir) || filepath.Dir(filepath.Dir(f)) != dir {
			t.Errorf("%s is outside %s/<test>/", f, dir)
		}
		b, _ := os.ReadFile(f)
		if hasHidden(string(b)) || strings.Contains(string(b), "\r") {
			t.Errorf("%s holds a control or hidden character: %q", f, b)
		}
	}
	if filepath.Base(files[0]) != "exit.txt" || filepath.Base(files[1]) != "crash.txt" {
		t.Errorf("files = %v", files)
	}
	exit, _ := os.ReadFile(files[0])
	if strings.Count(string(exit), "\n") != 2 || !strings.Contains(string(exit), "2026-11-15 19:31:09.870 pid=3277 kind=native_crash status=6 process=dev.probe.app description=a b [31m") {
		t.Errorf("exit.txt = %q", exit)
	}
	crash, _ := os.ReadFile(files[1])
	if !strings.HasPrefix(string(crash), "# Fatal signal 6 (SIGABRT) [2J\nFatal signal 6 (SIGABRT)\nat x.Y(Z.java:1)\n") || !strings.HasSuffix(string(crash), "# (cut: more text was left out)\n") {
		t.Errorf("crash.txt = %q", crash)
	}
	// nothing to write: no files, no folder
	empty := t.TempDir()
	if f, err := writeCrashFiles(empty, "a.probe", "t", 1, &CrashReport{Gone: true, Kind: device.ExitUnknown}); err != nil || len(f) != 0 {
		t.Errorf("empty evidence wrote %v (%v)", f, err)
	}
	if f, err := writeCrashFiles(empty, "a.probe", "t", 1, nil); f != nil || err != nil {
		t.Errorf("nil report: %v %v", f, err)
	}
}

// A test name is never a path: it cannot climb out of the folder, and an empty name still gets a folder.
func TestCrashFolderNamesAreNeverPaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../../etc/passwd", "/abs/path", "..", "", "a/b\\c", "\x00\x1b", strings.Repeat("x", 500)} {
		files, err := writeCrashFiles(dir, "a.probe", name, 1, hostileCrash())
		if err != nil || len(files) == 0 {
			t.Fatalf("%q: %v %v", name, files, err)
		}
		for _, f := range files {
			rel, err := filepath.Rel(dir, f)
			if err != nil || strings.HasPrefix(rel, "..") || strings.Count(rel, string(filepath.Separator)) != 1 {
				t.Errorf("%q wrote %s (relative %q)", name, f, rel)
			}
		}
	}
	if got := crashFolderName("f", "", 1); !strings.HasPrefix(got, "test-") {
		t.Errorf("empty name = %q", got)
	}
	if got := crashFolderName("f", strings.Repeat("x", 500), 1); len(got) > 60+9 {
		t.Errorf("long name = %d characters", len(got))
	}
}

// A folder that cannot be written is a warning, never a failed test and never a file in the report.
func TestAnUnwritableCrashFolderIsOnlyAWarning(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "reports")
	if err := os.WriteFile(blocker, []byte("a file where the folder should be"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{Reports: blocker}}
	c := hostileCrash()
	r.writeCrashFilesFor(c, "a.probe", "t", 1)
	if len(c.Files) != 0 {
		t.Errorf("files recorded after a failed write: %v", c.Files)
	}
	if j := crashJSON(c, dir); j == nil || len(j.Files) != 0 || j.Kind != "native_crash" {
		t.Errorf("the object after a failed write: %+v", j)
	}
	// and the usual case: the folder is <reports>/crash
	ok := &Runner{cfg: &config.Config{Reports: filepath.Join(dir, "out")}}
	c2 := hostileCrash()
	ok.writeCrashFilesFor(c2, "a.probe", "my test", 1)
	if len(c2.Files) != 2 || !strings.HasPrefix(c2.Files[0], filepath.Join(dir, "out", "crash", "my_test-")) {
		t.Errorf("files = %v", c2.Files)
	}
	ok.writeCrashFilesFor(nil, "a.probe", "x", 1) // a test with no evidence writes nothing and does not panic
}

// The hosted upload never carries the crash object.
func TestTheHostedUploadDropsTheCrashObject(t *testing.T) {
	report := []byte(`{"total_tests":1,"results":[{"name":"t","file":"a.probe","passed":false,"crash":{"kind":"crash","summary":"boom token=abc123abc123","files":["crash/t/crash.txt"]}}]}`)
	out, _, err := hosted.NewScrubber().Scrub(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "crash") || strings.Contains(string(out), "boom") {
		t.Errorf("the crash object was uploaded: %s", out)
	}
}

// End to end through a test attempt: a lost connection, a dead app, the files under <reports>/crash/<test>/ and the object in the JSON report. A test that passes
// and a run with the opt-in off leave nothing behind.
func TestAFailedTestWithADeadAppWritesItsEvidence(t *testing.T) {
	reports := t.TempDir()
	run := func(on bool, testSrc string) (TestResult, *ceAdb) {
		var log []string
		a := &ceAdb{log: &log, date: "2026-11-15 19:31:12", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
		dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: on, CrashADB: a, CrashPoll: time.Millisecond}
		cfg := &config.Config{Reports: reports}
		cfg.Agent.ReconnectAttempts = 1
		r := New(cfg, &ceClient{log: &log}, dc, RunOptions{Timeout: 80 * time.Millisecond})
		prog, err := parser.ParseFile(testSrc)
		if err != nil {
			t.Fatal(err)
		}
		return r.runSingleTest(context.Background(), prog, prog.Tests[0], "x.probe", nil, -1), a
	}
	res, _ := run(true, "test \"checkout dies\"\n  press enter\n")
	if res.Passed || res.Crash == nil || res.Crash.Kind != device.ExitCrash || len(res.Crash.Files) != 2 {
		t.Fatalf("result = passed %v, crash %+v", res.Passed, res.Crash)
	}
	for _, f := range res.Crash.Files {
		if !strings.HasPrefix(f, filepath.Join(reports, "crash", "checkout_dies-")) {
			t.Errorf("file %s is not under <reports>/crash/<test>/", f)
		}
		if _, err := os.Stat(f); err != nil {
			t.Errorf("file not written: %v", err)
		}
	}
	rep := &Reporter{outputDir: reports}
	b, _ := json.Marshal(rep.buildJSONReport([]TestResult{res}))
	if !strings.Contains(string(b), `"files":["crash/checkout_dies-`+crashFolderName("x.probe", "checkout dies", 1)[len("checkout_dies-"):]+`/exit.txt","crash/checkout_dies-`+crashFolderName("x.probe", "checkout dies", 1)[len("checkout_dies-"):]+`/crash.txt"]`) {
		t.Errorf("report = %s", b)
	}
	// opt-in off: no evidence, no folder entry for this test beyond the first run's
	before, _ := filepath.Glob(filepath.Join(reports, "crash", "*"))
	res, a := run(false, "test \"other dies\"\n  press enter\n")
	after, _ := filepath.Glob(filepath.Join(reports, "crash", "*"))
	if res.Crash != nil || a.words("exit-info") != 0 || a.words("logcat") != 0 || len(after) != len(before) {
		t.Errorf("opt-in off: crash %v, %d exit-info reads, folders %d -> %d", res.Crash, a.words("exit-info"), len(before), len(after))
	}
}

// Two things that must not share a folder never do, and the same thing always does.
func TestCrashFoldersAreUniquePerFileNameRowAndAttempt(t *testing.T) {
	type key struct {
		file, name string
		attempt    int
	}
	cases := []key{
		{"a.probe", "ログイン", 1}, {"a.probe", "支払い", 1}, // both reduce to no letters at all
		{"a.probe", "Login", 1}, {"a.probe", "login", 1}, {"a.probe", "LOGIN", 1}, // a disk that ignores case would merge these
		{"a.probe", "t [row 1]", 1}, {"a.probe", "t [row 2]", 1}, {"a.probe", "t [row 10]", 1}, // rows of a data-driven test
		{"a.probe", "t", 1}, {"a.probe", "t", 2}, {"a.probe", "t", 3}, // attempts of a retry
		{"b.probe", "t", 1},                                                 // the same name in another file
		{"a.probe", "a b", 1}, {"a.probe", "a_b", 1}, {"a.probe", "a-b", 1}, // reduce to the same letters
		{"a.probe", "CON", 1}, {"a.probe", "NUL", 1}, {"a.probe", "aux", 1}, // names Windows reserves
	}
	seen := map[string]key{}
	lower := map[string]key{}
	for _, c := range cases {
		f := crashFolderName(c.file, c.name, c.attempt)
		if other, dup := seen[f]; dup {
			t.Errorf("%v and %v share the folder %q", c, other, f)
		}
		seen[f] = c
		if other, dup := lower[strings.ToLower(f)]; dup {
			t.Errorf("%v and %v share a folder on a disk that ignores case: %q", c, other, f)
		}
		lower[strings.ToLower(f)] = c
		if f != crashFolderName(c.file, c.name, c.attempt) {
			t.Errorf("%v: the folder name is not stable", c)
		}
		// base, a dash, eight hex digits: never equal to a reserved device name, which Windows only reserves without a suffix
		i := strings.LastIndex(f, "-")
		if i < 1 || len(f)-i-1 != 8 || strings.Trim(f[i+1:], "0123456789abcdef") != "" {
			t.Errorf("%v: folder %q is not <name>-<8 hex>", c, f)
		}
		if strings.ContainsAny(f, "/\\:*?\"<>| ") || strings.HasPrefix(f, ".") {
			t.Errorf("%v: folder %q is not a plain name", c, f)
		}
	}
}

// Two attempts and two rows of one test, and two tests with non-ASCII names, each keep their own evidence on disk, and the report points at what was written.
func TestEvidenceOfSimilarTestsIsKeptApart(t *testing.T) {
	dir := t.TempDir()
	written := map[string]string{}
	for i, c := range []struct {
		name    string
		attempt int
	}{{"ログイン", 1}, {"支払い", 1}, {"t [row 1]", 1}, {"t [row 2]", 1}, {"t", 1}, {"t", 2}, {"Login", 1}, {"login", 1}} {
		rep := hostileCrash()
		rep.Evidence.Summary = "evidence number " + strconv.Itoa(i)
		files, err := writeCrashFiles(dir, "a.probe", c.name, c.attempt, rep)
		if err != nil || len(files) != 2 {
			t.Fatalf("%v: %v %v", c, files, err)
		}
		for _, f := range files {
			if prev, dup := written[f]; dup {
				t.Fatalf("%v overwrote the file of %s: %s", c, prev, f)
			}
			written[f] = c.name
		}
		if b, _ := os.ReadFile(files[1]); !strings.Contains(string(b), "evidence number "+strconv.Itoa(i)) {
			t.Errorf("%v: crash.txt holds %q", c, b)
		}
	}
	if len(written) != 16 {
		t.Errorf("%d files for 8 attempts", len(written))
	}
}

// Through the runner: the attempts of a retried test, and the rows of a data-driven one, each get a folder of their own.
func TestRetriedAttemptsAndRowsWriteTheirOwnFolders(t *testing.T) {
	reports := t.TempDir()
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:12", pidof: []string{""},
		exit: []string{ceEntry("3567", "2026-11-15 19:31:09.870", "5"), ceEntry("3999", "2026-11-15 19:31:10.500", "5"), ceEntry("4100", "2026-11-15 19:31:11.100", "5")}}
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: true, CrashADB: a, CrashPoll: time.Millisecond}
	cfg := &config.Config{Reports: reports}
	cfg.Agent.ReconnectAttempts = 1
	cfg.Defaults.RetryFailedTests = 1
	r := New(cfg, &ceClient{log: &log}, dc, RunOptions{Timeout: 80 * time.Millisecond})
	prog, err := parser.ParseFile("test \"t\"\n  press enter\n")
	if err != nil {
		t.Fatal(err)
	}
	res := r.runSingleTest(context.Background(), prog, prog.Tests[0], "x.probe", nil, -1)
	if res.Passed || res.Attempts != 2 || res.Crash == nil {
		t.Fatalf("result = passed %v, attempts %d, crash %v", res.Passed, res.Attempts, res.Crash)
	}
	folders, _ := filepath.Glob(filepath.Join(reports, "crash", "t-*"))
	if len(folders) != 2 {
		t.Errorf("two attempts left %d folders: %v", len(folders), folders)
	}
	if want := filepath.Join(reports, "crash", crashFolderName("x.probe", "t", 2)); !strings.HasPrefix(res.Crash.Files[0], want) {
		t.Errorf("the report points at %s, want it under %s", res.Crash.Files[0], want)
	}
	// the rows of a data-driven test: the row-suffixed name decides the folder
	if crashFolderName("x.probe", rowName("t", 0), 1) == crashFolderName("x.probe", rowName("t", 1), 1) {
		t.Error("two rows share a folder")
	}
}

// The rows of a data-driven test: each row's folder is made from its own result name ("t [row 2]"), so the rows keep their evidence apart.
func TestDataRowsWriteTheirOwnFolders(t *testing.T) {
	reports := t.TempDir()
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:12", pidof: []string{""},
		exit: []string{ceEntry("3567", "2026-11-15 19:31:09.870", "5"), ceEntry("3999", "2026-11-15 19:31:10.500", "5")}}
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: true, CrashADB: a, CrashPoll: time.Millisecond}
	cfg := &config.Config{Reports: reports}
	cfg.Agent.ReconnectAttempts = 1
	r := New(cfg, &ceClient{log: &log}, dc, RunOptions{Timeout: 80 * time.Millisecond})
	prog, err := parser.ParseFile("test \"t\"\n  press enter\n\nwith examples:\n  x\n  \"a\"\n  \"b\"\n")
	if err != nil {
		t.Fatal(err)
	}
	results, err := r.runTest(context.Background(), prog, prog.Tests[0], "x.probe")
	if err != nil || len(results) != 2 {
		t.Fatalf("%v, %d results", err, len(results))
	}
	folders, _ := filepath.Glob(filepath.Join(reports, "crash", "t_row_*"))
	if len(folders) != 2 {
		t.Errorf("two rows left %d folders: %v", len(folders), folders)
	}
	for i, res := range results {
		if res.Crash == nil || len(res.Crash.Files) == 0 || !strings.HasPrefix(res.Crash.Files[0], filepath.Join(reports, "crash", crashFolderName("x.probe", rowName("t", i), 1))) {
			t.Errorf("row %d: %+v", i+1, res.Crash)
		}
	}
}
