package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// AISettings is the provider form in Settings. It maps onto probe.yaml's ai: block and studio.ai_allow_remote_endpoint.
// The key is not a field: KeySource says where it comes from, and KeyEnvName names the variable that holds it.
type AISettings struct {
	Provider            string `json:"provider"`            // "", "anthropic", "local" or "openai"; empty means AI is off
	Endpoint            string `json:"endpoint"`            // OpenAI-compatible base URL, for "local" only
	Model               string `json:"model"`               // required for "local"
	KeySource           string `json:"keySource"`           // "keychain" (the saved Anthropic key), "env", or "file" (a plain key already in probe.yaml)
	KeyEnvName          string `json:"keyEnvName"`          // the environment variable, when KeySource is "env"
	Vision              bool   `json:"vision"`              // false for a text-only model; on unless probe.yaml says otherwise
	Timeout             string `json:"timeout"`             // a Go duration such as "60s"; empty means the default
	AllowRemoteEndpoint bool   `json:"allowRemoteEndpoint"` // confirms a local endpoint that is not this machine
	PlainKeyInFile      bool   `json:"plainKeyInFile"`      // read-only: probe.yaml holds a plain key, which Studio refuses to use
}

var (
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	aiProviderKind = map[string]bool{"": true, "none": true, "anthropic": true, "local": true, "openai": true}
)

// envRef returns the variable name of a ${NAME} reference.
func envRef(raw string) (string, bool) {
	if strings.HasPrefix(raw, "${") && strings.HasSuffix(raw, "}") {
		// One variable only: ${A}-${B} and ${A}${B} are not a reference.
		if name := raw[2 : len(raw)-1]; envNamePattern.MatchString(name) {
			return name, true
		}
	}
	return "", false
}

