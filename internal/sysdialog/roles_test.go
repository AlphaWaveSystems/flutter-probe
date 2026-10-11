package sysdialog

import "testing"

func withLabels(t *testing.T, m map[string]Role) {
	t.Helper()
	old := extraLabelRole
	extraLabelRole = func(f string) (Role, bool) { r, ok := m[f]; return r, ok }
	t.Cleanup(func() { extraLabelRole = old })
}

func TestRoleOfAndMatchButtonAcrossLanguages(t *testing.T) {
	withLabels(t, map[string]Role{"erlauben": RoleAllow, "nicht erlauben": RoleDeny, "einmal erlauben": RoleAllowOnce,
		"beim verwenden der app erlauben": RoleAllowWhileUsing, "abbrechen": RoleCancel})

	if r, ok := RoleOf("Don’t Allow"); !ok || r != RoleDeny {
		t.Fatalf("English deny: %v %v", r, ok)
	}
	german := Dialog{Buttons: []string{"Nicht erlauben", "Erlauben"}}
	if i := MatchButton(german, "Don't Allow"); i != 0 {
		t.Errorf("Don't Allow must find Nicht erlauben, got %d", i)
	}
	if i := MatchButton(german, "Allow"); i != 1 {
		t.Errorf("Allow must find Erlauben (not Nicht erlauben via substring), got %d", i)
	}
	if i := MatchButton(german, "erlauben"); i != 1 {
		t.Errorf("exact folded label wins: got %d", i)
	}
	english := Dialog{Buttons: []string{"Don’t Allow", "Allow"}}
	if i := MatchButton(english, "don't allow"); i != 0 {
		t.Errorf("apostrophe/case: %d", i)
	}
	// "Allow" must not match "Don't Allow" by substring when an Allow role button exists
	loc := Dialog{Buttons: []string{"Allow Once", "Allow While Using App", "Don’t Allow"}}
	if i := MatchButton(loc, "Allow While Using App"); i != 1 {
		t.Errorf("exact: %d", i)
	}
	if i := MatchButton(loc, "Only this time"); i != 0 {
		t.Errorf("role allow_once via another English label: %d", i)
	}
}

func TestAndroidIDRolesBeatLabels(t *testing.T) {
	d := Dialog{
		Buttons:   []string{"Zulassen", "Nicht zulassen"},
		ButtonIDs: []string{"com.android.permissioncontroller:id/permission_allow_button", "com.android.permissioncontroller:id/permission_deny_button"},
	}
	if i := MatchButton(d, "Don't allow"); i != 1 {
		t.Errorf("deny by resource id in any language: %d", i)
	}
	if i := MatchButton(d, "Allow"); i != 0 {
		t.Errorf("allow by resource id: %d", i)
	}
	if !d.HasRoles(RoleAllow, RoleDeny) || d.HasRoles(RoleAllowOnce) {
		t.Error("HasRoles wrong")
	}
	if i := DismissButton(d); i != 1 {
		t.Errorf("dismiss picks the deny button: %d", i)
	}
}

func TestDismissButtonByRole(t *testing.T) {
	withLabels(t, map[string]Role{"abbrechen": RoleCancel})
	if i := DismissButton(Dialog{Buttons: []string{"Löschen", "Abbrechen"}}); i != 1 {
		t.Errorf("German cancel: %d", i)
	}
	if i := DismissButton(Dialog{Buttons: []string{"Delete", "Cancel"}}); i != 1 {
		t.Errorf("English cancel: %d", i)
	}
}

func TestRoleOfMeasuredIOSLabels(t *testing.T) {
	cases := map[string]Role{
		"Nicht erlauben":                  RoleDeny,
		"Erlauben":                        RoleAllow,
		"Beim Verwenden der App erlauben": RoleAllowWhileUsing,
		"Einmal erlauben":                 RoleAllowOnce,
		"Autoriser":                       RoleAllow,
		"Ne pas autoriser":                RoleDeny,
	}
	for label, want := range cases {
		if got, ok := RoleOf(label); !ok || got != want {
			t.Errorf("RoleOf(%q) = %q, %v; want %q", label, got, ok, want)
		}
	}
}
