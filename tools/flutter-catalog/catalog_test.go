package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fakeSDK is a made-up SDK folder (not Flutter source) that has every case the tool has to get right.
const fakeSDK = "testdata/sdk"

func generateFake(t *testing.T) *Catalog {
	t.Helper()
	c, err := Generate(fakeSDK)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The drift test: the catalog of the made-up SDK is byte for byte the committed golden. A change of the output's shape or of what counts as a widget shows here.
func TestCatalogOfTheFakeSDKMatchesTheGolden(t *testing.T) {
	b, err := Marshal(generateFake(t))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/catalog.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("the catalog of testdata/sdk differs from testdata/catalog.golden.json; if the change is intended, regenerate the golden and review its diff\n--- got\n%s", b)
	}
}

// The golden is not the only oracle: the cases that matter are named here, so a wrongly recorded golden does not hide a wrong rule.
func TestWhatCountsAsAWidget(t *testing.T) {
	got := map[string]Widget{}
	for _, w := range generateFake(t).Widgets {
		got[w.Name] = w
	}
	in := func(name, lib string) {
		t.Helper()
		if w, ok := got[name]; !ok || w.Library != lib {
			t.Errorf("%s: want it in %s, got %+v (listed %v)", name, lib, w, ok)
		}
	}
	out := func(name, why string) {
		t.Helper()
		if _, ok := got[name]; ok {
			t.Errorf("%s must not be listed: %s", name, why)
		}
	}
	in("Text", "widgets")
	in("Padding", "widgets")         // generic arguments over several lines before `extends`
	in("Wrapper", "widgets")         // a public class whose base is private
	in("Chip", "material")           // the same, in another library
	in("ElevatedButton", "material") // two steps from Widget, through a class of the same file
	in("Theme", "material")          // through an InheritedWidget, a base from the widgets library
	in("CupertinoSwitch", "cupertino")
	in("CupertinoOnlyHere", "cupertino") // extends Widget directly
	out("Widget", "the root itself has no base")
	out("_Private", "a private class")
	out("NotExported", "the `show` list of its export does not name it")
	out("CupertinoHidden", "the `hide` list of its export names it")
	out("Helper", "not a widget")
	out("RenderThing", "a render object, not a widget")
	out("State", "not a widget")
	out("Loop1", "a superclass cycle never reaches Widget")
	out("Loop2", "a superclass cycle never reaches Widget")

	if !got["StatelessWidget"].Abstract || got["Text"].Abstract {
		t.Errorf("abstract flags wrong: StatelessWidget %+v, Text %+v", got["StatelessWidget"], got["Text"])
	}
	for name, want := range map[string]bool{"OldBar": true, "OldestBar": true, "NewBar": false, "NotOld": false, "Text": false} {
		if got[name].Deprecated != want {
			t.Errorf("%s: deprecated = %v, want %v (a doc comment that only says the word must not count)", name, got[name].Deprecated, want)
		}
	}
	if got["Chip"].Base != "_ChipBase" {
		t.Errorf("Chip base = %q, want the direct superclass _ChipBase", got["Chip"].Base)
	}
}

func TestStampAndCounts(t *testing.T) {
	c := generateFake(t)
	if c.Flutter.Version != "9.9.9" || c.Flutter.Dart != "9.0.1" || c.Flutter.Revision != "0000000000000000000000000000000000000000" || c.Schema != schemaVersion {
		t.Errorf("stamp = %+v schema %d", c.Flutter, c.Schema)
	}
	by := map[string]int{}
	for _, w := range c.Widgets {
		by[w.Library]++
	}
	total := 0
	for _, l := range libraries {
		if c.Counts.ByLibrary[l] != by[l] {
			t.Errorf("count of %s = %d, listed %d", l, c.Counts.ByLibrary[l], by[l])
		}
		total += by[l]
	}
	if c.Counts.Total != total || total != len(c.Widgets) {
		t.Errorf("total %d, sum %d, listed %d", c.Counts.Total, total, len(c.Widgets))
	}
}

