package cli

import (
	"os"
	"strings"
	"testing"
)

// probe test copies artifacts only from the folders the run writes to: it passes the video folder, the probelink client's screenshot folder
// and the compare screenshot diff folder to PullArtifacts, and never the folder of the -o file.
func TestProbeTestPassesTheClientScreenshotFolderToPullArtifacts(t *testing.T) {
	src, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`clientShots, _ := filepath.Abs(filepath.Join("reports", "screenshots"))`,
		`visualDiffs, _ := filepath.Abs(filepath.Join("reports", "visual-diff"))`,
		"runner.PullArtifacts(ctx, results, devCtx, screenshotDir, videoDir, clientShots, visualDiffs)",
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("probe test must pass the run's folders to PullArtifacts: %s", want)
		}
	}
	if strings.Contains(string(src), "PullArtifacts(ctx, results, devCtx, screenshotDir, filepath.Dir(outFile)") {
		t.Error("the folder of the -o file must never be an allowed folder")
	}
}
