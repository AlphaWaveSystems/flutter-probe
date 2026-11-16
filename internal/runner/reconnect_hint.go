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
	runner := dc.adbRunner()
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
	return device.ConnectHint(cctx, runner, dc.Serial, dc.Port, dc.devicePort(), dc.AppID)
}

// adbRunner is the adb the looks of a failed connect use: the one a test set (CrashADB), else the manager's. Nil when there is none.
func (dc *DeviceContext) adbRunner() device.AdbRunner {
	if dc.CrashADB != nil {
		return dc.CrashADB
	}
	if dc.Manager != nil {
		return dc.Manager.ADB()
	}
	return nil
}

// tokenReadHint is appended to a failed token read after a restart (Android): which other app holds the agent port on the device when that can be read, else one
// hedged line without names. It says nothing when the app is not running (the error already says so), on another platform, or when the run is cancelled. At most
// three read-only adb calls, bounded to five seconds.
func (dc *DeviceContext) tokenReadHint(ctx context.Context, readErr error) string {
	if dc == nil || dc.Platform != device.PlatformAndroid || ctx.Err() != nil || readErr == nil || strings.Contains(readErr.Error(), "is not running") {
		return ""
	}
	if h := device.PortOwnerHint(ctx, dc.adbRunner(), dc.Serial, dc.devicePort(), dc.AppID); h != "" {
		return h
	}
	return device.NeutralPortHint
}

// devicePort is the agent's port on the device: DevicePort, or Port when none is set.
func (dc *DeviceContext) devicePort() int {
	if dc.DevicePort != 0 {
		return dc.DevicePort
	}
	return dc.Port
}
