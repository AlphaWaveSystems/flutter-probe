package ios

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	tokenA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tokenB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// simulatorWith makes a simulator data folder under a temporary HOME, with the data containers named in tokens (name -> token text; an
// empty text means a container with no token), and returns the simulator's udid.
func simulatorWith(t *testing.T, tokens map[string]string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	const udid = "00000000-0000-0000-0000-000000000001"
	base := filepath.Join(home, "Library", "Developer", "CoreSimulator", "Devices", udid, "data", "Containers", "Data", "Application")
	for name, token := range tokens {
		dir := filepath.Join(base, name, "tmp", "probe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if token != "" {
			if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return udid
}

func readToken(s *SimCtl, udid string, bundle ...string) (string, error) {
	return s.ReadToken(context.Background(), udid, 300*time.Millisecond, bundle...)
}

// With no app id, one container with a token is the token, and none is "not found" (not a guess).
func TestReadTokenWithNoAppIDNeedsExactlyOneContainer(t *testing.T) {
	// zero containers with a token
	udid := simulatorWith(t, map[string]string{"container-1": "", "container-2": ""})
	if tok, err := readToken(New(), udid); err == nil || errors.Is(err, ErrAmbiguousToken) || tok != "" {
		t.Errorf("zero: %q %v", tok, err)
	}
	// one
	udid = simulatorWith(t, map[string]string{"container-1": tokenA, "container-2": ""})
	if tok, err := readToken(New(), udid); err != nil || tok != tokenA {
		t.Errorf("one: %q %v", tok, err)
	}
	// two
	udid = simulatorWith(t, map[string]string{"container-1": tokenA, "container-2": tokenB})
	tok, err := readToken(New(), udid)
	if !errors.Is(err, ErrAmbiguousToken) || tok != "" {
		t.Fatalf("two: %q %v", tok, err)
	}
	if strings.Contains(err.Error(), tokenA) || strings.Contains(err.Error(), tokenB) {
		t.Errorf("the error shows a token: %v", err)
	}
	if !strings.Contains(err.Error(), "app.id") && !strings.Contains(err.Error(), "probe.yaml") {
		t.Errorf("the error does not say what to do: %v", err)
	}
	// a file too short to be a token does not count as a second match
	udid = simulatorWith(t, map[string]string{"container-1": tokenA, "container-2": "short"})
	if tok, err := readToken(New(), udid); err != nil || tok != tokenA {
		t.Errorf("a short file: %q %v", tok, err)
	}
	// three
	udid = simulatorWith(t, map[string]string{"container-1": tokenA, "container-2": tokenB, "container-3": strings.Repeat("c", 32)})
	if _, err := readToken(New(), udid); !errors.Is(err, ErrAmbiguousToken) || !strings.Contains(err.Error(), "(3)") {
		t.Errorf("three: %v", err)
	}
}

// With an app id, that app's container is read first, whatever else is on the simulator, and the refusal does not apply.
func TestReadTokenWithAnAppIDReadsThatContainerFirst(t *testing.T) {
	udid := simulatorWith(t, map[string]string{"container-1": tokenA, "container-2": tokenB})
	s := New()
	root := filepath.Join(os.Getenv("HOME"), "Library", "Developer", "CoreSimulator", "Devices", udid, "data", "Containers", "Data", "Application")
	s.containerOf = func(_ context.Context, _, bundle string) string {
		if bundle == "app.two" {
			return filepath.Join(root, "container-2")
		}
		if bundle == "app.one" {
			return filepath.Join(root, "container-1")
		}
		return ""
	}
	if tok, err := readToken(s, udid, "app.two"); err != nil || tok != tokenB {
		t.Errorf("app two: %q %v", tok, err)
	}
	if tok, err := readToken(s, udid, "app.one"); err != nil || tok != tokenA {
		t.Errorf("app one: %q %v", tok, err)
	}
	// an empty app id is no app id
	if _, err := readToken(s, udid, ""); !errors.Is(err, ErrAmbiguousToken) {
		t.Errorf("an empty id: %v", err)
	}
}

func TestCountValidTokens(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(text), 0o644)
		return p
	}
	paths := []string{write("a", tokenA), write("b", "short"), write("c", "  "+tokenB+"\n"), filepath.Join(dir, "missing")}
	if n := countValidTokens(paths); n != 2 {
		t.Errorf("%d valid tokens", n)
	}
}
