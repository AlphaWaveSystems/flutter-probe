package cli

import (
	"reflect"
	"testing"
)

func TestStripFlag(t *testing.T) {
	got := stripFlag([]string{"test", "a.probe", "--locales", "de,ja", "-y"}, "--locales", "")
	if !reflect.DeepEqual(got, []string{"test", "a.probe", "-y"}) {
		t.Fatalf("got %v", got)
	}
	got = stripFlag([]string{"test", "--locales=de,ja", "-y"}, "--locales", "")
	if !reflect.DeepEqual(got, []string{"test", "-y"}) {
		t.Fatalf("got %v", got)
	}
}

func TestWithLocaleReplaces(t *testing.T) {
	got := withLocale([]string{"test", "--locale", "fr", "x"}, "de")
	if !reflect.DeepEqual(got, []string{"test", "x", "--locale", "de"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSuffixOutput(t *testing.T) {
	cases := [][2][]string{
		{{"test", "-o", "reports/r.json"}, {"test", "-o", "reports/r.de.json"}},
		{{"test", "--output", "r.html"}, {"test", "--output", "r.de.html"}},
		{{"test", "--output=r.xml"}, {"test", "--output=r.de.xml"}},
		{{"test", "x"}, {"test", "x"}},
	}
	for _, c := range cases {
		if got := suffixOutput(c[0], "de"); !reflect.DeepEqual(got, c[1]) {
			t.Errorf("suffixOutput(%v) = %v, want %v", c[0], got, c[1])
		}
	}
}
