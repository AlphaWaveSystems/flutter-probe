package runner

import (
	"github.com/alphawavesystems/flutter-probe/internal/perf"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

// TestResult captures the outcome of a single test run.
type TestResult struct {
	TestName   string
	File       string
	Passed     bool
	Skipped    bool
	Duration   time.Duration
	Error      error
	Row        int // data-driven row index, -1 = not data-driven
	Artifacts  []string
	VideoURL   string // cloud provider video URL (session-level)
	DeviceID   string // device serial/UDID that ran this test
	DeviceName string // human-readable device name
	Perf       []perf.Metrics // measurements the test took (`start measuring` ... `stop measuring`)
	Attempts   int    // how many times the test ran (1 unless defaults.retry_failed_tests re-ran it)
}

// Runner coordinates parsing, connecting, and executing .probe files.
type Runner struct {
	cfg             *config.Config
	client          probelink.ProbeClient
	deviceCtx       *DeviceContext  // nil in dry-run mode
	opts            RunOptions
	recipes         map[string]parser.RecipeDef
	baseRecipes     map[string]parser.RecipeDef // recipes_folder only; dry-run starts every file from it
	missingUses     []string                     // `use` targets that do not exist, for error messages
	visual          *visual.Comparator  // nil if visual regression is not configured
	onResult        func(TestResult)    // optional per-result callback for streaming
	compositeRunner *CompositeRunner    // nil if composite tests are not configured
}

// RunOptions configures a test run.
type RunOptions struct {
	Files        []string // .probe files to run
	Tags         []string // filter by tag
	Watch        bool     // re-run on file change
	Timeout      time.Duration
	DryRun       bool   // parse only
	Verbose      bool
	VideoEnabled bool   // record device screen during tests
	VideoDir     string // directory to store video recordings
	DeviceID     string // device serial/UDID (for tagging results)
	DeviceName   string // human-readable device name
	Grant        []string // permissions to pre-grant before the first test (--grant)

	PerfBaseline  *perf.Baseline // compare measurements with this baseline (nil = off)
	PerfTolerance float64        // percent a metric may exceed its baseline by (default 20)
}

// New creates a Runner.
func New(cfg *config.Config, client probelink.ProbeClient, deviceCtx *DeviceContext, opts RunOptions) *Runner {
	if opts.Timeout == 0 {
		opts.Timeout = cfg.Defaults.Timeout
	}
	return &Runner{
		cfg:       cfg,
		client:    client,
		deviceCtx: deviceCtx,
		opts:      opts,
		recipes:   make(map[string]parser.RecipeDef),
	}
}

// SetVisual configures visual regression comparison for this runner.
func (r *Runner) SetVisual(c *visual.Comparator) {
	r.visual = c
}

// OnResult registers a callback fired after each test completes. Used for
// live streaming output (e.g. ndjson via Reporter.StreamResult). Must be
// safe to call from the goroutine that runs tests.
func (r *Runner) OnResult(cb func(TestResult)) {
	r.onResult = cb
}

// SetCompositeRunner attaches a CompositeRunner so that composite tests found
// in .probe files are executed instead of skipped.
func (r *Runner) SetCompositeRunner(cr *CompositeRunner) {
	r.compositeRunner = cr
}

// newExecutor builds an Executor preconfigured with this Runner's reconnect
// policy from probe.yaml (agent.reconnect_attempts and agent.reconnect_backoff).
func (r *Runner) newExecutor() *Executor {
	exec := NewExecutor(r.client, r.deviceCtx, func(newClient probelink.ProbeClient) {
		r.client = newClient
	}, r.opts.Timeout, r.opts.Verbose)
	exec.SetReconnectPolicy(r.cfg.Agent.ReconnectAttempts, r.cfg.Agent.ReconnectBackoff)
	exec.SetLaunchTimeout(r.cfg.Agent.LaunchTimeout)
	exec.SetImplicitWait(r.cfg.Defaults.ImplicitWait)
	exec.SetAI(r.cfg.AI)
	exec.useNote = r.missingUsesNote()
	return exec
}

// Run executes all specified test files and returns results.
func (r *Runner) Run(ctx context.Context) ([]TestResult, error) {
	// Load recipes
	if err := r.loadRecipes(ctx); err != nil {
		return nil, fmt.Errorf("runner: loading recipes: %w", err)
	}
	r.baseRecipes = make(map[string]parser.RecipeDef, len(r.recipes))
	for k, v := range r.recipes {
		r.baseRecipes[k] = v
	}

	stopGrants, err := r.applyGrants(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		stopGrants()
		if r.deviceCtx != nil {
			r.deviceCtx.CloseSystemDriver() // no-op unless a system-dialog step started it
		}
	}()

	var results []TestResult
	for _, file := range r.opts.Files {
		fileResults, err := r.runFile(ctx, file)
		if err != nil {
			return results, fmt.Errorf("runner: %s: %w", file, err)
		}
		results = append(results, fileResults...)
	}
	return results, nil
}

func (r *Runner) runFile(ctx context.Context, path string) ([]TestResult, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	prog, err := parser.ParseFile(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// --dry-run resolves each file on its own, like running that file alone: the
	// recipes of other files in the same run must not make an unresolved call look
	// fine (a runtime run of this one file could not see them).
	if r.opts.DryRun && r.baseRecipes != nil {
		r.recipes = make(map[string]parser.RecipeDef, len(r.baseRecipes))
		for k, v := range r.baseRecipes {
			r.recipes[k] = v
		}
	}

	// Import additional recipes declared with `use` (nested `use` lines are
	// followed, each relative to the file that contains them).
	r.missingUses = nil
	seen := map[string]bool{}
	if abs, err := filepath.Abs(path); err == nil {
		seen[abs] = true
	}
	for _, u := range prog.Uses {
		if err := r.loadUse(ctx, path, u, seen); err != nil {
			return nil, err
		}
	}

	// Register recipes from this file
	for _, rec := range prog.Recipes {
		r.recipes[rec.Name] = rec
	}

	// Filter tests by tag
	tests := r.filterTests(prog.Tests)

	if r.opts.DryRun {
		// Parse and resolve only: a step that is not a built-in is a recipe
		// call, so an unknown one (a typo, a verb that does not exist) is
		// reported here instead of at runtime.
		var results []TestResult
		for _, t := range tests {
			res := TestResult{TestName: t.Name, File: path, Passed: true, Row: -1}
			if err := r.unresolvedCalls(prog, t); err != nil {
				res.Passed = false
				res.Error = err
			}
			results = append(results, res)
		}
		return results, nil
	}

	var results []TestResult

	// Run beforeAll hooks (fail-fast: if beforeAll fails, skip all tests in file)
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookBeforeAll {
			exec := r.newExecutor()
			if err := exec.RunBody(ctx, hook.Body); err != nil {
				// Mark all tests as failed due to beforeAll failure
				for _, t := range tests {
					results = append(results, TestResult{
						TestName: t.Name,
						File:     path,
						Passed:   false,
						Error:    fmt.Errorf("before all: %w", err),
						Row:      -1,
					})
				}
				return results, nil
			}
		}
	}

	// Run tests
	for _, t := range tests {
		testResults, err := r.runTest(ctx, prog, t, path)
		if err != nil {
			return results, err
		}
		results = append(results, testResults...)
	}

	// Run afterAll hooks (always, best-effort)
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookAfterAll {
			exec := r.newExecutor()
			_ = exec.RunBody(ctx, hook.Body)
		}
	}

	// Run composite tests. If no CompositeRunner is configured, skip them
	// and report each as SKIPPED with a descriptive reason.
	for _, ct := range prog.CompositeTests {
		var res TestResult
		if r.compositeRunner == nil {
			res = TestResult{
				TestName: ct.Name,
				File:     path,
				Skipped:  true,
				Row:      -1,
			}
			fmt.Printf("  \033[33m⟳\033[0m  %s \033[2m(skipped: no composite devices configured — use --composite-device)\033[0m\n", ct.Name)
		} else {
			// Share recipes with the composite runner so recipe files loaded
			// by this .probe file are available to all device goroutines.
			for _, rec := range r.recipes {
				r.compositeRunner.RegisterRecipe(rec)
			}
			ctr := r.compositeRunner.RunCompositeTest(ctx, ct, path)
			res = ctr.ToTestResult()
			printCompositeResult(ctr)
		}
		if r.onResult != nil {
			r.onResult(res)
		}
		results = append(results, res)
	}

	return results, nil
}

