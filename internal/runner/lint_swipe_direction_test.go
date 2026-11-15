package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// TestLintWarnsOnAnUnknownSwipeDirection checks the warning for a word where a swipe direction goes: the swipe runs down,
// and lint says so on the swipe's line. A real direction, with or without a distance, gets no such warning.
func TestLintWarnsOnAnUnknownSwipeDirection(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		warn bool
	}{
		{"unknown word", "test \"t\"\n  swipe sideways\n", true},
		{"real direction", "test \"t\"\n  swipe up\n", false},
		{"direction with distance", "test \"t\"\n  swipe up over 40 percent\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "swipe.probe")
			if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			findings, err := LintReferences(&config.Config{}, path)
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, f := range findings {
				if f.Warning && f.Line == 2 && strings.Contains(f.Message, "sideways") {
					got = true
				}
			}
			if got != tc.warn {
				t.Errorf("warning = %v, want %v; findings %+v", got, tc.warn, findings)
			}
		})
	}
}
