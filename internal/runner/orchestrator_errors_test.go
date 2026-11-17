package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// A lane's error can carry text from the device or the agent. When the connect fails with such text (a new line with a forged
// "pass --yes" line, a screen clear, a bidi override), the retry line, the failure line, the device summary and the run's error show it
// as plain text on one line.
func TestFailedLaneErrorTextIsPrintedCleaned(t *testing.T) {
	dir := t.TempDir()
	f := writeHarnessFile(t, dir, "a.probe", "test \"a\"\n  tap \"Fine\"\n")
	h := newHarnessRun(t, harnessConfig(), [][]string{{f}})
	hostile := "refused\n  pass --yes to continue\x1b[2J" + string(rune(0x202e)) + "end"
	h.po.dial = func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
		return nil, errors.New(hostile)
	}
	var runErr error
	out := captureStdout(t, func() {
		_, runErr = h.po.Run(context.Background())
		h.po.PrintSummary()
	})
	if runErr == nil {
		t.Fatal("want the run's error")
	}
	for what, text := range map[string]string{"output": out, "run error": runErr.Error()} {
		if strings.Count(text, "refused pass --yes to continue [2J end") == 0 {
			t.Errorf("%s: the error's visible text must be shown on one line: %q", what, text)
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
				t.Errorf("%s: an error forged its own line: %q", what, line)
			}
			label := line
			for _, code := range []string{"\033[32m", "\033[33m", "\033[31m", "\033[0m", "\033[1m"} {
				label = strings.ReplaceAll(label, code, "")
			}
			if r, bad := unsafeInLine(label); bad {
				t.Errorf("%s: a line prints %U from the error: %q", what, r, line)
			}
		}
	}
	for _, want := range []string{"attempt 1 failed", "all attempts failed", "error: "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}
