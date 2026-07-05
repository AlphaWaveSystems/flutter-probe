package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

// generateToken creates a random 32-character hex token for pre-shared restart tokens.
func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Executor walks an AST body and dispatches commands to a ProbeLink client.
type Executor struct {
	client      probelink.ProbeClient
	deviceCtx   *DeviceContext                // nil in dry-run mode
	onReconnect func(probelink.ProbeClient)       // callback to update Runner's client ref
	timeout     time.Duration
	recipes     map[string]parser.RecipeDef   // loaded recipes by name
	vars        map[string]string             // variable scope for data-driven tests
	verbose     bool
	depth       int      // indentation depth for verbose logging
	artifacts   []string // collected screenshot paths (on-device)
	visual      *visual.Comparator // nil if visual regression is not configured
	maxReconnectAttempts int           // max auto-reconnect attempts per call (default 4)
	reconnectBackoff     time.Duration // base delay for exponential reconnect backoff (default 1s)
	reconnectMu          sync.Mutex    // serializes concurrent tryReconnect calls
	clientGen            atomic.Uint64 // incremented on each successful reconnect
	launchTimeout        time.Duration // bounds restart/clear-data force-stop+relaunch+reconnect (default 120s, from agent.launch_timeout)
}

// NewExecutor creates an Executor.
func NewExecutor(client probelink.ProbeClient, deviceCtx *DeviceContext, onReconnect func(probelink.ProbeClient), timeout time.Duration, verbose bool) *Executor {
	return &Executor{
		client:      client,
		deviceCtx:   deviceCtx,
		onReconnect: onReconnect,
		timeout:     timeout,
		recipes:     make(map[string]parser.RecipeDef),
		vars:        make(map[string]string),
		verbose:     verbose,
	}
}

// SetReconnectPolicy configures the auto-reconnect behavior. Both values are taken
// from agent.reconnect_attempts and agent.reconnect_backoff in probe.yaml.
// Zero values fall back to defaults (4 attempts, 1s base) when first used.
func (e *Executor) SetReconnectPolicy(attempts int, backoff time.Duration) {
	e.maxReconnectAttempts = attempts
	e.reconnectBackoff = backoff
}

// defaultLaunchTimeout is used when SetLaunchTimeout is never called (e.g. dry-run mode)
// or is called with a zero value.
const defaultLaunchTimeout = 120 * time.Second

// SetLaunchTimeout configures the timeout applied to `restart the app`/`clear app data`,
// taken from agent.launch_timeout in probe.yaml. A zero value falls back to the default.
func (e *Executor) SetLaunchTimeout(d time.Duration) {
	e.launchTimeout = d
}

// launchTimeoutOrDefault returns the configured launch timeout, or defaultLaunchTimeout
// if it was never set.
func (e *Executor) launchTimeoutOrDefault() time.Duration {
	if e.launchTimeout <= 0 {
		return defaultLaunchTimeout
	}
	return e.launchTimeout
}

// RegisterRecipe adds a recipe to the executor's scope.
func (e *Executor) RegisterRecipe(r parser.RecipeDef) {
	e.recipes[r.Name] = r
}

// SetVar sets a variable (used for data-driven row substitution).
func (e *Executor) SetVar(key, value string) {
	e.vars[key] = value
}

// SetVisual configures visual regression comparison for this executor.
func (e *Executor) SetVisual(c *visual.Comparator) {
	e.visual = c
}

// Artifacts returns the list of screenshot paths collected during execution.
func (e *Executor) Artifacts() []string {
	return e.artifacts
}

// AddArtifact appends a path to the collected artifacts.
func (e *Executor) AddArtifact(path string) {
	e.artifacts = append(e.artifacts, path)
}

