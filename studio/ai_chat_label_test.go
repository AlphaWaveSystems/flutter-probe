package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The header names the provider and the model id only, for every state, and never an endpoint, a key, a key source or a path.
func TestChatLabelNamesTheProviderAndModelOnly(t *testing.T) {
	const savedKey = "sk" + "-ant-" + "label-probe-000000000060"
	t.Setenv("PROBE_LABEL_KEY", "plainLabelSecretValue0000000061")
	cases := []struct {
		name  string
		yaml  string
		saved string
		want  string
	}{
		{"no block and no key", "agent:\n  port: 9999\n", "", chatLabelOff},
		{"provider none with a saved key", "ai:\n  provider: none\n", savedKey, chatLabelOff},
		{"local on this machine", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: qwen3-coder:30b\n", "", "qwen3-coder:30b, on this machine"},
		{"local on localhost", "ai:\n  provider: local\n  endpoint: http://localhost:11434/v1\n  model: m\n", "", "m, on this machine"},
		{"local at another address, confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\nstudio:\n  ai_allow_remote_endpoint: true\n", "", "m, remote endpoint"},
		{"local at another address, not confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n", "", chatLabelFix},
		{"local with a plain key", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n  api_key: plain-value-000000000062\n", "", chatLabelFix},
		{"openai with a variable", "ai:\n  provider: openai\n  model: gpt-x\n  api_key: ${PROBE_LABEL_KEY}\n", "", "OpenAI, gpt-x"},
		{"anthropic with a saved key", "ai:\n  provider: anthropic\n", savedKey, "Anthropic, " + chatModel},
		{"anthropic with a named model", "ai:\n  provider: anthropic\n  model: claude-x\n", savedKey, "Anthropic, claude-x"},
		{"no provider with a saved key", "agent:\n  port: 9999\n", savedKey, "Anthropic, " + chatModel},
		{"anthropic chosen without a key", "ai:\n  provider: anthropic\n", "", chatLabelKey},
		{"openai with no model", "ai:\n  provider: openai\n  api_key: ${PROBE_LABEL_KEY}\n", "", "OpenAI"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			got := appIn(writeProbeYAML(t, c.yaml)).AIChatLabel()
			if got != c.want {
				t.Fatalf("label = %q, want %q", got, c.want)
			}
			for _, leak := range []string{"127.0.0.1", "localhost", "example.test", "http", "PROBE_LABEL_KEY", "plainLabelSecret", "label-probe", "probe.yaml", "keychain", "/"} {
				if strings.Contains(got, leak) && !strings.Contains(c.want, leak) {
					t.Errorf("the label %q shows %q", got, leak)
				}
			}
		})
	}
}

// A model id from probe.yaml can hold anything, so the label gates it and cuts it like every provider string, on every path that
// shows one: a key-shaped token is redacted, the exact key is removed, and an id of any length comes back at 200 bytes or less.
// An HTML tag stays as text, because the header is set with textContent.
func TestChatLabelGatesAndCutsTheModelID(t *testing.T) {
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	const envKey = "plainLabelSecretValue0000000061"
	savedKey := "sk" + "-ant-" + "label-probe-000000000067"
	paths := []struct {
		name   string
		prefix string
		yaml   func(model string) string
		saved  string
		key    string
	}{
		{"openai", "OpenAI, ", func(m string) string {
			return "ai:\n  provider: openai\n  model: \"" + m + "\"\n  api_key: ${PROBE_LABEL_KEY}\n"
		}, "", envKey},
		{"local", "", func(m string) string {
			return "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: \"" + m + "\"\n  api_key: ${PROBE_LABEL_KEY}\n"
		}, "", envKey},
		{"anthropic", "Anthropic, ", func(m string) string {
			return "ai:\n  provider: anthropic\n  model: \"" + m + "\"\n"
		}, savedKey, savedKey},
	}
	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			isolateProvider(t, p.saved)
			t.Setenv("PROBE_LABEL_KEY", envKey)
			model := "m-" + token + "-<b>bold</b>-" + p.key + "-" + strings.Repeat("z", 400)
			got := appIn(writeProbeYAML(t, p.yaml(model))).AIChatLabel()
			if strings.Contains(got, "ghp_") || strings.Contains(got, p.key[:12]) {
				t.Fatalf("the label shows a token or the key: %q", got)
			}
			if !strings.Contains(got, "<b>bold</b>") {
				t.Fatalf("the tag was changed instead of left as text: %q", got)
			}
			maxLen := len(p.prefix) + 200
			if p.name == "local" {
				maxLen += len(", on this machine")
			}
			if len(got) > maxLen {
				t.Fatalf("the label is %d bytes, want at most %d", len(got), maxLen)
			}
		})
	}
}

