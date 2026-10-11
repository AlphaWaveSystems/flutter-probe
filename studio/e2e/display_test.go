//go:build studio_e2e

package e2e

import "testing"

// A point on a display left of the main one has a negative x; cliclick only
// treats it as absolute with the `=` prefix.
func TestClickArgIsAbsoluteForNegativeCoordinates(t *testing.T) {
	cases := []struct {
		x, y int
		want string
	}{
		{100, 200, "c:=100,=200"},
		{-779, 69, "c:=-779,=69"},
		{-1880, -40, "c:=-1880,=-40"},
	}
	for _, c := range cases {
		if got := clickArg(c.x, c.y); got != c.want {
			t.Errorf("clickArg(%d,%d) = %q, want %q", c.x, c.y, got, c.want)
		}
	}
}

func TestDisplayContains(t *testing.T) {
	main := Display{X: 0, Y: 0, W: 1728, H: 1117}
	second := Display{X: -1920, Y: 37, W: 1920, H: 1080}
	x, y := second.topLeft(main, 40, 60)
	if !second.contains(main, x, y) {
		t.Errorf("(%d,%d) should be on the second display", x, y)
	}
	if second.contains(main, 100, 100) {
		t.Error("a point on the main display must not be on the second")
	}
}
