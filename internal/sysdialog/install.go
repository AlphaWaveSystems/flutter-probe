package sysdialog

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const releaseURLFmt = "https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v%s/probe-ios-driver.zip"

// IOSDriverRoot is where installed runner builds live: ~/.probe/ios-driver.
func IOSDriverRoot() string {
	if d := os.Getenv("PROBE_HOME"); d != "" {
		return filepath.Join(d, "ios-driver")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".probe", "ios-driver")
}

// FindIOSDriverDir locates a runner build: $PROBE_IOS_DRIVER_DIR, then the
// installed build for this version.
func FindIOSDriverDir(version string) (string, error) {
	if d := os.Getenv("PROBE_IOS_DRIVER_DIR"); d != "" {
		if _, err := findXCTestRun(d); err != nil {
			return "", fmt.Errorf("PROBE_IOS_DRIVER_DIR=%s: %w", d, err)
		}
		return d, nil
	}
	dir := filepath.Join(IOSDriverRoot(), version)
	if _, err := findXCTestRun(dir); err == nil {
		return dir, nil
	}
	return "", fmt.Errorf("the iOS system-dialog driver %s is not installed — run `probe ios-driver install` (or build it from source with `make ios-driver` and set PROBE_IOS_DRIVER_DIR)", version)
}

// InstallIOSDriver downloads the runner build for version from the GitHub
// release and unpacks it under ~/.probe/ios-driver/<version>.
func InstallIOSDriver(ctx context.Context, version string) (string, error) {
	if version == "" || strings.Contains(version, "dev") {
		return "", fmt.Errorf("this probe build (%q) has no released iOS driver to download — build it with `make ios-driver` and set PROBE_IOS_DRIVER_DIR", version)
	}
	url := fmt.Sprintf(releaseURLFmt, version)
	return InstallIOSDriverFrom(ctx, url, filepath.Join(IOSDriverRoot(), version))
}

// InstallIOSDriverFrom downloads a runner zip from url into dest.
func InstallIOSDriverFrom(ctx context.Context, url, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading the iOS driver: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading the iOS driver from %s: HTTP %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "probe-ios-driver-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	if err := Unzip(tmp.Name(), dest); err != nil {
		return "", err
	}
	if _, err := findXCTestRun(dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Unzip extracts src into dest, refusing entries that escape dest.
func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening iOS driver archive: %w", err)
	}
	defer r.Close()
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("archive entry %q escapes the install directory", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			link, err := readAll(f)
			if err != nil {
				return err
			}
			resolved := filepath.Join(filepath.Dir(target), string(link))
			if filepath.IsAbs(string(link)) || !strings.HasPrefix(resolved, root) {
				return fmt.Errorf("archive symlink %q points outside the install directory", f.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()|0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 4096))
}
