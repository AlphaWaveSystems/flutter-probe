package record

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// Placeholder replaces recorded credentials.
const Placeholder = "<redacted>"

// These exact names mirror probe_agent/lib/src/http_capture.dart. A header whose name also passes the token rule is masked
// too (see RedactHeaders).
var credentialHeaders = map[string]bool{
	"authorization": true, "proxy-authorization": true, "cookie": true,
	"set-cookie": true, "x-api-key": true, "x-goog-api-key": true,
	"api-key": true, "x-auth-token": true, "x-access-token": true,
	"x-amz-security-token": true, "x-csrf-token": true, "x-xsrf-token": true,
}

// credentialQuery are query and fragment names that are always redacted. They predate the token rule, and they include
// names such as key and sig, which no token covers.
var credentialQuery = map[string]bool{
	"key": true, "api_key": true, "apikey": true, "access_token": true,
	"token": true, "id_token": true, "refresh_token": true, "auth": true,
	"secret": true, "password": true, "sig": true, "signature": true,
	"x-amz-signature": true, "x-goog-signature": true, "x-amz-security-token": true, "x-amz-credential": true,
}

// credentialTokens are the words that mark a name as a credential when one of its tokens is the word. A name is split
// into tokens by sysdialog.NameTokens, so current_password, currentPassword and passwordHash each hold the token password,
// and shipping holds no token even though it contains pin.
var credentialTokens = map[string]bool{
	"password": true, "passwd": true, "pwd": true, "pw": true, "pass": true, "passphrase": true, "passcode": true,
	"secret": true, "secrets": true, "token": true, "tokens": true, "jwt": true,
	"credential": true, "credentials": true, "otp": true, "pin": true, "cvv": true, "cvc": true, "ssn": true,
	"auth": true, "authorization": true, "session": true, "bearer": true, "cookie": true,
	"apikey": true, "apikeys": true, "sid": true, "csrf": true,
	"signature": true, "sig": true,
}

// credentialPairs are two adjacent tokens that mark a name as a credential together: api_key, private_key, card_number,
// signing_key, encryption_key and access_key_id (access and key).
var credentialPairs = map[string]bool{
	"api key": true, "private key": true, "card number": true, "signing key": true, "encryption key": true,
	"access key": true, "card no": true, "credit card": true,
}

// credentialCompounds are credential names written as one word, which the tokens cannot split. A name matches when the
// whole name, with separators removed, is one of them, so a longer word that contains one does not match.
var credentialCompounds = map[string]bool{
	"accesskey": true, "accesstoken": true, "refreshtoken": true, "idtoken": true, "clientsecret": true,
	"pincode": true, "otpcode": true, "cardnumber": true, "ccnumber": true, "smscode": true, "mfacode": true,
	"authcode": true, "verificationcode": true, "passwordconfirmation": true, "passwordhash": true,
	"jsessionid": true, "phpsessid": true, "sessionid": true, "cardno": true, "creditcard": true,
}

// isCredentialName reports whether a name, already percent-decoded, holds a credential: one of its tokens is in
// credentialTokens, two adjacent tokens are in credentialPairs, or the whole name with separators removed is in
// credentialCompounds.
func isCredentialName(name string) bool {
	var compact strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			compact.WriteRune(r)
		}
	}
	if credentialCompounds[compact.String()] {
		return true
	}
	tokens := sysdialog.NameTokens(name)
	for i, tok := range tokens {
		if credentialTokens[tok] {
			return true
		}
		if i+1 < len(tokens) && credentialPairs[tok+" "+tokens[i+1]] {
			return true
		}
	}
	return false
}

// decodeName returns name with its percent escapes read, or name itself when the escapes do not decode.
func decodeName(name string) string {
	if decoded, err := url.QueryUnescape(name); err == nil {
		return decoded
	}
	return name
}

