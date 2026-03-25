package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

// Executor walks an AST body and dispatches commands to a ProbeLink client.
type Executor struct {
	client      *probelink.Client
	deviceCtx   *DeviceContext                // nil in dry-run mode
	onReconnect func(*probelink.Client)       // callback to update Runner's client ref
	timeout     time.Duration
	recipes     map[string]parser.RecipeDef   // loaded recipes by name
	vars        map[string]string             // variable scope for data-driven tests
	verbose     bool
	depth       int      // indentation depth for verbose logging
	artifacts   []string // collected screenshot paths (on-device)
	visual      *visual.Comparator // nil if visual regression is not configured
}

// NewExecutor creates an Executor.
func NewExecutor(client *probelink.Client, deviceCtx *DeviceContext, onReconnect func(*probelink.Client), timeout time.Duration, verbose bool) *Executor {
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

func (e *Executor) runStep(ctx context.Context, step parser.Step) error {
	// Use a longer timeout for restart/clear — they kill the app and reconnect
	stepTimeout := e.timeout
	if a, ok := step.(parser.ActionStep); ok {
		if a.Verb == parser.VerbRestart || a.Verb == parser.VerbClearAppData {
			stepTimeout = 90 * time.Second
		}
	}
	stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()

	start := time.Now()
	desc := e.stepDescription(step)
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

	if e.verbose && desc != "" {
		elapsed := time.Since(start)
		indent := strings.Repeat("  ", e.depth)
		status := "\033[32m✓\033[0m"
		if err != nil {
			status = "\033[31m✗\033[0m"
		}
		fmt.Printf("    %s%s %s \033[2m(%.1fs)\033[0m\n", indent, status, desc, elapsed.Seconds())
	}

	return err
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
	switch a.Verb {
	case parser.VerbOpen:
		screen := ""
		if a.Sel != nil {
			screen = a.Sel.Text
		}
		return e.client.Open(ctx, screen)

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
		_, err := e.client.DumpWidgetTree(ctx)
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
		e.client.Close()
		if err := e.deviceCtx.RestartApp(ctx); err != nil {
			return err
		}
		newClient, err := e.deviceCtx.Reconnect(ctx)
		if err != nil {
			return fmt.Errorf("restart the app: %w", err)
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
	}
	params := probelink.SeeParams{
		Selector: toSelectorParam(a.Sel),
		Negated:  a.Negated,
		Count:    a.Count,
		Check:    checkStr,
		CheckVal: a.CheckVal,
		Pattern:  a.Pattern,
	}
	return e.client.See(ctx, params)
}

// ---- Wait execution ----

func (e *Executor) runWait(ctx context.Context, w parser.WaitStep) error {
	kindStr := map[parser.WaitKind]string{
		parser.WaitDuration:   "duration",
		parser.WaitAppears:    "appears",
		parser.WaitDisappears: "disappears",
		parser.WaitPageLoad:   "page_load",
		parser.WaitNetworkIdle: "network_idle",
		parser.WaitSelector:   "selector",
	}[w.Kind]

	return e.client.Wait(ctx, probelink.WaitParams{
		Kind:     kindStr,
		Target:   w.Target,
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
	if !ok {
		// Try matching by stripping <arg> placeholders and filler words from the call name.
		// e.g., call "enter credentials <arg> and <arg>" should match recipe "enter credentials"
		stripped := stripRecipeCallArgs(rc.Name)
		recipe, ok = e.recipes[stripped]
	}
	if !ok {
		// Unknown recipe — skip rather than error (may be a filler line)
		return nil
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

// ---- Helpers ----

// resolve substitutes <variable> placeholders with values from the vars map
// and expands <random.*> generators.
func (e *Executor) resolve(s string) string {
	// First: expand random data generators
	s = resolveRandomVars(s)
	// Then: substitute data-driven and recipe variables
	for k, v := range e.vars {
		old := "<" + k + ">"
		for len(s) > 0 && containsSubstr(s, old) {
			s = replaceFirst(s, old, v)
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
	}
	return probelink.SelectorParam{
		Kind:      kinds[s.Kind],
		Text:      s.Text,
		Ordinal:   s.Ordinal,
		Container: s.Container,
	}
}