// printCompositeResult prints the composite test outcome with per-device detail.
func printCompositeResult(r CompositeTestResult) {
	if r.Skipped {
		fmt.Printf("  \033[33m⟳\033[0m  %s \033[2m(skipped)\033[0m\n", r.TestName)
		return
	}
	icon := "\033[32m✓\033[0m"
	if !r.Passed {
		icon = "\033[31m✗\033[0m"
	}
	fmt.Printf("  %s  %s \033[2m(%s) [composite]\033[0m\n", icon, r.TestName, r.Duration.Round(time.Millisecond))
	for alias, dr := range r.DeviceResults {
		if dr.Error != nil {
			fmt.Printf("       \033[31m[%s] %v\033[0m\n", alias, dr.Error)
		}
	}
}

func (r *Runner) runTest(ctx context.Context, prog *parser.Program, t parser.TestDef, file string) ([]TestResult, error) {
	// Load CSV examples if Source is set
	if t.Examples != nil && t.Examples.Source != "" && len(t.Examples.Rows) == 0 {
		csvExamples, err := loadCSVExamples(filepath.Dir(file), t.Examples.Source)
		if err != nil {
			return nil, err
		}
		t.Examples.Headers = csvExamples.Headers
		t.Examples.Rows = csvExamples.Rows
	}

	// Data-driven: expand rows
	if t.Examples != nil && len(t.Examples.Rows) > 0 {
		return r.runDataDriven(ctx, prog, t, file)
	}
	res := r.runSingleTest(ctx, prog, t, file, nil, -1)
	if r.onResult != nil {
		r.onResult(res)
	}
	return []TestResult{res}, nil
}

