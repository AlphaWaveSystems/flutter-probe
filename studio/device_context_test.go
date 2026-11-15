package main

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// A Studio connection builds its device context through runner.NewDeviceContext: the manager, serial and platform are set, the agent ports are the connection's, and the physical question was asked (a context with no
// manager cannot answer, so the device counts as physical, as the helper documents).
func TestStudioDeviceContextComesFromTheHelper(t *testing.T) {
	a := &App{ctx: context.Background()}
	dc := newStudioDeviceContext(a, device.PlatformIOS, "192.0.2.7:48686", 48686)
	if dc.Serial != "192.0.2.7:48686" || dc.Platform != device.PlatformIOS || dc.Port != 48686 || dc.DevicePort != 48686 {
		t.Fatalf("context = %+v", dc)
	}
	if !dc.IsPhysical {
		t.Fatal("a context built without a manager must count as physical (the helper's rule), not as an emulator")
	}
}
