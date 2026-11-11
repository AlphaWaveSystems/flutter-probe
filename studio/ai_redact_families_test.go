package main

import (
	"strings"
	"testing"
)

// Each credential family is removed from the text, and the rest of the text is kept. Emails and file paths are not in this
// list: they are ordinary test text, so the gate does not redact them.
func TestCredentialFamiliesAreRedacted(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		secret string
	}{
		{"github classic token", "token ghp_abcdefghijklmnopqrstuvwxyz0123456789 here", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"github oauth token", "gho_abcdefghijklmnopqrstuvwxyz0123456789", "gho_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"github fine-grained token", "github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz", "github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz"},
		{"aws access key id", "key AKIAIOSFODNN7EXAMPLE end", "AKIAIOSFODNN7EXAMPLE"},
		{"slack bot token", "xoxb-1234567890-abcdefghijkl", "xoxb-1234567890-abcdefghijkl"},
		{"google api key", "AIzaSyD-abcdefghijklmnopqrstuvwxyz12345", "AIzaSyD-abcdefghijklmnopqrstuvwxyz12345"},
		{"stripe live secret", "sk_live_abcdefghijklmnop1234", "sk_live_abcdefghijklmnop1234"},
		{"stripe live restricted", "rk_live_abcdefghijklmnop1234", "rk_live_abcdefghijklmnop1234"},
		{"pem private key body", "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf9Cnzj4p4WGeKLs1Pt8Qu\n-----END RSA PRIVATE KEY-----", "MIIBOgIBAAJBAKj34GkxFh"},
		{"basic authorization header", "Authorization: Basic dXNlcjpwYXNzd29yZDEyMw==", "dXNlcjpwYXNzd29yZDEyMw"},
		{"secret-named assignment", "MY_API_KEY=abcd1234efgh5678", "abcd1234efgh5678"},
		{"password line", "password: hunter2!", "hunter2"},
		{"token in a url", "https://x.example/cb?token=abc123def456&x=1", "abc123def456"},
	}
	for _, c := range cases {
		out := redactCredentials(c.input)
		if strings.Contains(out, c.secret) {
			t.Errorf("%s: the secret survived: %q", c.name, out)
		}
		if !strings.Contains(out, redactedMark) {
			t.Errorf("%s: no redaction marker: %q", c.name, out)
		}
	}
}

// The names of an assignment and a query parameter stay, so the text still says what was there.
func TestAssignmentAndQueryKeepTheirNames(t *testing.T) {
	if out := redactCredentials("MY_API_KEY=abcd1234efgh5678"); !strings.HasPrefix(out, "MY_API_KEY=") {
		t.Errorf("the assignment lost its name: %q", out)
	}
	if out := redactCredentials("https://x.example/cb?token=abc123def456&x=1"); !strings.Contains(out, "&x=1") {
		t.Errorf("the query lost the parameter after the secret: %q", out)
	}
}

// Ordinary test text is kept unchanged, including a word that begins like a credential family.
func TestOrdinaryTestTextIsKept(t *testing.T) {
	kept := []string{
		`tap "Sign in"`,
		`wait until "Welcome back, Ada" appears`,
		`type "ada@example.com" into "Email"`,
		`open the app at /Users/alex/dev/app/lib/main.dart`,
		`wait until "basic plan" appears`,
		`see "tokenizer settings" appears`,
	}
	for _, s := range kept {
		if out := redactCredentials(s); out != s {
			t.Errorf("ordinary text changed: %q became %q", s, out)
		}
	}
}
