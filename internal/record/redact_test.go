package record

import (
	"strings"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	names := []string{"authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key", "api-key", "x-auth-token", "x-access-token", "x-amz-security-token", "x-csrf-token", "x-xsrf-token"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			key := strings.ToUpper(name)
			h := map[string]string{key: "sensitive", "Content-Type": "application/json", "prefix-" + name: "ordinary"}
			got := RedactHeaders(h)
			if got[key] != Placeholder || got["Content-Type"] != "application/json" || got["prefix-"+name] != Placeholder {
				t.Fatalf("unexpected headers: %#v", got)
			}
			if h[key] != "sensitive" {
				t.Fatal("input mutated")
			}
			got["Content-Type"] = "changed"
			if h["Content-Type"] != "application/json" {
				t.Fatal("copy aliases input")
			}
		})
	}
	if got := RedactHeaders(nil); len(got) != 0 {
		t.Fatalf("nil: %#v", got)
	}
}

func TestRedactURL(t *testing.T) {
	for _, name := range []string{"key", "api_key", "apikey", "access_token", "token", "id_token", "refresh_token", "auth", "secret", "password", "sig", "signature"} {
		t.Run(name, func(t *testing.T) {
			for _, prefix := range []string{"https://example.test/path", "http://[broken", "/relative"} {
				raw := prefix + "?" + strings.ToUpper(name) + "=sensitive&shipping=safe&" + name + "=again#fragment"
				want := prefix + "?" + strings.ToUpper(name) + "=" + Placeholder + "&shipping=safe&" + name + "=" + Placeholder + "#fragment"
				if got := RedactURL(raw); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
		})
	}
	cases := []struct{ in, want string }{
		{"https://example.test/path?sort=a%20b&flag#token=visible", "https://example.test/path?sort=a%20b&flag#token=<redacted>"},
		{"/path?%74oken=hidden&key=&token=last", "/path?%74oken=<redacted>&key=<redacted>&token=<redacted>"},
		{"/path#fragment?token=visible", "/path#fragment?token=<redacted>"},
		{"http://[broken?token=%zz&safe=ok", "http://[broken?token=<redacted>&safe=ok"},
		{"/path?token&safe=ok", "/path?token&safe=ok"},
		{"/path", "/path"},
		{"https://example.test/path#access_token=abc123&state=ok", "https://example.test/path#access_token=<redacted>&state=ok"},
		{"https://example.test/path;jsessionid=abc123/list", "https://example.test/path;jsessionid=<redacted>/list"},
		{"//alice:hunter2@example.test/x", "//alice:<redacted>@example.test/x"},
	}
	for _, tc := range cases {
		if got := RedactURL(tc.in); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRedactBody(t *testing.T) {
	cases := []struct{ name, positive, negative string }{
		{"api key", "sk-Abc123_-", "sk-Abc123_"},
		{"github", "ghp_Abcdefghijklmnopqr123", "ghp_Abcdefghijklmnopqr1"},
		{"bearer", "BeArEr abc12345", "Bearer abc1234"},
		{"hex", strings.Repeat("a1", 16), strings.Repeat("a1", 15)},
		{"base digit", strings.Repeat("Z", 39) + "1", strings.Repeat("Z", 38) + "1"},
		{"base plus", strings.Repeat("Z", 39) + "+", strings.Repeat("Z", 60)},
		{"base slash", strings.Repeat("Z", 39) + "/", "ordinary words"},
		{"base padding", strings.Repeat("Z", 40) + "=", "550e8400-e29b-41d4-a716-446655440000"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----", "-----BEGIN PUBLIC KEY-----abc"},
		{"aws", "AKIA1234567890ABCDEF", "AKIA1234567890ABCDE"},
		{"github variants", "gho_Abcdefghijklmnopqr123", "gho_Abcdefghijklmnopqr1"},
		{"github pat", "github_pat_Abcdefghijklmnopqr123", "github_pat_Abcdefghijklmnopqr1"},
		// Split so the source holds no complete secret-shaped token; the repository's scanner flags those.
		{"stripe", "sk_" + "live_Abcdef1234", "sk_" + "live_Abcdef123"},
		{"stripe restricted", "rk_" + "test_Abcdef1234", "rk_" + "test_Abcdef123"},
		{"webhook", "whs" + "ec_Abcdef1234", "whs" + "ec_Abcdef123"},
		{"slack", "xoxb-Abcdef1234", "xoxb-Abcdef123"},
		{"jwt", "eyJabcdefgh.abcdefgh.abcdefgh", "eyJabcdefg.abcdefgh.abcdefgh"},
		{"probe", "FP2.Abcdef1234", "FP2.Abcdef123"},
		{"assignment", "password=abcd", "password=abc"},
		{"pin assignment", "pin=1234", "pin=123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactBody(tc.positive); got != Placeholder {
				t.Errorf("positive: got %q", got)
			}
			if got := RedactBody(tc.negative); got != tc.negative {
				t.Errorf("negative: got %q, want %q", got, tc.negative)
			}
		})
	}
}

func TestRedactBodyJSON(t *testing.T) {
	for _, name := range []string{"password", "passwd", "token", "secret", "authorization", "refresh_token", "access_token", "api_key", "card_number", "cvv"} {
		t.Run(name, func(t *testing.T) {
			prefix := `{"` + strings.ToUpper(name) + `" : `
			suffix := `, "username": "alice", "count": 42, "ok": true, "id": "550e8400-e29b-41d4-a716-446655440000", "shipping": "safe"}`
			in := prefix + `"a\"b\\c"` + suffix
			want := prefix + `"<redacted>"` + suffix
			if got := RedactBody(in); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
	cases := []struct{ in, want string }{
		{`{"token":"", "cvv":"1", "nested":{"password":"two words"}}`, `{"token":"<redacted>", "cvv":"<redacted>", "nested":{"password":"<redacted>"}}`},
		{`{"message":"password=abcd", "normal":"hello"}`, `{"message":"<redacted>", "normal":"hello"}`},
		{`{"token":123,"tokens":"safe","description":"ordinary words"}`, `{"token":"<redacted>","tokens":"<redacted>","description":"ordinary words"}`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}

// A JSON member is redacted by its name's tokens: a credential word in any position, in snake, camel or kebab case, with a
// digit after it, or as an adjacent pair. A name that only contains a credential word inside one token is kept.
func TestRedactBodyCredentialNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"new_password":"abcdef"}`, `{"new_password":"<redacted>"}`},
		{`{"newPassword":"abcdef"}`, `{"newPassword":"<redacted>"}`},
		{`{"new-password":"abcdef"}`, `{"new-password":"<redacted>"}`},
		{`{"confirm_password":"abcdef"}`, `{"confirm_password":"<redacted>"}`},
		{`{"passwordConfirmation":"abcdef"}`, `{"passwordConfirmation":"<redacted>"}`},
		{`{"pass":"abcdef"}`, `{"pass":"<redacted>"}`},
		{`{"old_pin":1234}`, `{"old_pin":"<redacted>"}`},
		{`{"ssn":"123-45-6789"}`, `{"ssn":"<redacted>"}`},
		{`{"card_number":4111111111111111}`, `{"card_number":"<redacted>"}`},
		{`{"Access-Token":"abcdef"}`, `{"Access-Token":"<redacted>"}`},
		// code is not a credential token on its own, so a bare code member is kept. verification_code is a credential through
		// its compound spelling, and otp_code through the otp token.
		{`{"code":"123456"}`, `{"code":"123456"}`},
		{`{"verification_code":"123456"}`, `{"verification_code":"<redacted>"}`},
		{`{"otp_code":"123456"}`, `{"otp_code":"<redacted>"}`},
		{
			`{"statusCode":200,"orderNumber":"42","bypass":"ok","tokenizer":"safe"}`,
			`{"statusCode":200,"orderNumber":"42","bypass":"ok","tokenizer":"safe"}`,
		},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.in, got, tc.want)
		}
	}
}

// The reviewer's names: each holds a credential token, through a camel, snake or digit boundary, or as an adjacent pair.
// shipping contains pin but holds no token, so it is kept.
func TestRedactBodyTokenNames(t *testing.T) {
	for _, name := range []string{"current_password", "currentPassword", "secretKey", "api_secret", "private_key", "jwt", "session", "passwordHash", "passwd2"} {
		t.Run(name, func(t *testing.T) {
			in := `{"` + name + `":"abcdef","shipping":"DHLExpress"}`
			want := `{"` + name + `":"<redacted>","shipping":"DHLExpress"}`
			if got := RedactBody(in); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}

// The token words added in round four, and the key pairs: each name is redacted.
func TestRedactBodyNewTokenWords(t *testing.T) {
	for _, name := range []string{"passphrase", "passcode", "pw", "secrets", "apikeys", "sid", "csrf", "signing_key", "encryption_key", "access_key_id"} {
		t.Run(name, func(t *testing.T) {
			in := `{"` + name + `":"abcdef"}`
			if got, want := RedactBody(in), `{"`+name+`":"<redacted>"}`; got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}

// A credential member keeps its place when its value is not a string: a number, a boolean, null, an object or an array.
func TestRedactBodyNonStringValues(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"token":true}`, `{"token":"<redacted>"}`},
		{`{"pin":null}`, `{"pin":"<redacted>"}`},
		{`{"session":{"id":"abc","user":"alice"}}`, `{"session":"<redacted>"}`},
		{`{"card_number":[4111,1111]}`, `{"card_number":"<redacted>"}`},
		{`{"items":[{"password":"abcdef"}],"count":1}`, `{"items":[{"password":"<redacted>"}],"count":1}`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.in, got, tc.want)
		}
	}
}

// A form or plain-text assignment is redacted by the same token rule. The whole assignment becomes the placeholder, as it
// did before, so the name does not survive.
func TestRedactBodyFormAssignments(t *testing.T) {
	cases := []struct{ in, want string }{
		{"new_password=hunter2", Placeholder},
		{"user[password]=hunter2", Placeholder},
		{"user%5Bpassword%5D=hunter2", Placeholder},
		{"password%3Dhunter2", Placeholder},
		{"password: hunter2", Placeholder},
		{"a=1&new_password=hunter2&b=2", "a=1&" + Placeholder + "&b=2"},
		{"shipping=DHLExpress&b=2", "shipping=DHLExpress&b=2"},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// XML: the text of an element whose name is a credential is masked. An element with child elements is left to its children.
func TestRedactBodyXMLElements(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<login><password>hunter2</password><user>alice</user></login>`, `<login><password><redacted></password><user>alice</user></login>`},
		{`<auth><password>hunter2</password></auth>`, `<auth><password><redacted></password></auth>`},
		{`<note>keep this</note>`, `<note>keep this</note>`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.in, got, tc.want)
		}
	}
}

// Multipart: the value of a part whose form name is a credential is masked, and the other parts are kept.
func TestRedactBodyMultipartParts(t *testing.T) {
	in := "--b\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nhunter2\r\n--b\r\nContent-Disposition: form-data; name=\"note\"\r\n\r\nhello\r\n--b--"
	want := "--b\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\n<redacted>\r\n--b\r\nContent-Disposition: form-data; name=\"note\"\r\n\r\nhello\r\n--b--"
	if got := RedactBody(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A part with its own header lines, such as Content-Type, has its value masked as well.
func TestRedactBodyMultipartPartHeaders(t *testing.T) {
	in := "--b\r\nContent-Disposition: form-data; name=\"password\"\r\nContent-Type: text/plain\r\n\r\nhunter2\r\n--b--"
	want := "--b\r\nContent-Disposition: form-data; name=\"password\"\r\nContent-Type: text/plain\r\n\r\n<redacted>\r\n--b--"
	if got := RedactBody(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Header lines in a text dump: the whole value of an authorization, proxy-authorization, cookie or set-cookie line is masked,
// so Basic and the rest of a cookie do not survive.
func TestRedactBodyHeaderLines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GET /x HTTP/1.1\r\nAuthorization: Basic dXNlcjpwYXNz\r\nAccept: text/plain", "GET /x HTTP/1.1\r\nAuthorization: <redacted>\r\nAccept: text/plain"},
		{"Cookie: sid=abc; theme=dark", "Cookie: <redacted>"},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The password in a URL's userinfo is always masked, in a URL and in text that carries one. A host with a port and no
// userinfo password is kept, and so is a scheme-relative URL without one.
func TestRedactUserinfo(t *testing.T) {
	if got, want := RedactURL("https://alice:hunter2@example.test/path"), "https://alice:<redacted>@example.test/path"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	if got, want := RedactBody("connect https://alice:hunter2@example.test/x failed"), "connect https://alice:<redacted>@example.test/x failed"; got != want {
		t.Errorf("RedactBody = %q, want %q", got, want)
	}
	if got, want := RedactURL("https://example.test:8443/path?sort=a"), "https://example.test:8443/path?sort=a"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	if got, want := RedactURL("//example.test/x?sort=a"), "//example.test/x?sort=a"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	// A password with no user is still a password.
	if got, want := RedactURL("redis://:hunter2@cache:6379/0"), "redis://:<redacted>@cache:6379/0"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	if got, want := RedactURL("//:hunter2@example.test/x"), "//:<redacted>@example.test/x"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
}

// A truncated JSON body is not valid JSON, so it is read as plain text, and a credential member still loses its value.
func TestRedactBodyTruncatedJSON(t *testing.T) {
	if got := RedactBody(`{"password":"abcd"`); strings.Contains(got, "abcd") {
		t.Errorf("a truncated body kept the value: %q", got)
	}
}

// A query name is redacted by the same token rule as a JSON member, and by the names in credentialQuery.
func TestRedactURLCredentialNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/path?code=abc&pin=1234&count=2", "/path?code=abc&pin=<redacted>&count=2"},
		{"/path?new_password=abc&shipping=safe", "/path?new_password=<redacted>&shipping=safe"},
		{
			"/path?current_password=a&secretKey=b&api_secret=c&private_key=d&jwt=e&session=f&passwordHash=g&passwd2=h&shipping=safe",
			"/path?current_password=<redacted>&secretKey=<redacted>&api_secret=<redacted>&private_key=<redacted>&jwt=<redacted>&session=<redacted>&passwordHash=<redacted>&passwd2=<redacted>&shipping=safe",
		},
	}
	for _, tc := range cases {
		if got := RedactURL(tc.in); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

// These are documented limits, pinned so that a change to them is deliberate: free text without an assignment, a lookalike
// name that uses a Cyrillic letter, and a name encoded twice are not redacted. The docs say the same.
func TestRedactBodyKnownLimits(t *testing.T) {
	lookalike := string(rune(0x0440)) + "assword=hunter2" // a Cyrillic er in place of the Latin p
	for _, in := range []string{"my password is hunter2", lookalike, "%2570assword=hunter2"} {
		if got := RedactBody(in); got != in {
			t.Errorf("RedactBody(%q) = %q; this documented limit must stay as it is, or the docs must change", in, got)
		}
	}
}

// A header is a credential by the token rule as well as by the fixed names, so a custom credential header is masked.
func TestRedactHeadersByTokenRule(t *testing.T) {
	credential := []string{
		"X-Api-Token", "Private-Token", "X-Auth-Key", "X-Client-Secret", "X-Session-Id", "X-Api-Secret",
		"X-Gitlab-Token", "Api-Token", "X-Authorization", "Token", "Password", "X-Amz-Signature",
	}
	in := map[string]string{"Accept": "text/plain", "X-Request-Id": "req-1", "Content-Type": "application/json"}
	for _, name := range credential {
		in[name] = "value-of-" + name
	}
	out := RedactHeaders(in)
	for _, name := range credential {
		if out[name] != Placeholder {
			t.Errorf("header %s = %q, want the placeholder", name, out[name])
		}
	}
	for _, name := range []string{"Accept", "X-Request-Id", "Content-Type"} {
		if out[name] != in[name] {
			t.Errorf("header %s = %q, want it kept", name, out[name])
		}
	}
}

// Presigned URL signatures and credentials are query values. The token rule masks them, and the other parameters stay.
func TestRedactURLPresignedSignatures(t *testing.T) {
	raw := "https://bucket.example.test/o?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAEXAMPLE%2F20261026%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20261026T000000Z&X-Amz-Signature=abc123&X-Amz-Security-Token=tok456&X-Goog-Signature=def789&Signature=ghi012"
	got := RedactURL(raw)
	for _, secret := range []string{"AKIAEXAMPLE", "abc123", "tok456", "def789", "ghi012"} {
		if strings.Contains(got, secret) {
			t.Errorf("RedactURL kept %q: %q", secret, got)
		}
	}
	for _, kept := range []string{"X-Amz-Algorithm=AWS4-HMAC-SHA256", "X-Amz-Date=20261026T000000Z"} {
		if !strings.Contains(got, kept) {
			t.Errorf("RedactURL dropped %q: %q", kept, got)
		}
	}
}

// A multipart part whose form name is not quoted is masked the same way as one whose name is quoted.
func TestRedactBodyMultipartUnquotedName(t *testing.T) {
	in := "--b\r\nContent-Disposition: form-data; name=password\r\n\r\nhunter2\r\n--b\r\nContent-Disposition: form-data; name=note\r\n\r\nhello\r\n--b--"
	want := "--b\r\nContent-Disposition: form-data; name=password\r\n\r\n<redacted>\r\n--b\r\nContent-Disposition: form-data; name=note\r\n\r\nhello\r\n--b--"
	if got := RedactBody(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A name/value pair object masks its value when a name, key, field, id or label member names a credential, at any depth.
func TestRedactBodyNameValuePairs(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"field":"password","value":"hunter2"}`, `{"field":"password","value":"<redacted>"}`},
		{`{"items":[{"name":"password","value":"hunter2"}]}`, `{"items":[{"name":"password","value":"<redacted>"}]}`},
		{`{"label":"PIN","value":1234}`, `{"label":"PIN","value":"<redacted>"}`},
		{`{"name":"username","value":"alice"}`, `{"name":"username","value":"alice"}`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("RedactBody(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Card numbers written as cardNo, card_no, creditCard and cardno are credentials, and a cardholder name is not.
func TestRedactBodyCardNameSpellings(t *testing.T) {
	in := `{"cardNo":"4111111111111111","card_no":"4111111111111112","creditCard":"4111111111111113","cardno":"4111111111111114","cardholder":"Alice Example"}`
	got := RedactBody(in)
	if strings.Contains(got, "41111111111111") {
		t.Errorf("RedactBody kept a card number: %q", got)
	}
	if !strings.Contains(got, "Alice Example") {
		t.Errorf("RedactBody dropped a name that is not a card number: %q", got)
	}
}

// A form pair whose value is in a separate field, and a JSON pair whose name member is not a listed name, are documented
// limits, so they are pinned as limits.
func TestRedactBodyPairLimits(t *testing.T) {
	if got := RedactBody("name=password&value=hunter2"); !strings.Contains(got, "hunter2") {
		t.Errorf("RedactBody = %q, want the separate value kept as a documented limit", got)
	}
	if got := RedactBody(`{"item":"password","value":"hunter2"}`); !strings.Contains(got, "hunter2") {
		t.Errorf("RedactBody = %q, want the value kept as a documented limit", got)
	}
}

// The pair-name members are name, key, field, id and label, and a name member may be an array of strings. The value member
// is value, val, data, content, text or newValue, in any case, so each of these pairs is read as a credential pair.
func TestRedactBodyPairMembers(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"key":"password","data":"X"}`, `{"key":"password","data":"<redacted>"}`},
		{`{"name":"password","val":"X"}`, `{"name":"password","val":"<redacted>"}`},
		{`{"name":["password"],"value":"X"}`, `{"name":["password"],"value":"<redacted>"}`},
		{`{"name":["username","password"],"value":"X"}`, `{"name":["username","password"],"value":"<redacted>"}`},
		{`{"key":"password","value":"hunter2"}`, `{"key":"password","value":"<redacted>"}`},
		{`{"id":"password","value":"hunter2"}`, `{"id":"password","value":"<redacted>"}`},
		{`{"name":"password","content":"hunter2"}`, `{"name":"password","content":"<redacted>"}`},
		{`{"name":"password","text":"hunter2"}`, `{"name":"password","text":"<redacted>"}`},
		{`{"name":"password","newValue":"hunter2"}`, `{"name":"password","newValue":"<redacted>"}`},
		{`{"name":"password","Value":"hunter2"}`, `{"name":"password","Value":"<redacted>"}`},
		{`{"name":["username"],"value":"alice"}`, `{"name":["username"],"value":"alice"}`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("RedactBody(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A value member outside the listed value names is a documented limit, so it is pinned as one.
func TestRedactBodyPairValueLimits(t *testing.T) {
	if got := RedactBody(`{"name":"password","amount":"hunter2"}`); !strings.Contains(got, "hunter2") {
		t.Errorf("RedactBody = %q, want the amount kept as a documented limit", got)
	}
}

// A multipart name is percent-decoded before the rule reads it, as a form or query name is, quoted or not.
func TestRedactBodyMultipartEncodedName(t *testing.T) {
	in := "--b\r\nContent-Disposition: form-data; name=\"user%5Bpassword%5D\"\r\n\r\nhunter2\r\n--b\r\nContent-Disposition: form-data; name=user%5Bpassword%5D\r\n\r\nhunter2\r\n--b--"
	want := "--b\r\nContent-Disposition: form-data; name=\"user%5Bpassword%5D\"\r\n\r\n<redacted>\r\n--b\r\nContent-Disposition: form-data; name=user%5Bpassword%5D\r\n\r\n<redacted>\r\n--b--"
	if got := RedactBody(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A name that holds a credential word is masked whatever else it says, counters included. This is a stated trade-off with
// no carve-out, so the counters are pinned as masked.
func TestRedactBodyCounterNamesAreMasked(t *testing.T) {
	in := `{"Token-Count":3,"Session-Count":2,"passcodeLength":6}`
	want := `{"Token-Count":"<redacted>","Session-Count":"<redacted>","passcodeLength":"<redacted>"}`
	if got := RedactBody(in); got != want {
		t.Errorf("RedactBody(%q) = %q, want %q", in, got, want)
	}
}

// These limits are reached when a rule reads a value in part, or not at all. Each one is pinned, so a fix changes this test
// on purpose, and the manifest's not_redacted list changes with it.
func TestRedactBodyPartialLimits(t *testing.T) {
	if got, want := RedactURL("https://alice:s3cr@t@h/x"), "https://alice:<redacted>@t@h/x"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	if got, want := RedactURL("https://alice:pa/ss@h/x"), "https://alice:pa/ss@h/x"; got != want {
		t.Errorf("RedactURL = %q, want %q", got, want)
	}
	if got, want := RedactBody("password=correct horse"), "<redacted> horse"; got != want {
		t.Errorf("RedactBody = %q, want %q", got, want)
	}
	kept := []struct{ name, got, keep string }{
		{"a query value with its own ?", RedactURL("https://h/x?next=/cb?token=X"), "token=X"},
		{"a JSON member name", RedactBody(`{"password=hunter2":1}`), "hunter2"},
		{"XML CDATA", RedactBody("<password><![CDATA[hunter2]]></password>"), "hunter2"},
		{"a credential element with child elements", RedactBody("<password><value>hunter2</value></password>"), "hunter2"},
		{"an authorization header that does not start its line", RedactBody("log: Authorization: Basic dXNlcjpwYXNz"), "dXNlcjpwYXNz"},
		{"a name outside the token lists", RedactBody("newpassword=hunter2"), "hunter2"},
		{"a short value in a JSON body that does not parse", RedactBody(`{"cvv":"123"`), "123"},
		{"a JSON string with escaped slashes", RedactBody(`{"url":"https:\/\/alice:hunter2@h\/x"}`), "hunter2"},
	}
	for _, c := range kept {
		if !strings.Contains(c.got, c.keep) {
			t.Errorf("%s: %q is not in %q", c.name, c.keep, c.got)
		}
	}
}
