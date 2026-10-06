package runner

import (
	"context"
	"fmt"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

// errNeedsVision is returned by the pixel-based AI steps when ai.vision is
// false: they genuinely need an image, and silently skipping or guessing from
// text would turn a real check into a fake pass.
func errNeedsVision(step string) error {
	return fmt.Errorf("%q needs a model that can see images, but ai.vision is false in probe.yaml — use a vision model for this step (see `probe ai doctor`)", step)
}

// runAssertWithAIText is `see "<assertion>" with ai` for a text-only model
// (ai.vision: false): the model judges the assertion from the screen's visible
// texts and widget keys instead of a screenshot.
//
// It refuses to run when ai.redact rules exist: those black out screenshot
// regions, which cannot be applied to a text list — sending the text of a
// widget the user asked to keep private would defeat the rule.
func (e *Executor) runAssertWithAIText(ctx context.Context, assertion string) error {
	if len(e.aiCfg.Redact) > 0 {
		return fmt.Errorf("with ai: ai.redact rules cannot be applied in text-only mode (ai.vision: false) — remove the rules or use a vision model")
	}
	if e.aiText == nil {
		return fmt.Errorf("with ai: text-only mode is not configured")
	}
	vs, ok := e.client.(visibleSummarizer)
	if !ok {
		return fmt.Errorf("with ai: this agent connection cannot report the visible screen contents (needs flutter_probe_agent with probe.visible_summary)")
	}
	sum, err := vs.VisibleSummary(ctx)
	if err != nil {
		return fmt.Errorf("with ai: reading the visible screen contents failed (agent too old for text-only mode?): %w", err)
	}
	verdict, err := ai.TextVerdict(ctx, e.aiText, assertion, sum.Texts, sum.Keys)
	if err != nil {
		return fmt.Errorf("with ai: %w", err)
	}
	if !verdict.True {
		return fmt.Errorf("AI assertion failed: %q — %s", assertion, verdict.Reasoning)
	}
	return nil
}
