package runner

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func populatedConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Project.App = "com.example.app"
	cfg.Agent.Port = 48690
	cfg.Agent.DevicePort = 48700
	cfg.Agent.DriverPort = 48691
	cfg.Agent.ReconnectDelay = 3 * time.Second
	cfg.Agent.TokenReadTimeout = 4 * time.Second
	cfg.Agent.DialTimeout = 5 * time.Second
	cfg.Device.RestartDelay = 6 * time.Second
	cfg.Defaults.CrashEvidence = true
	cfg.Defaults.GrantPermissionsOnClear = true
	return cfg
}

// Fields of DeviceContext that NewConfiguredDeviceContext does not set from probe.yaml, each with its reason. A new exported field is
// in neither list until someone decides: TestConfiguredContextSetsEveryField fails until it is set by the constructor or listed here.
var notSetByConstructor = map[string]string{
	"Manager":     "nil in the test: the caller's device manager, passed in and set by NewDeviceContext",
	"CLIVersion":  "set by the run that knows the binary's version",
	"Language":    "set by `set language` / --locale during the run",
	"CrashADB":    "a test fake",
	"CrashPoll":   "a test fake",
	"WebViewADB":  "a test fake",
	"WebViewOpen": "a test fake",
	"SettingsRun": "a test fake",
	"NoPrompt":    "a lane of a parallel or composite run: ApplyClearGate sets it; a single-device context never does",
}

func TestConfiguredContextSetsEveryField(t *testing.T) {
	confirm := func(string) bool { return true }
	dc := NewConfiguredDeviceContext(context.Background(), nil, populatedConfig(), device.PlatformAndroid, "serial-1", ConnFacts{
		AgentHost: "192.168.1.20", Port: 0, DevicePort: 0, AllowClearData: true, Confirm: confirm,
	})
	v := reflect.ValueOf(dc).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if !f.IsExported() {
			continue
		}
		if _, listed := notSetByConstructor[f.Name]; listed {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("DeviceContext.%s is not set by NewConfiguredDeviceContext from a fully populated config; set it there (so the CLI and Studio both get it) or list it in notSetByConstructor with a reason", f.Name)
		}
	}
	for name := range notSetByConstructor {
		if _, ok := v.Type().FieldByName(name); !ok {
			t.Errorf("notSetByConstructor lists %s, which is not a field of DeviceContext", name)
		}
	}
}

func TestConfiguredContextValues(t *testing.T) {
	cfg := populatedConfig()
	dc := NewConfiguredDeviceContext(context.Background(), nil, cfg, device.PlatformAndroid, "s", ConnFacts{AgentHost: "10.0.0.2"})
	if dc.AppID != "com.example.app" || dc.Port != 48690 || dc.DevicePort != 48700 || dc.DriverPort != 48691 || dc.AgentHost != "10.0.0.2" {
		t.Errorf("ids and ports: %+v", dc)
	}
	if dc.ReconnectDelay != 3*time.Second || dc.TokenReadTimeout != 4*time.Second || dc.DialTimeout != 5*time.Second || dc.RestartDelay != 6*time.Second || !dc.CrashEvidence {
		t.Errorf("timings and crash evidence: %+v", dc)
	}
	if !dc.UseHTTP {
		t.Error("a device the manager cannot vouch for counts as physical, and a physical device takes the HTTP fallback")
	}
	over := NewConfiguredDeviceContext(context.Background(), nil, cfg, device.PlatformAndroid, "s", ConnFacts{Port: 1111, DevicePort: 2222})
	if over.Port != 1111 || over.DevicePort != 2222 {
		t.Errorf("a port in the facts wins over the config: %d, %d", over.Port, over.DevicePort)
	}
	if dc.AllowClearData || dc.Confirm != nil {
		t.Error("destructive steps are off unless the caller says so")
	}
}

func TestConnFactsStringHoldsNoCallbackOrSecret(t *testing.T) {
	f := ConnFacts{AgentHost: "10.0.0.2", Port: 1, Confirm: func(string) bool { return true }}
	s := f.String()
	if strings.Contains(s, "0x") || strings.Contains(s, "func") {
		t.Errorf("String shows the callback itself: %s", s)
	}
	if !strings.Contains(s, "confirm=true") || !strings.Contains(s, "10.0.0.2") {
		t.Errorf("String = %s", s)
	}
	if none := (ConnFacts{}).String(); !strings.Contains(none, "confirm=false") {
		t.Errorf("String without a callback = %s", none)
	}
}
