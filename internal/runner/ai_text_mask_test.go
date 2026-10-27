package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// A resolved environment value in an AI assertion is masked before the text model sees the assertion, on the step path.
func TestAssertWithAITextMasksResolvedValuesBeforeTheModel(t *testing.T) {
	const secret = "ai-secret-6161"
	sysdialog.NoteSecret(secret)
	text := &fakeText{answer: `{"answer": true, "reasoning": "ok"}`}
	e := textOnlyExecutor(t, &summaryClient{}, text, nil)
	if err := e.runStep(context.Background(), withAI("the card shows "+secret)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if strings.Contains(text.gotUser, secret) || !strings.Contains(text.gotUser, "****") {
		t.Errorf("the model received the resolved value: %q", text.gotUser)
	}
}

// The step path masks the assertion before it calls runAssertWithAIText, so this test calls that function with the raw
// assertion. It checks that the function masks its own assertion and does not rely on the caller.
func TestRunAssertWithAITextMasksItsOwnAssertion(t *testing.T) {
	const secret = "ai-secret-7272"
	sysdialog.NoteSecret(secret)
	text := &fakeText{answer: `{"answer": true, "reasoning": "ok"}`}
	e := textOnlyExecutor(t, &summaryClient{}, text, nil)
	if err := e.runAssertWithAIText(context.Background(), "the card shows "+secret); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if strings.Contains(text.gotUser, secret) || !strings.Contains(text.gotUser, "****") {
		t.Errorf("runAssertWithAIText sent the raw assertion to the model: %q", text.gotUser)
	}
}
