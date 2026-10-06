package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// applyGrants pre-grants the permissions named by --grant before the first
// test, so flows never meet the system permission prompt (FP-13). It runs the
// same `allow permission "<name>"` step a test could: `adb shell pm grant` on
// Android, `simctl privacy grant` on iOS simulators — plus the iOS relaunch +
// reconnect, since simctl terminates the app when a permission changes.
//
// iOS "notifications" has no simctl service (FP-16): instead of pre-granting,
// a watcher taps Allow on the system alert whenever it shows up during the
// run. It returns a stop function that must be called when the run ends.
func (r *Runner) applyGrants(ctx context.Context) (stop func(), err error) {
	stop = func() {}
	if len(r.opts.Grant) == 0 || r.opts.DryRun {
		return stop, nil
	}
	if r.deviceCtx == nil {
		fmt.Printf("    \033[33m⚠\033[0m  --grant ignored: no local device context (cloud providers manage permissions through their own capabilities)\n")
		return stop, nil
	}
	exec := r.newExecutor()
	var watchers []func()
	for _, name := range r.opts.Grant {
		if name == "notifications" && r.deviceCtx.Platform == device.PlatformIOS && !r.deviceCtx.IsPhysical {
			watchers = append(watchers, r.watchNotificationAlerts(ctx))
			continue
		}
		step := parser.ActionStep{Verb: parser.VerbAllowPermission, Name: name}
		if err := exec.RunStep(ctx, step); err != nil {
			if errors.Is(err, device.ErrIOSNotificationsUnsupported) {
				fmt.Printf("    \033[33m⚠\033[0m  --grant %s: %v\n", name, err)
				continue
			}
			for _, w := range watchers {
				w()
			}
			return func() {}, fmt.Errorf("--grant %s: %w", name, err)
		}
	}
	return func() {
		for _, w := range watchers {
			w()
		}
	}, nil
}

// watchNotificationAlerts taps "Allow" on the iOS notification-permission
// alert whenever one appears, until the returned stop function is called. It
// is best effort and must never affect the run: every failure is reported once
// as a warning and the watcher gives up.
func (r *Runner) watchNotificationAlerts(ctx context.Context) (stop func()) {
	wctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d, err := r.deviceCtx.SystemDriver(wctx)
		if err != nil {
			fmt.Printf("    \033[33m⚠\033[0m  --grant notifications: cannot watch for the alert: %v\n", err)
			return
		}
		fmt.Printf("    \033[36mℹ\033[0m  --grant notifications: will tap Allow when the iOS notification alert appears\n")
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-wctx.Done():
				return
			case <-tick.C:
			}
			if ok, err := d.See(wctx, "Notifications"); err != nil || !ok {
				continue
			}
			if _, err := d.Tap(wctx, "Allow", "Notifications"); err == nil {
				fmt.Printf("    \033[36mℹ\033[0m  tapped Allow on the iOS notification alert\n")
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}
