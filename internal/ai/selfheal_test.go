package ai_test

import (
	"testing"

	"github.com/flutterprobe/probe/internal/ai"
)

var sampleTree = []ai.WidgetInfo{
	{Type: "ElevatedButton", Key: "login_btn", Text: "Log In", X: 180, Y: 400},
	{Type: "TextField", Key: "email_field", Text: "", Hint: "Email address", X: 180, Y: 200},
	{Type: "TextField", Key: "password_field", Text: "", Hint: "Password", X: 180, Y: 280},
	{Type: "Text", Key: "", Text: "Welcome back!", X: 180, Y: 100},
	{Type: "Text", Key: "", Text: "Dashboard", X: 180, Y: 50},
}

func TestSelfHealer_FuzzyTextMatch(t *testing.T) {
	healer := ai.NewSelfHealer()
	// "Log In" vs "Login" — should fuzzy match
	result, err := healer.TryHeal("Login", "text", sampleTree)
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if result.HealedSelector == "" {
		t.Error("no healed selector")
	}
	if result.Confidence < 0.5 {
		t.Errorf("confidence too low: %.2f", result.Confidence)
	}
}

func TestSelfHealer_ExactTextMatch(t *testing.T) {
	healer := ai.NewSelfHealer()
	result, err := healer.TryHeal("Dashboard", "text", sampleTree)
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if result.Confidence < 0.9 {
		t.Errorf("expected high confidence for exact match, got %.2f", result.Confidence)
	}
}

func TestSelfHealer_PartialKeyMatch(t *testing.T) {
	healer := ai.NewSelfHealer()
	// "login_button" was renamed to "login_btn"
	result, err := healer.TryHeal("#login_button", "id", sampleTree)
	if err != nil {
		t.Fatalf("heal: %v", err)
	}
	if result.Strategy != "key_partial" {
		t.Errorf("strategy: got %q, want key_partial", result.Strategy)
	}
}

func TestSelfHealer_NoMatch(t *testing.T) {
	healer := ai.NewSelfHealer()
	_, err := healer.TryHeal("XYZCompletelyUnrelated", "text", sampleTree)
	if err == nil {
		t.Error("expected error for no match")
	}
}

func TestSelfHealer_EmptyTree(t *testing.T) {
	healer := ai.NewSelfHealer()
	_, err := healer.TryHeal("anything", "text", nil)
	if err == nil {
		t.Error("expected error for empty tree")
	}
}

// ---- Test generator tests ----

func TestTestGenerator_BasicRecording(t *testing.T) {
	gen := ai.NewTestGenerator()
	events := []ai.RecordingEvent{
		{Action: "tap", Target: "Sign In"},
		{Action: "type", Target: "Email", Text: "user@test.com"},
		{Action: "type", Target: "Password", Text: "pass123"},
		{Action: "tap", Target: "Continue"},
		{Action: "see", Target: "Dashboard"},
	}

	probe := gen.GenerateFromRecording("login flow", events)
	if probe == "" {
		t.Fatal("empty output")
	}
	if !containsStr(probe, "test") {
		t.Error("missing test declaration")
	}
	if !containsStr(probe, "Sign In") {
		t.Error("missing Sign In action")
	}
	if !containsStr(probe, "Dashboard") {
		t.Error("missing Dashboard assertion")
	}
}

func TestTestGenerator_EmptyRecording(t *testing.T) {
	gen := ai.NewTestGenerator()
	probe := gen.GenerateFromRecording("empty", nil)
	if probe == "" {
		t.Error("expected minimal output for empty recording")
	}
}

func TestTestGenerator_DeduplicatesConsecutive(t *testing.T) {
	gen := ai.NewTestGenerator()
	events := []ai.RecordingEvent{
		{Action: "see", Target: "Loading"},
		{Action: "see", Target: "Loading"}, // duplicate
		{Action: "see", Target: "Done"},
	}
	probe := gen.GenerateFromRecording("t", events)
	// Count occurrences of "Loading"
	count := 0
	idx := 0
	for {
		pos := indexStr(probe[idx:], "Loading")
		if pos < 0 {
			break
		}
		count++
		idx += pos + 1
	}
	if count > 1 {
		t.Errorf("consecutive duplicate not removed: %q", probe)
	}
}

// ---- Helpers ----

func containsStr(s, sub string) bool {
	return indexStr(s, sub) >= 0
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
