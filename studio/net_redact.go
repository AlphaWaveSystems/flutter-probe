package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/alphawavesystems/flutter-probe/internal/record"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
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

	// Raw limits, applied BEFORE any pattern reads a value, so what the patterns cost does not depend on what the agent recorded: a
	// header value of a megabyte is read as its first 8 KB. A cut that falls inside a token is trimmed (boundRunes), so a secret cut in
	// two does not reach the output.
	netRawURL         = 4096
	netRawError       = 4096
	netRawHeaderName  = 256
	netRawHeaderValue = 8192
	netRawBody        = 64 * 1024 // the agent itself keeps 64 KB of a body
	netBudgetTotal    = 256 * 1024
)

// netBudget is the text one exchange may have processed in all, across its headers and bodies, so many large fields cost no more than
// one. A field that does not fit is shown as not shown.
type netBudget struct{ left int }

func newNetBudget() *netBudget { return &netBudget{left: netBudgetTotal} }

// take spends n and says whether there was enough.
func (b *netBudget) take(n int) bool {
	if n > b.left {
		b.left = 0
		return false
	}
	b.left -= n
	return true
}

const netNotShown = "[not shown: size limit]"

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
	name = boundRunes(name, netRawHeaderName)
	plain := withoutInvisible(strings.ToValidUTF8(name, "?"), false)
	folded := foldName(name)
	return secretNormalized(normalizeName(plain)) || secretNormalized(normalizeName(folded)) || record.IsCredentialName(folded) || endsInKey(folded)
}

