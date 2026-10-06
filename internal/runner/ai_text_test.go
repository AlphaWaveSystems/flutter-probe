package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type fakeText struct {
	answer string
	gotUser string
}

func (f *fakeText) Complete(_ context.Context, _, user string) (string, error) {
	f.gotUser = user
	return f.answer, nil
}

// panicVision fails the test if a screenshot-based provider is touched.
type panicVision struct{ t *testing.T }

func (p panicVision) AssertScreen(context.Context, []byte, string) (ai.VisionVerdict, error) {
	p.t.Fatal("text-only mode must not send a screenshot")
	return ai.VisionVerdict{}, nil
}
func (p panicVision) ExtractText(context.Context, []byte, string) (string, error) {
	p.t.Fatal("text-only mode must not send a screenshot")
	return "", nil
}

func textOnlyExecutor(t *testing.T, client *summaryClient, text *fakeText, redact []config.RedactRule) *Executor {
	f := false
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	e.aiCfg = config.AIConfig{Provider: "local", Vision: &f, Redact: redact}
	e.aiProvider = panicVision{t}
	e.aiText = text
	return e
}

func withAI(assertion string) parser.AssertStep {
	return parser.AssertStep{Sel: parser.Selector{Kind: parser.SelectorText, Text: assertion}, WithAI: true, Line: 3}
}

func TestTextOnlyWithAI_JudgesFromVisibleTextsNotPixels(t *testing.T) {
	client := &summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"Order total", "$42.00"}, Keys: []string{"checkout_total"}}}
	text := &fakeText{answer: `{"answer": true, "reasoning": "total is shown"}`}
	e := textOnlyExecutor(t, client, text, nil)

	if err := e.runStep(context.Background(), withAI("the order total is displayed")); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, want := range []string{"the order total is displayed", `"$42.00"`, "checkout_total"} {
		if !strings.Contains(text.gotUser, want) {
			t.Errorf("prompt missing %q:\n%s", want, text.gotUser)
		}
	}
}

func TestTextOnlyWithAI_FalseVerdictFailsTheStep(t *testing.T) {
	client := &summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"Loading"}}}
	e := textOnlyExecutor(t, client, &fakeText{answer: `{"answer": false, "reasoning": "still loading"}`}, nil)
	err := e.runStep(context.Background(), withAI("checkout is complete"))
	if err == nil || !strings.Contains(err.Error(), "AI assertion failed") || !strings.Contains(err.Error(), "still loading") {
		t.Fatalf("got %v", err)
	}
}

func TestTextOnlyWithAI_RefusesWhenRedactRulesExist(t *testing.T) {
	client := &summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"secret"}}}
	text := &fakeText{answer: `{"answer": true, "reasoning": "x"}`}
	e := textOnlyExecutor(t, client, text, []config.RedactRule{{Selector: "#card_number"}})
	err := e.runStep(context.Background(), withAI("anything"))
	if err == nil || !strings.Contains(err.Error(), "ai.redact") {
		t.Fatalf("redact rules cannot protect text, so text-only must refuse: %v", err)
	}
	if text.gotUser != "" {
		t.Error("nothing may be sent to the model when redaction cannot be applied")
	}
}

func TestTextOnlyMode_PixelStepsSayTheyNeedVision(t *testing.T) {
	client := &summaryClient{}
	e := textOnlyExecutor(t, client, &fakeText{}, nil)
	if err := e.runStep(context.Background(), parser.AssertNoDefectsStep{Line: 1}); err == nil || !strings.Contains(err.Error(), "ai.vision is false") {
		t.Errorf("no-defects: %v", err)
	}
	read := parser.ActionStep{Verb: parser.VerbReadWithAI, Text: "the OTP", Name: "otp", Line: 2}
	if err := e.runStep(context.Background(), read); err == nil || !strings.Contains(err.Error(), "ai.vision is false") {
		t.Errorf("read: %v", err)
	}
}
