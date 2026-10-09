package sysdialog

import (
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"os"
	"strings"
	"time"
)

// ADB is the part of device.ADB the Android driver needs. An interface so the
// uiautomator parsing and tap/type logic can be tested without a device.
type ADB interface {
	UIAutomatorDump(ctx context.Context, serial string) (string, error)
	Tap(ctx context.Context, serial string, x, y int) error
	Shell(ctx context.Context, serial string, args ...string) ([]byte, error)
}

// systemPackages are the packages whose windows count as "system dialogs".
// The app under test is deliberately not here: its own dialogs are Flutter's.
var systemPackages = map[string]bool{
	"com.google.android.permissioncontroller": true,
	"com.android.permissioncontroller":        true,
	"com.android.packageinstaller":            true,
	"com.google.android.packageinstaller":     true,
	"com.android.systemui":                    true,
	"android":                                 true,
	"com.android.vending":                     true, // Play billing / sign-in sheets
	"com.google.android.gms":                  true, // Google account / consent sheets
}

type axNode struct {
	Text        string   `xml:"text,attr"`
	ResourceID  string   `xml:"resource-id,attr"`
	Class       string   `xml:"class,attr"`
	Package     string   `xml:"package,attr"`
	ContentDesc string   `xml:"content-desc,attr"`
	Clickable   string   `xml:"clickable,attr"`
	Password    string   `xml:"password,attr"`
	Bounds      string   `xml:"bounds,attr"`
	Nodes       []axNode `xml:"node"`
}

type axHierarchy struct {
	Nodes []axNode `xml:"node"`
}

// androidDialog is a parsed dialog with the bounds needed to tap its parts.
type androidDialog struct {
	Dialog
	buttonRects []image.Rectangle
	fieldRects  []image.Rectangle
}

func (n axNode) label() string {
	if strings.TrimSpace(n.Text) != "" {
		return strings.TrimSpace(n.Text)
	}
	return strings.TrimSpace(n.ContentDesc)
}

// ParseAndroidDialogs extracts the system dialogs from a uiautomator dump.
// extraPackages adds app-specific system-ish packages (PROBE_ANDROID_DIALOG_PACKAGES).
func ParseAndroidDialogs(dumpXML string, extraPackages ...string) ([]Dialog, error) {
	ds, err := parseAndroid(dumpXML, extraPackages)
	if err != nil {
		return nil, err
	}
	out := make([]Dialog, len(ds))
	for i := range ds {
		out[i] = ds[i].Dialog
	}
	return out, nil
}

func parseAndroid(dumpXML string, extraPackages []string) ([]androidDialog, error) {
	var h axHierarchy
	if err := xml.Unmarshal([]byte(dumpXML), &h); err != nil {
		return nil, fmt.Errorf("parse uiautomator dump: %w", err)
	}
	pkgs := map[string]bool{}
	for k := range systemPackages {
		pkgs[k] = true
	}
	for _, p := range extraPackages {
		if p = strings.TrimSpace(p); p != "" {
			pkgs[p] = true
		}
	}

	byPkg := map[string]*androidDialog{}
	var order []string
	var walk func(n axNode)
	walk = func(n axNode) {
		if pkgs[n.Package] {
			d := byPkg[n.Package]
			if d == nil {
				d = &androidDialog{Dialog: Dialog{App: n.Package}}
				byPkg[n.Package] = d
				order = append(order, n.Package)
			}
			rect, _ := parseBounds(n.Bounds)
			isField := strings.Contains(n.Class, "EditText")
			lbl := n.label()
			switch {
			case isField:
				name := lbl
				if name == "" {
					name = n.ResourceID
				}
				d.Fields = append(d.Fields, name)
				d.fieldRects = append(d.fieldRects, rect)
			case n.Clickable == "true" && lbl != "":
				d.Buttons = append(d.Buttons, lbl)
				d.ButtonIDs = append(d.ButtonIDs, n.ResourceID)
				d.buttonRects = append(d.buttonRects, rect)
			case lbl != "":
				d.Texts = append(d.Texts, lbl)
				if d.Title == "" || strings.HasSuffix(n.ResourceID, "alertTitle") ||
					strings.HasSuffix(n.ResourceID, "permission_message") {
					d.Title = lbl
				}
			}
		}
		for _, c := range n.Nodes {
			walk(c)
		}
	}
	for _, n := range h.Nodes {
		walk(n)
	}

	var out []androidDialog
	for _, p := range order {
		d := byPkg[p]
		// The notification shade, status bar and the keyboard also live in
		// systemui/android: only count a package as a dialog when it actually
		// offers something to press or type into.
		if len(d.Buttons) == 0 && len(d.Fields) == 0 {
			continue
		}
		if p == "com.android.systemui" && !looksLikeDialog(d) {
			continue
		}
		out = append(out, *d)
	}
	return out, nil
}

// looksLikeDialog filters systemui's always-present chrome: a real dialog
// has a title/message plus buttons.
func looksLikeDialog(d *androidDialog) bool {
	return len(d.Texts) > 0 && len(d.Buttons) > 0
}

func parseBounds(s string) (image.Rectangle, error) {
	var x1, y1, x2, y2 int
	if _, err := fmt.Sscanf(s, "[%d,%d][%d,%d]", &x1, &y1, &x2, &y2); err != nil {
		return image.Rectangle{}, err
	}
	return image.Rect(x1, y1, x2, y2), nil
}

