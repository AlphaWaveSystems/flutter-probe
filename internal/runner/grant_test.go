package runner

import (
	"context"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func TestApplyGrants_NoopWithoutGrantsDryRunOrDeviceContext(t *testing.T) {
	cfg := &config.Config{}
	cases := map[string]RunOptions{
		"no grants":         {Timeout: time.Second},
		"dry run":           {Timeout: time.Second, DryRun: true, Grant: []string{"camera"}},
		"cloud (no device)": {Timeout: time.Second, Grant: []string{"camera"}},
	}
	for name, opts := range cases {
		r := New(cfg, &fakeAIClient{}, nil, opts)
		if _, err := r.applyGrants(context.Background()); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}

// FP-16: on an iOS simulator `--grant notifications` cannot pre-grant, so it
// starts a watcher that taps Allow when the system alert appears.
func TestGrantNotifications_IOSWatcherTapsAllow(t *testing.T) {
	f := &fakeSys{dialog: &sysdialog.Dialog{Title: "“App” Would Like to Send You Notifications", Texts: []string{"“App” Would Like to Send You Notifications"}, Buttons: []string{"Don’t Allow", "Allow"}}}
	dc := &DeviceContext{Platform: device.PlatformIOS, sysDriver: f}
	r := New(&config.Config{}, &fakeAIClient{}, dc, RunOptions{Timeout: time.Second, Grant: []string{"notifications"}})

	stop, err := r.applyGrants(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.tapped)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop() // joins the watcher goroutine
	if len(f.tapped) == 0 || f.tapped[0] != "Allow" {
		t.Errorf("the watcher should have tapped Allow, tapped = %v", f.tapped)
	}
}
