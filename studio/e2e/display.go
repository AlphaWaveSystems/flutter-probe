package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Display is one attached screen in AppKit coordinates (origin bottom-left of
// the main screen). System Events window positions use a top-left origin on
// the main screen, converted in topLeft.
type Display struct {
	X, Y, W, H float64
}

// displays lists the attached screens, main first.
func displays() ([]Display, error) {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e",
		`ObjC.import("AppKit"); var s=$.NSScreen.screens; var o=[]; for (var i=0;i<s.count;i++){var f=s.objectAtIndex(i).frame; o.push([f.origin.x,f.origin.y,f.size.width,f.size.height].join(","))} o.join(";")`).Output()
	if err != nil {
		return nil, fmt.Errorf("NSScreen: %w", err)
	}
	var ds []Display
	for _, rec := range strings.Split(strings.TrimSpace(string(out)), ";") {
		p := strings.Split(rec, ",")
		if len(p) != 4 {
			continue
		}
		var d Display
		d.X, _ = strconv.ParseFloat(p[0], 64)
		d.Y, _ = strconv.ParseFloat(p[1], 64)
		d.W, _ = strconv.ParseFloat(p[2], 64)
		d.H, _ = strconv.ParseFloat(p[3], 64)
		ds = append(ds, d)
	}
	if len(ds) == 0 {
		return nil, fmt.Errorf("no displays reported")
	}
	return ds, nil
}

// resolveDisplay picks the display named by requested (the STUDIO_E2E_DISPLAY
// value) out of the attached displays, main first. It refuses instead of
// falling back: the suite must never run on the screen a person works on, so
// the request has to name an attached display of 2 or more. Every refusal says
// how many displays are attached.
func resolveDisplay(requested string, ds []Display) (Display, int, error) {
	attached := len(ds)
	if requested == "" {
		return Display{}, 0, fmt.Errorf("STUDIO_E2E_DISPLAY is not set: name the second display (2 or more); %d display(s) attached", attached)
	}
	n, err := strconv.Atoi(requested)
	if err != nil || n < 2 {
		return Display{}, 0, fmt.Errorf("STUDIO_E2E_DISPLAY=%q must be a display number of 2 or more (1 is the main display); %d display(s) attached", requested, attached)
	}
	if n > attached {
		return Display{}, 0, fmt.Errorf("STUDIO_E2E_DISPLAY=%d but only %d display(s) attached", n, attached)
	}
	return ds[n-1], n, nil
}

// suiteDisplay lists the attached displays and resolves the one the suite's
// windows open on (see resolveDisplay). It also returns the main display,
// because window positions are converted against it.
func suiteDisplay() (mainScreen, target Display, n int, err error) {
	ds, err := displays()
	if err != nil {
		return Display{}, Display{}, 0, fmt.Errorf("list displays: %w", err)
	}
	target, n, err = resolveDisplay(os.Getenv("STUDIO_E2E_DISPLAY"), ds)
	return ds[0], target, n, err
}

// topLeft converts a point offset from the display's top-left corner into
// System Events coordinates (top-left origin on the main screen, y down).
func (d Display) topLeft(main Display, dx, dy float64) (int, int) {
	return int(d.X + dx), int((main.H - (d.Y + d.H)) + dy)
}

// contains reports whether a System Events point lies on the display.
func (d Display) contains(main Display, x, y int) bool {
	left, top := d.topLeft(main, 0, 0)
	return x >= left && x < left+int(d.W) && y >= top && y < top+int(d.H)
}

// clickArg is the cliclick argument for a left click at an absolute screen
// point. The `=` prefix matters: cliclick reads a bare negative number (any
// point on a display left of or above the main one) as a move RELATIVE to the
// pointer, so without it every click on display 2 lands somewhere else.
func clickArg(x, y int) string {
	return fmt.Sprintf("c:=%d,=%d", x, y)
}

// moveWindowToDisplay places the window of process whose name starts with
// namePrefix (empty = window 1) on the chosen display with a small inset, then
// reads its position back and fails if it did not land there. It never moves a
// window to the main display: the display rule refuses that before this runs.
func moveWindowToDisplay(process, namePrefix string) error {
	mainScreen, d, n, err := suiteDisplay()
	if err != nil {
		return err
	}
	x, y := d.topLeft(mainScreen, 40, 60)
	sel := "window 1"
	if namePrefix != "" {
		sel = fmt.Sprintf(`(first window whose name starts with %q)`, namePrefix)
	}
	script := fmt.Sprintf(`tell application "System Events" to tell process %q
  set position of %s to {%d, %d}
  set p to position of %s
  return (item 1 of p as text) & "," & (item 2 of p as text)
end tell`, process, sel, x, y, sel)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("move %s window to display %d: %v: %s", process, n, err, strings.TrimSpace(string(out)))
	}
	var px, py int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d,%d", &px, &py); err != nil {
		return fmt.Errorf("read back %s window position %q: %w", process, out, err)
	}
	if !d.contains(mainScreen, px, py) {
		return fmt.Errorf("%s window is at (%d,%d), not on display %d", process, px, py, n)
	}
	fmt.Fprintf(os.Stderr, "display: %s window at (%d,%d) on display %d\n", process, px, py, n)
	return nil
}
