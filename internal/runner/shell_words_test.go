//go:build !windows

package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// hostileApp is an app id with a space, a semicolon and a quote: read as shell syntax it would run a command on the device.
const hostileApp = "a b;c'd"

// recordingDevice is a DeviceContext whose adb is a stand-in that records every call (arguments NUL-delimited, then a marker) and succeeds with the given stdout. Nothing else runs on the machine.
func recordingDevice(t *testing.T, stdout string) (*DeviceContext, func() [][]string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	fake := filepath.Join(dir, "adb")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\" >> '" + log + "'; done\nprintf -- '--end--\\0' >> '" + log + "'\nprintf '%s' '" + stdout + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dc := &DeviceContext{
		Manager:        device.NewManagerWithPaths(device.ToolPaths{ADB: fake}),
		Serial:         "emulator-5554",
		Platform:       device.PlatformAndroid,
		AppID:          hostileApp,
		AllowClearData: true,
		RestartDelay:   time.Millisecond,
	}
	read := func() [][]string {
		b, _ := os.ReadFile(log)
		var calls [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00") {
			if l == "--end--" {
				calls = append(calls, cur)
				cur = nil
				continue
			}
			cur = append(cur, l)
		}
		return calls
	}
	return dc, read
}

// deviceWordsOf reads the words after "shell" (or after "exec-out") the way the device's shell does: joined with spaces and read as one command line. The shell runs in a temporary directory.
func deviceWordsOf(t *testing.T, args []string) []string {
	t.Helper()
	i := -1
	for j, a := range args {
		if a == "shell" || a == "exec-out" {
			i = j
			break
		}
	}
	if i < 0 {
		t.Fatalf("not a device shell call: %q", args)
	}
	cmd := exec.Command("sh", "-c", "printf '%s\\0' "+strings.Join(args[i+1:], " "))
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh could not read %q: %v", args[i+1:], err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

func assertDeviceWords(t *testing.T, what string, args []string, want ...string) {
	t.Helper()
	got := deviceWordsOf(t, args)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("%s: the device shell reads %q, want %q", what, got, want)
	}
}

// findCall returns the first recorded call whose device words start with the given words.
func findCall(t *testing.T, calls [][]string, prefix ...string) []string {
	t.Helper()
	for _, c := range calls {
		hasShell := false
		for _, a := range c {
			if a == "shell" || a == "exec-out" {
				hasShell = true
			}
		}
		if !hasShell {
			continue
		}
		w := deviceWordsOf(t, c)
		if len(w) >= len(prefix) && strings.Join(w[:len(prefix)], "\x00") == strings.Join(prefix, "\x00") {
			return c
		}
	}
	t.Fatalf("no call starting with %q among %q", prefix, calls)
	return nil
}

func TestAppIDReachesTheDeviceShellAsOneWordInEveryLifecycleCommand(t *testing.T) {
	ctx := context.Background()
	t.Run("restart", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		_ = dc.RestartApp(ctx)
		assertDeviceWords(t, "force-stop", findCall(t, calls(), "am", "force-stop"), "am", "force-stop", hostileApp)
		// the launch asks the device for the launcher activity of the same app id
		assertDeviceWords(t, "resolve", findCall(t, calls(), "cmd", "package", "resolve-activity"), "cmd", "package", "resolve-activity", "--brief", hostileApp)
	})
	t.Run("clear data", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		_ = dc.ClearAppData(ctx)
		assertDeviceWords(t, "pm clear", findCall(t, calls(), "pm", "clear"), "pm", "clear", hostileApp)
		assertDeviceWords(t, "resolve", findCall(t, calls(), "cmd", "package", "resolve-activity"), "cmd", "package", "resolve-activity", "--brief", hostileApp)
	})
	t.Run("kill", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		_ = dc.KillApp(ctx)
		assertDeviceWords(t, "force-stop", findCall(t, calls(), "am", "force-stop"), "am", "force-stop", hostileApp)
	})
	t.Run("launch", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		_ = dc.LaunchApp(ctx)
		assertDeviceWords(t, "resolve", findCall(t, calls(), "cmd", "package", "resolve-activity"), "cmd", "package", "resolve-activity", "--brief", hostileApp)
	})
	t.Run("reconnect deletes the stale token with run-as", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		dc.ReconnectDelay = time.Millisecond
		dc.TokenReadTimeout = 50 * time.Millisecond
		dc.DialTimeout = 50 * time.Millisecond
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, _ = dc.Reconnect(cctx)
		assertDeviceWords(t, "run-as rm", findCall(t, calls(), "run-as"), "run-as", hostileApp, "rm", "-f", "cache/probe/token")
	})
	t.Run("permissions", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		_ = dc.AllowPermission(ctx, "camera")
		assertDeviceWords(t, "pm grant", findCall(t, calls(), "pm", "grant"), "pm", "grant", hostileApp, "android.permission.CAMERA")
	})
}

