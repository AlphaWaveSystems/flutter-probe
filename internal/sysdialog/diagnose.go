package sysdialog

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Diagnoser is implemented by drivers that can say why an awaited dialog never showed up. A wait that
// times out is usually not a driver problem: the app simply never asked.
type Diagnoser interface {
	// Diagnose returns a short, one-paragraph explanation (empty when there is nothing to add) for
	// a wait for a dialog matching title that timed out, for the app with id appID.
	Diagnose(ctx context.Context, appID, title string) string
}

var focusRe = regexp.MustCompile(`mCurrentFocus=Window\{[^ ]+ [^ ]+ ([^/\s}]+)/([^\s}]+)`)

// ParseFocus returns the package and activity of the focused window from `dumpsys window` output.
func ParseFocus(dumpsysWindow string) (pkg, activity string, ok bool) {
	m := focusRe.FindStringSubmatch(dumpsysWindow)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var permLineRe = regexp.MustCompile(`^\s*([A-Za-z0-9_.]+): granted=(true|false)`)

// ParsePermissionState returns whether permission is granted in `dumpsys package <app>` output. found is
// false when the app does not list the permission.
func ParsePermissionState(dumpsysPackage, permission string) (granted, found bool) {
	for _, line := range strings.Split(dumpsysPackage, "\n") {
		if m := permLineRe.FindStringSubmatch(line); m != nil && m[1] == permission {
			granted = m[2] == "true"
			found = true
			// A runtime-permissions entry wins over an earlier install-time one.
		}
	}
	return granted, found
}

// ParseDeniedRuntime lists the runtime permissions of the app that are currently denied.
func ParseDeniedRuntime(dumpsysPackage string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(dumpsysPackage, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "runtime permissions:"):
			in = true
			continue
		case in && trim != "" && !strings.Contains(trim, "granted="):
			in = false
		}
		if !in {
			continue
		}
		if m := permLineRe.FindStringSubmatch(line); m != nil && m[2] == "false" {
			out = append(out, strings.TrimPrefix(m[1], "android.permission."))
		}
	}
	sort.Strings(out)
	return out
}

// permissionForTitle maps the words of a permission dialog's title to the permission it asks for.
func permissionForTitle(title string) string {
	t := strings.ToLower(title)
	for _, p := range []struct{ word, perm string }{
		{"notification", "android.permission.POST_NOTIFICATIONS"},
		{"camera", "android.permission.CAMERA"},
		{"microphone", "android.permission.RECORD_AUDIO"},
		{"record audio", "android.permission.RECORD_AUDIO"},
		{"location", "android.permission.ACCESS_FINE_LOCATION"},
		{"contacts", "android.permission.READ_CONTACTS"},
		{"photos", "android.permission.READ_MEDIA_IMAGES"},
		{"calendar", "android.permission.READ_CALENDAR"},
	} {
		if strings.Contains(t, p.word) {
			return p.perm
		}
	}
	return ""
}

// explain assembles the message from what `dumpsys window` and `dumpsys package` printed.
func explain(appID, title, windowDump, packageDump string) string {
	var parts []string
	pkg, act, haveFocus := ParseFocus(windowDump)
	if haveFocus {
		parts = append(parts, fmt.Sprintf("focused window: %s/%s", pkg, act))
	}
	perm := permissionForTitle(title)
	state := ""
	switch {
	case perm != "" && packageDump != "":
		if g, found := ParsePermissionState(packageDump, perm); found {
			if g {
				state = "granted"
			} else {
				state = "denied"
			}
			parts = append(parts, fmt.Sprintf("%s: %s", strings.TrimPrefix(perm, "android.permission."), state))
		} else {
			parts = append(parts, fmt.Sprintf("%s is not declared by the app", strings.TrimPrefix(perm, "android.permission.")))
		}
	case packageDump != "":
		if denied := ParseDeniedRuntime(packageDump); len(denied) > 0 {
			if len(denied) > 5 {
				denied = append(denied[:5], "...")
			}
			parts = append(parts, "denied runtime permissions: "+strings.Join(denied, ", "))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	hint := ""
	if haveFocus && appID != "" && pkg == appID {
		hint = "no system dialog was requested: the app is in the foreground"
		if state != "" {
			hint += " and the permission is " + state
		}
		hint += ", so it never asked (or already had the answer)"
	} else if haveFocus {
		hint = fmt.Sprintf("another window (%s) has the focus, which is not one of the known system dialogs: check PROBE_ANDROID_DIALOG_PACKAGES", pkg)
	}
	msg := strings.Join(parts, "; ")
	if hint != "" {
		msg += ". Hint: " + hint
	}
	return msg
}

// Diagnose implements Diagnoser for Android with `dumpsys window` and `dumpsys package`.
func (a *AndroidDriver) Diagnose(ctx context.Context, appID, title string) string {
	win, _ := a.adb.Shell(ctx, a.serial, "dumpsys", "window")
	var pkgDump []byte
	if appID != "" {
		pkgDump, _ = a.adb.Shell(ctx, a.serial, "dumpsys", "package", appID)
	}
	return explain(appID, title, string(win), string(pkgDump))
}