// endsInKey says whether the last word of a name is key, for any spelling: x-functions-key, Foo_Key, licenseKey, clientKey,
// sendgrid-key and client_key. A word that merely ends in the letters, such as monkey, is not one. (The exact header names the
// recorder keeps, x-api-key, x-csrf-token, set-cookie and the rest, are all in the families above.)
func endsInKey(name string) bool {
	tokens := sysdialog.NameTokens(name)
	if len(tokens) == 0 {
		return false
	}
	last := tokens[len(tokens)-1]
	return last == "key" || last == "keys"
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

// shortText is cleanText cut to max characters. The raw text is bounded first (16 times the cut, at most 8 KB), so a very long value
// does not cost more than a short one.
func shortText(s string, max int) string {
	raw := max * 16
	if raw > netRawHeaderValue {
		raw = netRawHeaderValue
	}
	return strings.TrimSpace(capRunes(cleanText(boundRunes(s, raw), false), max))
}

// urlInText finds a URL inside free text, such as an error message.
var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"'<>]+`)

// redactURL shows a URL without what can identify or authorize: the user name and password are removed (everything before the
// last @ of the authority, even when the password holds a ? or a #), the fragment is dropped, every query value is replaced and
// only the names stay, a query component with no name=value form is replaced whole, and the path is scrubbed. It first runs the
// recorder's URL redactor, then works on the text and does not parse it, so a URL that does not parse is treated the same way.
func redactURL(raw string) string {
	s := record.RedactURL(withoutInvisible(strings.ToValidUTF8(boundRunes(raw, netRawURL), "?"), false))
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

// redactErrorText makes an error text from the app fit to show: it is bounded first, every URL in it goes through redactURL, and the
// rest through the recorder's body redactor (which reads a YAML block value), the named assignments and cleanText.
func redactErrorText(s string, max int) string {
	s = dropHiddenKeepSpace(strings.ToValidUTF8(boundRunes(s, netRawError), "?")) // line breaks stay until the end: a parent line is not a value
	s = urlInText.ReplaceAllStringFunc(s, redactURL)
	s = strings.ReplaceAll(record.RedactBody(s), record.Placeholder, redactedMark)
	s = redactNamedAssignments(s, nil)
	return strings.TrimSpace(capRunes(cleanText(s, false), max))
}

// valueNames are names that are secrets only inside a header value: the response and client nonce of a Digest challenge.
var valueNames = map[string]bool{"response": true, "cnonce": true}

// redactHeaderValue makes the value of a header whose name is not a secret fit to show: bounded, then the recorder's body redactor (an
// assignment such as sid=... or session=...), the named assignments and the Digest fields, then shapes.
func redactHeaderValue(v string, max int) string {
	s := dropHiddenKeepSpace(strings.ToValidUTF8(boundRunes(v, netRawHeaderValue), "?")) // line breaks stay until the end
	s = strings.ReplaceAll(record.RedactBody(s), record.Placeholder, redactedMark)
	s = redactNamedAssignments(s, valueNames)
	return strings.TrimSpace(capRunes(cleanText(s, false), max))
}

// namedAssignment matches name=value, name: value and name%3Dvalue in text, with quotes allowed around the separator and the value.
// The separator stays on the line: a name that ends its line is a parent, and the next line is not its value.
var namedAssignment = regexp.MustCompile(`((?:[A-Za-z0-9_.\-]|\[|\]|%[0-9A-Fa-f]{2})+)["']?(?:[ \t]*[=:][ \t]*|%3[Dd]|%3[Aa])["']?([^\s&,;"'<>]{4,})`)

// redactNamedAssignments replaces each assignment whose name is in a secret family by this file's rule (which includes the recorder's),
// or is in extra, with the placeholder. The whole assignment goes, so the name does not stay. A match whose value holds a colon or an
// equals sign ("db: licenseKey: x", "a: xsrf=x") is not consumed: the search goes on from that value, so the credential in it is read as
// an assignment of its own.
func redactNamedAssignments(s string, extra map[string]bool) string {
	var out strings.Builder
	last, pos := 0, 0
	for pos < len(s) {
		loc := namedAssignment.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		name := s[pos+loc[2] : pos+loc[3]]
		value := s[pos+loc[4] : pos+loc[5]]
		switch {
		case isSecretName(name) || extra[normalizeName(foldName(name))]:
			out.WriteString(s[last:start])
			out.WriteString(redactedMark)
			last, pos = end, end
		case strings.ContainsAny(value, ":="):
			pos += loc[4] // the value holds another assignment: read from it, so the credential in it is read on its own
		default:
			pos = end
		}
	}
	out.WriteString(s[last:])
	return out.String()
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
	return redactHeadersBudget(h, newNetBudget())
}

// redactHeadersBudget is redactHeaders with the budget of one exchange: each name and value is bounded before it is read, and a value
// that does not fit in what is left is shown as not shown (its name stays).
func redactHeadersBudget(h map[string]string, budget *netBudget) (out []NetHeader, more int) {
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
		name := boundRunes(n, netRawHeaderName)
		hdr := NetHeader{Name: shortText(name, netMaxHeaderName)}
		cost := len(name)
		if v := h[n]; len(v) > netRawHeaderValue {
			cost += netRawHeaderValue
		} else {
			cost += len(v)
		}
		switch {
		case isSecretName(name):
			hdr.Value = redactedMark
		case !budget.take(cost):
			hdr.Value = netNotShown
		case urlHeaders[normalizeName(name)]:
			hdr.Value = redactURL(h[n]) // a redirect target or a referring page carries a query, whose values are never shown
		default:
			hdr.Value = redactHeaderValue(h[n], netMaxHeaderValue)
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
	return redactBodyBudget(body, reported, contentType, newNetBudget())
}

// redactBodyBudget is redactBody with the budget of one exchange. Only the first 64 KB of a body is read (the agent keeps no more),
// with an unfinished token at the cut trimmed away, and the result is marked as cut.
func redactBodyBudget(body string, reported int64, contentType string, budget *netBudget) NetBody {
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
	read := len(body)
	if read > netRawBody {
		read = netRawBody
	}
	if !budget.take(read) {
		return NetBody{Kind: "limit", Size: size}
	}
	out := NetBody{Kind: "text", Size: size}
	raw := strings.ToValidUTF8(body, "?")
	if len(raw) > netRawBody {
		out.Truncated = true
	}
	text := dropHiddenKeepSpace(boundRunes(raw, netRawBody))
	shown := redactStructured(text, contentType)
	if r := []rune(shown); len(r) > netBodyShown {
		out.Truncated = true
		shown = string(r[:netBodyShown])
	}
	out.Text = shown
	return out
}

// redactStructured redacts a text body by what it is: a form by name, with each value read as the text it holds; JSON by key; markup
// by element and attribute; anything else by assignment and shape.
func redactStructured(text, contentType string) string {
	return redactStructuredDepth(text, contentType, 0)
}

const netMaxNesting = 3 // a form value that holds JSON that holds a form...

func redactStructuredDepth(text, contentType string, depth int) string {
	if depth > netMaxNesting {
		return cleanText(text, true)
	}
	lower := strings.ToLower(contentType)
	trimmed := strings.TrimSpace(text)
	// A form first: its values are decoded and read as text before any rule reads them, so a value that is URL-encoded JSON is
	// redacted as JSON.
	if strings.Contains(lower, "x-www-form-urlencoded") || looksLikeForm(trimmed) {
		return redactForm(trimmed, depth)
	}
	jsonLike := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.Contains(lower, "json")
	if jsonLike {
		text = stripJSONComments(text)
	}
	// The recorder's redactor: it reads JSON that is cut or has one value per line, XML elements (a WS-Security UsernameToken),
	// multipart parts, GraphQL arguments and YAML, which the structure rules below do not parse.
	text = strings.ReplaceAll(record.RedactBody(text), record.Placeholder, redactedMark)
	trimmed = strings.TrimSpace(text)
	if jsonLike {
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
	text = redactHTML(redactXML(text))
	return cleanText(redactNamedAssignments(text, nil), true)
}

// stripJSONComments removes /* */ and // comments from text that is meant to be JSON, outside strings, so the rules that read it do
// not see a comment between a name and its value. It reads cut text too: an unfinished string or comment ends at the end.
func stripJSONComments(s string) string {
	var out strings.Builder
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inString:
			out.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				out.WriteByte(s[i])
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
			out.WriteByte(c)
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return out.String()
			}
			i += 2 + end + 1
			out.WriteByte(' ')
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				out.WriteByte('\n')
			}
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

var (
	xmlElement = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.:\-]*)(\s[^<>]*)?>([^<]*)</([A-Za-z_][A-Za-z0-9_.:\-]*)\s*>`)
	xmlCDATA   = regexp.MustCompile(`(?s)<([A-Za-z_][A-Za-z0-9_.:\-]*)(\s[^<>]*)?>\s*<!\[CDATA\[.*?\]\]>\s*</([A-Za-z_][A-Za-z0-9_.:\-]*)\s*>`)
	htmlTag    = regexp.MustCompile(`(?i)<(input|meta|textarea|select|option)\b[^>]*>`)
	htmlAttr   = regexp.MustCompile(`(?i)\b([a-z][a-z0-9:_-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s>"']+)`)
)

// redactXML replaces the text of an element whose name is in a secret family, plain or as CDATA, as in <licenseKey>x</licenseKey> and
// <password><![CDATA[x]]></password>. The recorder's redactor reads the plain form for its own names; this adds this file's.
func redactXML(s string) string {
	s = xmlCDATA.ReplaceAllStringFunc(s, func(m string) string {
		sub := xmlCDATA.FindStringSubmatch(m)
		if sub[1] == sub[3] && isSecretName(sub[1]) {
			return "<" + sub[1] + sub[2] + ">" + redactedMark + "</" + sub[3] + ">"
		}
		return m
	})
	return xmlElement.ReplaceAllStringFunc(s, func(m string) string {
		sub := xmlElement.FindStringSubmatch(m)
		if sub[1] == sub[4] && isSecretName(sub[1]) {
			return "<" + sub[1] + sub[2] + ">" + redactedMark + "</" + sub[4] + ">"
		}
		return m
	})
}

// redactHTML replaces the value of an input whose name or id is in a secret family or whose type is password, and the content of a
// meta tag whose name is: <input name="authenticity_token" value="x">, <input type="password" value="x">, <meta name="csrf-token"
// content="x">.
func redactHTML(s string) string {
	return htmlTag.ReplaceAllStringFunc(s, func(tag string) string {
		attrs := map[string]string{}
		for _, m := range htmlAttr.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = strings.Trim(m[2], `"'`)
		}
		secret := strings.EqualFold(attrs["type"], "password")
		for _, key := range []string{"name", "id", "property", "http-equiv"} {
			if v, ok := attrs[key]; ok && isSecretName(v) {
				secret = true
			}
		}
		if !secret {
			return tag
		}
		return htmlAttr.ReplaceAllStringFunc(tag, func(a string) string {
			m := htmlAttr.FindStringSubmatch(a)
			switch strings.ToLower(m[1]) {
			case "value", "content":
				return m[1] + `="` + redactedMark + `"`
			}
			return a
		})
	})
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

// redactForm reads each field with its name and value decoded, and redacts it: a secret name loses its value, and any other value is
// read as the text it holds (JSON, another form, a header-like line) before it is shown.
func redactForm(s string, depth int) string {
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
			parts[i] = shortText(name, 100) + "=" + strings.TrimSpace(redactStructuredDepth(value, "", depth+1))
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
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key := shortText(k, 100)
			for n := 2; ; n++ { // two names that read the same once cleaned stay two members
				if _, taken := out[key]; !taken {
					break
				}
				key = shortText(k, 100) + " #" + strconv.Itoa(n)
			}
			if isSecretName(k) || (pair && pairValueMembers[strings.ToLower(k)]) {
				out[key] = redactedMark
			} else {
				out[key] = redactJSON(t[k], depth+1)
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
		return cleanText(redactXML(redactNamedAssignments(record.RedactBody(t), nil)), true)
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
