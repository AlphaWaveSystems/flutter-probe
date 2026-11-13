package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedIn writes content into a new workspace and returns the workspace and the file.
func seedIn(t *testing.T, content string) (ws, file string) {
	t.Helper()
	ws = t.TempDir()
	file = filepath.Join(ws, "seed.json")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws, file
}

func TestSeededResultsReadsAFinishedRun(t *testing.T) {
	ws, p := seedIn(t, `[{"name":"sign in","file":"tests/login.probe","error":"no widget"},{"name":"ok","passed":true}]`)
	got, note := seededResults(true, p, ws)
	if note != "" || len(got) != 2 || got[0].Name != "sign in" || got[0].Error != "no widget" || !got[1].Passed {
		t.Fatalf("results %+v, note %q", got, note)
	}
}

// Only the headless build reads it, so a person running the app can never start with a run they did not make.
func TestSeededResultsIsIgnoredOutsideTheHeadlessBuild(t *testing.T) {
	ws, p := seedIn(t, `[{"name":"a","error":"x"}]`)
	got, note := seededResults(false, p, ws)
	if len(got) != 0 || !strings.Contains(note, "headless build only") {
		t.Fatalf("results %+v, note %q", got, note)
	}
}

func TestSeededResultsNeedsNoVariable(t *testing.T) {
	if got, note := seededResults(true, "", ""); got != nil || note != "" {
		t.Fatalf("results %v, note %q", got, note)
	}
}

// The file must be inside the workspace once links are resolved: another folder, a link inside the workspace that points out, a
// workspace that is not set, and the workspace itself are all refused, and the note never names the path.
func TestSeededResultsMustBeInsideTheWorkspace(t *testing.T) {
	const content = `[{"name":"seedmarker7731","error":"x"}]`
	ws, _ := seedIn(t, content)
	otherWS, other := seedIn(t, content)
	_ = otherWS

	link := filepath.Join(ws, "link.json")
	if err := os.Symlink(other, link); err != nil {
		t.Skip("symbolic links are not available here")
	}
	linkDir := filepath.Join(ws, "linkdir")
	if err := os.Symlink(filepath.Dir(other), linkDir); err != nil {
		t.Fatal(err)
	}
	// Inside by name, outside by resolution: the workspace itself is reached through a link, and the file is a sibling of it.
	sibling := t.TempDir()
	siblingFile := filepath.Join(sibling, "seed.json")
	if err := os.WriteFile(siblingFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	dotdot := filepath.Join(ws, "..", filepath.Base(sibling), "seed.json")

	cases := map[string]struct{ path, ws string }{
		"a file in another folder":              {other, ws},
		"a link inside that points out":         {link, ws},
		"a link to a folder outside":            {filepath.Join(linkDir, "seed.json"), ws},
		"a path that climbs out with ..":        {dotdot, ws},
		"the workspace itself":                  {ws, ws},
		"no workspace set":                      {other, ""},
		"a workspace that does not exist":       {other, filepath.Join(ws, "absent")},
		"a file inside a workspace that is not": {filepath.Join(ws, "seed.json"), other},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, note := seededResults(true, c.path, c.ws)
			if len(got) != 0 || !strings.Contains(note, "PROBE_STUDIO_SEED_RUN was ignored") {
				t.Fatalf("results %d, note %q", len(got), note)
			}
			if strings.Contains(note, c.path) || strings.Contains(note, "seedmarker7731") || strings.Contains(note, ws) {
				t.Fatalf("the note names a path or quotes the file: %q", note)
			}
		})
	}

	// A link inside the workspace that points at another file inside it is fine.
	inside := filepath.Join(ws, "inside.json")
	if err := os.Symlink(filepath.Join(ws, "seed.json"), inside); err != nil {
		t.Fatal(err)
	}
	if got, note := seededResults(true, inside, ws); len(got) != 1 || note != "" {
		t.Fatalf("a link to a file inside the workspace: %d results, %q", len(got), note)
	}
}

