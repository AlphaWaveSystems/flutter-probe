package runner

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// A lane whose token is a shared one, on a device where another app holds the agent port and the app under test has opened none, returns the refusal as a connect error
// and never dials: the lane is not run against the holder.
func TestParallelLaneRefusesAPortAnotherAppHoldsAndNeverDials(t *testing.T) {
	r := newReconnectRig(t, 48686, listenRow(48911, 10100), "package:com.first uid:10100\npackage:com.second uid:10200\n", "")
	cfg := config.Default()
	cfg.Agent.DialTimeout = 1 * time.Second
	cfg.Agent.TokenReadTimeout = 3 * time.Second
	cfg.Project.App = "com.second"
	cfg.Agent.DevicePort = 48911 // where the holder listens: the port the agent is configured on, on the device
	var dials atomic.Int32
	po := &ParallelOrchestrator{
		cfg: cfg, manager: r.dc.Manager,
		dial: func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
			dials.Add(1)
			return nil, errors.New("the lane must not dial")
		},
	}
	start := time.Now()
	results, err := po.runOnDevice(context.Background(), &DeviceRun{DeviceID: "emulator-5554", Platform: device.PlatformAndroid, Port: 48686, AppID: "com.second"})
	if results != nil {
		t.Errorf("a refused lane returned results: %v", results)
	}
	var ce *connectError
	var held *device.PortHeldError
	if !errors.As(err, &ce) || !errors.As(err, &held) || held.Holders[0] != "com.first" || held.App != "com.second" {
		t.Fatalf("err = %v, want a connectError wrapping a *PortHeldError that names com.first and com.second", err)
	}
	if !strings.Contains(err.Error(), "[emulator-5554]") {
		t.Errorf("the error should name the device: %v", err)
	}
	if dials.Load() != 0 {
		t.Errorf("the lane dialed %d time(s): the holder could be reached", dials.Load())
	}
	if took := time.Since(start); took < cfg.Agent.DialTimeout {
		t.Errorf("refused after %s, want it only after the dial window (%s)", took, cfg.Agent.DialTimeout)
	}
}

// With a run-as token the lane does not look at the sockets at all and goes on to dial, as before.
func TestParallelLaneWithARunAsTokenDialsAndAsksAdbNothingAboutSockets(t *testing.T) {
	r := newReconnectRig(t, 48686, listenRow(48911, 10100), "package:com.first uid:10100\npackage:com.second uid:10200\n", "run-as-token-0123456789abcdef0123456789")
	cfg := config.Default()
	cfg.Agent.DialTimeout = 1 * time.Second
	cfg.Agent.TokenReadTimeout = 3 * time.Second
	cfg.Agent.DevicePort = 48911
	var dials atomic.Int32
	var callsBeforeTheDial []string
	po := &ParallelOrchestrator{
		cfg: cfg, manager: r.dc.Manager,
		dial: func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
			dials.Add(1)
			callsBeforeTheDial = r.calls(t) // what adb was asked up to the dial; the failure hint that follows a failed dial looks at the sockets, and is not what is measured here
			return nil, errors.New("stop here: the dial was reached")
		},
	}
	_, err := po.runOnDevice(context.Background(), &DeviceRun{DeviceID: "emulator-5554", Platform: device.PlatformAndroid, Port: 48686, AppID: "com.second"})
	var held *device.PortHeldError
	if errors.As(err, &held) {
		t.Fatalf("refused although the token proves the app: %v", err)
	}
	if dials.Load() != 1 {
		t.Errorf("dials = %d, want 1", dials.Load())
	}
	for _, c := range callsBeforeTheDial {
		if strings.Contains(c, "cat /proc/net") || strings.Contains(c, "pm list") {
			t.Errorf("a run-as token needs no look at the sockets or packages before the dial, but adb was asked: %q", c)
		}
	}
	if len(callsBeforeTheDial) == 0 {
		t.Errorf("no adb call was recorded before the dial: the test measured nothing")
	}
}
