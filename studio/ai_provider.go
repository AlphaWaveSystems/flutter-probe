package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

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
	Notice   string        // a line to show with the reply, such as a plain key that Studio did not use
}

var (
	errAIOff    = errors.New("AI is off: ai.provider is none in probe.yaml")
	errPlainKey = errors.New("probe.yaml holds a plain API key in ai.api_key, which Studio does not use. Save the key in the AI panel, " +
		"or name an environment variable instead, as ${NAME}")
)

// plainKeyNotice is shown with a reply that used the saved keychain key because probe.yaml held a plain key.
const plainKeyNotice = "probe.yaml holds a plain API key that Studio does not use. This reply used the saved keychain key."

// chatConfig is the part of probe.yaml that Studio's AI reads. Only these keys are decoded, so an error in an unrelated key
// (agent.port, for one) does not block the chat.
type chatConfig struct {
	AI struct {
		Provider string `yaml:"provider"`
		Endpoint string `yaml:"endpoint"`
		Model    string `yaml:"model"`
		APIKey   string `yaml:"api_key"`
		Timeout  string `yaml:"timeout"`
	} `yaml:"ai"`
	Studio struct {
		AllowRemoteEndpoint bool `yaml:"ai_allow_remote_endpoint"`
	} `yaml:"studio"`
}

// readChatConfig reads the AI keys of a workspace's probe.yaml. No workspace, or a missing file, means no configuration.
func readChatConfig(workspace string) (chatConfig, error) {
	var c chatConfig
	if workspace == "" {
		return c, nil
	}
	data, err := os.ReadFile(filepath.Join(workspace, "probe.yaml"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("reading probe.yaml: %w", err)
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("probe.yaml: the ai or studio section is not readable: %w", err)
	}
	return c, nil
}

// aiProvider reads the provider for the open workspace. provider: none turns AI off whatever is saved. Without an ai: block,
// a saved Anthropic key still turns the chat on, as it did before providers existed. A plain key in probe.yaml is never
// used: with a saved Anthropic key the reply uses that key and says so; otherwise the chat refuses and says how to fix it.
func (a *App) aiProvider() (aiProviderSettings, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()

	cfg, err := readChatConfig(ws)
	if err != nil {
		return aiProviderSettings{}, err
	}
	allowRemote := false
	if ws != "" {
		sw, err := readAIContext(ws)
		if err != nil {
			return aiProviderSettings{}, err
		}
		allowRemote = sw.AllowRemoteEndpoint
	}
	timeout, err := parseAITimeout(cfg.AI.Timeout)
	if err != nil {
		return aiProviderSettings{}, err
	}
	if cfg.AI.Provider == "none" {
		return aiProviderSettings{}, errAIOff
	}

	key, plain, err := envAPIKey(cfg.AI.APIKey)
	if err != nil {
		return aiProviderSettings{}, err
	}
	provider := cfg.AI.Provider
	if provider == "" {
		if saved, _ := apiKeyLookup(); saved != "" {
			provider = "anthropic"
		}
	}

	switch provider {
	case "":
		if plain {
			return aiProviderSettings{}, errPlainKey
		}
		return aiProviderSettings{}, errNoAPIKey
	case "anthropic":
		notice := ""
		if key == "" {
			saved, _ := apiKeyLookup()
			if saved == "" {
				if plain {
					return aiProviderSettings{}, errPlainKey
				}
				return aiProviderSettings{}, errNoAPIKey
			}
			key = saved
			if plain {
				notice = plainKeyNotice
			}
		}
		return aiProviderSettings{Provider: provider, Model: cfg.AI.Model, APIKey: key, Timeout: timeout, Notice: notice}, nil
	case "openai":
		if plain {
			return aiProviderSettings{}, errPlainKey
		}
		if key == "" {
			return aiProviderSettings{}, fmt.Errorf("provider openai needs ai.api_key set to an environment variable, as ${NAME}")
		}
		return aiProviderSettings{Provider: provider, Model: cfg.AI.Model, APIKey: key, Timeout: timeout}, nil
	case "local":
		if plain {
			return aiProviderSettings{}, errPlainKey
		}
		if cfg.AI.Endpoint == "" {
			return aiProviderSettings{}, fmt.Errorf("provider local needs ai.endpoint in probe.yaml, such as http://127.0.0.1:11434/v1")
		}
		if cfg.AI.Model == "" {
			return aiProviderSettings{}, fmt.Errorf("provider local needs ai.model in probe.yaml: there is no default model")
		}
		endpoint := config.ResolveEnvVar(cfg.AI.Endpoint)
		if err := checkLocalEndpoint(endpoint, allowRemote); err != nil {
			return aiProviderSettings{}, err
		}
		// The saved keychain key is never sent to a local endpoint: only an environment variable can supply a key here.
		return aiProviderSettings{Provider: provider, Model: cfg.AI.Model, Endpoint: endpoint, APIKey: key, Timeout: timeout}, nil
	default:
		return aiProviderSettings{}, fmt.Errorf("ai.provider %q is not one of anthropic, local, openai or none", provider)
	}
}

// parseAITimeout reads ai.timeout. An empty value means the provider default. The error names the value, which is not secret.
func parseAITimeout(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("ai.timeout %q is not a Go duration, such as 60s", raw)
	}
	return d, nil
}

