package ios

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Every token path is built on the simulator's data folder: with an id that is not a simulator UDID there is none, the legacy
// device-level file included.
func TestTokenFileCandidatesNeedASimulatorUDID(t *testing.T) {
	fakeXcrun(t)
	s := New()
	for _, u := range append([]string{"00008110-001A2C3E0E12801E"}, hostileUDIDs...) {
		if paths, globbed := s.tokenFileCandidates(context.Background(), u, ""); len(paths) != 0 || len(globbed) != 0 {
			t.Errorf("%q: token paths %q, want none", u, paths)
		}
	}
	const sim = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"
	paths, _ := s.tokenFileCandidates(context.Background(), sim, "")
	if len(paths) == 0 || !strings.HasSuffix(paths[len(paths)-1], "/Library/Developer/CoreSimulator/Devices/"+sim+"/data/tmp/probe/token") {
		t.Errorf("a simulator's token paths must end with the legacy file: %q", paths)
	}
}

// ReadToken with an id that is not a simulator UDID (hostile, or a physical device's) fails at once instead of polling until the timeout.
func TestReadTokenFailsFastForAnIDThatIsNotASimulator(t *testing.T) {
	fakeXcrun(t)
	for _, u := range []string{"../../x", "00008110-001A2C3E0E12801E", ""} {
		start := time.Now()
		_, err := New().ReadToken(context.Background(), u, 5*time.Second)
		if err == nil || !strings.Contains(err.Error(), "is not a simulator UDID") {
			t.Errorf("%q: want the refusal, got %v", u, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%q: ReadToken took %v; it must fail at once", u, d)
		}
	}
}

// Terminate cannot look up the app's process for a refused id: it says so instead of reporting "not running", and runs nothing.
func TestTerminateReportsALookupFailure(t *testing.T) {
	logPath := fakeXcrun(t)
	err := NewDeviceCtl().Terminate(context.Background(), "../../x", "com.example.app")
	if err == nil || !strings.Contains(err.Error(), "could not be looked up") || !strings.Contains(err.Error(), "is not a device id") {
		t.Errorf("want the lookup failure, got %v", err)
	}
	if b, _ := os.ReadFile(logPath); len(b) != 0 {
		t.Errorf("xcrun ran for a refused id: %q", b)
	}
}
