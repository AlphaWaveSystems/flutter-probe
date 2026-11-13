package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSeed(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSeededResultsReadsAFinishedRun(t *testing.T) {
	p := writeSeed(t, `[{"name":"sign in","file":"tests/login.probe","error":"no widget"},{"name":"ok","passed":true}]`)
	got, note := seededResults(true, p)
	if note != "" || len(got) != 2 || got[0].Name != "sign in" || got[0].Error != "no widget" || !got[1].Passed {
		t.Fatalf("results %+v, note %q", got, note)
	}
}

// Only the headless build reads it, so a person running the app can never start with a run they did not make.
func TestSeededResultsIsIgnoredOutsideTheHeadlessBuild(t *testing.T) {
	got, note := seededResults(false, writeSeed(t, `[{"name":"a","error":"x"}]`))
	if len(got) != 0 || !strings.Contains(note, "headless build only") {
		t.Fatalf("results %+v, note %q", got, note)
	}
}

func TestSeededResultsIgnoresABadFileWithANote(t *testing.T) {
	many := "[" + strings.Repeat(`{"name":"a"},`, maxSeedResults) + `{"name":"a"}]`
	cases := map[string]string{
		"not json":    writeSeed(t, "nope"),
		"not a list":  writeSeed(t, `{"name":"a"}`),
		"missing":     filepath.Join(t.TempDir(), "absent.json"),
		"too large":   writeSeed(t, `["`+strings.Repeat("x", maxSeedBytes)+`"]`),
		"too many":    writeSeed(t, many),
		"a directory": t.TempDir(),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			got, note := seededResults(true, p)
			if len(got) != 0 || !strings.Contains(note, "PROBE_STUDIO_SEED_RUN was ignored") {
				t.Fatalf("results %d, note %q", len(got), note)
			}
			if name == "too large" && !strings.Contains(note, "larger than 1 MB") {
				t.Fatalf("a file over the limit was not refused for its size: %q", note)
			}
			if strings.Contains(note, p) {
				t.Fatalf("the note names the path: %q", note)
			}
		})
	}
}

func TestSeededResultsNeedsNoVariable(t *testing.T) {
	if got, note := seededResults(true, ""); got != nil || note != "" {
		t.Fatalf("results %v, note %q", got, note)
	}
}

func TestSeedRunMakesTheLastRun(t *testing.T) {
	a := &App{}
	a.seedRun([]RunResult{{Name: "a", Error: "boom"}})
	st := a.RunState()
	if st.Running || !st.Finished || len(st.Results) != 1 || st.Results[0].Name != "a" {
		t.Fatalf("state %+v", st)
	}
}
