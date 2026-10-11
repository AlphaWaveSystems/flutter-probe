package l10n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalog(t *testing.T) *Catalog {
	dir := t.TempDir()
	write(t, dir, "app_en.arb", `{"@@locale":"en","save":"Save","title":"Settings","greet":"Hello {name}","@save":{"description":"x"}}`)
	write(t, dir, "app_de.arb", `{"@@locale":"de","save":"Speichern","title":"Einstellungen"}`)
	write(t, dir, "my_app_pt_BR.arb", `{"save":"Salvar"}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadAndLookup(t *testing.T) {
	c := catalog(t)
	if got := strings.Join(c.Languages(), ","); got != "de,en,pt-BR" {
		t.Fatalf("languages = %s", got)
	}
	cases := []struct{ key, tag, want string }{
		{"save", "de", "Speichern"},
		{"save", "de-DE", "Speichern"}, // falls back to the base language
		{"save", "pt-BR", "Salvar"},
		{"save", "pt", "Save"}, // no pt file: default
		{"title", "pt-BR", "Settings"},
		{"save", "ja", "Save"},
	}
	for _, tc := range cases {
		got, err := c.Lookup(tc.key, tc.tag, "en")
		if err != nil || got != tc.want {
			t.Errorf("Lookup(%q,%q) = %q, %v; want %q", tc.key, tc.tag, got, err, tc.want)
		}
	}
}

func TestLookupErrors(t *testing.T) {
	c := catalog(t)
	if _, err := c.Lookup("nope", "de", "en"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing key: %v", err)
	}
	if _, err := c.Lookup("greet", "en", "en"); err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Errorf("placeholder message: %v", err)
	}
	if _, err := c.Lookup("save", "ja", ""); err == nil {
		t.Error("no ARB for the language and no default must fail")
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	s := "tap " + Marker("save") + " and " + Marker("title")
	if got := strings.Join(Keys(s), ","); got != "save,title" {
		t.Fatalf("keys = %s", got)
	}
	c := catalog(t)
	out, err := Expand(s, func(k string) (string, error) { return c.Lookup(k, "de", "en") })
	if err != nil || out != "tap Speichern and Einstellungen" {
		t.Fatalf("Expand = %q, %v", out, err)
	}
	out, err = Expand(Marker("zzz"), func(k string) (string, error) { return c.Lookup(k, "de", "en") })
	if err == nil || out != "zzz" {
		t.Fatalf("missing key must keep the key and report: %q, %v", out, err)
	}
}

func TestCheck(t *testing.T) {
	c := catalog(t)
	if errs := c.Check("title", []string{"de", "pt-BR", "en"}, "en"); len(errs) != 0 {
		t.Fatalf("title resolves everywhere via fallback: %v", errs)
	}
	if errs := c.Check("greet", []string{"en"}, "en"); len(errs) != 1 {
		t.Fatalf("placeholder message must be reported: %v", errs)
	}
}
