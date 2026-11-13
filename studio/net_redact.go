package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The redactor for recorded backend traffic. Everything Studio shows of an exchange passes through it, in Go, before it reaches the
// page: the page never holds a raw header value, query value or body. Nothing here writes to disk or to a log, and nothing of an
// exchange is sent to an AI provider or anywhere else.
//
// The order is the one Explain uses: hidden characters are dropped first (the shared hiddenRune rule), so a name, a key or a token
// split by one is whole again; then the structure-aware rules (header names, query values, JSON keys, form keys); then the home-path
// and email scrub and the credential-shape redactor over every string that is shown; and the cut last.

const (
	netMaxURL         = 2000
	netMaxHeaderName  = 100
	netMaxHeaderValue = 500
	netMaxHeaders     = 100
	netMaxMethod      = 16
	netMaxError       = 300
	netBodyShown      = 4096    // characters of a text body that are shown
	netBodyMax        = 1 << 20 // a body over this is not shown at all
	netMaxDepth       = 64      // JSON nesting that is walked
	netSniff          = 8192    // bytes looked at to tell text from binary
)

// secretFamilies are the name families whose values are never shown. A name is matched after it is lower-cased and its separators
// and hidden characters are removed, so X_Api_Key, x-auth-token, APIKEY and user[password] all match.
var secretFamilies = []string{
	"authorization", "cookie", "token", "secret", "password", "passwd", "apikey", "privatekey", "accesskey", "credential",
	"session", "csrf", "xsrf", "signature", "bearer", "cardnumber", "creditcard", "jwt",
}

// secretExact are short names that are secrets only as a whole name, so "pin" does not match "shipping".
var secretExact = map[string]bool{"key": true, "keys": true, "pin": true, "otp": true, "cvv": true, "cvc": true, "pwd": true, "sig": true, "ssn": true, "iban": true}

// normalizeName lower-cases a name and removes hidden characters and the separators that vary between spellings.
func normalizeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case hiddenRune(r):
			return -1
		case r == '_' || r == '-' || r == '.' || r == ' ' || r == '[' || r == ']' || r == '\t' || r == '\n' || r == '\r':
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}

// isSecretName says whether a header, query, JSON or form name is in a secret family.
func isSecretName(name string) bool {
	n := normalizeName(name)
	if secretExact[n] {
		return true
	}
	for _, f := range secretFamilies {
		if strings.Contains(n, f) {
			return true
		}
	}
	// "auth" covers oauth, authn, authz and x-auth-token, but not author, authors or authority.
	return strings.Contains(strings.ReplaceAll(n, "author", ""), "auth")
}

// cleanText makes a string that came from traffic fit to show: hidden characters dropped, home folders and emails replaced, and
// credential shapes redacted. A line break is kept when keepLines is set.
func cleanText(s string, keepLines bool) string {
	return redactCredentials(scrubPersonal(withoutInvisible(strings.ToValidUTF8(s, "?"), keepLines)))
}

// shortText is cleanText cut to max characters.
func shortText(s string, max int) string {
	return strings.TrimSpace(capRunes(cleanText(s, false), max))
}

// redactURL shows a URL without what can identify or authorize: the user name and password are removed, the fragment is dropped,
// every query value is replaced and only the names stay, and the path is scrubbed. It works on the text and does not parse it, so a
// URL that does not parse is treated the same way.
func redactURL(raw string) string {
	s := withoutInvisible(strings.ToValidUTF8(raw, "?"), false)
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	query := ""
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s, query = s[:i], s[i+1:]
	}
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		end := strings.IndexByte(rest, '/')
		if end < 0 {
			end = len(rest)
		}
		if at := strings.LastIndexByte(rest[:end], '@'); at >= 0 {
			s = s[:i+3] + rest[at+1:]
		}
	}
	out := cleanText(s, false)
	if query != "" {
		parts := strings.Split(query, "&")
		for i, p := range parts {
			name := p
			if eq := strings.IndexByte(p, '='); eq >= 0 {
				name = p[:eq]
				if dec, err := url.QueryUnescape(name); err == nil {
					name = dec
				}
				parts[i] = shortText(name, 100) + "=" + redactedMark
				continue
			}
			if dec, err := url.QueryUnescape(name); err == nil {
				name = dec
			}
			parts[i] = shortText(name, 100)
		}
		out += "?" + strings.Join(parts, "&")
	}
	return capRunes(out, netMaxURL)
}

// urlHeaders are headers whose value is a URL.
var urlHeaders = map[string]bool{"location": true, "referer": true, "referrer": true, "contentlocation": true, "origin": true, "refresh": true, "xforwardedurl": true, "xoriginalurl": true}

