package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A Go module path is case sensitive. The repository's modules are declared in lowercase (go.mod), but the GitHub
// organisation is written AlphaWaveSystems, and `go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest`
// fails with "module declares its path as: github.com/alphawavesystems/flutter-probe but was required as ...". This
// test reads every `go install` line in the docs, the READMEs and the workflows and requires the ones that name this
// organisation to use a module path that a go.mod in the repository declares, byte for byte.

const goInstallRoot = ".."

var (
	moduleLine = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)
	// go install, then any flags (-v, -ldflags=... or -ldflags "..." with a quoted value) and make or shell variables ($(LDFLAGS),
	// ${FLAGS}, $FLAGS), then the target, which may itself be quoted.
	goInstallLine = regexp.MustCompile("go install\\s+(?:(?:-[\\w-]+(?:=(?:\"[^\"]*\"|'[^']*'|\\S+))?\\s+(?:(?:\"[^\"]*\"|'[^']*')\\s+)?)|(?:\\$\\([^)]*\\)|\\$\\{[^}]*\\}|\\$\\w+)\\s+)*[\"']?([^\\s`'\"\\\\)]+)")
)

// ourOrg is the organisation's prefix, compared without case: a target that starts with it must be spelled right.
const ourOrg = "github.com/alphawavesystems/"

// badGoInstallTargets returns the `go install` targets in text that name this organisation but do not start with one
// of the declared module paths.
func badGoInstallTargets(text string, modules []string) []string {
	var bad []string
	for _, m := range goInstallLine.FindAllStringSubmatch(text, -1) {
		target := m[1]
		if !strings.HasPrefix(strings.ToLower(target), ourOrg) {
			continue
		}
		ok := false
		for _, mod := range modules {
			if target == mod || strings.HasPrefix(target, mod+"/") || strings.HasPrefix(target, mod+"@") {
				ok = true
				break
			}
		}
		if !ok {
			bad = append(bad, target)
		}
	}
	return bad
}

func declaredModules(t *testing.T) []string {
	t.Helper()
	var mods []string
	err := filepath.WalkDir(goInstallRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := moduleLine.FindSubmatch(b); m != nil {
			mods = append(mods, string(m[1]))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) == 0 {
		t.Fatal("no go.mod found under " + goInstallRoot)
	}
	return mods
}

func TestGoInstallLinesUseTheDeclaredModulePath(t *testing.T) {
	mods := declaredModules(t)
	exts := map[string]bool{".md": true, ".mdx": true, ".astro": true, ".yml": true, ".yaml": true, ".txt": true, ".sh": true, ".json": true}
	named := map[string]bool{"Makefile": true, "makefile": true, "GNUmakefile": true} // files with no extension that hold commands
	skipFiles := map[string]bool{"CHANGELOG.md": true}                                // history: it may quote what a page once said
	scanned := 0
	var problems []string
	err := filepath.WalkDir(goInstallRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "research":
				return filepath.SkipDir
			}
			return nil
		}
		if !(exts[filepath.Ext(path)] || named[d.Name()]) || skipFiles[d.Name()] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, bad := range badGoInstallTargets(string(b), mods) {
			problems = append(problems, filepath.ToSlash(strings.TrimPrefix(path, goInstallRoot+string(filepath.Separator)))+": "+bad)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d files: the walk is not reaching the docs", scanned)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("go install names this organisation with a module path no go.mod declares (declared: %v): %s", mods, p)
	}
}

func TestBadGoInstallTargetsCases(t *testing.T) {
	mods := []string{"github.com/alphawavesystems/flutter-probe", "github.com/alphawavesystems/probe-convert"}
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"declared path", "go install github.com/alphawavesystems/flutter-probe/cmd/probe@latest", nil},
		{"capitalised organisation", "go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"capitalised in a workflow", "        run: go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe-mcp@latest\n", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe-mcp@latest"}},
		{"in a code span", "use `go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@v1` now", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@v1"}},
		{"after a flag", "go install -v github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"module root with a version", "go install github.com/alphawavesystems/probe-convert@latest", nil},
		{"a module that does not exist", "go install github.com/alphawavesystems/nothing/cmd/x@latest", []string{"github.com/alphawavesystems/nothing/cmd/x@latest"}},
		{"another organisation", "go install github.com/wailsapp/wails/v2/cmd/wails@latest", nil},
		{"a local path", "go install ./cmd/probe", nil},
		{"a quoted target", `go install "github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"`, []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a single-quoted target", "go install 'github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest'", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a flag with a quoted value", `go install -ldflags "-s -w" github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest`, []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a flag with an = and quotes", `go install -ldflags="-s -w -X main.V=1" github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest`, []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a quoted flag value and a declared target", `go install -ldflags "-s -w" github.com/alphawavesystems/flutter-probe/cmd/probe@latest`, nil},
		{"a Makefile recipe line", "\tgo install $(LDFLAGS) github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest\n", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a Makefile line with a local path", "\tgo install $(LDFLAGS) ./cmd/probe\n", nil},
		{"a prefix that is not a path element", "go install github.com/alphawavesystems/flutter-probe-extras/cmd/x@latest", []string{"github.com/alphawavesystems/flutter-probe-extras/cmd/x@latest"}},
	} {
		got := badGoInstallTargets(tc.text, mods)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
