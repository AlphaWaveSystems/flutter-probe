package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/l10n"
)

func setupCatalog(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"app_en.arb": `{"@@locale":"en","save":"Save","title":"Settings","greet":"Hi {n}"}`,
		"app_de.arb": `{"@@locale":"de","save":"Speichern"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := l10n.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	SetL10n(c, "en")
	SetL10nLanguage("")
	t.Cleanup(func() { SetL10n(nil, ""); SetL10nLanguage("") })
}

func writeProbe(t *testing.T, body string) []string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "a.probe")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return []string{f}
}

func TestL10nLookupUsesRunLanguageThenDefault(t *testing.T) {
	setupCatalog(t)
	if v, err := L10nLookup("save", ""); err != nil || v != "Save" {
		t.Fatalf("default language: %q, %v", v, err)
	}
	SetL10nLanguage("de")
	if v, err := L10nLookup("save", ""); err != nil || v != "Speichern" {
		t.Fatalf("run language: %q, %v", v, err)
	}
	if v, err := L10nLookup("title", ""); err != nil || v != "Settings" {
		t.Fatalf("falls back to the default language: %q, %v", v, err)
	}
	if v, err := L10nLookup("save", "en"); err != nil || v != "Save" {
		t.Fatalf("explicit language wins: %q, %v", v, err)
	}
}

func TestValidateL10n(t *testing.T) {
	setupCatalog(t)
	ok := writeProbe(t, "test \"t\"\n  tap l10n \"save\"\n  set language \"de\"\n  see l10n \"title\"\n")
	if err := ValidateL10n(ok, ""); err != nil {
		t.Fatalf("keys resolve in en and de (title falls back): %v", err)
	}
	bad := writeProbe(t, "test \"t\"\n  tap l10n \"nope\"\n")
	if err := ValidateL10n(bad, ""); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("missing key must fail: %v", err)
	}
	ph := writeProbe(t, "test \"t\"\n  see l10n \"greet\"\n")
	if err := ValidateL10n(ph, ""); err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Fatalf("placeholder message must fail: %v", err)
	}
	none := writeProbe(t, "test \"t\"\n  tap \"Save\"\n")
	if err := ValidateL10n(none, ""); err != nil {
		t.Fatal(err)
	}
}

func TestValidateL10nNeedsCatalog(t *testing.T) {
	SetL10n(nil, "")
	if err := ValidateL10n(writeProbe(t, "test \"t\"\n  tap l10n \"save\"\n"), ""); err == nil || !strings.Contains(err.Error(), "l10n.dir") {
		t.Fatalf("no catalog must say so: %v", err)
	}
}
