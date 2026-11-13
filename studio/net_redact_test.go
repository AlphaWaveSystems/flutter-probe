package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"
)

// Secret-shaped values, built from pieces so no credential literal is in the source.
const (
	netToken  = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	netAnt    = "sk" + "-ant-" + "api03-abcdefghijklmnopqrstuv"
	netPlain  = "plainNetSecretValue0000000090" // a secret of no known family: only its position says it is one
	netEmail  = "jane.doe@example.com"
	netHome   = "/Users/alice/project"
	netPEMKey = "-----BEGIN PRIVATE " + "KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE " + "KEY-----"
)

// jsonEscape is r as a JSON \u escape, as a surrogate pair above the first plane.
func jsonEscape(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf(`\u%04X`, r)
	}
	r -= 0x10000
	return fmt.Sprintf(`\u%04X\u%04X`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
}

func mustNotContain(t *testing.T, what, got string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if b != "" && strings.Contains(got, b) {
			t.Errorf("%s: %q is in %q", what, b, got)
		}
	}
}

// Header names in a secret family, in every spelling, show the name and never the value. Names outside the families keep theirs.
func TestSecretHeaderNamesAreRedactedInEverySpelling(t *testing.T) {
	secret := []string{
		"Authorization", "authorization", "AUTHORIZATION", "Proxy-Authorization", "Cookie", "COOKIE", "Set-Cookie", "set_cookie",
		"X-Api-Key", "X_Api_Key", "x-api-key", "APIKEY", "ApiKey", "api_key", "Api-Key", "X-Auth-Token", "x_auth_token", "X-Auth",
		"X-CSRF-Token", "X-XSRF-TOKEN", "csrf_token", "X-Amz-Security-Token", "Access-Token", "id_token", "refresh-token",
		"X-Session-Id", "Session", "client_secret", "Client-Secret", "Password", "passwd", "X-Password", "Private-Key", "X-Access-Key",
		"X-Signature", "Signature", "OAuth", "x-oauth-token", "X-Credentials", "Bearer", "X-JWT", "Key", "PIN", "OTP", "CVV",
		"user[password]", "x api key", "X.Api.Key",
	}
	for _, n := range secret {
		hs, _ := redactHeaders(map[string]string{n: netPlain})
		if len(hs) != 1 || hs[0].Value != redactedMark {
			t.Errorf("%q: value %q, want %q", n, hs[0].Value, redactedMark)
		}
	}
	plain := []string{"Content-Type", "Accept", "X-Request-Id", "User-Agent", "Content-Length", "Author", "Authority", "Authors", "Shipping", "Spinner", "Cache-Control", "ETag"}
	for _, n := range plain {
		hs, _ := redactHeaders(map[string]string{n: "visible-value"})
		if hs[0].Value != "visible-value" {
			t.Errorf("%q was redacted: %q", n, hs[0].Value)
		}
	}
}

