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
// and paragraph separators, every format (Cf) character, the blank-looking ones the category does not hold, and the comma that separates the model from the place
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
	case r == 0x2800, r == 0x3164, r == 0x115F, r == 0x1160, r == 0xFFA0: // blank-looking characters: braille blank, Hangul fillers
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF: // variation selectors
		return true
	case r >= 0xE0000 && r <= 0xE007F: // tag characters
		return true
	case r == 0x00AD, r == 0x180E, r == 0x034F: // soft hyphen, Mongolian vowel separator, combining grapheme joiner
		return true
	case r >= 0x206A && r <= 0x206F, r >= 0xFFF9 && r <= 0xFFFB: // deprecated format controls, interlinear annotation
		return true
	case r == 0xFFFC, r == 0x17B4, r == 0x17B5: // object replacement character; Khmer inherent vowels, which show as nothing (not Cf)
		return true
	case unicode.Is(unicode.Cf, r): // every other format character: the lists above are what was found, this is the category
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

// ChatLabelParts is the header line in three parts, so the page can shorten a long model id and keep the rest in view. The line is
// Prefix + Model + Suffix. Only Model comes from probe.yaml.
type ChatLabelParts struct {
	Prefix string `json:"prefix"`
	Model  string `json:"model"`
	Suffix string `json:"suffix"`
}

// String is the whole line.
func (p ChatLabelParts) String() string { return p.Prefix + p.Model + p.Suffix }

// chatLabel is the short line in the chat header: where the next message would go. It shows the provider and the model id and
// nothing else: no endpoint, no key, no key source and no path. The model id comes from probe.yaml, so it passes the gate with
// the exact key removed, and it is cut to 200 bytes, like every provider string. err is the refusal aiProvider returned, if any.
func chatLabel(s aiProviderSettings, err error) string { return labelParts(s, err).String() }

// labelParts builds the header line in its three parts. See chatLabel for what it shows.
func labelParts(s aiProviderSettings, err error) ChatLabelParts {
	if err != nil {
		if errors.Is(err, errAIOff) || errors.Is(err, errNoAPIKey) {
			return ChatLabelParts{Prefix: chatLabelOff}
		}
		return ChatLabelParts{Prefix: chatLabelFix}
	}
	switch s.Provider {
	case "anthropic":
		model := s.Model
		if model == "" {
			model = chatModel
		}
		return ChatLabelParts{Prefix: "Anthropic, ", Model: labelModel(model, s.APIKey)}
	case "openai":
		if s.Model == "" {
			return ChatLabelParts{Prefix: "OpenAI"}
		}
		return ChatLabelParts{Prefix: "OpenAI, ", Model: labelModel(s.Model, s.APIKey)}
	case "local":
		where := "remote endpoint"
		if onThisMachine(s.Endpoint) {
			where = "on this machine"
		}
		return ChatLabelParts{Model: labelModel(s.Model, s.APIKey), Suffix: ", " + where}
	}
	return ChatLabelParts{Prefix: chatLabelOff}
}

// AIChatLabel returns the chat header's line. It reads the same resolved settings as Chat and PreviewAIChat, so the header
// names the destination a message would go to, or says that AI is off or needs a fix. It makes no request.
//
// It reads the keychain only when the provider cannot be decided without it, which is when the provider is Anthropic or unset
// and probe.yaml names no key variable. For none, a local endpoint and OpenAI it reads no secret.
func (a *App) AIChatLabel() string { return a.chatLabelParts().String() }

// AIChatLabelParts is AIChatLabel in three parts, for the header, which shortens a long model id and keeps the rest in view.
func (a *App) AIChatLabelParts() ChatLabelParts { return a.chatLabelParts() }

func (a *App) chatLabelParts() ChatLabelParts {
	s, err := a.aiProvider()
	if errors.Is(err, errNoAPIKey) && a.configuredProvider() == "anthropic" {
		return ChatLabelParts{Prefix: chatLabelKey} // Anthropic was chosen, and there is no key yet
	}
	return labelParts(s, err)
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
