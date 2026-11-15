package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// Saving a request typed in the Send panel as a ProbeScript step. The step is the existing `call METHOD "url" [with body "..."]`, nothing
// else: if the grammar cannot say the request exactly (a method it has no word for, a body with a control character), nothing is saved.
// Every value that the Network tab's redactor would hide, and every value that looks like a credential or a person's address, is written
// as ${NAME}, which `probe test` fills from the environment and masks in its output, and the step says which names to set. Headers are
// not saved at all: the function is not given any. It reads the typed request only: not an answer, not the allowed hosts, and it makes no
// request and writes nothing; the window inserts the lines into the open editor, and saving the file is the editor's own.

// SaveStepInput is the typed request without its headers.
type SaveStepInput struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   string `json:"body"`
}

// SavedStep is what the window inserts.
type SavedStep struct {
	Lines []string `json:"lines"` // a comment that names what to fill in (when anything was replaced), then the call line
	Fill  []string `json:"fill"`  // the environment variable names the step uses
}

var stepEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
var bodyToken = regexp.MustCompile(`[^\s"',;:{}\[\]()<>=&?/\\|]+`)

// BuildCallStep builds the step lines for a request typed in the panel.
func (a *App) BuildCallStep(in SaveStepInput) (SavedStep, error) { return buildCallStep(in) }

// placeholders hands out ${NAME}s: upper case, letters, digits and underscores, a name used once for one value.
type placeholders struct {
	used  map[string]bool
	order []string
}

// savedPrefix is on every generated name, so a name can never be one the shell already has (USER, PATH, HOME) and so what to set is
// plain in the comment.
const savedPrefix = "PROBE_SAVED_"

// nameBase turns what a value was called into letters, digits and underscores, at most 40 of them: every other character, of any kind
// (a line break, a space, a quote, a brace, a letter that is not ASCII), is an underscore, so a name can neither end the comment line
// nor open a placeholder of its own.
func nameBase(base string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(base) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 200 { // enough: a longer name is cut below, and a very long one is not walked to its end
			break
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "VALUE"
	}
	if len(name) > 40 {
		name = strings.TrimRight(name[:40], "_")
	}
	return name
}

func (p *placeholders) next(base string) string {
	name := savedPrefix + nameBase(base)
	if p.used == nil {
		p.used = map[string]bool{}
	}
	cand := name
	for i := 2; p.used[cand]; i++ {
		cand = fmt.Sprintf("%s_%d", name, i)
	}
	p.used[cand] = true
	p.order = append(p.order, cand)
	return "${" + cand + "}"
}

// probeString writes s as a ProbeScript string: the lexer reads \" \n \t and a backslash before anything else as that character.
func probeString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var (
	envForm = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
	// a <name>, <response.body>, <random.number(1,5)>: what the runner replaces from its variables and generators
	varForm = regexp.MustCompile(`<[A-Za-z_][A-Za-z0-9_.\-]*(?:\([^)]*\))?>`)
)

// interpolatedForm names what in s the runner would replace when the step runs, or returns "".
func interpolatedForm(s string) string {
	switch {
	case envForm.MatchString(s):
		return "a ${NAME} (an environment variable)"
	case varForm.MatchString(s):
		return "a <name> (a variable or generator)"
	}
	return ""
}

var digitRun = regexp.MustCompile(`\d(?:[ -]?\d)*`)

