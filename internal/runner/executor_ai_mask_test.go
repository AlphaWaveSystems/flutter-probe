package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// A resolved environment value is a secret, and a vision provider is a third party. The assertion that reaches the
// provider must have the value masked, the same as the text-only path.
func TestWithAIVisionMasksEnvValue(t *testing.T) {
	const secret = "quorvex-vision-secret-value-longer-than-the-forty-character-cut"
	t.Setenv("PROBE_VISION_TEST_SECRET", secret)
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotPath: writeSolidPNGFixture(t)}
	fp := &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}
	e.aiProvider = fp

	if err := e.runAssert(context.Background(), aiAssertStep("Welcome ${PROBE_VISION_TEST_SECRET}")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(fp.gotAssertion, secret[:12]) {
		t.Fatalf("the vision provider received the start of a resolved environment value: %q", fp.gotAssertion)
	}
	if fp.gotAssertion != "Welcome ****" {
		t.Fatalf("the assertion reaching the vision provider = %q, want the value masked as %q", fp.gotAssertion, "Welcome ****")
	}
}

// The query of `read ... with ai` is masked the same way before the vision provider sees it.
func TestReadWithAIVisionMasksEnvValue(t *testing.T) {
	const secret = "quorvex-read-secret-value-longer-than-the-forty-character-cut"
	t.Setenv("PROBE_READ_VISION_TEST_SECRET", secret)
	e := newTestExecutor()
	e.vars = map[string]string{}
	e.client = &fakeAIClient{screenshotPath: writeSolidPNGFixture(t)}
	fp := &fakeVisionProvider{}
	e.aiProvider = fp

	step := parser.ActionStep{Text: "Code ${PROBE_READ_VISION_TEST_SECRET}", Name: "code"}
	if err := e.runReadWithAI(context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(fp.gotQuery, secret[:12]) {
		t.Fatalf("the vision provider received the start of a resolved environment value: %q", fp.gotQuery)
	}
	if fp.gotQuery != "Code ****" {
		t.Fatalf("the query reaching the vision provider = %q, want the value masked as %q", fp.gotQuery, "Code ****")
	}
}
