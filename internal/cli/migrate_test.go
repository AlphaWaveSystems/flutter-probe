package cli

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestDiscoverYAMLFiles_Recursive covers the G-3 fix: real Maestro suites
// commonly organize flows into subdirectories (nect-flutter's own real
// 76-flow suite is laid out exactly this way — flows/auth/, flows/settings/,
// etc.) — a single-level os.ReadDir here used to silently report "No
// Maestro YAML files found" for any project organized that way, rather than
// erroring or partially converting.
func TestDiscoverYAMLFiles_Recursive(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "top.yaml"), "- launchApp\n")
	mustWriteFile(t, filepath.Join(root, "auth", "login.yaml"), "- launchApp\n")
	mustWriteFile(t, filepath.Join(root, "settings", "logout.yaml"), "- launchApp\n")
	mustWriteFile(t, filepath.Join(root, "auth", "nested", "reset-password.yaml"), "- launchApp\n")

	files, err := discoverYAMLFiles([]string{root})
	if err != nil {
		t.Fatalf("discoverYAMLFiles: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 files discovered, got %d: %+v", len(files), files)
	}
}

// TestDiscoverYAMLFiles_PreservesRelativeDir covers the companion fix:
// output paths mirror the source subdirectory instead of flattening
// everything into one directory, so two files with the same base name in
// different subdirectories (a real, plausible layout — different features
// each having their own "login.yaml", say) don't collide.
func TestDiscoverYAMLFiles_PreservesRelativeDir(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "auth", "login.yaml"), "- launchApp\n")
	mustWriteFile(t, filepath.Join(root, "settings", "login.yaml"), "- launchApp\n")

	files, err := discoverYAMLFiles([]string{root})
	if err != nil {
		t.Fatalf("discoverYAMLFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(files), files)
	}

	relDirs := make([]string, len(files))
	for i, f := range files {
		relDirs[i] = f.relDir
	}
	sort.Strings(relDirs)
	want := []string{"auth", "settings"}
	for i := range want {
		if relDirs[i] != want[i] {
			t.Errorf("relDirs: got %v, want %v", relDirs, want)
			break
		}
	}
}

// TestDiscoverYAMLFiles_SingleFile covers passing a bare file path (not a
// directory) — the pre-existing, still-supported usage.
func TestDiscoverYAMLFiles_SingleFile(t *testing.T) {
	root := t.TempDir()
	yamlPath := filepath.Join(root, "solo.yaml")
	mustWriteFile(t, yamlPath, "- launchApp\n")

	files, err := discoverYAMLFiles([]string{yamlPath})
	if err != nil {
		t.Fatalf("discoverYAMLFiles: %v", err)
	}
	if len(files) != 1 || files[0].path != yamlPath {
		t.Fatalf("expected exactly the given file, got %+v", files)
	}
}

// TestDiscoverYAMLFiles_IgnoresNonYAML confirms the recursive walk still
// only picks up .yaml/.yml files, not every file in the tree.
func TestDiscoverYAMLFiles_IgnoresNonYAML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "flow.yaml"), "- launchApp\n")
	mustWriteFile(t, filepath.Join(root, "README.md"), "# not a flow\n")
	mustWriteFile(t, filepath.Join(root, "screenshots", "shot.png"), "not really a png")

	files, err := discoverYAMLFiles([]string{root})
	if err != nil {
		t.Fatalf("discoverYAMLFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected exactly 1 YAML file, got %d: %+v", len(files), files)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
