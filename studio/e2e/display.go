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

// targetDisplay resolves the display the suite's windows open on.
// STUDIO_E2E_DISPLAY=n picks display n (1 = main). Unset, the suite uses
// display 2 whenever a second display is attached and the main display
// otherwise. A requested display that does not exist falls back to main with a
// warning.
func targetDisplay() (Display, int) {
	ds, err := displays()
	if err != nil {
		fmt.Fprintln(os.Stderr, "display:", err, "- using main")
		return Display{}, 1
	}
	n := 1
	if v := os.Getenv("STUDIO_E2E_DISPLAY"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			fmt.Fprintf(os.Stderr, "display: invalid STUDIO_E2E_DISPLAY=%q - using main\n", v)
		} else {
			n = k
		}
	} else if len(ds) >= 2 {
		n = 2
	}
	if n > len(ds) {
		fmt.Fprintf(os.Stderr, "display: STUDIO_E2E_DISPLAY=%d but only %d display(s) attached - using main\n", n, len(ds))
		n = 1
	}
	return ds[n-1], n
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

// moveWindowToDisplay places the window of process whose name starts with
// namePrefix (empty = window 1) on the chosen display with a small inset, then
// reads its position back and fails if it did not land there. A no-op on the
// main display.
func moveWindowToDisplay(process, namePrefix string) error {
	ds, err := displays()
	if err != nil {
		return err
	}
	d, n := targetDisplay()
	if n == 1 {
		return nil
	}
	x, y := d.topLeft(ds[0], 40, 60)
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
	if !d.contains(ds[0], px, py) {
		return fmt.Errorf("%s window is at (%d,%d), not on display %d", process, px, py, n)
	}
	fmt.Fprintf(os.Stderr, "display: %s window at (%d,%d) on display %d\n", process, px, py, n)
	return nil
}