// envAPIKey resolves ai.api_key. A ${NAME} reference is read from the environment. Any other value is a plain key: it is
// reported as plain and never used. The error names the variable, never a value.
func envAPIKey(raw string) (key string, plain bool, err error) {
	if raw == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") {
		return "", true, nil
	}
	v := config.ResolveEnvVar(raw)
	if v == raw {
		return "", false, fmt.Errorf("ai.api_key names the environment variable %s, which is not set", raw[2:len(raw)-1])
	}
	return v, false, nil
}

// checkLocalEndpoint refuses an address that is not this machine unless the workspace confirmed it. It accepts only http and
// https, and it refuses user name and password parts in the address. The host is parsed, never matched as text, and it does
// not look the name up, so the check sends nothing.
func checkLocalEndpoint(endpoint string, allowRemote bool) error {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("ai.endpoint must be a full http or https address, such as http://127.0.0.1:11434/v1")
	}
	if u.User != nil {
		return fmt.Errorf("ai.endpoint must not contain a user name or password")
	}
	if isLoopbackHost(u.Host) || allowRemote {
		return nil
	}
	return fmt.Errorf("ai.endpoint %s is not this machine: set studio.ai_allow_remote_endpoint: true in probe.yaml to confirm it", u.Hostname())
}

// outbound is the one gate for text Studio sends to a provider. Every system prompt and every chat turn passes through it,
// so a credential in the open file or the conversation is redacted before any provider sees it.
func outbound(s string) string {
	return redactCredentials(s)
}

// fromProvider cleans text a provider returned before Studio shows it: the exact key sent in the request is removed, and the
// credential families are redacted. The key is removed first, so a key of no known family does not survive.
func fromProvider(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, redactedMark)
	}
	return outbound(s)
}

// noRedirects stops a provider client at the first answer. A redirect would carry the request, its prompt and any key to
// another host, so an endpoint that redirects is an error.
func noRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// newAIClient is the HTTP client for every provider request Studio makes.
func newAIClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: noRedirects}
}

// errorBody is the start of a provider's error body, for an error message. A body can echo what it was sent, so it is cut,
// and the message that carries it also goes through the gate.
func errorBody(b []byte) string {
	const max = 200
	if len(b) > max {
		b = b[:max]
	}
	return strings.ToValidUTF8(string(b), "?")
}

// completeChat sends one turn to the provider the settings name. The system prompt and every message pass through the gate
// first, for every provider, and an error from a provider goes through the gate too, because its text can echo what it was
// sent. Local and OpenAI-compatible replies report no token usage, so their cost stays zero.
func completeChat(s aiProviderSettings, system string, msgs []claudeMessage) (ChatResponse, error) {
	gated := make([]claudeMessage, len(msgs))
	for i, m := range msgs {
		gated[i] = claudeMessage{Role: m.Role, Content: outbound(m.Content)}
	}
	resp, err := sendTurn(s, outbound(system), gated)
	if err != nil {
		return ChatResponse{}, errors.New(outbound(err.Error()))
	}
	resp.Notice = s.Notice
	return resp, nil
}

// sendTurn makes the request for one turn. Its input has already been through the gate.
func sendTurn(s aiProviderSettings, system string, msgs []claudeMessage) (ChatResponse, error) {
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

// ListAIModels returns the model names the local endpoint reports, for the model list in Settings. Anthropic and OpenAI have
// no listing here, so Settings keeps a typed name for them. With AI off nothing is sent.
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
	resp, err := newAIClient(timeout).Do(req)
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
			ids = append(ids, fromProvider(m.ID, s.APIKey))
		}
	}
	sort.Strings(ids)
	return ids, nil
}
