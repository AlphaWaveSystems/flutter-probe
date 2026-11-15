package main

import (
	"strings"
	"testing"
)

// A JWT cut anywhere (a 500 or 1,000 character one at the end of a bounded field, or a clipped file) leaves nothing of its payload: once
// the header and four characters of the payload are there, the rest of the pattern is redacted. A cut before that sends only the start
// of the header, which holds no claim.
func TestAJwtCutInsideItsPayloadIsRedacted(t *testing.T) {
	header := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	for _, size := range []int{500, 1000} {
		payload := "eyJzdWIiOiJ" + strings.Repeat("Qx7_-", size/5)
		sig := strings.Repeat("Zy9", 20)
		jwt := header + "." + payload + "." + sig
		for _, cut := range []int{len(header), len(header) + 1, len(header) + 4, len(header) + 5, len(header) + 6, len(header) + 12, len(header) + 40, len(header) + 200, len(header) + len(payload), len(header) + len(payload) + 1, len(header) + len(payload) + 10} {
			if cut > len(jwt) {
				continue
			}
			text := "token: " + jwt[:cut] + " end"
			for name, out := range map[string]string{"redactCredentials": redactCredentials(text), "redactShapes": redactShapes(text)} {
				claims := ""
				if cut > len(header)+1 {
					claims = jwt[len(header)+1 : min(cut, len(header)+1+8)]
				}
				if claims != "" && len(claims) >= 4 && strings.Contains(out, claims) {
					t.Errorf("%s, a %d character JWT cut at %d: the payload start %q is sent: %q", name, size, cut, claims, out)
				}
				if cut >= len(header)+5 && !strings.Contains(out, redactedMark) {
					t.Errorf("%s, cut at %d: nothing was redacted: %q", name, cut, out)
				}
			}
		}
	}
	// A header with no payload yet holds no claim, and ordinary text that has a dot after eyJ is not changed.
	for _, in := range []string{"eyJhbGciOiJIUzI1NiJ9", "eyJhbGciOiJIUzI1NiJ9.", "eyJ.ab", "see eyJabcd.ef end", "xeyJabcd.efgh"} {
		if out := redactCredentials(in); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
}
