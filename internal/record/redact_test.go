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
			if got[key] != Placeholder || got["Content-Type"] != "application/json" || got["prefix-"+name] != "ordinary" {
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
				raw := prefix + "?" + strings.ToUpper(name) + "=sensitive&prefix_" + name + "=safe&" + name + "=again#fragment"
				want := prefix + "?" + strings.ToUpper(name) + "=" + Placeholder + "&prefix_" + name + "=safe&" + name + "=" + Placeholder + "#fragment"
				if got := RedactURL(raw); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
		})
	}
	cases := []struct{ in, want string }{
		{"https://example.test/path?sort=a%20b&flag#token=visible", "https://example.test/path?sort=a%20b&flag#token=visible"},
		{"/path?%74oken=hidden&key=&token=last", "/path?%74oken=<redacted>&key=<redacted>&token=<redacted>"},
		{"/path#fragment?token=visible", "/path#fragment?token=visible"},
		{"http://[broken?token=%zz&safe=ok", "http://[broken?token=<redacted>&safe=ok"},
		{"/path?token&safe=ok", "/path?token&safe=ok"},
		{"/path", "/path"},
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
		{"stripe", "sk_live_Abcdef1234", "sk_live_Abcdef123"},
		{"stripe restricted", "rk_test_Abcdef1234", "rk_test_Abcdef123"},
		{"webhook", "whsec_Abcdef1234", "whsec_Abcdef123"},
		{"slack", "xoxb-Abcdef1234", "xoxb-Abcdef123"},
		{"jwt", "eyJabcdefgh.abcdefgh.abcdefgh", "eyJabcdefg.abcdefgh.abcdefgh"},
		{"probe", "FP2.Abcdef1234", "FP2.Abcdef123"},
		{"assignment", "password=abcd", "password=abc"},
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
			suffix := `, "username": "alice", "count": 42, "ok": true, "id": "550e8400-e29b-41d4-a716-446655440000", "prefix_` + name + `": "safe"}`
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
		{`{"token":123,"tokens":"safe","description":"ordinary words"}`, `{"token":"<redacted>","tokens":"safe","description":"ordinary words"}`},
	}
	for _, tc := range cases {
		if got := RedactBody(tc.in); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}