// cardRuns are the spans of s that are 13 to 19 digits, in whole groups of digits (separated by a single space or hyphen, or one
// solid group), that pass the card number check. Every run of digit groups is searched in every span of groups, so a card number next
// to other digits ("0 4111 1111 1111 1111 0") is found, and a window that starts or ends inside a group is not tried: that would take
// one in ten long numbers for a card.
func cardRuns(s string) [][2]int {
	var out [][2]int
	for _, m := range digitRun.FindAllStringIndex(s, -1) {
		run := s[m[0]:m[1]]
		type group struct{ a, b int } // offsets in run
		var groups []group
		for i := 0; i < len(run); {
			if run[i] < '0' || run[i] > '9' {
				i++
				continue
			}
			j := i
			for j < len(run) && run[j] >= '0' && run[j] <= '9' {
				j++
			}
			groups = append(groups, group{i, j})
			i = j
		}
		if len(groups) > 2000 {
			continue
		}
		for a := 0; a < len(groups); {
			found := false
			digits := 0
			for z := a; z < len(groups); z++ {
				digits += groups[z].b - groups[z].a
				if digits > 19 {
					break
				}
				if digits < 13 {
					continue
				}
				var d strings.Builder
				for k := a; k <= z; k++ {
					d.WriteString(run[groups[k].a:groups[k].b])
				}
				if luhn(d.String()) {
					out = append(out, [2]int{m[0] + groups[a].a, m[0] + groups[z].b})
					a = z + 1
					found = true
					break
				}
			}
			if !found {
				a++
			}
		}
	}
	return out
}

func buildCallStep(in SaveStepInput) (SavedStep, error) {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	switch method {
	case "GET", "POST", "PUT", "DELETE":
	case "":
		return SavedStep{}, errors.New("there is no method to save")
	case "PATCH", "HEAD", "OPTIONS":
		return SavedStep{}, fmt.Errorf("the call step knows GET, POST, PUT and DELETE, so a %s request cannot be saved as a step", method)
	default:
		// What was typed in the method box is not repeated: it could be anything.
		return SavedStep{}, errors.New("the call step knows GET, POST, PUT and DELETE, so this request cannot be saved as a step")
	}
	u, _, _, err := splitTarget(in.URL)
	if err != nil {
		return SavedStep{}, err
	}
	// Invisible and control characters are not dropped (the step would say another body than the one typed) and not written: the save stops.
	for _, text := range []string{in.URL, in.Body} {
		runes := []rune(text)
		for i, r := range runes {
			if r == '\n' || r == '\t' || (r == '\r' && i+1 < len(runes) && runes[i+1] == '\n') {
				continue
			}
			if r < 0x20 || hiddenRune(r) {
				return SavedStep{}, errors.New("the request has an invisible or control character that a step cannot hold exactly, so nothing was saved")
			}
		}
	}
	body := strings.ReplaceAll(in.Body, "\r\n", "\n")
	// `probe test` fills ${NAME} from the environment and <name> and <random.*> from its variables whenever a step runs, and there is no way
	// to write them literally: a step that held one would do something other than what was typed (and could put an environment value into the
	// request). So the save stops, naming the form and nothing else.
	for _, text := range []string{in.URL, body} {
		if form := interpolatedForm(text); form != "" {
			return SavedStep{}, fmt.Errorf("the request has %s, which probe test fills in when the step runs, so it cannot be saved as typed", form)
		}
	}
	if body != "" && method == "GET" {
		return SavedStep{}, errors.New("a GET request has no body to save")
	}
	var ph placeholders
	savedURL, err := safeURL(u, &ph)
	if err != nil {
		return SavedStep{}, err
	}
	savedBody, err := safeBody(body, &ph)
	if err != nil {
		return SavedStep{}, err
	}
	line := "call " + method + " " + probeString(savedURL)
	if savedBody != "" {
		line += " with body " + probeString(savedBody)
	}
	if corruptLineForTest != nil {
		line = corruptLineForTest(line)
	}
	if err := checkSavedLine(line, in.URL, body, method, savedURL, savedBody, &ph); err != nil {
		return SavedStep{}, err
	}
	out := SavedStep{Lines: []string{line}, Fill: ph.order}
	if out.Fill == nil {
		out.Fill = []string{}
	}
	if len(ph.order) > 0 {
		out.Lines = []string{"# set before running probe test: " + strings.Join(ph.order, ", ") + " (environment variables). Review this step before you save the file: values the save could not recognise are written as typed.", line}
	}
	return out, nil
}

