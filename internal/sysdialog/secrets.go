package sysdialog

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// knownSecrets are the environment values this process has resolved for a
// step. A test script refers to them as ${NAME}, and a run must never print
// them: the error path has always masked them with Scrub, and every other
// printed line that can carry them now goes through ScrubKnown as well.
var knownSecrets struct {
	mu     sync.Mutex
	values []string // distinct and non-empty
}

// NoteSecret records v as a secret for the rest of the process, so that
// ScrubKnown masks it wherever printed text can carry it. Printed text can also
// carry v in a form a print site produces (see secretForms), so those forms are
// recorded with it. Empty values and repeats are ignored.
func NoteSecret(v string) {
	if v == "" {
		return
	}
	knownSecrets.mu.Lock()
	defer knownSecrets.mu.Unlock()
	for _, form := range secretForms(v) {
		if !containsSecret(knownSecrets.values, form) {
			knownSecrets.values = append(knownSecrets.values, form)
		}
	}
}

// Minimum lengths for NoteEnvValue. A value from a variable whose name is not
// secret-like must be long enough not to be a common short token; a value from a
// secret-like variable is credential-shaped already, so a shorter one is noted.
const (
	minEnvSecretLen   = 8
	minNamedSecretLen = 3
)

// envSecretWords are the words that mark a variable's value as a credential when one of them is a whole token of the
// variable's name (see NameTokens). SPINNER has the token spinner, so it does not match pin, and MONKEY_COUNT has no secret
// word.
var envSecretWords = map[string]bool{
	"pass": true, "password": true, "passwd": true, "pwd": true, "pw": true, "passphrase": true, "passcode": true,
	"secret": true, "secrets": true, "token": true, "tokens": true, "otp": true, "pin": true, "key": true,
	"cred": true, "credential": true, "credentials": true, "csrf": true, "sid": true, "signing": true, "encryption": true,
}

// plainWords are values so common that masking them would corrupt ordinary
// output. A value of under eight characters that is one of them is never noted, whatever its name.
var plainWords = map[string]bool{
	"true": true, "false": true, "null": true, "none": true,
	"yes": true, "no": true, "on": true, "off": true,
}

// NoteEnvValue records the value of the environment variable name for masking, when the value is a credential by its
// length, its plainness and the words of its name. A value of eight characters or more is noted. A shorter value is noted
// only when the name has a secret word as a whole token (see NameTokens), the value has three characters or more, and the
// value is not a plain word. So a value such as "1" is never masked wherever it appears, PIN=1234 and API_KEY=abc are
// masked from their names, and SPINNER=abc is not, because spinner is not a secret word.
func NoteEnvValue(name, v string) {
	if !envValueIsSecret(name, v) {
		return
	}
	NoteSecret(v)
}

// envValueIsSecret reports whether a value is noted for masking, by its length, its plainness and the words of its name.
func envValueIsSecret(name, v string) bool {
	if len(v) >= minEnvSecretLen {
		return true
	}
	if plainWords[strings.ToLower(v)] || len(v) < minNamedSecretLen {
		return false
	}
	for _, tok := range NameTokens(name) {
		if envSecretWords[tok] {
			return true
		}
	}
	return false
}

// NameTokens splits a name into lowercase words. A character that is neither a letter nor a digit ends a word, so the
// dash, the underscore, the dot and the brackets of user[password] all separate. A word also ends where letters meet
// digits (passwd2 is passwd and 2), at a lowercase letter followed by an uppercase one (currentPassword), and inside a run
// of capitals before a lowercase letter (APIKey is API and Key).
func NameTokens(name string) []string {
	var tokens []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if len(word) > 0 {
			prev := word[len(word)-1]
			switch {
			case unicode.IsDigit(r) != unicode.IsDigit(prev):
				flush()
			case unicode.IsLower(prev) && unicode.IsUpper(r):
				flush()
			case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
				flush()
			}
		}
		word = append(word, r)
	}
	flush()
	return tokens
}

// secretForms returns v and the forms of v that printed text takes once a print
// site or a recorded body formats it: the inner text of strconv.Quote(v), which
// is what %q prints between the quotes, url.QueryEscape(v), url.PathEscape(v),
// and the inner text of json.Marshal(v), where <, > and & are \u escapes. The
// result is distinct and non-empty, and it starts with v.
func secretForms(v string) []string {
	quoted := strconv.Quote(v)
	candidates := []string{v, quoted[1 : len(quoted)-1], url.QueryEscape(v), url.PathEscape(v), jsonInner(v)}
	forms := make([]string, 0, len(candidates))
	for _, c := range candidates {
		// Equivalent mutants: v is not empty, and each candidate is derived from it, so no candidate is empty, and a repeat is
		// also dropped by NoteSecret's own check. Removing either check changes the list's length, not what ScrubKnown masks.
		if c != "" && !containsSecret(forms, c) {
			forms = append(forms, c)
		}
	}
	return forms
}

// jsonInner is the inner text of json.Marshal(v): what a JSON body holds between
// the quotes. It is empty when v cannot be marshalled.
func jsonInner(v string) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) < 2 {
		return ""
	}
	return string(b[1 : len(b)-1])
}

func containsSecret(values []string, v string) bool {
	for _, s := range values {
		if s == v {
			return true
		}
	}
	return false
}

// ScrubKnown is Scrub applied to every value NoteSecret has recorded. Use it on
// any text that can carry agent text or a resolved step value before the text
// is printed, written to a report or a recording, or sent to a model. The
// longest secrets go first, so a short secret that is a prefix of a longer one
// cannot leave the rest of the longer one readable.
func ScrubKnown(s string) string {
	knownSecrets.mu.Lock()
	known := append([]string(nil), knownSecrets.values...)
	knownSecrets.mu.Unlock()
	if len(known) == 0 {
		return s
	}
	sort.SliceStable(known, func(i, j int) bool { return len(known[i]) > len(known[j]) })
	return Scrub(s, known...)
}
