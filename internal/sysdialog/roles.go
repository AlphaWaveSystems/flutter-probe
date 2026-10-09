package sysdialog

import "strings"

// Role is what a system dialog button means, independent of the device language.
type Role string

const (
	RoleAllow           Role = "allow"
	RoleDeny            Role = "deny"
	RoleAllowOnce       Role = "allow_once"
	RoleAllowWhileUsing Role = "allow_while_using"
	RoleAllowAlways     Role = "allow_always"
	RoleOK              Role = "ok"
	RoleCancel          Role = "cancel"
	RoleNotNow          Role = "not_now"
	RoleOpen            Role = "open"
	RoleClose           Role = "close"
)

// englishRoles are the English (iOS and Android) labels of each role, folded.
var englishRoles = map[string]Role{
	"allow": RoleAllow, "yes": RoleAllow,
	"don't allow": RoleDeny, "deny": RoleDeny, "deny & don't ask again": RoleDeny, "block": RoleDeny,
	"allow once": RoleAllowOnce, "only this time": RoleAllowOnce, "this time only": RoleAllowOnce,
	"allow while using app": RoleAllowWhileUsing, "while using the app": RoleAllowWhileUsing,
	"allow while using the app": RoleAllowWhileUsing,
	"allow all the time": RoleAllowAlways, "allow always": RoleAllowAlways,
	"ok": RoleOK, "got it": RoleOK, "continue": RoleOK,
	"cancel": RoleCancel, "no thanks": RoleCancel, "no": RoleCancel,
	"not now": RoleNotNow, "later": RoleNotNow, "maybe later": RoleNotNow, "ask me later": RoleNotNow,
	"open": RoleOpen,
	"close": RoleClose, "dismiss": RoleClose,
}

// androidIDRoles maps the language-independent resource ids of Android's permission dialog buttons.
var androidIDRoles = map[string]Role{
	"permission_allow_button":                   RoleAllow,
	"permission_allow_foreground_only_button":   RoleAllowWhileUsing,
	"permission_allow_one_time_button":          RoleAllowOnce,
	"permission_allow_always_button":            RoleAllowAlways,
	"permission_deny_button":                    RoleDeny,
	"permission_deny_and_dont_ask_again_button": RoleDeny,
}

// extraLabelRole is filled by the measured iOS label table (labels_table.go) and by
// user-supplied labels, to translate a button label of any language to its role.
var extraLabelRole func(folded string) (Role, bool)

// RoleOf returns the role of a button label in any supported language.
func RoleOf(label string) (Role, bool) {
	f := normalize(label)
	if f == "" {
		return "", false
	}
	if r, ok := englishRoles[f]; ok {
		return r, true
	}
	if extraLabelRole != nil {
		return extraLabelRole(f)
	}
	return "", false
}

// IDRole returns the role of an Android resource id such as
// "com.android.permissioncontroller:id/permission_allow_button".
func IDRole(resourceID string) (Role, bool) {
	if i := strings.LastIndex(resourceID, "/"); i >= 0 {
		resourceID = resourceID[i+1:]
	}
	r, ok := androidIDRoles[resourceID]
	return r, ok
}

// ButtonRole is the role of the i-th button: its Android resource id first, then its label.
func (d Dialog) ButtonRole(i int) (Role, bool) {
	if i < 0 || i >= len(d.Buttons) {
		return "", false
	}
	if i < len(d.ButtonIDs) {
		if r, ok := IDRole(d.ButtonIDs[i]); ok {
			return r, true
		}
	}
	return RoleOf(d.Buttons[i])
}

// HasRoles reports whether the dialog has a button for every given role.
func (d Dialog) HasRoles(roles ...Role) bool {
	for _, want := range roles {
		found := false
		for i := range d.Buttons {
			if r, ok := d.ButtonRole(i); ok && r == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// MatchButton picks the button for wanted: an exact (folded) label first; then the button
// with the same role as wanted (so "Don't Allow" finds "Nicht erlauben" on a German device);
// then a substring match. -1 when nothing fits.
func MatchButton(d Dialog, wanted string) int {
	w := normalize(wanted)
	if w == "" {
		return -1
	}
	for i, l := range d.Buttons {
		if normalize(l) == w {
			return i
		}
	}
	if r, ok := RoleOf(wanted); ok {
		for i := range d.Buttons {
			if br, ok := d.ButtonRole(i); ok && br == r {
				return i
			}
		}
	}
	for i, l := range d.Buttons {
		if strings.Contains(normalize(l), w) {
			return i
		}
	}
	return -1
}

// dismissRoles are tried in order by Dismiss.
var dismissRoles = []Role{RoleCancel, RoleDeny, RoleNotNow, RoleClose}

// DismissButton picks the cancel-like button of d (by role, in any language), or -1.
func DismissButton(d Dialog) int {
	for _, want := range dismissRoles {
		for i := range d.Buttons {
			if r, ok := d.ButtonRole(i); ok && r == want {
				return i
			}
		}
	}
	return DismissIndex(d.Buttons)
}
