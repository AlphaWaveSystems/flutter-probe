package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// saveKnobs loads the form for dir, sets the test limit and retries, and saves, the way the window does.
func saveKnobs(t *testing.T, dir string) error {
	t.Helper()
	a := &App{}
	loaded, _ := a.LoadWorkspaceSettings(dir)
	form := loaded
	form.Base = &loaded
	form.TestTimeout = "45s"
	form.RetryFailedTests = 2
	return inWS(a, dir).SaveWorkspaceSettings(dir, form)
}

// expectedAfter is the original's decoded content with the two edits made: what the engine must read after the save, and nothing else changed.
func expectedAfter(t *testing.T, orig string) map[string]any {
	t.Helper()
	want, err := flatten(orig)
	if err != nil {
		t.Fatal(err)
	}
	want["doc0\x00defaults\x00test_timeout"] = "45s"
	want["doc0\x00defaults\x00retry_failed_tests"] = 2
	return want
}

// Shapes where the last child of defaults spans lines, or the file mixes line endings. The new keys must land after the whole child, the
// result must read as the original plus the two keys, and the file must still load; where Studio cannot do that it must refuse and leave
// the file as it was. Save never reports success for a file that is wrong.
func TestSaveOnShapesThatSpanLinesOrMixLineEndings(t *testing.T) {
	cases := map[string]string{
		"a multi-line quoted scalar last":  "defaults:\n  timeout: 30s\n  note: \"line1\n    line2\"\nagent:\n  port: 1\n",
		"a multi-line plain scalar last":   "defaults:\n  timeout: 30s\n  note: line1\n    line2\nagent:\n  port: 1\n",
		"a flow sequence last":             "defaults:\n  timeout: 30s\n  list: [a,\n    b\n  ]\nagent:\n  port: 1\n",
		"a flow map last":                  "defaults:\n  timeout: 30s\n  m: {a: 1,\n    b: 2\n  }\nagent:\n  port: 1\n",
		"mixed line endings":               "defaults:\r\n  timeout: 30s\nagent:\n  port: 1\n",
		"CRLF with a multi-line scalar":    "defaults:\r\n  timeout: 30s\r\n  note: \"a\r\n    b\"\r\nagent:\r\n  port: 1\r\n",
		"a block scalar last":              "defaults:\n  timeout: 30s\n  note: |\n    text\n    more\nagent:\n  port: 1\n",
		"a nested map last":                "defaults:\n  timeout: 30s\n  extra:\n    a: 1\n    b: 2\nagent:\n  port: 1\n",
		"a comment after the last child":   "defaults:\n  timeout: 30s\n  # about the next\n\nagent:\n  port: 1\n",
		"a document end marker":            "defaults:\n  timeout: 30s\n...\n",
		"two documents":                    "---\ndefaults:\n  timeout: 30s\n---\nother: 1\n",
		"anchors and an alias":             "defaults:\n  timeout: &t 30s\n  test_timeout: &a 20s\n  implicit_wait: *a\n",
		"a merge key":                      "base: &base\n  test_timeout: 5s\ndefaults:\n  <<: *base\n  timeout: 30s\n",
		"a quoted key":                     "\"defaults\":\n  \"test_timeout\": 20s\n",
		"a tag":                            "defaults:\n  test_timeout: !!str 20s  # c\n",
		"a value on the next line":         "defaults:\n  test_timeout:\n    20s\n",
		"a block scalar at the file's end": "defaults:\n  timeout: 30s\n  note: |-\n    x",
		"null defaults":                    "defaults: null\nagent:\n  port: 1\n",
		"a nested profile":                 "profiles:\n  ci:\n    defaults:\n      test_timeout: 1s\n",
		"a key named like ours in a block": "note: |\n  defaults:\n    test_timeout: 1s\nagent:\n  port: 1\n",
		"a byte order mark":                "\xEF\xBB\xBFdefaults:\n  timeout: 30s\n",
	}
	succeeded := 0
	for name, src := range cases {
		dir, path := writeGolden(t, src, 0o644)
		err := saveKnobs(t, dir)
		after := read(t, path)
		if err != nil {
			if after != src {
				t.Errorf("%s: refused (%v) but the file changed:\n%q", name, err, after)
			}
			continue
		}
		succeeded++
		got, ferr := flatten(after)
		if ferr != nil {
			t.Errorf("%s: saved, but the result does not parse: %v\n%q", name, ferr, after)
			continue
		}
		if want := expectedAfter(t, src); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: saved, but the file does not read as the original plus the two keys:\n got %v\nwant %v\n%q", name, got, want, after)
		}
		if _, cerr := config.Load(dir); cerr != nil {
			t.Errorf("%s: saved, but config.Load fails: %v", name, cerr)
		}
	}
	// The multi-line shapes are within what the editor can do: they must be saved, not just refused.
	for _, name := range []string{"a multi-line quoted scalar last", "a multi-line plain scalar last", "a flow sequence last", "a flow map last", "mixed line endings", "a nested map last", "a block scalar last", "a comment after the last child"} {
		dir, _ := writeGolden(t, cases[name], 0o644)
		if err := saveKnobs(t, dir); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	if succeeded < 10 {
		t.Errorf("only %d of %d shapes could be saved", succeeded, len(cases))
	}
}