// A secret in a header whose name is not in a family is still caught by shape, and a redirect target loses its query values.
func TestHeaderValuesAreRedactedByShapeAndURLHeadersLoseTheirQuery(t *testing.T) {
	hs, _ := redactHeaders(map[string]string{
		"X-Forward":  "Bearer " + netPlain,
		"X-Note":     "see " + netToken + " and " + netAnt + " for " + netEmail + " in " + netHome,
		"Location":   "https://api.example.test/cb?code=" + netPlain + "&state=abc#frag",
		"Referer":    "https://user:" + netPlain + "@app.example.test/p?sig=" + netPlain,
		"Set-Cookie": "sid=" + netPlain + "; HttpOnly",
	})
	all := fmt.Sprint(hs)
	mustNotContain(t, "headers", all, netPlain, "ghp_", "sk-ant", "jane", "example.com", "alice", "frag")
	if !strings.Contains(all, "code=[redacted]") || !strings.Contains(all, "state=[redacted]") {
		t.Errorf("a redirect target keeps its query names and loses the values: %v", hs)
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.test/v1/users?id=7&token=" + netPlain, "https://api.example.test/v1/users?id=[redacted]&token=[redacted]"},
		{"https://user:" + netPlain + "@api.example.test/x", "https://api.example.test/x"},
		{"https://api.example.test/x#access_token=" + netPlain, "https://api.example.test/x"},
		{"https://api.example.test/x?flag&a=1", "https://api.example.test/x?flag&a=[redacted]"},
		{"https://api.example.test/u/" + netEmail + "/f", "https://api.example.test/u/[email]/f"},
		{"file://" + netHome + "/a.json", "file://~/project/a.json"},
		{"https://api.example.test/p?q=%61%62&%74oken=" + netPlain, "https://api.example.test/p?q=[redacted]&token=[redacted]"},
		{"not a url ?k=" + netPlain, "not a url ?k=[redacted]"},
	}
	for _, c := range cases {
		if got := redactURL(c.in); got != c.want {
			t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := redactURL(strings.Repeat("a", 5000)); len([]rune(got)) > netMaxURL {
		t.Errorf("a long URL is %d characters", len([]rune(got)))
	}
}

// JSON: a secret key's value is replaced at any depth and in arrays, the other strings are cleaned, numbers and structure stay.
func TestRedactBodyJSONByKeyAtAnyDepth(t *testing.T) {
	body := `{"user":{"name":"jane","password":"` + netPlain + `","profile":{"apiKey":"` + netPlain + `","list":[{"token":"` + netPlain + `"},{"ok":1}]}},` +
		`"refresh_token":"` + netPlain + `","note":"contact ` + netEmail + ` at ` + netHome + ` with ` + netToken + `","count":3,"nested":[[{"Authorization":"Bearer x"}]],"secret":{"a":1}}`
	b := redactBody(body, 0, "application/json")
	if b.Kind != "text" {
		t.Fatalf("kind %q", b.Kind)
	}
	mustNotContain(t, "json", b.Text, netPlain, "ghp_", "jane.doe", "example.com", "alice", `"a": 1`)
	for _, want := range []string{`"count": 3`, `"name": "jane"`, `"ok": 1`, `"password": "[redacted]"`, `"apiKey": "[redacted]"`, `"token": "[redacted]"`, `"secret": "[redacted]"`, "[email]"} {
		if !strings.Contains(b.Text, want) {
			t.Errorf("%q is not in %s", want, b.Text)
		}
	}
}

func TestRedactBodyFormAndText(t *testing.T) {
	form := redactBody("user=jane&password="+netPlain+"&api_key="+netPlain+"&next=%2Fhome%2Falice", 0, "application/x-www-form-urlencoded")
	mustNotContain(t, "form", form.Text, netPlain)
	if !strings.Contains(form.Text, "user=jane") || !strings.Contains(form.Text, "password=[redacted]") || !strings.Contains(form.Text, "api_key=[redacted]") {
		t.Errorf("form = %q", form.Text)
	}
	guess := redactBody("a=1&token="+netPlain, 0, "") // no content type: it looks like a form
	mustNotContain(t, "a form with no type", guess.Text, netPlain)

	text := redactBody("password: "+netPlain+"\nAuthorization: Bearer "+netPlain+"\n"+netPEMKey+"\nmail "+netEmail+" "+netToken+" "+netAnt, 0, "text/plain")
	mustNotContain(t, "text", text.Text, netPlain, "MIIEvQ", "ghp_", "sk-ant", "jane.doe")
	if strings.Count(text.Text, "\n") < 2 {
		t.Errorf("line breaks were lost: %q", text.Text)
	}
}

// A body that is not JSON but is declared JSON, or is JSON followed by more, is still redacted by shape.
func TestRedactBodyBrokenJSONIsStillRedacted(t *testing.T) {
	for _, body := range []string{`{"password": "` + netPlain + `", "x":`, `{"a":1} trailing ` + netPlain + ` password=` + netPlain, `[1,2,` + netToken} {
		b := redactBody(body, 0, "application/json")
		mustNotContain(t, "broken JSON "+body[:12], b.Text, netToken)
		if strings.Contains(body, "password=") && strings.Contains(b.Text, netPlain) {
			t.Errorf("assignment survived: %q", b.Text)
		}
	}
}

func TestRedactBodyKindsAndCaps(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		size   int64
		ct     string
		kind   string
		hasTxt bool
	}{
		{"empty", "", 0, "", "empty", false},
		{"a PNG by type", "\x89PNG\r\n", 0, "image/png", "binary", false},
		{"a NUL byte in text", "ab\x00cd", 0, "application/json", "binary", false},
		{"invalid UTF-8", "\xff\xfe\xfd\xfc", 0, "text/plain", "binary", false},
		{"octet-stream", "plain looking", 0, "application/octet-stream", "binary", false},
		{"over 1 MB by size", "x", netBodyMax + 1, "text/plain", "large", false},
		{"over 1 MB by content", strings.Repeat("a", netBodyMax+1), 0, "text/plain", "large", false},
		{"plain text", "hello", 0, "text/plain", "text", true},
	}
	for _, c := range cases {
		b := redactBody(c.body, c.size, c.ct)
		if b.Kind != c.kind || (b.Text != "") != c.hasTxt {
			t.Errorf("%s: kind %q text %q", c.name, b.Kind, b.Text)
		}
		if c.kind == "binary" || c.kind == "large" {
			if b.Size == 0 {
				t.Errorf("%s: no size", c.name)
			}
		}
	}
	long := redactBody(strings.Repeat("é", 10000), 0, "text/plain")
	if !long.Truncated || len([]rune(long.Text)) != netBodyShown || long.Size != 20000 {
		t.Errorf("long: truncated %v, %d characters, size %d", long.Truncated, len([]rune(long.Text)), long.Size)
	}
	// A character cut at the end of the sniffed sample is not binary.
	edge := strings.Repeat("a", netSniff-1) + "é"
	if redactBody(edge, 0, "text/plain").Kind != "text" {
		t.Error("a multi-byte character at the edge of the sample made text binary")
	}
}

// Hostile text stays text: markup is not touched, and a very deep or wide JSON is bounded.
func TestRedactBodyHostile(t *testing.T) {
	b := redactBody(`<img src=x onerror="alert(1)"><script>x</script>`, 0, "text/html")
	if b.Text != `<img src=x onerror="alert(1)"><script>x</script>` {
		t.Errorf("markup was changed: %q", b.Text)
	}
	deep := strings.Repeat("[", 5000) + strings.Repeat("]", 5000)
	start := time.Now()
	d := redactBody(deep, 0, "application/json")
	if d.Kind != "text" || time.Since(start) > 2*time.Second {
		t.Errorf("deep JSON: kind %q, took %v", d.Kind, time.Since(start))
	}
	// The walk stops at a fixed depth, so the output is bounded however deep the body is.
	var v any
	if err := json.Unmarshal([]byte(deep), &v); err != nil {
		t.Fatal(err)
	}
	if out, _ := json.Marshal(redactJSON(v, 0)); !strings.Contains(string(out), "[too deep]") || len(out) > 4*netMaxDepth {
		t.Errorf("deep value: %d bytes, bounded %v", len(out), strings.Contains(string(out), "[too deep]"))
	}
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&sb, `"k%d":"v",`, i)
	}
	sb.WriteString(`"end":1}`)
	if w := redactBody(sb.String(), 0, "application/json"); len([]rune(w.Text)) > netBodyShown {
		t.Errorf("wide: %d characters", len([]rune(w.Text)))
	}
}