// RunBody executes a slice of steps sequentially.
func (e *Executor) RunBody(ctx context.Context, steps []parser.Step) error {
	for _, step := range steps {
		if err := e.runStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

// RunStep executes a single step. CompositeRunner uses this to interleave
// device steps with sync barrier coordination.
func (e *Executor) RunStep(ctx context.Context, step parser.Step) error {
	return e.runStep(ctx, step)
}

func (e *Executor) runStep(ctx context.Context, step parser.Step) error {
	// Use a longer timeout for restart/clear — they kill the app and reconnect
	stepTimeout := e.timeout
	isLifecycleAction := false
	if a, ok := step.(parser.ActionStep); ok {
		if a.Verb == parser.VerbRestart || a.Verb == parser.VerbClearAppData {
			stepTimeout = e.launchTimeoutOrDefault()
		}
		// Don't auto-reconnect for actions that intentionally close the connection
		if a.Verb == parser.VerbRestart || a.Verb == parser.VerbClearAppData || a.Verb == parser.VerbKill {
			isLifecycleAction = true
		}
	}
	stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()

	start := time.Now()
	desc := e.stepDescription(step)

	// Real-time feedback: print the step description before it runs.
	if desc != "" {
		if e.verbose {
			PrintStepBefore(e.depth, desc)
		} else {
			PrintCurrentStep(desc)
		}
	}

	// Launch a ticker goroutine that prints progress lines every 5 seconds for
	// slow steps. It also emits a one-time warning at 80% of the step timeout.
	// The goroutine is always started when there's a description — it exits
	// immediately via <-stopTicker if the step finishes fast (< 5s).
	var extraLines atomic.Int32
	stopTicker := make(chan struct{})
	tickerDone := make(chan struct{})
	if desc != "" {
		go runStepTicker(stepCtx, desc, e.depth, 5*time.Second, stopTicker, tickerDone, &extraLines)
	}

	var err error
	switch s := step.(type) {
	case parser.ActionStep:
		err = e.runAction(stepCtx, s)
	case parser.AssertStep:
		err = e.runAssert(stepCtx, s)
	case parser.WaitStep:
		err = e.runWait(stepCtx, s)
	case parser.ConditionalStep:
		err = e.runConditional(ctx, s) // pass parent ctx — conditional manages its own timeout
	case parser.LoopStep:
		err = e.runLoop(ctx, s)
	case parser.DartBlock:
		err = e.runDart(stepCtx, s)
	case parser.MockBlock:
		err = e.runMock(stepCtx, s)
	case parser.RecipeCall:
		err = e.runRecipeCall(ctx, s)
	case parser.HTTPCallStep:
		err = e.runHTTPCall(stepCtx, s)
	}

	// Auto-reconnect: if the step failed due to a connection error and this
	// isn't a lifecycle action (restart/kill/clear), try to reconnect and retry.
	// Uses exponential backoff with jitter — see tryReconnect for the formula.
	if err != nil && !isLifecycleAction && isConnectionError(err) && e.deviceCtx != nil {
		maxAttempts := e.maxReconnectAttempts
		if maxAttempts == 0 {
			maxAttempts = 4
		}
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if reconnErr := e.tryReconnect(ctx, attempt); reconnErr != nil {
				err = fmt.Errorf("%w (auto-reconnect attempt %d failed: %v)", err, attempt, reconnErr)
				break
			}
			// Retry the step with a fresh timeout
			retryCtx, retryCancel := context.WithTimeout(ctx, stepTimeout)
			switch s := step.(type) {
			case parser.ActionStep:
				err = e.runAction(retryCtx, s)
			case parser.AssertStep:
				err = e.runAssert(retryCtx, s)
			case parser.WaitStep:
				err = e.runWait(retryCtx, s)
			case parser.DartBlock:
				err = e.runDart(retryCtx, s)
			case parser.MockBlock:
				err = e.runMock(retryCtx, s)
			}
			retryCancel()
			if err == nil || !isConnectionError(err) {
				break // either succeeded or a non-connection error
			}
		}
	}

	// Stop the ticker goroutine and wait for it to fully exit before reading
	// extraLines or printing the result — this eliminates any output race.
	if desc != "" {
		close(stopTicker)
		<-tickerDone
	}

	if desc != "" {
		elapsed := time.Since(start)
		if e.verbose {
			PrintStepAfterN(e.depth, desc, elapsed, err, int(extraLines.Load()))
		} else {
			ClearCurrentStep()
		}
	}

	return err
}

// runStepTicker is run in a goroutine by runStep. It emits ⏱ progress lines
// every tickInterval for slow steps and a one-time ⚠ warning at 80% of the
// step's context deadline. It exits when stop is closed or ctx is cancelled.
func runStepTicker(
	ctx context.Context,
	desc string,
	depth int,
	tickInterval time.Duration,
	stop <-chan struct{},
	done chan<- struct{},
	extraLines *atomic.Int32,
) {
	defer close(done)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	goroutineStart := time.Now()
	warnPrinted := false

	// Derive the total step timeout from the context deadline so we can emit
	// the 80% warning at the right moment.
	var timeoutDur time.Duration
	if deadline, ok := ctx.Deadline(); ok {
		timeoutDur = deadline.Sub(goroutineStart)
	}

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			elapsed := t.Sub(goroutineStart)
			PrintStepTick(depth, desc, elapsed)
			extraLines.Add(1)

			// Emit the 80% warning exactly once, after the threshold is crossed.
			if !warnPrinted && timeoutDur > 0 {
				threshold := time.Duration(float64(timeoutDur) * 0.80)
				if elapsed >= threshold {
					PrintStepWarning(depth, desc, elapsed, timeoutDur)
					extraLines.Add(1)
					warnPrinted = true
				}
			}
		}
	}
}

