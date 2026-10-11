package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

type fakeSys struct {
	mu          sync.Mutex
	dialog      *sysdialog.Dialog
	typed       [][2]string
	tapped      []string
	typeErrText string
	dismissed   bool
}

func (f *fakeSys) Dialogs(context.Context) ([]sysdialog.Dialog, error) {
	if f.dialog == nil {
		return nil, nil
	}
	return []sysdialog.Dialog{*f.dialog}, nil
}
func (f *fakeSys) See(_ context.Context, title string) (bool, error) {
	return f.dialog != nil && f.dialog.MatchesTitle(title), nil
}
func (f *fakeSys) Wait(ctx context.Context, title string, appear bool, _ time.Duration) (bool, error) {
	ok, _ := f.See(ctx, title)
	return ok == appear, nil
}
func (f *fakeSys) Tap(_ context.Context, b, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dialog == nil {
		return "", errors.New("no system dialog is showing")
	}
	f.tapped = append(f.tapped, b)
	return b, nil
}
func (f *fakeSys) Type(_ context.Context, field, text, _ string) error {
	f.typed = append(f.typed, [2]string{field, text})
	if f.typeErrText != "" {
		return errors.New(strings.ReplaceAll(f.typeErrText, "{text}", text))
	}
	return nil
}
func (f *fakeSys) Dismiss(context.Context, string) (bool, error) {
	f.dismissed = true
	return true, nil
}
func (f *fakeSys) Close() error { return nil }

func sysExecutor(f *fakeSys) *Executor {
	dc := &DeviceContext{sysDriver: f}
	return NewExecutor(&fakeAIClient{}, dc, nil, 5*time.Second, false)
}

func TestSystemDialogStep_TypeResolvesEnvAndMasksEverywhere(t *testing.T) {
	t.Setenv("PROBE_TEST_PW", "s3cretPW")
	f := &fakeSys{dialog: &sysdialog.Dialog{Title: "Sign in"}, typeErrText: "field rejected {text}"}
	e := sysExecutor(f)
	step := parser.SystemDialogStep{Op: parser.SysType, Text: "$PROBE_TEST_PW", Field: "Password", Line: 4}

	if desc := e.stepDescription(step); strings.Contains(desc, "s3cretPW") || strings.Contains(desc, "PROBE_TEST_PW") || !strings.Contains(desc, "****") {
		t.Errorf("step description must be masked, got %q", desc)
	}
	err := e.runStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected the driver's error")
	}
	if strings.Contains(err.Error(), "s3cretPW") {
		t.Errorf("secret leaked into the error: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "line 4: type \"****\" into system field") {
		t.Errorf("error should carry the masked step and line: %v", err)
	}
	if len(f.typed) != 1 || f.typed[0] != [2]string{"Password", "s3cretPW"} {
		t.Errorf("the driver must receive the real value, got %v", f.typed)
	}
}

func TestSystemDialogStep_LiteralTextIsMaskedToo(t *testing.T) {
	e := sysExecutor(&fakeSys{dialog: &sysdialog.Dialog{}})
	d := e.stepDescription(parser.SystemDialogStep{Op: parser.SysType, Text: "plain-literal", Field: "Email"})
	if strings.Contains(d, "plain-literal") {
		t.Errorf("literals could be passwords; always mask: %q", d)
	}
}

func TestSystemDialogStep_UnsetEnvVarFailsWithItsName(t *testing.T) {
	e := sysExecutor(&fakeSys{dialog: &sysdialog.Dialog{}})
	err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysType, Text: "$PROBE_NOPE_UNSET", Field: "Password", Line: 1})
	if err == nil || !strings.Contains(err.Error(), "PROBE_NOPE_UNSET") {
		t.Errorf("got %v", err)
	}
}

func TestSystemDialogStep_TapOptionalIsIdempotent(t *testing.T) {
	f := &fakeSys{} // no dialog showing
	e := sysExecutor(f)
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysTap, Button: "Allow", Line: 2}); err == nil {
		t.Error("tapping with no dialog must fail when not optional")
	}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysTap, Button: "Allow", Optional: true, Line: 2}); err != nil {
		t.Errorf("optional tap with no dialog must pass, got %v", err)
	}
	f.dialog = &sysdialog.Dialog{Buttons: []string{"Allow"}}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysTap, Button: "Allow", Line: 2}); err != nil || len(f.tapped) != 1 {
		t.Errorf("tap: %v %v", err, f.tapped)
	}
}

func TestSystemDialogStep_SeeAndWaitFailuresSayWhatIsShowing(t *testing.T) {
	f := &fakeSys{dialog: &sysdialog.Dialog{Title: "Allow Notifications?", Texts: []string{"Allow Notifications?"}, Buttons: []string{"Allow", "Don’t Allow"}}}
	e := sysExecutor(f)

	err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysSee, Title: "Apple Account", Line: 7})
	if err == nil || !strings.Contains(err.Error(), "Allow Notifications?") || !strings.Contains(err.Error(), "Don’t Allow") {
		t.Errorf("see failure should list what is showing: %v", err)
	}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysSee, Title: "Notifications", Line: 7}); err != nil {
		t.Errorf("see: %v", err)
	}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysSee, Title: "Notifications", Negated: true, Line: 7}); err == nil {
		t.Error("don't see must fail while the dialog is up")
	}
	err = e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysWait, Title: "Apple Account", Appear: true, Line: 8})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for the system dialog") {
		t.Errorf("wait: %v", err)
	}
}

func TestSystemDialogStep_NeedsALocalDevice(t *testing.T) {
	e := NewExecutor(&fakeAIClient{}, nil, nil, time.Second, false)
	err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysDismiss, Line: 1})
	if err == nil || !strings.Contains(err.Error(), "local simulator/emulator") {
		t.Errorf("got %v", err)
	}
}

func TestSystemDialogDescriptions(t *testing.T) {
	cases := map[string]parser.SystemDialogStep{
		`tap "Allow" in system dialog`:          {Op: parser.SysTap, Button: "Allow"},
		`see system dialog "X"`:                 {Op: parser.SysSee, Title: "X"},
		`don't see system dialog`:               {Op: parser.SysSee, Negated: true},
		`wait for system dialog "X" appears`:    {Op: parser.SysWait, Title: "X", Appear: true},
		`wait for system dialog "X" disappears`: {Op: parser.SysWait, Title: "X"},
		`dismiss system dialog`:                 {Op: parser.SysDismiss},
		`sign in sandbox tester`:                {Op: parser.SysSandbox},
	}
	for want, s := range cases {
		if got := systemDialogDescription(s); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
