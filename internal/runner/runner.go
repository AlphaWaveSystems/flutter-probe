package runner

import (
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
}

// Runner coordinates parsing, connecting, and executing .probe files.
type Runner struct {
	cfg       *config.Config
	client    probelink.ProbeClient
	deviceCtx *DeviceContext // nil in dry-run mode
	opts      RunOptions
	recipes   map[string]parser.RecipeDef
	visual    *visual.Comparator // nil if visual regression is not configured
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

// Run executes all specified test files and returns results.
func (r *Runner) Run(ctx context.Context) ([]TestResult, error) {
	// Load recipes
	if err := r.loadRecipes(ctx); err != nil {
		return nil, fmt.Errorf("runner: loading recipes: %w", err)
	}

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

	// Import additional recipes declared with `use`
	for _, u := range prog.Uses {
		if err := r.loadRecipeFile(ctx, filepath.Join(filepath.Dir(path), u.Path)); err != nil {
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
		// Parse-only — just return names as passed
		var results []TestResult
		for _, t := range tests {
			results = append(results, TestResult{TestName: t.Name, File: path, Passed: true, Row: -1})
		}
		return results, nil
	}

	var results []TestResult

	// Run beforeAll hooks (fail-fast: if beforeAll fails, skip all tests in file)
	for _, hook := range prog.Hooks {
		if hook.Kind == parser.HookBeforeAll {
			exec := NewExecutor(r.client, r.deviceCtx, func(newClient probelink.ProbeClient) {
				r.client = newClient
			}, r.opts.Timeout, r.opts.Verbose)
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
			exec := NewExecutor(r.client, r.deviceCtx, func(newClient probelink.ProbeClient) {
				r.client = newClient
			}, r.opts.Timeout, r.opts.Verbose)
			_ = exec.RunBody(ctx, hook.Body)
		}
	}

	return results, nil
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
		results = append(results, res)
	}
	return results, nil
}

func (r *Runner) runSingleTest(ctx context.Context, prog *parser.Program, t parser.TestDef, file string, vars map[string]string, row int) TestResult {
	start := time.Now()
	exec := NewExecutor(r.client, r.deviceCtx, func(newClient probelink.ProbeClient) {
		r.client = newClient
	}, r.opts.Timeout, r.opts.Verbose)
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

	return TestResult{
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
			if dc.Platform == device.PlatformIOS {
				// iOS simulator: file is already on host, just copy it
				pullErr = copyFile(remotePath, localPath)
			} else {
				// Android: screenshots are in the app's private cache dir,
				// use run-as to read them since adb pull can't access private dirs
				data, err := dc.Manager.ADB().Run(ctx, dc.Serial,
					"exec-out", "run-as", dc.AppID, "cat", remotePath)
				if err == nil && len(data) > 0 {
					pullErr = os.WriteFile(localPath, data, 0644)
				} else {
					pullErr = err
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
