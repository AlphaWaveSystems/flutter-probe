package main

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A server can echo back what was sent: a token in an error page, a header in a debug response, a Location that carries the query.
// The redactor finds credentials by name and by shape; a value typed under a name it does not know, with no shape, would get through.
// So everything that was typed is remembered for the one call, and wherever it comes back (body, headers, address, redirects) it is
// replaced, after hidden characters are dropped and before anything is cut.

const (
	typedMinLen    = 3   // a shorter value would be replaced everywhere
	typedMaxSecret = 200 // the most values remembered for one call
	typedMaxLen    = 4096
)

var schemeWords = map[string]bool{"bearer": true, "basic": true, "token": true, "digest": true, "ntlm": true, "negotiate": true, "apikey": true, "bot": true, "hmac": true}

// looksSecret is a token that has the look of a credential: eight characters or more, with a letter and a digit. It is a guess, and a
// wrong one only hides a harmless value.
func looksSecret(tok string) bool {
	if len(tok) < 8 || len(tok) > typedMaxLen {
		return false
	}
	letter, digit := false, false
	for _, r := range tok {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letter = true
		}
	}
	return letter && digit
}

var tokenSplit = regexp.MustCompile(`[\s"',;:{}\[\]()<>=&?/\\|]+`)
var assignment = regexp.MustCompile(`([A-Za-z0-9_.\-]+)["']?\s*[:=]\s*["']?([^"'&,\s}\]]+)`)

// typedValues are the values typed for one call.
type typedValues struct {
	items map[string]bool
}

func (t *typedValues) add(v string) {
	v = strings.TrimSpace(v)
	if len(v) < typedMinLen || len(v) > typedMaxLen || len(t.items) >= typedMaxSecret {
		return
	}
	t.items[v] = true
	// The forms a server may echo it in.
	if q := url.QueryEscape(v); q != v {
		t.items[q] = true
	}
	if q := url.PathEscape(v); q != v {
		t.items[q] = true
	}
}

// addFromJSON remembers every string and number under a credential name, however deep, and a string anywhere that looks like one.
func (t *typedValues) addFromJSON(v any, secret bool) {
	switch x := v.(type) {
	case map[string]any:
		for name, val := range x {
			t.addFromJSON(val, isSecretName(name))
		}
	case []any:
		for _, val := range x {
			t.addFromJSON(val, secret)
		}
	case string:
		if secret || looksSecret(x) {
			t.add(x)
		}
		for _, tok := range tokenSplit.Split(x, -1) {
			if looksSecret(tok) {
				t.add(tok)
			}
		}
	case float64:
		if secret {
			t.add(strconv.FormatFloat(x, 'f', -1, 64))
		}
	}
}

// collectTyped remembers what was typed: header values (the whole value and each token of it) under a credential name, and any token
// that looks like a credential; the query values of the address under a credential name and any that look like one; and the same from
// the body, including the value of a name: value or name=value pair whose name is a credential name.
func collectTyped(rawURL string, headers map[string]string, body string) *typedValues {
	t := &typedValues{items: map[string]bool{}}
	for name, value := range headers {
		family := isSecretName(name)
		if family {
			t.add(value)
		}
		for _, tok := range strings.Fields(value) {
			if schemeWords[strings.ToLower(tok)] {
				continue
			}
			if family || looksSecret(tok) {
				t.add(tok)
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		for name, vals := range u.Query() {
			for _, v := range vals {
				if isSecretName(name) || looksSecret(v) {
					t.add(v)
				}
			}
		}
		for _, seg := range strings.Split(u.Path, "/") {
			if looksSecret(seg) {
				t.add(seg)
			}
		}
	}
	var doc any
	if json.Unmarshal([]byte(body), &doc) == nil {
		// Whole values, spaces and all, and the tokens inside each string. The raw text is not read: its escapes (\u2028) are not what the
		// server echoes, and a token made of one would be a false find.
		t.addFromJSON(doc, false)
		return t
	} else if form, err := url.ParseQuery(body); err == nil && strings.Contains(body, "=") {
		for name, vals := range form {
			for _, v := range vals {
				if isSecretName(name) {
					t.add(v)
				}
			}
		}
	}
	for _, m := range assignment.FindAllStringSubmatch(body, -1) {
		if isSecretName(m[1]) && !schemeWords[strings.ToLower(m[2])] {
			t.add(m[2])
		}
	}
	for _, tok := range tokenSplit.Split(body, -1) {
		if looksSecret(tok) {
			t.add(tok)
		}
	}
	return t
}

// scrub drops hidden characters from s and replaces every remembered value in it, whatever its case. The longest values go first, so a
// value that holds another is replaced whole.
func (t *typedValues) scrub(s string) string {
	if t == nil || len(t.items) == 0 || s == "" {
		return s
	}
	s = withoutInvisible(s, true)
	vals := make([]string, 0, len(t.items))
	for v := range t.items {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		return len(vals[i]) > len(vals[j]) || (len(vals[i]) == len(vals[j]) && vals[i] < vals[j])
	})
	for _, v := range vals {
		s = replaceFold(s, v, redactedMark)
	}
	return s
}

// lowerASCII lower-cases ASCII letters only, so the length of the text never changes.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// replaceFold replaces every occurrence of old in s, ignoring the case of ASCII letters.
func replaceFold(s, old, with string) string {
	if old == "" {
		return s
	}
	lo, lold := lowerASCII(s), lowerASCII(old)
	var out strings.Builder
	i := 0
	for {
		j := strings.Index(lo[i:], lold)
		if j < 0 {
			out.WriteString(s[i:])
			return out.String()
		}
		out.WriteString(s[i : i+j])
		out.WriteString(with)
		i += j + len(old)
	}
}