func TestDevicePullQuotesTheAppAndRefusesPathsOutsideItsStorage(t *testing.T) {
	// a path inside the app's private storage: the command reads one word for the app id and one for the path
	dc, calls := recordingDevice(t, "")
	dir := t.TempDir()
	results := []TestResult{{Artifacts: []string{"/data/user/0/" + hostileApp + "/cache/shot one;x.png"}}}
	PullArtifacts(context.Background(), results, dc, dir)
	assertDeviceWords(t, "pull", findCall(t, calls(), "run-as"), "run-as", hostileApp, "cat", "/data/user/0/"+hostileApp+"/cache/shot one;x.png")

	// paths that are not inside it never reach the device
	for _, bad := range []string{
		"/etc/passwd",
		"/data/user/0/other.app/cache/x.png",
		"/data/user/0/" + hostileApp + "/../other/x.png",
		"/data/user/0/" + hostileApp + "/cache/../../other/x.png",
		"/data/data/other/x.png",
		"/data/user/abc/" + hostileApp + "/x.png",
		"/data/user//" + hostileApp + "/x.png",
		"/data/user/0/" + hostileApp,
		"/data/user/0/" + hostileApp + "/",
		"cache/x.png",
	} {
		dc2, calls2 := recordingDevice(t, "")
		PullArtifacts(context.Background(), []TestResult{{Artifacts: []string{bad}}}, dc2, t.TempDir())
		for _, c := range calls2() {
			for _, a := range c {
				if a == "exec-out" {
					t.Errorf("a path outside the app's storage reached the device: %q: %q", bad, c)
				}
			}
		}
	}
}

func TestAppPrivateFile(t *testing.T) {
	ok := []string{"/data/user/0/com.example.app/cache/x.png", "/data/user/10/com.example.app/files/a/b.png", "/data/data/com.example.app/cache/x.png"}
	for _, p := range ok {
		if !appPrivateFile("com.example.app", p) {
			t.Errorf("%q refused", p)
		}
	}
	bad := []string{"", "x", "/data/user/0/com.example.app", "/data/user/0/com.example.app/", "/data/data/com.example.app", "/data/data/com.example.app/", "/data/user/0/com.example.app/../x", "/data/user/0/com.example.app//x", "/data/user/0/com.example.app/./x", "/data/user/x/com.example.app/x", "/data/user//com.example.app/x", "/data/user/0/com.example.appx/x", "/data/data/com.example.appx/x", "/sdcard/x"}
	for _, p := range bad {
		if appPrivateFile("com.example.app", p) {
			t.Errorf("%q accepted", p)
		}
	}
	if appPrivateFile("", "/data/data//x") {
		t.Error("an empty app id was accepted")
	}
}

func TestPerfPidofQuotesTheAppID(t *testing.T) {
	dc, calls := recordingDevice(t, "")
	reader := dc.cpuReader(context.Background())
	if reader == nil {
		t.Fatal("no CPU reader for an Android device with an app id")
	}
	_, _ = reader(context.Background()) // the stand-in has no process: the lookup fails after asking for the pid
	assertDeviceWords(t, "pidof", findCall(t, calls(), "pidof"), "pidof", hostileApp)
}

func TestScreenrecordQuotesItsWordsAndRefusesAResolutionThatIsNotWxH(t *testing.T) {
	newRec := func(t *testing.T, res string) (*VideoRecorder, func() [][]string) {
		dc, calls := recordingDevice(t, "")
		vr := &VideoRecorder{
			manager:   dc.Manager,
			serial:    dc.Serial,
			platform:  device.PlatformAndroid,
			outputDir: t.TempDir(),
			videoCfg:  config.VideoConfig{Resolution: res, ScreenrecordCycle: time.Hour, Framerate: 1},
			stopCh:    make(chan struct{}),
		}
		t.Cleanup(func() { close(vr.stopCh); time.Sleep(50 * time.Millisecond) })
		return vr, calls
	}
	vr, calls := newRec(t, "720x1280")
	if err := vr.startScreenrecord(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if vr.cmd != nil {
		_ = vr.cmd.Wait()
	}
	assertDeviceWords(t, "screenrecord", findCall(t, calls(), "screenrecord"), "screenrecord", "--size", "720x1280", "/sdcard/probe_recording.mp4")

	for _, bad := range []string{"720x1280; touch /tmp/X", "720x1280 && id", "$(id)", "720x", "x1280", "720X1280", "7x1280", "1234567x1280", "720x1280\n", "", " 720x1280"} {
		vr, calls := newRec(t, bad)
		_ = vr.startScreenrecord(context.Background(), "t")
		for _, c := range calls() {
			for _, a := range c {
				if a == "screenrecord" {
					t.Errorf("resolution %q reached the device: %q", bad, c)
				}
			}
		}
		if vr.backend != "screencap" {
			t.Errorf("resolution %q: backend = %q, want the screencap fallback", bad, vr.backend)
		}
	}
}
