package textfold

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFoldCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ In, Out string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Fold(c.In); got != c.Out {
			t.Errorf("Fold(%q) = %q, want %q", c.In, got, c.Out)
		}
	}
}

func TestEqualContains(t *testing.T) {
	if !Equal("Don’t Allow", "don't allow") || !Contains("Müller Straße 5", "strasse") || Equal("a", "b") {
		t.Fatal("Equal/Contains wrong")
	}
}