// A character that can scramble or spoof the label is shown as ?, for each range: control characters, line and paragraph
// separators, bidirectional controls and zero-width characters. A comma too, so a model id cannot read as a place.
func TestChatLabelReplacesCharactersThatSpoof(t *testing.T) {
	var runes []rune
	runes = append(runes, 0x01, 0x1F, 0x7F, 0x80, 0x85, 0x9F, 0x2028, 0x2029, 0x200E, 0x200F, 0x061C, 0x2060, 0xFEFF, ',')
	for r := rune(0x202A); r <= 0x202E; r++ {
		runes = append(runes, r)
	}
	for r := rune(0x2066); r <= 0x2069; r++ {
		runes = append(runes, r)
	}
	for r := rune(0x200B); r <= 0x200D; r++ {
		runes = append(runes, r)
	}
	for _, r := range runes {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			isolateProvider(t, "")
			dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: \"a\\u"+fmt.Sprintf("%04X", r)+"b\"\n")
			got := appIn(dir).AIChatLabel()
			if got != "a?b, on this machine" {
				t.Fatalf("label = %q, want the character shown as ?", got)
			}
		})
	}
}

// A model id that reads like a place does not: on a confirmed remote endpoint "x, on this machine" is shown with its comma
// replaced, so the only place named is the real one.
func TestChatLabelModelCannotNameAPlace(t *testing.T) {
	isolateProvider(t, "")
	dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: \"x, on this machine\"\nstudio:\n  ai_allow_remote_endpoint: true\n")
	if got := appIn(dir).AIChatLabel(); got != "x? on this machine, remote endpoint" {
		t.Fatalf("label = %q", got)
	}
}

// The label reads no secret unless the provider cannot be decided without one. For none, a local endpoint and OpenAI it makes no
// keychain lookup at all, so showing the header never prompts for keychain access.
func TestChatLabelReadsNoKeychainWhenTheProviderIsDecidedWithoutAKey(t *testing.T) {
	t.Setenv("PROBE_LABEL_KEY", "plainLabelSecretValue0000000061")
	lookups := 0
	cases := []struct {
		name  string
		yaml  string
		reads bool // the provider cannot be decided without the keychain
	}{
		{"provider none", "ai:\n  provider: none\n", false},
		{"local on this machine", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n", false},
		{"local, remote confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\nstudio:\n  ai_allow_remote_endpoint: true\n", false},
		{"openai with a variable", "ai:\n  provider: openai\n  model: m\n  api_key: ${PROBE_LABEL_KEY}\n", false},
		{"anthropic with a variable", "ai:\n  provider: anthropic\n  api_key: ${PROBE_LABEL_KEY}\n", false},
		{"anthropic with no variable", "ai:\n  provider: anthropic\n", true},
		{"no provider", "agent:\n  port: 9999\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, "")
			apiKeyLookup = func() (string, error) { lookups++; return "", nil }
			lookups = 0
			appIn(writeProbeYAML(t, c.yaml)).AIChatLabel()
			if c.reads && lookups == 0 {
				t.Fatalf("the provider needs the keychain, yet it was not read, so this test cannot tell")
			}
			if !c.reads && lookups != 0 {
				t.Fatalf("the label read the keychain %d time(s) for a provider decided without a key", lookups)
			}
		})
	}
}