func TestRedactHeadersCapsAndSorts(t *testing.T) {
	h := map[string]string{}
	for i := 0; i < 10000; i++ {
		h[fmt.Sprintf("X-H-%05d", i)] = "v"
	}
	hs, more := redactHeaders(h)
	if len(hs) != netMaxHeaders || more != 10000-netMaxHeaders || hs[0].Name != "X-H-00000" {
		t.Errorf("%d headers, %d more, first %q", len(hs), more, hs[0].Name)
	}
	long, _ := redactHeaders(map[string]string{strings.Repeat("N", 500): strings.Repeat("v", 5000)})
	if len([]rune(long[0].Name)) > netMaxHeaderName || len([]rune(long[0].Value)) > netMaxHeaderValue {
		t.Errorf("name %d, value %d", len([]rune(long[0].Name)), len([]rune(long[0].Value)))
	}
}

// What each position must catch when a hidden character is in the middle of what it looks for. The hidden character is dropped
// first, so a name, a key, a token, an email address or a home folder that was split is whole again before its step reads it.
func netAttacks(r rune) []struct {
	name string
	run  func() string
	bad  []string
	good string
} {
	hdrName := split("Authorization", r)
	return []struct {
		name string
		run  func() string
		bad  []string
		good string
	}{
		{"a secret header name", func() string { hs, _ := redactHeaders(map[string]string{hdrName: netPlain}); return fmt.Sprint(hs) }, []string{netPlain}, redactedMark},
		{"an api key header name", func() string {
			hs, _ := redactHeaders(map[string]string{split("X-Api-Key", r): netPlain})
			return fmt.Sprint(hs)
		}, []string{netPlain}, redactedMark},
		{"a token in a header value", func() string {
			hs, _ := redactHeaders(map[string]string{"X-A": split(netToken, r)})
			return fmt.Sprint(hs)
		}, []string{"ghp_", netToken[4:20]}, redactedMark},
		{"a query value", func() string { return redactURL("https://h.example.test/p?" + split("token", r) + "=" + netPlain) }, []string{netPlain}, "=[redacted]"},
		{"a query name", func() string { return redactURL("https://h.example.test/p?a=1&" + split(netToken, r)) }, []string{"ghp_", netToken[4:20]}, redactedMark},
		{"user info in a URL", func() string { return redactURL("https://" + split("user", r) + ":" + netPlain + "@h.example.test/p") }, []string{netPlain, "user"}, "https://h.example.test/p"},
		{"a URL path email", func() string { return redactURL("https://h.example.test/" + split(netEmail, r)) }, []string{"jane", "example.com"}, "[email]"},
		{"a URL path home folder", func() string { return redactURL("https://h.example.test" + split(netHome, r)) }, []string{"alice", "Users"}, "~"},
		{"a JSON key", func() string {
			return redactBody(`{"`+split("password", r)+`":"`+netPlain+`"}`, 0, "application/json").Text
		}, []string{netPlain}, `"[redacted]"`},
		{"a JSON key as an escape", func() string {
			return redactBody(`{"pass`+jsonEscape(r)+`word":"`+netPlain+`"}`, 0, "application/json").Text
		}, []string{netPlain}, "[redacted]"},
		{"a token in a JSON value", func() string { return redactBody(`{"a":"`+split(netToken, r)+`"}`, 0, "application/json").Text }, []string{"ghp_", netToken[4:20]}, redactedMark},
		{"an email in a JSON value", func() string { return redactBody(`{"a":"`+split(netEmail, r)+`"}`, 0, "application/json").Text }, []string{"jane", "example.com"}, "[email]"},
		{"a form name", func() string {
			return redactBody(split("password", r)+"="+netPlain+"&a=1", 0, "application/x-www-form-urlencoded").Text
		}, []string{netPlain}, "[redacted]"},
		{"a token in a text body", func() string { return redactBody("note "+split(netAnt, r)+" end", 0, "text/plain").Text }, []string{"sk-ant", netAnt[8:20]}, redactedMark},
		{"an assignment in a text body", func() string { return redactBody(split("password", r)+": "+netPlain, 0, "text/plain").Text }, []string{netPlain}, redactedMark},
		{"an error text", func() string { return shortText("dial failed for "+split(netToken, r), netMaxError) }, []string{"ghp_", netToken[4:20]}, redactedMark},
	}
}