func center(r image.Rectangle) (int, int) { return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2 }

// AndroidDriver drives system dialogs through uiautomator and `input`.
type AndroidDriver struct {
	adb    ADB
	serial string
	// settle is the pause between focusing a field and typing into it: input
	// sent before the IME attaches is silently lost.
	settle time.Duration
}

// NewAndroid returns a driver for the device with the given adb serial.
func NewAndroid(adb ADB, serial string) *AndroidDriver {
	return &AndroidDriver{adb: adb, serial: serial, settle: 500 * time.Millisecond}
}

func (a *AndroidDriver) snapshot(ctx context.Context) ([]androidDialog, error) {
	dump, err := a.adb.UIAutomatorDump(ctx, a.serial)
	if err != nil {
		// uiautomator allows a single client at a time; a running Maestro
		// driver or another UiAutomation session makes the dump fail or be killed.
		return nil, fmt.Errorf("%w (if another UI-automation tool such as Maestro's driver is running on this device, stop it: only one uiautomator client can be active)", err)
	}
	return parseAndroid(dump, strings.Split(os.Getenv("PROBE_ANDROID_DIALOG_PACKAGES"), ","))
}

func (a *AndroidDriver) find(ctx context.Context, title string) (*androidDialog, error) {
	ds, err := a.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ds {
		if ds[i].MatchesTitle(title) {
			return &ds[i], nil
		}
	}
	return nil, nil
}

func (a *AndroidDriver) Dialogs(ctx context.Context) ([]Dialog, error) {
	ds, err := a.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Dialog, len(ds))
	for i := range ds {
		out[i] = ds[i].Dialog
	}
	return out, nil
}

func (a *AndroidDriver) See(ctx context.Context, title string) (bool, error) {
	d, err := a.find(ctx, title)
	return d != nil, err
}

func (a *AndroidDriver) Wait(ctx context.Context, title string, appear bool, timeout time.Duration) (bool, error) {
	// A single failed UI dump must not end the wait: keep polling, and only report the dump error
	// if the dialog was never seen before the deadline.
	var lastErr error
	ok, err := pollUntil(ctx, timeout, func() (bool, error) {
		present, err := a.See(ctx, title)
		if err != nil {
			lastErr = err
			return false, nil
		}
		lastErr = nil
		return present == appear, nil
	})
	if err != nil {
		return ok, err
	}
	if !ok && lastErr != nil {
		return false, lastErr
	}
	return ok, nil
}

func (a *AndroidDriver) Tap(ctx context.Context, button, title string) (string, error) {
	d, err := a.find(ctx, title)
	if err != nil {
		return "", err
	}
	if d == nil {
		return "", noDialogErr(title)
	}
	i := MatchButton(d.Dialog, button)
	if i < 0 {
		return "", fmt.Errorf("no button %q in the dialog (buttons: %s)", button, strings.Join(d.Buttons, ", "))
	}
	x, y := center(d.buttonRects[i])
	if err := a.adb.Tap(ctx, a.serial, x, y); err != nil {
		return "", err
	}
	return d.Buttons[i], nil
}

func (a *AndroidDriver) Type(ctx context.Context, field, text, title string) error {
	d, err := a.find(ctx, title)
	if err != nil {
		return err
	}
	if d == nil {
		return noDialogErr(title)
	}
	i := Match(field, d.Fields)
	if i < 0 && len(d.Fields) == 1 {
		i = 0
	}
	if i < 0 {
		return fmt.Errorf("no field %q in the dialog (fields: %s)", field, strings.Join(d.Fields, ", "))
	}
	x, y := center(d.fieldRects[i])
	if err := a.adb.Tap(ctx, a.serial, x, y); err != nil {
		return err
	}
	select {
	case <-time.After(a.settle):
	case <-ctx.Done():
		return ctx.Err()
	}
	// The remote shell would interpret metacharacters in a password
	// (&, ;, $, quotes...), so the text goes in single-quoted. Space is
	// `input text`'s own token separator and is written %s.
	if _, err := a.adb.Shell(ctx, a.serial, "input", "text", ShellQuote(strings.ReplaceAll(text, " ", "%s"))); err != nil {
		return fmt.Errorf("input text failed: %s", Scrub(err.Error(), text))
	}
	return nil
}

func (a *AndroidDriver) Dismiss(ctx context.Context, title string) (bool, error) {
	d, err := a.find(ctx, title)
	if err != nil || d == nil {
		return false, err
	}
	i := DismissButton(d.Dialog)
	if i < 0 {
		return false, fmt.Errorf("no cancel-like button in the dialog (buttons: %s)", strings.Join(d.Buttons, ", "))
	}
	x, y := center(d.buttonRects[i])
	return true, a.adb.Tap(ctx, a.serial, x, y)
}

func (a *AndroidDriver) Close() error { return nil }

// ShellQuote single-quotes s for a POSIX shell.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func noDialogErr(title string) error {
	if title == "" {
		return fmt.Errorf("no system dialog is showing")
	}
	return fmt.Errorf("no system dialog matching %q is showing", title)
}

// pollUntil calls cond every 300ms until it reports true or timeout passes.
// It returns whether cond was satisfied.
func pollUntil(ctx context.Context, timeout time.Duration, cond func() (bool, error)) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := cond()
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}
