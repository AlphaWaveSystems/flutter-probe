package sysdialog

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SandboxOptions configures SignInSandbox.
type SandboxOptions struct {
	User, Password string
	// WaitForSheet is how long to wait for the sign-in sheet to show up before
	// concluding there is nothing to do (default 5s).
	WaitForSheet time.Duration
	// WaitForClose is how long the sheet may take to go away after OK (default 30s).
	WaitForClose time.Duration
}

// sandboxTitles identify the StoreKit / Google Play sign-in sheet across OS
// versions and locales that use English UI.
var sandboxTitles = []string{"Sign in to Apple Account", "Apple Account", "Apple ID", "Sign in with your Apple", "Sandbox Account"}

// SignInSandbox signs a StoreKit sandbox tester in through the system sheet.
// It is idempotent: when no sheet shows up within WaitForSheet it returns
// (false, nil) and does nothing, so it is safe at the start of every run.
// The password is never logged or returned in an error.
func SignInSandbox(ctx context.Context, d Driver, o SandboxOptions) (bool, error) {
	if o.User == "" || o.Password == "" {
		return false, fmt.Errorf("sandbox sign-in needs a user and password (set PROBE_SANDBOX_USER and PROBE_SANDBOX_PASSWORD)")
	}
	if o.WaitForSheet == 0 {
		o.WaitForSheet = 5 * time.Second
	}
	if o.WaitForClose == 0 {
		o.WaitForClose = 30 * time.Second
	}

	title, err := waitForAny(ctx, d, sandboxTitles, o.WaitForSheet)
	if err != nil {
		return false, err
	}
	if title == "" {
		return false, nil
	}

	// Find the sheet's own labels instead of assuming them.
	dialogs, err := d.Dialogs(ctx)
	if err != nil {
		return false, err
	}
	var sheet *Dialog
	for i := range dialogs {
		if dialogs[i].MatchesTitle(title) {
			sheet = &dialogs[i]
			break
		}
	}
	if sheet == nil {
		return false, nil // went away between the two calls
	}
	userField := pick(sheet.Fields, []string{"Apple Account", "Apple ID", "Email", "User"}, 0)
	passField := pick(sheet.Fields, []string{"Password"}, len(sheet.Fields)-1)
	if userField == "" || passField == "" {
		return false, fmt.Errorf("the sign-in sheet has no recognizable fields (fields: %s)", strings.Join(sheet.Fields, ", "))
	}

	if err := d.Type(ctx, userField, o.User, title); err != nil {
		return false, fmt.Errorf("typing the user: %s", Scrub(err.Error(), o.User, o.Password))
	}
	if err := d.Type(ctx, passField, o.Password, title); err != nil {
		return false, fmt.Errorf("typing the password: %s", Scrub(err.Error(), o.User, o.Password))
	}
	btn := "OK"
	if Match("OK", sheet.Buttons) < 0 {
		for _, c := range []string{"Sign In", "Sign in", "Continue", "Done"} {
			if Match(c, sheet.Buttons) >= 0 {
				btn = c
				break
			}
		}
	}
	if _, err := d.Tap(ctx, btn, title); err != nil {
		return false, fmt.Errorf("confirming the sign-in sheet: %s", Scrub(err.Error(), o.User, o.Password))
	}

	closed, err := d.Wait(ctx, title, false, o.WaitForClose)
	if err != nil {
		return false, err
	}
	if !closed {
		return false, fmt.Errorf("the sign-in sheet did not close within %s — the account may have been rejected (check the tester credentials)", o.WaitForClose)
	}
	return true, nil
}

// waitForAny returns the first of titles to appear within timeout, or "".
func waitForAny(ctx context.Context, d Driver, titles []string, timeout time.Duration) (string, error) {
	var found string
	_, err := pollUntil(ctx, timeout, func() (bool, error) {
		for _, t := range titles {
			ok, err := d.See(ctx, t)
			if err != nil {
				return false, err
			}
			if ok {
				found = t
				return true, nil
			}
		}
		return false, nil
	})
	return found, err
}

// pick returns the first field whose label matches one of wanted; otherwise the
// field at fallback (when valid).
func pick(fields, wanted []string, fallback int) string {
	for _, w := range wanted {
		if i := Match(w, fields); i >= 0 {
			return fields[i]
		}
	}
	if fallback >= 0 && fallback < len(fields) {
		return fields[fallback]
	}
	return ""
}
