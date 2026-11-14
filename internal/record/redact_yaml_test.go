package record

import (
	"strings"
	"testing"
)

// A parent line such as "db:" must not take the next line's key as its value: that swallowed "password:" and left its value.
func TestRedactBodyNestedConfigKeepsTheCredentialLineVisibleToTheRule(t *testing.T) {
	const secret = "hunter2-Xk9pq"
	names := []string{"password", "secret", "token", "api_key", "pass", "sid", "otp", "pin", "passphrase", "session", "auth", "credential", "jwt", "csrf", "sig", "signature", "bearer", "client_secret", "access_token"}
	for _, name := range names {
		for _, quote := range []string{"", "'", `"`} {
			body := "db:\n  host: example.test\n  " + name + ": " + quote + secret + quote + "\n  port: 5432\n"
			got := RedactBody(body)
			if strings.Contains(got, secret) {
				t.Errorf("%s with quote %q: the value is shown:\n%s", name, quote, got)
			}
			if !strings.Contains(got, "host: example.test") || !strings.Contains(got, "port: 5432") {
				t.Errorf("%s: the other lines were changed:\n%s", name, got)
			}
		}
	}
	// The same text inside a JSON string, and under a parent that is itself a word the rule knows nothing about.
	json := `{"config":"svc:\n  password: '` + secret + `'\n  user: bob"}`
	if got := RedactBody(json); strings.Contains(got, secret) {
		t.Errorf("JSON string: %s", got)
	}
	if got := RedactBody("a:\n  b:\n    c:\n      password: " + secret + "\n"); strings.Contains(got, secret) {
		t.Errorf("deep nesting: %s", got)
	}
}

// The YAML block form puts the value alone on the next line.
func TestRedactBodyYAMLBlockValue(t *testing.T) {
	const secret = "hunter2-Xk9pq"
	got := RedactBody("password:\n  " + secret + "\nuser: bob\n")
	if strings.Contains(got, secret) || !strings.Contains(got, "user: bob") {
		t.Errorf("block value: %q", got)
	}
	// A credential name followed by a mapping is a parent, not a value: its children are read on their own.
	got = RedactBody("credentials:\n  user: bob\n  password: " + secret + "\n")
	if strings.Contains(got, secret) || !strings.Contains(got, "user") {
		t.Errorf("a credential parent: %q", got)
	}
	// A name that is not a credential is left alone, with its next line.
	plain := "title:\n  Quarterly report\nauthor: someone\n"
	if got := RedactBody(plain); got != plain {
		t.Errorf("plain text changed: %q", got)
	}
}

// The value must still be read on the same line when it is there, with spaces or tabs, and an empty value is no match.
func TestRedactBodySameLineAssignmentsStillWork(t *testing.T) {
	for _, body := range []string{"password:   hunter2-Xk9pq", "password =\thunter2-Xk9pq", "password=hunter2-Xk9pq&a=1", `"password" : "hunter2-Xk9pq"`} {
		if got := RedactBody(body); strings.Contains(got, "hunter2") {
			t.Errorf("%q -> %q", body, got)
		}
	}
	if got := RedactBody("password:\n"); got != "password:\n" {
		t.Errorf("an empty value: %q", got)
	}
}
