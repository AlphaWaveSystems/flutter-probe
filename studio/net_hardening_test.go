package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The second review's classes: text that nests a credential under a parent line, names that end in key in bodies, header values under a
// neutral name, formats the first rules did not read, the cost of large inputs, and members that merge.

func TestNestedConfigTextHidesTheCredential(t *testing.T) {
	names := []string{"password", "secret", "token", "api_key", "pass", "sid", "otp", "pin", "passphrase", "session", "auth", "credential", "jwt", "csrf", "sig", "signature", "bearer"}
	for _, name := range names {
		for _, quote := range []string{"", "'", `"`} {
			yaml := "db:\n  host: example.test\n  " + name + ": " + quote + netPlain + quote + "\n"
			for where, got := range map[string]string{
				"a text body":    redactBody(yaml, 0, "text/yaml").Text,
				"a JSON string":  redactBody(`{"config":`+jsonString(yaml)+`}`, 0, "application/json").Text,
				"an error text":  redactErrorText(yaml, 1000),
				"a header value": func() string { hs, _ := redactHeaders(map[string]string{"X-Config": yaml}); return hs[0].Value }(),
				"a form value":   redactBody("cfg="+strings.NewReplacer("\n", "%0A", " ", "+", ":", "%3A").Replace(yaml), 0, "application/x-www-form-urlencoded").Text,
			} {
				if strings.Contains(got, netPlain) {
					t.Errorf("%s in %s: the value is shown: %q", name, where, got)
				}
			}
		}
	}
	// A name only this file knows, under a parent line: this file's own assignment rule must not cross the line break either.
	for _, name := range []string{"licenseKey", "xsrf", "subscription_key", "client_key"} {
		for where, got := range map[string]string{
			"a text body":   redactBody("db:\n  "+name+": '"+netPlain+"'\n", 0, "text/plain").Text,
			"an error text": redactErrorText("db:\n  "+name+": "+netPlain+"\n", 500),
			"a header value": func() string {
				hs, _ := redactHeaders(map[string]string{"X-Cfg": "db:\n  " + name + ": " + netPlain})
				return hs[0].Value
			}(),
		} {
			if strings.Contains(got, netPlain) {
				t.Errorf("%s in %s: %q", name, where, got)
			}
		}
	}
	// The block form, and a parent that is not a credential, keep the other lines.
	got := redactBody("password:\n  "+netPlain+"\nuser: bob\n", 0, "text/yaml").Text
	if strings.Contains(got, netPlain) || !strings.Contains(got, "user: bob") {
		t.Errorf("block form: %q", got)
	}
}

// A name that ends in the word key is a secret wherever it is, not only in a header.
func TestNamesEndingInKeyAreSecretsInBodies(t *testing.T) {
	for _, name := range []string{"licenseKey", "clientKey", "serviceKey", "sendgrid-key", "client_key", "masterKey", "fastlyKey", "X-Functions-Key", "Ocp-Apim-Subscription-Key", "apiKey2Key"} {
		json := redactBody(`{"`+name+`":"`+netPlain+`","ok":1}`, 0, "application/json").Text
		form := redactBody(name+"="+netPlain+"&a=1", 0, "application/x-www-form-urlencoded").Text
		line := redactBody("note\n"+name+": "+netPlain+"\nmore", 0, "text/plain").Text
		xml := redactBody("<c><"+name+">"+netPlain+"</"+name+"></c>", 0, "text/xml").Text
		for where, got := range map[string]string{"JSON": json, "form": form, "a text line": line, "XML": xml} {
			if strings.Contains(got, netPlain) {
				t.Errorf("%s in %s: %q", name, where, got)
			}
		}
		if !strings.Contains(json, `"ok": 1`) || !strings.Contains(form, "a=1") {
			t.Errorf("%s: the rest was changed: %q %q", name, json, form)
		}
	}
	for _, keep := range []string{"monkey", "turkey", "hockey", "keyboard", "keystone"} {
		if got := redactBody(`{"`+keep+`":"visible"}`, 0, "application/json").Text; !strings.Contains(got, "visible") {
			t.Errorf("%s was redacted: %q", keep, got)
		}
	}
	// The word rule itself.
	for name, want := range map[string]bool{"licenseKey": true, "client_key": true, "key": true, "keys": true, "monkey": false, "keyboard": false, "primary-key": true} {
		if got := endsInKey(name); got != want {
			t.Errorf("endsInKey(%q) = %v", name, got)
		}
	}
}

