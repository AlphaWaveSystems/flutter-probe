package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// fakeXcrunList puts an xcrun stand-in first on PATH. It answers `simctl list devices --json` with listJSON, and every other
// simctl command succeeds without output, so no simulator is needed.
func fakeXcrunList(t *testing.T, listJSON string) {
	t.Helper()
	dir := t.TempDir()
	listPath := filepath.Join(dir, "list.json")
	if err := os.WriteFile(listPath, []byte(listJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n  *\"list devices\"*) cat '" + listPath + "' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// singleDeviceCases are the devices a single-device run can be given, with what its device context must say. A platform the
// run does not know, which is "both" when --device names no listed device, counts as physical.
var singleDeviceCases = []struct {
	name     string
	platform device.Platform
	serial   string
	hardware string // what the fake adb answers for ro.hardware
	physical bool
}{
	{name: "emulator", platform: device.PlatformAndroid, serial: "emulator-5554", hardware: "ranchu", physical: false},
	{name: "USB device", platform: device.PlatformAndroid, serial: "R58M123ABC", hardware: "qcom", physical: true},
	{name: "unknown platform", platform: device.Platform("both"), serial: "R58M123ABC", hardware: "ranchu", physical: true},
}

// TestSingleDeviceContextAsksTheDevice checks the physical question of a single-device run: it is asked of the device, and
// a physical device takes the HTTP fallback.
func TestSingleDeviceContextAsksTheDevice(t *testing.T) {
	ctx := context.Background()
	for _, tc := range singleDeviceCases {
		t.Run(tc.name, func(t *testing.T) {
			dm := compositeFakeADB(t, tc.hardware)
			dc := singleDeviceContext(ctx, dm, &config.Config{}, tc.platform, tc.serial, "", false, false, nil)
			if dc.IsPhysical != tc.physical {
				t.Errorf("IsPhysical = %v, want %v", dc.IsPhysical, tc.physical)
			}
			if dc.UseHTTP != tc.physical {
				t.Errorf("UseHTTP = %v, want %v: a physical device takes the HTTP fallback", dc.UseHTTP, tc.physical)
			}
		})
	}
}

// TestSingleDeviceContextTakesTheRunFlags checks that the config and flags of the run reach the single-device context, and
// that a clear of the app's data grants its permissions unless --no-grant-on-clear turns that off.
func TestSingleDeviceContextTakesTheRunFlags(t *testing.T) {
	ctx := context.Background()
	dm := compositeFakeADB(t, "qcom")
	confirm := func(string) bool { return true }
	for _, tc := range []struct {
		name           string
		autoYes        bool
		noGrantOnClear bool
		cfgGrant       bool
		wantGrant      bool
	}{
		{name: "no flags", wantGrant: false},
		{name: "--yes grants on clear", autoYes: true, wantGrant: true},
		{name: "--yes with --no-grant-on-clear", autoYes: true, noGrantOnClear: true, wantGrant: false},
		{name: "probe.yaml grants on clear", cfgGrant: true, wantGrant: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Project.App = "com.example.app"
			cfg.Agent.Port = 48690
			cfg.Agent.DriverPort = 48691
			cfg.Device.RestartDelay = 3 * time.Second
			cfg.Defaults.GrantPermissionsOnClear = tc.cfgGrant
			dc := singleDeviceContext(ctx, dm, cfg, device.PlatformAndroid, "R58M123ABC", "192.168.1.20", tc.autoYes, tc.noGrantOnClear, confirm)
			if dc.AppID != "com.example.app" || dc.Port != 48690 || dc.DriverPort != 48691 || dc.DevicePort != cfg.Agent.AgentDevicePort() {
				t.Errorf("AppID, Port, DriverPort, DevicePort = %q, %d, %d, %d; want com.example.app, 48690, 48691, %d",
					dc.AppID, dc.Port, dc.DriverPort, dc.DevicePort, cfg.Agent.AgentDevicePort())
			}
			if dc.AgentHost != "192.168.1.20" || dc.RestartDelay != 3*time.Second || dc.AllowClearData != tc.autoYes || dc.Confirm == nil {
				t.Errorf("AgentHost, RestartDelay, AllowClearData, Confirm = %q, %v, %v, %v; want the run's host, 3s, %v, a callback",
					dc.AgentHost, dc.RestartDelay, dc.AllowClearData, dc.Confirm == nil, tc.autoYes)
			}
			if dc.GrantPermissionsOnClear != tc.wantGrant {
				t.Errorf("GrantPermissionsOnClear = %v, want %v", dc.GrantPermissionsOnClear, tc.wantGrant)
			}
		})
	}
}

// TestLocaleDeviceContextAsksTheDevice checks the device context of the --locale step: IsPhysical is asked of the device, so
// SetLanguage refuses a physical iPhone and takes a simulator, and the app ID and restart delay reach the context.
func TestLocaleDeviceContextAsksTheDevice(t *testing.T) {
	ctx := context.Background()
	const simUDID = "AAAAAAAA-1111-2222-3333-444444444444"
	const physUDID = "BBBBBBBB-1111-2222-3333-444444444444"
	simList := `{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-26-4":[{"udid":"` + simUDID +
		`","name":"sim","state":"Shutdown","deviceTypeIdentifier":"x"}]}}`
	for _, tc := range []struct {
		name     string
		platform device.Platform
		serial   string
		hardware string // what the fake adb answers for ro.hardware, for an Android device
		list     string // what simctl lists, for an iOS device
		physical bool
	}{
		{name: "Android emulator", platform: device.PlatformAndroid, serial: "emulator-5554", hardware: "ranchu", physical: false},
		{name: "Android USB device", platform: device.PlatformAndroid, serial: "R58M123ABC", hardware: "qcom", physical: true},
		{name: "physical iPhone", platform: device.PlatformIOS, serial: physUDID, list: `{"devices":{}}`, physical: true},
		{name: "simulator", platform: device.PlatformIOS, serial: simUDID, list: simList, physical: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.platform == device.PlatformIOS {
				fakeXcrunList(t, tc.list)
			}
			cfg := &config.Config{}
			cfg.Project.App = "com.example.app"
			cfg.Device.RestartDelay = 2 * time.Second
			lc := localeDeviceContext(ctx, compositeFakeADB(t, tc.hardware), cfg, tc.platform, tc.serial)
			if lc.IsPhysical != tc.physical {
				t.Errorf("IsPhysical = %v, want %v", lc.IsPhysical, tc.physical)
			}
			if lc.AppID != "com.example.app" || lc.RestartDelay != 2*time.Second {
				t.Errorf("AppID, RestartDelay = %q, %v; want com.example.app, 2s", lc.AppID, lc.RestartDelay)
			}
			if tc.platform != device.PlatformIOS {
				return
			}
			err := lc.SetLanguage(ctx, "de")
			if tc.physical {
				if err == nil || !strings.Contains(err.Error(), "physical iOS") {
					t.Errorf("SetLanguage on a physical iPhone: err = %v, want the physical iOS refusal", err)
				}
				return
			}
			if err != nil {
				t.Errorf("SetLanguage on a simulator: %v", err)
			}
		})
	}
}

// TestSingleDeviceContextIsTheSharedConstructorOutput checks, field by field with a populated config, that the CLI wrapper adds nothing of
// its own and drops nothing: its context equals the shared constructor's for the same facts. A field the wrapper overrides differently fails.
func TestSingleDeviceContextIsTheSharedConstructorOutput(t *testing.T) {
	ctx := context.Background()
	dm := compositeFakeADB(t, "qcom")
	cfg := &config.Config{}
	cfg.Project.App = "com.example.app"
	cfg.Agent.Port = 48690
	cfg.Agent.DriverPort = 48691
	cfg.Agent.ReconnectDelay = 3 * time.Second
	cfg.Agent.TokenReadTimeout = 4 * time.Second
	cfg.Agent.DialTimeout = 5 * time.Second
	cfg.Device.RestartDelay = 6 * time.Second
	cfg.Defaults.CrashEvidence = true
	got := singleDeviceContext(ctx, dm, cfg, device.PlatformAndroid, "R58M123ABC", "192.168.1.20", true, false, nil)
	want := runner.NewConfiguredDeviceContext(ctx, dm, cfg, device.PlatformAndroid, "R58M123ABC", runner.ConnFacts{AgentHost: "192.168.1.20", AllowClearData: true})
	g, w := reflect.ValueOf(got).Elem(), reflect.ValueOf(want).Elem()
	for i := 0; i < g.NumField(); i++ {
		f := g.Type().Field(i)
		if !f.IsExported() || f.Type.Kind() == reflect.Func {
			continue
		}
		if !reflect.DeepEqual(g.Field(i).Interface(), w.Field(i).Interface()) {
			t.Errorf("DeviceContext.%s: the CLI wrapper gives %v, the shared constructor %v", f.Name, g.Field(i).Interface(), w.Field(i).Interface())
		}
	}
}
