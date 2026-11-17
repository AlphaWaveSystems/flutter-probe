package probelink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// screenshotDir is where a screenshot the agent sends as data is saved on the host.
var screenshotDir = filepath.Join("reports", "screenshots")

// simulatorDevicesRoot is the folder that holds every simulator's data. A test replaces it.
var simulatorDevicesRoot = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Developer", "CoreSimulator", "Devices"), nil
}

// savedScreenshot turns an agent's screenshot answer into a host file that the run may read, change and attach. The agent decides what it
// sends, so its path is never trusted as a host path:
//   - With image data (every current agent sends it), the image is saved in screenshotDir under the base name of the agent's path, in a
//     new file: an existing file or a link of that name is never written through.
//   - Without data (an older agent on an iOS simulator, which shares the host's file system), the path is used only when it resolves to a
//     regular .png file inside a simulator's data folder (<simulators>/<UUID>/data/...). Any other path is an error, so an agent cannot make
//     the run read, change or copy another host file.
func savedScreenshot(result ScreenshotResult) (string, error) {
	if result.Data != "" {
		decoded, err := base64Decode(result.Data)
		if err != nil || len(decoded) == 0 {
			return "", errors.New("screenshot: the agent's image data cannot be decoded")
		}
		return saveScreenshotFile(screenshotDir, result.Path, decoded)
	}
	return simulatorScreenshotPath(result.Path)
}

// saveScreenshotFile writes data to a new file in dir named after the base name of agentPath (".png" when it has none usable), adding
// "-2", "-3"... when that name is taken. It returns the absolute path.
func saveScreenshotFile(dir, agentPath string, data []byte) (string, error) {
	base := filepath.Base(filepath.FromSlash(strings.ReplaceAll(agentPath, `\`, "/")))
	if base == "." || base == ".." || base == string(filepath.Separator) || strings.ContainsRune(base, 0) {
		base = "screenshot.png"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i <= 1000; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		p := filepath.Join(dir, name)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("screenshot: %w", err)
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(p)
			return "", fmt.Errorf("screenshot: writing %s: %w", name, errors.Join(werr, cerr))
		}
		if abs, err := filepath.Abs(p); err == nil {
			return abs, nil
		}
		return p, nil
	}
	return "", errors.New("screenshot: no free file name in " + dir)
}

// simulatorScreenshotPath returns p, resolved, when it is a regular .png file inside a simulator's data folder.
func simulatorScreenshotPath(p string) (string, error) {
	refuse := func() (string, error) {
		return "", fmt.Errorf("screenshot: the agent sent no image data, and %q is not a screenshot in a simulator's data folder", p)
	}
	if p == "" || !filepath.IsAbs(p) || !strings.EqualFold(filepath.Ext(p), ".png") {
		return refuse()
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return refuse()
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return refuse()
	}
	root, err := simulatorDevicesRoot()
	if err != nil {
		return refuse()
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return refuse()
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 3 || !validUDID(parts[0]) || parts[1] != "data" {
		return refuse()
	}
	return real, nil
}
