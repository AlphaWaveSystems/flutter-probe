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
