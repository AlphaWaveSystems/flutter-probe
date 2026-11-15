package device

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/shellquote"
)

// ADB wraps the Android Debug Bridge binary.
type ADB struct {
	bin string // path to adb binary
}

// NewADB creates an ADB wrapper using "adb" from PATH.
func NewADB() *ADB {
	return &ADB{bin: "adb"}
}

// NewADBWithPath creates an ADB wrapper using a specific binary path.
func NewADBWithPath(bin string) *ADB {
	if bin == "" {
		bin = "adb"
	}
	return &ADB{bin: bin}
}

// Devices returns all currently connected Android emulators/devices.
func (a *ADB) Devices(ctx context.Context) ([]Device, error) {
	out, err := a.run(ctx, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w", err)
	}

	var devices []Device
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "List of") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		serial := parts[0]
		state := parts[1]
		name := serial
		// Extract "model:" from the long listing
		for _, f := range parts[2:] {
			if strings.HasPrefix(f, "model:") {
				name = strings.ReplaceAll(strings.TrimPrefix(f, "model:"), "_", " ")
				break
			}
		}
		// A running emulator is identified by its AVD name (the name it was
		// created and started under), not by the hardware model it emulates.
		if strings.HasPrefix(serial, "emulator-") && state == "device" {
			if avd := a.AVDName(ctx, serial); avd != "" {
				name = avd
			}
		}
		devices = append(devices, Device{
			ID:       serial,
			Name:     name,
			Platform: PlatformAndroid,
			State:    state,
		})
	}
	return devices, nil
}

// AVDName returns the AVD name of a running emulator ("" when unknown or the
// serial is not an emulator).
func (a *ADB) AVDName(ctx context.Context, serial string) string {
	out, err := a.run(ctx, "-s", serial, "emu", "avd", "name")
	if err != nil {
		return ""
	}
	first := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return strings.TrimSpace(strings.TrimSuffix(first, "\r"))
}

