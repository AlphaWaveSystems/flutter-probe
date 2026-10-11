package sysdialog

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	labels := []string{"Don’t Allow", "Allow", "Allow Once"}
	cases := map[string]int{"Allow": 1, "allow": 1, "Don't Allow": 0, "don’t allow": 0, "once": 2, "Nope": -1, "": -1}
	for in, want := range cases {
		if got := Match(in, labels); got != want {
			t.Errorf("Match(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDialogMatchesTitle(t *testing.T) {
	d := Dialog{Title: "“App” Would Like to Send You Notifications", Texts: []string{"Notifications may include alerts"}}
	for _, ok := range []string{"", "notifications", "WOULD LIKE", "alerts"} {
		if !d.MatchesTitle(ok) {
			t.Errorf("%q should match", ok)
		}
	}
	if d.MatchesTitle("Apple Account") {
		t.Error("must not match unrelated title")
	}
}

func TestDismissIndexPrefersCancelOverAllow(t *testing.T) {
	if i := DismissIndex([]string{"Allow", "Don’t Allow"}); i != 1 {
		t.Errorf("got %d", i)
	}
	if i := DismissIndex([]string{"OK", "Cancel"}); i != 1 {
		t.Errorf("got %d", i)
	}
	if i := DismissIndex([]string{"OK"}); i != -1 {
		t.Errorf("a lone OK is not a dismiss button, got %d", i)
	}
}

func TestResolveSecret(t *testing.T) {
	t.Setenv("PROBE_TEST_SECRET", "hunter2")
	for _, ref := range []string{"$PROBE_TEST_SECRET", "${PROBE_TEST_SECRET}"} {
		v, fromEnv, err := ResolveSecret(ref)
		if err != nil || v != "hunter2" || !fromEnv {
			t.Errorf("%s -> %q %v %v", ref, v, fromEnv, err)
		}
	}
	if v, fromEnv, _ := ResolveSecret("literal"); v != "literal" || fromEnv {
		t.Errorf("literal passthrough broken: %q %v", v, fromEnv)
	}
	if _, _, err := ResolveSecret("$PROBE_DEFINITELY_UNSET_VAR"); err == nil || !strings.Contains(err.Error(), "PROBE_DEFINITELY_UNSET_VAR") {
		t.Errorf("unset var must name itself in the error, got %v", err)
	}
	// The value itself must not be in any error text.
	t.Setenv("PROBE_EMPTY_SECRET", "")
	if _, _, err := ResolveSecret("$PROBE_EMPTY_SECRET"); err == nil {
		t.Error("empty env var should be rejected")
	}
}

func TestScrub(t *testing.T) {
	got := Scrub("failed typing hunter2 into field; hunter2 again", "hunter2", "")
	if strings.Contains(got, "hunter2") || strings.Count(got, "****") != 2 {
		t.Errorf("got %q", got)
	}
}
