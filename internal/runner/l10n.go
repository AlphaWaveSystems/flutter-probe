package runner

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/alphawavesystems/flutter-probe/internal/l10n"
)

var (
	l10nMu       sync.RWMutex
	l10nCatalog  *l10n.Catalog
	l10nDefault  string // probe.yaml l10n.default: the fallback language of every lookup
	l10nRunLang  string // --locale: the language the run starts in
	l10nSetLangs = regexp.MustCompile(`set\s+language\s+"([^"]+)"`)
	l10nRefs     = regexp.MustCompile(`l10n[ \t]+"([^"]+)"`)
)

// SetL10n installs the ARB catalog `l10n "key"` resolves from. def is the
// language used until `set language` / --locale picks one (probe.yaml l10n.default).
func SetL10n(c *l10n.Catalog, def string) {
	l10nMu.Lock()
	defer l10nMu.Unlock()
	l10nCatalog, l10nDefault = c, def
}

// SetL10nLanguage sets the language `l10n "key"` uses when the device context has none yet (--locale).
func SetL10nLanguage(tag string) {
	l10nMu.Lock()
	defer l10nMu.Unlock()
	l10nRunLang = tag
}

// L10nLookup returns the ARB text of key for tag.
func L10nLookup(key, tag string) (string, error) {
	l10nMu.RLock()
	c, def, run := l10nCatalog, l10nDefault, l10nRunLang
	l10nMu.RUnlock()
	if c == nil {
		return "", fmt.Errorf("l10n %q: probe.yaml has no l10n.dir (the folder with your .arb files)", key)
	}
	if tag == "" {
		tag = run
	}
	if tag == "" {
		tag = def
	}
	return c.Lookup(key, tag, def)
}

// ValidateL10n reads the test files and fails when a `l10n "key"` cannot be
// resolved for the languages the run will use: lang (--locale or l10n.default)
// and every `set language "x"` literal in the files.
func ValidateL10n(files []string, lang string) error {
	keys := map[string]bool{}
	langs := map[string]bool{}
	if lang != "" {
		langs[lang] = true
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range l10nRefs.FindAllStringSubmatch(string(b), -1) {
			keys[m[1]] = true
		}
		for _, m := range l10nSetLangs.FindAllStringSubmatch(string(b), -1) {
			if !strings.EqualFold(m[1], "system") && !strings.EqualFold(m[1], "default") {
				langs[m[1]] = true
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	l10nMu.RLock()
	c, def, run := l10nCatalog, l10nDefault, l10nRunLang
	l10nMu.RUnlock()
	if run != "" {
		langs[run] = true
	}
	if c == nil {
		return fmt.Errorf("tests use l10n \"...\" but probe.yaml has no l10n.dir (the folder with your .arb files)")
	}
	var langList []string
	for l := range langs {
		langList = append(langList, l)
	}
	if len(langList) == 0 {
		langList = []string{def}
	}
	sort.Strings(langList)
	var keyList []string
	for k := range keys {
		keyList = append(keyList, k)
	}
	sort.Strings(keyList)
	var problems []string
	for _, k := range keyList {
		for _, err := range c.Check(k, langList, def) {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("l10n keys do not resolve:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