func (r *Runner) runDataDriven(ctx context.Context, prog *parser.Program, t parser.TestDef, file string) ([]TestResult, error) {
	var results []TestResult
	for rowIdx, row := range t.Examples.Rows {
		vars := make(map[string]string)
		for i, header := range t.Examples.Headers {
			if i < len(row) {
				vars[header] = row[i]
			}
		}
		res := r.runSingleTest(ctx, prog, t, file, vars, rowIdx)
		if r.onResult != nil {
			r.onResult(res)
		}
		results = append(results, res)
	}
	return results, nil
}

// runSingleTest runs a test and, when defaults.retry_failed_tests (or --retry-failed) is set,
// runs it again after a failure, up to that many extra times. A test that passes on a retry
// counts as passed and records how many attempts it took, so flakiness stays visible.
func (r *Runner) runSingleTest(ctx context.Context, prog *parser.Program, t parser.TestDef, file string, vars map[string]string, row int) TestResult {
	retries := r.cfg.Defaults.RetryFailedTests
	if retries < 0 {
		retries = 0
	}
	var res TestResult
	var earlier []string
	for attempt := 1; ; attempt++ {
		res = r.runSingleAttempt(ctx, prog, t, file, vars, row)
		r.applyPerfBaseline(&res)
		res.Attempts = attempt
		res.Artifacts = append(earlier, res.Artifacts...)
		if res.Passed || attempt > retries || ctx.Err() != nil || r.client == nil {
			return res
		}
		earlier = res.Artifacts
		fmt.Printf("    \033[33m↻\033[0m  %s failed (attempt %d of %d): %v — retrying\n", res.TestName, attempt, retries+1, res.Error)
	}
}

