package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// systemDialogDescription is the step text shown in progress output, reports
// and error prefixes. Anything typed into a system field is ALWAYS masked: the
// text may be a password, and we cannot tell a secret from a literal.
func systemDialogDescription(s parser.SystemDialogStep) string {
	title := ""
	if s.Title != "" {
		title = fmt.Sprintf(" %q", s.Title)
	}
	switch s.Op {
	case parser.SysTap:
		return fmt.Sprintf("tap %q in system dialog%s", s.Button, title)
	case parser.SysType:
		return fmt.Sprintf("type \"****\" into system field %q", s.Field)
	case parser.SysSee:
		if s.Negated {
			return "don't see system dialog" + title
		}
		return "see system dialog" + title
	case parser.SysWait:
		if s.Appear {
			return "wait for system dialog" + title + " appears"
		}
		return "wait for system dialog" + title + " disappears"
	case parser.SysDismiss:
		return "dismiss system dialog" + title
	case parser.SysSandbox:
		return "sign in sandbox tester"
	}
	return "system dialog step"
}

// runSystemDialog executes a system-dialog step. Secrets (env-resolved or
// typed literals) are scrubbed from every error it returns.
func (e *Executor) runSystemDialog(ctx context.Context, s parser.SystemDialogStep) (err error) {
	if e.deviceCtx == nil {
		return fmt.Errorf("system dialog steps need a local simulator/emulator (not available in cloud or dry-run mode)")
	}
	var secrets []string
	defer func() {
		if err != nil && len(secrets) > 0 {
			err = fmt.Errorf("%s", sysdialog.Scrub(err.Error(), secrets...))
		}
	}()

	d, derr := e.deviceCtx.SystemDriver(ctx)
	if derr != nil {
		return derr
	}

	switch s.Op {
	case parser.SysTap:
		_, err = d.Tap(ctx, e.resolve(s.Button), s.Title)
		return err

	case parser.SysType:
		text := e.resolve(s.Text)
		value, _, rerr := sysdialog.ResolveSecret(text)
		if rerr != nil {
			return rerr
		}
		secrets = append(secrets, value, text)
		return d.Type(ctx, e.resolve(s.Field), value, s.Title)

	case parser.SysSee:
		ok, serr := d.See(ctx, s.Title)
		if serr != nil {
			return serr
		}
		if ok == s.Negated {
			if s.Negated {
				return fmt.Errorf("expected NOT to see a system dialog%s, but one is showing%s", titleSuffix(s.Title), e.showingNow(ctx, d))
			}
			return fmt.Errorf("expected to see a system dialog%s, but none is showing%s", titleSuffix(s.Title), e.showingNow(ctx, d))
		}
		return nil

	case parser.SysWait:
		timeout := e.timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ok, werr := d.Wait(ctx, s.Title, s.Appear, timeout-time.Second)
		if werr != nil {
			return werr
		}
		if !ok {
			state := "appear"
			if !s.Appear {
				state = "disappear"
			}
			return fmt.Errorf("timed out waiting for the system dialog%s to %s%s", titleSuffix(s.Title), state, e.showingNow(ctx, d))
		}
		return nil

	case parser.SysDismiss:
		_, err = d.Dismiss(ctx, s.Title)
		return err

	case parser.SysSandbox:
		user, pass := os.Getenv("PROBE_SANDBOX_USER"), os.Getenv("PROBE_SANDBOX_PASSWORD")
		secrets = append(secrets, user, pass)
		_, err = sysdialog.SignInSandbox(ctx, d, sysdialog.SandboxOptions{User: user, Password: pass})
		return err
	}
	return fmt.Errorf("unknown system dialog operation %q", s.Op)
}

func titleSuffix(title string) string {
	if title == "" {
		return ""
	}
	return fmt.Sprintf(" matching %q", title)
}

// showingNow describes what system dialogs are actually up, to make a failed
// see/wait self-explanatory. Best effort.
func (e *Executor) showingNow(ctx context.Context, d sysdialog.Driver) string {
	ds, err := d.Dialogs(ctx)
	if err != nil || len(ds) == 0 {
		return " (no system dialog is showing)"
	}
	var parts []string
	for _, x := range ds {
		parts = append(parts, fmt.Sprintf("%q [%s]", x.Title, strings.Join(x.Buttons, " | ")))
	}
	return " (showing: " + strings.Join(parts, "; ") + ")"
}
