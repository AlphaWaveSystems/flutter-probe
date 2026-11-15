package probelink

import (
	"encoding/json"
	"testing"
)

// TestSwipeParamsSendsADistanceOnlyWhenSet checks the wire format: a swipe without a distance sends the same JSON it always
// did, and a swipe with one adds the percent field.
func TestSwipeParamsSendsADistanceOnlyWhenSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    SwipeParams
		want string
	}{
		{"no distance", SwipeParams{Direction: "up"}, `{"direction":"up"}`},
		{"distance", SwipeParams{Direction: "up", Percent: 40}, `{"direction":"up","percent":40}`},
	} {
		got, err := json.Marshal(tc.p)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}