// stepDescription returns a human-readable description of the step.
func (e *Executor) stepDescription(step parser.Step) string {
	switch s := step.(type) {
	case parser.ActionStep:
		switch s.Verb {
		case parser.VerbTap:
			if s.Sel != nil {
				return fmt.Sprintf("tap %q", s.Sel.Text)
			}
		case parser.VerbDoubleTap:
			if s.Sel != nil {
				return fmt.Sprintf("double tap %q", s.Sel.Text)
			}
		case parser.VerbType:
			target := ""
			if s.Sel != nil {
				target = fmt.Sprintf(" into %q", s.Sel.Text)
			}
			return fmt.Sprintf("type %q%s", e.resolve(s.Text), target)
		case parser.VerbClear:
			if s.Sel != nil {
				return fmt.Sprintf("clear %q", s.Sel.Text)
			}
		case parser.VerbSwipe:
			return fmt.Sprintf("swipe %s", s.Direction)
		case parser.VerbScroll:
			return fmt.Sprintf("scroll %s", s.Direction)
		case parser.VerbOpen:
			return "open the app"
		case parser.VerbClose:
			return "close the app"
		case parser.VerbGoBack:
			return "go back"
		case parser.VerbTakeShot:
			return fmt.Sprintf("screenshot %q", s.Name)
		case parser.VerbCompareShot:
			return fmt.Sprintf("compare screenshot %q", s.Name)
		case parser.VerbDumpTree:
			return "dump tree"
		case parser.VerbLog:
			return fmt.Sprintf("log %q", s.Name)
		case parser.VerbPause:
			return "pause"
		case parser.VerbRestart:
			return "restart the app"
		case parser.VerbClearAppData:
			return "clear app data"
		case parser.VerbAllowPermission:
			return fmt.Sprintf("allow permission %q", s.Name)
		case parser.VerbDenyPermission:
			return fmt.Sprintf("deny permission %q", s.Name)
		case parser.VerbGrantAllPerms:
			return "grant all permissions"
		case parser.VerbRevokeAllPerms:
			return "revoke all permissions"
		case parser.VerbLongPress:
			if s.Sel != nil {
				return fmt.Sprintf("long press %q", s.Sel.Text)
			}
		case parser.VerbKill:
			return "kill the app"
		case parser.VerbCopyClipboard:
			return fmt.Sprintf("copy %q to clipboard", s.Text)
		case parser.VerbPasteClipboard:
			return "paste from clipboard"
		case parser.VerbSetLocation:
			return fmt.Sprintf("set location %s", s.Name)
		case parser.VerbVerifyBrowser:
			return "verify external browser opened"
		case parser.VerbEnrollBiometric:
			return "enroll biometric"
		case parser.VerbBiometricMatch:
			return "biometric match"
		case parser.VerbBiometricNoMatch:
			return "biometric no match"
		case parser.VerbDeliverSignal:
			return fmt.Sprintf("deliver signal %q", s.Name)
		default:
			return string(s.Verb)
		}
	case parser.AssertStep:
		neg := ""
		if s.Negated {
			neg = "don't "
		}
		return fmt.Sprintf("%ssee %q", neg, s.Sel.Text)
	case parser.WaitStep:
		switch s.Kind {
		case parser.WaitDuration:
			return fmt.Sprintf("wait %.0f seconds", s.Duration)
		case parser.WaitAppears:
			return fmt.Sprintf("wait until %q appears", s.Target)
		case parser.WaitDisappears:
			return fmt.Sprintf("wait until %q disappears", s.Target)
		case parser.WaitPageLoad:
			return "wait for page to load"
		case parser.WaitNetworkIdle:
			return "wait for network idle"
		case parser.WaitAnimations:
			return "wait for animations to end"
		default:
			return "wait"
		}
	case parser.ConditionalStep:
		return fmt.Sprintf("if %q appears", s.Condition)
	case parser.RecipeCall:
		return s.Name
	case parser.LoopStep:
		return fmt.Sprintf("repeat %d times", s.Count)
	case parser.HTTPCallStep:
		return fmt.Sprintf("call %s %q", s.Method, s.URL)
	}
	return ""
}

// ---- Action execution ----