func (r *Runner) runSingleAttempt(ctx context.Context, prog *parser.Program, t parser.TestDef, file string, vars map[string]string, row int) TestResult {
	start := time.Now()
	exec := r.newExecutor()
	for name, rec := range r.recipes {
		exec.RegisterRecipe(rec)
		_ = name
	}
	for k, v := range vars {
		exec.SetVar(k, v)
	}
	if r.visual != nil {
		exec.SetVisual(r.visual)
	}

	// Start video recording if enabled
	var recorder *VideoRecorder
	if r.opts.VideoEnabled && r.deviceCtx != nil {
		videoDir := r.opts.VideoDir
		if videoDir == "" {
			videoDir = "reports/videos"
		}
		recorder = NewVideoRecorder(r.deviceCtx.Manager, r.deviceCtx.Serial, r.deviceCtx.Platform, videoDir, r.cfg.Video)
		if err := recorder.Start(ctx, t.Name); err != nil {
			fmt.Printf("    \033[33m⚠\033[0m  video recording failed to start: %v\n", err)
			recorder = nil
		}
	}

	var runErr error

	exec.beginHTTP(ctx) // each test starts with an empty request log and no mocks

	// Run before-each hooks
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookBeforeEach {
			if err := exec.RunBody(ctx, hook.Body); err != nil {
				runErr = fmt.Errorf("before each: %w", err)
				break
			}
		}
	}

	// Run test body
	if runErr == nil {
		runErr = exec.RunBody(ctx, t.Body)
	}

	// Auto-screenshot on failure
	if runErr != nil && r.client != nil {
		shotCtx, shotCancel := context.WithTimeout(ctx, 10*time.Second)
		shotName := fmt.Sprintf("failure_%s", sanitizeName(t.Name))
		path, shotErr := r.client.Screenshot(shotCtx, shotName)
		shotCancel()
		if shotErr != nil {
			fmt.Printf("    \033[33m⚠\033[0m  failure screenshot: %v\n", shotErr)
		} else if path != "" {
			exec.AddArtifact(path)
			fmt.Printf("    \033[36m📸\033[0m  failure screenshot saved: %s\n", path)
		}
	}

	// Run on-failure hooks
	if runErr != nil {
		for _, hook := range prog.Hooks {
			if hook.Kind == parser.HookOnFailure {
				_ = exec.RunBody(ctx, hook.Body) // best-effort
			}
		}
	}

	// Run after-each hooks (always)
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookAfterEach {
			_ = exec.RunBody(ctx, hook.Body) // best-effort
		}
	}

	// Stop video recording and add as artifact
	if recorder != nil {
		videoPath, err := recorder.Stop(ctx)
		if err != nil {
			fmt.Printf("    \033[33m⚠\033[0m  video recording stop: %v\n", err)
		} else if videoPath != "" {
			absPath, _ := filepath.Abs(videoPath)
			if absPath != "" {
				exec.AddArtifact(absPath)
			} else {
				exec.AddArtifact(videoPath)
			}
			fmt.Printf("    \033[36m🎬\033[0m  video saved: %s\n", videoPath)
		}
	}

	name := t.Name
	if row >= 0 && t.Examples != nil {
		name = fmt.Sprintf("%s [row %d]", t.Name, row+1)
	}

	exec.closePerf(ctx)

	return TestResult{
		Perf:       exec.PerfResults(),
		TestName:   name,
		File:       file,
		Passed:     runErr == nil,
		Duration:   time.Since(start),
		Error:      runErr,
		Row:        row,
		Artifacts:  exec.Artifacts(),
		DeviceID:   r.opts.DeviceID,
		DeviceName: r.opts.DeviceName,
	}
}

// loadRecipes reads all .probe files from the recipes folder.
func (r *Runner) loadRecipes(_ context.Context) error {
	if r.cfg.Recipes == "" {
		return nil
	}
	entries, err := os.ReadDir(r.cfg.Recipes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".probe") {
			continue
		}
		if err := r.loadRecipeFile(context.TODO(), filepath.Join(r.cfg.Recipes, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) loadRecipeFile(_ context.Context, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prog, err := parser.ParseFile(string(src))
	if err != nil {
		return fmt.Errorf("parse recipe file %s: %w", path, err)
	}
	for _, rec := range prog.Recipes {
		r.recipes[rec.Name] = rec
	}
	return nil
}

// loadUse loads the file named by a `use` statement found in `from`, then the
// `use` statements of that file in turn (relative to it). A target that does not
// exist is remembered (see missingUsesNote) instead of silently ignored.
func (r *Runner) loadUse(ctx context.Context, from string, u parser.UseStmt, seen map[string]bool) error {
	target := filepath.Join(filepath.Dir(from), u.Path)
	abs, err := filepath.Abs(target)
	if err != nil {
		abs = target
	}
	if seen[abs] {
		return nil
	}
	seen[abs] = true
	src, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			r.missingUses = append(r.missingUses, fmt.Sprintf("%s:%d `use %q` -> %s", from, u.Line, u.Path, target))
			return nil
		}
		return err
	}
	prog, err := parser.ParseFile(string(src))
	if err != nil {
		return fmt.Errorf("parse recipe file %s: %w", target, err)
	}
	for _, rec := range prog.Recipes {
		r.recipes[rec.Name] = rec
	}
	for _, nested := range prog.Uses {
		if err := r.loadUse(ctx, target, nested, seen); err != nil {
			return err
		}
	}
	return nil
}