// corruptLineForTest lets a test change the finished line before it is checked, to show that the checks cannot be skipped. It is nil.
var corruptLineForTest func(line string) string

// checkSavedLine is every check on the finished line, in one place that the build always goes through: nothing typed that the saver
// recognises is left in it (the same heuristics as the echo scrubber, plus emails and card numbers), the only ${...} in it are the
// placeholders it made, no <name> is in it, and the ProbeScript parser reads it back as exactly this request.
func checkSavedLine(line, rawURL, body, method, savedURL, savedBody string, ph *placeholders) error {
	if leftover(line, rawURL, body) {
		return errors.New("a value that looks like a credential could not be hidden, so nothing was saved")
	}
	for _, m := range envForm.FindAllString(savedURL+"\x00"+savedBody, -1) {
		if !ph.used[strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")] {
			return errors.New("the step would hold a ${NAME} that is not one it generated, so nothing was saved")
		}
	}
	if varForm.MatchString(savedURL + "\x00" + savedBody) {
		return errors.New("the step would hold a <name> that probe test fills in, so nothing was saved")
	}
	return verifyCall(line, method, savedURL, savedBody)
}

// leftover says whether any value found in the typed request is still in the line, whatever the code that built it did: a typed value
// the echo scrubber would remember, an email address, or a card number. It uses the same heuristics as the saver, so it catches a bug
// in the saver and not a shape the heuristics do not know.
func leftover(line, rawURL, body string) bool {
	low := strings.ToLower(line)
	for v := range collectTyped(rawURL, nil, body).items {
		if strings.Contains(low, strings.ToLower(v)) {
			return true
		}
	}
	return stepEmail.MatchString(line) || len(cardRuns(line)) > 0
}

// verifyCall parses the line with the real parser and compares what it reads with what was meant.
func verifyCall(line, method, wantURL, wantBody string) error {
	prog, err := parser.ParseFile("test \"x\"\n  " + line + "\n")
	if err != nil || len(prog.Tests) != 1 || len(prog.Tests[0].Body) != 1 {
		return errors.New("ProbeScript cannot read this request back as a call step, so nothing was saved")
	}
	step, ok := prog.Tests[0].Body[0].(parser.HTTPCallStep)
	if !ok || step.Method != method || step.URL != wantURL || step.Body != wantBody {
		return errors.New("ProbeScript cannot say this request exactly as a call step, so nothing was saved")
	}
	return nil
}

// safeURL writes the address with every path segment that looks like a credential and every query value as a placeholder. The query
// values are all replaced, as the Network tab hides all of them; the fragment is not sent and is dropped.
func safeURL(u *url.URL, ph *placeholders) (string, error) {
	var b strings.Builder
	b.WriteString(strings.ToLower(u.Scheme) + "://" + u.Host)
	segs := strings.Split(u.EscapedPath(), "/")
	for i, seg := range segs {
		if i == 0 && seg == "" {
			continue
		}
		b.WriteByte('/')
		plain, err := url.PathUnescape(seg)
		if err != nil {
			plain = seg
		}
		if looksSecret(plain) || stepEmail.MatchString(plain) || len(cardRuns(plain)) > 0 {
			b.WriteString(ph.next(fmt.Sprintf("path_%d", i)))
		} else {
			b.WriteString(seg)
		}
	}
	if u.RawQuery != "" {
		b.WriteByte('?')
		for i, pair := range strings.Split(u.RawQuery, "&") {
			if i > 0 {
				b.WriteByte('&')
			}
			name, value, hasValue := strings.Cut(pair, "=")
			plainName, err := url.QueryUnescape(name)
			if err != nil {
				plainName = name
			}
			if hasValue && (looksSecret(plainName) || stepEmail.MatchString(plainName) || len(cardRuns(plainName)) > 0) {
				return "", errors.New("a query name looks like a credential, so nothing was saved")
			}
			switch {
			case !hasValue && name == "":
			case !hasValue:
				// A bare component is a value with no name.
				b.WriteString(ph.next("query_" + fmt.Sprint(i+1)))
			case value == "":
				b.WriteString(name + "=")
			default:
				b.WriteString(name + "=" + ph.next(plainName))
			}
		}
	}
	return b.String(), nil
}

