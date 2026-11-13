package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The leak table: classes of ordinary traffic, each with a secret in it, and what must hold of what is shown. A row names a class, runs
// the redactor on a value that carries the secret, and says what must not appear.

const (
	// Three-part tokens, one signed with HS256 and one with RS256, under names that are in no family.
	netJWTHS256 = "eyJ" + "hbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." + "eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4ifQ." + "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	netJWTRS256 = "eyJ" + "hbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + "eyJpc3MiOiJodHRwczovL2V4YW1wbGUudGVzdCIsInN1YiI6IjEyMyJ9." + "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk-zDWsqBHNa8uPh1yLIQqUo8Td2hAmQCT1YpiuInYfCA"
	netB64URL   = "Zm9vYmFyLWJhei1xdXV4X2NvcmdlLWdyYXVsdC0xMjM0NTY3ODkw" // base64url, with - and _, 24 or more characters
	netCard     = "4111111111111111"                                     // passes the card number check
)

// big builds a JSON array body of about n bytes, with a credential near the start and one in the part that is cut.
func bigJSONCut(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"password":"` + netPlain + `","data":[`)
	for sb.Len() < n-200 {
		sb.WriteString(`{"id":12,"name":"widget","ok":true},`)
	}
	sb.WriteString(`{"id":13,"client_secret":"` + netPlain + `","note":"cut here ` + netPlain + strings.Repeat("x", n))
	return sb.String()[:n-20] // the agent keeps 64 KB and cuts the rest, so the end is in the middle of a value
}

func TestLeakTableBodies(t *testing.T) {
	secretJSON := `{"password":"` + netPlain + `"}`
	cases := []struct {
		name, ct, body string
		bad            []string
		show           []string
	}{
		{"JSON cut at 64 KB, the credential in the cut part", "application/json", bigJSONCut(70 * 1024), []string{netPlain}, nil},
		{"JSON cut in the middle of a value", "application/json", `{"a":1,"token":"` + netPlain[:10], []string{netPlain[:10]}, nil},
		{"NDJSON", "application/x-ndjson", secretJSON + "\n" + `{"access_token":"` + netPlain + `"}` + "\n" + `{"ok":true}`, []string{netPlain}, []string{`"ok"`}},
		{"JSON with comments", "application/json", "/* login */ {\"password\":\"" + netPlain + "\" // the secret\n}", []string{netPlain}, nil},
		{"XML with a WS-Security UsernameToken", "text/xml", `<s:Envelope><s:Header><wsse:Security><wsse:UsernameToken><wsse:Username>bob</wsse:Username><wsse:Password Type="x">` + netPlain + `</wsse:Password></wsse:UsernameToken></wsse:Security></s:Header></s:Envelope>`, []string{netPlain}, []string{"bob"}},
		{"XML credential elements", "application/xml", `<login><user>bob</user><apiKey>` + netPlain + `</apiKey><token>` + netPlain + `</token></login>`, []string{netPlain}, []string{"bob"}},
		{"multipart with a credential part", "multipart/form-data; boundary=xyz", "--xyz\r\nContent-Disposition: form-data; name=\"user\"\r\n\r\nbob\r\n--xyz\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\n" + netPlain + "\r\n--xyz--", []string{netPlain}, []string{"bob"}},
		{"GraphQL with inline arguments", "application/json", `{"query":"mutation { login(user: \"bob\", password: \"` + netPlain + `\") { id } }"}`, []string{netPlain}, nil},
		{"GraphQL with variables", "application/json", `{"query":"mutation($p:String){login(password:$p){id}}","variables":{"p":"x","password":"` + netPlain + `"}}`, []string{netPlain}, nil},
		{"JSON inside a string", "application/json", `{"payload":"{\"password\":\"` + netPlain + `\"}"}`, []string{netPlain}, nil},
		{"a JWT under a name in no family (HS256)", "application/json", `{"payload":"` + netJWTHS256 + `"}`, []string{"eyJ", "SflKxwRJ"}, nil},
		{"a JWT under a name in no family (RS256)", "application/json", `{"blob":"` + netJWTRS256 + `"}`, []string{"eyJ", "dBjftJeZ"}, nil},
		{"a JWT in text", "text/plain", "got " + netJWTHS256 + " back", []string{"eyJ"}, []string{"got"}},
		{"a base64url run", "application/json", `{"opaque":"` + netB64URL + `"}`, []string{netB64URL, netB64URL[:20]}, nil},
		{"a card number, spaced, dashed and plain", "text/plain", "a 4111 1111 1111 1111 b 4111-1111-1111-1111 c " + netCard, []string{"4111"}, []string{"a", "b", "c"}},
		{"a card number under a parent", "application/json", `{"card":{"number":"` + netCard + `","holder":"Bob"}}`, []string{netCard}, []string{"Bob"}},
		{"a pair named by a family only this file knows", "application/json", `{"name":"xsrf","value":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a pair with a list of names this file knows", "application/json", `{"field":["a","xsrf"],"data":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a pair named by param", "application/json", `{"param":"password","value":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"subscription, functions and app keys as body names", "application/json", `{"subscription_key":"` + netPlain + `","functions_key":"` + netPlain + `","app_key":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a short JWT", "text/plain", "t eyJhbGciOiJub25lIn0.eyJhIjoxfQ.abcd end", []string{"eyJ"}, []string{"t ", " end"}},
		{"a name/value pair", "application/json", `[{"name":"api_key","value":"` + netPlain + `"},{"name":"colour","value":"blue"}]`, []string{netPlain}, []string{"blue"}},
		{"a key/val pair", "application/json", `{"key":"password","val":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a field/data pair with a list of names", "application/json", `{"field":["x","otp"],"data":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"the names pass, sid, passphrase", "application/json", `{"pass":"` + netPlain + `","sid":"` + netPlain + `","passphrase":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"the names code_verifier, code, otp_code, verification_code, private", "application/json", `{"code_verifier":"` + netPlain + `","code":"` + netPlain + `","otp_code":"` + netPlain + `","verification_code":"` + netPlain + `","private":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a full-width name", "application/json", `{"ｐａｓｓｗｏｒｄ":"` + netPlain + `"}`, []string{netPlain}, nil},
		{"a percent-encoded name", "application/x-www-form-urlencoded", "%70assword=" + netPlain + "&a=1", []string{netPlain}, nil},
		{"a cookie jar name in a form with no type", "", "cookiejar=" + netPlain + "&a=1", []string{netPlain}, []string{"a=1"}},
	}
	for _, c := range cases {
		b := redactBody(c.body, 0, c.ct)
		mustNotContain(t, c.name, b.Text, c.bad...)
		for _, s := range c.show {
			if !strings.Contains(b.Text, s) {
				t.Errorf("%s: %q is not shown in %q", c.name, s, b.Text)
			}
		}
	}
	// Values that look like a card number but do not pass the check, and ordinary numbers, are left as they are.
	for _, keep := range []string{"1234567890123", "order 20240115", "id 9007199254740993"} {
		if got := redactBody(keep, 0, "text/plain").Text; got != keep {
			t.Errorf("%q was changed to %q", keep, got)
		}
	}
	// A UUID is kept: it is an identifier, and the limit is stated.
	if got := redactBody(`{"id":"123e4567-e89b-12d3-a456-426614174000"}`, 0, "application/json").Text; !strings.Contains(got, "123e4567-e89b-12d3-a456-426614174000") {
		t.Errorf("a UUID was redacted: %q", got)
	}
}

// Headers whose names are not in a short list but are credentials, and tokens under ordinary names.
func TestLeakTableHeaders(t *testing.T) {
	secretNames := []string{
		"Ocp-Apim-Subscription-Key", "X-Functions-Key", "x-app-key", "X-Master-Key", "Fastly-Key", "X-Custom-Key", "Foo_Key", "My Key", "key",
		"X-Sid", "X-Pass", "X-Code-Verifier", "X-Verification-Code", "X-Otp-Code", "X-Passphrase",
		"ｘ-ａｐｉ-ｋｅｙ", "%58-Api-%4Bey", "X-Hidden​-Token",
	}
	for _, n := range secretNames {
		hs, _ := redactHeaders(map[string]string{n: netPlain})
		if hs[0].Value != redactedMark {
			t.Errorf("%q: value %q", n, hs[0].Value)
		}
	}
	for _, n := range []string{"Keyboard", "Monkey", "Turkey", "X-Keyed", "Accept-Charset"} {
		if hs, _ := redactHeaders(map[string]string{n: "ok"}); hs[0].Value != "ok" {
			t.Errorf("%q was redacted", n)
		}
	}
	hs, _ := redactHeaders(map[string]string{"DPoP": netJWTRS256, "X-Firebase-AppCheck": netJWTHS256, "X-Opaque": netB64URL, "X-Card": netCard})
	all := fmt.Sprint(hs)
	mustNotContain(t, "token headers", all, "eyJ", netB64URL[:20], netCard)
	// The name families this file adds to the recorder's, one at a time: each name is shown and its value is not.
	for _, n := range []string{"sig", "iban", "csrf", "cardnumber", "signature", "xsrf", "SSN", "CVC", "smscode", "mfacode", "jsessionid", "pincode"} {
		if hs, _ := redactHeaders(map[string]string{n: netPlain}); hs[0].Value != redactedMark {
			t.Errorf("%q: %q", n, hs[0].Value)
		}
	}
	// Origin carries a URL like Location and Referer do.
	hs, _ = redactHeaders(map[string]string{"Origin": "https://u:" + netPlain + "@app.example.test/p?code=XYZ12&a=b"})
	if !strings.Contains(hs[0].Value, "code=[redacted]") || strings.Contains(hs[0].Value, netPlain) {
		t.Errorf("Origin = %q", hs[0].Value)
	}
}

func TestLeakTableURLs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://h.example.test/p?" + netToken, "https://h.example.test/p?[redacted]"},
		{"https://h.example.test/p?a=1&SECRETTOKENabc", "https://h.example.test/p?a=[redacted]&[redacted]"},
		{"https://bob:pa?ss@h.example.test/p?x=1", "https://h.example.test/p?x=[redacted]"},
		{"https://bob:pa#ss@h.example.test/p", "https://h.example.test/p"},
		{"https://bob:" + netPlain + "@h.example.test:8443/p;jsessionid=" + netPlain, "https://h.example.test:8443/p;jsessionid=[redacted]"},
		{"//bob:" + netPlain + "@h.example.test/p", "//h.example.test/p"},
		{"redis://:" + netPlain + "@h.example.test:6379/0", "redis://h.example.test:6379/0"},
		{"https://h.example.test/p?%74oken=" + netPlain, "https://h.example.test/p?token=[redacted]"},
		{"https://h.example.test/cb?state=" + netJWTHS256, "https://h.example.test/cb?state=[redacted]"},
	}
	for _, c := range cases {
		if got := redactURL(c.in); got != c.want {
			t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A secret in a path segment that has a secret shape is caught by the shape.
	if got := redactURL("https://h.example.test/reset/" + netJWTHS256); strings.Contains(got, "eyJ") {
		t.Errorf("a JWT in a path: %q", got)
	}
}

// The exchange's error text can hold a URL with a query: the values are not shown, and the filter searches what is shown.
func TestLeakTableErrorText(t *testing.T) {
	e := probelink.HTTPEntry{Seq: 1, Method: "GET", URL: "https://h.example.test/p", Error: `Get "https://bob:` + netPlain + `@h.example.test/p?token=` + netPlain + `&id=7": dial tcp 10.0.0.5:443: connect: refused; password=` + netPlain + ` ` + netJWTHS256}
	r := netRow(e)
	mustNotContain(t, "error text", r.Error, netPlain, "eyJ", "bob")
	if !strings.Contains(r.Error, "token=[redacted]") || !strings.Contains(r.Error, "dial tcp") {
		t.Errorf("error = %q", r.Error)
	}
	if (NetFilter{Text: netPlain}).matches(r) || !(NetFilter{Text: "dial tcp"}).matches(r) {
		t.Error("the filter must search what is shown")
	}
}

// A filter text longer than the limit is cut by characters, not bytes.
func TestFilterTextIsCutByCharacters(t *testing.T) {
	needle := filterNeedle("  " + strings.Repeat("É€", 400) + "  ")
	if !utf8.ValidString(needle) || len([]rune(needle)) != netMaxFilter || needle != strings.ToLower(strings.Repeat("É€", netMaxFilter/2)) {
		t.Errorf("needle: %d characters, valid %v", len([]rune(needle)), utf8.ValidString(needle))
	}
	if filterNeedle("   ") != "" || filterNeedle("AbC") != "abc" {
		t.Error("filterNeedle trims and lower-cases")
	}
	// A row that holds the first 200 characters matches a text that runs on past them.
	row := NetRow{URL: strings.Repeat("é", netMaxFilter)}
	if !(NetFilter{Text: strings.Repeat("é", netMaxFilter+50)}).matches(row) {
		t.Error("a long filter text does not match after being cut")
	}
}

// The JSON that is shown is not HTML-escaped, so markup in a value reads as it was written.
func TestJSONBodyIsNotHTMLEscaped(t *testing.T) {
	b := redactBody(`{"a":"<b>x</b> & y"}`, 0, "application/json")
	if !strings.Contains(b.Text, "<b>x</b> & y") || strings.Contains(b.Text, "\\u003c") {
		t.Errorf("body = %q", b.Text)
	}
}

// JSON followed by more is not cut to its first value: the rest is shown, redacted.
func TestJSONWithTrailingDataKeepsTheRest(t *testing.T) {
	b := redactBody(`{"a":"first"}`+"\n"+`{"b":"second-value"}`, 0, "application/json")
	if !strings.Contains(b.Text, "first") || !strings.Contains(b.Text, "second-value") {
		t.Errorf("a later value was dropped: %q", b.Text)
	}
}

// A form with no content type is read as one: a name in a family that the recorder's rule does not know is redacted.
func TestFormWithoutAContentType(t *testing.T) {
	if !looksLikeForm("cookiejar=x&a=1") || looksLikeForm("a b=1") || looksLikeForm("{a=1}") || looksLikeForm("a=1&b") {
		t.Error("looksLikeForm")
	}
}
