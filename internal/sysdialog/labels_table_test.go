package sysdialog

import "testing"

// TestSystemLabelsResolveToTheirRole checks every label of the system alerts in every locale: each must resolve to the
// role it is listed under, unambiguously. The count is the number of locales in the iOS 26.5 TCC bundle they were read from.
func TestSystemLabelsResolveToTheirRole(t *testing.T) {
	if len(iosSystemLabels) != 55 {
		t.Errorf("expected 55 system alert locales, got %d", len(iosSystemLabels))
	}
	for loc, byRole := range iosSystemLabels {
		for role, labels := range byRole {
			for _, label := range labels {
				if got, ok := LabelRole(label); !ok || got != role {
					t.Errorf("system label %s %s %q: LabelRole = (%q, %v), want %s", loc, role, label, got, ok, role)
				}
			}
		}
	}
}

// measuredToSystem maps each measured language to its locale in the system bundle.
var measuredToSystem = map[string]string{
	"en": "en", "de": "de", "fr": "fr", "es": "es", "it": "it", "pt-BR": "pt_BR", "nl": "nl", "sv": "sv", "da": "da",
	"nb": "no", "fi": "fi", "pl": "pl", "cs": "cs", "tr": "tr", "ru": "ru", "uk": "uk", "el": "el", "he": "he", "ar": "ar",
	"hi": "hi", "th": "th", "vi": "vi", "id": "id", "ja": "ja", "ko": "ko", "zh-Hans": "zh_CN", "zh-Hant": "zh_TW",
}

// TestMeasuredLanguagesHaveSystemLabels checks that each measured language has a system locale, and that every label
// measured on a device still resolves to the same role once the system labels are added.
func TestMeasuredLanguagesHaveSystemLabels(t *testing.T) {
	for tag, byRole := range iosRoleLabels {
		loc, ok := measuredToSystem[tag]
		if !ok {
			t.Errorf("measured language %s has no system locale", tag)
			continue
		}
		if _, ok := iosSystemLabels[loc]; !ok {
			t.Errorf("system locale %s for %s is missing", loc, tag)
		}
		for role, labels := range byRole {
			for _, label := range labels {
				if got, ok := LabelRole(label); !ok || got != role {
					t.Errorf("measured %s %s %q: LabelRole = (%q, %v), want %s", tag, role, label, got, ok, role)
				}
			}
		}
	}
}

// TestLabelRoleRefusesALabelUnderTwoRoles puts one label under two roles, in a locale that does not exist, and checks
// that LabelRole gives up on it rather than picking one. The entry is removed again afterwards.
func TestLabelRoleRefusesALabelUnderTwoRoles(t *testing.T) {
	iosSystemLabels["zz"] = map[string][]string{
		"allow": {"Colliding"},
		"deny":  {"Colliding"},
	}
	t.Cleanup(func() { delete(iosSystemLabels, "zz") })
	if role, ok := LabelRole("Colliding"); ok {
		t.Errorf("LabelRole(Colliding) = %q, want no role for a label under two roles", role)
	}
	if role, ok := LabelRole("Allow"); !ok || role != "allow" {
		t.Errorf("LabelRole(Allow) = (%q, %v), want allow: a collision must not change other labels", role, ok)
	}
}

func TestDismissButtonTakesTheDeclineInOtherLanguages(t *testing.T) {
	cases := []struct {
		name    string
		buttons []string
		want    int
	}{
		{"German ATT", []string{"Erlauben", "App-Tracking ablehnen"}, 1},
		{"French ATT", []string{"Autoriser", "Demander à l’app de ne pas me suivre"}, 1},
		{"Japanese ATT", []string{"許可", "アプリにトラッキングしないように要求"}, 1},
		{"Dutch camera", []string{"Sta toe", "Weiger"}, 1},
		{"German location keeps the allow-once button", []string{"Einmal erlauben", "Beim Verwenden der App erlauben", "Nicht erlauben"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if i := DismissButton(Dialog{Buttons: tc.buttons}); i != tc.want {
				t.Errorf("DismissButton(%q) = %d, want %d", tc.buttons, i, tc.want)
			}
		})
	}
}

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
