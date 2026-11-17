package ios

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Ids that must never shape a path. Unicode characters are written as code points, so the source stays plain ASCII.
var hostileUDIDs = []string{
	"../../x",
	"/etc/passwd",
	"..",
	"00008110-001A2C3E0E12801E/../../x",
	"6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B/../../x",
	"x/../6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B",
	"00008110\x00-001A2C3E0E12801E",
	"*",
	"0000811?-001A2C3E0E12801E",
	strings.Repeat("A", 4096),
	"00008110-001A2C3E0E12801" + string(rune(0x0660)), // an Arabic-Indic digit, not hex
	"",
	"short",
}

func TestUDIDForms(t *testing.T) {
	for _, s := range []string{"00008110-001A2C3E0E12801E", "0123456789abcdef0123456789abcdef01234567", "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"} {
		if !ValidDeviceUDID(s) {
			t.Errorf("%q is a device id", s)
		}
	}
	if !ValidSimulatorUDID("6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B") {
		t.Error("a UUID is a simulator id")
	}
	if ValidSimulatorUDID("00008110-001A2C3E0E12801E") {
		t.Error("a device UDID is not a simulator id")
	}
	for _, s := range hostileUDIDs {
		if ValidDeviceUDID(s) || ValidSimulatorUDID(s) {
			t.Errorf("%q must not be accepted", s)
		}
	}
}

// fakeXcrun puts an xcrun on PATH that logs its arguments; for "devicectl device info processes ... --json-output <file>" it writes an
// empty process list to the file, and records the file's path.
func fakeXcrun(t *testing.T) (logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake xcrun is a shell script")
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"prev=''\nfor a in \"$@\"; do if [ \"$prev\" = '--json-output' ]; then printf '{\"result\":{\"runningProcesses\":[]}}' > \"$a\"; printf 'OUT %s\\n' \"$a\" >> '" + logPath + "'; fi; prev=$a; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// A device id that is not one is refused before anything runs: no xcrun call, no file.
func TestFindPIDRefusesAHostileUDID(t *testing.T) {
	logPath := fakeXcrun(t)
	d := NewDeviceCtl()
	for _, u := range hostileUDIDs {
		if _, err := d.FindPID(context.Background(), u, "com.example.app"); err == nil || !strings.Contains(err.Error(), "is not a device id") {
			t.Errorf("FindPID(%q) = %v, want a refusal", u, err)
		}
	}
	if b, _ := os.ReadFile(logPath); len(b) != 0 {
		t.Errorf("xcrun ran for a refused id: %q", b)
	}
}

// devicectl's output goes to a file in a new private folder whose name holds nothing of the id, and the folder is gone afterwards.
func TestFindPIDWritesToAPrivateFolder(t *testing.T) {
	logPath := fakeXcrun(t)
	udid := "00008110-001A2C3E0E12801E"
	if _, err := NewDeviceCtl().FindPID(context.Background(), udid, "com.example.app"); err != nil {
		t.Fatalf("FindPID: %v", err)
	}
	b, _ := os.ReadFile(logPath)
	var out string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "OUT ") {
			out = strings.TrimPrefix(l, "OUT ")
		}
	}
	if out == "" {
		t.Fatalf("no --json-output file was given: %q", b)
	}
	dir := filepath.Dir(out)
	if strings.Contains(out, udid[:8]) || filepath.Base(out) != "processes.json" || !strings.HasPrefix(filepath.Base(dir), "probe-devicectl-") {
		t.Errorf("the output file is %q, want processes.json in a probe-devicectl- folder, with nothing of the id", out)
	}
	if rel, err := filepath.Rel(os.TempDir(), dir); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("the folder %q is not under the temporary folder", dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder %q must be removed after the call: %v", dir, err)
	}
}

// With an id that is not a simulator UDID there is no data path, so no token is read from a path the id or the working directory
// would shape: a token planted where a relative glob would look is not found.
func TestReadTokenIgnoresPathsAHostileUDIDWouldShape(t *testing.T) {
	fakeXcrun(t)
	cwd := t.TempDir()
	planted := filepath.Join(cwd, "Containers", "Data", "Application", "x", "tmp", "probe", "token")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planted, []byte("planted-token-0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
	s := New()
	if p := s.simDataPath("../../x"); p != "" {
		t.Errorf("simDataPath of a hostile id = %q, want none", p)
	}
	if tok, err := s.ReadToken(context.Background(), "../../x", 200*time.Millisecond); err == nil || strings.Contains(tok, "planted") {
		t.Errorf("ReadToken read %q (err %v) for a hostile id", tok, err)
	}
}
