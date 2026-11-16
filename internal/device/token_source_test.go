package device

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// sourceADB writes a script that stands in for adb and answers the three token sources the way a device does: run-as answers with runAs (or fails when it is empty),
// the shared file with tmp, and logcat with a PROBE_TOKEN= line when logcat is not empty.
func sourceADB(t *testing.T, runAs, tmp, logcat string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for adb")
	}
	runAsCase := `exit 1`
	if runAs != "" {
		runAsCase = `echo "` + runAs + `"`
	}
	logLine := ""
	if logcat != "" {
		logLine = `echo "07-05 07:26:33.593 13547 13547 I flutter : PROBE_TOKEN=` + logcat + `"`
	}
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"  *run-as*) " + runAsCase + ";;\n" +
		"  *probe/token*) echo \"" + tmp + "\";;\n" +
		"  *logcat*) " + logLine + ";;\n" +
		"  *pidof*) echo 1234;;\n" +
		"  *) exit 0;;\nesac\n"
	path := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadTokenAndroidSourceSaysWhereTheTokenCameFrom(t *testing.T) {
	const runAs, tmp, log = "r1r1r1r1r1r1r1r1r1r1", "t2t2t2t2t2t2t2t2t2t2", "l3l3l3l3l3l3l3l3l3l3"
	cases := []struct {
		name       string
		a, b, c    string
		appID      string
		wantToken  string
		wantSource TokenSource
	}{
		{"run-as first", runAs, tmp, log, "com.example.app", runAs, TokenSourceRunAs},
		{"run-as fails: the shared file", "", tmp, log, "com.example.app", tmp, TokenSourceTmpFile},
		{"no app id: the shared file, never run-as", runAs, tmp, log, "", tmp, TokenSourceTmpFile},
		{"only the log has one", "", "", log, "com.example.app", log, TokenSourceLogcat},
	}
	for _, tc := range cases {
		m := NewManagerWithPaths(ToolPaths{ADB: sourceADB(t, tc.a, tc.b, tc.c)})
		got, src, err := m.ReadTokenAndroidSource(context.Background(), "emulator-5554", 2*time.Second, tc.appID, nil)
		if err != nil || got != tc.wantToken || src != tc.wantSource {
			t.Errorf("%s: got (%q, %v, %v), want (%q, %v, nil)", tc.name, got, src, err, tc.wantToken, tc.wantSource)
		}
		// the old function answers the same token
		old, err := m.ReadTokenAndroid(context.Background(), "emulator-5554", 2*time.Second, tc.appID, nil)
		if err != nil || old != tc.wantToken {
			t.Errorf("%s: ReadTokenAndroid = (%q, %v), want %q", tc.name, old, err, tc.wantToken)
		}
	}
}

func TestReadTokenAndroidSourceOnFailureHasNoSource(t *testing.T) {
	m := NewManagerWithPaths(ToolPaths{ADB: sourceADB(t, "", "", "")})
	got, src, err := m.ReadTokenAndroidSource(context.Background(), "emulator-5554", 300*time.Millisecond, "", nil)
	if err == nil || got != "" || src != TokenSourceNone {
		t.Errorf("got (%q, %v, %v), want an error with no token and TokenSourceNone", got, src, err)
	}
}
