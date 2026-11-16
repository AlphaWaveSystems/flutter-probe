package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// inDecoyFolder makes the process folder hold a probe.yaml of another project, and returns its app id: whatever Studio does, a run must never
// pick it up.
const decoyApp = "com.decoy.otherproject"

func inDecoyFolder(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	yaml := "project:\n  app: " + decoyApp + "\nagent:\n  port: 49999\ndefaults:\n  timeout: 1s\ntools:\n  adb: /decoy/adb\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	here, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(here) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func workspaceWithYaml(t *testing.T, yaml string) (*App, string) {
	t.Helper()
	ws := t.TempDir()
	if yaml != "" {
		if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.SetWorkspace(ws)
	return a, ws
}

func notTheDecoy(t *testing.T, cfg *config.Config) {
	t.Helper()
	if cfg.Project.App == decoyApp || cfg.Agent.Port == 49999 || cfg.Tools.ADB == "/decoy/adb" || cfg.Defaults.Timeout == time.Second {
		t.Fatalf("the config of the process folder was used: %+v", cfg)
	}
}

// With no workspace and with a workspace that has no probe.yaml the defaults are used, never the process folder's config.
func TestWorkspaceConfigDefaultsNeverComeFromTheProcessFolder(t *testing.T) {
	inDecoyFolder(t)
	none := &App{}
	cfg, err := none.workspaceConfig()
	if err != nil {
		t.Fatal(err)
	}
	notTheDecoy(t, cfg)
	if cfg.Project.App != "" {
		t.Errorf("no workspace: app = %q, want none", cfg.Project.App)
	}
	a, _ := workspaceWithYaml(t, "")
	cfg, err = a.workspaceConfig()
	if err != nil {
		t.Fatal(err)
	}
	notTheDecoy(t, cfg)
	if cfg.Project.App != "" || cfg.Agent.Port != config.Default().Agent.Port {
		t.Errorf("absent probe.yaml: %+v", cfg.Agent)
	}
}

func TestWorkspaceConfigReadsTheWorkspaceFile(t *testing.T) {
	inDecoyFolder(t)
	a, _ := workspaceWithYaml(t, "project:\n  app: com.example.mine\nagent:\n  port: 48777\n")
	cfg, err := a.workspaceConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project.App != "com.example.mine" || cfg.Agent.Port != 48777 {
		t.Errorf("cfg = %+v", cfg.Project)
	}
}

// A probe.yaml that cannot be loaded is an error with the one line, not another config: invalid YAML and an invalid app id, with a decoy in the
// process folder.
func TestWorkspaceConfigThatCannotBeLoadedIsRefusedNotReplaced(t *testing.T) {
	inDecoyFolder(t)
	for name, yaml := range map[string]string{
		"invalid yaml":       "project: [unclosed\n",
		"an invalid app id":  "project:\n  app: \"not a valid id\"\n",
		"a wrong value type": "agent:\n  port: [1, 2]\n",
	} {
		a, ws := workspaceWithYaml(t, yaml)
		cfg, err := a.workspaceConfig()
		if err == nil || cfg != nil {
			t.Errorf("%s: cfg %v err %v, want a refusal", name, cfg, err)
			continue
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "probe.yaml cannot be read: ") || !strings.HasSuffix(msg, ": fix it or remove it to use the defaults") {
			t.Errorf("%s: message = %q", name, msg)
		}
		if strings.Contains(msg, ws) || strings.Contains(msg, "\n") || strings.Contains(msg, decoyApp) {
			t.Errorf("%s: the message shows a path, a line break or the decoy: %q", name, msg)
		}
	}
}

// Every caller of the config gives the same answer: Connect and ConnectWiFi are refused before anything else is done, a run is refused, the
// lint says it was not checked, and the Workspace note names it.
func TestEveryCallerRefusesAnUnreadableProbeYaml(t *testing.T) {
	inDecoyFolder(t)
	a, ws := workspaceWithYaml(t, "project: [unclosed\n")
	const want = "probe.yaml cannot be read: "
	if _, err := a.Connect("any-device"); err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("Connect = %v", err)
	}
	if _, err := a.ConnectWiFi("192.168.1.20", 48686, "token"); err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("ConnectWiFi = %v", err)
	}
	file := filepath.Join(ws, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  log \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.conn = &connection{client: &runFake{}, cfg: config.Default()}
	if _, err := a.executeRun(context.Background(), file, runSelection{}, RunPlan{}); err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("a run = %v", err)
	}
	a.conn = nil
	res, err := a.LintProblems(file, "test \"t\"\n  log \"x\"\n")
	if err != nil || !strings.Contains(res.Unavailable, want) || len(res.Problems) != 0 {
		t.Errorf("lint = %+v, %v", res, err)
	}
	if n := a.WorkspaceNote(); n.Kind != "unreadable" || !strings.Contains(n.Text, want) || !strings.Contains(n.Text, "fix it or remove it to use the defaults") {
		t.Errorf("note = %+v", n)
	}
}

// The device context and the confirmation dialog take their app id from the workspace's config, never from the decoy.
func TestTheDeviceContextAndTheDialogNameTheWorkspacesApp(t *testing.T) {
	inDecoyFolder(t)
	a, _ := workspaceWithYaml(t, "project:\n  app: com.example.mine\n")
	cfg, err := a.workspaceConfig()
	if err != nil {
		t.Fatal(err)
	}
	dc := newStudioDeviceContext(a, cfg, "android", "emu-1", 48686, "")
	if dc.AppID != "com.example.mine" {
		t.Errorf("the device context's app = %q", dc.AppID)
	}
	if strings.Contains(dc.AppID, "decoy") {
		t.Error("the decoy's app id reached the device context")
	}
}