// A header whose name is neutral can still carry an assignment in its value.
func TestHeaderValuesHideAssignmentsAndDigestFields(t *testing.T) {
	cases := map[string]string{
		"X-Note":        "sid=" + netPlain + "; path=/",
		"X-Other":       "session=" + netPlain,
		"X-Digest":      `Digest username="bob", realm="r", nonce="n0nce1234", uri="/p", response="` + netPlain + `", cnonce="` + netPlain + `x"`,
		"X-Aws":         "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20240101/us/s3/aws4_request, SignedHeaders=host, Signature=" + netPlain,
		"X-Yaml":        "svc:\n  password: " + netPlain,
		"X-Long-Value":  strings.Repeat("a ", 100) + "token=" + netPlain,
		"X-Camel-Value": "licenseKey=" + netPlain,
	}
	for name, value := range cases {
		hs, _ := redactHeaders(map[string]string{name: value})
		if strings.Contains(hs[0].Value, netPlain) {
			t.Errorf("%s: %q", name, hs[0].Value)
		}
	}
	hs, _ := redactHeaders(map[string]string{"Cache-Control": "max-age=300, public", "Content-Type": "text/html; charset=utf-8"})
	if hs[0].Value != "max-age=300, public" || hs[1].Value != "text/html; charset=utf-8" {
		t.Errorf("ordinary values were changed: %+v", hs)
	}
}

