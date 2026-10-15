// Package devsettings changes OS-level settings of a named emulator or simulator: appearance (dark mode), font
// scale, app language, screen orientation and opening a link. Studio's device settings panel, `probe device set` and
// the device_set MCP tool all go through it, so the allowed values and the quoting live in one place.
//
// Every value is checked against an allow-list or a strict pattern before it reaches a command, because the values
// come from a UI field or an AI client. Commands run without a shell on the host (the argument vector goes straight
// to adb or xcrun); the one string the Android device shell parses is single-quoted.
package devsettings

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Platform string

const (
	Android Platform = "android"
	IOS     Platform = "ios"
)

type Setting string

const (
	Appearance  Setting = "appearance"  // light | dark (Android also: auto)
	FontScale   Setting = "font_scale"  // Android: a factor such as 1.3; iOS: a content size category
	Locale      Setting = "locale"      // an app language tag such as de-DE (needs the app id)
	Orientation Setting = "orientation" // Android only
	OpenURL     Setting = "open_url"    // a deep link
)

// Request is one change. Device is the serial (Android) or UDID (iOS) of a specific device: "booted" and empty are refused,
// so a change never lands on whichever device happens to be running.
type Request struct {
	Platform Platform
	Device   string
	Setting  Setting
	Value    string
	// App is the application id (Android) or bundle id (iOS). Required for Locale; restricts OpenURL on Android.
	App string
}

// Runner runs a host command and returns its combined output. The default runs the real program; tests pass a fake.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// DefaultRunner runs real adb and xcrun from PATH.
var DefaultRunner Runner = execRunner{}

// IOSContentSizes are the content size categories `simctl ui content_size` accepts (`xcrun simctl help ui`).
var IOSContentSizes = []string{
	"extra-small", "small", "medium", "large", "extra-large", "extra-extra-large", "extra-extra-extra-large",
	"accessibility-medium", "accessibility-large", "accessibility-extra-large", "accessibility-extra-extra-large",
	"accessibility-extra-extra-extra-large",
}

// AndroidFontScales are the factors the panel offers; any factor from 0.5 to 3.0 is accepted.
var AndroidFontScales = []string{"0.85", "1.0", "1.15", "1.3", "1.5", "2.0"}

var androidOrientations = map[string]string{"portrait": "0", "landscape": "1", "reverse-portrait": "2", "reverse-landscape": "3"}

// Orientations are the values Android accepts.
var Orientations = []string{"portrait", "landscape", "reverse-portrait", "reverse-landscape"}

