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
// test reads every `go install` command in the docs, the READMEs, the workflows, the scripts and the Makefiles and requires the
// targets that name this organisation to use a module path that a go.mod in the repository declares, byte for byte. A command is
// read as a shell would split it: flags (and the value of a flag that takes one), make and shell variables and line continuations
// are skipped, and every remaining word is a target, so `go install -tags netgo a b` checks both a and b.

const goInstallRoot = ".."

var (
	moduleLine     = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)
	goInstallStart = regexp.MustCompile(`\bgo\s+install\b`)
	goInstallJoin  = regexp.MustCompile(`\\[ \t]*\n[ \t]*`)
)

// valueFlags are the go build flags that take their value as the next word (`-tags netgo`, `-C dir`).
var valueFlags = map[string]bool{
	"-C": true, "-tags": true, "-ldflags": true, "-gcflags": true, "-asmflags": true, "-gccgoflags": true, "-mod": true,
	"-modfile": true, "-overlay": true, "-p": true, "-pgo": true, "-pkgdir": true, "-toolexec": true, "-installsuffix": true,
	"-compiler": true, "-covermode": true, "-coverpkg": true, "-buildmode": true, "-o": true,
}

// shellWords splits s into words the way a shell would for the cases a command line has: spaces separate words, a quoted string is
// one word (the quotes are dropped), and a $( ... ) group, which may nest, stays inside its word.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	depth := 0
	inWord := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			depth++
			inWord = true
			cur.WriteString("$(")
			i++
		case r == '(' && depth > 0:
			depth++
			cur.WriteRune(r)
		case r == ')' && depth > 0:
			depth--
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && depth == 0:
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			inWord = true
			cur.WriteRune(r)
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}

// goInstallTargets returns the targets of every `go install` command in text.
func goInstallTargets(text string) []string {
	text = goInstallJoin.ReplaceAllString(text, " ")
	var targets []string
	for _, loc := range goInstallStart.FindAllStringIndex(text, -1) {
		rest := text[loc[1]:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[:nl]
		}
		for _, sep := range []string{";", "&&", "||", "|"} {
			if k := strings.Index(rest, sep); k >= 0 {
				rest = rest[:k]
			}
		}
		words := shellWords(rest)
		for k := 0; k < len(words); k++ {
			w := strings.Trim(words[k], "`),.")
			switch {
			case w == "":
			case strings.HasPrefix(w, "-"):
				if valueFlags[w] {
					k++ // its value
				}
			case strings.HasPrefix(w, "$"):
			default:
				targets = append(targets, w)
			}
		}
	}
	return targets
}

// ourOrg is the organisation's prefix, compared without case: a target that starts with it must be spelled right.
const ourOrg = "github.com/alphawavesystems/"

// badGoInstallTargets returns the `go install` targets in text that name this organisation but do not start with one
// of the declared module paths.
func badGoInstallTargets(text string, modules []string) []string {
	var bad []string
	for _, target := range goInstallTargets(text) {
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

// goInstallProblems walks root and returns the `go install` targets that name this organisation with a module path no go.mod declares,
// as "relative/path: target", and the number of files it read. The files it reads are the documents, workflows, scripts, Makefiles
// (Makefile, makefile, GNUmakefile, *.mk) and container files (Containerfile, Dockerfile*) of the tree; CHANGELOG.md is skipped (history:
// it may quote what a page once said), and so are .git, .worktrees, node_modules and research.
func goInstallProblems(root string, mods []string) ([]string, int, error) {
	exts := map[string]bool{".md": true, ".mdx": true, ".astro": true, ".yml": true, ".yaml": true, ".txt": true, ".sh": true, ".json": true, ".mk": true}
	named := map[string]bool{"Makefile": true, "makefile": true, "GNUmakefile": true, "Containerfile": true} // files with no extension that hold commands
	skipFiles := map[string]bool{"CHANGELOG.md": true}
	scanned := 0
	var problems []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		if !(exts[filepath.Ext(path)] || named[d.Name()] || strings.HasPrefix(d.Name(), "Dockerfile")) || skipFiles[d.Name()] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, bad := range badGoInstallTargets(string(b), mods) {
			problems = append(problems, filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))+": "+bad)
		}
		return nil
	})
	sort.Strings(problems)
	return problems, scanned, err
}

func TestGoInstallLinesUseTheDeclaredModulePath(t *testing.T) {
	mods := declaredModules(t)
	problems, scanned, err := goInstallProblems(goInstallRoot, mods)
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d files: the walk is not reaching the docs", scanned)
	}
	for _, p := range problems {
		t.Errorf("go install names this organisation with a module path no go.mod declares (declared: %v): %s", mods, p)
	}
}

// The walk of the real tree finds nothing wrong today, so a kind of file that the walk stopped reading would go unnoticed. This plants
// a bad target in one file of each kind and requires every one to be reported, and requires the skipped places to stay skipped.
func TestGoInstallWalkReadsEveryKindOfFile(t *testing.T) {
	const bad = "go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest\n"
	mods := []string{"github.com/alphawavesystems/flutter-probe"}
	root := t.TempDir()
	read := []string{"README.md", "docs/page.mdx", "site/page.astro", "ci/workflow.yml", "ci/workflow.yaml", "notes.txt", "scripts/install.sh",
		"data.json", "Makefile", "lower/makefile", "gnu/GNUmakefile", "build/rules.mk", "Dockerfile", "Dockerfile.dev", "Containerfile", "deep/er/path/Makefile"}
	skipped := []string{"CHANGELOG.md", ".git/config.md", "node_modules/pkg/README.md", "docs/research/note.md", ".worktrees/x/README.md", "main.go", "image.png"}
	for _, f := range append(append([]string{}, read...), skipped...) {
		path := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	problems, scanned, err := goInstallProblems(root, mods)
	if err != nil {
		t.Fatal(err)
	}
	if scanned != len(read) {
		t.Errorf("read %d files, want %d (one of each kind, none of the skipped places)", scanned, len(read))
	}
	got := map[string]bool{}
	for _, p := range problems {
		got[strings.SplitN(p, ": ", 2)[0]] = true
	}
	for _, f := range read {
		if !got[f] {
			t.Errorf("a bad target in %s was not reported", f)
		}
	}
	for _, f := range skipped {
		if got[f] {
			t.Errorf("%s must be skipped but was reported", f)
		}
	}
}

// A flag's value is not a target: `-tags netgo` must not make `netgo` a target, and the word after it is the target.
func TestGoInstallTargetsSkipFlagValues(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"-tags with a value", "go install -tags netgo example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"-C with a directory", "go install -C tools example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"-ldflags unquoted value", "go install -ldflags -s example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"a flag that takes no value", "go install -v example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"a flag with =", "go install -tags=netgo example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"two value flags", "go install -tags netgo -C tools example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
		{"a variable and a flag value", "go install $(FLAGS) -tags netgo example.com/a/cmd/x@latest", []string{"example.com/a/cmd/x@latest"}},
	} {
		got := goInstallTargets(tc.text)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
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
		{"a flag with an unquoted separate value", "go install -tags netgo github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a -C directory", "go install -C tools github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a line continuation after go install", "go install \\\n  github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"two spaces", "go  install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a nested make call", "\tgo install $(call flags,$(X)) github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest\n", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a second target on the line", "go install github.com/alphawavesystems/flutter-probe/cmd/probe@latest github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a command after the install", "go install github.com/alphawavesystems/flutter-probe/cmd/probe@latest && echo github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", nil},
		{"after a semicolon", "go install ./cmd/probe; go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
		{"a -o flag with a value", "go install -o /tmp/x github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest", []string{"github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest"}},
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
