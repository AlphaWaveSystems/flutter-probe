package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	chatModel      = "claude-sonnet-4-6"
	chatAPIURL     = "https://api.anthropic.com/v1/messages"
	chatAPIVersion = "2023-06-01"
	chatMaxTokens  = 4096

	// Sonnet 4.6 pricing per 1M tokens (USD)
	priceInputPer1M  = 3.0
	priceOutputPer1M = 15.0
)

// ChatMessage is one turn in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is returned from the Chat method.
type ChatResponse struct {
	Content      string  `json:"content"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
}

var chatHTTPClient = &http.Client{Timeout: 120 * time.Second}

// GetAPIKey retrieves the stored Anthropic API key from the platform keychain.
// Returns an empty string (not an error) when no key is stored.
func (a *App) GetAPIKey() (string, error) {
	key, err := keychainGet()
	if err != nil {
		return "", nil
	}
	return key, nil
}

// SetAPIKey stores the Anthropic API key in the platform keychain.
func (a *App) SetAPIKey(key string) error {
	if key == "" {
		return keychainDelete()
	}
	return keychainSet(key)
}

// DeleteAPIKey removes the stored API key from the platform keychain.
func (a *App) DeleteAPIKey() error {
	return keychainDelete()
}

// Chat sends a multi-turn conversation to the Claude API and returns the
// assistant reply together with token usage for the cost counter.
//
// messages is the full conversation history (user + assistant alternating).
// fileContent is the current editor file, injected into the system prompt.
func (a *App) Chat(messages []ChatMessage, fileContent string) (ChatResponse, error) {
	apiKey, _ := keychainGet()
	if apiKey == "" {
		return ChatResponse{}, fmt.Errorf("no API key stored — add your Anthropic key in the AI panel")
	}

	systemPrompt := `You are an expert FlutterProbe test assistant. You help users write, debug, and improve ProbeScript test files.

ProbeScript syntax reference:
- Tests: test "name" (indented steps below)
- Steps: tap "Text", tap #key, type "value" into "Field", see "Text", wait until "Text" appears, swipe up/down/left/right, scroll up/down, go back, open the app, take screenshot "name"
- Conditional: tap "X" if visible, wait until "X" appears
- Tags on next line: @smoke @critical

Keep answers concise. When suggesting changes, output valid ProbeScript.`

	if fileContent != "" {
		systemPrompt += "\n\nCurrent file:\n```\n" + fileContent + "\n```"
	}

	type reqMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model     string   `json:"model"`
		MaxTokens int      `json:"max_tokens"`
		System    string   `json:"system"`
		Messages  []reqMsg `json:"messages"`
	}

	msgs := make([]reqMsg, len(messages))
	for i, m := range messages {
		msgs[i] = reqMsg{Role: m.Role, Content: m.Content}
	}

	payload, err := json.Marshal(reqBody{
		Model:     chatModel,
		MaxTokens: chatMaxTokens,
		System:    systemPrompt,
		Messages:  msgs,
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatAPIURL, bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", chatAPIVersion)

	resp, err := chatHTTPClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == 401 {
		return ChatResponse{}, fmt.Errorf("invalid API key — check your key in the AI panel")
	}
	if resp.StatusCode == 429 {
		return ChatResponse{}, fmt.Errorf("rate limited — try again shortly")
	}
	if resp.StatusCode != 200 {
		return ChatResponse{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return ChatResponse{}, fmt.Errorf("parse response: %w", err)
	}
	if apiResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	text := ""
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	in := apiResp.Usage.InputTokens
	out := apiResp.Usage.OutputTokens
	cost := float64(in)*priceInputPer1M/1_000_000 + float64(out)*priceOutputPer1M/1_000_000

	return ChatResponse{
		Content:      text,
		InputTokens:  in,
		OutputTokens: out,
		CostUSD:      cost,
	}, nil
}
