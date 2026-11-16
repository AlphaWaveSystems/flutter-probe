package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The text-entry widgets of the framework (see editableWidgetLine): a secret one loses its key in the tree sent to an AI provider.
var textEntryWidgets = []string{
	"EditableText", "TextField", "TextFormField", "CupertinoTextField", "CupertinoSearchTextField", "CupertinoTextFormFieldRow",
	"SearchBar", "SearchAnchor", "DropdownMenu", "DropdownMenuFormField", "Autocomplete", "RawAutocomplete", "InputDatePickerFormField",
	"_TextField", "_CupertinoTextFieldState", "PasswordTextField", "PinCodeTextField", "MySearchField",
}

// Each of them, with a key that names a secret, is redacted, and a harmless key is kept.
func TestEveryTextEntryWidgetLosesASecretKey(t *testing.T) {
	for _, w := range textEntryWidgets {
		secret := "  " + w + "(key=[<'password'>], controller: x)"
		got := redactTree("Root\n" + secret + "\n")
		if strings.Contains(got, "password") || !strings.Contains(got, "  "+w+"(key=[redacted], obscured)") {
			t.Errorf("%s: a secret key was not redacted: %q", w, got)
		}
		plain := "  " + w + "(key=[<'email'>])"
		if got := redactTree(plain); got != plain {
			t.Errorf("%s: a harmless key changed: %q", w, got)
		}
		obscured := "    " + w + "(obscureText: true, key=[<'x'>])"
		if got := redactTree(obscured); strings.Contains(got, "'x'") {
			t.Errorf("%s: an obscured field kept its key: %q", w, got)
		}
	}
}

// Names that are not text entry are left alone (a button called PasswordButton is not a field).
func TestNonInputWidgetsAreNotTouched(t *testing.T) {
	for _, w := range []string{"TextButton", "Text", "RichText", "SelectionArea", "ElevatedButton", "PasswordButton", "SearchDelegate"} {
		line := "  " + w + "(key=[<'password'>])"
		if got := redactTree(line); got != line {
			t.Errorf("%s was redacted as an input: %q", w, got)
		}
	}
}

// catalogEntries reads the Flutter widget catalog (docs/compat/catalog.json) from the repository root.
func catalogEntries(t *testing.T) []struct{ Name, Library, Base string } {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "docs", "compat", "catalog.json"))
	if err != nil {
		t.Skipf("docs/compat/catalog.json is not in this checkout yet: %v", err)
	}
	var doc struct {
		Widgets []struct{ Name, Library, Base string } `json:"widgets"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Widgets) < 100 {
		t.Fatalf("the catalog lists %d widgets: wrong file?", len(doc.Widgets))
	}
	return doc.Widgets
}

// looksLikeInput is a broad, separate reading of "could take typed text": a catalog widget that matches it must be covered by the
// redaction or be listed below as reviewed and not an input. A new Flutter version that adds an input fails here until someone decides.
var looksLikeInput = regexp.MustCompile(`(?i)text(field|form)|searchfield|searchbar|searchanchor|editable|autocomplete|dropdownmenu|inputdate|inputdecorator|formfield|pinput|passw`)

// notInputs are catalog widgets that match looksLikeInput and do not take typed text themselves: themes and decoration around a field,
// an item of a menu, the tap region a field sits in, a non-text form field. Each was read in the Flutter API docs.
var notInputs = map[string]string{
	"InputDecorationTheme":    "a theme",
	"InputDecorator":          "decoration around a field; the field is its own line",
	"DropdownButtonFormField": "a choice from a list, no typed text",
	"FormField":               "the base class: a field has no text of its own",
}

// Every catalog widget that could take typed text is covered by the redaction, or reviewed as not one.
func TestEveryTextEntryWidgetOfTheCatalogIsCovered(t *testing.T) {
	for _, w := range catalogEntries(t) {
		if !looksLikeInput.MatchString(w.Name) {
			continue
		}
		if editableWidgetLine.MatchString(w.Name + "(") {
			continue
		}
		if _, reviewed := notInputs[w.Name]; reviewed {
			continue
		}
		t.Errorf("%s (%s, base %s) looks like a text input and is neither covered by editableWidgetLine nor reviewed in notInputs", w.Name, w.Library, w.Base)
	}
}

// The reviewed list does not rot: an entry that is no longer in the catalog, or that the redaction now covers anyway, is noise.
func TestNotInputsListIsCurrent(t *testing.T) {
	in := map[string]bool{}
	for _, w := range catalogEntries(t) {
		in[w.Name] = true
	}
	for name := range notInputs {
		if !in[name] {
			t.Errorf("notInputs lists %s, which the catalog does not have", name)
		}
	}
}
