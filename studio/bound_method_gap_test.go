package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// Tests for the bound methods that no other test called (bound_methods.json lists the tests of every method; bound_methods_tests_test.go checks it).

func TestInitialWorkspaceIsTheEnvironmentVariable(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	if got := (&App{}).InitialWorkspace(); got != "" {
		t.Errorf("unset: %q", got)
	}
	t.Setenv("PROBE_STUDIO_WORKSPACE", "/some/workspace")
	if got := (&App{}).InitialWorkspace(); got != "/some/workspace" {
		t.Errorf("set: %q", got)
	}
}

// PickWorkspace returns what the person chose and changes nothing: choosing which folder is open is SetWorkspace's job, after the page has it.
func TestPickWorkspaceReturnsTheChoiceAndOpensNothing(t *testing.T) {
	a := &App{}
	a.pickDirFn = func(title string) (string, error) {
		if title != "Open workspace" {
			t.Errorf("title = %q", title)
		}
		return "/chosen/folder", nil
	}
	got, err := a.PickWorkspace()
	if err != nil || got != "/chosen/folder" {
		t.Fatalf("%q, %v", got, err)
	}
	if a.workspace != "" || a.humanWorkspace != "" {
		t.Errorf("picking opened a workspace: %q %q", a.workspace, a.humanWorkspace)
	}
	boom := errors.New("dialog failed")
	a.pickDirFn = func(string) (string, error) { return "", boom }
	if _, err := a.PickWorkspace(); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	a.pickDirFn = func(string) (string, error) { return "", nil } // cancelled
	if got, err := a.PickWorkspace(); got != "" || err != nil {
		t.Errorf("cancelled: %q, %v", got, err)
	}
}

func TestLoadAISettingsReadsTheWorkspacesProbeYaml(t *testing.T) {
	a := &App{}
	if _, err := a.LoadAISettings(""); err == nil || !strings.Contains(err.Error(), "workspace path is required") {
		t.Errorf("an empty workspace: %v", err)
	}
	dir := t.TempDir()
	yaml := "project:\n  app: com.example.app\nai:\n  provider: local\n  endpoint: http://127.0.0.1:1234/v1\n  model: some-model\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := a.LoadAISettings(dir)
	if err != nil || got.Provider != "local" || got.Model != "some-model" || got.Endpoint != "http://127.0.0.1:1234/v1" {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestTakeScreenshotAndGetWidgetTreeNeedAConnection(t *testing.T) {
	a := &App{ctx: context.Background()}
	if _, err := a.TakeScreenshot(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("TakeScreenshot: %v", err)
	}
	if _, err := a.GetWidgetTree(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("GetWidgetTree: %v", err)
	}
	// connected: what the agent answers is what is returned, and an agent that fails is an error, not an empty answer
	f := &viewFakeClient{}
	a.conn = &connection{client: f, deviceID: "dev-1", deviceName: "Phone", platform: device.PlatformAndroid}
	if tree, err := a.GetWidgetTree(); err != nil || tree != "" {
		t.Errorf("GetWidgetTree = %q, %v", tree, err)
	}
	if shot, err := a.TakeScreenshot(); err == nil || shot != "" {
		t.Errorf("TakeScreenshot of an agent that fails = %q, %v", shot, err)
	}
}

func TestRunFileScopedNeedsAFileInTheWorkspaceAndAConnection(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	a := NewApp()
	a.SetWorkspace(dir)
	outside := filepath.Join(t.TempDir(), "x.probe")
	if err := os.WriteFile(outside, []byte("test \"x\"\n  log \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunFileScoped(outside, 0, "", false); err == nil || !strings.Contains(err.Error(), "outside the folders") {
		t.Errorf("a file outside the workspace: %v", err)
	}
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"x\"\n  log \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunFileScoped(file, 2, "", false); !errors.Is(err, errNotConnected) {
		t.Errorf("no connection: %v", err)
	}
	if a.runActive() {
		t.Error("a refused run holds the run slot")
	}
	// connected, the scope arguments reach the run: a line and a tag together are refused, and so is "the failed tests" of a file that has not run
	a.conn = &connection{client: &runFake{}, deviceID: "dev-1", platform: device.PlatformAndroid}
	if _, err := a.RunFileScoped(file, 2, "smoke", false); err == nil || !strings.Contains(err.Error(), "choose one run scope") {
		t.Errorf("a line and a tag: %v", err)
	}
	if _, err := a.RunFileScoped(file, 0, "", true); err == nil || !strings.Contains(err.Error(), "has not run in Studio yet") {
		t.Errorf("the failed tests of a file that has not run: %v", err)
	}
	if a.runActive() {
		t.Error("a refused scope holds the run slot")
	}
}

func TestStartRecordingNeedsAWebSocketConnection(t *testing.T) {
	a := &App{ctx: context.Background()}
	if err := a.StartRecording(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("no connection: %v", err)
	}
	a.conn = &connection{client: &runFake{}, deviceID: "dev-1", platform: device.PlatformAndroid}
	if err := a.StartRecording(); err == nil || !strings.Contains(err.Error(), "WebSocket connection") {
		t.Errorf("a connection that is not a WebSocket one: %v", err)
	}
}

func TestWiFiDiscoveryStartAndStop(t *testing.T) {
	if err := (&App{}).StartWiFiDiscovery(); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("no discovery: %v", err)
	}
	(&App{}).StopWiFiDiscovery() // nothing to stop: no panic
	var cancelled atomic.Int32
	a := &App{wifi: &wifiDiscovery{cancel: func() { cancelled.Add(1) }}}
	if err := a.StartWiFiDiscovery(); err != nil || cancelled.Load() != 0 {
		t.Errorf("already running: %v, cancelled %d", err, cancelled.Load())
	}
	a.StopWiFiDiscovery()
	if cancelled.Load() != 1 || a.wifi.cancel != nil {
		t.Errorf("stop: cancelled %d, cancel kept %v", cancelled.Load(), a.wifi.cancel != nil)
	}
	a.StopWiFiDiscovery() // stopping twice cancels once
	if cancelled.Load() != 1 {
		t.Errorf("a second stop cancelled again: %d", cancelled.Load())
	}
}