func (e *Executor) runAction(ctx context.Context, a parser.ActionStep) error {
	// "if visible" suffix: check if the selector is visible before executing.
	// If not visible, skip silently (no error). Connection errors propagate.
	if a.IfVisible && a.Sel != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		err := e.client.See(checkCtx, probelink.SeeParams{
			Selector: toSelectorParam(*a.Sel),
		})
		cancel()
		if err != nil {
			if isConnectionError(err) {
				return err // propagate connection errors for auto-reconnect
			}
			return nil // not visible — skip silently
		}
	}

	switch a.Verb {
	case parser.VerbOpen:
		screen := ""
		if a.Sel != nil {
			screen = a.Sel.Text
		}
		err := e.client.Open(ctx, screen)
		if err == nil || !isConnectionError(err) || e.deviceCtx == nil {
			return err
		}
		// PT-09 (found while documenting cross-test state behavior): `open the
		// app` after `kill the app` closes the connection but never actually
		// relaunches — this just sent an RPC over the now-dead client, which
		// failed, and runStep's generic reconnect-on-error path only re-dials
		// assuming the app process is already running again on its own. It
		// never was (kill force-stopped it), so this used to retry a dial to
		// a genuinely dead process until it gave up. Launch the app for real
		// here, the same way `restart the app` does, before reconnecting.
		if err := e.deviceCtx.RestartApp(ctx); err != nil {
			return fmt.Errorf("open the app: %w", err)
		}
		newClient, reconnErr := e.deviceCtx.Reconnect(ctx)
		if reconnErr != nil {
			return fmt.Errorf("open the app: %w", reconnErr)
		}
		e.client = newClient
		if e.onReconnect != nil {
			e.onReconnect(newClient)
		}
		return nil

	case parser.VerbTap:
		if a.Sel == nil {
			return fmt.Errorf("tap: missing selector at line %d", a.Line)
		}
		return e.client.Tap(ctx, toSelectorParam(*a.Sel))

	case parser.VerbDoubleTap:
		if a.Sel == nil {
			return fmt.Errorf("double tap: missing selector at line %d", a.Line)
		}
		return e.client.DoubleTap(ctx, toSelectorParam(*a.Sel))

	case parser.VerbLongPress:
		if a.Sel == nil {
			return fmt.Errorf("long press: missing selector at line %d", a.Line)
		}
		return e.client.LongPress(ctx, toSelectorParam(*a.Sel))

	case parser.VerbType:
		text := e.resolve(a.Text)
		var sel probelink.SelectorParam
		if a.Sel != nil {
			sel = toSelectorParam(*a.Sel)
		}
		return e.client.TypeText(ctx, sel, text)

	case parser.VerbClear:
		if a.Sel == nil {
			return nil
		}
		return e.client.Clear(ctx, toSelectorParam(*a.Sel))

	case parser.VerbSwipe:
		var sel *probelink.SelectorParam
		if a.Sel != nil {
			sp := toSelectorParam(*a.Sel)
			sel = &sp
		}
		return e.client.Swipe(ctx, string(a.Direction), sel)

	case parser.VerbScroll:
		var sel *probelink.SelectorParam
		if a.Sel != nil {
			sp := toSelectorParam(*a.Sel)
			sel = &sp
		}
		return e.client.Scroll(ctx, string(a.Direction), sel)

	case parser.VerbGoBack:
		return e.client.DeviceAction(ctx, "go_back", "")

	case parser.VerbClose:
		return e.client.DeviceAction(ctx, "close", a.Name)

	case parser.VerbDrag:
		if a.Sel == nil || a.To == nil {
			return nil
		}
		type dragParams struct {
			From probelink.SelectorParam `json:"from"`
			To   probelink.SelectorParam `json:"to"`
		}
		_, err := e.client.Call(ctx, probelink.MethodDrag, dragParams{
			From: toSelectorParam(*a.Sel),
			To:   toSelectorParam(*a.To),
		})
		return err

	case parser.VerbRotate:
		return e.client.DeviceAction(ctx, "rotate", a.Name)

	case parser.VerbToggle:
		return e.client.DeviceAction(ctx, "toggle", a.Name)

	case parser.VerbShake:
		return e.client.DeviceAction(ctx, "shake", "")

	case parser.VerbTakeShot:
		path, err := e.client.Screenshot(ctx, a.Name)
		if err == nil && path != "" {
			e.artifacts = append(e.artifacts, path)
		}
		return err

	case parser.VerbCompareShot:
		path, err := e.client.Screenshot(ctx, a.Name)
		if err != nil {
			return fmt.Errorf("compare screenshot: take screenshot failed: %w", err)
		}
		if path != "" {
			e.artifacts = append(e.artifacts, path)
		}
		if e.visual != nil && path != "" {
			result, cmpErr := e.visual.Compare(a.Name, path)
			if cmpErr != nil {
				return fmt.Errorf("compare screenshot %q: %w", a.Name, cmpErr)
			}
			if !result.Passed {
				return fmt.Errorf("visual regression: %q differs by %.2f%% (threshold %.2f%%), diff: %s",
					a.Name, result.DiffPercent, result.Threshold, result.DiffPath)
			}
			if result.DiffPath != "" {
				e.artifacts = append(e.artifacts, result.DiffPath)
			}
		}
		return nil

	case parser.VerbDumpTree:
		tree, err := e.client.DumpWidgetTree(ctx)
		if err == nil {
			fmt.Printf("[widget_tree]\n%s\n[/widget_tree]\n", tree)
		}
		return err

	case parser.VerbSaveLogs:
		return e.client.SaveLogs(ctx)

	case parser.VerbPause:
		time.Sleep(1 * time.Second)
		return nil

	case parser.VerbLog:
		fmt.Printf("  [log] %s\n", a.Name)
		return nil

	case parser.VerbRestart:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping restart (cloud mode — not supported without ADB/simctl)")
			return nil
		}
		// Pre-share a token before restart so we can reconnect without
		// reading device logs (critical for WiFi mode where idevicesyslog
		// is unavailable). The agent persists it and uses it after restart.
		nextToken := generateToken()
		if err := e.client.SetNextToken(ctx, nextToken); err != nil {
			// Non-fatal: fall back to normal token reading
			fmt.Printf("    \033[33m⚠\033[0m  pre-share token: %v (will read from logs)\n", err)
			nextToken = ""
		}
		e.client.Close()
		if err := e.deviceCtx.RestartApp(ctx); err != nil {
			return err
		}
		var newClient probelink.ProbeClient
		var reconnErr error
		if nextToken != "" {
			newClient, reconnErr = e.deviceCtx.ReconnectWithToken(ctx, nextToken)
		} else {
			newClient, reconnErr = e.deviceCtx.Reconnect(ctx)
		}
		if reconnErr != nil {
			return fmt.Errorf("restart the app: %w", reconnErr)
		}
		e.client = newClient
		if e.onReconnect != nil {
			e.onReconnect(newClient)
		}
		return nil

	case parser.VerbClearAppData:
		if e.deviceCtx == nil {
			// In cloud mode, there's no device context (no ADB/simctl).
			// The app is already fresh since it was just installed on the cloud device.
			fmt.Println("    \033[33m⚠\033[0m  Skipping clear app data (cloud mode — app is already fresh)")
			return nil
		}
		e.client.Close()
		if err := e.deviceCtx.ClearAppData(ctx); err != nil {
			return err
		}
		newClient, err := e.deviceCtx.Reconnect(ctx)
		if err != nil {
			return fmt.Errorf("clear app data: %w", err)
		}
		e.client = newClient
		if e.onReconnect != nil {
			e.onReconnect(newClient)
		}
		return nil

	case parser.VerbAllowPermission:
		if e.deviceCtx == nil {
			// Cloud mode: permissions auto-granted via Appium capabilities
			return nil
		}
		return e.deviceCtx.AllowPermission(ctx, a.Name)

	case parser.VerbDenyPermission:
		if e.deviceCtx == nil {
			return nil
		}
		return e.deviceCtx.DenyPermission(ctx, a.Name)

	case parser.VerbGrantAllPerms:
		if e.deviceCtx == nil {
			return nil
		}
		return e.deviceCtx.GrantAllPermissions(ctx)

	case parser.VerbRevokeAllPerms:
		if e.deviceCtx == nil {
			return nil
		}
		return e.deviceCtx.RevokeAllPermissions(ctx)

	case parser.VerbKill:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping kill (cloud mode)")
			return nil
		}
		e.client.Close()
		return e.deviceCtx.KillApp(ctx)

	case parser.VerbCopyClipboard:
		return e.client.CopyToClipboard(ctx, e.resolve(a.Text))

	case parser.VerbPasteClipboard:
		text, err := e.client.PasteFromClipboard(ctx)
		if err != nil {
			return err
		}
		e.vars["clipboard"] = text
		return nil

	case parser.VerbSetLocation:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping set location (cloud mode)")
			return nil
		}
		parts := strings.SplitN(a.Name, ",", 2)
		if len(parts) != 2 {
			return fmt.Errorf("set location: expected lat,lng but got %q", a.Name)
		}
		return e.deviceCtx.SetLocation(ctx, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))

	case parser.VerbVerifyBrowser:
		return e.client.VerifyBrowser(ctx)

	case parser.VerbOpenLink:
		return e.client.OpenLink(ctx, e.resolve(a.Name))

	case parser.VerbStore:
		e.vars[a.Name] = e.resolve(a.Text)
		return nil

	case parser.VerbEnrollBiometric:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping enroll biometric (cloud mode)")
			return nil
		}
		return e.deviceCtx.EnrollBiometric(ctx)

	case parser.VerbBiometricMatch:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping biometric match (cloud mode)")
			return nil
		}
		if err := e.deviceCtx.BiometricMatch(ctx); err != nil {
			return err
		}
		_, err := e.client.Call(ctx, probelink.MethodBiometricSignal, map[string]any{"result": true})
		return err

	case parser.VerbBiometricNoMatch:
		if e.deviceCtx == nil {
			fmt.Println("    \033[33m⚠\033[0m  Skipping biometric no-match (cloud mode)")
			return nil
		}
		if err := e.deviceCtx.BiometricNoMatch(ctx); err != nil {
			return err
		}
		_, err := e.client.Call(ctx, probelink.MethodBiometricSignal, map[string]any{"result": false})
		return err

	case parser.VerbDeliverSignal:
		value := a.Text
		if value == "" {
			value = "true"
		}
		_, err := e.client.Call(ctx, probelink.MethodSignal, map[string]any{
			"name":  a.Name,
			"value": value,
		})
		return err
	}

	return fmt.Errorf("unknown action verb %q at line %d", a.Verb, a.Line)
}

