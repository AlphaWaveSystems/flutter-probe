package cli

import "testing"

func TestCompositeFlagsReplaceConfiguredDevices(t *testing.T) {
	cfg := map[string]string{"A": "udid-a", "B": "udid-b"}
	got := parseCompositeDeviceSpecs([]string{"Android=emulator-5554", "iOS=127.0.0.1:48686/tok"}, cfg)
	if len(got) != 2 || got["Android"] != "emulator-5554" || got["iOS"] != "127.0.0.1:48686/tok" {
		t.Errorf("flags must replace the configured devices, got %v", got)
	}
	if _, ok := got["A"]; ok {
		t.Errorf("configured device A must not leak in when flags are given: %v", got)
	}
	got = parseCompositeDeviceSpecs(nil, cfg)
	if len(got) != 2 || got["A"] != "udid-a" {
		t.Errorf("without flags the configured devices are used, got %v", got)
	}
}
