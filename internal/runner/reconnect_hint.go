package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

const reconnectHintTimeout = 5 * time.Second

// reconnectHint is appended to the error of a step whose reconnect failed. It says what is wrong in the one case that has a cause worth naming, without changing
// anything the reconnect does: it runs after the reconnect attempt has failed, makes two read-only adb calls at most (`pidof`, `forward --list`), and is empty on iOS, on
// a cloud run, when no adb is available or when nothing can be read.
//   - The app has no process: the app is named as the cause. After the test's own `kill the app` it says that; otherwise it says the app ended during the step, and
//     points at the crash evidence when the run asked for it (or at the flag when it did not).
//   - The app runs and the agent port is forwarded to another device: that device is named, with the command that removes the forward.
//
// An `adb forward` of this device's own is never blamed: the port is the CLI's own forward on every Android run.
func (e *Executor) reconnectHint(ctx context.Context) string {
	dc := e.deviceCtx
	if dc == nil || dc.Platform != device.PlatformAndroid || !device.ValidPackageName(dc.AppID) || strings.TrimSpace(dc.Serial) == "" {
		return ""
	}
	var runner device.AdbRunner = dc.CrashADB
	if runner == nil && dc.Manager != nil {
		runner = dc.Manager.ADB()
	}
	if runner == nil || ctx.Err() != nil {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, reconnectHintTimeout)
	defer cancel()
	p := crashProbe{runner: runner, serial: dc.Serial, app: dc.AppID}
	// a failing pidof means no process, but only when the device itself answers: ask its clock first, as the crash look does
	if _, err := p.deviceNow(cctx); err != nil {
		return ""
	}
	if !p.alive(cctx) {
		switch {
		case e.appStopped:
			return fmt.Sprintf(" — the app %s is not running: the test stopped it (`kill the app`); start it again with `open the app` or `restart the app`", dc.AppID)
		case dc.CrashEvidence:
			return fmt.Sprintf(" — the app %s is not running: it ended during this step (see the crash evidence for this test)", dc.AppID)
		default:
			return fmt.Sprintf(" — the app %s is not running: it ended during this step (run with --crash-evidence to see why)", dc.AppID)
		}
	}
	if other, ok := device.ForwardSerial(cctx, runner, dc.Port); ok && other != dc.Serial {
		return fmt.Sprintf(" — agent port %d is forwarded to another device (%s): remove that forward with `adb -s %s forward --remove tcp:%d`, or pick another port with --agent-port", dc.Port, other, other, dc.Port)
	}
	return ""
}