// ---- Assert execution ----

func (e *Executor) runAssert(ctx context.Context, a parser.AssertStep) error {
	checkStr := ""
	switch a.Check {
	case parser.StateEnabled:
		checkStr = "enabled"
	case parser.StateDisabled:
		checkStr = "disabled"
	case parser.StateChecked:
		checkStr = "checked"
	case parser.StateContains:
		checkStr = "contains"
	case parser.StateFocused:
		checkStr = "focused"
	}
	// Resolve variables in selector text (for data-driven tests)
	sel := a.Sel
	sel.Text = e.resolve(sel.Text)
	params := probelink.SeeParams{
		Selector: toSelectorParam(sel),
		Negated:  a.Negated,
		Count:    a.Count,
		Check:    checkStr,
		CheckVal: e.resolve(a.CheckVal),
		Pattern:  a.Pattern,
	}
	return e.client.See(ctx, params)
}

// ---- Wait execution ----

func (e *Executor) runWait(ctx context.Context, w parser.WaitStep) error {
	kindStr := map[parser.WaitKind]string{
		parser.WaitDuration:    "duration",
		parser.WaitAppears:     "appears",
		parser.WaitDisappears:  "disappears",
		parser.WaitPageLoad:    "page_load",
		parser.WaitNetworkIdle: "network_idle",
		parser.WaitSelector:    "selector",
		parser.WaitAnimations:  "animations",
	}[w.Kind]

	return e.client.Wait(ctx, probelink.WaitParams{
		Kind:     kindStr,
		Target:   e.resolve(w.Target),
		Duration: w.Duration,
		Timeout:  e.timeout.Seconds(),
	})
}

