package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// aiProviderSettings is the provider a Studio chat turn or selector fix uses. It is read from probe.yaml's ai: block, the
// block the CLI reads too, so Studio and the CLI share one provider. An empty Provider means AI is off: nothing is sent
// and the device is not asked for anything.
type aiProviderSettings struct {
	Provider string        // "anthropic", "local" or "openai"; empty when AI is off
	Model    string        // required for "local"; empty means the Anthropic default
	Endpoint string        // OpenAI-compatible base URL, for "local" only
	APIKey   string        // resolved at request time; never stored, returned or logged
	Timeout  time.Duration // zero means the provider default
}

// aiProvider reads the provider for the open workspace. Without an ai: block, a saved Anthropic key still turns the chat
// on, as it did before providers existed. A plain key written into probe.yaml is refused, so a key stays in the keychain
// or in an environment variable named by ${NAME}.
func (a *App) aiProvider() (aiProviderSettings, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()

	var block config.AIConfig
	allowRemote := false
	if ws != "" {
		cfg, err := config.Load(ws)
		if err != nil {
			return aiProviderSettings{}, fmt.Errorf("loading probe.yaml: %w", err)
		}
		block = cfg.AI
		sw, err := readAIContext(ws)
		if err != nil {
			return aiProviderSettings{}, err
		}
		allowRemote = sw.AllowRemoteEndpoint
	}

	key, err := apiKeyFromConfig(block.APIKey)
	if err != nil {
		return aiProviderSettings{}, err
	}

	provider := block.Provider
	if provider == "" {
		if saved, _ := apiKeyLookup(); saved != "" {
			provider = "anthropic"
		}
	}

	switch provider {
	case "":
		return aiProviderSettings{}, errNoAPIKey
	case "anthropic":
		if key == "" {
			key, _ = apiKeyLookup()
		}
		if key == "" {
			return aiProviderSettings{}, errNoAPIKey
		}
		return aiProviderSettings{Provider: provider, Model: block.Model, APIKey: key, Timeout: block.Timeout}, nil
	case "openai":
		if key == "" {
			return aiProviderSettings{}, fmt.Errorf("provider openai needs ai.api_key in probe.yaml, as an environment variable name: ${NAME}")
		}
		return aiProviderSettings{Provider: provider, Model: block.Model, APIKey: key, Timeout: block.Timeout}, nil
	case "local":
		if block.Endpoint == "" {
			return aiProviderSettings{}, fmt.Errorf("provider local needs ai.endpoint in probe.yaml, such as http://127.0.0.1:11434/v1")
		}
		if block.Model == "" {
			return aiProviderSettings{}, fmt.Errorf("provider local needs ai.model in probe.yaml: there is no default model")
		}
		endpoint := config.ResolveEnvVar(block.Endpoint)
		if err := checkLocalEndpoint(endpoint, allowRemote); err != nil {
			return aiProviderSettings{}, err
		}
		return aiProviderSettings{Provider: provider, Model: block.Model, Endpoint: endpoint, APIKey: key, Timeout: block.Timeout}, nil
	default:
		return aiProviderSettings{}, fmt.Errorf("ai.provider %q is not one of anthropic, local or openai", provider)
	}
}

// apiKeyFromConfig resolves ai.api_key. The value must be an environment variable reference, so the key itself never sits
// in probe.yaml. The error names the variable, never the value.
func apiKeyFromConfig(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") {
		return "", fmt.Errorf("probe.yaml holds a plain API key in ai.api_key: name an environment variable instead, as ${NAME}")
	}
	v := config.ResolveEnvVar(raw)
	if v == raw {
		return "", fmt.Errorf("ai.api_key names the environment variable %s, which is not set", strings.TrimSuffix(strings.TrimPrefix(raw, "${"), "}"))
	}
	return v, nil
}

// checkLocalEndpoint refuses an address that is not on this machine unless the workspace confirmed it (R11). It does not
// look the name up, so the check sends nothing.
func checkLocalEndpoint(endpoint string, allowRemote bool) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("ai.endpoint must be a full address, such as http://127.0.0.1:11434/v1")
	}
	if isLoopbackHost(u.Host) || allowRemote {
		return nil
	}
	return fmt.Errorf("ai.endpoint %s is not this machine: set studio.ai_allow_remote_endpoint: true in probe.yaml to confirm it", u.Hostname())
}

// outbound is the one gate for text Studio sends to a provider. Every system prompt and every chat turn passes through it
// (R10), so a credential in the open file or the conversation is redacted before any provider sees it.
func outbound(s string) string {
	return redactCredentials(s)
}

// completeChat sends one turn to the provider the settings name. Local and OpenAI-compatible replies report no token
// usage, so their cost stays zero.
func completeChat(s aiProviderSettings, system string, msgs []claudeMessage) (ChatResponse, error) {
	system = outbound(system)
	for i := range msgs {
		msgs[i].Content = outbound(msgs[i].Content)
	}
	switch s.Provider {
	case "anthropic":
		return callClaude(s.APIKey, s.Model, system, msgs)
	case "local", "openai":
		c, err := ai.NewChatCompleter(s.Provider, s.APIKey, s.Model, s.Endpoint, s.Timeout)
		if err != nil {
			return ChatResponse{}, err
		}
		turns := make([]ai.Turn, len(msgs))
		for i, m := range msgs {
			turns[i] = ai.Turn{Role: m.Role, Content: m.Content}
		}
		timeout := s.Timeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		text, err := c.CompleteTurns(ctx, system, turns)
		if err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{Content: text}, nil
	}
	return ChatResponse{}, errNoAPIKey
}

// ListAIModels returns the model names the local endpoint reports, for the model list in Settings (R3). Anthropic and
// OpenAI have no listing here: Settings keeps a typed name for them. With AI off nothing is sent.
func (a *App) ListAIModels() ([]string, error) {
	s, err := a.aiProvider()
	if err != nil {
		return nil, err
	}
	if s.Provider != "local" {
		return nil, nil
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(s.Endpoint, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if s.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the endpoint answered %d to the model listing", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading the model listing: %w", err)
	}
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("the model listing did not parse: %w", err)
	}
	ids := make([]string, 0, len(listing.Data))
	for _, m := range listing.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
