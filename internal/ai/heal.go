package ai

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// HealResult is the output of an LLM-powered selector heal.
type AIHealResult struct {
	Original   string
	Suggested  string
	Confidence float64
	Reason     string
}

// HealSelector uses the LLM to suggest a replacement for a broken selector,
// combining the failed selector text with the live widget tree dump.
//
// If autoHeal is true the suggestion is returned directly. When false the
// caller should present the suggestion to the user for confirmation.
func (g *Generator) HealSelector(ctx context.Context, original string, widgetTree string, autoHeal bool) (*AIHealResult, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("ai: API key is required for LLM-based healing")
	}

	system := `You are a FlutterProbe selector repair assistant.

Given a broken ProbeScript selector and the current Flutter widget tree, suggest the best replacement.

Respond in EXACTLY this format (3 lines, no extra text):
SELECTOR: <the new selector>
CONFIDENCE: <0.0 to 1.0>
REASON: <one-sentence explanation>

Selector types you may suggest:
- "Visible Text"
- #value_key
- "text" in "Container"
- 1st "Item" / 2nd "Item"

Choose the most specific selector that uniquely identifies the widget.`

	user := fmt.Sprintf("Broken selector: %s\n\nWidget tree:\n%s", original, widgetTree)

	body, err := g.callAPI(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("ai heal: %w", err)
	}

	result := parseHealResponse(original, body)

	if !autoHeal && result.Confidence < 0.8 {
		result.Reason += " (low confidence — review suggested)"
	}

	return result, nil
}

// parseHealResponse extracts structured fields from the LLM's 3-line response.
func parseHealResponse(original, raw string) *AIHealResult {
	result := &AIHealResult{
		Original:   original,
		Confidence: 0.5, // default if parsing fails
	}

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "SELECTOR:"):
			result.Suggested = strings.TrimSpace(strings.TrimPrefix(line, "SELECTOR:"))
		case strings.HasPrefix(line, "CONFIDENCE:"):
			valStr := strings.TrimSpace(strings.TrimPrefix(line, "CONFIDENCE:"))
			if v, err := strconv.ParseFloat(valStr, 64); err == nil {
				result.Confidence = v
			}
		case strings.HasPrefix(line, "REASON:"):
			result.Reason = strings.TrimSpace(strings.TrimPrefix(line, "REASON:"))
		}
	}

	// Fallback: if the LLM didn't follow the format, use the whole response as the selector
	if result.Suggested == "" {
		result.Suggested = strings.TrimSpace(raw)
		result.Reason = "LLM response used as-is (unstructured)"
	}

	return result
}