func TestMoreFormats(t *testing.T) {
	cases := []struct {
		name, ct, body string
		bad            []string
		show           []string
	}{
		{"CDATA in a password element", "text/xml", `<login><user>bob</user><password><![CDATA[` + netPlain + `]]></password></login>`, []string{netPlain}, []string{"bob"}},
		{"CDATA in a licenseKey element", "text/xml", `<c><licenseKey><![CDATA[` + netPlain + `]]></licenseKey></c>`, []string{netPlain}, nil},
		{"an HTML password input", "text/html", `<form><input type="text" name="user" value="bob"><input type="password" name="login_field" value="` + netPlain + `"></form>`, []string{netPlain}, []string{"bob"}},
		{"an HTML hidden token input", "text/html", `<input type=hidden name=authenticity_token value='` + netPlain + `'><input name="csrf" value="` + netPlain + `">`, []string{netPlain}, nil},
		{"an HTML meta csrf token", "text/html", `<head><meta name="csrf-token" content="` + netPlain + `"><meta name="viewport" content="width=device-width"></head>`, []string{netPlain}, []string{"device-width"}},
		{"a form field with URL-encoded JSON", "application/x-www-form-urlencoded", "payload=%7B%22password%22%3A%22" + netPlain + "%22%2C%22ok%22%3A1%7D&a=1", []string{netPlain}, []string{"a=1", `"ok"`}},
		{"a form field with an encoded form", "application/x-www-form-urlencoded", "inner=" + "token%3D" + netPlain + "%26x%3D1" + "&a=1", []string{netPlain}, []string{"a=1"}},
		{"a JSON comment between the colon and the value", "application/json", `{"password": /* c */ "` + netPlain + `", "ok": 1}`, []string{netPlain}, []string{`"ok"`}},
		{"a line comment before the value", "application/json", "{\"password\": // c\n \"" + netPlain + "\"}", []string{netPlain}, nil},
		{"comments in a cut JSON body", "application/json", `{"a":1, /* c */ "password": /* c */ "` + netPlain, []string{netPlain}, nil},
		{"a URL with // inside a string is not a comment", "application/json", `{"url":"https://x.example.test/a","ok":2}`, nil, []string{"https://x.example.test/a", `"ok"`}},
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
}

// Two member names that read the same once they are cleaned stay two members.
func TestJSONMembersDoNotMerge(t *testing.T) {
	k1 := "user_profile_notification_settings_v2" // 24 or more characters with a digit: read as a token and cleaned
	k2 := "user_profile_notification_settings_v3"
	b := redactBody(`{"`+k1+`":"one","`+k2+`":"two","plain":"three"}`, 0, "application/json").Text
	for _, v := range []string{"one", "two", "three"} {
		if !strings.Contains(b, v) {
			t.Errorf("the value %q was lost: %s", v, b)
		}
	}
}

// A cut at the raw limit does not move a token into the shown part when a block shrinks what comes before it.
func TestBodyCutDoesNotLeakAToken(t *testing.T) {
	for k := 6; k <= 20; k += 2 { // the body is then a few characters over the limit, so it is cut inside the token
		body := pemOf(netRawBody-k) + " password=" + netPlain
		got := redactBody(body, 0, "text/plain")
		mustNotContain(t, fmt.Sprintf("a cut %d characters into the token", k), got.Text, "hunter", "password")
		if !got.Truncated {
			t.Fatal("a body over the raw limit is not marked cut")
		}
	}
}

// What a large input costs does not depend on its size: the raw text is bounded before any pattern reads it, and one exchange has a
// budget for all its fields.
func TestLargeInputsAreCheap(t *testing.T) {
	big := strings.Repeat("ab=cd; ", 150000) // about a megabyte
	huge := map[string]string{}
	for i := 0; i < 100; i++ {
		huge[fmt.Sprintf("X-Big-%03d", i)] = big
		huge[fmt.Sprintf("Location-%03d", i)] = "https://h.example.test/" + big
	}
	huge["Location"] = "https://h.example.test/?" + big
	e := probelink.HTTPEntry{
		Seq: 1, Method: "GET", URL: "https://h.example.test/p?" + big, Status: 500, Error: "failed " + big,
		RequestHeaders: huge, ResponseHeaders: huge, RequestBody: big[:900000], ResponseBody: big[:900000], RequestBytes: 900000, ResponseBytes: 900000, // under 1 MB, so they are read as text
	}
	a := netApp(&fakeNetClient{res: probelink.HTTPLogResult{Epoch: "e1", Entries: []probelink.HTTPEntry{e}}})
	start := time.Now()
	d, err := a.HTTPExchange("e1", 1)
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("one exchange with 200 headers of 1 MB, two 1 MB bodies, a 1 MB URL and a 1 MB error: %v", took)
	if took > 2*time.Second && !raceEnabled || took > 10*time.Second {
		t.Errorf("took %v", took)
	}
	if len(d.RequestHeaders) != netMaxHeaders || len(d.URL) > 4*netMaxURL {
		t.Errorf("%d headers, URL %d bytes", len(d.RequestHeaders), len(d.URL))
	}
	notShown := 0
	for _, h := range d.RequestHeaders {
		if h.Value == netNotShown {
			notShown++
		}
		if h.Name == "" {
			t.Error("a header lost its name")
		}
	}
	if notShown == 0 {
		t.Error("the exchange budget never ran out")
	}
	// The headers used the budget up, so the bodies that come after them are not read.
	if d.RequestBody.Kind != "limit" || d.ResponseBody.Kind != "limit" || d.RequestBody.Text != "" {
		t.Errorf("bodies after the budget: %q and %q", d.RequestBody.Kind, d.ResponseBody.Kind)
	}

	// The listing: many rows with a very large URL and error each.
	var entries []probelink.HTTPEntry
	for i := 1; i <= 60; i++ {
		entries = append(entries, probelink.HTTPEntry{Seq: i, Method: "GET", URL: "https://h.example.test/p?" + big, Error: big})
	}
	a = netApp(&fakeNetClient{res: probelink.HTTPLogResult{Epoch: "e1", Entries: entries}})
	start = time.Now()
	if _, err := a.HTTPExchanges(NetFilter{}); err != nil {
		t.Fatal(err)
	}
	t.Logf("a listing of 60 rows with a 1 MB URL and a 1 MB error each: %v", time.Since(start))
	if time.Since(start) > 2*time.Second && !raceEnabled || time.Since(start) > 10*time.Second {
		t.Errorf("the listing took %v", time.Since(start))
	}
}

// The budget and the raw limits are pinned as numbers.
func TestNetLimitsArePinned(t *testing.T) {
	for name, p := range map[string][2]int{
		"URL": {netRawURL, 4096}, "error": {netRawError, 4096}, "header name": {netRawHeaderName, 256}, "header value": {netRawHeaderValue, 8192},
		"body": {netRawBody, 65536}, "budget": {netBudgetTotal, 262144}, "shown": {netBodyShown, 4096}, "URL shown": {netMaxURL, 2000},
		"headers": {netMaxHeaders, 100}, "rows": {netMaxRows, 500}, "depth": {netMaxDepth, 64},
	} {
		if p[0] != p[1] {
			t.Errorf("%s limit is %d, want %d", name, p[0], p[1])
		}
	}
}

// Names that the first round only tested through other rules.
func TestNameRulesOneByOne(t *testing.T) {
	for _, n := range []string{"oauth", "OAuthToken", "authn", "authz", "x-oauth", "AuthKey2"} {
		if !isSecretName(n) {
			t.Errorf("%q is not a secret name", n)
		}
	}
	for _, n := range []string{"author", "authors", "authority", "Authorship", "monkey", "keyboard"} {
		if isSecretName(n) {
			t.Errorf("%q is a secret name", n)
		}
	}
	for _, n := range []string{"key", "keys", "iban", "code", "private"} {
		if !isSecretName(n) {
			t.Errorf("%q is not a secret name", n)
		}
		if isSecretName("my" + n + "s2x") {
			t.Errorf("my%ss2x matched as a whole name", n)
		}
	}
	for _, n := range []string{"masterKey", "fastlyKey", "x-master-key", "Fastly-Key"} {
		if !isSecretName(n) {
			t.Errorf("%q is not a secret name", n)
		}
	}
}

// An error text with a YAML block value is read by the recorder's redactor, which the named assignments do not do.
func TestErrorTextReadsABlockValue(t *testing.T) {
	got := redactErrorText("bad config\npassphrase:\n  "+netPlain+"\nend", 500)
	if strings.Contains(got, netPlain) {
		t.Errorf("error = %q", got)
	}
	// And a header value: the same block form, which only the recorder's rule reads.
	hs, _ := redactHeaders(map[string]string{"X-Cfg": "passphrase:\n  " + netPlain})
	if strings.Contains(hs[0].Value, netPlain) {
		t.Errorf("header = %q", hs[0].Value)
	}
}

// The raw text is bounded before any pattern reads it: a 20 MB value costs no more than a short one, in every place a value is read.
func TestEveryPlaceBoundsItsRawInput(t *testing.T) {
	huge := strings.Repeat("ab=cd; ", 3*1024*1024) // about 20 MB
	places := map[string]func(){
		"a URL":          func() { _ = redactURL("https://h.example.test/p?" + huge) },
		"an error text":  func() { _ = redactErrorText(huge, netMaxError) },
		"short text":     func() { _ = shortText(huge, 500) },
		"a header value": func() { _ = redactHeaderValue(huge, netMaxHeaderValue) },
		"a header": func() {
			_, _ = redactHeaders(map[string]string{"X-A": huge, "Location": "https://h.example.test/" + huge})
		},
		"a header name": func() { _ = isSecretName(huge) },
		"a body":        func() { _ = redactBody(huge[:netBodyMax], 0, "text/plain") },
		"a JSON body":   func() { _ = redactBody(`{"a":"`+huge[:netBodyMax-20]+`"}`, 0, "application/json") },
	}
	limit := time.Second
	if raceEnabled {
		limit = 5 * time.Second // the race detector makes the patterns several times slower
	}
	for name, run := range places {
		start := time.Now()
		run()
		if took := time.Since(start); took > limit {
			t.Errorf("%s: a 20 MB input took %v", name, took)
		}
	}
}

// Assignments on one line: a parent that is followed by a key is not a name with a value.
func TestChainedAssignmentsOnOneLine(t *testing.T) {
	for _, text := range []string{
		"svc: licenseKey: " + netPlain,
		"a: b: c: xsrf=" + netPlain,
		"cfg=db=licenseKey: " + netPlain,
		"db: client_key=" + netPlain + " next: ok",
	} {
		for where, got := range map[string]string{
			"an error text":  redactErrorText(text, 500),
			"a header value": func() string { hs, _ := redactHeaders(map[string]string{"X-Cfg": text}); return hs[0].Value }(),
			"a text body":    redactBody(text, 0, "text/plain").Text,
			"a JSON string":  redactBody(`{"c":`+jsonString(text)+`}`, 0, "application/json").Text,
		} {
			if strings.Contains(got, netPlain) {
				t.Errorf("%q in %s: %q", text, where, got)
			}
		}
	}
	if got := redactErrorText("svc: ok: fine and next: value1234", 500); !strings.Contains(got, "fine") || !strings.Contains(got, "value1234") {
		t.Errorf("ordinary chains were changed: %q", got)
	}
}
