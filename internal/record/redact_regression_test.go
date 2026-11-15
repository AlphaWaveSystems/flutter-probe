package record

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/record/testdata/oldref"
)

// A recorder bundle must never be weaker than it was. testdata/oldref is a copy of this package's redactor from before the same-line
// fix; for every input below, whatever the old one hid, this one hides: no case where the old output lacks the secret and the new one
// has it. The corpus is names x secrets x layouts, with the layouts of nested, block, blank-line and carriage-return forms.
func TestRedactorIsNeverWeakerThanItWas(t *testing.T) {
	names := []string{"password", "passwd", "secret", "token", "api_key", "apiKey", "access_token", "client_secret", "authorization", "sid", "session", "refresh_token", "X-Api-Key", "private_key", "pin", "otp", "passphrase", "credential", "jwt", "csrf", "sig", "signature", "bearer", "auth"}
	secrets := []string{"hunter2-Xk9pq", "ab=cd:ef12", "abc,def12", "dGVzdHBhc3N3b3Jk", "48151623", "p@ss/w0rd!", "a-b-c-d-e-f", "Zm9vYmFy=="}
	layouts := []string{
		`{"N":"S"}`, `{"a":{"b":{"N":"S"}}}`, "{\n  \"N\": \"S\"\n}", `[{"N":"S"}]`, `{"N" : "S"}`, "{\"N\":\n\"S\"}", `{"N":"S"`,
		`N=S`, `N: S`, `N : S`, "N:\n  S", "N:\n\tS", "N=\nS", "N:  \r\n  S", "db:\n  N: S", `N: "S"`, `N: 'S'`, "N:\n\n  S", "a:\n  b:\n    N: S\n",
		`a=1&N=S&b=2`, `N%3DS`, `<N>S</N>`, `<N attr="1">S</N>`, `login(N:"S")`, `"N": "S"`, `N="S"`, "N\t=\tS",
		"N:\n  - S", "N: |\n  S", "N=S\nother=1", "export N=S", "--N=S", "--N S", "N S", "N=>S", "N:\n  S\nz: 1", "N:\n  \"S\"", "N:\n  'S'", "N: S # c",
		"parent:\n  N:\n    S", "N =\nS", "N:\r\nS", "N:\nS", "---\nN: S\n", "N:   S", "- N: S", "svc:\n  N:\n  S", "x: y\nN:\n\n\nS", "N:\n  S1\n  S2",
		"N:\nS\nM: z", "N=\n\nS", "svc: N: S", "a: b: N=S", "N:\n  a=b\n  S", "k=v|N=S|z=1", "N:S", "N=S;x=1", "x:\n  N:\n    S\n  y: 1\n",
	}
	build := func(layout, name, secret string) string {
		var b strings.Builder
		for i := 0; i < len(layout); i++ {
			switch layout[i] {
			case 'N':
				b.WriteString(name)
			case 'S':
				b.WriteString(secret)
			default:
				b.WriteByte(layout[i])
			}
		}
		return b.String()
	}
	total, regress := 0, 0
	var examples []string
	for _, n := range names {
		for _, l := range layouts {
			for _, s := range secrets {
				in := build(l, n, s)
				total++
				old, now := oldref.RedactBody(in), RedactBody(in)
				if !strings.Contains(old, s) && strings.Contains(now, s) {
					regress++
					if len(examples) < 8 {
						examples = append(examples, fmt.Sprintf("%q secret %q: old %q new %q", in, s, old, now))
					}
				}
			}
		}
	}
	for _, n := range names {
		for _, f := range []string{"https://h/p?N=S", "https://h/p?a=1&N=S&b=2", "https://h/p#N=S", "https://h/p;N=S/x", "https://u:S@h/p", "https://h/p?N%3DS", "https://h/p?N[]=S", "https://h/p?user[N]=S"} {
			for _, s := range secrets {
				in := build(f, n, s)
				total += 2
				if old, now := oldref.RedactURL(in), RedactURL(in); !strings.Contains(old, s) && strings.Contains(now, s) {
					regress++
					examples = append(examples, fmt.Sprintf("url %q", in))
				}
				if old, now := oldref.RedactHeaders(map[string]string{n: s})[n], RedactHeaders(map[string]string{n: s})[n]; !strings.Contains(old, s) && strings.Contains(now, s) {
					regress++
					examples = append(examples, fmt.Sprintf("header %q", n))
				}
			}
		}
	}
	t.Logf("%d cases, %d where the old redactor hid a secret and this one shows it", total, regress)
	if total < 300 {
		t.Fatalf("only %d cases", total)
	}
	if regress != 0 {
		t.Fatalf("%d regressions, for example:\n%s", regress, strings.Join(examples, "\n"))
	}
}
