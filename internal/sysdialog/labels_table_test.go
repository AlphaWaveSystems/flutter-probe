package sysdialog

import "testing"

func TestLabelRole(t *testing.T) {
	tests := []struct {
		label string
		role  string
		ok    bool
	}{
		{"Allow", "allow", true},
		{"Don’t Allow", "deny", true},
		{"don't allow", "deny", true},
		{"Allow Once", "allow_once", true},
		{"Allow While Using App", "allow_while_using", true},
		{"Nicht erlauben", "deny", true},
		{"Erlauben", "allow", true},
		{"Autoriser lorsque l’app est active", "allow_while_using", true},
		{"Ne pas autoriser", "deny", true},
		{"許可しない", "deny", true},
		{"アプリの使用中は許可", "allow_while_using", true},
		{"عدم السماح", "deny", true},
		{"Zakázat", "deny", true},
		{"Nepovolovat", "deny", true},
		{"Запретить", "deny", true},
		{"  ALLOW  ", "allow", true},
		{"", "", false},
		{"Something else", "", false},
	}
	for _, tt := range tests {
		role, ok := LabelRole(tt.label)
		if role != tt.role || ok != tt.ok {
			t.Errorf("LabelRole(%q) = (%q, %v), want (%q, %v)", tt.label, role, ok, tt.role, tt.ok)
		}
	}
}

func TestRoleLabels(t *testing.T) {
	for _, role := range []string{"allow", "deny", "allow_once", "allow_while_using"} {
		labels := RoleLabels(role)
		if len(labels) < len(iosRoleLabels)/2 {
			t.Errorf("RoleLabels(%q) = %d labels, want at least one per ~language", role, len(labels))
		}
		seen := map[string]bool{}
		for _, l := range labels {
			if seen[normalize(l)] {
				t.Errorf("RoleLabels(%q) has duplicate %q", role, l)
			}
			seen[normalize(l)] = true
		}
	}
	if got := RoleLabels("allow")[0]; got != "Allow" {
		t.Errorf("English must come first, got %q", got)
	}
	if got := RoleLabels("deny")[0]; got != "Don’t Allow" {
		t.Errorf("English must come first, got %q", got)
	}
	// Unobserved roles are missing, not guessed.
	for _, role := range []string{"ok", "cancel", "open", "nonsense"} {
		if got := RoleLabels(role); got != nil {
			t.Errorf("RoleLabels(%q) = %v, want nil", role, got)
		}
	}
}

func TestIOSRoleLabelsConsistency(t *testing.T) {
	if len(iosRoleLabels) != 27 {
		t.Errorf("expected 27 measured languages, got %d", len(iosRoleLabels))
	}
	roles := map[string]bool{}
	for _, r := range roleOrder {
		roles[r] = true
	}
	owner := map[string]string{} // normalized label -> role, across all languages
	for lang, byRole := range iosRoleLabels {
		if _, hasDeny := byRole["deny"]; hasDeny {
			if len(byRole["allow"]) == 0 {
				t.Errorf("%s has deny but no allow", lang)
			}
		}
		for role, labels := range byRole {
			if !roles[role] {
				t.Errorf("%s: unknown role %q", lang, role)
			}
			if len(labels) == 0 {
				t.Errorf("%s/%s: empty label list (omit the role instead)", lang, role)
			}
			for _, l := range labels {
				n := normalize(l)
				if n == "" {
					t.Errorf("%s/%s: empty label", lang, role)
				}
				if prev, ok := owner[n]; ok && prev != role {
					t.Errorf("label %q is both %q and %q (%s)", l, prev, role, lang)
				}
				owner[n] = role
			}
		}
	}
}