// The label and the preview agree for every provider state: both come from the same resolved settings, so the header can never
// claim a destination the request does not use. A state that the settings refuse is refused by both.
func TestChatLabelAgreesWithThePreview(t *testing.T) {
	const savedKey = "sk" + "-ant-" + "label-probe-000000000063"
	t.Setenv("PROBE_LABEL_KEY", "plainLabelSecretValue0000000064")
	cases := []struct {
		name  string
		yaml  string
		saved string
	}{
		{"no block and no key", "agent:\n  port: 9999\n", ""},
		{"provider none", "ai:\n  provider: none\n", savedKey},
		{"local on this machine", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: qwen3-coder:30b\n", ""},
		{"local at another address, confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\nstudio:\n  ai_allow_remote_endpoint: true\n", ""},
		{"local at another address, not confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n", ""},
		{"local with a plain key", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n  api_key: plain-value-000000000065\n", ""},
		{"openai with a variable", "ai:\n  provider: openai\n  model: gpt-x\n  api_key: ${PROBE_LABEL_KEY}\n", ""},
		{"openai with a plain key", "ai:\n  provider: openai\n  model: gpt-x\n  api_key: plain-value-000000000066\n", ""},
		{"anthropic with a saved key", "ai:\n  provider: anthropic\n  model: claude-x\n", savedKey},
		{"anthropic without a key", "ai:\n  provider: anthropic\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			app := appIn(writeProbeYAML(t, c.yaml))
			label := app.AIChatLabel()
			preview, perr := app.PreviewAIChat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
			s, rerr := app.aiProvider()
			if (perr == nil) != (rerr == nil) {
				t.Fatalf("the preview returned %v and the settings returned %v: they must agree", perr, rerr)
			}
			if rerr != nil {
				wantOff := errors.Is(rerr, errAIOff) || errors.Is(rerr, errNoAPIKey)
				want := chatLabelFix
				if wantOff {
					want = chatLabelOff
				}
				if errors.Is(rerr, errNoAPIKey) && app.configuredProvider() == "anthropic" {
					want = chatLabelKey
				}
				if label != want {
					t.Fatalf("the settings refuse (%v) and the label is %q, want %q", rerr, label, want)
				}
				return
			}
			first := strings.SplitN(preview, "\n", 2)[0]
			switch s.Provider {
			case "anthropic":
				if !strings.Contains(first, "Anthropic API") || !strings.HasPrefix(label, "Anthropic, ") {
					t.Fatalf("preview %q and label %q name different destinations", first, label)
				}
			case "openai":
				if !strings.Contains(first, "OpenAI API") || !strings.HasPrefix(label, "OpenAI, ") {
					t.Fatalf("preview %q and label %q name different destinations", first, label)
				}
			case "local":
				here := strings.Contains(first, "127.0.0.1")
				if !strings.Contains(first, "local endpoint") || here != strings.HasSuffix(label, ", on this machine") || !here != strings.HasSuffix(label, ", remote endpoint") {
					t.Fatalf("preview %q and label %q disagree about where the message goes", first, label)
				}
			}
			model := s.Model
			if model == "" {
				model = chatModel
			}
			if !strings.Contains(first, model) || !strings.Contains(label, model) {
				t.Fatalf("preview %q and label %q do not both name the model %q", first, label, model)
			}
		})
	}
}

// onThisMachine is true for this machine only, for hostile look-alikes too, so the header never calls an address local that the
// settings refuse as not local.
func TestOnThisMachineIsLoopbackOnly(t *testing.T) {
	cases := map[string]bool{
		"http://127.0.0.1:11434/v1":         true,
		"http://localhost:11434/v1":         true,
		"http://[::1]:11434/v1":             true,
		"http://LOCALHOST/v1":               true,
		"https://127.0.0.1:8443/v1":         true,
		"http://localhost.evil.test/v1":     false,
		"http://127.0.0.1.evil.test/v1":     false,
		"http://0.0.0.0:11434/v1":           false,
		"http://[::]:11434/v1":              false,
		"http://foo.localhost/v1":           false,
		"http://example.test/v1":            false,
		"http://127.0.0.1@evil.test/v1":     false,
		"http://user:pw@127.0.0.1:11434/v1": false,
		"ftp://127.0.0.1/v1":                false,
		"http://":                           false,
		"not a url":                         false,
		"":                                  false,
	}
	for endpoint, want := range cases {
		if got := onThisMachine(endpoint); got != want {
			t.Errorf("onThisMachine(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

// More characters that look blank or hide text are shown as ?: braille blank and Hangul fillers, variation selectors, tag
// characters, the soft hyphen, the Mongolian vowel separator, the combining grapheme joiner, the deprecated format controls and
// the interlinear annotation marks. One case for each, and for both ends of each range.
func TestChatLabelReplacesMoreInvisibleCharacters(t *testing.T) {
	runes := []rune{0x2800, 0x3164, 0x115F, 0x1160, 0xFFA0, 0xFE00, 0xFE0F, 0xE0100, 0xE01EF, 0xE0000, 0xE007F, 0x00AD, 0x180E, 0x034F}
	for r := rune(0x206A); r <= 0x206F; r++ {
		runes = append(runes, r)
	}
	for r := rune(0xFFF9); r <= 0xFFFB; r++ {
		runes = append(runes, r)
	}
	for _, r := range runes {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			isolateProvider(t, "")
			escape := fmt.Sprintf("\\u%04X", r)
			if r > 0xFFFF {
				escape = fmt.Sprintf("\\U%08X", r)
			}
			dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: \"a"+escape+"b\"\n")
			if got := appIn(dir).AIChatLabel(); got != "a?b, on this machine" {
				t.Fatalf("label = %q, want the character shown as ?", got)
			}
		})
	}
}

// The replacement runs after the gate. An exact key that holds a comma and a control character is removed first; replacing
// the characters before the gate would turn the key into text that no longer matches it, and its pieces would be shown.
func TestChatLabelReplacesAfterTheGate(t *testing.T) {
	isolateProvider(t, "")
	const key = "sec,ret\x01tail0000000075"
	t.Setenv("PROBE_LABEL_KEY", key)
	dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: \"m-sec,ret\\x01tail0000000075-x\"\n  api_key: ${PROBE_LABEL_KEY}\n")
	got := appIn(dir).AIChatLabel()
	if strings.Contains(got, "sec") || strings.Contains(got, "tail0000") {
		t.Fatalf("a piece of the key is shown: %q", got)
	}
	if got != "m-[redacted]-x, on this machine" {
		t.Fatalf("label = %q", got)
	}
}

// The wording when Anthropic is chosen without a key is pinned as text, not through the constant, so a change to it is a decision.
func TestChatLabelKeyNeededWordingIsPinned(t *testing.T) {
	if chatLabelKey != "Anthropic, key needed" {
		t.Fatalf("chatLabelKey = %q", chatLabelKey)
	}
	isolateProvider(t, "")
	if got := appIn(writeProbeYAML(t, "ai:\n  provider: anthropic\n")).AIChatLabel(); got != "Anthropic, key needed" {
		t.Fatalf("label = %q", got)
	}
}

// The three parts join to the whole line, and only the model part comes from probe.yaml: the page shortens it and keeps the rest.
func TestChatLabelPartsJoinToTheLabel(t *testing.T) {
	const savedKey = "sk" + "-ant-" + "label-probe-000000000068"
	cases := []struct {
		name   string
		yaml   string
		saved  string
		prefix string
		model  string
		suffix string
	}{
		{"local on this machine", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: qwen3-coder:30b\n", "", "", "qwen3-coder:30b", ", on this machine"},
		{"local, remote confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\nstudio:\n  ai_allow_remote_endpoint: true\n", "", "", "m", ", remote endpoint"},
		{"anthropic", "ai:\n  provider: anthropic\n  model: claude-x\n", savedKey, "Anthropic, ", "claude-x", ""},
		{"ai off", "ai:\n  provider: none\n", "", chatLabelOff, "", ""},
		{"a refused configuration", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n", "", chatLabelFix, "", ""},
		{"anthropic without a key", "ai:\n  provider: anthropic\n", "", "Anthropic, key needed", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			app := appIn(writeProbeYAML(t, c.yaml))
			got := app.AIChatLabelParts()
			if got.Prefix != c.prefix || got.Model != c.model || got.Suffix != c.suffix {
				t.Fatalf("parts = %+v, want %q %q %q", got, c.prefix, c.model, c.suffix)
			}
			if got.String() != app.AIChatLabel() {
				t.Fatalf("the parts join to %q, the label is %q", got.String(), app.AIChatLabel())
			}
		})
	}
}
