package probelink

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The port holder names a process of this machine, whose name any program can choose: through the real lsof path, it is printed as
// plain text on one line.
func TestPortHolderNamesAHostileProcessCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PortHolder uses lsof")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf 'p4242\\ncRun\\033[2Jner\\n  pass --yes\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "lsof"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	got := PortHolder(48686)
	if got == "" || strings.ContainsAny(got, "\n\x1b") {
		t.Errorf("the holder must be one clean line: %q", got)
	}
	if !strings.Contains(got, "pid 4242 (Run [2Jner)") {
		t.Errorf("the holder's visible text must stay: %q", got)
	}
}

// An agent's handshake can report any app id. Only a plain application id is kept (plainAppID), so a hostile one never reaches the
// mismatch warning: there is then no warning at all.
func TestHostileHandshakeAppIDNeverReachesTheWarning(t *testing.T) {
	hostile := "com.other\n  pass --yes to continue\x1b[2J" + string(rune(0x202e))
	h := newHandshakeResult(PingResult{AppID: OptionalText(hostile)})
	if h.AppID != "" {
		t.Errorf("a hostile app id must be dropped, kept %q", h.AppID)
	}
	if w := AppIDMismatchWarning(*h, "com.example.app", false); w != "" {
		t.Errorf("no warning may name a dropped id: %q", w)
	}
	plain := newHandshakeResult(PingResult{AppID: OptionalText("com.other.app")})
	if w := AppIDMismatchWarning(*plain, "com.example.app", false); !strings.Contains(w, "com.other.app") {
		t.Errorf("a plain id is still reported: %q", w)
	}
}