// ---- Conditional execution ----

func (e *Executor) runConditional(ctx context.Context, c parser.ConditionalStep) error {
	// Check visibility with a short timeout
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	sel := probelink.SelectorParam{Kind: "text", Text: c.Condition}
	if strings.HasPrefix(c.Condition, "#") {
		sel = probelink.SelectorParam{Kind: "id", Text: c.Condition}
	}
	err := e.client.See(checkCtx, probelink.SeeParams{
		Selector: sel,
	})
	if err == nil {
		// condition is visible — run then branch
		return e.RunBody(ctx, c.Then)
	}
	// condition not visible — run else branch
	if len(c.Else) > 0 {
		return e.RunBody(ctx, c.Else)
	}
	return nil
}

// ---- Loop execution ----

func (e *Executor) runLoop(ctx context.Context, l parser.LoopStep) error {
	for i := 0; i < l.Count; i++ {
		if err := e.RunBody(ctx, l.Body); err != nil {
			return fmt.Errorf("loop iteration %d: %w", i+1, err)
		}
	}
	return nil
}

// ---- Dart block execution ----

func (e *Executor) runDart(ctx context.Context, d parser.DartBlock) error {
	return e.client.RunDart(ctx, d.Code)
}

// ---- Mock block execution ----

func (e *Executor) runMock(ctx context.Context, m parser.MockBlock) error {
	return e.client.RegisterMock(ctx, probelink.MockParam{
		Method: m.Method,
		Path:   m.Path,
		Status: m.Status,
		Body:   m.Body,
	})
}

