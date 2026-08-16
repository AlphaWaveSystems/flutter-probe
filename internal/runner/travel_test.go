package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// ---- buildTravelRoute: pure interpolation math, no device I/O ----

// TestBuildTravelRoute_TwoWaypoints covers the basic single-leg case: over a
// 3s route sampled at the 1s travelInterval, the leg splits into 3 equal
// steps, so including the starting waypoint there are 4 frames and 3 gaps of
// 1s each. The very first frame must be the origin waypoint unchanged (the
// device visibly starts there, not somewhere already interpolated), and the
// very last frame must land exactly on the destination waypoint (frac==1.0
// is an exact float64 division, not an approximation).
func TestBuildTravelRoute_TwoWaypoints(t *testing.T) {
	waypoints := []LatLng{
		{Lat: 37.7749, Lng: -122.4194},
		{Lat: 37.7849, Lng: -122.4094},
	}
	frames, gaps := buildTravelRoute(waypoints, 3*time.Second)

	if len(frames) != 4 {
		t.Fatalf("frames: got %d, want 4", len(frames))
	}
	if len(gaps) != len(frames)-1 {
		t.Fatalf("gaps: got %d, want %d (frames-1)", len(gaps), len(frames)-1)
	}
	for i, g := range gaps {
		if g != 1*time.Second {
			t.Errorf("gap %d: got %v, want 1s", i, g)
		}
	}
	if frames[0].lat != waypoints[0].Lat || frames[0].lng != waypoints[0].Lng {
		t.Errorf("first frame: got %+v, want origin waypoint %+v", frames[0], waypoints[0])
	}
	last := frames[len(frames)-1]
	if last.lat != waypoints[1].Lat || last.lng != waypoints[1].Lng {
		t.Errorf("last frame: got %+v, want destination waypoint %+v", last, waypoints[1])
	}
}

// TestBuildTravelRoute_DefaultDuration confirms that an unspecified duration
// (0, meaning `travel to` had no "over N seconds" clause) doesn't jump
// instantly — it falls back to exactly one travelInterval per leg, so a
// 2-waypoint route still produces a start frame, a 1s gap, and an end frame.
func TestBuildTravelRoute_DefaultDuration(t *testing.T) {
	waypoints := []LatLng{
		{Lat: 0, Lng: 0},
		{Lat: 1, Lng: 1},
	}
	frames, gaps := buildTravelRoute(waypoints, 0)

	if len(frames) != 2 {
		t.Fatalf("frames: got %d, want 2 (default 1 travelInterval leg)", len(frames))
	}
	if len(gaps) != 1 || gaps[0] != travelInterval {
		t.Errorf("gaps: got %v, want [%v]", gaps, travelInterval)
	}
}

// TestBuildTravelRoute_MultiLeg confirms waypoint order and count carry
// through for a 3-waypoint (2-leg) route, and that the frame landing exactly
// on the leg boundary matches the middle waypoint precisely — a route
// through N waypoints must actually visit each one, not just start and end.
func TestBuildTravelRoute_MultiLeg(t *testing.T) {
	waypoints := []LatLng{
		{Lat: 37.7749, Lng: -122.4194},
		{Lat: 37.7849, Lng: -122.4094},
		{Lat: 37.7949, Lng: -122.3994},
	}
	frames, gaps := buildTravelRoute(waypoints, 4*time.Second)

	// legDuration = 4s/2 legs = 2s; steps/leg = int(2s/1s) = 2, so each leg
	// contributes 2 frames: start (1) + 2 steps/leg * 2 legs = 5 frames.
	if len(frames) != 5 {
		t.Fatalf("frames: got %d, want 5", len(frames))
	}
	if len(gaps) != 4 {
		t.Fatalf("gaps: got %d, want 4", len(gaps))
	}
	// Frame index 2 is the last frame of leg 0 (frac==1.0) — it must land
	// exactly on the middle waypoint.
	mid := frames[2]
	if mid.lat != waypoints[1].Lat || mid.lng != waypoints[1].Lng {
		t.Errorf("leg boundary frame: got %+v, want middle waypoint %+v", mid, waypoints[1])
	}
	last := frames[len(frames)-1]
	if last.lat != waypoints[2].Lat || last.lng != waypoints[2].Lng {
		t.Errorf("last frame: got %+v, want final waypoint %+v", last, waypoints[2])
	}
}

