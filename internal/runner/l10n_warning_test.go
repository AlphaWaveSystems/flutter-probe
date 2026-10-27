package runner

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/l10n"
)

// A key can be built from an environment value. The value is expanded before the key is looked up, so the warning for a
// key that does not exist names the value. The warning goes to stderr, and it must be masked there too.
func TestL10nWarningMasksEnvValueInKey(t *testing.T) {
	setupCatalog(t)
	const name, secret = "PROBE_L10N_WARN_SECRET", "quorvex-secret-123"
	t.Setenv(name, secret)
	e := &Executor{vars: map[string]string{}}

	stderr := captureStderrForTest(t, func() {
		e.resolve(l10n.Marker("save_${" + name + "}"))
	})
	if strings.Contains(stderr, secret) {
		t.Fatalf("the l10n warning printed the environment value: %q", stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "save_****") {
		t.Fatalf("the warning should still name the key with the value masked: %q", stderr)
	}
}

// captureStderrForTest runs fn with os.Stderr redirected to a pipe, and returns what fn wrote to it.
func captureStderrForTest(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stderr
	os.Stderr = w
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	func() {
		defer func() {
			os.Stderr = old
			w.Close()
		}()
		fn()
	}()
	return <-out
}
