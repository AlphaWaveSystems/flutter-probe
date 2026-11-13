package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/alphawavesystems/flutter-probe/internal/record"
)

// The redactor for recorded backend traffic. Everything Studio shows of an exchange passes through it, in Go, before it reaches the
// page: the page never holds a raw header value, query value or body. Nothing here writes to disk or to a log, and nothing of an
// exchange is sent to an AI provider or anywhere else.
//
// It is layered on the redactor the recorder and the agent already share (internal/record): that one is run first on bodies, URLs and
// header names, and the rules here are added to it, so there is one list of credential words, not two.
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

// secretFamilies are the name families whose values are never shown. A name is matched after it is percent-decoded, folded to its
// compatibility form (so full-width and other look-alike letters read as ASCII), lower-cased, and stripped of separators and hidden
// characters, so X_Api_Key, x-auth-token, APIKEY and user[password] all match.
var secretFamilies = []string{
	"authorization", "cookie", "token", "secret", "password", "passwd", "passphrase", "passcode", "apikey", "privatekey", "accesskey",
	"credential", "session", "xsrf", "signature", "bearer", "creditcard", "jwt", "codeverifier", "verificationcode",
	"subscriptionkey", "functionskey", "appkey", "masterkey", "fastlykey", "signingkey", "encryptionkey",
}

// secretExact are short names that are secrets only as a whole name, so "code" does not match "zipcode". The recorder's token rule adds
// pass, sid, pin, otp, cvv, cvc, ssn, sig and csrf the same way, so they are not repeated here.
var secretExact = map[string]bool{"key": true, "keys": true, "iban": true, "code": true, "private": true}

// foldName reads the escapes of a name and folds it to its compatibility form, with hidden characters dropped.
func foldName(name string) string {
	name = withoutInvisible(strings.ToValidUTF8(name, "?"), false)
	if dec, err := url.QueryUnescape(name); err == nil {
		name = dec
	}
	return norm.NFKC.String(name)
}

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

func secretNormalized(n string) bool {
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

// isSecretName says whether a header, query, JSON or form name is in a secret family: by this file's families, or by the token rule of
// the recorder's redactor (pass, sid, passphrase, otp, pin, session and the rest).
func isSecretName(name string) bool {
	plain := withoutInvisible(strings.ToValidUTF8(name, "?"), false)
	folded := foldName(name)
	return secretNormalized(normalizeName(plain)) || secretNormalized(normalizeName(folded)) || record.IsCredentialName(folded)
}

// keyHeader matches a header name that ends in the word key, such as x-functions-key or Ocp-Apim-Subscription-Key.
var keyHeader = regexp.MustCompile(`(?i)(^|[-_. ])key$`)

// isSecretHeader is isSecretName, and any header name that ends in "key" (x-foo-key, Subscription_Key). The exact header names the
// recorder keeps (x-api-key, x-csrf-token, set-cookie and the rest) are all in the families above.
func isSecretHeader(name string) bool {
	return isSecretName(name) || keyHeader.MatchString(foldName(name))
}

var (
	jwtPattern       = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]*`)
	base64urlPattern = regexp.MustCompile(`[A-Za-z0-9_-]{24,}`)
	uuidPattern      = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
	cardPattern      = regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`)
)

// luhn says whether a run of digits passes the card number check.
func luhn(digits string) bool {
	sum, double := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// redactShapes redacts what has the shape of a secret but no name: a JWT, a base64url run of 24 characters or more that holds a digit
// (a UUID is kept), and a run of 13 to 19 digits that passes the card number check.
func redactShapes(s string) string {
	s = jwtPattern.ReplaceAllString(s, redactedMark)
	s = base64urlPattern.ReplaceAllStringFunc(s, func(run string) string {
		if strings.ContainsAny(run, "0123456789") && !uuidPattern.MatchString(run) {
			return redactedMark
		}
		return run
	})
	return cardPattern.ReplaceAllStringFunc(s, func(run string) string {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, run)
		if len(digits) >= 13 && len(digits) <= 19 && luhn(digits) {
			return redactedMark
		}
		return run
	})
}

// cleanText makes a string that came from traffic fit to show: hidden characters dropped, home folders and emails replaced, and
// credential shapes redacted. A line break is kept when keepLines is set.
func cleanText(s string, keepLines bool) string {
	return redactShapes(redactCredentials(scrubPersonal(withoutInvisible(strings.ToValidUTF8(s, "?"), keepLines))))
}

// shortText is cleanText cut to max characters.
func shortText(s string, max int) string {
	return strings.TrimSpace(capRunes(cleanText(s, false), max))
}

