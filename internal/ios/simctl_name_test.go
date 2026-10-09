package ios

import (
	"context"
	"testing"
)

func TestFindByName(t *testing.T) {
	sims := []Simulator{{UDID: "A", Name: "iPhone 15"}, {UDID: "B", Name: "probe-ios-1"}}
	if got := FindByName(sims, "probe-ios-1"); got == nil || got.UDID != "B" {
		t.Fatalf("FindByName = %+v, want UDID B", got)
	}
	if got := FindByName(sims, "missing"); got != nil {
		t.Fatalf("FindByName(missing) = %+v, want nil", got)
	}
}

func TestCreateRequiresName(t *testing.T) {
	if _, err := New().Create(context.TODO(), "  ", "type", "runtime"); err == nil {
		t.Fatal("Create with empty name must fail")
	}
}

func TestSetAppLanguageLaunchArgs(t *testing.T) {
	s := New()
	if got := s.extraLaunchArgs("U", "app.id"); got != nil {
		t.Fatalf("no override yet, got %v", got)
	}
	s.SetAppLanguage("U", "app.id", "de-DE", "de_DE")
	got := s.extraLaunchArgs("U", "app.id")
	want := []string{"-AppleLanguages", "(de-DE)", "-AppleLocale", "de_DE"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if s.extraLaunchArgs("U", "other.app") != nil || s.extraLaunchArgs("V", "app.id") != nil {
		t.Fatal("override must be per simulator and per app")
	}
	s.SetAppLanguage("U", "app.id", "", "")
	if s.extraLaunchArgs("U", "app.id") != nil {
		t.Fatal("empty tag must remove the override")
	}
}
