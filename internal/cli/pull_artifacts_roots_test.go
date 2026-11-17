package cli

import (
	"os"
	"strings"
	"testing"
)

// probe test copies artifacts only from the folders the run writes to: it passes the probelink client's own screenshot folder to
// PullArtifacts (the reports folder, screenshotDir and the videos under it are PullArtifacts' own roots).
func TestProbeTestPassesTheClientScreenshotFolderToPullArtifacts(t *testing.T) {
	src, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `clientShots, _ := filepath.Abs(filepath.Join("reports", "screenshots"))`) ||
		!strings.Contains(string(src), "runner.PullArtifacts(ctx, results, devCtx, screenshotDir, clientShots)") {
		t.Error("probe test must pass ./reports/screenshots to PullArtifacts as an allowed folder")
	}
}
