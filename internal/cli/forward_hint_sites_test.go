package cli

import (
	"os"
	"strings"
	"testing"
)

// The dial sites of an Android run name a forward to another device through device.ForwardHint, and none of them builds that text itself. (The sites need a
// device to run, so this reads the source, as the reconnect paths are checked.) The parallel orchestrator is checked from internal/runner.
func TestAndroidDialSitesUseTheSharedForwardHint(t *testing.T) {
	for file, wantCalls := range map[string]int{"test.go": 1, "composite.go": 1} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if got := strings.Count(src, "device.ForwardHint("); got != wantCalls {
			t.Errorf("%s: %d calls of device.ForwardHint, want %d", file, got, wantCalls)
		}
		if strings.Contains(src, "is forwarded to another device") || strings.Contains(src, "an adb port forward") {
			t.Errorf("%s builds a forward message itself", file)
		}
	}
}
