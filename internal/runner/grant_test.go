package runner

import (
	"context"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

func TestApplyGrants_NoopWithoutGrantsDryRunOrDeviceContext(t *testing.T) {
	cfg := &config.Config{}
	cases := map[string]RunOptions{
		"no grants":         {Timeout: time.Second},
		"dry run":           {Timeout: time.Second, DryRun: true, Grant: []string{"camera"}},
		"cloud (no device)": {Timeout: time.Second, Grant: []string{"camera"}},
	}
	for name, opts := range cases {
		r := New(cfg, &fakeAIClient{}, nil, opts)
		if err := r.applyGrants(context.Background()); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}
