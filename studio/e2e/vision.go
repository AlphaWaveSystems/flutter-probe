package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

// Vision is the optional screenshot judge, used only where a check cannot
// be expressed against the accessibility tree. It is configured from the
// same environment a `probe.yaml` ai: block would use:
//
//	STUDIO_E2E_AI_PROVIDER   openai | anthropic | local (unset → vision checks are skipped)
//	STUDIO_E2E_AI_MODEL      model name (required for local)
//	STUDIO_E2E_AI_ENDPOINT   OpenAI-compatible endpoint for local
//	OPENAI_API_KEY / ANTHROPIC_API_KEY
type Vision struct {
	provider ai.VisionProvider
}

// NewVisionFromEnv returns nil (and no error) when no provider is configured.
func NewVisionFromEnv() (*Vision, error) {
	provider := os.Getenv("STUDIO_E2E_AI_PROVIDER")
	if provider == "" {
		return nil, nil
	}
	key := os.Getenv("OPENAI_API_KEY")
	if provider == "anthropic" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	p, err := ai.NewVisionProvider(provider, key, os.Getenv("STUDIO_E2E_AI_MODEL"), os.Getenv("STUDIO_E2E_AI_ENDPOINT"), 90*time.Second)
	if err != nil {
		return nil, err
	}
	return &Vision{provider: p}, nil
}

// Assert judges a screenshot file against a plain-English assertion.
func (v *Vision) Assert(ctx context.Context, shotPath, assertion string) error {
	img, err := os.ReadFile(shotPath)
	if err != nil {
		return err
	}
	verdict, err := v.provider.AssertScreen(ctx, img, assertion)
	if err != nil {
		return fmt.Errorf("vision: %w", err)
	}
	if !verdict.True {
		return fmt.Errorf("vision assertion failed: %q — %s", assertion, verdict.Reasoning)
	}
	return nil
}
