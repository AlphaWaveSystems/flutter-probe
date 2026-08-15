package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// scriptedClient wraps fakeAIClient (same package, executor_ai_test.go) and
// overrides Tap/See with call-counting, scriptable failure behavior — for
// testing retry (succeeds after N failures) and optional (always fails).
type scriptedClient struct {
	*fakeAIClient
	tapCalls    int
	tapFailN    int // Tap fails for the first N calls, then succeeds
	tapAlwaysOK bool

	seeCalls    int
	seeFailN    int
	seeAlwaysOK bool

	openLinkCalls int
	openLinkURLs  []string
}

func (f *scriptedClient) Tap(ctx context.Context, sel probelink.SelectorParam) error {
	f.tapCalls++
	if f.tapAlwaysOK {
		return nil
	}
	if f.tapCalls <= f.tapFailN {
		return fmt.Errorf("widget not found (attempt %d)", f.tapCalls)
	}
	return nil
}

func (f *scriptedClient) See(ctx context.Context, params probelink.SeeParams) error {
	f.seeCalls++
	if f.seeAlwaysOK {
		return nil
	}
	if f.seeCalls <= f.seeFailN {
		return fmt.Errorf("widget not found (attempt %d)", f.seeCalls)
	}
	return nil
}

func (f *scriptedClient) OpenLink(ctx context.Context, url string) error {
	f.openLinkCalls++
	f.openLinkURLs = append(f.openLinkURLs, url)
	return nil
}

func newScriptedExecutor(client *scriptedClient) *Executor {
	return NewExecutor(client, nil, nil, 5*time.Second, false) // deviceCtx/onReconnect unused
}

// TestRunRetry_SucceedsAfterFailures covers the core `retry N times`
// contract: the block re-runs from the top on failure, stopping at the
// first success, rather than always running every iteration like `repeat`.
func TestRunRetry_SucceedsAfterFailures(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 2}
	e := newScriptedExecutor(client)

	step := parser.RetryStep{
		Count: 3,
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Flaky"}},
		},
	}
	err := e.runRetry(context.Background(), step)
	if err != nil {
		t.Fatalf("expected success on the 3rd attempt, got: %v", err)
	}
	if client.tapCalls != 3 {
		t.Errorf("tap calls: got %d, want 3 (2 failures + 1 success)", client.tapCalls)
	}
}

// TestRunRetry_ExhaustsAllAttempts confirms a genuinely-always-failing body
// still errors out after Count attempts, rather than retrying forever or
// silently succeeding.
func TestRunRetry_ExhaustsAllAttempts(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100} // always fails within 2 attempts
	e := newScriptedExecutor(client)

	step := parser.RetryStep{
		Count: 2,
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "NeverThere"}},
		},
	}
	err := e.runRetry(context.Background(), step)
	if err == nil {
		t.Fatal("expected an error after exhausting all attempts, got nil")
	}
	if !strings.Contains(err.Error(), "all 2 attempt(s) failed") {
		t.Errorf("error should report exhausted attempts, got: %v", err)
	}
	if client.tapCalls != 2 {
		t.Errorf("tap calls: got %d, want exactly 2 (no more, no fewer)", client.tapCalls)
	}
}

// TestRunStep_OptionalActionSwallowsFailure covers the E-1 "optional"
// modifier's core contract: the step is attempted (unlike "if visible",
// which skips it entirely), and a failure is logged but doesn't fail the
// test.
func TestRunStep_OptionalActionSwallowsFailure(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100}
	e := newScriptedExecutor(client)

	step := parser.ActionStep{
		Verb:     parser.VerbTap,
		Sel:      &parser.Selector{Kind: parser.SelectorText, Text: "MaybeThere"},
		Optional: true,
	}
	err := e.RunStep(context.Background(), step)
	if err != nil {
		t.Errorf("expected optional step's failure to be swallowed, got: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("expected exactly 1 attempt (optional doesn't retry), got %d", client.tapCalls)
	}
}

// TestRunStep_NonOptionalActionStillFails is the regression guard: without
// the "optional" suffix, an identical failing step must still fail the
// test, proving the swallow logic is actually gated on the flag.
func TestRunStep_NonOptionalActionStillFails(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100}
	e := newScriptedExecutor(client)

	step := parser.ActionStep{
		Verb: parser.VerbTap,
		Sel:  &parser.Selector{Kind: parser.SelectorText, Text: "AlwaysMissing"},
	}
	err := e.RunStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected a non-optional step's failure to propagate, got nil")
	}
}

// TestRunStep_OptionalAssertSwallowsFailure confirms "optional" also works
// on assertions (see ... optional), not just actions.
func TestRunStep_OptionalAssertSwallowsFailure(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, seeFailN: 100}
	e := newScriptedExecutor(client)

	step := parser.AssertStep{
		Sel:      parser.Selector{Kind: parser.SelectorText, Text: "MaybeThere"},
		Optional: true,
	}
	err := e.RunStep(context.Background(), step)
	if err != nil {
		t.Errorf("expected optional assertion's failure to be swallowed, got: %v", err)
	}
}

// TestRunStep_OpenLink_ExternalBrowser covers E-3's backward-compat case:
// without the "in the app" suffix, `open link` must still go through the
// Dart agent's url_launcher path (client.OpenLink), unchanged from before.
func TestRunStep_OpenLink_ExternalBrowser(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client)

	step := parser.ActionStep{Verb: parser.VerbOpenLink, Name: "https://example.com"}
	if err := e.RunStep(context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.openLinkCalls != 1 {
		t.Errorf("expected client.OpenLink to be called once, got %d", client.openLinkCalls)
	}
	if len(client.openLinkURLs) != 1 || client.openLinkURLs[0] != "https://example.com" {
		t.Errorf("unexpected OpenLink URL(s): %v", client.openLinkURLs)
	}
}

// TestRunStep_OpenLink_DeepLink_CloudModeSkips covers E-3's DeepLink path in
// cloud mode (no DeviceContext available, since deep-linking is dispatched
// CLI-side via adb/simctl, not through the agent RPC): it must skip with a
// warning rather than panic or error, and critically must NOT fall through
// to client.OpenLink — that would silently open the URL externally instead
// of the "into the app" behavior the test author asked for.
func TestRunStep_OpenLink_DeepLink_CloudModeSkips(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client) // deviceCtx is nil — see newScriptedExecutor

	step := parser.ActionStep{Verb: parser.VerbOpenLink, Name: "myapp://profile/42", DeepLink: true}
	if err := e.RunStep(context.Background(), step); err != nil {
		t.Fatalf("expected a graceful cloud-mode skip, got error: %v", err)
	}
	if client.openLinkCalls != 0 {
		t.Errorf("expected client.OpenLink NOT to be called for a DeepLink step, got %d calls", client.openLinkCalls)
	}
}

// TestRunStep_AddMedia_CloudModeSkips covers E-4's cloud-mode guard: `add
// media` is CLI-side only (adb/simctl, no Dart RPC), so with no
// DeviceContext available it must skip gracefully with a warning instead of
// panicking on a nil deviceCtx.
func TestRunStep_AddMedia_CloudModeSkips(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client) // deviceCtx is nil — see newScriptedExecutor

	step := parser.ActionStep{Verb: parser.VerbAddMedia, Name: "testdata/photo.jpg"}
	if err := e.RunStep(context.Background(), step); err != nil {
		t.Fatalf("expected a graceful cloud-mode skip, got error: %v", err)
	}
}
