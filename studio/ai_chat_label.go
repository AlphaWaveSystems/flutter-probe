package main

import (
	"errors"
	"strings"
	"unicode"
)

const (
	chatLabelOff = "AI is off"
	chatLabelFix = "AI needs a fix, see Settings"
	chatLabelKey = "Anthropic, key needed"
)

// unsafeLabelRune says whether a character in a model id could scramble or spoof the one-line label: control characters, line
// and paragraph separators, bidirectional controls, zero-width characters, and the comma that separates the model from the place
// ("x, on this machine" must not read as a place). They are shown as ?.
func unsafeLabelRune(r rune) bool {
	switch {
	case unicode.IsControl(r): // C0, DEL and C1
		return true
	case r == 0x2028 || r == 0x2029:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F, r == 0x061C:
		return true
	case r >= 0x200B && r <= 0x200D, r == 0x2060, r == 0xFEFF:
		return true
	case r == ',':
		return true
	}
	return false
}

// labelModel is a model id made fit for the label: gated and cut like every provider string, then with the characters above
// replaced.
func labelModel(id, key string) string {
	return strings.Map(func(r rune) rune {
		if unsafeLabelRune(r) {
			return '?'
		}
		return r
	}, modelID(id, key))
}

// onThisMachine says whether an endpoint is this machine. It is the decision checkLocalEndpoint makes with no confirmation, so
// the header can never call an address local that the settings refuse as not local, and there is no second parser to drift.
func onThisMachine(endpoint string) bool {
	return checkLocalEndpoint(endpoint, false) == nil
}

// chatLabel is the short line in the chat header: where the next message would go. It shows the provider and the model id and
// nothing else: no endpoint, no key, no key source and no path. The model id comes from probe.yaml, so it passes the gate with
// the exact key removed, and it is cut to 200 bytes, like every provider string. err is the refusal aiProvider returned, if any.
func chatLabel(s aiProviderSettings, err error) string {
	if err != nil {
		if errors.Is(err, errAIOff) || errors.Is(err, errNoAPIKey) {
			return chatLabelOff
		}
		return chatLabelFix
	}
	switch s.Provider {
	case "anthropic":
		model := s.Model
		if model == "" {
			model = chatModel
		}
		return "Anthropic, " + labelModel(model, s.APIKey)
	case "openai":
		if s.Model == "" {
			return "OpenAI"
		}
		return "OpenAI, " + labelModel(s.Model, s.APIKey)
	case "local":
		where := "remote endpoint"
		if onThisMachine(s.Endpoint) {
			where = "on this machine"
		}
		return labelModel(s.Model, s.APIKey) + ", " + where
	}
	return chatLabelOff
}

// AIChatLabel returns the chat header's line. It reads the same resolved settings as Chat and PreviewAIChat, so the header
// names the destination a message would go to, or says that AI is off or needs a fix. It makes no request.
//
// It reads the keychain only when the provider cannot be decided without it, which is when the provider is Anthropic or unset
// and probe.yaml names no key variable. For none, a local endpoint and OpenAI it reads no secret.
func (a *App) AIChatLabel() string {
	s, err := a.aiProvider()
	if errors.Is(err, errNoAPIKey) && a.configuredProvider() == "anthropic" {
		return chatLabelKey // Anthropic was chosen, and there is no key yet
	}
	return chatLabel(s, err)
}

// configuredProvider is ai.provider as probe.yaml says it, with no key read.
func (a *App) configuredProvider() string {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	cfg, err := readChatConfig(ws)
	if err != nil {
		return ""
	}
	return cfg.AI.Provider
}
