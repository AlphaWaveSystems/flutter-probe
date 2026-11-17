package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// On a physical iPhone, a kill whose process lookup fails says so in one line instead of passing silently: the app may still run.
func TestKillOnAPhysicalIPhoneWarnsWhenTheLookupFails(t *testing.T) {
	dc := &DeviceContext{Manager: device.NewManager(), Platform: device.PlatformIOS, IsPhysical: true, Serial: "../../x", AppID: "com.example.app"}
	out := captureStdout(t, func() {
		if err := dc.KillApp(context.Background()); err != nil {
			t.Errorf("KillApp: %v", err)
		}
	})
	if !strings.Contains(out, "the app was not stopped") || !strings.Contains(out, "is not a device id") {
		t.Errorf("want the warning, got %q", out)
	}
}
