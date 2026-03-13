package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/flutterprobe/probe/internal/parser"
	"github.com/flutterprobe/probe/internal/probelink"
)

// Executor walks an AST body and dispatches commands to a ProbeLink client.
type Executor struct {
	client  *probelink.Client
	timeout time.Duration
	recipes map[string]parser.RecipeDef // loaded recipes by name
	vars    map[string]string            // variable scope for data-driven tests
}

// NewExecutor creates an Executor.
func NewExecutor(client *probelink.Client, timeout time.Duration) *Executor {
	return &Executor{
		client:  client,
		timeout: timeout,
		recipes: make(map[string]parser.RecipeDef),
		vars:    make(map[string]string),
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
	stepCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	switch s := step.(type) {
	case parser.ActionStep:
		return e.runAction(stepCtx, s)
	case parser.AssertStep:
		return e.runAssert(stepCtx, s)
	case parser.WaitStep:
		return e.runWait(stepCtx, s)
	case parser.ConditionalStep:
		return e.runConditional(ctx, s) // pass parent ctx — conditional manages its own timeout
	case parser.LoopStep:
		return e.runLoop(ctx, s)
	case parser.DartBlock:
		return e.runDart(stepCtx, s)
	case parser.MockBlock:
		return e.runMock(stepCtx, s)
	case parser.RecipeCall:
		return e.runRecipeCall(ctx, s)
	default:
		return nil
	}
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
		_, err := e.client.Screenshot(ctx, a.Name)
		return err

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
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err := e.client.See(checkCtx, probelink.SeeParams{
		Selector: probelink.SelectorParam{Kind: "text", Text: c.Condition},
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
		// Unknown recipe — skip rather than error (may be a filler line)
		return nil
	}
	// Bind arguments to parameter names
	for i, param := range recipe.Params {
		if i < len(rc.Args) {
			e.vars[param] = rc.Args[i]
		}
	}
	return e.RunBody(ctx, recipe.Body)
}

// ---- Helpers ----

// resolve substitutes <variable> placeholders with values from the vars map.
func (e *Executor) resolve(s string) string {
	for k, v := range e.vars {
		old := "<" + k + ">"
		for len(s) > 0 && containsSubstr(s, old) {
			s = replaceFirst(s, old, v)
		}
	}
	return s
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
