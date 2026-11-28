package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

// A simulator that never writes a token is told what to check (its log, not the physical-device tool), and a physical device still gets the device-log path.
func TestReadTokenIOSHintMatchesTheDeviceKind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := &Manager{simctl: ios.New()}
	const simulator = "00000000-0000-0000-0000-000000000003"
	_, err := m.ReadTokenIOS(context.Background(), simulator, 300*time.Millisecond)
	if err == nil || strings.Contains(err.Error(), "idevicesyslog") || !strings.Contains(err.Error(), "xcrun simctl spawn "+simulator+" log") {
		t.Fatalf("simulator error mentions the physical-device log tool or lacks the simulator log hint: %v", err)
	}
	_, err = m.ReadTokenIOS(context.Background(), "00008110-001A2C3E0E12801E", 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "ios physical") {
		t.Fatalf("physical device did not take the device-log path: %v", err)
	}
}

// A simulator with two tokens is refused as such, and the physical-device fallback (the device log) is not tried for it.
func TestReadTokenIOSSurfacesAnAmbiguousSimulator(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const udid = "00000000-0000-0000-0000-000000000002"
	for _, name := range []string{"container-1", "container-2"} {
		dir := filepath.Join(home, "Library", "Developer", "CoreSimulator", "Devices", udid, "data", "Containers", "Data", "Application", name, "tmp", "probe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "token"), []byte(name+"-0123456789abcdef"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manager{simctl: ios.New()}
	_, err := m.ReadTokenIOS(context.Background(), udid, 300*time.Millisecond)
	if !errors.Is(err, ios.ErrAmbiguousToken) {
		t.Fatalf("got %v", err)
	}
}
