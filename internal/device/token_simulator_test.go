package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

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
