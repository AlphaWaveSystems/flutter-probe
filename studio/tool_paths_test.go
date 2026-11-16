package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The device manager uses the adb that probe.yaml names (tools.adb), as `probe test` does, so a Studio with no shell PATH can still list devices.
func TestStudioUsesTheAdbThatProbeYamlNames(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "adb.log")
	script := filepath.Join(dir, "my-adb")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\nif [ \"$1\" = devices ]; then echo 'List of devices attached'; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("project:\n  app: com.example.app\ntools:\n  adb: "+script+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(ws)
	if got := a.mgr().ADB().Bin(); got != script {
		t.Fatalf("adb = %q, want %q", got, script)
	}
	if note := a.DeviceListNote(); note != "" {
		t.Errorf("adb is found at the named path, but the note says %q", note)
	}
	_, _ = a.ListDevices()
	if out, _ := os.ReadFile(log); !strings.Contains(string(out), "devices") {
		t.Errorf("the named adb was not asked for the devices (log %q)", out)
	}
	// another workspace without tools.adb goes back to the default
	other := t.TempDir()
	a.SetWorkspace(other)
	if got := a.mgr().ADB().Bin(); got == script {
		t.Errorf("the adb of the previous workspace is still in use: %q", got)
	}
}

// With a device connected the manager is left alone: the connection keeps the one it was made with.
func TestToolPathsAreNotSwappedUnderAConnection(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("tools:\n  adb: /somewhere/adb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	before := a.mgr()
	a.conn = &connection{}
	a.SetWorkspace(ws)
	if a.mgr() != before {
		t.Error("the device manager was replaced while a device is connected")
	}
}

func TestDeviceListNoteNamesAMissingAdb(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("tools:\n  adb: "+filepath.Join(ws, "no-such-adb")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(ws)
	note := a.DeviceListNote()
	if !strings.Contains(note, "adb was not found") || !strings.Contains(note, "tools.adb in probe.yaml") || !strings.Contains(note, "PATH") {
		t.Errorf("note = %q", note)
	}
	if strings.Contains(note, ws) {
		t.Errorf("the note shows a path: %q", note)
	}
}

// A probe.yaml that cannot be read changes nothing about the tools: Connect and runs refuse with the reason, and no other config is read.
func TestAnUnreadableProbeYamlLeavesTheToolPathsAlone(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("tools: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	before := a.mgr()
	a.SetWorkspace(ws)
	if a.mgr() != before {
		t.Error("the device manager was replaced on a probe.yaml that cannot be read")
	}
}