// ---- Recipe call execution ----

func (e *Executor) runRecipeCall(ctx context.Context, rc parser.RecipeCall) error {
	recipe, ok := e.recipes[rc.Name]
	stripped := rc.Name
	if !ok {
		// Try matching by stripping <arg> placeholders and filler words from the call name.
		// e.g., call "enter credentials <arg> and <arg>" should match recipe "enter credentials"
		stripped = stripRecipeCallArgs(rc.Name)
		recipe, ok = e.recipes[stripped]
	}
	if !ok {
		// PT-02(a): an unrecognized recipe call used to silently no-op ("may
		// be a filler line"), which masked genuine typos and broken recipe
		// references for as long as no test ever surfaced the resulting
		// missing behavior. Every other "no matching case" fallthrough in
		// this executor already errors loudly (see runAction's default
		// case) — this was the sole silent exception.
		if stripped != rc.Name {
			return fmt.Errorf("line %d: unknown recipe call %q (also tried %q with placeholders/fillers stripped) — no recipe with that name is defined; check recipes_folder and 'use' statements for a typo or a missing recipe file", rc.Line, rc.Name, stripped)
		}
		return fmt.Errorf("line %d: unknown recipe call %q — no recipe with that name is defined; check recipes_folder and 'use' statements for a typo or a missing recipe file", rc.Line, rc.Name)
	}
	// Bind arguments to parameter names
	for i, param := range recipe.Params {
		if i < len(rc.Args) {
			e.vars[param] = rc.Args[i]
		}
	}
	e.depth++
	err := e.RunBody(ctx, recipe.Body)
	e.depth--
	return err
}

// ---- Auto-Reconnect ----

// isConnectionError returns true if the error indicates a broken WebSocket connection.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "probelink: write:") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "rpc error -32000")
}

// tryReconnect attempts to re-establish the connection to the agent. The app
// is still running — we just lost the TCP connection (e.g., physical device
// via iproxy, USB-C cable mode-flip). It does NOT restart the app.
//
// The attempt parameter (1-indexed) drives the backoff:
//
//	delay = min(reconnectBackoff << (attempt-1), 8s) + jitter (±20%)
//
// Concurrent callers are serialized via reconnectMu. If a newer client was
// already established while this caller waited for the lock, it returns
// immediately without re-dialing — the caller's e.client reference is
// already up-to-date because the previous winner called e.onReconnect.
func (e *Executor) tryReconnect(ctx context.Context, attempt int) error {
	if e.deviceCtx == nil {
		return fmt.Errorf("reconnect: no device context (cloud mode)")
	}

	// Capture the generation before locking. If somebody else reconnects while
	// we wait for the lock, we'll see a higher gen on the other side and skip.
	gen := e.clientGen.Load()
	e.reconnectMu.Lock()
	defer e.reconnectMu.Unlock()
	if e.clientGen.Load() > gen {
		return nil
	}

	base := e.reconnectBackoff
	if base == 0 {
		base = 1 * time.Second
	}
	delay := reconnectDelay(base, attempt)

	fmt.Printf("    \033[33m⟳\033[0m  Connection lost — attempt %d (waiting %s)...\n", attempt, delay.Round(100*time.Millisecond))
	e.client.Close()

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return ctx.Err()
	}

	newClient, err := e.deviceCtx.Reconnect(ctx)
	if err != nil && isConnectionRefused(err) {
		// PT-18: a plain re-dial only helps if the app process is still
		// running and the connection merely dropped (e.g. a transient
		// network blip) — Reconnect never relaunches anything. If the
		// process itself is gone (most commonly because a prior
		// `kill the app` step was followed by something other than
		// `open the app`/`restart the app`), nothing is listening at all
		// and re-dialing can never succeed no matter how many attempts are
		// left. A connection-refused dial failure specifically indicates
		// that — a transient drop would show up as a timeout or reset on a
		// socket that's still accepting connections, not "refused" on a
		// fresh dial. Relaunch once and retry immediately instead of
		// wasting the remaining attempts on a doomed re-dial.
		fmt.Printf("    \033[33m⟳\033[0m  Nothing listening — relaunching before retrying...\n")
		if restartErr := e.deviceCtx.RestartApp(ctx); restartErr == nil {
			newClient, err = e.deviceCtx.Reconnect(ctx)
		}
	}
	if err != nil {
		return fmt.Errorf("auto-reconnect failed: %w", err)
	}
	e.client = newClient
	e.clientGen.Add(1)
	if e.onReconnect != nil {
		e.onReconnect(newClient)
	}
	fmt.Printf("    \033[32m⟳\033[0m  Reconnected successfully (attempt %d)\n", attempt)
	return nil
}

