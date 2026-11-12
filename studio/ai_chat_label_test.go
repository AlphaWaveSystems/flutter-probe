package main

import (
	"errors"
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

// A model id from probe.yaml can hold anything, so the label gates it and cuts it like every provider string: a key-shaped
// token is redacted, the exact key is removed, and an id of any length comes back at 200 bytes or less. An HTML tag stays as
// text, because the header is set with textContent.
func TestChatLabelGatesAndCutsTheModelID(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	t.Setenv("PROBE_LABEL_KEY", "plainLabelSecretValue0000000061")
	model := "m-" + token + "-<b>bold</b>-plainLabelSecretValue0000000061-" + strings.Repeat("z", 400)
	dir := writeProbeYAML(t, "ai:\n  provider: openai\n  model: \""+model+"\"\n  api_key: ${PROBE_LABEL_KEY}\n")
	got := appIn(dir).AIChatLabel()
	if strings.Contains(got, "ghp_") || strings.Contains(got, "plainLabelSecret") {
		t.Fatalf("the label shows a token or the key: %q", got)
	}
	if !strings.Contains(got, "<b>bold</b>") {
		t.Fatalf("the tag was changed instead of left as text: %q", got)
	}
	if want := len("OpenAI, ") + 200; len(got) > want {
		t.Fatalf("the label is %d bytes, want at most %d", len(got), want)
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
				if wantOff && label != chatLabelOff || !wantOff && label != chatLabelFix {
					t.Fatalf("the settings refuse (%v) and the label is %q", rerr, label)
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
