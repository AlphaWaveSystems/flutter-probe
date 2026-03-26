package device_test

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func TestNewManager(t *testing.T) {
	m := device.NewManager()
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestNewManagerWithPaths(t *testing.T) {
	m := device.NewManagerWithPaths(device.ToolPaths{ADB: "adb"})
	if m == nil {
		t.Fatal("NewManagerWithPaths() returned nil")
	}
}

func TestNewManagerWithPaths_EmptyADB(t *testing.T) {
	// Empty ADB path should default to "adb" from PATH
	m := device.NewManagerWithPaths(device.ToolPaths{})
	if m == nil {
		t.Fatal("NewManagerWithPaths() returned nil")
	}
}

func TestKillStaleIProxy_NoProcesses(t *testing.T) {
	// Should not panic when there are no matching processes
	device.KillStaleIProxy("nonexistent-udid-12345")
}

func TestEnsureADB_InvalidBinary(t *testing.T) {
	m := device.NewManagerWithPaths(device.ToolPaths{ADB: "/nonexistent/adb"})
	err := m.EnsureADB(context.Background(), "fake-serial", 48686)
	if err == nil {
		t.Error("EnsureADB with invalid binary should return error")
	}
}

func TestIsPhysicalAndroid_EmulatorSerial(t *testing.T) {
	m := device.NewManager()
	// Emulator serials always start with "emulator-"
	if m.IsPhysicalAndroid(context.Background(), "emulator-5554") {
		t.Error("emulator-5554 should not be detected as physical")
	}
}

func TestADB_Bin(t *testing.T) {
	adb := device.NewADB()
	if adb.Bin() != "adb" {
		t.Errorf("NewADB().Bin() = %q, want %q", adb.Bin(), "adb")
	}
}

func TestADB_BinWithPath(t *testing.T) {
	adb := device.NewADBWithPath("/custom/path/adb")
	if adb.Bin() != "/custom/path/adb" {
		t.Errorf("NewADBWithPath().Bin() = %q, want %q", adb.Bin(), "/custom/path/adb")
	}
}

func TestADB_BinWithPathEmpty(t *testing.T) {
	adb := device.NewADBWithPath("")
	if adb.Bin() != "adb" {
		t.Errorf("NewADBWithPath(\"\").Bin() = %q, want %q", adb.Bin(), "adb")
	}
}
