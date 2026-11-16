package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func parityConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Project.App = "com.example.app"
	cfg.Agent.Port = 48690
	cfg.Agent.DriverPort = 48691
	cfg.Agent.ReconnectDelay = 3 * time.Second
	cfg.Agent.TokenReadTimeout = 4 * time.Second
	cfg.Agent.DialTimeout = 5 * time.Second
	cfg.Device.RestartDelay = 6 * time.Second
	cfg.Defaults.CrashEvidence = true
	return cfg
}

// A Studio connection's context is what `probe test` builds for the same config, apart from the facts of the connection (the forwarded
// port and, for Wi-Fi, the host). Compared field by field, so a field Studio sets differently, or leaves out, fails.
func TestStudioDeviceContextMatchesTheSharedConstructor(t *testing.T) {
	a := &App{ctx: context.Background()}
	cfg := parityConfig()
	got := newStudioDeviceContext(a, cfg, device.PlatformAndroid, "R58M123ABC", 51234, "192.168.1.20")
	want := runner.NewConfiguredDeviceContext(context.Background(), nil, cfg, device.PlatformAndroid, "R58M123ABC", runner.ConnFacts{AgentHost: "192.168.1.20", Port: 51234, DevicePort: 51234})
	g, w := reflect.ValueOf(got).Elem(), reflect.ValueOf(want).Elem()
	for i := 0; i < g.NumField(); i++ {
		f := g.Type().Field(i)
		if !f.IsExported() || f.Type.Kind() == reflect.Func {
			continue
		}
		if !reflect.DeepEqual(g.Field(i).Interface(), w.Field(i).Interface()) {
			t.Errorf("DeviceContext.%s: Studio gives %v, the shared constructor %v", f.Name, g.Field(i).Interface(), w.Field(i).Interface())
		}
	}
	if got.AppID != "com.example.app" || !got.CrashEvidence || got.RestartDelay != 6*time.Second || got.DriverPort != 48691 || got.AgentHost != "192.168.1.20" || got.Port != 51234 {
		t.Errorf("a Studio run does not read probe.yaml like probe test: %+v", got)
	}
}

// A Studio run does not wipe app data: no window answers yet, so the destructive step is refused, whatever probe.yaml says.
func TestStudioDeviceContextRefusesClearData(t *testing.T) {
	a := &App{ctx: context.Background()}
	cfg := parityConfig()
	cfg.Defaults.GrantPermissionsOnClear = true
	dc := newStudioDeviceContext(a, cfg, device.PlatformAndroid, "R58M123ABC", 51234, "")
	if dc.AllowClearData || dc.Confirm != nil {
		t.Fatalf("destructive steps are on: AllowClearData=%v Confirm=%v", dc.AllowClearData, dc.Confirm != nil)
	}
	err := dc.ClearAppData(context.Background())
	if err == nil || !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("ClearAppData = %v, want a refusal", err)
	}
}