// urlInText finds a URL inside free text, such as an error message.
var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"'<>]+`)

// redactURL shows a URL without what can identify or authorize: the user name and password are removed (everything before the
// last @ of the authority, even when the password holds a ? or a #), the fragment is dropped, every query value is replaced and
// only the names stay, a query component with no name=value form is replaced whole, and the path is scrubbed. It first runs the
// recorder's URL redactor, then works on the text and does not parse it, so a URL that does not parse is treated the same way.
func redactURL(raw string) string {
	s := record.RedactURL(withoutInvisible(strings.ToValidUTF8(raw, "?"), false))
	s = strings.ReplaceAll(s, record.Placeholder, redactedMark)
	start := -1 // where the authority begins: after "://", or after a leading "//"
	if i := strings.Index(s, "://"); i >= 0 {
		start = i + 3
	} else if strings.HasPrefix(s, "//") {
		start = 2
	}
	if start >= 0 {
		rest := s[start:]
		end := strings.IndexByte(rest, '/')
		if end < 0 {
			end = len(rest)
		}
		if at := strings.LastIndexByte(rest[:end], '@'); at >= 0 {
			s = s[:start] + rest[at+1:]
		}
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	query := ""
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s, query = s[:i], s[i+1:]
	}
	out := cleanText(s, false)
	if query != "" {
		parts := strings.Split(query, "&")
		for i, p := range parts {
			eq := strings.IndexByte(p, '=')
			if eq < 0 {
				parts[i] = redactedMark // a component with no name is the value
				continue
			}
			name := p[:eq]
			if dec, err := url.QueryUnescape(name); err == nil {
				name = dec
			}
			parts[i] = shortText(name, 100) + "=" + redactedMark
		}
		out += "?" + strings.Join(parts, "&")
	}
	return capRunes(out, netMaxURL)
}

// redactErrorText makes an error text from the app fit to show: every URL in it goes through redactURL, and the rest through
// cleanText and the recorder's body redactor.
func redactErrorText(s string, max int) string {
	s = withoutInvisible(strings.ToValidUTF8(s, "?"), false)
	s = urlInText.ReplaceAllStringFunc(s, redactURL)
	s = strings.ReplaceAll(record.RedactBody(s), record.Placeholder, redactedMark)
	return strings.TrimSpace(capRunes(cleanText(s, false), max))
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
		if isSecretHeader(n) {
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

// dropHiddenKeepSpace drops hidden characters but keeps line breaks, carriage returns and tabs as they are, for the recorder's
// redactor that reads a multipart body by its CRLF lines. They become spaces in what is shown, at the end (cleanText).
func dropHiddenKeepSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return r
		case hiddenRune(r):
			return -1
		}
		return r
	}, s)
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
	text := dropHiddenKeepSpace(strings.ToValidUTF8(body, "?"))
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
	// The recorder's redactor first: it reads JSON that is cut, has comments or one value per line, XML elements (a WS-Security
	// UsernameToken), multipart parts and GraphQL arguments, which the structure rules below do not parse.
	text = strings.ReplaceAll(record.RedactBody(text), record.Placeholder, redactedMark)
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.Contains(strings.ToLower(contentType), "json") {
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil {
			if _, err := dec.Token(); err != nil { // nothing may follow the value
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				enc.SetEscapeHTML(false) // markup in a value is shown as it is, not as \u003c
				enc.SetIndent("", "  ")
				if err := enc.Encode(redactJSON(v, 0)); err == nil {
					return strings.TrimSuffix(buf.String(), "\n")
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
		pair := namesASecret(t)
		for k, val := range t {
			key := shortText(k, 100)
			if isSecretName(k) || (pair && pairValueMembers[strings.ToLower(k)]) {
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

// pairNameMembers and pairValueMembers are the members of a {"name": "api_key", "value": "..."} object: when a name member names a
// secret, the value member is one.
var (
	pairNameMembers  = map[string]bool{"name": true, "key": true, "field": true, "param": true, "id": true, "label": true}
	pairValueMembers = map[string]bool{"value": true, "val": true, "data": true, "content": true, "text": true, "newvalue": true}
)

// namesASecret says whether an object has a pair name member, as a string or an array of strings, that is in a secret family.
func namesASecret(obj map[string]any) bool {
	for member, v := range obj {
		if !pairNameMembers[strings.ToLower(member)] {
			continue
		}
		switch names := v.(type) {
		case string:
			if isSecretName(names) {
				return true
			}
		case []any:
			for _, item := range names {
				if name, ok := item.(string); ok && isSecretName(name) {
					return true
				}
			}
		}
	}
	return false
}
