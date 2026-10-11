package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// scrollClient scripts a list: the target becomes visible after revealAfter
// scrolls. Embedding fakeAIClient supplies the rest of ProbeClient.
type scrollClient struct {
	fakeAIClient
	revealAfter int
	scrolls     int
	dirs        []string
}

func (c *scrollClient) Scroll(_ context.Context, dir string, _ *probelink.SelectorParam) error {
	c.scrolls++
	c.dirs = append(c.dirs, dir)
	return nil
}

func (c *scrollClient) See(context.Context, probelink.SeeParams) error {
	if c.scrolls >= c.revealAfter {
		return nil
	}
	return errors.New("Expected to see it but it was not found")
}

// agentScrollClient additionally implements the agent-side scroll-until.
type agentScrollClient struct {
	scrollClient
	untilCalls int
	untilErr   error
}

func (c *agentScrollClient) ScrollUntil(_ context.Context, _ string, _, _ *probelink.SelectorParam) error {
	c.untilCalls++
	if c.untilErr == nil {
		c.scrolls = c.revealAfter // the agent did all the scrolling itself
	}
	return c.untilErr
}

func scrollUntilStep(dir parser.SwipeDirection, text string) parser.ActionStep {
	u := parser.Selector{Kind: parser.SelectorText, Text: text}
	return parser.ActionStep{Verb: parser.VerbScroll, Direction: dir, Until: &u, Line: 9}
}

func TestScrollUntil_CLILoopScrollsUntilVisible(t *testing.T) {
	c := &scrollClient{revealAfter: 3}
	e := NewExecutor(c, nil, nil, 30*time.Second, false)

	if err := e.runStep(context.Background(), scrollUntilStep(parser.SwipeDown, "Rate this app")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.scrolls != 3 {
		t.Errorf("scrolls = %d, want 3", c.scrolls)
	}
	for _, d := range c.dirs {
		if d != "down" {
			t.Errorf("direction %q, want down", d)
		}
	}
}

func TestScrollUntil_AlreadyVisibleNeverScrolls(t *testing.T) {
	c := &scrollClient{revealAfter: 0}
	e := NewExecutor(c, nil, nil, 30*time.Second, false)
	if err := e.runStep(context.Background(), scrollUntilStep(parser.SwipeDown, "Quick Add")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.scrolls != 0 {
		t.Errorf("scrolls = %d, want 0 — target was already on screen", c.scrolls)
	}
}

func TestScrollUntil_GivesUpWithActionableError(t *testing.T) {
	c := &scrollClient{revealAfter: 1 << 30}
	e := NewExecutor(c, nil, nil, 30*time.Second, false)

	err := e.runStep(context.Background(), scrollUntilStep(parser.SwipeUp, "Nowhere"))
	if err == nil {
		t.Fatal("expected an error when the target never appears")
	}
	for _, want := range []string{`scroll up until "Nowhere" appears`, "not visible after", "`scroll down` reveals later content"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	if c.scrolls != maxScrollUntilAttempts {
		t.Errorf("scrolls = %d, want exactly %d", c.scrolls, maxScrollUntilAttempts)
	}
}

func TestScrollUntil_AgentSideHandlesItInOneCall(t *testing.T) {
	c := &agentScrollClient{scrollClient: scrollClient{revealAfter: 5}}
	e := NewExecutor(c, nil, nil, 30*time.Second, false)

	if err := e.runStep(context.Background(), scrollUntilStep(parser.SwipeDown, "History")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.untilCalls != 1 {
		t.Errorf("ScrollUntil calls = %d, want 1", c.untilCalls)
	}
}

func TestScrollUntil_AgentErrorIsFinal(t *testing.T) {
	c := &agentScrollClient{scrollClient: scrollClient{revealAfter: 1 << 30}, untilErr: errors.New("scroll down until text(\"X\"): not found")}
	e := NewExecutor(c, nil, nil, 30*time.Second, false)

	err := e.runStep(context.Background(), scrollUntilStep(parser.SwipeDown, "X"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want the agent's error, got %v", err)
	}
	if c.scrolls != 0 {
		t.Errorf("CLI loop must not run after a definitive agent answer, scrolls = %d", c.scrolls)
	}
}
