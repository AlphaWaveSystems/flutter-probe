// Package sysdialog drives OS-level system dialogs — permission alerts, the
// StoreKit "Sign in to Apple Account" sheet, Android permission controller
// dialogs — which live outside the Flutter widget tree and so are invisible to
// the Dart agent.
//
// iOS is driven through an XCUITest runner app (see ios-driver/) that exposes
// SpringBoard's accessibility tree over a loopback HTTP API; Android through
// uiautomator dumps and `input`. The feature is optional: nothing in a normal
// test run touches this package.
package sysdialog

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/textfold"
)

// Dialog is a snapshot of one system dialog.
type Dialog struct {
	Title   string   `json:"title"`
	Texts   []string `json:"texts"`
	Buttons []string `json:"buttons"`
	Fields  []string `json:"fields"`
	App     string   `json:"app,omitempty"`
}

// Driver is the platform-independent surface used by ProbeScript steps and the
// `probe system-dialog` command. title arguments are case-insensitive
// substring filters on the dialog's text; empty matches any dialog.
type Driver interface {
	Dialogs(ctx context.Context) ([]Dialog, error)
	// See reports whether a dialog matching title is currently shown.
	See(ctx context.Context, title string) (bool, error)
	// Wait blocks until a matching dialog appears (or disappears), up to timeout.
	// It returns whether the awaited state was reached.
	Wait(ctx context.Context, title string, appear bool, timeout time.Duration) (bool, error)
	// Tap taps the button whose label matches button. It returns the tapped label.
	Tap(ctx context.Context, button, title string) (string, error)
	// Type types text into the field matching field. The text is never logged
	// or echoed by the driver.
	Type(ctx context.Context, field, text, title string) error
	// Dismiss taps a cancel-like button. It is a no-op (false, nil) when no
	// dialog is showing, so flows stay idempotent.
	Dismiss(ctx context.Context, title string) (bool, error)
	// Close releases driver resources (stops the iOS runner if this process
	// started it).
	Close() error
}

// normalize folds case, typographic apostrophes and whitespace so that
// "Don’t Allow" and "don't allow" compare equal.
func normalize(s string) string { return textfold.Fold(s) }

// Match returns the index of the best label for wanted: an exact (normalized)
// match first, then a substring match; -1 if none.
func Match(wanted string, labels []string) int {
	w := normalize(wanted)
	if w == "" {
		return -1
	}
	for i, l := range labels {
		if normalize(l) == w {
			return i
		}
	}
	for i, l := range labels {
		if strings.Contains(normalize(l), w) {
			return i
		}
	}
	return -1
}

// MatchesTitle reports whether d matches the title filter.
func (d Dialog) MatchesTitle(title string) bool {
	t := normalize(title)
	if t == "" {
		return true
	}
	for _, s := range d.Texts {
		if strings.Contains(normalize(s), t) {
			return true
		}
	}
	return strings.Contains(normalize(d.Title), t)
}

// dismissLabels are tapped by Dismiss, in preference order.
var dismissLabels = []string{"Cancel", "Don't Allow", "Don't allow", "Not Now", "Close", "Dismiss", "Later", "No Thanks", "Deny"}

// DismissIndex picks the cancel-like button among buttons, or -1.
func DismissIndex(buttons []string) int {
	for _, want := range dismissLabels {
		w := normalize(want)
		for i, b := range buttons {
			if normalize(b) == w {
				return i
			}
		}
	}
	return -1
}

var envRef = regexp.MustCompile(`^\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?$`)

// ResolveSecret turns "$NAME" / "${NAME}" into the value of that environment
// variable. Any other text is returned unchanged. isSecret reports whether
// the value came from the environment (callers mask it either way).
func ResolveSecret(text string) (value string, fromEnv bool, err error) {
	m := envRef.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return text, false, nil
	}
	v, ok := os.LookupEnv(m[1])
	if !ok || v == "" {
		return "", true, fmt.Errorf("environment variable %s is not set (needed for a system-field value)", m[1])
	}
	return v, true, nil
}

// Scrub removes every occurrence of the given secrets from s. Used on error
// strings and anything else that could carry typed text.
func Scrub(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "****")
		}
	}
	return s
}
