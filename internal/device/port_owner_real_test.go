package device

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

// An opt-in check against a real device or emulator: two apps are installed, the first holds the port and the second lost it. Set
//
//	PROBE_PORTOWNER_SERIAL=<serial>  PROBE_PORTOWNER_PORT=<port on the device>  PROBE_PORTOWNER_LOSER=<package of the app that lost the port>
//	PROBE_PORTOWNER_HOLDER=<package that holds it>
//
// The captures in testdata/portowner were made this way; the unit tests use them, so this one only proves the real adb path.
func TestPortOwnersOnARealDevice(t *testing.T) {
	serial, loser, holder := os.Getenv("PROBE_PORTOWNER_SERIAL"), os.Getenv("PROBE_PORTOWNER_LOSER"), os.Getenv("PROBE_PORTOWNER_HOLDER")
	port, _ := strconv.Atoi(os.Getenv("PROBE_PORTOWNER_PORT"))
	if serial == "" || loser == "" || holder == "" || port == 0 {
		t.Skip("set PROBE_PORTOWNER_SERIAL, _PORT, _LOSER and _HOLDER to run against a device")
	}
	adb := NewADB()
	got := PortOwners(context.Background(), adb, serial, port, loser)
	t.Logf("owners of %d on %s as seen by %s: %v", port, serial, loser, got)
	if len(got) != 1 || got[0] != holder {
		t.Errorf("owners = %v, want [%s]", got, holder)
	}
	if h := PortOwnerHint(context.Background(), adb, serial, port, loser); !strings.Contains(h, "held by "+holder) {
		t.Errorf("hint = %q", h)
	}
	// the holder itself is told nothing
	if got := PortOwners(context.Background(), adb, serial, port, holder); len(got) != 0 {
		t.Errorf("the holder is not 'another app': %v", got)
	}
}