// missingUsesNote explains an unresolved call when some `use` target was missing.
func (r *Runner) missingUsesNote() string {
	if len(r.missingUses) == 0 {
		return ""
	}
	return " — note: these `use` targets do not exist: " + strings.Join(r.missingUses, "; ")
}

// filterTests removes tests that don't match the tag filter.
func (r *Runner) filterTests(tests []parser.TestDef) []parser.TestDef {
	if len(r.opts.Tags) == 0 {
		return tests
	}
	var filtered []parser.TestDef
	for _, t := range tests {
		for _, tag := range r.opts.Tags {
			for _, tt := range t.Tags {
				if tt == tag {
					filtered = append(filtered, t)
					break
				}
			}
		}
	}
	return filtered
}

// CollectFiles finds all .probe files under the given paths.
func CollectFiles(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err = filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fi.IsDir() && strings.HasSuffix(path, ".probe") {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			files = append(files, p)
		}
	}
	return files, nil
}

// PullArtifacts copies on-device screenshot paths to localDir and rewrites
// TestResult.Artifacts to local paths. For Android, uses `run-as` + cat to read
// from the app's private cache. For iOS simulators, files are already on the host.
func PullArtifacts(ctx context.Context, results []TestResult, dc *DeviceContext, localDir string) {
	if dc == nil || dc.Manager == nil {
		return
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return
	}
	for i := range results {
		var localPaths []string
		for _, remotePath := range results[i].Artifacts {
			// Skip video files that are already in the output directory
			if filepath.IsAbs(remotePath) {
				if _, err := os.Stat(remotePath); err == nil {
					ext := filepath.Ext(remotePath)
					if ext == ".mov" || ext == ".mp4" || ext == ".webm" {
						localPaths = append(localPaths, remotePath)
						continue
					}
				}
			}
			// Copy screenshots to the local reports directory
			localPath := filepath.Join(localDir, filepath.Base(remotePath))
			var pullErr error
			switch {
			case dc.Platform == device.PlatformIOS:
				// iOS simulator: file is already on host, just copy it
				pullErr = copyFileIfDifferent(remotePath, localPath)
			case filepath.IsAbs(remotePath) && fileExists(remotePath):
				// Android screenshots arrive as base64 in the RPC reply and are
				// already saved on the host by the probelink client; this path is
				// that host path, not a device path. Re-reading it through
				// `adb exec-out run-as ... cat` printed "No such file" on stdout
				// and that text overwrote the good PNG (FP-19).
				pullErr = copyFileIfDifferent(remotePath, localPath)
			default:
				// Android: screenshots are in the app's private cache dir,
				// use run-as to read them since adb pull can't access private dirs
				data, err := dc.Manager.ADB().Run(ctx, dc.Serial,
					"exec-out", "run-as", dc.AppID, "cat", remotePath)
				switch {
				case err != nil:
					pullErr = err
				case !looksLikeImage(data):
					// exec-out reports failures (missing file, "package not
					// debuggable") as text on stdout with exit 0: never write that
					// out as an image.
					pullErr = fmt.Errorf("pulling %s: not an image (%s)", remotePath, strings.TrimSpace(string(data[:min(len(data), 120)])))
				default:
					pullErr = os.WriteFile(localPath, data, 0644)
				}
			}
			if pullErr == nil {
				absPath, _ := filepath.Abs(localPath)
				if absPath != "" {
					localPaths = append(localPaths, absPath)
				} else {
					localPaths = append(localPaths, localPath)
				}
			}
		}
		results[i].Artifacts = localPaths
	}
}