// readAISettings reads the provider form from probe.yaml. A missing file means AI is off, with the key in the keychain and
// vision on. A plain key in the file is reported as PlainKeyInFile and never returned.
func readAISettings(workspace string) (AISettings, error) {
	s := AISettings{KeySource: "keychain", Vision: true}
	data, err := os.ReadFile(filepath.Join(workspace, "probe.yaml"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("reading probe.yaml: %w", err)
	}
	var doc struct {
		AI struct {
			Provider string `yaml:"provider"`
			Endpoint string `yaml:"endpoint"`
			Model    string `yaml:"model"`
			APIKey   string `yaml:"api_key"`
			Vision   *bool  `yaml:"vision"`
			Timeout  string `yaml:"timeout"`
		} `yaml:"ai"`
		Studio struct {
			AllowRemoteEndpoint bool `yaml:"ai_allow_remote_endpoint"`
		} `yaml:"studio"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return s, fmt.Errorf("parsing probe.yaml: %w", err)
	}
	s.Provider = doc.AI.Provider
	s.Endpoint = doc.AI.Endpoint
	s.Model = doc.AI.Model
	s.Timeout = doc.AI.Timeout
	if doc.AI.Vision != nil {
		s.Vision = *doc.AI.Vision
	}
	s.AllowRemoteEndpoint = doc.Studio.AllowRemoteEndpoint
	if name, ok := envRef(doc.AI.APIKey); ok {
		s.KeySource = "env"
		s.KeyEnvName = name
	} else if doc.AI.APIKey != "" {
		// A plain key is its own state, so a form that was loaded from such a file is valid when it is saved unchanged.
		s.KeySource = "file"
		s.PlainKeyInFile = true
	}
	return s, nil
}

// validate refuses a form it can tell would not run: an unknown provider, a bad variable name or timeout, a provider that
// needs a model, an endpoint or a variable and has none, and a local endpoint that is not allowed. It cannot know whether
// a variable is set or a server is up, so a saved file can still fail when it is used; Test connection says why.
func (s AISettings) validate() error {
	if !aiProviderKind[s.Provider] {
		return fmt.Errorf("provider %q is not one of anthropic, local or openai", s.Provider)
	}
	switch s.KeySource {
	case "keychain", "env", "file":
	default:
		return fmt.Errorf("key source %q is not one of keychain, env or file", s.KeySource)
	}
	if s.KeySource == "env" && !envNamePattern.MatchString(s.KeyEnvName) {
		return fmt.Errorf("the key variable name must be letters, digits and underscores, and must not start with a digit")
	}
	if s.Provider == "openai" && s.KeySource == "keychain" {
		// Studio sends a saved keychain key only to the Anthropic provider, so openai needs a variable to run.
		return fmt.Errorf("provider openai needs the key source Environment variable: Studio sends a saved keychain key only to Anthropic")
	}
	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return fmt.Errorf("timeout %q is not a Go duration: %w", s.Timeout, err)
		}
	}
	if s.Provider == "local" {
		if s.Model == "" {
			return fmt.Errorf("provider local needs a model name")
		}
		if s.Endpoint == "" {
			return fmt.Errorf("provider local needs an endpoint, such as http://127.0.0.1:11434/v1")
		}
		// A ${NAME} endpoint is resolved and checked when a request is made, because the variable may be set only in the
		// shell that launches Studio.
		if _, isRef := envRef(s.Endpoint); !isRef {
			if err := checkLocalEndpoint(s.Endpoint, s.AllowRemoteEndpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeAISettings writes the provider form into probe.yaml. Only the managed keys change, so the user's other keys
// (redact rules, device IDs, and the rest) are kept. A key that is not a map is refused rather than overwritten.
func writeAISettings(workspace string, s AISettings) error {
	if workspace == "" {
		return fmt.Errorf("workspace path is required")
	}
	if err := s.validate(); err != nil {
		return err
	}
	// The edit is made on the file's own text (probe_yaml_edit.go): comments, blank lines and every key the form does not manage stay exactly
	// as they were, only a changed line is touched, and the file is replaced atomically with its mode. A new probe.yaml is private to the
	// user (0600); an existing one keeps its own mode. A key that is not a map is refused rather than overwritten.
	probeYAMLMu.Lock()
	defer probeYAMLMu.Unlock()
	y, err := openProbeYAML(workspace)
	if err != nil {
		return err
	}
	y.check = func(text string) error {
		var cfg config.Config
		if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
			return fmt.Errorf("the edited probe.yaml would not load (%v), so nothing was written", err)
		}
		return nil
	}
	set := func(value string, path ...string) error {
		if strings.TrimSpace(value) == "" {
			return y.remove(path...)
		}
		return y.set(value, path...)
	}
	endpoint := ""
	if s.Provider == "local" {
		endpoint = s.Endpoint
	}
	for _, e := range []struct {
		value string
		key   string
	}{{s.Provider, "provider"}, {endpoint, "endpoint"}, {s.Model, "model"}, {s.Timeout, "timeout"}} {
		if err := set(e.value, "ai", e.key); err != nil {
			return err
		}
	}
	// The key line changes only when the form asks for it. A form that names an environment variable writes that reference. A
	// form that uses the keychain removes an existing reference, because the person switched the source, and leaves a plain
	// key alone: Studio never reads a plain key, but the CLI does, and deleting it would break a setup Save did not make.
	existingKey, _ := y.current("ai", "api_key")
	_, existingIsRef := envRef(existingKey)
	switch {
	case s.KeySource == "env":
		if err := set("${"+s.KeyEnvName+"}", "ai", "api_key"); err != nil {
			return err
		}
	case existingKey == "" || existingIsRef:
		if err := set("", "ai", "api_key"); err != nil {
			return err
		}
	}
	// Otherwise the file holds a plain key and the form names the keychain or the file: the line stays as it is.
	if s.Vision {
		if cur, ok := y.current("ai", "vision"); ok && cur == "false" {
			if err := y.remove("ai", "vision"); err != nil {
				return err
			}
		}
	} else if err := y.set(false, "ai", "vision"); err != nil {
		return err
	}
	if s.AllowRemoteEndpoint {
		err = y.set(true, "studio", "ai_allow_remote_endpoint")
	} else {
		err = y.remove("studio", "ai_allow_remote_endpoint")
	}
	if err != nil {
		return err
	}
	return y.save()
}

// LoadAISettings returns the provider form for a workspace's probe.yaml.
func (a *App) LoadAISettings(workspace string) (AISettings, error) {
	if workspace == "" {
		return AISettings{}, fmt.Errorf("workspace path is required")
	}
	return readAISettings(workspace)
}

// SaveAISettings writes the provider form into a workspace's probe.yaml. It never writes a key: a key stays in the keychain
// or in the environment.
func (a *App) SaveAISettings(workspace string, s AISettings) error {
	if workspace == "" {
		return fmt.Errorf("workspace path is required") // before the compare: an empty path would read ./probe.yaml
	}
	if err := a.requireOpenWorkspace(workspace); err != nil {
		return err
	}
	// A form that equals what the file already says changes nothing, so it is neither validated nor written. A setup the CLI
	// accepts, such as OpenAI with a plain key, must not stop the rest of Settings from saving.
	if cur, err := readAISettings(workspace); err == nil {
		s.PlainKeyInFile = cur.PlainKeyInFile // read-only: the form cannot change it
		if s == cur {
			return nil
		}
	}
	return writeAISettings(workspace, s)
}

// AIConnectionCheck is one line of the connection test.
type AIConnectionCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// AIConnectionReport is the outcome of the connection test. OK means text features work. Vision is reported apart, because
// a text-only model is a supported setup.
type AIConnectionReport struct {
	OK            bool                `json:"ok"`
	Vision        bool                `json:"vision"`
	VisionChecked bool                `json:"visionChecked"`
	Checks        []AIConnectionCheck `json:"checks"`
}

// runDoctor is ai.Doctor. A test replaces it to hand back details no cleaning has touched, so what Studio does with them is
// tested apart from what internal/ai does.
var runDoctor = ai.Doctor

// TestAIConnection runs the checks of `probe ai doctor` against the saved provider. It sends a fixed prompt and a fixed
// one-pixel image, and never any file or conversation text. With AI off it fails before any request.
func (a *App) TestAIConnection() (AIConnectionReport, error) {
	s, err := a.aiProvider()
	if err != nil {
		return AIConnectionReport{}, err
	}
	r := runDoctor(context.Background(), ai.DoctorInput{
		Provider: s.Provider,
		APIKey:   s.APIKey,
		Model:    s.Model,
		Endpoint: s.Endpoint,
		Timeout:  s.Timeout,
	})
	out := AIConnectionReport{
		OK:            r.OK(),
		Vision:        r.Vision,
		VisionChecked: r.VisionChecked,
		Checks:        make([]AIConnectionCheck, 0, len(r.Checks)),
	}
	// A server can echo the request in an error body, so what the doctor reports passes the same gate as everything else that
	// comes back from a provider.
	for _, c := range r.Checks {
		out.Checks = append(out.Checks, AIConnectionCheck{Name: c.Name, OK: c.OK, Detail: cleanProviderText(c.Detail, s.APIKey, false)})
	}
	return out, nil
}

// PreviewAIChat returns the text the next chat turn would send, after the outbound gate, and the provider it would go to.
// It sends nothing. The preview and the request each build the turn when they run, with the same chatTurn and the same gate,
// so the preview shows what the message would send now. A change between the preview and Send, such as a new run result or
// a different widget tree, can make the request differ from what was shown.
func (a *App) PreviewAIChat(messages []ChatMessage, fileContent string) (string, error) {
	s, err := a.aiProvider()
	if err != nil {
		return "", err
	}
	system, msgs := a.chatTurn(messages, fileContent)
	system, msgs = gateTurn(s, system, msgs) // the one path the request takes
	var b strings.Builder
	fmt.Fprintf(&b, "Sent to: %s\n\n", providerLabel(s))
	b.WriteString("System prompt and context:\n")
	b.WriteString(system)
	b.WriteString("\n\nConversation:\n")
	if omitted := len(messages) - len(msgs); omitted > 0 {
		fmt.Fprintf(&b, "(the %d oldest messages are not sent: a turn carries up to the newest %d)\n", omitted, maxChatMessages)
	}
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
	}
	return b.String(), nil
}

// providerLabel names the provider a request goes to. An endpoint is shown without any user:password part.
func providerLabel(s aiProviderSettings) string {
	switch s.Provider {
	case "anthropic":
		model := s.Model
		if model == "" {
			model = chatModel
		}
		return "Anthropic API, model " + model
	case "local":
		endpoint := s.Endpoint
		if u, err := url.Parse(s.Endpoint); err == nil {
			endpoint = u.Redacted()
		}
		return "local endpoint " + endpoint + ", model " + s.Model
	case "openai":
		return "OpenAI API, model " + s.Model
	}
	return "nothing: AI is off"
}