// Where the keys go, exactly, for the shapes the review named.
func TestKeysLandAfterTheWholeLastChild(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"quoted":  {"defaults:\n  timeout: 30s\n  note: \"line1\n    line2\"\nagent:\n  port: 1\n", "defaults:\n  timeout: 30s\n  note: \"line1\n    line2\"\n  test_timeout: 45s\n  retry_failed_tests: 2\nagent:\n  port: 1\n"},
		"plain":   {"defaults:\n  timeout: 30s\n  note: line1\n    line2\nagent:\n  port: 1\n", "defaults:\n  timeout: 30s\n  note: line1\n    line2\n  test_timeout: 45s\n  retry_failed_tests: 2\nagent:\n  port: 1\n"},
		"flowseq": {"defaults:\n  timeout: 30s\n  list: [a,\n    b\n  ]\nagent:\n  port: 1\n", "defaults:\n  timeout: 30s\n  list: [a,\n    b\n  ]\n  test_timeout: 45s\n  retry_failed_tests: 2\nagent:\n  port: 1\n"},
		"mixed":   {"defaults:\r\n  timeout: 30s\nagent:\n  port: 1\n", "defaults:\r\n  timeout: 30s\n  test_timeout: 45s\n  retry_failed_tests: 2\nagent:\n  port: 1\n"}, // most lines are LF, so the added ones are
	} {
		dir, path := writeGolden(t, tc.src, 0o644)
		if err := saveKnobs(t, dir); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := read(t, path); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, tc.want)
		}
	}
}

// A property test with a fixed seed: 500 random probe.yaml files (comments, blank lines, odd indentation, quoted and multi-line values, flow
// collections, block scalars, nested maps, LF, CRLF and mixed endings, a missing final newline). Each is saved through the form. Either the
// save is refused and the file is byte for byte as it was, or the result reads, by the engine's parser, as the original plus exactly the
// edited keys, and config.Load reads it.
func TestRandomProbeYAMLShapesAreEditedExactlyOrRefused(t *testing.T) {
	rng := rand.New(rand.NewSource(20261116))
	saved := 0
	for i := 0; i < 500; i++ {
		src := randomProbeYAML(rng)
		dir, path := writeGolden(t, src, 0o644)
		a := &App{}
		loaded, err := a.LoadWorkspaceSettings(dir)
		if err != nil {
			continue // the engine itself cannot read this shape: nothing to edit
		}
		form := loaded
		form.Base = &loaded
		form.TestTimeout = "45s"
		form.RetryFailedTests = 2
		err = inWS(a, dir).SaveWorkspaceSettings(dir, form)
		after := read(t, path)
		if err != nil {
			if after != src {
				t.Fatalf("case %d refused (%v) but the file changed\n%q\n%q", i, err, src, after)
			}
			continue
		}
		saved++
		got, ferr := flatten(after)
		if ferr != nil {
			t.Fatalf("case %d saved but does not parse: %v\n%q\n%q", i, ferr, src, after)
		}
		want := expectedAfter(t, src)
		// A shape whose test_timeout was already 45s and retries already 2 is a no-op: the same expectation holds.
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d saved but does not read as the original plus the edits:\n%q\n%q\n got %v\nwant %v", i, src, after, got, want)
		}
		if _, cerr := config.Load(dir); cerr != nil {
			t.Fatalf("case %d saved but config.Load fails: %v\n%q", i, cerr, after)
		}
	}
	if saved < 250 {
		t.Errorf("only %d of 500 random files could be saved: refusing everything would pass this test", saved)
	}
}

