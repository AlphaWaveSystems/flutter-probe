package record

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// Placeholder replaces recorded credentials.
const Placeholder = "<redacted>"

// These names mirror probe_agent/lib/src/http_capture.dart.
var credentialHeaders = map[string]bool{
	"authorization": true, "proxy-authorization": true, "cookie": true,
	"set-cookie": true, "x-api-key": true, "x-goog-api-key": true,
	"api-key": true, "x-auth-token": true, "x-access-token": true,
	"x-amz-security-token": true, "x-csrf-token": true, "x-xsrf-token": true,
}
var credentialQuery = map[string]bool{
	"key": true, "api_key": true, "apikey": true, "access_token": true,
	"token": true, "id_token": true, "refresh_token": true, "auth": true,
	"secret": true, "password": true, "sig": true, "signature": true,
}

var (
	apiKeyPattern      = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
	githubTokenPattern = regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}`)
	bearerPattern      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	longHexPattern     = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	longBasePattern    = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	queryPartPattern   = regexp.MustCompile(`(^|&)([^=&]+)=([^&]*)`)
	// A credential field can hold a string or a bare number (a card number is often sent as a number).
	jsonCredentialPattern = regexp.MustCompile(`(?i)("(?:password|passwd|token|secret|authorization|refresh_token|access_token|api_key|card_number|cvv)"\s*:\s*)(?:"(?:\\.|[^"\\])*"|-?[0-9][0-9.eE+-]*)`)
	jsonStringPattern     = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
)

// Copied from internal/hosted/scrub.go, without importing the hosted package.
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
	regexp.MustCompile(`(?i)\b(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|authorization)\b["']?\s*[:=]\s*["']?[^\s"',;]{4,}`),
}

// RedactHeaders returns an independent copy with credential values removed.
func RedactHeaders(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if credentialHeaders[strings.ToLower(k)] {
			v = Placeholder
		}
		out[k] = v
	}
	return out
}

// RedactURL preserves URL spelling and query order, including malformed URLs.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.RawQuery == "" {
		return raw
	}
	// Work on the original query to avoid normalizing unrelated URL content.
	start := strings.IndexByte(raw, '?')
	if start < 0 {
		return raw
	}
	end := len(raw)
	if hash := strings.IndexByte(raw, '#'); hash >= 0 {
		if hash < start {
			return raw
		}
		end = hash
	}
	query := raw[start+1 : end]
	query = queryPartPattern.ReplaceAllStringFunc(query, func(part string) string {
		eq := strings.IndexByte(part, '=')
		name := strings.TrimPrefix(part[:eq], "&")
		if decoded, e := url.QueryUnescape(name); e == nil {
			name = decoded
		}
		if credentialQuery[strings.ToLower(name)] {
			return part[:eq+1] + Placeholder
		}
		return part
	})
	return raw[:start+1] + query + raw[end:]
}

// RedactBody removes credential shapes and credential-valued JSON strings.
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
	s = jsonCredentialPattern.ReplaceAllString(s, `${1}"`+Placeholder+`"`)
	scrub := func(value string) string {
		for _, re := range secretPatterns {
			value = re.ReplaceAllString(value, Placeholder)
		}
		return value
	}
	if !json.Valid([]byte(s)) {
		return scrub(s)
	}
	// Hosted assignment patterns must not consume JSON field names or syntax.
	var out strings.Builder
	last := 0
	for _, loc := range jsonStringPattern.FindAllStringIndex(s, -1) {
		out.WriteString(s[last:loc[0]])
		quoted := s[loc[0]:loc[1]]
		if !strings.HasPrefix(strings.TrimSpace(s[loc[1]:]), ":") {
			var value string
			_ = json.Unmarshal([]byte(quoted), &value)
			if clean := scrub(value); clean != value {
				// Quote changed values without escaping the placeholder's angle brackets.
				var b strings.Builder
				enc := json.NewEncoder(&b)
				enc.SetEscapeHTML(false)
				_ = enc.Encode(clean)
				quoted = strings.TrimSuffix(b.String(), "\n")
			}
		}
		out.WriteString(quoted)
		last = loc[1]
	}
	out.WriteString(s[last:])
	return out.String()
}
