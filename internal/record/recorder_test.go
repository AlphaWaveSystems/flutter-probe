package record

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type recorderReply struct {
	result probelink.HTTPLogResult
	err    error
}
type recorderClient struct {
	replies            []recorderReply
	since              []int
	screenshotErr      error
	screenshots, trees int
}

func (c *recorderClient) HTTPLog(_ context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	c.since = append(c.since, p.Since)
	if len(c.replies) == 0 {
		return probelink.HTTPLogResult{Epoch: "one"}, nil
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return reply.result, reply.err
}
func (c *recorderClient) DumpWidgetTree(context.Context) (string, error) {
	c.trees++
	return "Root\n  Text", nil
}
func (c *recorderClient) Screenshot(context.Context, string) (string, error) {
	return "", errors.New("inline capture should be preferred")
}
func (c *recorderClient) ScreenshotBytes(context.Context, string) ([]byte, error) {
	c.screenshots++
	return []byte("\x89PNG\r\n\x1a\n"), c.screenshotErr
}

func recorderSetup(t *testing.T, policy string) (*Recorder, Event, string) {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, "test.probe")
	if err := os.WriteFile(file, []byte("test recording"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(Options{Dir: filepath.Join(root, "recordings"), RunID: "run", Device: "device", Screenshots: policy, CLIVersion: "version"})
	ev := Event{File: file, TestName: "My Test!", Attempt: 1, Line: 1, Description: "first", Status: "started", At: time.Now()}
	return r, ev, filepath.Join(root, "recordings")
}
func readRecorderJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func recordStep(t *testing.T, r *Recorder, c Client, ev Event, status string) {
	t.Helper()
	if err := r.BeginStep(c, ev); err != nil {
		t.Fatal(err)
	}
	ev.Status = status
	ev.Duration = 42 * time.Millisecond
	ev.Error = "Bearer abcdefghijklmnop"
	if err := r.EndStep(c, ev); err != nil {
		t.Fatal(err)
	}
}

func TestRecorderManifestCarriesPlatformAndOSVersion(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "test.probe")
	if err := os.WriteFile(file, []byte("test recording"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(Options{Dir: filepath.Join(root, "recordings"), RunID: "run", Device: "device", Screenshots: "never", CLIVersion: "version", Platform: "ios", OSVersion: "iOS 18.6"})
	ev := Event{File: file, TestName: "My Test!", Attempt: 1, Line: 1, Description: "first", Status: "started", At: time.Now()}
	recordStep(t, r, &recorderClient{}, ev, "passed")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "recordings", "my-test-attempt-1", "manifest.json")
	if m := readRecorderJSON[Manifest](t, path); m.Platform != "ios" || m.OSVersion != "iOS 18.6" {
		t.Fatal(m)
	}
	// The JSON keys are part of the bundle format, so check them by name as well.
	if raw := readRecorderJSON[map[string]any](t, path); raw["platform"] != "ios" || raw["os_version"] != "iOS 18.6" {
		t.Fatal(raw)
	}
}

func TestRecorderBundleAndRedaction(t *testing.T) {
	r, ev, root := recorderSetup(t, "every_step")
	c := &recorderClient{replies: []recorderReply{
		{result: probelink.HTTPLogResult{Epoch: "one", Latest: 1, Entries: []probelink.HTTPEntry{{Seq: 1, Method: "GET", URL: "https://example.test/?token=credential", Status: 200, ResponseBody: `{"password":"credential"}`, RequestHeaders: map[string]string{"Authorization": "credential"}}}}},
		{result: probelink.HTTPLogResult{Epoch: "one", Latest: 2, Entries: []probelink.HTTPEntry{{Seq: 2, Method: "POST", URL: "https://example.test/", Status: 500}}}},
	}}
	recordStep(t, r, c, ev, "passed")
	ev.Line = 2
	ev.Description = "second"
	recordStep(t, r, c, ev, "failed")
	dir := filepath.Join(root, "my-test-attempt-1")
	if got := readRecorderJSON[Timeline](t, filepath.Join(dir, "timeline.json")); len(got.Steps) != 2 {
		t.Fatal(got)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m := readRecorderJSON[Manifest](t, filepath.Join(dir, "manifest.json"))
	if m.Status != "failed" || m.FileSHA256 == "" || m.Capture.Native != "off" || !m.Capture.Redaction || m.EndedAt.IsZero() {
		t.Fatal(m)
	}
	timeline := readRecorderJSON[Timeline](t, filepath.Join(dir, "timeline.json"))
	for i, s := range timeline.Steps {
		if s.Index != i+1 || s.DurationMs != 42 || s.StartedAt.IsZero() || s.Error != Placeholder {
			t.Fatal(s)
		}
		if len(s.Exchanges) != 1 || s.Exchanges[0] != []string{"0001", "0002"}[i] {
			t.Fatal(s)
		}
		for _, path := range []string{filepath.Join("http", s.Exchanges[0]+".json"), filepath.Join("tree", s.Tree+".txt"), filepath.Join("screenshots", s.ScreenshotBefore), filepath.Join("screenshots", s.ScreenshotAfter)} {
			if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
				t.Fatal(err)
			}
		}
	}
	e := readRecorderJSON[Exchange](t, filepath.Join(dir, "http", "0001.json"))
	if e.URL != "https://example.test/?token="+Placeholder || e.ResponseBody != `{"password":"<redacted>"}` || e.RequestHeaders["Authorization"] != Placeholder || e.Source != "dart_io" {
		t.Fatal(e)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings()) != 0 {
		t.Fatal(r.Warnings())
	}
}

func TestRecorderTestsRetriesAndCollisions(t *testing.T) {
	r, ev, root := recorderSetup(t, "never")
	c := &recorderClient{}
	recordStep(t, r, c, ev, "passed")
	ev.TestName = "Other test"
	recordStep(t, r, c, ev, "skipped")
	first := filepath.Join(root, "my-test-attempt-1", "manifest.json")
	before, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	ev.TestName = "My Test!"
	ev.Attempt = 2
	recordStep(t, r, c, ev, "passed")
	ev.Attempt = 1
	recordStep(t, r, c, ev, "passed")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("first attempt changed")
	}
	for _, name := range []string{"my-test-attempt-1", "other-test-attempt-1", "my-test-attempt-2", "my-test-attempt-1-2"} {
		if _, err := os.Stat(filepath.Join(root, name, "index.html")); err != nil {
			t.Fatal(err)
		}
	}
	if c.screenshots != 0 || c.trees != 0 {
		t.Fatal("never policy captured artifacts")
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".png") {
			t.Errorf("unexpected PNG %s", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecorderDeviceErrorsPreserveStepsAndCursor(t *testing.T) {
	r, ev, root := recorderSetup(t, "every_step")
	c := &recorderClient{screenshotErr: errors.New("screenshot failed"), replies: []recorderReply{{err: errors.New("HTTP failed")}, {result: probelink.HTTPLogResult{Epoch: "one", Latest: 1, Entries: []probelink.HTTPEntry{{Seq: 1}}}}}}
	recordStep(t, r, c, ev, "passed")
	ev.Line = 2
	recordStep(t, r, c, ev, "passed")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.since, []int{0, 0}) {
		t.Fatal(c.since)
	}
	warnings := strings.Join(r.Warnings(), "\n")
	if !strings.Contains(warnings, "HTTP failed") || !strings.Contains(warnings, "screenshot failed") {
		t.Fatal(warnings)
	}
	timeline := readRecorderJSON[Timeline](t, filepath.Join(root, "my-test-attempt-1", "timeline.json"))
	if len(timeline.Steps) != 2 || len(timeline.Steps[1].Exchanges) != 1 {
		t.Fatal(timeline)
	}
}

func TestRecorderOnFailureAndNilClient(t *testing.T) {
	r, ev, _ := recorderSetup(t, "on_failure")
	c := &recorderClient{}
	recordStep(t, r, c, ev, "passed")
	ev.Line = 2
	recordStep(t, r, c, ev, "failed")
	if c.screenshots != 1 || c.trees != 1 {
		t.Fatal(c)
	}
	ev.Line = 3
	recordStep(t, r, nil, ev, "failed")
	if len(r.Warnings()) != 3 {
		t.Fatal(r.Warnings())
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordingSlug(t *testing.T) {
	for input, want := range map[string]string{" A__B ☃ C ": "a-b-c", "☃": "test", strings.Repeat("a", 70): strings.Repeat("a", 60)} {
		if got := recordingSlug(input); got != want {
			t.Errorf("%q => %q want %q", input, got, want)
		}
	}
}

func TestRecorderWriteFailureStillRecordsStep(t *testing.T) {
	r, ev, root := recorderSetup(t, "every_step")
	c := &recorderClient{}
	if err := r.BeginStep(c, ev); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "my-test-attempt-1", "screenshots", "0001-after.png")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	ev.Status = "passed"
	if err := r.EndStep(c, ev); err == nil {
		t.Fatal("expected bundle write error")
	}
	timeline := readRecorderJSON[Timeline](t, filepath.Join(root, "my-test-attempt-1", "timeline.json"))
	if len(timeline.Steps) != 1 || timeline.Steps[0].Status != "passed" || timeline.Steps[0].ScreenshotAfter != "" {
		t.Fatal(timeline)
	}
	if len(r.Warnings()) == 0 {
		t.Fatal("missing write warning")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecorderDiscontinuities(t *testing.T) {
	r, ev, root := recorderSetup(t, "never")
	c := &recorderClient{replies: []recorderReply{
		{result: probelink.HTTPLogResult{Epoch: "one", Latest: 301, Entries: []probelink.HTTPEntry{{Seq: 301}}}},
		{result: probelink.HTTPLogResult{Epoch: "two", Latest: 1}},
		{result: probelink.HTTPLogResult{Epoch: "two", Latest: 1, Entries: []probelink.HTTPEntry{{Seq: 1}}}},
	}}
	recordStep(t, r, c, ev, "passed")
	ev.Line = 2
	recordStep(t, r, c, ev, "passed")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "my-test-attempt-1")
	m := readRecorderJSON[Manifest](t, filepath.Join(dir, "manifest.json"))
	timeline := readRecorderJSON[Timeline](t, filepath.Join(dir, "timeline.json"))
	if m.Truncated.Exchanges != 1 || !timeline.Steps[1].AppRestarted {
		t.Fatal(m, timeline)
	}
}