func randomProbeYAML(rng *rand.Rand) string {
	pick := func(xs ...string) string { return xs[rng.Intn(len(xs))] }
	var b strings.Builder
	eol := func() string {
		switch rng.Intn(5) {
		case 0:
			return "\r\n"
		default:
			return "\n"
		}
	}
	if rng.Intn(3) == 0 {
		b.WriteString("# header comment" + eol())
	}
	sections := []string{"project", "agent", "defaults", "device", "recipes_folder"}
	rng.Shuffle(len(sections), func(i, j int) { sections[i], sections[j] = sections[j], sections[i] })
	indent := pick("  ", "  ", "    ")
	for _, sec := range sections[:2+rng.Intn(len(sections)-1)] {
		switch sec {
		case "recipes_folder":
			b.WriteString("recipes_folder: tests/recipes" + pick("", "   # where") + eol())
		case "project":
			b.WriteString("project:" + eol() + indent + "app: com.example.app" + pick("", " # id") + eol())
		case "agent":
			b.WriteString("agent:" + eol() + indent + "port: 48686" + pick("", " # port") + eol())
		case "device":
			b.WriteString("device:" + eol() + indent + "ios_device_id: ABC" + eol())
		case "defaults":
			b.WriteString("defaults:" + pick("", " # my defaults") + eol())
			for n := rng.Intn(4); n > 0; n-- {
				switch rng.Intn(9) {
				case 0:
					b.WriteString(indent + "timeout: 30s" + pick("", "   # step") + eol())
				case 1:
					b.WriteString(indent + "note: \"line1" + eol() + indent + indent + "line2\"" + eol())
				case 2:
					b.WriteString(indent + "note2: word1" + eol() + indent + indent + "word2" + eol())
				case 3:
					b.WriteString(indent + "list: [a," + eol() + indent + indent + "b" + eol() + indent + "]" + eol())
				case 4:
					b.WriteString(indent + "m: {a: 1," + eol() + indent + indent + "b: 2}" + eol())
				case 5:
					b.WriteString(indent + "text: |" + eol() + indent + indent + "x" + eol() + indent + indent + "y" + eol())
				case 6:
					b.WriteString(indent + "extra:" + eol() + indent + indent + "a: 1" + eol())
				case 7:
					b.WriteString(indent + "platform: ios" + eol() + indent + "# a comment" + eol())
				default:
					b.WriteString(indent + "screenshots: \"on_failure\"" + eol())
				}
			}
		}
		if rng.Intn(3) == 0 {
			b.WriteString(eol())
		}
		if rng.Intn(4) == 0 {
			b.WriteString("# next section" + eol())
		}
	}
	out := b.String()
	if rng.Intn(6) == 0 {
		out = strings.TrimRight(out, "\r\n")
	}
	return out
}

// The form is a snapshot: a field changed on disk since it was loaded is not put back, and a change to it is refused with a reload message.
func TestSaveRefusesWhenTheFileChangedUnderTheForm(t *testing.T) {
	dir, path := writeGolden(t, "defaults:\n  test_timeout: 20s\n", 0o644)
	a := &App{}
	loaded, _ := a.LoadWorkspaceSettings(dir)
	if err := os.WriteFile(path, []byte("defaults:\n  test_timeout: 99s\n"), 0o644); err != nil { // an editor writes the file
		t.Fatal(err)
	}
	form := loaded
	form.Base = &loaded
	form.ImplicitWait = "2s" // the person changes another field
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, form); err != nil {
		t.Fatalf("a change to a field the file did not change under the form was refused: %v", err)
	}
	if got := read(t, path); !strings.Contains(got, "test_timeout: 99s") || !strings.Contains(got, "implicit_wait: 2s") {
		t.Errorf("the edit made elsewhere was reverted or the change lost:\n%s", got)
	}
	form = loaded
	form.Base = &loaded
	form.TestTimeout = "30s" // the person changes the field that changed on disk
	err := inWS(a, dir).SaveWorkspaceSettings(dir, form)
	if err == nil || !strings.Contains(err.Error(), "changed on disk") || !strings.Contains(err.Error(), "test time limit") {
		t.Errorf("err = %v", err)
	}
	if got := read(t, path); !strings.Contains(got, "99s") {
		t.Errorf("the file was written: %s", got)
	}
}

