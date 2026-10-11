// Package l10n reads Flutter ARB files so a test can say `tap l10n "saveButton"`
// and have the text in the language the app currently runs in.
//
// The lexer turns `l10n "key"` into an ordinary string whose text is Marker(key);
// the executor swaps it for the catalog's text at run time (Expand).
package l10n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/locale"
)

const (
	markStart = "\x1el10n:"
	markEnd   = "\x1f"
)

var markRe = regexp.MustCompile(markStart + `([^` + markEnd + `]*)` + markEnd)

// Marker is the placeholder text the lexer stores for `l10n "key"`.
func Marker(key string) string { return markStart + key + markEnd }

// Keys returns the l10n keys referenced in s.
func Keys(s string) []string {
	var out []string
	for _, m := range markRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// Expand replaces every marker in s with lookup(key). A lookup error leaves the
// key itself in place and is returned (first one) so the caller can report it.
func Expand(s string, lookup func(key string) (string, error)) (string, error) {
	var first error
	out := markRe.ReplaceAllStringFunc(s, func(m string) string {
		key := markRe.FindStringSubmatch(m)[1]
		v, err := lookup(key)
		if err != nil {
			if first == nil {
				first = err
			}
			return key
		}
		return v
	})
	return out, first
}

// Catalog holds the messages of every ARB file, per language.
type Catalog struct {
	byLang map[string]map[string]string // normalised BCP-47 ("de", "pt-BR") -> key -> text
}

// Load reads every *.arb file of dir. The language of a file is its "@@locale"
// entry, or else the suffix of its name (app_de.arb, app_pt_BR.arb).
func Load(dir string) (*Catalog, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.arb"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .arb files in %s", dir)
	}
	c := &Catalog{byLang: map[string]map[string]string{}}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		tag := ""
		if v, ok := doc["@@locale"].(string); ok {
			tag = v
		}
		if tag == "" {
			tag = localeFromName(filepath.Base(f))
		}
		t, err := locale.Parse(tag)
		if err != nil || t.System {
			return nil, fmt.Errorf("%s: cannot tell the language (add \"@@locale\" or name the file app_<lang>.arb)", f)
		}
		msgs := map[string]string{}
		for k, v := range doc {
			if strings.HasPrefix(k, "@") {
				continue
			}
			if s, ok := v.(string); ok {
				msgs[k] = s
			}
		}
		c.byLang[t.BCP47] = msgs
	}
	return c, nil
}

func localeFromName(base string) string {
	base = strings.TrimSuffix(base, ".arb")
	parts := strings.Split(base, "_")
	// app_pt_BR: the longest suffix of the form <lang>[_<Script>][_<REGION>].
	for i := 1; i < len(parts); i++ {
		if isLang(parts[i]) && allSubtags(parts[i+1:]) {
			return strings.Join(parts[i:], "_")
		}
	}
	return ""
}

func isLang(s string) bool {
	if len(s) < 2 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func allSubtags(parts []string) bool {
	for _, p := range parts {
		switch {
		case len(p) == 4 && p[0] >= 'A' && p[0] <= 'Z' && strings.ToLower(p[1:]) == p[1:]: // Script
		case len(p) == 2 && strings.ToUpper(p) == p: // REGION
		case len(p) == 3 && strings.Trim(p, "0123456789") == "": // UN M.49 region
		default:
			return false
		}
	}
	return true
}

// Languages lists the loaded languages, sorted.
func (c *Catalog) Languages() []string {
	var out []string
	for l := range c.byLang {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// Lookup finds key for the language tag, falling back to the tag's base
// language and then to def (and its base language). A message with ICU
// placeholders is rejected: the text on screen differs from the template.
func (c *Catalog) Lookup(key, tag, def string) (string, error) {
	var tried []string
	for _, cand := range candidates(tag, def) {
		msgs, ok := c.byLang[cand]
		if !ok {
			continue
		}
		tried = append(tried, cand)
		if v, ok := msgs[key]; ok {
			if strings.ContainsAny(v, "{}") {
				return "", fmt.Errorf("l10n %q (%s) has placeholders (%q): use the literal text on screen instead", key, cand, v)
			}
			return v, nil
		}
	}
	if len(tried) == 0 {
		return "", fmt.Errorf("l10n %q: no ARB file for language %q (loaded: %s)", key, tag, strings.Join(c.Languages(), ", "))
	}
	return "", fmt.Errorf("l10n key %q not found for %s", key, strings.Join(tried, ", "))
}

// Check verifies key resolves for every language in langs.
func (c *Catalog) Check(key string, langs []string, def string) []error {
	var errs []error
	for _, l := range langs {
		if _, err := c.Lookup(key, l, def); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func candidates(tag, def string) []string {
	var out []string
	seen := map[string]bool{}
	push := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add := func(s string) {
		if s == "" {
			return
		}
		t, err := locale.Parse(s)
		if err != nil || t.System {
			return
		}
		push(t.BCP47)
		if i := strings.Index(t.BCP47, "-"); i > 0 {
			push(t.BCP47[:i])
		}
	}
	add(tag)
	add(def)
	return out
}
