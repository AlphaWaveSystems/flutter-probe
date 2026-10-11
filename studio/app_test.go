package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// ---- isDeviceReady ----------------------------------------------------

func TestIsDeviceReady(t *testing.T) {
	cases := []struct {
		state string
		want  bool
	}{
		{"booted", true},
		{"BOOTED", true}, // case-insensitive
		{"online", true},
		{"device", true},
		{"shutdown", false},
		{"offline", false},
		{"unauthorized", false},
		{"", false},
	}
	for _, c := range cases {
		d := device.Device{State: c.state}
		if got := isDeviceReady(d); got != c.want {
			t.Errorf("isDeviceReady(state=%q) = %v, want %v", c.state, got, c.want)
		}
	}
}

// ---- extractLineCol -----------------------------------------------------

func TestExtractLineCol(t *testing.T) {
	cases := []struct {
		msg      string
		wantLine int
		wantCol  int
	}{
		{"parse error at line 3 column 7: unexpected token", 3, 7},
		{"no position info here", 1, 1},
		{"line 12 only, no column", 12, 1},
	}
	for _, c := range cases {
		line, col := extractLineCol(c.msg)
		if line != c.wantLine || col != c.wantCol {
			t.Errorf("extractLineCol(%q) = (%d, %d), want (%d, %d)", c.msg, line, col, c.wantLine, c.wantCol)
		}
	}
}

// ---- App.Status / activeClient / Disconnect (no connection) -------------

func TestApp_Status_Disconnected(t *testing.T) {
	a := NewApp()
	status := a.Status()
	if status.Connected {
		t.Error("Status().Connected should be false with no connection")
	}
	if status.DeviceID != "" || status.DeviceName != "" || status.Platform != "" {
		t.Errorf("Status() should be zero-valued when disconnected, got %+v", status)
	}
}

func TestApp_ActiveClient_NotConnected(t *testing.T) {
	a := NewApp()
	_, err := a.activeClient()
	if err == nil {
		t.Fatal("activeClient() should error when not connected")
	}
}

func TestApp_Disconnect_NoOpWhenNotConnected(t *testing.T) {
	a := NewApp()
	// Deliberately leave a.ctx unset (nil): Disconnect() guards its
	// wails runtime EventsEmit call on a.ctx != nil specifically so it's
	// safe to call outside of a running Wails app, e.g. from a test.
	a.Disconnect()
	if a.Status().Connected {
		t.Error("expected disconnected status after Disconnect() with no prior connection")
	}
}

// ---- Connect / ConnectWiFi input validation ------------------------------

// Connect() with an unknown device ID exercises the same device.Manager.List
// path the CLI uses, then must fail with a clear "not found" error rather
// than attempting to dial. device.Manager.List swallows tool-missing errors
// (adb/simctl/idevice_id) internally, so this is safe to run without any
// device tooling installed.
func TestApp_Connect_UnknownDevice(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()
	_, err := a.Connect("definitely-not-a-real-device-id")
	if err == nil {
		t.Fatal("Connect() with an unknown device ID should return an error")
	}
}

func TestApp_Connect_EmptyDeviceID(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()
	_, err := a.Connect("")
	if err == nil {
		t.Fatal("Connect(\"\") should return an error")
	}
}

// ConnectWiFi requires host, port, and token since — unlike USB/adb-visible
// devices — a WiFi-mode agent isn't autodetected; the caller must supply
// everything (mirrors the CLI's --host/--token flags).
func TestApp_ConnectWiFi_RequiresAllParams(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()

	cases := []struct {
		name  string
		host  string
		port  int
		token string
	}{
		{"missing host", "", 8787, "tok"},
		{"missing port", "192.168.1.5", 0, "tok"},
		{"missing token", "192.168.1.5", 8787, ""},
		{"all missing", "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := a.ConnectWiFi(c.host, c.port, c.token)
			if err == nil {
				t.Errorf("ConnectWiFi(%q, %d, %q) should require host+port+token", c.host, c.port, c.token)
			}
		})
	}
}

// ---- File I/O guards ------------------------------------------------------

func TestApp_ReadFile_RejectsNonProbeExtension(t *testing.T) {
	a := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReadFile(path); err == nil {
		t.Error("ReadFile should reject non-.probe files")
	}
}

func TestApp_ReadFile_RejectsPathTraversal(t *testing.T) {
	a := NewApp()
	if _, err := a.ReadFile("../../etc/passwd.probe"); err == nil {
		t.Error("ReadFile should reject paths containing ..")
	}
}

func TestApp_WriteFile_RejectsNonProbeExtension(t *testing.T) {
	a := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := a.WriteFile(path, "content"); err == nil {
		t.Error("WriteFile should reject non-.probe files")
	}
}

func TestApp_WriteFile_RejectsPathTraversal(t *testing.T) {
	a := NewApp()
	if err := a.WriteFile("../../tmp/evil.probe", "content"); err == nil {
		t.Error("WriteFile should reject paths containing ..")
	}
}

func TestApp_WriteFile_ReadFile_RoundTrip(t *testing.T) {
	a := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.probe")
	content := "test \"round trip\"\n  wait 1 seconds\n"

	if err := a.WriteFile(path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := a.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != content {
		t.Errorf("ReadFile roundtrip = %q, want %q", got, content)
	}
}

// ---- Lint ------------------------------------------------------------

func TestApp_Lint_ParseError(t *testing.T) {
	a := NewApp()
	// An unterminated quoted string is a reliable lexer/parser error,
	// matching the pattern used by internal/parser's own error-path tests.
	diags := a.Lint("test \"unterminated\n  tap \"broken\n")
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for invalid ProbeScript")
	}
	if diags[0].Severity != 1 {
		t.Errorf("parse error diagnostic severity = %d, want 1 (error)", diags[0].Severity)
	}
}

func TestApp_Lint_EmptyTestWarning(t *testing.T) {
	a := NewApp()
	diags := a.Lint("test \"empty test\"\n")
	found := false
	for _, d := range diags {
		if d.Severity == 4 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a severity=4 warning for a test with no steps, got %+v", diags)
	}
}

func TestApp_Lint_ValidScript(t *testing.T) {
	a := NewApp()
	diags := a.Lint("test \"valid\"\n  wait 1 seconds\n")
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for a valid script with steps, got %+v", diags)
	}
}

// ---- ListDir ------------------------------------------------------------

func TestApp_ListDir_MissingDirReturnsEmpty(t *testing.T) {
	a := NewApp()
	entries, err := a.ListDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("ListDir on missing dir should not error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ListDir on missing dir should be empty, got %v", entries)
	}
}

func TestApp_ListDir_FiltersHiddenEntries(t *testing.T) {
	a := NewApp()
	dir := t.TempDir()
	for _, name := range []string{"visible.probe", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := a.ListDir(dir)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "visible.probe" {
		t.Errorf("ListDir should filter dot-prefixed entries, got %+v", entries)
	}
}
