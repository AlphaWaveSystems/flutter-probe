package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

func writeProbeFile(t *testing.T, content string) []string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return []string{path}
}

const aiTestProbeSrc = `test "t"
  see "x" with ai
`

func TestValidateAIConfig_NoAIUsage_Passes(t *testing.T) {
	files := writeProbeFile(t, `test "t"
  see "Dashboard"
`)
	if err := validateAIConfig(files, &config.Config{}); err != nil {
		t.Errorf("expected no error for a test that doesn't use AI, got: %v", err)
	}
}

func TestValidateAIConfig_NoProvider_FailsFast(t *testing.T) {
	files := writeProbeFile(t, aiTestProbeSrc)
	err := validateAIConfig(files, &config.Config{})
	if err == nil {
		t.Fatal("expected an error when ai: isn't configured at all")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error should say ai isn't configured, got: %v", err)
	}
}

func TestValidateAIConfig_CloudProviderWithoutKey_FailsFast(t *testing.T) {
	files := writeProbeFile(t, aiTestProbeSrc)
	cfg := &config.Config{AI: config.AIConfig{Provider: "anthropic"}}
	if err := validateAIConfig(files, cfg); err == nil {
		t.Fatal("expected an error when a cloud provider is set without an api_key")
	}
}

func TestValidateAIConfig_LocalProviderWithoutEndpoint_FailsFast(t *testing.T) {
	files := writeProbeFile(t, aiTestProbeSrc)
	cfg := &config.Config{AI: config.AIConfig{Provider: "local"}}
	err := validateAIConfig(files, cfg)
	if err == nil {
		t.Fatal("expected an error when provider: local is set without ai.endpoint")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error should mention ai.endpoint, got: %v", err)
	}
}

func TestValidateAIConfig_LocalProviderWithEndpoint_Passes(t *testing.T) {
	files := writeProbeFile(t, aiTestProbeSrc)
	cfg := &config.Config{AI: config.AIConfig{Provider: "local", Endpoint: "http://localhost:11434/v1", Model: "llava"}}
	if err := validateAIConfig(files, cfg); err != nil {
		t.Errorf("expected no error for a fully-configured provider: local, got: %v", err)
	}
}

func TestValidateAIConfig_CloudProviderWithKey_Passes(t *testing.T) {
	files := writeProbeFile(t, aiTestProbeSrc)
	cfg := &config.Config{AI: config.AIConfig{Provider: "anthropic", APIKey: "sk-x"}}
	if err := validateAIConfig(files, cfg); err != nil {
		t.Errorf("expected no error for a fully-configured cloud provider, got: %v", err)
	}
}
