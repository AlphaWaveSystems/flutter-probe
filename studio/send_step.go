package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
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

func (p *placeholders) next(base string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(base) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" || len(name) > 40 {
		name = "VALUE"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "V_" + name
	}
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

func buildCallStep(in SaveStepInput) (SavedStep, error) {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	switch method {
	case "GET", "POST", "PUT", "DELETE":
	case "":
		return SavedStep{}, errors.New("there is no method to save")
	default:
		return SavedStep{}, fmt.Errorf("the call step knows GET, POST, PUT and DELETE, so a %s request cannot be saved as a step", method)
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
	// Nothing typed may be left in the lines: what collectTyped finds in the request is looked for in the result, whatever the code
	// above did. A value that is still there stops the save, and the message says nothing of it.
	if leftover(line, in.URL, body) {
		return SavedStep{}, errors.New("a value that looks like a credential could not be hidden, so nothing was saved")
	}
	// The step has to read back as exactly this request, or it is not saved.
	if err := verifyCall(line, method, savedURL, savedBody); err != nil {
		return SavedStep{}, err
	}
	out := SavedStep{Lines: []string{line}, Fill: ph.order}
	if out.Fill == nil {
		out.Fill = []string{}
	}
	if len(ph.order) > 0 {
		out.Lines = []string{"# set before running probe test: " + strings.Join(ph.order, ", ") + " (environment variables; nothing typed in the panel is saved)", line}
	}
	return out, nil
}

// leftover says whether any value found in the typed request is still in the line, whatever the code that built it did.
func leftover(line, rawURL, body string) bool {
	low := strings.ToLower(line)
	for v := range collectTyped(rawURL, nil, body).items {
		if strings.Contains(low, strings.ToLower(v)) {
			return true
		}
	}
	return false
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
		if looksSecret(plain) || stepEmail.MatchString(plain) {
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
			if hasValue && (looksSecret(plainName) || stepEmail.MatchString(plainName)) {
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
			if isSecretName(plainName) || looksSecret(plainValue) || stepEmail.MatchString(plainValue) {
				parts[i] = name + "=" + ph.next(plainName)
			}
		}
		return strings.Join(parts, "&"), nil
	}
	return safeText(body, ph), nil
}

// safeText replaces, in free text, the value of a credential-named assignment, each email address and each token that looks like a credential.
func safeText(s string, ph *placeholders) string {
	type span struct {
		a, b int
		name string
	}
	var spans []span
	for _, m := range assignment.FindAllStringSubmatchIndex(s, -1) {
		if isSecretName(s[m[2]:m[3]]) {
			spans = append(spans, span{m[4], m[5], s[m[2]:m[3]]})
		}
	}
	for _, m := range stepEmail.FindAllStringIndex(s, -1) {
		spans = append(spans, span{m[0], m[1], "email"})
	}
	for _, m := range bodyToken.FindAllStringIndex(s, -1) {
		if looksSecret(s[m[0]:m[1]]) {
			spans = append(spans, span{m[0], m[1], "token"})
		}
	}
	// Earliest first; a span inside an earlier one is skipped.
	for i := range spans {
		for j := i + 1; j < len(spans); j++ {
			if spans[j].a < spans[i].a {
				spans[i], spans[j] = spans[j], spans[i]
			}
		}
	}
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
				out.WriteString(enc(name) + ":")
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
		if secret || looksSecret(t) || stepEmail.MatchString(t) {
			out.WriteString(enc(ph.next(key)))
		} else {
			out.WriteString(enc(t))
		}
	case json.Number:
		if secret {
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
