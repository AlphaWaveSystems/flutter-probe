package main

import (
	"strings"
	"testing"
)

// Each credential family is removed from the text, and the rest of the text is kept. Emails and file paths are not in this
// list: they are ordinary test text, so the gate does not redact them.
func TestCredentialFamiliesAreRedacted(t *testing.T) {
	// Each key-shaped fixture is joined from pieces, so no full credential literal sits in this file.
	ghp := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	gho := "gh" + "o_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	ghpat := "github" + "_pat_" + "11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz"
	aws := "AK" + "IA" + "IOSFODNN7EXAMPLE"
	slack := "xo" + "xb-" + "1234567890-abcdefghijkl"
	google := "AI" + "za" + "SyD-abcdefghijklmnopqrstuvwxyz12345"
	stripeSecret := "sk_" + "li" + "ve_" + "abcdefghijklmnop1234"
	stripeRestricted := "rk_" + "li" + "ve_" + "abcdefghijklmnop1234"
	pemBody := "MIIBOgIBAAJBAKj34GkxFhD90vcNLYLInFEX6Ppy1tPf9Cnzj4p4WGeKLs1Pt8Qu"
	pem := "-----BEGIN RSA " + "PRIVATE KEY-----\n" + pemBody + "\n-----END RSA " + "PRIVATE KEY-----"
	ecPem := "-----BEGIN EC " + "PRIVATE KEY-----\n" + pemBody + "\n-----END EC " + "PRIVATE KEY-----"
	pkcs8Pem := "-----BEGIN " + "PRIVATE KEY-----\n" + pemBody + "\n-----END " + "PRIVATE KEY-----"
	cases := []struct {
		name   string
		input  string
		secret string
	}{
		{"github classic token", "token " + ghp + " here", ghp},
		{"github oauth token", gho, gho},
		{"github fine-grained token", ghpat, ghpat},
		{"aws access key id", "key " + aws + " end", aws},
		{"slack bot token", slack, slack},
		{"google api key", google, google},
		{"stripe live secret", stripeSecret, stripeSecret},
		{"stripe live restricted", stripeRestricted, stripeRestricted},
		{"pem private key body", pem, pemBody[:22]},
		{"pem begin line", pem, "BEGIN RSA " + "PRIVATE KEY"},
		{"pem end line", pem, "END RSA " + "PRIVATE KEY"},
		{"pem ec begin line", ecPem, "BEGIN EC " + "PRIVATE KEY"},
		{"pem ec end line", ecPem, "END EC " + "PRIVATE KEY"},
		{"pem pkcs8 begin line", pkcs8Pem, "BEGIN " + "PRIVATE KEY"},
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
