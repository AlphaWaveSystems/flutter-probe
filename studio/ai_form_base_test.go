package main

import (
	"os"
	"strings"
	"testing"
)

const aiBaseYAML = "# demo\nai:\n  provider: local   # mine\n  endpoint: http://127.0.0.1:11434/v1\n  model: old\n  timeout: 30s\nproject:\n  app: a.b\n"

func aiFormAt(t *testing.T, dir string) (*App, AISettings) {
	t.Helper()
	a := appIn(dir)
	loaded, err := a.LoadAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	return a, loaded
}

// The form is a snapshot: a field the file changed under it is not put back, and the field the person changed is written.
func TestAIFormDoesNotRevertAnEditMadeUnderIt(t *testing.T) {
	dir, path := writeGolden(t, aiBaseYAML, 0o644)
	a, loaded := aiFormAt(t, dir)
	if err := os.WriteFile(path, []byte(strings.Replace(aiBaseYAML, "model: old", "model: external", 1)), 0o644); err != nil { // an editor writes the model
		t.Fatal(err)
	}
	form := loaded
	form.Base = &loaded
	form.Timeout = "45s" // the person changes the timeout only
	if err := a.SaveAISettings(dir, form); err != nil {
		t.Fatalf("a change to a field the file did not change under the form was refused: %v", err)
	}
	got := read(t, path)
	if !strings.Contains(got, "model: external") || !strings.Contains(got, "timeout: 45s") || !strings.Contains(got, "provider: local   # mine") {
		t.Errorf("the external edit was reverted, or the change lost, or a comment dropped:\n%s", got)
	}
}

// A change to a field that changed on disk is refused with a reload message, and nothing is written.
func TestAIFormRefusesAChangeToAFieldThatChangedOnDisk(t *testing.T) {
	for name, tc := range map[string]struct {
		from, to string // what an editor changes in the file
		change   func(f *AISettings)
		field    string
	}{
		"the model":    {"model: old", "model: external", func(f *AISettings) { f.Model = "mine" }, "the model"},
		"the endpoint": {"http://127.0.0.1:11434/v1", "http://127.0.0.1:7777/v1", func(f *AISettings) { f.Endpoint = "http://127.0.0.1:9999/v1" }, "the endpoint"},
		"the timeout":  {"timeout: 30s", "timeout: 31s", func(f *AISettings) { f.Timeout = "90s" }, "the timeout"},
	} {
		dir, path := writeGolden(t, aiBaseYAML, 0o644)
		a, loaded := aiFormAt(t, dir)
		changed := strings.Replace(aiBaseYAML, tc.from, tc.to, 1)
		if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
			t.Fatal(err)
		}
		form := loaded
		form.Base = &loaded
		tc.change(&form)
		err := a.SaveAISettings(dir, form)
		if err == nil || !strings.Contains(err.Error(), "changed on disk") || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: err = %v", name, err)
		}
		if read(t, path) != changed {
			t.Errorf("%s: the file was written", name)
		}
	}
}

// With no base (an older caller) the save is what it was: the form is written as it is.
func TestAIFormWithoutABaseIsUnchanged(t *testing.T) {
	dir, path := writeGolden(t, aiBaseYAML, 0o644)
	a, loaded := aiFormAt(t, dir)
	form := loaded
	form.Model = "new"
	if err := a.SaveAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); !strings.Contains(got, "model: new") {
		t.Errorf("not written: %s", got)
	}
}

// A form with a base that equals what the file says writes nothing, also when the file has since moved on in a field the form did not touch.
func TestAIFormWithABaseAndNoChangeWritesNothing(t *testing.T) {
	dir, path := writeGolden(t, aiBaseYAML, 0o644)
	a, loaded := aiFormAt(t, dir)
	moved := strings.Replace(aiBaseYAML, "model: old", "model: moved", 1)
	if err := os.WriteFile(path, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	form := loaded
	form.Base = &loaded
	if err := a.SaveAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if read(t, path) != moved || !os.SameFile(before, after) {
		t.Error("an untouched form wrote the file")
	}
}

// The key source and its variable name are one field: changing the name while the file's key line changed under the form is refused.
func TestAIFormKeySourceIsOneField(t *testing.T) {
	src := "ai:\n  provider: openai\n  model: m\n  api_key: ${OLD_KEY}\n"
	dir, path := writeGolden(t, src, 0o644)
	a, loaded := aiFormAt(t, dir)
	changed := strings.Replace(src, "${OLD_KEY}", "${OTHER_KEY}", 1)
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	form := loaded
	form.Base = &loaded
	form.KeyEnvName = "MY_KEY"
	if err := a.SaveAISettings(dir, form); err == nil || !strings.Contains(err.Error(), "the key source") {
		t.Errorf("err = %v", err)
	}
	if read(t, path) != changed {
		t.Error("the file was written")
	}
}