// The output is sorted (library in the fixed order, then name) and the same twice.
func TestOutputIsSortedAndStable(t *testing.T) {
	a, _ := Marshal(generateFake(t))
	b, _ := Marshal(generateFake(t))
	if !bytes.Equal(a, b) {
		t.Fatal("two runs on the same tree differ")
	}
	ws := generateFake(t).Widgets
	if !sort.SliceIsSorted(ws, func(i, j int) bool {
		if li, lj := libIndex(ws[i].Library), libIndex(ws[j].Library); li != lj {
			return li < lj
		}
		return ws[i].Name < ws[j].Name
	}) {
		t.Error("widgets are not sorted by library, then name")
	}
	if bytes.Contains(a, []byte("\r")) || !bytes.HasSuffix(a, []byte("}\n")) || bytes.HasSuffix(a, []byte("\n\n")) {
		t.Error("the file must use \\n and end in exactly one newline")
	}
}

// Only names and flags are recorded: every key is one of a fixed set and every text is an identifier or a version. No documentation text can get in.
func TestOnlyFactsAreRecorded(t *testing.T) {
	b, _ := Marshal(generateFake(t))
	var raw struct {
		Widgets []map[string]any `json:"widgets"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"name": true, "library": true, "base": true, "abstract": true, "deprecated": true}
	ident := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, w := range raw.Widgets {
		for k, v := range w {
			if !allowed[k] {
				t.Errorf("unexpected key %q in %v", k, w)
			}
			if s, ok := v.(string); ok && !ident.MatchString(s) {
				t.Errorf("%s = %q is not an identifier", k, s)
			}
		}
	}
	ver := regexp.MustCompile(`^[0-9A-Za-z.+-]*$`)
	c := generateFake(t)
	for _, s := range []string{c.Flutter.Version, c.Flutter.Revision, c.Flutter.Dart} {
		if !ver.MatchString(s) {
			t.Errorf("stamp text %q is not a plain version", s)
		}
	}
}

func TestNotAnSDKFolder(t *testing.T) {
	if _, err := Generate(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a Flutter SDK folder") {
		t.Errorf("empty folder: err = %v", err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, versionFile), `{"frameworkVersion":"1.0.0"}`)
	if _, err := Generate(dir); err == nil || !strings.Contains(err.Error(), "dartSdkVersion") {
		t.Errorf("no Dart version: err = %v", err)
	}
}

func write(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// treeHash is a digest of every file name and content under dir.
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		h.Write([]byte(p + "\x00"))
		h.Write(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(h.Sum(nil))
}

// The tool reads the SDK and writes only the file named by -out: the tree is the same after a run, and a second run gives the same bytes (idempotent).
func TestRunLeavesTheSDKAloneAndIsIdempotent(t *testing.T) {
	before := treeHash(t, fakeSDK)
	out := filepath.Join(t.TempDir(), "docs", "compat", "catalog.json")
	if err := run(fakeSDK, out, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(out)
	if err := run(fakeSDK, out, false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(out)
	if !bytes.Equal(first, second) {
		t.Error("a second run changed the file")
	}
	want, _ := os.ReadFile("testdata/catalog.golden.json")
	if !bytes.Equal(first, want) {
		t.Error("the written file is not the golden")
	}
	if treeHash(t, fakeSDK) != before {
		t.Error("the SDK folder changed")
	}
}

func TestRunRefusesToWriteInsideTheSDK(t *testing.T) {
	sdk := t.TempDir()
	copyTree(t, fakeSDK, sdk)
	before := treeHash(t, sdk)
	for _, out := range []string{filepath.Join(sdk, "catalog.json"), filepath.Join(sdk, "packages", "x", "..", "catalog.json")} {
		if err := run(sdk, out, false); err == nil || !strings.Contains(err.Error(), "inside the SDK") {
			t.Errorf("-out %s: err = %v", out, err)
		}
	}
	if treeHash(t, sdk) != before {
		t.Error("the SDK folder changed")
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		write(t, filepath.Join(to, rel), string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// -check writes nothing and fails when the file is missing, edited, or made from another Flutter version.
func TestCheckFailsOnDrift(t *testing.T) {
	out := filepath.Join(t.TempDir(), "catalog.json")
	if err := run(fakeSDK, out, true); err == nil {
		t.Error("check passed with no file")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("check wrote the file")
	}
	if err := run(fakeSDK, out, false); err != nil {
		t.Fatal(err)
	}
	if err := run(fakeSDK, out, true); err != nil {
		t.Errorf("check failed on an up to date file: %v", err)
	}

	// a widget disappears from the SDK: the committed file is now behind
	sdk := t.TempDir()
	copyTree(t, fakeSDK, sdk)
	write(t, filepath.Join(sdk, libDir, "src", "widgets", "basic.dart"), "class Text extends StatelessWidget {}\n")
	if err := run(sdk, out, true); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Errorf("check missed a removed widget: %v", err)
	}
	// a new Flutter version with the same widgets still differs: the stamp is part of the file
	copyTree(t, fakeSDK, sdk)
	write(t, filepath.Join(sdk, versionFile), `{"frameworkVersion":"9.9.10","dartSdkVersion":"9.0.1"}`)
	if err := run(sdk, out, true); err == nil {
		t.Error("check missed a new Flutter version")
	}
	// an edited file differs
	b, _ := os.ReadFile(out)
	write(t, out, strings.Replace(string(b), `"Text"`, `"Txet"`, 1))
	if err := run(fakeSDK, out, true); err == nil {
		t.Error("check missed an edited file")
	}
}

// The tool is read-only by construction: no process execution, no network, and file writes only in main.go (the -out file), never in catalog.go.
func TestToolHasNoExecNetworkOrSDKWrites(t *testing.T) {
	banned := map[string]bool{"os/exec": true, "net": true, "net/http": true, "net/rpc": true, "plugin": true, "syscall": true, "unsafe": true}
	writes := map[string]bool{"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true, "Remove": true, "RemoveAll": true, "Rename": true,
		"Mkdir": true, "MkdirAll": true, "Chmod": true, "Chown": true, "Chtimes": true, "Symlink": true, "Link": true, "Truncate": true}
	allowedInMain := map[string]bool{"WriteFile": true, "MkdirAll": true}
	fset := token.NewFileSet()
	for _, name := range []string{"catalog.go", "main.go"} {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); banned[p] {
				t.Errorf("%s imports %s", name, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && writes[sel.Sel.Name] && !(name == "main.go" && allowedInMain[sel.Sel.Name]) {
				t.Errorf("%s: os.%s at %s", name, sel.Sel.Name, fset.Position(sel.Pos()))
			}
			return true
		})
	}
}

// A smoke test on a real SDK, only when one is named: PROBE_FLUTTER_SDK=<folder> go test ./tools/flutter-catalog.
func TestRealSDK(t *testing.T) {
	root := os.Getenv("PROBE_FLUTTER_SDK")
	if root == "" {
		t.Skip("PROBE_FLUTTER_SDK is not set")
	}
	c, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]string{}
	for _, w := range c.Widgets {
		have[w.Name] = w.Library
		if strings.HasPrefix(w.Name, "_") || w.Base == "" {
			t.Errorf("bad entry %+v", w)
		}
	}
	for name, lib := range map[string]string{"Text": "widgets", "ListView": "widgets", "Scaffold": "material", "TextField": "material", "CupertinoSwitch": "cupertino"} {
		if have[name] != lib {
			t.Errorf("%s: library = %q, want %q", name, have[name], lib)
		}
	}
	if c.Counts.Total < 300 {
		t.Errorf("only %d widgets", c.Counts.Total)
	}
}

// The channel of an SDK says how it was installed, not which release it is: two installs of the same release give the same file whatever channel they report.
func TestTheChannelDoesNotChangeTheCatalog(t *testing.T) {
	a := t.TempDir()
	copyTree(t, fakeSDK, a)
	b := t.TempDir()
	copyTree(t, fakeSDK, b)
	write(t, filepath.Join(a, versionFile), `{"frameworkVersion":"9.9.9","channel":"stable","frameworkRevision":"0000000000000000000000000000000000000000","dartSdkVersion":"9.0.1"}`)
	write(t, filepath.Join(b, versionFile), `{"frameworkVersion":"9.9.9","channel":"[user-branch]","frameworkRevision":"0000000000000000000000000000000000000000","dartSdkVersion":"9.0.1"}`)
	ca, err := Generate(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := Generate(b)
	if err != nil {
		t.Fatal(err)
	}
	ba, _ := Marshal(ca)
	bb, _ := Marshal(cb)
	if !bytes.Equal(ba, bb) {
		t.Error("the same release on two channels gave two different catalogs")
	}
	if bytes.Contains(ba, []byte("channel")) || bytes.Contains(ba, []byte("user-branch")) {
		t.Errorf("the catalog records a channel: %s", ba)
	}
}