// Every hidden code point, by its ranges, every format character and the named ones, in the middle of each position.
func TestNetRedactorDropsEveryHiddenClassBeforeEveryStep(t *testing.T) {
	probes := append(everyCf(), namedHidden...)
	for _, rg := range droppedRanges() {
		probes = append(probes, rg[0], rg[1], rg[0]+(rg[1]-rg[0])/2)
	}
	probes = append(probes, 0x180B, 0x180C, 0x180D, 0x180F, 0x2065, 0xE000, 0xF0000, 0xFFFE, 0xFFFF, 0xFFF0, 0xFFF8, 0xFDD0, 0x10FFFF)
	failures := 0
	for _, r := range probes {
		if r == '\n' || r == '\t' || r == '\r' || r == '"' || r == 0 || unicode.Is(unicode.Zs, r) {
			continue // a NUL makes a body binary, which shows no content at all: TestRedactBodyKindsAndCaps
		}
		for _, c := range netAttacks(r) {
			got := c.run()
			for _, b := range c.bad {
				if strings.Contains(got, b) {
					t.Errorf("U+%04X in %s: %q leaves in %q", r, c.name, b, got)
					failures++
				}
			}
			if !strings.Contains(got, c.good) {
				t.Errorf("U+%04X in %s: the step did not run, got %q", r, c.name, got)
				failures++
			}
			if failures > 20 {
				t.Fatal("too many failures")
			}
		}
	}
	t.Logf("%d probes x %d positions", len(probes), len(netAttacks('x')))
}

// Every code point, in batches: what the redactor shows contains no character that hiddenRune says is hidden. (The rule itself is
// swept in TestHiddenRuleOverEveryCodePoint; this is the redactor using it on every string it shows.)
func TestNetRedactorShowsNoHiddenCharacterForAnyCodePoint(t *testing.T) {
	var batch []rune
	check := func() {
		if len(batch) == 0 {
			return
		}
		s := "a" + string(batch) + "b"
		for name, got := range map[string]string{
			"text":   cleanText(s, false),
			"header": func() string { hs, _ := redactHeaders(map[string]string{"X-A": s}); return hs[0].Value + hs[0].Name }(),
			"url":    redactURL("https://h.example.test/" + s + "?a=" + s),
			"body":   redactBody(s, 0, "text/plain").Text,
		} {
			for _, r := range got {
				if hiddenRune(r) && r != '\n' {
					t.Fatalf("%s: U+%04X is shown", name, r)
				}
			}
		}
		batch = batch[:0]
	}
	count := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF || r == 0 || !hiddenRune(r) {
			continue
		}
		batch = append(batch, r)
		count++
		if len(batch) == 5000 {
			check()
		}
	}
	check()
	if count < 900000 {
		t.Fatalf("only %d hidden code points were checked", count)
	}
}
