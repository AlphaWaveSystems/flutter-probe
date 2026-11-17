package probelink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/confined"
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
//   - Without data (an older agent on an iOS simulator, which shares the host's file system), the file is opened once and read from that
//     open file only when it is a regular .png file with a single name inside a simulator's data folder (<simulators>/<UUID>/data/...),
//     checked on the open file itself (internal/confined), and its bytes are saved like data. No caller ever opens the agent's path, so a
//     link swapped in after the check cannot make the run read, change or copy another host file. Any other path is an error.
func savedScreenshot(result ScreenshotResult) (string, error) {
	if result.Data != "" {
		decoded, err := base64Decode(result.Data)
		if err != nil || len(decoded) == 0 {
			return "", errors.New("screenshot: the agent's image data cannot be decoded")
		}
		return saveScreenshotFile(screenshotDir, result.Path, decoded)
	}
	data, err := simulatorScreenshotBytes(result.Path)
	if err != nil {
		return "", err
	}
	return saveScreenshotFile(screenshotDir, result.Path, data)
}

// screenshotFileName is the file name a screenshot is saved under: the base name of the agent's path, always ending in .png, or
// "screenshot.png" when the base name is not usable as a file name on every system (".", "..", a separator, NUL, a trailing dot or space,
// a name Windows reserves such as CON, NUL, COM1 or LPT1, with or without an extension).
func screenshotFileName(agentPath string) string {
	base := filepath.Base(filepath.FromSlash(strings.ReplaceAll(agentPath, `\`, "/")))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if base == "." || base == ".." || base == string(filepath.Separator) || strings.ContainsRune(base, 0) ||
		strings.HasSuffix(base, ".") || strings.HasSuffix(base, " ") || stem == "" || windowsReserved(stem) {
		return "screenshot.png"
	}
	if !strings.EqualFold(filepath.Ext(base), ".png") {
		return base + ".png"
	}
	return base
}

// windowsReserved reports whether stem (a file name without its extension) is a device name Windows reserves.
func windowsReserved(stem string) bool {
	u := strings.ToUpper(strings.TrimRight(stem, ". "))
	switch u {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(u) == 4 && (strings.HasPrefix(u, "COM") || strings.HasPrefix(u, "LPT")) && u[3] >= '0' && u[3] <= '9' {
		return true
	}
	return false
}

// saveScreenshotFile writes data to a new file in dir named after the base name of agentPath (screenshotFileName), adding
// "-2", "-3"... when that name is taken. It returns the absolute path.
func saveScreenshotFile(dir, agentPath string, data []byte) (string, error) {
	base := screenshotFileName(agentPath)
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

// simulatorScreenshotBytes reads p when it is a regular .png file with a single name inside a simulator's data folder, checked on the
// opened file (confined.ReadFile), and returns its bytes.
func simulatorScreenshotBytes(p string) ([]byte, error) {
	refuse := func() ([]byte, error) {
		return nil, fmt.Errorf("screenshot: the agent sent no image data, and %q is not a screenshot in a simulator's data folder", p)
	}
	if p == "" || !filepath.IsAbs(p) || !strings.EqualFold(filepath.Ext(p), ".png") {
		return refuse()
	}
	root, err := simulatorDevicesRoot()
	if err != nil {
		return refuse()
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	data, _, err := confined.ReadFile(p, func(real string) bool {
		rel, err := filepath.Rel(root, real)
		if err != nil {
			return false
		}
		parts := strings.Split(rel, string(filepath.Separator))
		return len(parts) >= 3 && validUDID(parts[0]) && parts[1] == "data"
	})
	if err != nil {
		return refuse()
	}
	return data, nil
}
