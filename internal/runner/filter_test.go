package runner

import (
	"slices"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// filterTests keeps the tests a run selects: the named ones when TestNames is set, then the ones carrying a tag.
func TestFilterTests(t *testing.T) {
	tests := []parser.TestDef{
		{Name: "login", Tags: []string{"smoke"}},
		{Name: "checkout", Tags: []string{"smoke", "payments"}},
		{Name: "settings"},
	}
	cases := []struct {
		name string
		opts RunOptions
		want []string
	}{
		{"no filter runs every test", RunOptions{}, []string{"login", "checkout", "settings"}},
		{"named tests only", RunOptions{TestNames: []string{"checkout", "settings"}}, []string{"checkout", "settings"}},
		{"a name the file does not have selects nothing", RunOptions{TestNames: []string{"missing"}}, nil},
		{"an empty name list selects nothing", RunOptions{TestNames: []string{}}, nil},
		{"a tag selects the tests that carry it", RunOptions{Tags: []string{"smoke"}}, []string{"login", "checkout"}},
		{"names and tags narrow together", RunOptions{TestNames: []string{"checkout", "settings"}, Tags: []string{"payments"}}, []string{"checkout"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Runner{opts: c.opts}
			var got []string
			for _, def := range r.filterTests(tests) {
				got = append(got, def.Name)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("selected %v, want %v", got, c.want)
			}
		})
	}
}

// A data-driven row's result is named "<test> [row N]"; DefName gives back the name the file writes.
func TestTestResultDefName(t *testing.T) {
	cases := []struct {
		name string
		res  TestResult
		want string
	}{
		{"a test without rows", TestResult{TestName: "login", Row: -1}, "login"},
		{"the first row", TestResult{TestName: "login [row 1]", Row: 0}, "login"},
		{"a later row", TestResult{TestName: "login [row 12]", Row: 11}, "login"},
		{"a name that only looks like a row", TestResult{TestName: "login [row 2]", Row: -1}, "login [row 2]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.res.DefName(); got != c.want {
				t.Errorf("DefName() = %q, want %q", got, c.want)
			}
		})
	}
	for row := range 3 {
		res := TestResult{TestName: rowName("checkout", row), Row: row}
		if got := res.DefName(); got != "checkout" {
			t.Errorf("row %d: DefName() = %q, want %q", row, got, "checkout")
		}
	}
}
