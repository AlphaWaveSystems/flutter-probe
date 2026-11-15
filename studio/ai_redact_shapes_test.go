package main

import (
	"strings"
	"testing"
)

func TestGateRedactsShortJWTAndSpacedCards(t *testing.T) {
	redacted := []string{
		"token eyJhbGci.eyJzdWIi.abc123 sent",
		"jwt=eyJhbGciOi.eyJzdWIiOiIx.",
		"card 4111 1111 1111 1111 declined",
		"card 4111-1111-1111-1111 declined",
		"card 4111111111111111 declined",
		"amex 3782 822463 10005 declined",
	}
	for _, in := range redacted {
		out := redactCredentials(in)
		if !strings.Contains(out, redactedMark) {
			t.Errorf("not redacted: %q -> %q", in, out)
		}
		for _, leak := range []string{"eyJhbGci", "4111", "1111", "3782", "822463"} {
			if strings.Contains(out, leak) {
				t.Errorf("%q still contains %q: %q", in, leak, out)
			}
		}
	}
}

func TestGateKeepsOrdinaryNumbers(t *testing.T) {
	kept := []string{
		"tap the button 3 times",
		"order 4111 1111 1111 1112 failed", // fails the Luhn check
		"phone 555 123 4567",
		"version 1.2.3 build 20240101",
		"eyJ.short",
		"12 34 56",
	}
	for _, in := range kept {
		if out := redactCredentials(in); out != in {
			t.Errorf("changed ordinary text: %q -> %q", in, out)
		}
	}
}

func TestGateShapesReachEveryOutboundPath(t *testing.T) {
	in := "pay with 4111 1111 1111 1111 using eyJhbGci.eyJzdWIi.abc123"
	out := outbound(in)
	if strings.Contains(out, "4111") || strings.Contains(out, "eyJhbGci") {
		t.Fatalf("outbound leaked: %q", out)
	}
}

func TestGateCardLengthBounds(t *testing.T) {
	for _, n := range []string{"4000000000006", "4000 0000 0000 0000 006", "4000000000000000006"} {
		if out := redactCredentials("card " + n + " end"); out != "card "+redactedMark+" end" {
			t.Errorf("%q not redacted: %q", n, out)
		}
	}
	// 12 and 20 digits pass the Luhn check but are not card lengths.
	for _, n := range []string{"400000000002", "40000000000000000002"} {
		if out := redactCredentials("n " + n); out != "n "+n {
			t.Errorf("%q changed: %q", n, out)
		}
	}
}

func TestGateShortJWTWithUrlSafeCharacters(t *testing.T) {
	for _, in := range []string{"t eyJab-_cd.ef-_gh.ij-_ end", "t eyJab_cd.ef_gh. end", "t eyJab-cd.ef-gh.- end"} {
		if out := redactCredentials(in); out != "t "+redactedMark+" end" {
			t.Errorf("%q -> %q", in, out)
		}
	}
	// Too short to be a JWT shape: kept.
	for _, in := range []string{"eyJab.cd.ef", "eyJabcd.ef.gh"} {
		if out := redactCredentials(in); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
}