// The file is read strictly and bounded, and the note says why in words without quoting the file.
func TestSeededResultsIgnoresABadFileWithANote(t *testing.T) {
	many := "[" + strings.Repeat(`{"name":"a"},`, maxSeedResults) + `{"name":"a"}]`
	cases := map[string]struct{ content, want string }{
		"not json":               {"seedmarker7731", "not a JSON list"},
		"not a list":             {`{"name":"seedmarker7731"}`, "not a JSON list"},
		"an unknown field":       {`[{"name":"a","seedmarker7731":1}]`, "not a JSON list"},
		"a wrong type":           {`[{"name":"a","passed":"seedmarker7731"}]`, "not a JSON list"},
		"more after the list":    {`[{"name":"a"}] seedmarker7731`, "more after the list"},
		"a second list after it": {`[{"name":"a"}][{"name":"seedmarker7731"}]`, "more after the list"},
		"too large":              {`[{"name":"` + strings.Repeat("x", maxSeedBytes) + `"}]`, "larger than 1 MB"},
		"too many":               {many, "more than 50 tests"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ws, p := seedIn(t, c.content)
			got, note := seededResults(true, p, ws)
			if len(got) != 0 || !strings.Contains(note, "PROBE_STUDIO_SEED_RUN was ignored") || !strings.Contains(note, c.want) {
				t.Fatalf("results %d, note %q, want %q", len(got), note, c.want)
			}
			if strings.Contains(note, "seedmarker7731") || strings.Contains(note, p) {
				t.Fatalf("the note quotes the file or names the path: %q", note)
			}
		})
	}
	t.Run("a directory and a missing file", func(t *testing.T) {
		ws, _ := seedIn(t, "[]")
		sub := filepath.Join(ws, "dir")
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{sub, filepath.Join(ws, "absent.json")} {
			if got, note := seededResults(true, p, ws); len(got) != 0 || !strings.Contains(note, "was ignored") {
				t.Fatalf("%s: results %d, note %q", p, len(got), note)
			}
		}
	})
}

func TestSeedRunMakesTheLastRun(t *testing.T) {
	a := &App{}
	a.seedRun([]RunResult{{Name: "a", Error: "boom"}})
	st := a.RunState()
	if st.Running || !st.Finished || len(st.Results) != 1 || st.Results[0].Name != "a" {
		t.Fatalf("state %+v", st)
	}
}

// A seeded run is hostile data like any run: error text with markup, hidden and glyphless characters and credential-shaped text
// goes through Explain's preview and request as it would from a real run, and the card's text is what the model answered, cleaned.
func TestSeededHostileErrorTextIsTreatedAsHostile(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	hostile := "<img src=x onerror=alert(1)> " + split(token, 0x2D7F) + " " + split("jane.doe@example.com", 0xE000) + " /Users/al⁥ice/x \x1b[31m"
	ws, p := seedIn(t, `[{"name":"<b>t</b>","file":"/Users/alice/app/tests/a.probe","error":`+jsonString(hostile)+`}]`)
	results, note := seededResults(true, p, ws)
	if note != "" || len(results) != 1 {
		t.Fatalf("results %d, note %q", len(results), note)
	}
	prov := newExplainProvider(t, 200, "ok <script>x</script>​", 0)
	a := &App{}
	a.workspace = writeProbeYAML(t, localYAML(prov.srv.URL))
	a.seedRun(results)
	prev, err := a.PreviewExplain()
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	prov.mu.Lock()
	sent := strings.Join(prov.users, "\n")
	prov.mu.Unlock()
	for _, text := range []string{prev.Text, sent} {
		for _, bad := range []string{"ghp_", "jane", "example.com", "alice", "\x1b"} {
			if strings.Contains(text, bad) {
				t.Errorf("%q reaches the provider or the preview:\n%s", bad, text)
			}
		}
	}
	if len(got.Notes) != 1 || strings.Contains(got.Notes[0].Advice, "​") || !strings.Contains(got.Notes[0].Advice, "<script>x</script>") {
		t.Fatalf("notes %+v: the answer is data, cleaned of hidden characters and not altered otherwise", got.Notes)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
