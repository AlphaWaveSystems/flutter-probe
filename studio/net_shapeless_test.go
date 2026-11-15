package main

import (
	"net/url"
	"strings"
	"testing"
)

// Layouts that carry a secret with no shape of its own, read by structure: YAML blocks, lists and tags, XML and HTML pairs, name/value
// objects with short member names, and forms inside forms. Each is run as a body and as an error text.
func TestShapelessLayoutsHideTheSecret(t *testing.T) {
	s := netPlain
	form := func(inner string) string { return "payload=" + url.QueryEscape(inner) }
	cases := map[string]string{
		"a YAML block scalar":             "password: |\n  " + s + "\nnext: 1",
		"a folded YAML scalar":            "password: >\n  " + s + "\nnext: 1",
		"a stripped YAML scalar":          "password: |-\n  " + s + "\nnext: 1",
		"a YAML list":                     "password:\n  - " + s + "\n  - other-value\nnext: 1",
		"a YAML comment before the value": "password: # note\n  " + s + "\nnext: 1",
		"a YAML tag":                      "password: !!str " + s,
		"a YAML anchor":                   "password: &p " + s,
		"a nested YAML list":              "db:\n  licenseKey:\n    - " + s + "\n  host: x",
		"an XML item":                     `<item name="password" value="` + s + `"/>`,
		"an XML add":                      `<add key="licenseKey" value="` + s + `"/>`,
		"an XML parameter":                `<parameter name="token" value="` + s + `"></parameter>`,
		"an XML property":                 `<property name="password">` + s + `</property>`,
		"an XML entry":                    `<entry key="password">` + s + `</entry>`,
		"an HTML textarea":                `<textarea name="password">` + s + `</textarea>`,
		"an HTML select":                  `<select name="token"><option value="` + s + `">x</option></select>`,
		"an HTML placeholder":             `<input type="password" placeholder="` + s + `">`,
		"an HTML data attribute":          `<input name="token" data-value="` + s + `" value="x">`,
		"a > inside an attribute":         `<input name="password" title="a>b" value="` + s + `">`,
		"an object with k and v":          `{"k":"password","v":"` + s + `"}`,
		"an object with n and v":          `{"n":"token","v":"` + s + `"}`,
		"an object with prop":             `{"prop":"password","value":"` + s + `"}`,
		"a form inside a form, four deep": form(form(form(form("password=" + s)))),
		"a form with JSON with a form":    form(`{"inner":"` + url.QueryEscape("token="+s) + `"}`),
	}
	for name, in := range cases {
		for where, got := range map[string]string{
			"a body":        redactBody(in, 0, "text/plain").Text,
			"a JSON body":   redactBody(in, 0, "application/json").Text,
			"an error text": redactErrorText(in, 2000),
		} {
			if strings.Contains(got, s) {
				t.Errorf("%s in %s: %q", name, where, got)
			}
		}
	}
	// What is not a secret in the same layouts stays.
	keep := map[string]string{
		"a YAML block":  "title: |\n  hello world\n",
		"an XML item":   `<item name="colour" value="blue"/>`,
		"an HTML input": `<input name="user" value="bob" placeholder="your name">`,
		"a pair":        `{"k":"colour","v":"blue"}`,
		"a YAML list":   "items:\n  - apple\n  - pear\n",
	}
	for name, in := range keep {
		got := redactBody(in, 0, "text/plain").Text
		for _, want := range []string{"hello world", "blue", "bob", "your name", "apple"} {
			if strings.Contains(in, want) && !strings.Contains(got, want) {
				t.Errorf("%s: %q was lost: %q", name, want, got)
			}
		}
	}
}

// A header whose name says nothing can carry a credential after its scheme word.
func TestHeaderValuesWithASchemeWord(t *testing.T) {
	for _, v := range []string{"Basic " + netPlain, "NTLM " + netPlain, "Negotiate " + netPlain, "Token " + netPlain, "Digest " + netPlain, "DPoP " + netPlain, "Bearer " + netPlain, "bearer\t" + netPlain} {
		hs, _ := redactHeaders(map[string]string{"X-Neutral": v})
		if strings.Contains(hs[0].Value, netPlain) {
			t.Errorf("%q -> %q", v, hs[0].Value)
		}
		if !strings.Contains(hs[0].Value, redactedMark) {
			t.Errorf("%q was not redacted: %q", v, hs[0].Value)
		}
	}
	hs, _ := redactHeaders(map[string]string{"Accept": "text/html, tokenizer-app; q=0.9", "X-Note": "basic usage of the thing"})
	if hs[0].Value != "text/html, tokenizer-app; q=0.9" || hs[1].Value != "basic usage of the thing" {
		t.Errorf("ordinary values changed: %+v", hs)
	}
}
