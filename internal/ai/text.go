package ai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TextCompleter is a text-in/text-out LLM call. It is the one thing the
// optional AI helpers (generation, selector suggestions, failure triage, the
// doctor) need, so they all work with any configured provider — including a
// small local model with no vision support — without caring which one it is.
//
// FP-14: nothing that decides whether a test passes may depend on a
// TextCompleter. Callers use it only for advisory output or explicit,
// user-invoked commands, and treat every error as "no AI answer available".
type TextCompleter interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// NewTextCompleter builds a TextCompleter for an ai: provider ("openai",
// "anthropic" or "local"). Same validation as NewVisionProvider so a
// misconfigured block fails the same way everywhere.
func NewTextCompleter(provider, apiKey, model, endpoint string, timeout time.Duration) (TextCompleter, error) {
	switch provider {
	case "openai", "local":
		vp, err := NewVisionProvider(provider, apiKey, model, endpoint, timeout)
		if err != nil {
			return nil, err
		}
		return vp.(*openAIVision), nil
	case "anthropic":
		if apiKey == "" {
			return nil, fmt.Errorf("ai: Anthropic API key is required (ai.api_key in probe.yaml)")
		}
		g := NewGenerator(apiKey, orDefault(model, defaultModel))
		g.Client = &http.Client{Timeout: timeoutOrDefault(timeout), CheckRedirect: noRedirects}
		return g, nil
	default:
		return nil, fmt.Errorf("ai: unknown provider %q — must be \"openai\", \"anthropic\", or \"local\"", provider)
	}
}

// Complete makes openAIVision (which also backs provider: local) a TextCompleter.
func (o *openAIVision) Complete(ctx context.Context, system, user string) (string, error) {
	return o.chatCompletion(ctx, system, user, nil)
}

// Turn is one message of a multi-turn conversation: Role is "user" or "assistant".
type Turn struct {
	Role    string
	Content string
}

// ChatCompleter sends a system prompt and a whole conversation, for chat surfaces such as Studio. Like TextCompleter, it
// is advisory only: nothing that decides a pass or a fail may depend on it.
type ChatCompleter interface {
	CompleteTurns(ctx context.Context, system string, turns []Turn) (string, error)
}

// NewChatCompleter builds a ChatCompleter for an OpenAI-compatible provider ("local" or "openai"). The anthropic provider
// has its own Messages API client in the caller, so it is refused here rather than built twice.
func NewChatCompleter(provider, apiKey, model, endpoint string, timeout time.Duration) (ChatCompleter, error) {
	switch provider {
	case "openai", "local":
		vp, err := NewVisionProvider(provider, apiKey, model, endpoint, timeout)
		if err != nil {
			return nil, err
		}
		return vp.(*openAIVision), nil
	default:
		return nil, fmt.Errorf("ai: chat completion needs \"openai\" or \"local\", not %q", provider)
	}
}

// CompleteTurns sends the conversation in order, after the system prompt. It shares the request and response handling
// with Complete.
func (o *openAIVision) CompleteTurns(ctx context.Context, system string, turns []Turn) (string, error) {
	msgs := make([]map[string]any, 0, len(turns)+1)
	msgs = append(msgs, map[string]any{"role": "system", "content": system})
	for _, t := range turns {
		msgs = append(msgs, map[string]any{"role": t.Role, "content": t.Content})
	}
	return o.postChat(ctx, msgs)
}

// Complete makes the Claude-backed Generator a TextCompleter.
func (g *Generator) Complete(ctx context.Context, system, user string) (string, error) {
	if !g.usable() {
		return "", fmt.Errorf("ai: API key is required (set --api-key or ai.api_key in probe.yaml)")
	}
	return g.callAPI(ctx, system, user)
}

const textVerdictSystemPrompt = `You are evaluating the current screen of a mobile app against a single natural-language assertion.
You cannot see the screen. You are given the texts and widget keys that are visible on it.
Reply with ONLY a single-line JSON object, no markdown fences, no other text:
{"answer": true, "reasoning": "one sentence explaining why"}
"answer" must be a JSON boolean: true only if the listed texts/keys clearly support the assertion. If they do not contain enough information to tell, answer false and say what is missing. Never assume anything that is not listed.`

// TextVerdict evaluates assertion against the screen's visible texts and keys
// for models without vision (FP-14). Same verdict shape as the vision path.
func TextVerdict(ctx context.Context, c TextCompleter, assertion string, texts, keys []string) (VisionVerdict, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Assertion: %s\n\nVisible texts:\n", assertion)
	for _, t := range texts {
		fmt.Fprintf(&sb, "- %q\n", t)
	}
	sb.WriteString("\nVisible widget keys:\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "- %s\n", k)
	}
	out, err := c.Complete(ctx, textVerdictSystemPrompt, sb.String())
	if err != nil {
		return VisionVerdict{}, err
	}
	return parseVisionAnswer(ReasoningStripped(out), secretOf(c))
}

// ReasoningStripped removes <think>...</think> blocks some local reasoning
// models emit before their answer, so the answer is what callers parse/print.
func ReasoningStripped(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end < 0 {
			s = s[:start] // unterminated: drop the reasoning tail
			break
		}
		s = s[:start] + s[start+end+len("</think>"):]
	}
	return strings.TrimSpace(s)
}