// safeBody writes the body with the values that look like credentials, are under a credential name, or are an email address as
// placeholders. JSON keeps its order and is written on one line (a step cannot hold a raw line break); a form keeps its pairs; any other
// text keeps its words and replaces the ones that look like a credential.
func safeBody(body string, ph *placeholders) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	trimmed := strings.TrimSpace(body)
	if (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		var out bytes.Buffer
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		if err := copyJSON(dec, &out, "body", false, ph); err != nil {
			if errors.Is(err, errKeyLooksSecret) {
				return "", err
			}
			return "", errors.New("the JSON body could not be read, so nothing was saved")
		}
		return out.String(), nil
	}
	if !strings.ContainsAny(trimmed, " \n\t") && strings.Contains(trimmed, "=") {
		parts := strings.Split(trimmed, "&")
		for i, pair := range parts {
			name, value, hasValue := strings.Cut(pair, "=")
			if !hasValue || value == "" {
				continue
			}
			plainName, _ := url.QueryUnescape(name)
			plainValue, _ := url.QueryUnescape(value)
			if isSecretName(plainName) || looksSecret(plainValue) || stepEmail.MatchString(plainValue) || len(cardRuns(plainValue)) > 0 {
				parts[i] = name + "=" + ph.next(plainName)
			}
		}
		return strings.Join(parts, "&"), nil
	}
	return safeText(body, ph), nil
}

var (
	// <password type="x">value</password>: an element named for a credential, with or without attributes
	stepXMLElement = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.\-]*)(?:\s[^<>]*)?>([^<]+)</([A-Za-z_][A-Za-z0-9_.\-]*)>`)
	// Authorization: Bearer abc, as it may be pasted into a body: the scheme word stays, the credential goes
	stepAuthLine = regexp.MustCompile(`(?i)\b(?:proxy-)?authorization\s*[:=]\s*(?:(?:bearer|basic|token|digest|negotiate|ntlm|apikey|hmac)\s+)?([^\s"',;]+)`)
	// password: my secret phrase: a credential name, a colon and the rest of the line
	stepNameLine = regexp.MustCompile(`(?m)([A-Za-z0-9_.\-]+)[ \t]*:[ \t]+([^\n]+)$`)
)

// isAuthName is an Authorization name, whose scheme word is kept and whose credential is replaced by its own rule.
func isAuthName(n string) bool {
	n = strings.ToLower(n)
	return n == "authorization" || n == "proxy-authorization"
}

// safeText replaces, in free text, the value of a credential-named assignment (the whole rest of the line after `name:`), the credential
// in an Authorization line, the text of an element named for a credential, each email address, each card number and each token that
// looks like a credential.
func safeText(s string, ph *placeholders) string {
	type span struct {
		a, b int
		name string
	}
	var spans []span
	for _, m := range assignment.FindAllStringSubmatchIndex(s, -1) {
		if isSecretName(s[m[2]:m[3]]) && !isAuthName(s[m[2]:m[3]]) {
			spans = append(spans, span{m[4], m[5], s[m[2]:m[3]]})
		}
	}
	for _, m := range stepNameLine.FindAllStringSubmatchIndex(s, -1) {
		if isSecretName(s[m[2]:m[3]]) && !isAuthName(s[m[2]:m[3]]) {
			spans = append(spans, span{m[4], m[5], s[m[2]:m[3]]})
		}
	}
	for _, m := range stepAuthLine.FindAllStringSubmatchIndex(s, -1) {
		spans = append(spans, span{m[2], m[3], "authorization"})
	}
	for _, m := range stepXMLElement.FindAllStringSubmatchIndex(s, -1) {
		if s[m[2]:m[3]] == s[m[6]:m[7]] && isSecretName(s[m[2]:m[3]]) {
			spans = append(spans, span{m[4], m[5], s[m[2]:m[3]]})
		}
	}
	for _, m := range stepEmail.FindAllStringIndex(s, -1) {
		spans = append(spans, span{m[0], m[1], "email"})
	}
	for _, m := range cardRuns(s) {
		spans = append(spans, span{m[0], m[1], "card"})
	}
	for _, m := range bodyToken.FindAllStringIndex(s, -1) {
		if looksSecret(s[m[0]:m[1]]) {
			spans = append(spans, span{m[0], m[1], "token"})
		}
	}
	// Earliest first, the longer first on a tie; a span inside an earlier one is skipped.
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].a != spans[j].a {
			return spans[i].a < spans[j].a
		}
		return spans[i].b > spans[j].b
	})
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		if sp.a < at {
			continue
		}
		b.WriteString(s[at:sp.a])
		b.WriteString(ph.next(sp.name))
		at = sp.b
	}
	b.WriteString(s[at:])
	return b.String()
}

