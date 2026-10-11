package sysdialog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// scriptedDriver is an in-memory Driver for flow tests.
type scriptedDriver struct {
	dialog    *Dialog // current dialog; nil = none
	typed     [][2]string
	tapped    []string
	typeErr   error
	closeOnOK bool // tapping OK removes the dialog
}

func (s *scriptedDriver) Dialogs(context.Context) ([]Dialog, error) {
	if s.dialog == nil {
		return nil, nil
	}
	return []Dialog{*s.dialog}, nil
}
func (s *scriptedDriver) See(_ context.Context, title string) (bool, error) {
	return s.dialog != nil && s.dialog.MatchesTitle(title), nil
}
func (s *scriptedDriver) Wait(ctx context.Context, title string, appear bool, timeout time.Duration) (bool, error) {
	return pollUntil(ctx, timeout, func() (bool, error) { ok, _ := s.See(ctx, title); return ok == appear, nil })
}
func (s *scriptedDriver) Tap(_ context.Context, b, _ string) (string, error) {
	s.tapped = append(s.tapped, b)
	if s.closeOnOK && Match(b, []string{"OK"}) == 0 {
		s.dialog = nil
	}
	return b, nil
}
func (s *scriptedDriver) Type(_ context.Context, f, text, _ string) error {
	s.typed = append(s.typed, [2]string{f, text})
	return s.typeErr
}
func (s *scriptedDriver) Dismiss(context.Context, string) (bool, error) { return false, nil }
func (s *scriptedDriver) Close() error                                  { return nil }

func appleSheet() *Dialog {
	return &Dialog{Title: "Sign in to Apple Account", Texts: []string{"Sign in to Apple Account"},
		Fields: []string{"Apple Account", "Password"}, Buttons: []string{"Cancel", "OK"}}
}

func TestSignInSandbox_FillsTheSheetAndWaitsForItToClose(t *testing.T) {
	d := &scriptedDriver{dialog: appleSheet(), closeOnOK: true}
	done, err := SignInSandbox(context.Background(), d, SandboxOptions{User: "tester@example.com", Password: "pw", WaitForSheet: time.Second, WaitForClose: time.Second})
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if len(d.typed) != 2 || d.typed[0] != [2]string{"Apple Account", "tester@example.com"} || d.typed[1] != [2]string{"Password", "pw"} {
		t.Errorf("typed = %v", d.typed)
	}
	if len(d.tapped) != 1 || d.tapped[0] != "OK" {
		t.Errorf("tapped = %v", d.tapped)
	}
}

func TestSignInSandbox_IsANoOpWhenNoSheetAppears(t *testing.T) {
	d := &scriptedDriver{}
	start := time.Now()
	done, err := SignInSandbox(context.Background(), d, SandboxOptions{User: "u", Password: "p", WaitForSheet: 400 * time.Millisecond})
	if err != nil || done {
		t.Fatalf("done=%v err=%v; want a quiet no-op", done, err)
	}
	if len(d.typed) != 0 || len(d.tapped) != 0 {
		t.Error("nothing must be typed or tapped when there is no sheet")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("the wait is not bounded")
	}
}

func TestSignInSandbox_RequiresCredentialsAndNeverEchoesThem(t *testing.T) {
	if _, err := SignInSandbox(context.Background(), &scriptedDriver{}, SandboxOptions{}); err == nil || !strings.Contains(err.Error(), "PROBE_SANDBOX_USER") {
		t.Errorf("missing creds should name the env vars: %v", err)
	}
	d := &scriptedDriver{dialog: appleSheet(), typeErr: errors.New("could not type s3cret for tester@example.com")}
	_, err := SignInSandbox(context.Background(), d, SandboxOptions{User: "tester@example.com", Password: "s3cret", WaitForSheet: time.Second})
	if err == nil {
		t.Fatal("expected the type error")
	}
	if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "tester@example.com") {
		t.Errorf("credentials leaked into the error: %v", err)
	}
}

func TestSignInSandbox_SheetThatStaysOpenIsAnActionableError(t *testing.T) {
	d := &scriptedDriver{dialog: appleSheet(), closeOnOK: false}
	_, err := SignInSandbox(context.Background(), d, SandboxOptions{User: "u", Password: "p", WaitForSheet: time.Second, WaitForClose: 400 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "did not close") {
		t.Errorf("got %v", err)
	}
}

func TestSignInSandbox_FallsBackToSignInButtonAndOnlyField(t *testing.T) {
	sheet := &Dialog{Texts: []string{"Apple ID"}, Fields: []string{"Email", "Secret"}, Buttons: []string{"Sign In", "Cancel"}}
	d := &scriptedDriver{dialog: sheet}
	d.closeOnOK = false
	_, _ = SignInSandbox(context.Background(), d, SandboxOptions{User: "u", Password: "p", WaitForSheet: time.Second, WaitForClose: 300 * time.Millisecond})
	if len(d.tapped) != 1 || d.tapped[0] != "Sign In" {
		t.Errorf("tapped = %v", d.tapped)
	}
	if len(d.typed) != 2 || d.typed[0][0] != "Email" || d.typed[1][0] != "Secret" {
		t.Errorf("typed = %v (user -> Email, last field -> password)", d.typed)
	}
}