// A value already in probe.yaml that the form would not accept does not block saving another field; changing it is held to the rules.
func TestAPreExistingOutOfRangeValueDoesNotBlockOtherSaves(t *testing.T) {
	dir, path := writeGolden(t, "defaults:\n  test_timeout: 500ms\n  retry_failed_tests: 25\n  implicit_wait: 20m\n", 0o644)
	a := &App{}
	loaded, _ := a.LoadWorkspaceSettings(dir)
	form := loaded
	form.Base = &loaded
	form.AgentPort = 48800
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, form); err != nil {
		t.Fatalf("saving the port was blocked by values that were already there: %v", err)
	}
	if !strings.Contains(read(t, path), "test_timeout: 500ms") {
		t.Error("the existing value changed")
	}
	form.TestTimeout = "200ms"
	if err := inWS(a, dir).SaveWorkspaceSettings(dir, form); err == nil {
		t.Error("a changed value under 1s was accepted")
	}
}

// If the last step fails, the original stays and no temp file is left.
func TestAFailedRenameLeavesTheOriginalAndNoTempFile(t *testing.T) {
	dir, path := writeGolden(t, goldenYAML, 0o644)
	renameFile = func(string, string) error { return errors.New("disk full") }
	defer func() { renameFile = os.Rename }()
	if err := saveKnobs(t, dir); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("err = %v", err)
	}
	if read(t, path) != goldenYAML {
		t.Error("the original changed")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder holds %v", entries)
	}
}

// Two saves at once never interleave: the file is whole after both, and the race detector sees nothing.
func TestConcurrentSavesKeepTheFileWhole(t *testing.T) {
	dir, path := writeGolden(t, goldenYAML, 0o644)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &App{}
			loaded, _ := a.LoadWorkspaceSettings(dir)
			form := loaded
			form.Base = &loaded
			form.RetryFailedTests = 1 + i%3
			_ = inWS(a, dir).SaveWorkspaceSettings(dir, form)
			_ = writeAISettings(dir, AISettings{Provider: "local", Endpoint: "http://127.0.0.1:11434/v1", Model: fmt.Sprintf("m%d", i), KeySource: "keychain", Vision: true})
		}(i)
	}
	wg.Wait()
	if _, err := flatten(read(t, path)); err != nil {
		t.Errorf("the file does not parse after concurrent saves: %v", err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Errorf("config.Load: %v", err)
	}
}

