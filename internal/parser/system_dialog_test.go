package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func sysStep(t *testing.T, line string) parser.SystemDialogStep {
	t.Helper()
	prog := mustParse(t, "test \"t\"\n  "+line+"\n")
	if len(prog.Tests[0].Body) != 1 {
		t.Fatalf("%q: want exactly 1 step, got %d: %#v", line, len(prog.Tests[0].Body), prog.Tests[0].Body)
	}
	s, ok := prog.Tests[0].Body[0].(parser.SystemDialogStep)
	if !ok {
		t.Fatalf("%q: want SystemDialogStep, got %#v", line, prog.Tests[0].Body[0])
	}
	return s
}

func TestSystemDialog_Tap(t *testing.T) {
	s := sysStep(t, `tap "Allow" in system dialog`)
	if s.Op != parser.SysTap || s.Button != "Allow" || s.Title != "" {
		t.Errorf("%+v", s)
	}
	s = sysStep(t, `tap "OK" in system dialog "Apple Account"`)
	if s.Button != "OK" || s.Title != "Apple Account" {
		t.Errorf("title filter: %+v", s)
	}
	s = sysStep(t, `tap "Allow" in system dialog optional`)
	if !s.Optional {
		t.Errorf("optional not parsed: %+v", s)
	}
}

func TestSystemDialog_Type(t *testing.T) {
	s := sysStep(t, `type "$PROBE_SANDBOX_PASSWORD" into system field "Password"`)
	if s.Op != parser.SysType || s.Text != "$PROBE_SANDBOX_PASSWORD" || s.Field != "Password" {
		t.Errorf("%+v", s)
	}
}

func TestSystemDialog_SeeWaitDismissSandbox(t *testing.T) {
	if s := sysStep(t, `see system dialog "Sign in to Apple Account"`); s.Op != parser.SysSee || s.Negated || s.Title != "Sign in to Apple Account" {
		t.Errorf("see: %+v", s)
	}
	if s := sysStep(t, `don't see system dialog "Notifications"`); s.Op != parser.SysSee || !s.Negated {
		t.Errorf("don't see: %+v", s)
	}
	if s := sysStep(t, `wait for system dialog "Notifications" appears`); s.Op != parser.SysWait || !s.Appear || s.Title != "Notifications" {
		t.Errorf("wait appears: %+v", s)
	}
	if s := sysStep(t, `wait for system dialog "Notifications" disappears`); s.Op != parser.SysWait || s.Appear {
		t.Errorf("wait disappears: %+v", s)
	}
	if s := sysStep(t, `dismiss system dialog`); s.Op != parser.SysDismiss {
		t.Errorf("dismiss: %+v", s)
	}
	if s := sysStep(t, `sign in sandbox tester`); s.Op != parser.SysSandbox {
		t.Errorf("sandbox: %+v", s)
	}
}

// A system dialog step must not swallow its neighbours, and ordinary Flutter
// steps with similar words must keep working.
func TestSystemDialog_DoesNotAffectOtherSteps(t *testing.T) {
	prog := mustParse(t, `test "t"
  tap "Allow"
  tap "Allow" in system dialog
  see "system dialog"
  tap "Settings" in "Menu"
  dismiss system dialog
  type "x" into the "Email" field
`)
	body := prog.Tests[0].Body
	if len(body) != 6 {
		t.Fatalf("want 6 steps, got %d: %#v", len(body), body)
	}
	if _, ok := body[0].(parser.ActionStep); !ok {
		t.Errorf("plain tap became %T", body[0])
	}
	if _, ok := body[1].(parser.SystemDialogStep); !ok {
		t.Errorf("system tap became %T", body[1])
	}
	if a, ok := body[2].(parser.AssertStep); !ok || a.Sel.Text != "system dialog" {
		t.Errorf("a quoted \"system dialog\" must stay a Flutter selector: %#v", body[2])
	}
	if _, ok := body[3].(parser.ActionStep); !ok {
		t.Errorf("relational tap became %T", body[3])
	}
	if _, ok := body[4].(parser.SystemDialogStep); !ok {
		t.Errorf("dismiss became %T", body[4])
	}
	if _, ok := body[5].(parser.ActionStep); !ok {
		t.Errorf("flutter type became %T", body[5])
	}
}

func TestSystemDialog_MalformedStepsAreParseErrors(t *testing.T) {
	for _, src := range []string{
		`tap in system dialog`,
		`type "x" into system field`,
	} {
		if _, err := parser.ParseFile("test \"t\"\n  " + src + "\n"); err == nil {
			t.Errorf("%q should be a parse error", src)
		}
	}
}