// NetHeader is one header as it is shown: the name, and the value unless the name is in a secret family.
type NetHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// redactHeaders returns the headers sorted by name, at most netMaxHeaders of them, with the number that were not shown.
func redactHeaders(h map[string]string) (out []NetHeader, more int) {
	names := make([]string, 0, len(h))
	for n := range h {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if len(out) >= netMaxHeaders {
			more++
			continue
		}
		hdr := NetHeader{Name: shortText(n, netMaxHeaderName)}
		if isSecretName(n) {
			hdr.Value = redactedMark
		} else if urlHeaders[normalizeName(n)] {
			hdr.Value = redactURL(h[n]) // a redirect target or a referring page carries a query, whose values are never shown
		} else {
			hdr.Value = shortText(h[n], netMaxHeaderValue)
		}
		out = append(out, hdr)
	}
	return out, more
}

// NetBody is a body as it is shown. Kind is "empty", "text", "binary" (the size only) or "large" (over 1 MB: the size only).
type NetBody struct {
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
}

func headerValue(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// binaryType says whether a Content-Type names content that is not text.
func binaryType(ct string) bool {
	ct = strings.ToLower(ct)
	for _, p := range []string{"image/", "video/", "audio/", "font/", "application/octet-stream", "application/zip", "application/pdf", "application/gzip", "application/x-protobuf", "application/grpc", "application/wasm"} {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}

// looksBinary says whether the start of a body is not text: a NUL, or bytes that are not valid UTF-8.
func looksBinary(b []byte) bool {
	if len(b) > netSniff {
		b = b[:netSniff]
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	if utf8.Valid(b) {
		return false
	}
	// A multi-byte character cut at the end of the sample is not binary.
	for i := 1; i <= 3 && i < len(b); i++ {
		if utf8.Valid(b[:len(b)-i]) {
			return false
		}
	}
	return true
}

// redactBody makes a body fit to show. reported is the size the agent recorded, which can be larger than the text it kept.
func redactBody(body string, reported int64, contentType string) NetBody {
	size := int64(len(body))
	if reported > size {
		size = reported
	}
	switch {
	case size == 0:
		return NetBody{Kind: "empty"}
	case size > netBodyMax:
		return NetBody{Kind: "large", Size: size}
	case binaryType(contentType) || looksBinary([]byte(body)):
		return NetBody{Kind: "binary", Size: size}
	}
	text := withoutInvisible(strings.ToValidUTF8(body, "?"), true)
	shown := redactStructured(text, contentType)
	out := NetBody{Kind: "text", Size: size}
	if r := []rune(shown); len(r) > netBodyShown {
		out.Truncated = true
		shown = string(r[:netBodyShown])
	}
	out.Text = shown
	return out
}

// redactStructured redacts a text body by what it is: JSON by key, a form by name, anything else by shape.
func redactStructured(text, contentType string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.Contains(strings.ToLower(contentType), "json") {
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil {
			if _, err := dec.Token(); err != nil { // nothing may follow the value
				if out, err := json.MarshalIndent(redactJSON(v, 0), "", "  "); err == nil {
					return string(out)
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(contentType), "x-www-form-urlencoded") || looksLikeForm(trimmed) {
		return redactForm(trimmed)
	}
	return cleanText(text, true)
}

func looksLikeForm(s string) bool {
	if s == "" || strings.ContainsAny(s, " \n\t{}<>") {
		return false
	}
	for _, p := range strings.Split(s, "&") {
		if eq := strings.IndexByte(p, '='); eq <= 0 {
			return false
		}
	}
	return true
}

func redactForm(s string) string {
	parts := strings.Split(s, "&")
	for i, p := range parts {
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			parts[i] = cleanText(p, false)
			continue
		}
		name, value := p[:eq], p[eq+1:]
		if dec, err := url.QueryUnescape(name); err == nil {
			name = dec
		}
		if dv, err := url.QueryUnescape(value); err == nil {
			value = dv
		}
		if isSecretName(name) {
			parts[i] = shortText(name, 100) + "=" + redactedMark
		} else {
			parts[i] = shortText(name, 100) + "=" + cleanText(value, false)
		}
	}
	return strings.Join(parts, "&")
}

// redactJSON replaces the value of every key in a secret family, at any depth and inside arrays, and cleans every other string.
func redactJSON(v any, depth int) any {
	if depth > netMaxDepth {
		return "[too deep]"
	}
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			key := shortText(k, 100)
			if isSecretName(k) {
				out[key] = redactedMark
			} else {
				out[key] = redactJSON(val, depth+1)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactJSON(val, depth+1)
		}
		return out
	case string:
		return cleanText(t, true)
	}
	return v
}
