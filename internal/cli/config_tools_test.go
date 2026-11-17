package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// repoWithConfig makes a repository (a .git folder) with a probe.yaml in a subfolder whose tools section is tools, and returns the loaded
// config and the repository's folder.
func repoWithConfig(t *testing.T, tools string) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	app := filepath.Join(repo, "app")
	for _, d := range []string{filepath.Join(repo, ".git"), app} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	yamlPath := filepath.Join(app, "probe.yaml")
	if err := os.WriteFile(yamlPath, []byte("project:\n  app: com.example.app\ntools:\n"+tools), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, repo
}

func execFile(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// LoadFile records the absolute path of the file it read; with no file there is no source.
func TestConfigRecordsItsSource(t *testing.T) {
	cfg, repo := repoWithConfig(t, "")
	if want := filepath.Join(repo, "app", "probe.yaml"); cfg.Source != want {
		t.Errorf("Source = %q, want %q", cfg.Source, want)
	}
	missing, err := config.LoadFile(filepath.Join(t.TempDir(), "probe.yaml"))
	if err != nil || missing.Source != "" {
		t.Errorf("a missing probe.yaml has no source: %q, %v", missing.Source, err)
	}
}

// A tools.adb that points into the repository is refused, with a message that names the key and the file; one outside it is used.
func TestProbeYAMLToolsMustBeOutsideTheRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute bits")
	}
	outside := execFile(t, filepath.Join(t.TempDir(), "platform-tools"), "adb")
	cfg, repo := repoWithConfig(t, "  adb: "+outside+"\n")
	paths, err := runToolPaths(cfg, "", "")
	if err != nil {
		t.Fatalf("an adb outside the repository must be used: %v", err)
	}
	if real, _ := filepath.EvalSymlinks(outside); paths.ADB != real {
		t.Errorf("ADB = %q, want %q", paths.ADB, real)
	}

	inRepo := execFile(t, filepath.Join(repo, "tools"), "adb")
	cfg.Tools.ADB = inRepo
	_, err = runToolPaths(cfg, "", "")
	if err == nil || !strings.Contains(err.Error(), "tools.adb in "+cfg.Source+" is not used") || !strings.Contains(err.Error(), "inside the repository") || !strings.Contains(err.Error(), "--adb") {
		t.Fatalf("an adb inside the repository must be refused naming the key, the file and the way out, got %v", err)
	}

	cfg.Tools.ADB = ""
	cfg.Tools.Flutter = "tools/flutter"
	if _, err := runToolPaths(cfg, "", ""); err == nil || !strings.Contains(err.Error(), "tools.flutter") || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("a relative tools.flutter must be refused, got %v", err)
	}
}

// A flag the person typed is used as given, even when probe.yaml names a path that would be refused.
func TestToolFlagsWinAndAreNotChecked(t *testing.T) {
	cfg, _ := repoWithConfig(t, "  adb: tools/adb\n  flutter: tools/flutter\n")
	paths, err := runToolPaths(cfg, "./my-adb", "./my-flutter")
	if err != nil || paths.ADB != "./my-adb" || paths.Flutter != "./my-flutter" {
		t.Fatalf("the flags must win as typed: %+v, %v", paths, err)
	}
	if _, err := runToolPaths(cfg, "./my-adb", ""); err == nil || !strings.Contains(err.Error(), "tools.flutter") {
		t.Fatalf("a flag for adb does not excuse tools.flutter: %v", err)
	}
}

// No tools section: nothing to check, the tools come from PATH.
func TestNoToolsMeansPath(t *testing.T) {
	cfg, _ := repoWithConfig(t, "")
	paths, err := runToolPaths(cfg, "", "")
	if err != nil || paths.ADB != "" || paths.Flutter != "" {
		t.Fatalf("want empty paths, got %+v, %v", paths, err)
	}
	if paths, err := runToolPaths(nil, "", ""); err != nil || paths.ADB != "" {
		t.Fatalf("a nil config has no tools: %+v, %v", paths, err)
	}
}

// probe doctor fails the check instead of running or looking up a refused path.
func TestDoctorFailsARefusedToolsPath(t *testing.T) {
	cfg, _ := repoWithConfig(t, "  adb: tools/adb\n  flutter: tools/flutter\n")
	cfg.Defaults.Platform = "android"
	env := doctorEnv{goos: runtime.GOOS, lookPath: func(s string) (string, error) { return s, nil }}
	if c := checkFlutter(env, cfg); c.status != "fail" || !strings.Contains(c.detail, "tools.flutter") {
		t.Errorf("flutter: want fail naming tools.flutter, got %+v", c)
	}
	var adb *doctorCheck
	for _, c := range checkPlatformTools(env, cfg) {
		if c.name == "adb" {
			c := c
			adb = &c
		}
	}
	if adb == nil || adb.status != "fail" || !strings.Contains(adb.detail, "tools.adb") {
		t.Errorf("adb: want fail naming tools.adb, got %+v", adb)
	}
}

// probe test and probe doctor never take cfg.Tools as it is: every use goes through runToolPaths or configTool.
func TestToolsFromProbeYAMLAreAlwaysChecked(t *testing.T) {
	for _, f := range []string{"test.go", "doctor.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "cfg.Tools.") {
			t.Errorf("%s reads cfg.Tools directly; use runToolPaths or configTool", f)
		}
	}
}
