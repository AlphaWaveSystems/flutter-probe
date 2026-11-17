package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	paths, err := runToolPaths(cfg, "", "", true)
	if err != nil {
		t.Fatalf("an adb outside the repository must be used: %v", err)
	}
	if real, _ := filepath.EvalSymlinks(outside); paths.ADB != real {
		t.Errorf("ADB = %q, want %q", paths.ADB, real)
	}

	inRepo := execFile(t, filepath.Join(repo, "tools"), "adb")
	cfg.Tools.ADB = inRepo
	_, err = runToolPaths(cfg, "", "", true)
	if err == nil || !strings.Contains(err.Error(), "tools.adb in "+cfg.Source+" is not used") || !strings.Contains(err.Error(), "inside the repository") || !strings.Contains(err.Error(), "--adb") {
		t.Fatalf("an adb inside the repository must be refused naming the key, the file and the way out, got %v", err)
	}

	cfg.Tools.ADB = ""
	cfg.Tools.Flutter = "tools/flutter"
	if _, err := runToolPaths(cfg, "", "", true); err == nil || !strings.Contains(err.Error(), "tools.flutter") || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("a relative tools.flutter must be refused, got %v", err)
	}
}

// A flag the person typed is used as given, even when probe.yaml names a path that would be refused.
func TestToolFlagsWinAndAreNotChecked(t *testing.T) {
	cfg, _ := repoWithConfig(t, "  adb: tools/adb\n  flutter: tools/flutter\n")
	paths, err := runToolPaths(cfg, "./my-adb", "./my-flutter", true)
	if err != nil || paths.ADB != "./my-adb" || paths.Flutter != "./my-flutter" {
		t.Fatalf("the flags must win as typed: %+v, %v", paths, err)
	}
	if _, err := runToolPaths(cfg, "./my-adb", "", true); err == nil || !strings.Contains(err.Error(), "tools.flutter") {
		t.Fatalf("a flag for adb does not excuse tools.flutter: %v", err)
	}
}

// No tools section: nothing to check, the tools come from PATH.
func TestNoToolsMeansPath(t *testing.T) {
	cfg, _ := repoWithConfig(t, "")
	paths, err := runToolPaths(cfg, "", "", true)
	if err != nil || paths.ADB != "" || paths.Flutter != "" {
		t.Fatalf("want empty paths, got %+v, %v", paths, err)
	}
	if paths, err := runToolPaths(nil, "", "", true); err != nil || paths.ADB != "" {
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

// No code takes a tools path from probe.yaml as it is: every non-test Go file of the CLI, Studio and the commands that reads .Tools.ADB or
// .Tools.Flutter is one of the two that check it (configTool here; Studio's workspaceADB, which calls toolpath.Check). A new reader fails
// this test until it goes through the check and is added here.
func TestToolsFromProbeYAMLAreAlwaysChecked(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("internal", "cli", "config_tools.go"): true,
		filepath.Join("studio", "app.go"):                   true,
	}
	reader := regexp.MustCompile(`\.Tools\.(ADB|Flutter)\b`)
	root := filepath.Join("..", "..")
	seen := 0
	for _, top := range []string{"internal", "studio", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "frontend") {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			if reader.Match(src) {
				seen++
				if !allowed[rel] {
					t.Errorf("%s reads a tools path from probe.yaml; go through configTool (CLI) or toolpath.Check (Studio)", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen < len(allowed) {
		t.Errorf("only %d readers found: the walk missed the known ones", seen)
	}
}

// probe doctor has no --adb flag: its message does not offer one. probe test's does.
func TestConfigToolMessageOffersTheFlagOnlyWhereThereIsOne(t *testing.T) {
	cfg := &config.Config{}
	cfg.Tools.ADB = "./tools/adb"
	_, errDoctor := configTool(cfg, "adb", false)
	_, errTest := configTool(cfg, "adb", true)
	if errDoctor == nil || strings.Contains(errDoctor.Error(), "--adb") {
		t.Errorf("doctor's message must not offer --adb: %v", errDoctor)
	}
	if errTest == nil || !strings.Contains(errTest.Error(), "or pass --adb") {
		t.Errorf("probe test's message must offer --adb: %v", errTest)
	}
}

// A refused tools path stops probe test with an error, so the process exits non-zero.
func TestProbeTestExitsNonZeroOnARefusedToolsPath(t *testing.T) {
	dir := t.TempDir()
	yaml := filepath.Join(dir, "probe.yaml")
	if err := os.WriteFile(yaml, []byte("tools:\n  adb: ./tools/adb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	test := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(test, []byte("test \"a\"\n  see \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	c := rootCmd
	c.SetOut(&out)
	c.SetErr(&out)
	// rootCmd is shared by the package's tests: put --config back so a later test does not run with this probe.yaml.
	t.Cleanup(func() { _ = c.PersistentFlags().Set("config", "") })
	c.SetArgs([]string{"test", test, "--config", yaml})
	err := c.Execute()
	c.SetArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "tools.adb") {
		t.Fatalf("probe test must fail on the refused tools.adb, got %v", err)
	}
	if code := ExitCode(err); code == 0 {
		t.Errorf("exit code = 0, want non-zero")
	}
}

// A probe.yaml reached through a symlink (from a folder outside any repository) is protected where it really is: a tool in its repository
// is refused.
func TestASymlinkedProbeYAMLIsProtectedWhereItReallyIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks and a shell-script tool")
	}
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	adb := filepath.Join(repoDir, "tools", "adb")
	if err := os.MkdirAll(filepath.Dir(adb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adb, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	realYAML := filepath.Join(repoDir, "probe.yaml")
	if err := os.WriteFile(realYAML, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "probe.yaml")
	if err := os.Symlink(realYAML, link); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Source: link}
	cfg.Tools.ADB = adb
	if _, err := configTool(cfg, "adb", true); err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Errorf("a tool in the repository of the real probe.yaml must be refused, got %v", err)
	}
}
