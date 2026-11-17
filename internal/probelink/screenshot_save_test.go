package probelink

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const simUUID = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"

// fakeSimulators points the simulator root at a new folder and returns it.
func fakeSimulators(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := simulatorDevicesRoot
	simulatorDevicesRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { simulatorDevicesRoot = old })
	return root
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An agent that sends no image data and names a host file outside a simulator's data folder gets an error: the run never reads,
// changes or attaches that file.
func TestScreenshotWithoutDataRefusesAHostFile(t *testing.T) {
	root := fakeSimulators(t)
	victim := filepath.Join(t.TempDir(), "photo.png")
	writeFile(t, victim, "VICTIM")
	inSim := filepath.Join(root, simUUID, "data", "Containers", "Data", "Application", "x", "tmp", "shot.png")
	writeFile(t, inSim, "SHOT")
	notUUID := filepath.Join(root, "not-a-uuid", "data", "shot.png")
	writeFile(t, notUUID, "X")
	notPNG := filepath.Join(root, simUUID, "data", "notes.txt")
	writeFile(t, notPNG, "X")
	notData := filepath.Join(root, simUUID, "notdata", "shot.png")
	writeFile(t, notData, "X")
	dirPNG := filepath.Join(root, simUUID, "data", "folder.png")
	if err := os.MkdirAll(dirPNG, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{
		"a host file":                 victim,
		"an absolute system path":     "/etc/passwd",
		"a relative path":             "reports/x.png",
		"a folder that is not a UUID": notUUID,
		"a file that is not a png":    notPNG,
		"outside the data folder":     notData,
		"a folder named like a png":   dirPNG,
		"a traversal out of the sim":  filepath.Join(root, simUUID, "data", "..", "..", "..", filepath.Base(filepath.Dir(victim)), "photo.png"),
		"nothing":                     "",
	} {
		if got, err := savedScreenshot(ScreenshotResult{Path: p}); err == nil {
			t.Errorf("%s: %q was accepted as %q", name, p, got)
		}
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, simUUID, "data", "link.png")
		if err := os.Symlink(victim, link); err != nil {
			t.Fatal(err)
		}
		if got, err := savedScreenshot(ScreenshotResult{Path: link}); err == nil {
			t.Errorf("a link from a simulator's data folder to a host file was accepted as %q", got)
		}
	}
	got, err := savedScreenshot(ScreenshotResult{Path: inSim})
	if real, _ := filepath.EvalSymlinks(inSim); err != nil || got != real {
		t.Errorf("a screenshot in a simulator's data folder must be accepted: %q, %v", got, err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "VICTIM" {
		t.Error("the host file was changed")
	}
}

// With image data the screenshot is a new file in the screenshots folder: the agent's path picks only a base name, and an existing file
// or a planted link of that name is never written through.
func TestScreenshotWithDataIsANewFileInTheScreenshotsFolder(t *testing.T) {
	t.Chdir(t.TempDir())
	data := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	dir, _ := filepath.Abs(screenshotDir)

	for _, agentPath := range []string{"../../../x.png", "/etc/x.png", "..", "", `C:\Windows\x.png`} {
		got, err := savedScreenshot(ScreenshotResult{Path: agentPath, Data: data})
		if err != nil {
			t.Fatalf("%q: %v", agentPath, err)
		}
		if filepath.Dir(got) != dir {
			t.Errorf("%q was saved as %q, want it in %q", agentPath, got, dir)
		}
	}

	victim := filepath.Join(t.TempDir(), "victim.png")
	writeFile(t, victim, "VICTIM")
	writeFile(t, filepath.Join(screenshotDir, "taken.png"), "OLD")
	if runtime.GOOS != "windows" {
		if err := os.Symlink(victim, filepath.Join(screenshotDir, "linked.png")); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"taken.png", "linked.png"} {
		got, err := savedScreenshot(ScreenshotResult{Path: "/data/" + name, Data: data})
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(got) == name || !strings.HasPrefix(filepath.Base(got), strings.TrimSuffix(name, ".png")+"-") {
			t.Errorf("%s: saved as %q, want a new name", name, got)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(screenshotDir, "taken.png")); string(b) != "OLD" {
		t.Error("an existing screenshot was written over")
	}
	if b, _ := os.ReadFile(victim); string(b) != "VICTIM" {
		t.Error("a planted link was written through")
	}
}

// Data that cannot be decoded is an error, not a fallback to the agent's path.
func TestScreenshotWithBadDataIsAnError(t *testing.T) {
	t.Chdir(t.TempDir())
	root := fakeSimulators(t)
	inSim := filepath.Join(root, simUUID, "data", "shot.png")
	writeFile(t, inSim, "SHOT")
	// Even a path that would pass without data is not used: bad data is an error.
	if got, err := savedScreenshot(ScreenshotResult{Path: inSim, Data: "%%%not base64%%%"}); err == nil {
		t.Errorf("undecodable data fell back to %q", got)
	}
}

// A name the agent's path cannot give (".", "..", nothing) is saved as screenshot.png.
func TestScreenshotWithoutAUsableNameIsScreenshotPNG(t *testing.T) {
	t.Chdir(t.TempDir())
	data := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	for _, agentPath := range []string{"..", "", "/"} {
		got, err := savedScreenshot(ScreenshotResult{Path: agentPath, Data: data})
		if err != nil || !strings.HasPrefix(filepath.Base(got), "screenshot") || filepath.Ext(got) != ".png" {
			t.Errorf("%q was saved as %q (%v), want screenshot.png or screenshot-N.png", agentPath, got, err)
		}
	}
}

// Both clients hand the agent's answer to savedScreenshot (the WebSocket client is checked in its source).
func TestBothClientsUseSavedScreenshot(t *testing.T) {
	for _, f := range []string{"client.go", "http_client.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(src), "return savedScreenshot(result)") != 1 || strings.Contains(string(src), "return result.Path, nil") {
			t.Errorf("%s must return savedScreenshot(result) and never the agent's path", f)
		}
	}
	t.Chdir(t.TempDir())
	c := screenshotServer(t, `{"path":"/etc/passwd"}`, nil)
	if got, err := c.Screenshot(context.Background(), "shot"); err == nil {
		t.Errorf("the HTTP client returned the agent's path %q", got)
	}
}
