package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// idAgent is a lane's fake agent that says which app it runs in.
type idAgent struct {
	*harnessAgent
	appID string
}

func (a *idAgent) Handshake(ctx context.Context, clientVersion string) (*probelink.HandshakeResult, error) {
	return &probelink.HandshakeResult{AgentVersion: clientVersion, AppID: a.appID}, nil
}

// A lane that finds another app's agent says so, once, with both ids, and still runs: it is a warning, not a refusal.
func TestLaneWarnsWhenTheAgentReportsAnotherApp(t *testing.T) {
	dir := t.TempDir()
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine\"\n  tap \"Fine\"\n")
	for _, tc := range []struct {
		name, reported string
		wantWarn       bool
	}{
		{"another app", "com.other.app", true},
		{"a difference of case alone is another app on an Android lane", "com.Example.app", true},
		{"the same app", "com.example.app", false},
		{"the agent does not say", "", false},
	} {
		h := newHarnessRun(t, harnessConfig(), [][]string{{fine}})
		base := h.po.dial
		h.po.dial = func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
			c, err := base(ctx, opts)
			if err != nil {
				return nil, err
			}
			return &idAgent{harnessAgent: c.(*harnessAgent), appID: tc.reported}, nil
		}
		var runErr error
		out := captureStdout(t, func() { _, runErr = h.po.Run(context.Background()) })
		if runErr != nil {
			t.Errorf("%s: the lane must still run: %v", tc.name, runErr)
		}
		has := strings.Contains(out, "the agent that answered reports app "+tc.reported+", but project.app is com.example.app")
		if has != tc.wantWarn {
			t.Errorf("%s: warning printed = %v, want %v\n%s", tc.name, has, tc.wantWarn, out)
		}
		if tc.wantWarn && strings.Count(out, "reports app") != 1 {
			t.Errorf("%s: the warning must be printed once:\n%s", tc.name, out)
		}
		if n := countOf(h.taps(0), "Fine"); n != 1 {
			t.Errorf("%s: the test ran %d times, want 1", tc.name, n)
		}
	}
}

// The handshake after a reconnect says the same, and compares an iOS bundle id without regard to case (an Android package name is case-sensitive).
func TestReconnectHandshakeWarnsWhenTheAgentReportsAnotherApp(t *testing.T) {
	for _, tc := range []struct {
		name, reported, appID string
		platform              device.Platform
		wantWarn              bool
	}{
		{"another app", "com.other.app", "com.example.app", device.PlatformAndroid, true},
		{"the same app", "com.example.app", "com.example.app", device.PlatformAndroid, false},
		{"the agent does not say", "", "com.example.app", device.PlatformAndroid, false},
		{"the context has no app id", "com.other.app", "", device.PlatformAndroid, false},
		{"android: a difference of case is another app", "com.Example.app", "com.example.app", device.PlatformAndroid, true},
		{"ios: a difference of case says nothing", "com.Example.app", "com.example.app", device.PlatformIOS, false},
		{"ios: another app is said", "com.other.app", "com.example.app", device.PlatformIOS, true},
	} {
		dc := &DeviceContext{AppID: tc.appID, CLIVersion: harnessCLIVersion, Platform: tc.platform}
		client := &idAgent{harnessAgent: &harnessAgent{slowClient: newSlowClient()}, appID: tc.reported}
		var err error
		out := captureStdout(t, func() { err = dc.handshakeNewClient(context.Background(), client) })
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if has := strings.Contains(out, "reports app "+tc.reported); has != tc.wantWarn {
			t.Errorf("%s: warning printed = %v, want %v\n%s", tc.name, has, tc.wantWarn, out)
		}
	}
}
