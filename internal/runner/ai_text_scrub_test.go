package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// promptRecorder is a text model that keeps the prompt it was sent, and answers with a fixed reply.
type promptRecorder struct {
	answer string
	prompt string
}

func (p *promptRecorder) Complete(_ context.Context, system, user string) (string, error) {
	p.prompt = system + "\n" + user
	return p.answer, nil
}

// The text model is a remote service. The assertion, which a step builds from an environment value, and the screen's
// texts and keys must reach it with that value masked.
func TestTextOnlyWithAI_PromptMasksEnvValues(t *testing.T) {
	const name, secret = "PROBE_AI_TEXT_SECRET", "quorvex-secret-123"
	t.Setenv(name, secret)
	f := false
	client := &summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"Welcome " + secret, "Sign in"},
		Keys:  []string{"login_" + secret},
	}}
	model := &promptRecorder{answer: `{"answer": true, "reasoning": "the banner is shown"}`}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	e.aiCfg = config.AIConfig{Provider: "local", Vision: &f}
	e.aiProvider = panicVision{t}
	e.aiText = model

	step := withAI("the banner says ${" + name + "}")
	if err := e.runStep(context.Background(), step); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if strings.Contains(model.prompt, secret) {
		t.Fatalf("the text model was sent the environment value:\n%s", model.prompt)
	}
	for _, want := range []string{"the banner says ****", `"Welcome ****"`, "login_****", `"Sign in"`} {
		if !strings.Contains(model.prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, model.prompt)
		}
	}

	// A failing verdict prints the assertion, so the printed assertion is masked too.
	model.answer = `{"answer": false, "reasoning": "not shown"}`
	err := e.runStep(context.Background(), step)
	if err == nil || !strings.Contains(err.Error(), "AI assertion failed") || strings.Contains(err.Error(), secret) {
		t.Fatalf("the failure should name the assertion without the environment value: %v", err)
	}
}