var (
	apiKeyPattern      = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
	githubTokenPattern = regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}`)
	bearerPattern      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	longHexPattern     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	longBasePattern    = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	queryPartPattern   = regexp.MustCompile(`(^|&)([^=&]+)=([^&]*)`)
	// userinfoPattern matches the password of a URL's userinfo, absolute or scheme-relative: scheme://user:password@host
	// or //user:password@host. The user may be empty, as in redis://:password@host.
	userinfoPattern = regexp.MustCompile(`((?:[A-Za-z][A-Za-z0-9+.\-]*:)?//[^/?#\s:@]*:)[^/?#\s@]+@`)
	// matrixParamPattern matches a path parameter, as in ;jsessionid=X.
	matrixParamPattern = regexp.MustCompile(`;([A-Za-z0-9_.\-]+)=([^;/?#&\s]+)`)
	// xmlElementPattern matches an element whose content is text: <name attributes>text</name>. Go's regexp has no
	// backreferences, so redactXMLElements compares the opening and closing names itself.
	xmlElementPattern = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.:\-]*)(?:\s[^<>]*)?>([^<]*)</([A-Za-z_][A-Za-z0-9_.:\-]*)\s*>`)
	// multipartPartPattern matches a multipart part: the form name, quoted or not, in its Content-Disposition line, then the value after
	// the blank line that ends the part's header lines, such as a Content-Type line.
	multipartPartPattern = regexp.MustCompile(`(?i)(?:^|[\s;])name=(?:"([^"]*)"|([^";\s\r\n]*))[^\r\n]*(?:\r?\n[^\r\n]+)*\r?\n\r?\n([^\r\n]*)`)
	// credentialHeaderPattern matches a credential header line in text, with its name and colon, and captures its value.
	credentialHeaderPattern = regexp.MustCompile(`(?im)^([ \t]*(?:proxy-authorization|authorization|set-cookie|cookie)[ \t]*:[ \t]*)([^\r\n]*)`)
	// credentialPairPattern matches a name, a separator and a value in plain or form text: name=value, name: value and
	// name%3Dvalue, with quotes allowed around the separator and the value. The name may hold brackets and percent escapes
	// (user[password] and user%5Bpassword%5D). A value needs four characters, so a short value is left as written.
	credentialPairPattern = regexp.MustCompile(`((?:[A-Za-z0-9_.\-]|\[|\]|%[0-9A-Fa-f]{2})+)["']?(?:\s*[=:]\s*|%3[Dd]|%3[Aa])["']?([^\s&,;"'<>]{4,})`)
)

// Copied from internal/hosted/scrub.go, without importing the hosted package. The assignment rule that used to be its
// last entry is credentialPairPattern, which reads the name by token.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{10,}`),
	regexp.MustCompile("whsec" + `_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`\bFP2\.[A-Za-z0-9._-]{10,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{12,}`),
}

// RedactHeaders returns an independent copy with credential values removed. A header is a credential when its name is one
// of the fixed names, or when its name holds a credential word by the token rule, as X-Api-Token and X-Amz-Signature do.
func RedactHeaders(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if credentialHeaders[strings.ToLower(k)] || isCredentialName(k) {
			v = Placeholder
		}
		out[k] = v
	}
	return out
}

// RedactURL preserves URL spelling and parameter order, including malformed URLs. The password of a userinfo is always
// masked. A parameter is masked when its name is a credential: a query parameter, a fragment parameter, or a path parameter
// such as ;jsessionid=.
func RedactURL(raw string) string {
	raw = redactUserinfo(raw)
	raw = redactMatrixParams(raw)
	hash := strings.IndexByte(raw, '#')
	if hash >= 0 {
		raw = raw[:hash+1] + redactParams(raw[hash+1:])
	}
	// Work on the original query to avoid normalizing unrelated URL content.
	start := strings.IndexByte(raw, '?')
	if start < 0 || (hash >= 0 && start > hash) {
		return raw
	}
	end := len(raw)
	if hash >= 0 {
		end = hash
	}
	return raw[:start+1] + redactParams(raw[start+1:end]) + raw[end:]
}

// redactParams masks the value of each name=value parameter in s, joined by &, whose name is a credential.
func redactParams(s string) string {
	return queryPartPattern.ReplaceAllStringFunc(s, func(part string) string {
		eq := strings.IndexByte(part, '=')
		name := decodeName(strings.TrimPrefix(part[:eq], "&"))
		if credentialQuery[strings.ToLower(name)] || isCredentialName(name) {
			return part[:eq+1] + Placeholder
		}
		return part
	})
}

// redactUserinfo masks the password in every URL userinfo in s.
func redactUserinfo(s string) string {
	return userinfoPattern.ReplaceAllString(s, "${1}"+Placeholder+"@")
}

// redactMatrixParams masks the value of each path parameter whose name is a credential, as in ;jsessionid=X.
func redactMatrixParams(s string) string {
	return matrixParamPattern.ReplaceAllStringFunc(s, func(param string) string {
		m := matrixParamPattern.FindStringSubmatch(param)
		if isCredentialName(decodeName(m[1])) {
			return ";" + m[1] + "=" + Placeholder
		}
		return param
	})
}

// RedactBody removes credential shapes, the password of a URL's userinfo, and credential members and assignments. A JSON
// body keeps its layout: the value of each credential member becomes the placeholder, whatever its type.
func RedactBody(s string) string {
	for _, re := range []*regexp.Regexp{apiKeyPattern, githubTokenPattern, bearerPattern, longHexPattern} {
		s = re.ReplaceAllString(s, Placeholder)
	}
	s = longBasePattern.ReplaceAllStringFunc(s, func(value string) string {
		if strings.ContainsAny(value, "0123456789+/=") {
			return Placeholder
		}
		return value
	})
	s = redactUserinfo(s)
	if !json.Valid([]byte(s)) {
		return redactSecretText(s)
	}
	c := jsonCursor{s: s}
	var out strings.Builder
	c.space(&out)
	c.value(&out, false, redactSecretText)
	if c.i < len(s) {
		out.WriteString(s[c.i:])
	}
	return out.String()
}

// redactSecretText removes credential shapes, credential elements, parts, header lines, path parameters and assignments
// from plain text, and from one string value of a JSON body.
func redactSecretText(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, Placeholder)
	}
	s = redactXMLElements(s)
	s = redactMultipartParts(s)
	s = credentialHeaderPattern.ReplaceAllString(s, "${1}"+Placeholder)
	s = redactMatrixParams(s)
	return redactAssignments(s)
}

// redactXMLElements masks the text of an element whose name is a credential, as in <password>X</password>. An element
// that has child elements is not matched here, and its children are masked on their own.
func redactXMLElements(s string) string {
	var out strings.Builder
	last := 0
	for _, loc := range xmlElementPattern.FindAllStringSubmatchIndex(s, -1) {
		if s[loc[2]:loc[3]] != s[loc[6]:loc[7]] {
			continue
		}
		out.WriteString(s[last:loc[4]])
		if isCredentialName(s[loc[2]:loc[3]]) {
			out.WriteString(Placeholder)
		} else {
			out.WriteString(s[loc[4]:loc[5]])
		}
		last = loc[5]
	}
	out.WriteString(s[last:])
	return out.String()
}

// redactMultipartParts masks the value of a multipart part whose form name is a credential.
func redactMultipartParts(s string) string {
	var out strings.Builder
	last := 0
	for _, loc := range multipartPartPattern.FindAllStringSubmatchIndex(s, -1) {
		// The name is the quoted group when it matched, else the unquoted one.
		name := ""
		switch {
		case loc[2] >= 0:
			name = s[loc[2]:loc[3]]
		case loc[4] >= 0:
			name = s[loc[4]:loc[5]]
		}
		out.WriteString(s[last:loc[6]])
		if isCredentialName(decodeName(name)) {
			out.WriteString(Placeholder)
		} else {
			out.WriteString(s[loc[6]:loc[7]])
		}
		last = loc[7]
	}
	out.WriteString(s[last:])
	return out.String()
}

// redactAssignments replaces each assignment whose name is a credential with the placeholder. The whole assignment is
// replaced, as before, so the name does not survive.
func redactAssignments(s string) string {
	var out strings.Builder
	last := 0
	for _, loc := range credentialPairPattern.FindAllStringSubmatchIndex(s, -1) {
		out.WriteString(s[last:loc[0]])
		if isCredentialName(decodeName(s[loc[2]:loc[3]])) {
			out.WriteString(Placeholder)
		} else {
			out.WriteString(s[loc[0]:loc[1]])
		}
		last = loc[1]
	}
	out.WriteString(s[last:])
	return out.String()
}

// pairNameMembers are the member names that name the credential in a name/value pair object.
var pairNameMembers = map[string]bool{"name": true, "key": true, "field": true, "id": true, "label": true}

// pairValueMembers are the member names that hold the value of a name/value pair object, matched without case.
var pairValueMembers = map[string]bool{"value": true, "val": true, "data": true, "content": true, "text": true, "newvalue": true}

// namesCredential reports whether the JSON object in raw has a pair-name member that names a credential, as a string or as one of an array of strings, so
// that its value member is a credential as well.
func namesCredential(raw string) bool {
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return false
	}
	for member, v := range obj {
		if !pairNameMembers[strings.ToLower(member)] {
			continue
		}
		switch names := v.(type) {
		case string:
			if isCredentialName(names) {
				return true
			}
		case []any:
			for _, item := range names {
				if s, ok := item.(string); ok && isCredentialName(s) {
					return true
				}
			}
		}
	}
	return false
}

// jsonCursor copies a JSON document, which json.Valid has accepted, and replaces the value of each credential member.
type jsonCursor struct {
	s string
	i int
}

// peek returns the byte at the cursor, or 0 at the end.
func (c *jsonCursor) peek() byte {
	if c.i >= len(c.s) {
		return 0
	}
	return c.s[c.i]
}

// space copies the whitespace at the cursor to out.
func (c *jsonCursor) space(out *strings.Builder) {
	for c.i < len(c.s) && strings.IndexByte(" \t\r\n", c.s[c.i]) >= 0 {
		out.WriteByte(c.s[c.i])
		c.i++
	}
}

// str returns the raw text of the string at the cursor, quotes included, and moves past it.
func (c *jsonCursor) str() string {
	start := c.i
	for c.i++; c.i < len(c.s); c.i++ {
		switch c.s[c.i] {
		case '\\':
			c.i++
		case '"':
			c.i++
			return c.s[start:c.i]
		}
	}
	c.i = len(c.s)
	return c.s[start:]
}

// scalar returns the raw text of a number, true, false or null at the cursor, and moves past it.
func (c *jsonCursor) scalar() string {
	start := c.i
	for c.i < len(c.s) && strings.IndexByte(",]} \t\r\n", c.s[c.i]) < 0 {
		c.i++
	}
	return c.s[start:c.i]
}

// skip moves past the value at the cursor without copying it.
func (c *jsonCursor) skip() {
	switch c.peek() {
	case '"':
		c.str()
	case '{', '[':
		depth := 0
		for c.i < len(c.s) {
			switch c.s[c.i] {
			case '"':
				c.str()
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			c.i++
			if depth == 0 {
				return
			}
		}
	default:
		c.scalar()
	}
}

// value copies the value at the cursor to out. A credential value becomes the placeholder whatever its type. Other
// strings go through text, and the members and elements of objects and arrays are copied in turn, so a credential
// member nested inside them is replaced too.
func (c *jsonCursor) value(out *strings.Builder, credential bool, text func(string) string) {
	if credential {
		c.skip()
		out.WriteString(`"` + Placeholder + `"`)
		return
	}
	switch c.peek() {
	case '{':
		// A name/value pair object, such as {"field":"password","value":"x"}, masks its value member when a name, key, field,
		// id or label member names a credential.
		start := c.i
		c.skip()
		pair := namesCredential(c.s[start:c.i])
		c.i = start
		out.WriteByte('{')
		c.i++
		for {
			c.space(out)
			switch c.peek() {
			case '}':
				out.WriteByte('}')
				c.i++
				return
			case ',':
				out.WriteByte(',')
				c.i++
			case '"':
				key := c.str()
				out.WriteString(key)
				c.space(out)
				out.WriteByte(':')
				c.i++
				c.space(out)
				var name string
				_ = json.Unmarshal([]byte(key), &name)
				c.value(out, isCredentialName(name) || (pair && pairValueMembers[strings.ToLower(name)]), text)
			default:
				return
			}
		}
	case '[':
		out.WriteByte('[')
		c.i++
		for {
			c.space(out)
			switch c.peek() {
			case ']':
				out.WriteByte(']')
				c.i++
				return
			case ',':
				out.WriteByte(',')
				c.i++
			case 0, '}':
				return
			default:
				c.value(out, false, text)
			}
		}
	case '"':
		raw := c.str()
		var decoded string
		_ = json.Unmarshal([]byte(raw), &decoded)
		clean := text(decoded)
		if clean == decoded {
			out.WriteString(raw)
		} else {
			out.WriteString(jsonQuote(clean))
		}
	default:
		out.WriteString(c.scalar())
	}
}

// jsonQuote encodes v as a JSON string. It leaves <, > and & unescaped, as the placeholder is written.
func jsonQuote(v string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}
