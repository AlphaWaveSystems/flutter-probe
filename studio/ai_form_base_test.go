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

const aiEveryFieldYAML = "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: old-model\n  timeout: 30s\n  api_key: ${OLD_KEY}\n  vision: false\nstudio:\n  ai_allow_remote_endpoint: false\n"

// Every field of the form: a change to it is written, and the same change is refused, naming only the field, when the file changed that field to
// something else under the form. Each case also checks that an edit to another field is kept.
func TestAIFormEveryFieldIsWrittenOrRefusedOnConflict(t *testing.T) {
	type tc struct {
		name     string
		change   func(f *AISettings) // the person's change
		external func(src string) string
		other    func(src string) string // an edit made elsewhere to a different field
		written  string                  // what the file holds after the person's change
		field    string                  // what the refusal names
		same2    bool                    // a two-valued field: what the file changed it to is what the person asked for, so there is no conflict
	}
	cases := []tc{
		{"provider", func(f *AISettings) { f.Provider = "openai"; f.Endpoint = "" }, func(s string) string { return strings.Replace(s, "provider: local", "provider: anthropic", 1) },
			func(s string) string { return strings.Replace(s, "timeout: 30s", "timeout: 41s", 1) }, "provider: openai", "the AI provider", false},
		{"endpoint", func(f *AISettings) { f.Endpoint = "http://127.0.0.1:9999/v1" }, func(s string) string { return strings.Replace(s, ":11434", ":7777", 1) },
			func(s string) string { return strings.Replace(s, "timeout: 30s", "timeout: 41s", 1) }, "endpoint: http://127.0.0.1:9999/v1", "the endpoint", false},
		{"model", func(f *AISettings) { f.Model = "my-model" }, func(s string) string { return strings.Replace(s, "old-model", "external-model", 1) },
			func(s string) string { return strings.Replace(s, "timeout: 30s", "timeout: 41s", 1) }, "model: my-model", "the model", false},
		{"key source", func(f *AISettings) { f.KeyEnvName = "MY_KEY" }, func(s string) string { return strings.Replace(s, "${OLD_KEY}", "${EXTERNAL_KEY}", 1) },
			func(s string) string { return strings.Replace(s, "timeout: 30s", "timeout: 41s", 1) }, "api_key: ${MY_KEY}", "the key source", false},
		{"timeout", func(f *AISettings) { f.Timeout = "90s" }, func(s string) string { return strings.Replace(s, "timeout: 30s", "timeout: 31s", 1) },
			func(s string) string { return strings.Replace(s, "old-model", "other-model", 1) }, "timeout: 90s", "the timeout", false},
		{"vision", func(f *AISettings) { f.Vision = true }, func(s string) string { return strings.Replace(s, "vision: false\n", "", 1) },
			func(s string) string { return strings.Replace(s, "old-model", "other-model", 1) }, "", "the vision switch", true},
		{"remote", func(f *AISettings) { f.AllowRemoteEndpoint = true }, func(s string) string {
			return strings.Replace(s, "ai_allow_remote_endpoint: false", "ai_allow_remote_endpoint: true", 1)
		},
			func(s string) string { return strings.Replace(s, "old-model", "other-model", 1) }, "ai_allow_remote_endpoint: true", "the remote-endpoint switch", true},
	}
	for _, c := range cases {
		// the change is written, and an edit made elsewhere under the form is kept
		dir, path := writeGolden(t, aiEveryFieldYAML, 0o644)
		a, loaded := aiFormAt(t, dir)
		if err := os.WriteFile(path, []byte(c.other(aiEveryFieldYAML)), 0o644); err != nil {
			t.Fatal(err)
		}
		form := loaded
		form.Base = &loaded
		c.change(&form)
		if err := a.SaveAISettings(dir, form); err != nil {
			t.Errorf("%s: a change was refused: %v", c.name, err)
			continue
		}
		got := read(t, path)
		if c.name == "vision" && strings.Contains(got, "vision: false") {
			t.Errorf("vision: the switch back on did not remove the explicit false:\n%s", got)
		}
		if c.written != "" && !strings.Contains(got, c.written) {
			t.Errorf("%s: the change was not written (want %q):\n%s", c.name, c.written, got)
		}
		if other := c.other(aiEveryFieldYAML); strings.Contains(other, "timeout: 41s") && !strings.Contains(got, "timeout: 41s") || strings.Contains(other, "other-model") && !strings.Contains(got, "other-model") {
			t.Errorf("%s: the edit made elsewhere was reverted:\n%s", c.name, got)
		}
		// the same field changed on disk: refused, nothing written
		dir, path = writeGolden(t, aiEveryFieldYAML, 0o644)
		a, loaded = aiFormAt(t, dir)
		changed := c.external(aiEveryFieldYAML)
		if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
			t.Fatal(err)
		}
		form = loaded
		form.Base = &loaded
		c.change(&form)
		err := a.SaveAISettings(dir, form)
		if c.same2 {
			// base false, the file says true, the person asked for true: nothing is lost, so it is accepted
			if err != nil {
				t.Errorf("%s: a two-valued change the file already has was refused: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "changed on disk") || !strings.Contains(err.Error(), c.field) {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if read(t, path) != changed {
			t.Errorf("%s: the file was written", c.name)
		}
	}
}

// The refusal names the field and carries no value: not the one on disk, not the person's, not the base's, and no key.
func TestAIFormRefusalCarriesNoValue(t *testing.T) {
	src := "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: base-model-QZ1\n  api_key: ${BASE_KEY_QZ2}\n"
	dir, path := writeGolden(t, src, 0o644)
	a, loaded := aiFormAt(t, dir)
	changed := strings.NewReplacer("base-model-QZ1", "disk-model-QZ3", "${BASE_KEY_QZ2}", "${DISK_KEY_QZ4}").Replace(src)
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(f *AISettings){
		"the model":      func(f *AISettings) { f.Model = "form-model-QZ5" },
		"the key source": func(f *AISettings) { f.KeyEnvName = "FORM_KEY_QZ6" },
	} {
		form := loaded
		form.Base = &loaded
		change(&form)
		err := a.SaveAISettings(dir, form)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		for _, v := range []string{"QZ1", "QZ2", "QZ3", "QZ4", "QZ5", "QZ6"} {
			if strings.Contains(err.Error(), v) {
				t.Errorf("%s: the refusal carries a value (%s): %v", name, v, err)
			}
		}
	}
}

// If the file already says what the person asked for, nothing is lost: the change is accepted.
func TestAIFormAcceptsAChangeTheFileAlreadyHas(t *testing.T) {
	dir, path := writeGolden(t, aiBaseYAML, 0o644)
	a, loaded := aiFormAt(t, dir)
	moved := strings.Replace(aiBaseYAML, "model: old", "model: same", 1) // an editor sets the model the person is about to set
	if err := os.WriteFile(path, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	form := loaded
	form.Base = &loaded
	form.Model = "same"
	if err := a.SaveAISettings(dir, form); err != nil {
		t.Fatalf("a change the file already has was refused: %v", err)
	}
	if read(t, path) != moved {
		t.Error("the file changed although it already had the value")
	}
}
