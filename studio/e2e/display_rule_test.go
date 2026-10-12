package e2e

import (
	"strings"
	"testing"
)

// The display rule is pure logic, so these tests run in the default
// go test ./... on any OS. The native suite itself is behind the studio_e2e
// build tag.
func TestResolveDisplayRefusesWithoutASecondDisplay(t *testing.T) {
	main := Display{X: 0, Y: 0, W: 1728, H: 1117}
	second := Display{X: -1920, Y: 37, W: 1920, H: 1080}
	third := Display{X: 1728, Y: 0, W: 1280, H: 720}
	one := []Display{main}
	two := []Display{main, second}
	three := []Display{main, second, third}

	cases := []struct {
		name      string
		requested string
		ds        []Display
		want      int    // display number expected; 0 = refused
		reason    string // a fragment of the refusal, when refused
	}{
		{"unset", "", two, 0, "STUDIO_E2E_DISPLAY is not set"},
		{"main display", "1", two, 0, "1 is the main display"},
		{"zero", "0", two, 0, "2 or more"},
		{"negative", "-1", two, 0, "2 or more"},
		{"not a number", "left", two, 0, "2 or more"},
		{"second display missing", "2", one, 0, "only 1 display(s) attached"},
		{"beyond the attached displays", "3", two, 0, "only 2 display(s) attached"},
		{"second display", "2", two, 2, ""},
		{"third display", "3", three, 3, ""},
	}
	for _, c := range cases {
		d, n, err := resolveDisplay(c.requested, c.ds)
		if c.want == 0 {
			if err == nil {
				t.Errorf("%s: resolveDisplay(%q) = display %d, want a refusal", c.name, c.requested, n)
				continue
			}
			if !strings.Contains(err.Error(), c.reason) || !strings.Contains(err.Error(), "display(s) attached") {
				t.Errorf("%s: refusal %q should give the reason and the attached count (%q)", c.name, err, c.reason)
			}
			continue
		}
		if err != nil || n != c.want || d != c.ds[n-1] {
			t.Errorf("%s: resolveDisplay(%q) = (%+v, %d, %v), want display %d", c.name, c.requested, d, n, err, c.want)
		}
	}
}
