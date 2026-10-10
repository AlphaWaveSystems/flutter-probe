package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Display is one attached screen in AppKit coordinates (origin bottom-left of
// the main screen; System Events window positions use top-left of the main
// screen, converted in topLeft).
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

// targetDisplay resolves STUDIO_E2E_DISPLAY (1 = main, default) to a screen.
// A display that does not exist falls back to main with a warning.
func targetDisplay() (Display, int) {
	ds, err := displays()
	if err != nil {
		fmt.Fprintln(os.Stderr, "display:", err, "- using main")
		return Display{}, 1
	}
	n := 1
	if v := os.Getenv("STUDIO_E2E_DISPLAY"); v != "" {
		if k, err := strconv.Atoi(v); err == nil && k >= 1 {
			n = k
		}
	}
	if n > len(ds) {
		fmt.Fprintf(os.Stderr, "display: STUDIO_E2E_DISPLAY=%d but only %d display(s) attached - using main\n", n, len(ds))
		n = 1
	}
	return ds[n-1], n
}

// topLeft returns the System Events position (top-left origin, main screen
// y down) of a point offset from the display's top-left corner.
func (d Display) topLeft(main Display, dx, dy float64) (int, int) {
	x := d.X + dx
	// AppKit y grows upward from the main screen's bottom; System Events y grows
	// downward from the main screen's top.
	y := (main.H - (d.Y + d.H)) + dy
	return int(x), int(y)
}

// moveWindowToDisplay places window 1 of the named process on the chosen
// display with a small inset. No-op on the main display.
func moveWindowToDisplay(process string) {
	ds, err := displays()
	if err != nil {
		return
	}
	d, n := targetDisplay()
	if n == 1 {
		return
	}
	x, y := d.topLeft(ds[0], 40, 60)
	script := fmt.Sprintf(`tell application "System Events" to tell process %q to set position of window 1 to {%d, %d}`, process, x, y)
	if err := osascript(script); err != nil {
		fmt.Fprintf(os.Stderr, "display: could not move %s window: %v\n", process, err)
	}
}