// TestBuildTravelRoute_TooFewWaypointsReturnsNil guards the degenerate case
// (0 or 1 waypoints — no legs to interpolate) so callers can't be handed a
// route that would panic on division by zero legs.
func TestBuildTravelRoute_TooFewWaypointsReturnsNil(t *testing.T) {
	if frames, gaps := buildTravelRoute(nil, 5*time.Second); frames != nil || gaps != nil {
		t.Errorf("nil waypoints: got frames=%v gaps=%v, want nil, nil", frames, gaps)
	}
	if frames, gaps := buildTravelRoute([]LatLng{{Lat: 1, Lng: 1}}, 5*time.Second); frames != nil || gaps != nil {
		t.Errorf("single waypoint: got frames=%v gaps=%v, want nil, nil", frames, gaps)
	}
}

// ---- runTravel / dispatchStep: executor-level wiring ----

// TestDispatchStep_TravelStep_CloudModeSkips covers the same nil-deviceCtx
// "cloud mode" contract every other DeviceContext-dispatched verb has
// (see VerbSetLocation, VerbKill, etc.): with no device connection at all,
// travel must skip gracefully rather than panicking on a nil deviceCtx.
func TestDispatchStep_TravelStep_CloudModeSkips(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client) // deviceCtx is nil — see newScriptedExecutor

	step := parser.TravelStep{
		Waypoints: []parser.Waypoint{
			{Lat: "37.7749", Lng: "-122.4194"},
			{Lat: "37.7849", Lng: "-122.4094"},
		},
		Duration: 2,
	}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("unexpected error in cloud mode: %v", err)
	}
}

// TestRunTravel_TooFewWaypointsIsError confirms the waypoint-count guard
// runs — and rejects a malformed route — before ever touching e.deviceCtx,
// so it fires the same way whether or not a device is connected.
func TestRunTravel_TooFewWaypointsIsError(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client)

	step := parser.TravelStep{
		Waypoints: []parser.Waypoint{{Lat: "37.7749", Lng: "-122.4194"}},
		Duration:  2,
	}
	err := e.dispatchStep(context.Background(), context.Background(), step)
	if err == nil {
		t.Fatal("expected an error for a route with fewer than 2 waypoints, got nil")
	}
	if !strings.Contains(err.Error(), "at least 2 waypoints") {
		t.Errorf("error message: got %q, want it to mention 'at least 2 waypoints'", err.Error())
	}
}

// TestRunTravel_InvalidCoordinateIsError guards against a malformed
// waypoint's lat/lng string (never expected from the parser, but runTravel
// parses these independently and should fail clearly rather than panicking
// or silently sending garbage to the device).
func TestRunTravel_InvalidCoordinateIsError(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client)

	step := parser.TravelStep{
		Waypoints: []parser.Waypoint{
			{Lat: "not-a-number", Lng: "-122.4194"},
			{Lat: "37.7849", Lng: "-122.4094"},
		},
		Duration: 2,
	}
	// deviceCtx is nil in this test executor, so the cloud-mode skip would
	// normally short-circuit before parsing coordinates — construct a
	// minimal non-nil DeviceContext instead so runTravel reaches the
	// lat/lng parsing step this test targets.
	e.deviceCtx = &DeviceContext{}
	err := e.dispatchStep(context.Background(), context.Background(), step)
	if err == nil {
		t.Fatal("expected an error for an invalid latitude, got nil")
	}
	if !strings.Contains(err.Error(), "invalid latitude") {
		t.Errorf("error message: got %q, want it to mention 'invalid latitude'", err.Error())
	}
}

// TestStepDescription_Travel covers the progress-output description for
// both the with-duration and without-duration forms.
func TestStepDescription_Travel(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}})

	withDuration := e.stepDescription(parser.TravelStep{
		Waypoints: []parser.Waypoint{{}, {}},
		Duration:  10,
	})
	if withDuration != "travel through 2 waypoints over 10 seconds" {
		t.Errorf("with duration: got %q", withDuration)
	}

	withoutDuration := e.stepDescription(parser.TravelStep{
		Waypoints: []parser.Waypoint{{}, {}},
	})
	if withoutDuration != "travel through 2 waypoints" {
		t.Errorf("without duration: got %q", withoutDuration)
	}
}
