package device

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// scriptedRunner answers each adb call with what reply returns for its arguments.
type scriptedRunner struct {
	reply func(args []string) ([]byte, error)
}

func (s scriptedRunner) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	return s.reply(args)
}

// fallbackRunner prints no forwarded port, cannot list the rules, and answers the --no-rebind fallback with noRebind.
func fallbackRunner(noRebind error) scriptedRunner {
	return scriptedRunner{func(args []string) ([]byte, error) {
		j := strings.Join(args, " ")
		switch {
		case strings.Contains(j, "--no-rebind"):
			return nil, noRebind
		case strings.Contains(j, "--list"):
			return nil, errors.New("cannot list")
		}
		return []byte(""), nil
	}}
}

// The fallback forward that adb refuses names the port and the serial cleaned.
func TestWebViewFallbackForwardErrorNamesTheSerialCleaned(t *testing.T) {
	_, err := ForwardWebView(context.Background(), fallbackRunner(errors.New("boom")), hostileID, WebViewSocket{PID: 1, Name: "webview_devtools_remote_1"})
	if err == nil || !strings.Contains(err.Error(), "for the WebView socket failed on R5 pass --yes") || !strings.Contains(err.Error(), "adb forward tcp:") {
		t.Fatalf("want the fallback error naming the serial cleaned, got %v", err)
	}
	requireClean(t, "fallback forward", err.Error())
}

// When every fallback port is in use, the error names the serial cleaned and the number of ports tried.
func TestWebViewForwardPortsInUseErrorNamesTheSerialCleaned(t *testing.T) {
	_, err := ForwardWebView(context.Background(), fallbackRunner(errors.New("cannot bind listener: Address already in use")), hostileID,
		WebViewSocket{PID: 1, Name: "webview_devtools_remote_1"})
	if err == nil || !strings.Contains(err.Error(), "failed on R5 pass --yes") || !strings.Contains(err.Error(), "after 5 ports were in use") {
		t.Fatalf("want the ports-in-use error naming the serial cleaned, got %v", err)
	}
	requireClean(t, "ports in use", err.Error())
}

// A removal that fails names the serial cleaned.
func TestWebViewForwardCloseErrorNamesTheSerialCleaned(t *testing.T) {
	f := &WebViewForward{runner: &alwaysFailingRunner{}, serial: hostileID, port: 41234}
	err := f.Close()
	if err == nil || !strings.Contains(err.Error(), "--remove tcp:41234 failed on R5 pass --yes") {
		t.Fatalf("want the removal error naming the serial cleaned, got %v", err)
	}
	requireClean(t, "forward removal", err.Error())
}

// With permissions granted, the "Granting all permissions" line names the app cleaned, and every grant gets the real app id.
func TestAndroidGrantPrintsTheAppCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "")
	logPath := strings.TrimSuffix(adb, "adb") + "calls.log"
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\" >> '" + logPath + "'; done\nprintf -- '--end--\\0' >> '" + logPath + "'\nexit 0\n"
	if err := os.WriteFile(adb, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		_ = NewManagerWithPaths(ToolPaths{ADB: adb}).InstallAndLaunchApp(context.Background(), "emulator-5554", PlatformAndroid, "/tmp/app.apk", hostileAppID, true)
	})
	if !strings.Contains(out, "Granting all permissions for com.example pass --yes") {
		t.Fatalf("want the grant line naming the app cleaned, got %q", out)
	}
	requireClean(t, "grant line", out)
	grants := 0
	for _, c := range calls(t, logPath) {
		j := strings.Join(c, " ")
		if strings.Contains(j, "pm") && strings.Contains(j, "grant") {
			grants++
			if !strings.Contains(j, hostileAppID) {
				t.Errorf("a grant must get the real app id: %q", c)
			}
		}
	}
	if grants == 0 {
		t.Error("no grant call was made")
	}
}

// adb's error names the command it ran cleaned: an argument (here the serial) cannot forge a line.
func TestADBErrorNamesTheArgumentsCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "exit 1\n")
	_, err := NewADBWithPath(adb).RunArgs(context.Background(), "-s", hostileID, "shell", "true")
	if err == nil || !strings.Contains(err.Error(), "-s R5 pass --yes") {
		t.Fatalf("want the adb error naming the arguments cleaned, got %v", err)
	}
	requireClean(t, "adb arguments", err.Error())
}