// StartEmulator boots an AVD and returns a Device once it appears.
// bootTimeout controls how long to wait for the emulator to appear (default 120s).
// pollInterval controls how often to check for the device (default 2s).
func (a *ADB) StartEmulator(ctx context.Context, avdName string, bootTimeout, pollInterval time.Duration) (*Device, error) {
	if bootTimeout == 0 {
		bootTimeout = 120 * time.Second
	}
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}

	// Start the emulator in background
	cmd := exec.CommandContext(ctx, "emulator", "-avd", avdName, "-no-audio", "-no-boot-anim")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("adb: emulator start: %w", err)
	}

	// Poll for the device to appear
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		devices, err := a.Devices(ctx)
		if err == nil {
			for _, d := range devices {
				if d.Name == avdName || strings.Contains(d.Name, avdName) || strings.Contains(d.ID, "emulator") {
					return &d, nil
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("adb: emulator %q did not appear within %s", avdName, bootTimeout)
}

// Forward creates an adb forward rule: tcp:hostPort -> tcp:devicePort.
func (a *ADB) Forward(ctx context.Context, serial string, hostPort, devicePort int) error {
	rule := fmt.Sprintf("tcp:%d", hostPort)
	target := fmt.Sprintf("tcp:%d", devicePort)
	_, err := a.run(ctx, "-s", serial, "forward", rule, target)
	if err != nil {
		return fmt.Errorf("adb forward: %w", err)
	}
	return nil
}

// RemoveForward removes an adb forward rule for the given host port.
func (a *ADB) RemoveForward(ctx context.Context, serial string, hostPort int) error {
	rule := fmt.Sprintf("tcp:%d", hostPort)
	_, err := a.run(ctx, "-s", serial, "forward", "--remove", rule)
	return err
}

// Shell runs an adb shell command on the given device.
//
// The device's shell reads the words as one command line: a word that comes from a script, a file name or a configuration file must be quoted (shellquote), or use ShellQ, which does it for every word.
func (a *ADB) Shell(ctx context.Context, serial string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-s", serial, "shell"}, args...)
	return a.run(ctx, cmdArgs...)
}

// ShellQ is Shell with every word passed through shellquote.Arg, so no word can be read as shell syntax. Use it for any command whose words include text that is not a fixed literal of this program.
func (a *ADB) ShellQ(ctx context.Context, serial string, args ...string) ([]byte, error) {
	quoted := make([]string, len(args))
	for i, w := range args {
		quoted[i] = shellquote.Arg(w)
	}
	return a.Shell(ctx, serial, quoted...)
}

// Run executes an adb command (not shell) on the given device.
func (a *ADB) Run(ctx context.Context, serial string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-s", serial}, args...)
	return a.run(ctx, cmdArgs...)
}

// GrantPermission grants a runtime permission to an app.
func (a *ADB) GrantPermission(ctx context.Context, serial, appID, permission string) error {
	_, err := a.ShellQ(ctx, serial, "pm", "grant", appID, permission)
	return err
}

// RevokePermission revokes a runtime permission from an app.
func (a *ADB) RevokePermission(ctx context.Context, serial, appID, permission string) error {
	_, err := a.ShellQ(ctx, serial, "pm", "revoke", appID, permission)
	return err
}

// OpenURL opens url via Android's OS-level intent resolution (am start -a
// VIEW) — the same mechanism a user tapping a link or notification would
// trigger. Unlike the Dart agent's `probe.open_link` (which calls
// url_launcher from inside the running app and always opens an external
// browser/app), this routes through the OS itself, so a custom-scheme or
// App Links URL registered by the app under test is delivered to *that
// app*, not launched externally.
func (a *ADB) OpenURL(ctx context.Context, serial, url string) error {
	return a.OpenURLIn(ctx, serial, url, "")
}

// OpenURLIn fires the VIEW intent for url, restricted to package pkg when set. Without
// the restriction Android picks any app that claims the scheme: with two flavours of
// one app installed (production and staging) the wrong one can start, and the CLI then
// waits for an agent that never comes. If pkg cannot handle the link, the intent is
// retried unrestricted so links meant for other apps (a browser) keep working. The
// URL is single-quoted for the device shell, so characters like & and ? survive.
func (a *ADB) OpenURLIn(ctx context.Context, serial, url, pkg string) error {
	quoted := shellquote.Quote(url)
	if pkg != "" {
		out, err := a.Shell(ctx, serial, "am", "start", "-a", "android.intent.action.VIEW", "-d", quoted, "-p", shellquote.Arg(pkg))
		if err == nil && !strings.Contains(string(out), "Error") {
			return nil
		}
	}
	_, err := a.Shell(ctx, serial, "am", "start", "-a", "android.intent.action.VIEW", "-d", quoted)
	return err
}

// Install installs an APK on the given device, replacing any existing version.
func (a *ADB) Install(ctx context.Context, serial, apkPath string) error {
	_, err := a.run(ctx, "-s", serial, "install", "-r", apkPath)
	if err != nil {
		return fmt.Errorf("adb install: %w", err)
	}
	return nil
}

// LaunchApp starts an app by resolving its launcher activity and launching it.
// Falls back to {package}/.MainActivity if resolution fails.
func (a *ADB) LaunchApp(ctx context.Context, serial, packageName string) error {
	activity := a.ResolveLauncherActivity(ctx, serial, packageName)
	_, err := a.ShellQ(ctx, serial, "am", "start", "-n", activity)
	if err != nil {
		return fmt.Errorf("adb launch: %w", err)
	}
	return nil
}

// ResolveLauncherActivity uses cmd package resolve-activity to find the
// launcher activity for a package. Returns "{package}/{activity}" format.
// Falls back to "{package}/.MainActivity" if resolution fails.
func (a *ADB) ResolveLauncherActivity(ctx context.Context, serial, packageName string) string {
	out, err := a.ShellQ(ctx, serial, "cmd", "package", "resolve-activity", "--brief", packageName)
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, packageName+"/") {
				return line
			}
		}
	}
	return packageName + "/.MainActivity"
}

// GetProp reads a system property from the device.
func (a *ADB) GetProp(ctx context.Context, serial, prop string) (string, error) {
	out, err := a.ShellQ(ctx, serial, "getprop", prop)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetAppVersion returns the versionName for an installed package.
func (a *ADB) GetAppVersion(ctx context.Context, serial, appID string) (string, error) {
	out, err := a.ShellQ(ctx, serial, "dumpsys", "package", appID)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "versionName=") {
			return strings.TrimPrefix(line, "versionName="), nil
		}
	}
	return "", fmt.Errorf("adb: versionName not found for %s", appID)
}