// isConnectionRefused reports whether err indicates a dial found nothing
// listening at all (ECONNREFUSED), as opposed to a timeout or reset on a
// connection to a process that's still alive.
func isConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}

// reconnectDelay returns base << (attempt-1), capped at 8s, plus ±20% jitter.
// Exposed (unexported) so tests can verify the math without invoking reconnect.
func reconnectDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	const cap = 8 * time.Second
	d := base << (attempt - 1)
	if d <= 0 || d > cap {
		d = cap
	}
	fifth := int64(d) / 5
	if fifth <= 0 {
		return d
	}
	// Jitter in [-fifth, +fifth) — i.e. [-20%, +20%) of d.
	jitter := time.Duration(mathrand.Int63n(2*fifth) - fifth)
	return d + jitter
}

// ---- Helpers ----

// resolve substitutes <variable> placeholders with values from the vars map
// and expands <random.*> generators.
func (e *Executor) resolve(s string) string {
	// First: expand random data generators
	s = resolveRandomVars(s)
	// Then: substitute data-driven and recipe variables
	for k, v := range e.vars {
		old := "<" + k + ">"
		// PT-02(c): a variable can end up bound to a value that itself
		// contains its own placeholder marker — e.g. passing the unquoted
		// literal "<email>" as the argument for a recipe param named email
		// (runRecipeCall binds it verbatim). Substituting "<email>" for
		// "<email>" makes no progress and previously looped forever, hanging
		// the CLI with no error. maxSubstitutions bounds the loop so this
		// terminates; the `next == s` check exits immediately for the exact
		// self-reference case (v == old) without waiting out the full cap.
		const maxSubstitutions = 1000
		for i := 0; i < maxSubstitutions && len(s) > 0 && containsSubstr(s, old); i++ {
			next := replaceFirst(s, old, v)
			if next == s {
				break
			}
			s = next
		}
	}
	return s
}

// recipeFillerWords are common filler words stripped from recipe call names
// to enable fuzzy matching against recipe definitions.
var recipeFillerWords = map[string]bool{
	"and": true, "with": true, "the": true, "then": true,
}

// stripRecipeCallArgs removes <arg> placeholders and common filler words
// from a recipe call name so it can match the recipe definition name.
// e.g., "sign in with <arg> and <arg>" → "sign in with"
//       "enter credentials <arg> and <arg>" → "enter credentials"
func stripRecipeCallArgs(name string) string {
	words := strings.Fields(name)
	var result []string
	for _, w := range words {
		if w == "<arg>" || recipeFillerWords[w] {
			continue
		}
		result = append(result, w)
	}
	return strings.TrimSpace(strings.Join(result, " "))
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstrInner(s, sub))
}

func containsSubstrInner(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func replaceFirst(s, old, new string) string {
	for i := 0; i <= len(s)-len(old); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

// ---- HTTP call execution ----

func (e *Executor) runHTTPCall(ctx context.Context, h parser.HTTPCallStep) error {
	url := e.resolve(h.URL)
	body := e.resolve(h.Body)

	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequestWithContext(ctx, h.Method, url, strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("http call: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, h.Method, url, nil)
		if err != nil {
			return fmt.Errorf("http call: %w", err)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http call %s %s: %w", h.Method, url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Store response in variables accessible as <response.status> and <response.body>
	e.vars["response.status"] = strconv.Itoa(resp.StatusCode)
	e.vars["response.body"] = string(respBody)

	return nil
}

// toSelectorParam converts an AST Selector to a probelink SelectorParam.
func toSelectorParam(s parser.Selector) probelink.SelectorParam {
	kinds := map[parser.SelectorKind]string{
		parser.SelectorText:        "text",
		parser.SelectorID:          "id",
		parser.SelectorType:        "type",
		parser.SelectorOrdinal:     "ordinal",
		parser.SelectorPositional:  "positional",
		parser.SelectorRelational:  "relational",
	}
	return probelink.SelectorParam{
		Kind:      kinds[s.Kind],
		Text:      s.Text,
		Ordinal:   s.Ordinal,
		Container: s.Container,
		Relation:  s.Relation,
		Anchor:    s.Anchor,
	}
}
