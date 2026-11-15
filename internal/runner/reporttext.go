package runner

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// Limits of the tags and run properties a report carries.
const (
	MaxProperties         = 32   // --property entries per run
	MaxPropertyValueBytes = 1024 // bytes in one --property value
	MaxPropertyKeyBytes   = 64   // bytes in one --property key
	MaxTags               = 32   // tags reported per test; the rest are left out
	MaxTagRunes           = 64   // characters of one tag; a longer tag is cut
)

var propertyKey = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// reservedProperties are the names the reports already write themselves; a run property of the same name would be a second
// property of that name next to the built-in one.
var reservedProperties = map[string]bool{
	"probe_version": true, "agent_version": true, "platform": true, "os_version": true, "app_id": true,
	"app_version": true, "device_name": true, "device_class": true, "provider": true, "locale": true,
	"run_label": true, "tags": true, "row": true, "device": true, "attempt": true, "attempts": true,
}

// Property is one run property of a report.
type Property struct {
	Name  string
	Value string
}

// ParseProperties reads the `--property key=value` entries. A bad entry is refused with a message that names the rule and
// the entry's position, never the entry itself (the value can be a secret).
func ParseProperties(pairs []string) (map[string]string, error) {
	if len(pairs) > MaxProperties {
		return nil, fmt.Errorf("--property: at most %d properties are allowed", MaxProperties)
	}
	out := make(map[string]string, len(pairs))
	for i, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok || key == "" || len(key) > MaxPropertyKeyBytes || !propertyKey.MatchString(key) {
			return nil, fmt.Errorf("--property: entry %d is not key=value with a key of 1 to %d letters, digits, _ . or -", i+1, MaxPropertyKeyBytes)
		}
		if reservedProperties[key] {
			return nil, fmt.Errorf("--property: the key %q is used by the report itself", key)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("--property: the key %q is given twice", key)
		}
		if len(value) > MaxPropertyValueBytes {
			return nil, fmt.Errorf("--property: the value of %q is longer than %d bytes", key, MaxPropertyValueBytes)
		}
		out[key] = value
	}
	return out, nil
}

// ReportProperties is what the reports write for the run properties: sorted by name, values masked and made safe to write
// (see reportText). It is called when a report is written, so a secret the run registered after the flags were read is masked too.
// An entry with a key or value the limits refuse is left out; ParseProperties already refused those, this guards a hand-edited file.
func ReportProperties(m map[string]string) []Property {
	keys := make([]string, 0, len(m))
	for k := range m {
		if propertyKey.MatchString(k) && len(k) <= MaxPropertyKeyBytes && !reservedProperties[k] && len(m[k]) <= MaxPropertyValueBytes {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > MaxProperties {
		keys = keys[:MaxProperties]
	}
	out := make([]Property, 0, len(keys))
	for _, k := range keys {
		out = append(out, Property{Name: k, Value: reportText(m[k])})
	}
	return out
}

// ReportTags is what the reports write for a test's tags: masked, made safe to write, empty ones dropped, each cut to
// MaxTagRunes characters, at most MaxTags of them.
func ReportTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(reportText(t))
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > MaxTagRunes {
			t = string([]rune(t)[:MaxTagRunes])
		}
		out = append(out, t)
		if len(out) == MaxTags {
			break
		}
	}
	return out
}

// reportText masks the values the run knows are secrets, then replaces every character XML 1.0 cannot carry (and every
// control or line-separator character, so a value stays on one line) with a space.
func reportText(s string) string {
	s = sysdialog.ScrubKnown(s)
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == utf8.RuneError && size == 1,
			unicode.IsControl(r), r == '\u2028', r == '\u2029',
			r >= 0xFDD0 && r <= 0xFDEF, r&0xFFFE == 0xFFFE:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
