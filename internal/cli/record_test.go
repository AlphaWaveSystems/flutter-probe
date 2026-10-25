package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/record"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func TestRecordingEvent(t *testing.T) {
	at := time.Now()
	ev := runner.StepEvent{File: "test.probe", TestName: "checkout", Line: 7, Description: "tap buy", Status: runner.StepFailed, Duration: time.Second, Error: errors.New("step failed"), Attempt: 2, Depth: 1, At: at}
	want := record.Event{File: ev.File, TestName: ev.TestName, Line: ev.Line, Description: ev.Description, Status: "failed", Duration: ev.Duration, Error: "step failed", Attempt: ev.Attempt, Depth: ev.Depth, At: at}
	if got := recordingEvent(ev); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	ev.Error = nil
	if got := recordingEvent(ev).Error; got != "" {
		t.Fatalf("nil error converted to %q", got)
	}
}

func TestRecordingFinishWithoutEvents(t *testing.T) {
	base := filepath.Join(t.TempDir(), "recordings")
	r := newRunRecording(base)
	r.NewDevice("Pixel 7")
	dirs, warnings := r.Finish()
	if len(dirs) != 0 || len(warnings) != 0 {
		t.Fatalf("dirs=%v warnings=%v", dirs, warnings)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("recording root should not exist: %v", err)
	}
}

func TestRecordingDeviceSlug(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"Pixel 7 (emulator-5554)", "pixel-7-emulator-5554"},
		{"  IPHONE___15!! ", "iphone-15"},
		{"abc123", "abc123"},
		{"", "device"},
		{"---🦋---", "device"},
		{"A / B", "a-b"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := deviceSlug(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRecordingNewDeviceInfoWritesPlatformAndOSVersion(t *testing.T) {
	r := newRunRecording(t.TempDir())
	rec := r.NewDeviceInfo("Pixel 7", "android", "Android 14")
	ev := runner.StepEvent{File: "missing.probe", TestName: "test", Line: 1, Description: "step", Status: runner.StepStarted, At: time.Now()}
	// A nil client simulates capture failures; the manifest is still written before the first capture.
	_ = rec.BeginStep(nil, ev)
	dirs, _ := r.Finish()
	if len(dirs) != 1 {
		t.Fatalf("dirs=%v", dirs)
	}
	matches, err := filepath.Glob(filepath.Join(dirs[0], "*", "manifest.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("manifest files=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var m record.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Platform != "android" || m.OSVersion != "Android 14" {
		t.Fatalf("platform=%q os_version=%q", m.Platform, m.OSVersion)
	}
}

func TestRecordingFinishWritesAndWarns(t *testing.T) {
	r := newRunRecording(t.TempDir())
	rec := r.NewDevice("Pixel 7")
	ev := runner.StepEvent{File: "missing.probe", TestName: "test", Line: 1, Description: "step", Status: runner.StepStarted, At: time.Now()}
	// A nil client simulates capture failures; they remain advisory.
	_ = rec.BeginStep(nil, ev)
	ev.Status = runner.StepPassed
	_ = rec.EndStep(nil, ev)
	dirs, warnings := r.Finish()
	if len(dirs) != 1 || len(warnings) == 0 {
		t.Fatalf("dirs=%v warnings=%v", dirs, warnings)
	}
	matches, err := filepath.Glob(filepath.Join(dirs[0], "*", "manifest.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("manifest files=%v err=%v", matches, err)
	}
	dirs, warnings = r.Finish()
	if len(dirs) != 0 || len(warnings) != 0 {
		t.Fatalf("second Finish: dirs=%v warnings=%v", dirs, warnings)
	}
}
