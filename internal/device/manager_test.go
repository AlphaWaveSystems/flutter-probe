package device_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestReadTokenAndroid_NilTracerDoesNotPanic(t *testing.T) {
	m := device.NewManagerWithPaths(device.ToolPaths{ADB: "/nonexistent/adb"})
	// Should not panic with a nil Tracer, even though every adb shell call
	// below fails immediately (invalid binary) and each failure logs via trace.
	_, err := m.ReadTokenAndroid(context.Background(), "fake-serial", 150*time.Millisecond, "", nil)
	if err == nil {
		t.Error("expected an error with an invalid adb binary")
	}
}

func TestReadTokenAndroid_TracerReceivesMessages(t *testing.T) {
	m := device.NewManagerWithPaths(device.ToolPaths{ADB: "/nonexistent/adb"})
	var mu sync.Mutex
	var lines []string
	trace := func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	_, _ = m.ReadTokenAndroid(context.Background(), "fake-serial", 150*time.Millisecond, "", trace)

	mu.Lock()
	defer mu.Unlock()
	if len(lines) == 0 {
		t.Error("expected at least one trace line to be emitted while attempting token reads")
	}
}

func TestReadTokenAndroid_SkipsRunAsSourceWithoutAppID(t *testing.T) {
	m := device.NewManagerWithPaths(device.ToolPaths{ADB: "/nonexistent/adb"})
	var mu sync.Mutex
	var lines []string
	trace := func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	_, _ = m.ReadTokenAndroid(context.Background(), "fake-serial", 150*time.Millisecond, "", trace)

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, l := range lines {
		if strings.Contains(l, "skipping run-as source") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a trace line noting the run-as source was skipped (empty appID), got: %v", lines)
	}
}