var (
	hexRun            = regexp.MustCompile(`^[0-9A-Fa-f]{16,}$`)
	jwtShape          = regexp.MustCompile(`^eyJ[A-Za-z0-9_\-]{4,}\.[A-Za-z0-9_\-]{4,}\.`)
	paddedBase64      = regexp.MustCompile(`^[A-Za-z0-9+/_\-]{14,}={1,2}$`)
	errKeyLooksSecret = errors.New("an object key in the body looks like a credential, so nothing was saved")
)

// strongSecretShape says whether a name is a hex string of 16 or more digits, a JWT, or base64 with padding: shapes an ordinary key
// does not have, so a name of one of them is a value written where a name goes.
func strongSecretShape(name string) bool {
	return hexRun.MatchString(name) || jwtShape.MatchString(name) || paddedBase64.MatchString(name)
}

// copyJSON copies one JSON value, replacing a string or number that is under a credential name, a string that looks like a
// credential, and an email address. Keys are kept; they are names, not values.
func copyJSON(dec *json.Decoder, out *bytes.Buffer, key string, secret bool, ph *placeholders) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	enc := func(v any) string {
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		_ = e.Encode(v)
		return strings.TrimRight(b.String(), "\n")
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			out.WriteByte('{')
			first := true
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				name, _ := k.(string)
				if !first {
					out.WriteByte(',')
				}
				first = false
				if strongSecretShape(name) {
					return errKeyLooksSecret // a name that is a hex string, a JWT or padded base64 is a value written as a name
				}
				if stepEmail.MatchString(name) || len(cardRuns(name)) > 0 {
					out.WriteString(enc(ph.next("key")) + ":") // a name that carries an address or a card number is a value too
				} else {
					out.WriteString(enc(name) + ":")
				}
				if err := copyJSON(dec, out, name, isSecretName(name), ph); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return err
			}
			out.WriteByte('}')
		case '[':
			out.WriteByte('[')
			first := true
			for dec.More() {
				if !first {
					out.WriteByte(',')
				}
				first = false
				if err := copyJSON(dec, out, key, secret, ph); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return err
			}
			out.WriteByte(']')
		}
	case string:
		if secret || looksSecret(t) || stepEmail.MatchString(t) || len(cardRuns(t)) > 0 {
			out.WriteString(enc(ph.next(key)))
		} else {
			out.WriteString(enc(t))
		}
	case json.Number:
		if secret || len(cardRuns(t.String())) > 0 {
			out.WriteString(enc(ph.next(key)))
		} else {
			out.WriteString(t.String())
		}
	case bool:
		out.WriteString(fmt.Sprint(t))
	case nil:
		out.WriteString("null")
	}
	return nil
}
