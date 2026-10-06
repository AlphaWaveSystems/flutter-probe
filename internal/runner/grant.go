package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// applyGrants pre-grants the permissions named by --grant before the first
// test, so flows never meet the system permission prompt (FP-13). It runs the
// same `allow permission "<name>"` step a test could, which means the existing
// platform handling applies unchanged: `adb shell pm grant` on Android,
// `simctl privacy grant` on iOS simulators — plus the iOS relaunch +
// reconnect, since simctl terminates the app when a permission changes.
//
// iOS "notifications" cannot be pre-granted (no simctl service); that is
// reported as a warning rather than failing a run whose other platforms can.
func (r *Runner) applyGrants(ctx context.Context) error {
	if len(r.opts.Grant) == 0 || r.opts.DryRun {
		return nil
	}
	if r.deviceCtx == nil {
		fmt.Printf("    \033[33m⚠\033[0m  --grant ignored: no local device context (cloud providers manage permissions through their own capabilities)\n")
		return nil
	}
	exec := r.newExecutor()
	for _, name := range r.opts.Grant {
		step := parser.ActionStep{Verb: parser.VerbAllowPermission, Name: name}
		if err := exec.RunStep(ctx, step); err != nil {
			if errors.Is(err, device.ErrIOSNotificationsUnsupported) {
				fmt.Printf("    \033[33m⚠\033[0m  --grant %s: %v\n", name, err)
				continue
			}
			return fmt.Errorf("--grant %s: %w", name, err)
		}
	}
	return nil
}
