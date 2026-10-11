// Package locale normalises the language tags used by `set language`,
// `--locale` and `--locales` and derives what each platform needs from them.
package locale

import (
	"fmt"
	"strings"
)

// Tag is a validated language tag in the spellings the platforms want.
type Tag struct {
	BCP47 string // de-DE            (Android app locales)
	POSIX string // de_DE            (iOS AppleLocale)
	Lang  string // de               (iOS AppleLanguages uses BCP47; Lang is the primary subtag)
	RTL   bool   // right-to-left script
	// System is true for the reset tags ("system", "default"): remove the per-app override.
	System bool
}

var rtlLangs = map[string]bool{"ar": true, "he": true, "iw": true, "fa": true, "ur": true, "ps": true, "sd": true, "yi": true, "dv": true, "ug": true}

// Parse accepts "de", "de-DE", "de_DE", "zh-Hans-CN", "pt_BR" and the reset
// words "system" / "default". Case is normalised (language lower, region upper,
// script title).
func Parse(s string) (Tag, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "system", "default":
		return Tag{System: true}, nil
	case "":
		return Tag{}, fmt.Errorf("language tag is empty (use e.g. \"de\", \"pt-BR\" or \"system\")")
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	lang := strings.ToLower(parts[0])
	if len(lang) < 2 || len(lang) > 3 || !alpha(lang) {
		return Tag{}, fmt.Errorf("invalid language tag %q: the first part must be a 2-3 letter language code (e.g. \"de\", \"pt-BR\")", s)
	}
	out := []string{lang}
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && alpha(p): // script
			out = append(out, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
		case (len(p) == 2 && alpha(p)) || (len(p) == 3 && digits(p)): // region
			out = append(out, strings.ToUpper(p))
		default:
			return Tag{}, fmt.Errorf("invalid language tag %q: unexpected part %q (expected script like Hans or region like DE)", s, p)
		}
	}
	return Tag{
		BCP47: strings.Join(out, "-"),
		POSIX: strings.Join(out, "_"),
		Lang:  lang,
		RTL:   rtlLangs[lang],
	}, nil
}

// ParseList splits a comma-separated list ("de, ja,ar").
func ParseList(s string) ([]Tag, error) {
	var tags []Tag
	for _, p := range strings.Split(s, ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		t, err := Parse(p)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("no languages given")
	}
	return tags, nil
}

func alpha(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return s != ""
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