// PackageInstalled reports whether pkg is installed on the device. `pm list packages` filters by substring, so the
// output is matched by exact name.
func (a *ADB) PackageInstalled(ctx context.Context, serial, pkg string) (bool, error) {
	out, err := a.ShellQ(ctx, serial, "pm", "list", "packages", pkg)
	if err != nil {
		return false, err
	}
	return listsPackage(string(out), pkg), nil
}

// listsPackage reports whether the output of `pm list packages` names pkg exactly.
func listsPackage(out, pkg string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "package:"+pkg {
			return true
		}
	}
	return false
}

// Bin returns the path to the adb binary.
func (a *ADB) Bin() string {
	return a.bin
}

// ClearLogcat clears the logcat buffer on the device.
func (a *ADB) ClearLogcat(ctx context.Context, serial string) error {
	_, err := a.run(ctx, "-s", serial, "logcat", "-c")
	return err
}

// Pull copies a file from the device to the host.
func (a *ADB) Pull(ctx context.Context, serial, remotePath, localPath string) error {
	_, err := a.run(ctx, "-s", serial, "pull", remotePath, localPath)
	return err
}

// AddMedia pushes a local file to the device's Pictures directory and
// triggers a media-scanner broadcast so it's immediately visible to the
// gallery/image picker — there's no Android equivalent of iOS's `simctl
// addmedia` that does this in one step.
func (a *ADB) AddMedia(ctx context.Context, serial, localPath string) error {
	remotePath := "/sdcard/Pictures/" + filepath.Base(localPath)
	if _, err := a.run(ctx, "-s", serial, "push", localPath, remotePath); err != nil {
		return fmt.Errorf("adb push: %w", err)
	}
	if _, err := a.ShellQ(ctx, serial, "am", "broadcast", "-a",
		"android.intent.action.MEDIA_SCANNER_SCAN_FILE", "-d", "file://"+remotePath); err != nil {
		return fmt.Errorf("media scanner broadcast: %w", err)
	}
	return nil
}

