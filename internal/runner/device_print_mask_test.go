package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// The link and media steps print the URL or path they act on, so a resolved environment value in either is masked before
// the line is printed. A zero-value device context has no platform to act on, so each step prints its line and returns.
func TestOpenDeepLinkAndAddMediaMaskResolvedValues(t *testing.T) {
	const secret = "link-secret-7373"
	sysdialog.NoteSecret(secret)
	dc := &DeviceContext{}
	out := captureDumpOutput(t, func() {
		_ = dc.OpenDeepLink(context.Background(), "https://example.test/cb?code="+secret)
		_ = dc.AddMedia(context.Background(), "/tmp/"+secret+".png")
	})
	if strings.Contains(out, secret) || !strings.Contains(out, "Opening https://example.test/cb?code=****") || !strings.Contains(out, "/tmp/****.png") {
		t.Errorf("a printed link or path carried the resolved value: %q", out)
	}
}