var (
	deviceRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	appRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$`)
	localeRe  = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)
	scaleRe   = regexp.MustCompile(`^[0-9](\.[0-9]{1,2})?$`)
	schemeRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	badURLRe  = regexp.MustCompile(`[\x00-\x20\x7f-\x9f]`)
	maxURLLen = 2000
)

// Command is one host command of a plan.
type Command struct {
	Name string
	Args []string
	// Optional commands may fail without failing the change (terminating an app that is not running).
	Optional bool
}

func (c Command) String() string { return c.Name + " " + strings.Join(c.Args, " ") }

// Plan validates r and returns the commands that apply it. It runs nothing.
func Plan(r Request) ([]Command, error) {
	if r.Device == "" || strings.EqualFold(r.Device, "booted") || !deviceRe.MatchString(r.Device) {
		return nil, fmt.Errorf("name one specific device (a serial or UDID); %q is not accepted", r.Device)
	}
	if r.App != "" && !appRe.MatchString(r.App) {
		return nil, fmt.Errorf("%q is not an application id such as com.example.app", r.App)
	}
	switch r.Platform {
	case Android:
		return planAndroid(r)
	case IOS:
		return planIOS(r)
	}
	return nil, fmt.Errorf("unknown platform %q (use android or ios)", r.Platform)
}

func adb(serial string, shell ...string) Command {
	return Command{Name: "adb", Args: append([]string{"-s", serial, "shell"}, shell...)}
}

func planAndroid(r Request) ([]Command, error) {
	switch r.Setting {
	case Appearance:
		switch r.Value {
		case "light":
			return []Command{adb(r.Device, "cmd", "uimode", "night", "no")}, nil
		case "dark":
			return []Command{adb(r.Device, "cmd", "uimode", "night", "yes")}, nil
		case "auto":
			return []Command{adb(r.Device, "cmd", "uimode", "night", "auto")}, nil
		}
		return nil, fmt.Errorf("appearance must be light, dark or auto, not %q", r.Value)
	case FontScale:
		if !scaleRe.MatchString(r.Value) {
			return nil, fmt.Errorf("font scale must be a factor such as 1.3, not %q", r.Value)
		}
		f, _ := strconv.ParseFloat(r.Value, 64)
		if f < 0.5 || f > 3.0 {
			return nil, fmt.Errorf("font scale must be between 0.5 and 3.0, not %s", r.Value)
		}
		return []Command{adb(r.Device, "settings", "put", "system", "font_scale", r.Value)}, nil
	case Locale:
		if r.App == "" {
			return nil, fmt.Errorf("a language is set per app on Android: give the application id")
		}
		if !localeRe.MatchString(r.Value) {
			return nil, fmt.Errorf("language must be a tag such as de or pt-BR, not %q", r.Value)
		}
		return []Command{adb(r.Device, "cmd", "locale", "set-app-locales", r.App, "--locales", r.Value)}, nil
	case Orientation:
		n, ok := androidOrientations[r.Value]
		if !ok {
			return nil, fmt.Errorf("orientation must be one of %s, not %q", strings.Join(Orientations, ", "), r.Value)
		}
		return []Command{
			adb(r.Device, "settings", "put", "system", "accelerometer_rotation", "0"),
			adb(r.Device, "settings", "put", "system", "user_rotation", n),
		}, nil
	case OpenURL:
		if err := checkURL(r.Value); err != nil {
			return nil, err
		}
		quoted := "'" + strings.ReplaceAll(r.Value, "'", `'\''`) + "'"
		args := []string{"am", "start", "-a", "android.intent.action.VIEW", "-d", quoted}
		if r.App != "" {
			args = append(args, "-p", r.App)
		}
		return []Command{adb(r.Device, args...)}, nil
	}
	return nil, fmt.Errorf("unknown setting %q", r.Setting)
}

func planIOS(r Request) ([]Command, error) {
	simctl := func(args ...string) Command { return Command{Name: "xcrun", Args: append([]string{"simctl"}, args...)} }
	switch r.Setting {
	case Appearance:
		if r.Value != "light" && r.Value != "dark" {
			return nil, fmt.Errorf("appearance must be light or dark on iOS, not %q", r.Value)
		}
		return []Command{simctl("ui", r.Device, "appearance", r.Value)}, nil
	case FontScale:
		for _, s := range IOSContentSizes {
			if s == r.Value {
				return []Command{simctl("ui", r.Device, "content_size", r.Value)}, nil
			}
		}
		return nil, fmt.Errorf("font size must be one of the iOS content size categories (%s), not %q", strings.Join(IOSContentSizes, ", "), r.Value)
	case Locale:
		if r.App == "" {
			return nil, fmt.Errorf("a language is set per app on iOS: give the bundle id (the app restarts)")
		}
		if !localeRe.MatchString(r.Value) {
			return nil, fmt.Errorf("language must be a tag such as de or pt-BR, not %q", r.Value)
		}
		region := strings.ReplaceAll(r.Value, "-", "_")
		return []Command{
			optional(simctl("terminate", r.Device, r.App)),
			simctl("launch", r.Device, r.App, "-AppleLanguages", "("+r.Value+")", "-AppleLocale", region),
		}, nil
	case Orientation:
		return nil, fmt.Errorf("orientation cannot be changed on an iOS simulator from the command line (simctl has no rotate command)")
	case OpenURL:
		if err := checkURL(r.Value); err != nil {
			return nil, err
		}
		return []Command{simctl("openurl", r.Device, r.Value)}, nil
	}
	return nil, fmt.Errorf("unknown setting %q", r.Setting)
}

func optional(c Command) Command { c.Optional = true; return c }

func checkURL(u string) error {
	if u == "" || len(u) > maxURLLen {
		return fmt.Errorf("the link must be 1 to %d characters", maxURLLen)
	}
	if !schemeRe.MatchString(u) {
		return fmt.Errorf("the link must start with a scheme, such as myapp:// or https://")
	}
	if badURLRe.MatchString(u) {
		return fmt.Errorf("the link contains spaces or control characters")
	}
	return nil
}

// Apply validates r and runs its commands in order, stopping at the first failure. A non-nil error carries the tool's
// output so the caller can show why.
func Apply(ctx context.Context, run Runner, r Request) error {
	cmds, err := Plan(r)
	if err != nil {
		return err
	}
	if run == nil {
		run = DefaultRunner
	}
	for _, c := range cmds {
		out, err := run.Run(ctx, c.Name, c.Args...)
		if err != nil && c.Optional {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s failed: %v: %s", c.Name, err, strings.TrimSpace(string(out)))
		}
		// adb shell exits 0 even when the on-device command prints an error.
		if r.Platform == Android && looksFailed(string(out)) {
			return fmt.Errorf("the device refused the change: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func looksFailed(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "exception") || strings.Contains(o, "error:") || strings.Contains(o, "unknown command") ||
		strings.Contains(o, "permission denied") || strings.Contains(o, "securityexception")
}

// Capability lists what a platform supports and the values the panel should offer.
type Capability struct {
	Setting  Setting  `json:"setting"`
	Label    string   `json:"label"`
	Choices  []string `json:"choices,omitempty"` // empty: free text (a language tag, a link)
	NeedsApp bool     `json:"needsApp,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// Capabilities returns the settings available on p, in display order.
func Capabilities(p Platform) []Capability {
	switch p {
	case Android:
		return []Capability{
			{Setting: Appearance, Label: "Appearance", Choices: []string{"light", "dark", "auto"}},
			{Setting: FontScale, Label: "Font scale", Choices: AndroidFontScales},
			{Setting: Locale, Label: "App language", NeedsApp: true, Note: "Android 13 or later; applies to the app under test."},
			{Setting: Orientation, Label: "Orientation", Choices: Orientations, Note: "Turns auto-rotate off."},
			{Setting: OpenURL, Label: "Open link", Note: "Opens a deep link; the app under test handles it when it claims the scheme."},
		}
	case IOS:
		return []Capability{
			{Setting: Appearance, Label: "Appearance", Choices: []string{"light", "dark"}},
			{Setting: FontScale, Label: "Font size", Choices: IOSContentSizes},
			{Setting: Locale, Label: "App language", NeedsApp: true, Note: "Restarts the app with the language."},
			{Setting: OpenURL, Label: "Open link"},
		}
	}
	return nil
}