// UIAutomatorDump returns the current native (non-Flutter) UI hierarchy as
// XML — the same surface `uiautomator dump` gives any Android UI test tool,
// covering pickers, share sheets, and other OS-owned UI the Dart agent can
// never see.
func (a *ADB) UIAutomatorDump(ctx context.Context, serial string) (string, error) {
	const dumpPath = "/sdcard/probe_uidump.xml"
	// A dump fails now and then while the screen is animating or the device is busy ("could not
	// get idle state", a killed UiAutomation client). Retry a few times, and keep the tool's own
	// output so the error says why instead of an empty message. `adb shell` hides the output of a
	// command that exits non-zero, hence the wrapper that always exits 0.
	var last string
	for attempt := 0; attempt < 4; attempt++ {
		out, err := a.Shell(ctx, serial, "sh", "-c", "'uiautomator dump "+dumpPath+" 2>&1; echo'")
		if err == nil && strings.Contains(string(out), "dumped to") {
			xml, rerr := a.Shell(ctx, serial, "cat", dumpPath)
			if rerr != nil {
				return "", fmt.Errorf("uiautomator dump: reading %s: %w", dumpPath, rerr)
			}
			return string(xml), nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = strings.TrimSpace(string(out))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("uiautomator dump failed after 4 attempts: %s", last)
}

// Tap issues a raw coordinate tap via `input tap`. Used internally by
// native-UI verbs after resolving a query to on-screen bounds via
// UIAutomatorDump — ordinary Flutter taps go through the Dart agent
// instead, never this.
func (a *ADB) Tap(ctx context.Context, serial string, x, y int) error {
	_, err := a.Shell(ctx, serial, "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

// InputText types text into whatever native element currently has focus via `input text`.
//
// adb joins the words after "shell" into one line and the device's shell reads it, so text that reaches it unquoted is a command line: "a;reboot" would run reboot. The text is therefore checked by
// ValidateInputText and then passed as ONE single-quoted word (a single quote inside is closed, written as an escaped quote, and reopened), with each space written as %s, which is how `input text` takes a space. Text that cannot be passed
// this way safely is refused, not guessed at: see ValidateInputText. The empty text types nothing and does not call adb.
func (a *ADB) InputText(ctx context.Context, serial, text string) error {
	args, err := InputTextArgs(text)
	if err != nil {
		return err
	}
	if args == nil {
		return nil
	}
	if _, err = a.Shell(ctx, serial, args...); err != nil {
		return maskTypedText(err, text, args[2])
	}
	return nil
}

// minMaskedTextLen is the shortest typed text whose bare forms are masked: a text of one to three characters would mask every occurrence of those characters in the rest of the message ("e" would turn "device" into
// "d<typed text>vic<typed text>"). Its quoted form, which contains the quotes, is always masked.
const minMaskedTextLen = 4

// maskTypedText returns err with the typed text, in every form it appears in the failed command line or in what a device echoes back, replaced by a marker: the quoted word as sent, the text with %s for spaces and its
// quotes escaped for the shell (what a shell shows when it echoes the word without the outer quotes), the text with %s for spaces, and the text as typed. The error of a failed adb call quotes the whole command line,
// and the typed text can be a password.
func maskTypedText(err error, text, quotedWord string) error {
	const marker = "<typed text>"
	inner := strings.ReplaceAll(text, " ", "%s")
	forms := []string{quotedWord}
	if len(text) >= minMaskedTextLen {
		forms = append(forms, strings.ReplaceAll(inner, "'", `'\''`), inner, text)
	}
	var pairs []string
	for _, f := range forms {
		pairs = append(pairs, f, marker)
	}
	return fmt.Errorf("%s", strings.NewReplacer(pairs...).Replace(err.Error()))
}

// ValidateInputText says whether text can be typed through `adb shell input text`. It refuses: control characters (new line, tab, NUL and the rest), because `input text` cannot type them and a new
// line would end the shell command; any character outside ASCII, because `input text` does not support them (use another path for such text; there is none for native fields yet); "%", because
// `input text` reads "%s" as a space and has no way to type a literal "%"; and a text that starts with "-", because it can be read as an option and what `input` does with it was not verified.
func ValidateInputText(text string) error {
	for i, r := range text {
		switch {
		case r < 0x20 || r == 0x7f:
			return fmt.Errorf("type native: the text has a control character at position %d; control characters cannot be typed through adb", i)
		case r > 0x7e:
			return fmt.Errorf("type native: the text has a character outside ASCII at position %d; non-ASCII text is not supported on this path", i)
		case r == '%':
			return fmt.Errorf("type native: the text has a %% character at position %d; adb's input text reads %%s as a space and cannot type a %%", i)
		}
	}
	if strings.HasPrefix(text, "-") {
		return fmt.Errorf("type native: text that starts with \"-\" is not supported on this path")
	}
	return nil
}

// InputTextArgs returns the arguments after "shell" for typing text: input, text and the text as one single-quoted word. It returns nil for the empty text.
func InputTextArgs(text string) ([]string, error) {
	if err := ValidateInputText(text); err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	word := strings.ReplaceAll(text, " ", "%s")
	return []string{"input", "text", shellquote.Quote(word)}, nil
}

// run executes an adb command and returns combined stdout.
func (a *ADB) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", strings.Join(args, " "), ee.Stderr)
		}
		return nil, err
	}
	return out, nil
}

// ListAVDs returns the list of available AVD names.
func (a *ADB) ListAVDs(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "emulator", "-list-avds").Output()
	if err != nil {
		return nil, err
	}
	var avds []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			avds = append(avds, name)
		}
	}
	return avds, nil
}

// SetAppLocale sets the per-app language of pkg (Android 13+ / API 33). An
// empty bcp47 removes the override so the app follows the system language.
func (a *ADB) SetAppLocale(ctx context.Context, serial, pkg, bcp47 string) error {
	sdk, _ := a.GetProp(ctx, serial, "ro.build.version.sdk")
	if n, err := strconv.Atoi(strings.TrimSpace(sdk)); err == nil && n < 33 {
		return fmt.Errorf("set language needs Android 13+ (API 33) for per-app languages; this device is API %d. Use an API 33+ emulator image", n)
	}
	args := []string{"cmd", "locale", "set-app-locales", pkg}
	if bcp47 != "" {
		args = append(args, "--locales", bcp47)
	}
	out, err := a.ShellQ(ctx, serial, args...)
	if err != nil {
		return fmt.Errorf("set-app-locales: %w", err)
	}
	if s := strings.TrimSpace(string(out)); strings.Contains(strings.ToLower(s), "exception") || strings.Contains(strings.ToLower(s), "error") {
		return fmt.Errorf("set-app-locales: %s", s)
	}
	return nil
}
