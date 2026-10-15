package mcp

import (
	"reflect"
	"testing"
)

func TestSetDeviceSettingArgs(t *testing.T) {
	got := setDeviceSettingArgs(map[string]string{"device": "emulator-5554", "setting": "locale", "value": "de-DE", "app": "com.example.app"})
	want := []string{"device", "set", "--device", "emulator-5554", "--app", "com.example.app", "--", "locale", "de-DE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	// A value that looks like a flag stays after "--".
	got = setDeviceSettingArgs(map[string]string{"device": "d1", "setting": "open_url", "value": "--help"})
	want = []string{"device", "set", "--device", "d1", "--", "open_url", "--help"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}
