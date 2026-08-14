package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultVisionTimeout = 60 * time.Second
	defaultOpenAIModel   = "gpt-4o-mini"
	openAIVisionURL      = "https://api.openai.com/v1/chat/completions"
)

// VisionVerdict is the result of an AI-backed "with ai" screen assertion.
type VisionVerdict struct {
	True      bool
	Reasoning string
}

// NoDefectsAssertion is the fixed prompt for "assert no visual defects with
// ai" — passed to VisionProvider.AssertScreen like any other assertion, so
// the command needs no dedicated provider method or request shape.
const NoDefectsAssertion = "This screen has no visual defects: no text or UI elements are " +
	"cut off, overlapping, or noticeably mis-centered within their containers. If there " +
	"are any such defects, describe them specifically in your reasoning."

// VisionProvider evaluates a natural-language assertion against a screenshot.
// Implementations call the vendor directly — never through a FlutterProbe-
// operated relay, per the project's AI-assertions PRD.
type VisionProvider interface {
	AssertScreen(ctx context.Context, image []byte, assertion string) (VisionVerdict, error)
}

// NewVisionProvider builds a VisionProvider for the given provider name.
// Returns an error for anything other than "openai"/"anthropic"/"local" — a
// "with ai" step must never silently no-op or fall back to a different
// provider than configured.
//
// "local" points at any OpenAI-compatible local inference server (Ollama,
// LM Studio, etc.) via endpoint — nothing leaves the device/host at all,
// not even to a BYO-key cloud vendor. It reuses openAIVision's request
// shape since that's the protocol these servers already speak; api_key is
// optional (most local servers don't require one), but model is required
// with no default — there's no universally-sensible local vision model
// name to fall back to.
func NewVisionProvider(provider, apiKey, model, endpoint string) (VisionProvider, error) {
	client := &http.Client{Timeout: defaultVisionTimeout}
	switch provider {
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("ai: OpenAI API key is required (ai.api_key in probe.yaml)")
		}
		return &openAIVision{apiKey: apiKey, model: orDefault(model, defaultOpenAIModel), client: client, baseURL: openAIVisionURL}, nil
	case "anthropic":
		if apiKey == "" {
			return nil, fmt.Errorf("ai: Anthropic API key is required (ai.api_key in probe.yaml)")
		}
		return &anthropicVision{apiKey: apiKey, model: orDefault(model, defaultModel), client: client}, nil
	case "local":
		if endpoint == "" {
			return nil, fmt.Errorf("ai: ai.endpoint is required for provider: local (e.g. http://localhost:11434/v1 for Ollama)")
		}
		if model == "" {
			return nil, fmt.Errorf("ai: ai.model is required for provider: local — no default vision model to fall back to")
		}
		baseURL := strings.TrimSuffix(endpoint, "/") + "/chat/completions"
		return &openAIVision{apiKey: apiKey, model: model, client: client, baseURL: baseURL}, nil
	default:
		return nil, fmt.Errorf("ai: unknown provider %q — must be \"openai\", \"anthropic\", or \"local\"", provider)
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// ---- Shared verdict prompt/parsing ----

const visionSystemPrompt = `You are evaluating a screenshot of a mobile app screen against a single natural-language assertion.
Reply with ONLY a single-line JSON object, no markdown fences, no other text:
{"answer": true, "reasoning": "one sentence explaining why"}
"answer" must be a JSON boolean: true if the assertion is true of this screenshot, false otherwise.`

func visionUserPrompt(assertion string) string {
	return fmt.Sprintf("Assertion: %s", assertion)
}

type visionAnswer struct {
	Answer    bool   `json:"answer"`
	Reasoning string `json:"reasoning"`
}

// parseVisionAnswer strips markdown fences (in case the model wrapped its
// output despite instructions, mirroring extractProbeScript's tolerance)
// and parses the strict JSON verdict shape.
func parseVisionAnswer(raw string) (VisionVerdict, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) > 1 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var a visionAnswer
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: could not parse provider response as a verdict: %w (raw: %s)", err, raw)
	}
	return VisionVerdict{True: a.Answer, Reasoning: a.Reasoning}, nil
}

// ---- OpenAI vision ----

type openAIVision struct {
	apiKey  string
	model   string
	client  *http.Client
	baseURL string // full chat-completions URL — OpenAI's endpoint, or a local server's per NewVisionProvider's "local" case
}

func (o *openAIVision) AssertScreen(ctx context.Context, image []byte, assertion string) (VisionVerdict, error) {
	b64 := base64.StdEncoding.EncodeToString(image)
	reqBody := map[string]any{
		"model": o.model,
		"messages": []map[string]any{
			{"role": "system", "content": visionSystemPrompt},
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": visionUserPrompt(assertion)},
					{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + b64}},
				},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(payload))
	if err != nil {
		return VisionVerdict{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Most local inference servers (Ollama, LM Studio) don't require auth;
	// only send the header when a key was actually configured.
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: request to %s failed: %w", o.baseURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: reading response from %s: %w", o.baseURL, err)
	}
	if resp.StatusCode == 429 {
		return VisionVerdict{}, fmt.Errorf("ai: %s rate limited (429) — try again shortly", o.baseURL)
	}
	if resp.StatusCode != 200 {
		return VisionVerdict{}, fmt.Errorf("ai: %s returned %d: %s", o.baseURL, resp.StatusCode, string(respBytes))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: parsing response from %s: %w", o.baseURL, err)
	}
	if apiResp.Error != nil {
		return VisionVerdict{}, fmt.Errorf("ai: %s error: %s", o.baseURL, apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return VisionVerdict{}, fmt.Errorf("ai: %s returned no choices", o.baseURL)
	}

	return parseVisionAnswer(apiResp.Choices[0].Message.Content)
}

// ---- Anthropic vision ----

// anthropicVision reuses apiResponse/apiVersion from generate.go's Claude
// Messages API plumbing — same response shape, just a multimodal request.
type anthropicVision struct {
	apiKey string
	model  string
	client *http.Client
}

func (a *anthropicVision) AssertScreen(ctx context.Context, image []byte, assertion string) (VisionVerdict, error) {
	if a.apiKey == "" {
		return VisionVerdict{}, fmt.Errorf("ai: Anthropic API key is required (ai.api_key in probe.yaml)")
	}

	b64 := base64.StdEncoding.EncodeToString(image)
	reqBody := map[string]any{
		"model":      a.model,
		"max_tokens": maxTokens,
		"system":     visionSystemPrompt,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "image", "source": map[string]string{"type": "base64", "media_type": "image/png", "data": b64}},
					{"type": "text", "text": visionUserPrompt(assertion)},
				},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultAPIURL, bytes.NewReader(payload))
	if err != nil {
		return VisionVerdict{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: reading anthropic response: %w", err)
	}
	if resp.StatusCode == 429 {
		return VisionVerdict{}, fmt.Errorf("ai: anthropic API rate limited (429) — try again shortly")
	}
	if resp.StatusCode != 200 {
		return VisionVerdict{}, fmt.Errorf("ai: anthropic API returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return VisionVerdict{}, fmt.Errorf("ai: parsing anthropic response: %w", err)
	}
	if apiResp.Error != nil {
		return VisionVerdict{}, fmt.Errorf("ai: anthropic API error (%s): %s", apiResp.Error.Type, apiResp.Error.Message)
	}
	if len(apiResp.Content) == 0 {
		return VisionVerdict{}, fmt.Errorf("ai: anthropic API returned empty content")
	}

	var sb strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return parseVisionAnswer(sb.String())
}