// LocalizeArtifacts ensures artifact paths are valid for cloud mode where
// screenshots were saved locally by the probelink client (base64 in RPC response).
// It creates the screenshot directory and converts paths to absolute.
func LocalizeArtifacts(results []TestResult, localDir string) {
	_ = os.MkdirAll(localDir, 0755)
	for i := range results {
		var localPaths []string
		for _, p := range results[i].Artifacts {
			// If the file already exists on disk (saved by probelink client), use as-is
			if _, err := os.Stat(p); err == nil {
				absPath, _ := filepath.Abs(p)
				if absPath != "" {
					localPaths = append(localPaths, absPath)
				} else {
					localPaths = append(localPaths, p)
				}
			}
		}
		results[i].Artifacts = localPaths
	}
}

// fileExists reports whether path exists on the host.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyFileIfDifferent copies src to dst unless they are the same file (an
// artifact already saved into the destination directory).
func copyFileIfDifferent(src, dst string) error {
	a, errA := filepath.Abs(src)
	b, errB := filepath.Abs(dst)
	if errA == nil && errB == nil && a == b {
		return nil
	}
	return copyFile(src, dst)
}

// looksLikeImage reports whether data starts with a PNG or JPEG signature.
func looksLikeImage(data []byte) bool {
	return bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) || bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sanitizeName converts a test name into a safe filename component.
func sanitizeName(name string) string {
	s := nonAlphaNum.ReplaceAllString(name, "_")
	s = strings.Trim(s, "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// unresolvedCalls reports the first recipe call reachable from the test (its
// body, the hooks and every recipe body it can call) that matches no loaded
// recipe. Used by --dry-run.
func (r *Runner) unresolvedCalls(prog *parser.Program, t parser.TestDef) error {
	seen := map[string]bool{}
	var check func(steps []parser.Step) error
	check = func(steps []parser.Step) error {
		for _, s := range steps {
			switch st := s.(type) {
			case parser.RecipeCall:
				rec, _, stripped, ok := resolveCall(r.recipes, st)
				if !ok {
					if stripped != st.Name {
						return fmt.Errorf("line %d: unknown step %q (also tried %q) — not a built-in step and no recipe with that name is defined%s", st.Line, st.Name, stripped, r.missingUsesNote())
					}
					return fmt.Errorf("line %d: unknown step %q — not a built-in step and no recipe with that name is defined%s", st.Line, st.Name, r.missingUsesNote())
				}
				if !seen[rec.Name] {
					seen[rec.Name] = true
					if err := check(rec.Body); err != nil {
						return fmt.Errorf("in recipe %q: %w", rec.Name, err)
					}
				}
			case parser.ConditionalStep:
				if err := check(st.Then); err != nil {
					return err
				}
				if err := check(st.Else); err != nil {
					return err
				}
			case parser.LoopStep:
				if err := check(st.Body); err != nil {
					return err
				}
			case parser.RetryStep:
				if err := check(st.Body); err != nil {
					return err
				}
			case parser.DeviceStep:
				if err := check([]parser.Step{st.Step}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, h := range prog.Hooks {
		if err := check(h.Body); err != nil {
			return err
		}
	}
	return check(t.Body)
}

// applyPerfBaseline fails a passing test whose measurements are clearly worse than the baseline.
func (r *Runner) applyPerfBaseline(res *TestResult) {
	b := r.opts.PerfBaseline
	if b == nil || !res.Passed || len(res.Perf) == 0 {
		return
	}
	tol := r.opts.PerfTolerance
	if tol <= 0 {
		tol = 20
	}
	var problems []string
	for _, m := range res.Perf {
		base, ok := b.Entries[perf.Key(res.File, res.TestName, m.Name)]
		if !ok {
			continue // a new measurement: nothing to compare with yet
		}
		for _, reg := range perf.Compare(base, m, tol) {
			problems = append(problems, fmt.Sprintf("%s: %s", measurementLabel(m.Name), reg))
		}
	}
	if len(problems) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("performance regression (tolerance %.0f%%): %s", tol, strings.Join(problems, "; "))
	}
}