// The AI writer removes what the form clears, keeps the rest, and a vision switch back on removes the explicit false.
func TestTheAIWriterRemovesWhatTheFormClears(t *testing.T) {
	src := "# demo\nai:\n  provider: local   # mine\n  endpoint: http://127.0.0.1:11434/v1\n  model: old\n  timeout: 30s\n  vision: false\nstudio:\n  ai_allow_remote_endpoint: true\nproject:\n  app: a.b\n"
	dir, path := writeGolden(t, src, 0o644)
	if err := writeAISettings(dir, AISettings{Provider: "local", Endpoint: "http://127.0.0.1:11434/v1", Model: "old", KeySource: "keychain", Vision: true}); err != nil {
		t.Fatal(err)
	}
	want := "# demo\nai:\n  provider: local   # mine\n  endpoint: http://127.0.0.1:11434/v1\n  model: old\nproject:\n  app: a.b\n"
	if got := read(t, path); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The check before the write is what protects the file when a splice goes wrong in a way nobody foresaw: an edit that lands inside a value, drops
// a key or adds one that was not asked for is refused, and nothing is written.
func TestTheCheckBeforeTheWriteRefusesAWrongEdit(t *testing.T) {
	src := "defaults:\n  timeout: 30s\n  note: line1\n    line2\nagent:\n  port: 1\n"
	for name, corrupt := range map[string]func(text string) string{
		"a key inside a multi-line value": func(text string) string {
			return strings.Replace(text, "  note: line1\n", "  note: line1\n  test_timeout: 45s\n", 1)
		},
		"another key lost":         func(text string) string { return strings.Replace(text, "  port: 1\n", "", 1) },
		"another value changed":    func(text string) string { return strings.Replace(text, "port: 1", "port: 2", 1) },
		"an extra key":             func(text string) string { return text + "extra: 1\n" },
		"the edit not applied":     func(text string) string { return strings.Replace(text, "  test_timeout: 45s\n", "", 1) },
		"text that does not parse": func(text string) string { return text + "agent: [\n" },
	} {
		dir, path := writeGolden(t, src, 0o644)
		y, err := openProbeYAML(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := y.set("45s", "defaults", "test_timeout"); err != nil {
			t.Fatal(err)
		}
		y.text = corrupt(y.text)
		if err := y.save(); err == nil || !strings.Contains(err.Error(), "nothing was written") {
			t.Errorf("%s: save = %v", name, err)
		}
		if read(t, path) != src {
			t.Errorf("%s: the file changed", name)
		}
	}
}

// A file with only comments, in CRLF, gets its new lines in CRLF.
func TestAnAddedChainKeepsTheFilesLineEnding(t *testing.T) {
	dir, path := writeGolden(t, "# nothing yet\r\n# more\r\n", 0o644)
	if err := saveKnobs(t, dir); err != nil {
		t.Fatal(err)
	}
	want := "# nothing yet\r\n# more\r\ndefaults:\r\n  test_timeout: 45s\r\n  retry_failed_tests: 2\r\n"
	if got := read(t, path); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// inWS puts a in the workspace dir, as opening it in the window does, and returns it.
func inWS(a *App, dir string) *App {
	a.workspace = dir
	return a
}

// A crafted call cannot make Studio write the probe.yaml of any folder: only the workspace that is open, however its path is spelled.
func TestSettingsAreSavedOnlyToTheOpenWorkspace(t *testing.T) {
	open, other := t.TempDir(), t.TempDir()
	a := &App{workspace: open}
	if err := a.SaveWorkspaceSettings(other, WorkspaceSettings{AgentPort: 9}); err == nil || !strings.Contains(err.Error(), "only to the workspace that is open") {
		t.Errorf("another folder: %v", err)
	}
	if err := a.SaveAISettings(other, AISettings{Provider: "none", Vision: true, KeySource: "keychain"}); err == nil || !strings.Contains(err.Error(), "only to the workspace that is open") {
		t.Errorf("another folder (AI): %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "probe.yaml")); err == nil {
		t.Error("a probe.yaml was written in the other folder")
	}
	if err := (&App{}).SaveWorkspaceSettings(open, WorkspaceSettings{AgentPort: 9}); err == nil || !strings.Contains(err.Error(), "open a workspace first") {
		t.Errorf("no workspace open: %v", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(open, link); err == nil {
		if err := a.SaveWorkspaceSettings(link, WorkspaceSettings{AgentPort: 9}); err != nil {
			t.Errorf("the open workspace by a link to it was refused: %v", err)
		}
	}
	if err := a.SaveWorkspaceSettings(open+"/.", WorkspaceSettings{AgentPort: 10}); err != nil {
		t.Errorf("the open workspace spelled with a dot was refused: %v", err)
	}
}

// A value that is not a string is compared as written: a current value of "" must not make an int look already set and report success.
func TestAnIntIsNotTakenForAnEmptyString(t *testing.T) {
	dir, path := writeGolden(t, "defaults:\n  retry_failed_tests: \"\"\n", 0o644)
	y, err := openProbeYAML(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := y.set(5, "defaults", "retry_failed_tests"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(y.text, "retry_failed_tests: 5") {
		t.Errorf("the int was not written: %q", y.text)
	}
	if err := y.set(true, "defaults", "flag"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(y.text, "flag: true") {
		t.Errorf("the bool was not written: %q", y.text)
	}
	_ = path
}

// A top-level key added to a multi-document file goes into the first document, before the marker, and the other documents are untouched.
func TestATopLevelKeyInAMultiDocumentFile(t *testing.T) {
	src := "defaults:\n  timeout: 30s\n---\nother: 1\n"
	dir, path := writeGolden(t, src, 0o644)
	a := inWS(&App{}, dir)
	loaded, _ := a.LoadWorkspaceSettings(dir)
	form := loaded
	form.Base = &loaded
	form.AgentPort = 4444
	if err := a.SaveWorkspaceSettings(dir, form); err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got, want := read(t, path), "defaults:\n  timeout: 30s\nagent:\n  port: 4444\n---\nother: 1\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// The check before the write covers every document, not only the first, and a key nobody asked for.
func TestTheCheckCoversEveryDocumentAndExtraKeys(t *testing.T) {
	src := "defaults:\n  timeout: 30s\n---\nother: 1\n"
	for name, corrupt := range map[string]func(string) string{
		"a change in the second document":    func(s string) string { return strings.Replace(s, "other: 1", "other: 2", 1) },
		"a key added to the second document": func(s string) string { return s + "added: 1\n" },
		"an extra key in the first":          func(s string) string { return strings.Replace(s, "---", "extra: 1\n---", 1) },
	} {
		dir, path := writeGolden(t, src, 0o644)
		y, err := openProbeYAML(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := y.set("45s", "defaults", "test_timeout"); err != nil {
			t.Fatal(err)
		}
		y.text = corrupt(y.text)
		if err := y.save(); err == nil || !strings.Contains(err.Error(), "nothing was written") {
			t.Errorf("%s: save = %v", name, err)
		}
		if read(t, path) != src {
			t.Errorf("%s: the file changed", name)
		}
	}
}

// A probe.yaml the engine cannot load is not saved over, even by an edit that is itself right: Studio does not write a file it cannot vouch for.
func TestAFileTheEngineCannotLoadIsNotWritten(t *testing.T) {
	src := "defaults:\n  retry_failed_tests: many\n"
	dir, path := writeGolden(t, src, 0o644)
	a := inWS(&App{}, dir)
	err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 4444})
	if err == nil || !strings.Contains(err.Error(), "would not load") {
		t.Errorf("err = %v", err)
	}
	if read(t, path) != src {
		t.Error("the file changed")
	}
}

// Saves of different fields that start together all land: none is lost to another's read-modify-write.
func TestConcurrentSavesOfDifferentFieldsAllLand(t *testing.T) {
	for round := 0; round < 15; round++ {
		dir, path := writeGolden(t, "project:\n  app: a.b\n", 0o644)
		edits := []func(f *WorkspaceSettings){
			func(f *WorkspaceSettings) { f.AgentPort = 4001 },
			func(f *WorkspaceSettings) { f.DefaultsTimeout = "41s" },
			func(f *WorkspaceSettings) { f.TestTimeout = "42s" },
			func(f *WorkspaceSettings) { f.RetryFailedTests = 3 },
			func(f *WorkspaceSettings) { f.ImplicitWait = "5s" },
			func(f *WorkspaceSettings) { f.IOSDeviceID = "IOS-1" },
			func(f *WorkspaceSettings) { f.AndroidDeviceID = "emu-1" },
			func(f *WorkspaceSettings) { f.AIContextRunResults = true },
		}
		a := inWS(&App{}, dir)
		loaded, _ := a.LoadWorkspaceSettings(dir)
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, len(edits))
		for i, e := range edits {
			wg.Add(1)
			go func(i int, e func(*WorkspaceSettings)) {
				defer wg.Done()
				form := loaded
				form.Base = &loaded
				e(&form)
				<-start
				errs[i] = a.SaveWorkspaceSettings(dir, form)
			}(i, e)
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d save %d: %v", round, i, err)
			}
		}
		got := read(t, path)
		for _, want := range []string{"port: 4001", "timeout: 41s", "test_timeout: 42s", "retry_failed_tests: 3", "implicit_wait: 5s", "ios_device_id: IOS-1", "android_device_id: emu-1", "run_results: true"} {
			if !strings.Contains(got, want) {
				t.Fatalf("round %d: a save was lost, %q is missing:\n%s", round, want, got)
			}
		}
	}
}
