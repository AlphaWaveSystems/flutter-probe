package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flutterprobe/probe/internal/config"
	"github.com/flutterprobe/probe/internal/parser"
	"github.com/flutterprobe/probe/internal/probelink"
)

// TestResult captures the outcome of a single test run.
type TestResult struct {
	TestName  string
	File      string
	Passed    bool
	Skipped   bool
	Duration  time.Duration
	Error     error
	Row       int // data-driven row index, -1 = not data-driven
	Artifacts []string
}

// Runner coordinates parsing, connecting, and executing .probe files.
type Runner struct {
	cfg     *config.Config
	client  *probelink.Client
	opts    RunOptions
	recipes map[string]parser.RecipeDef
}

// RunOptions configures a test run.
type RunOptions struct {
	Files   []string // .probe files to run
	Tags    []string // filter by tag
	Watch   bool     // re-run on file change
	Shard   int      // number of shards (0 = no sharding)
	Timeout time.Duration
	DryRun  bool // parse only
	Verbose bool
}

// New creates a Runner.
func New(cfg *config.Config, client *probelink.Client, opts RunOptions) *Runner {
	if opts.Timeout == 0 {
		opts.Timeout = cfg.Defaults.Timeout
	}
	return &Runner{
		cfg:     cfg,
		client:  client,
		opts:    opts,
		recipes: make(map[string]parser.RecipeDef),
	}
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
	// Run hooks + tests
	for _, t := range tests {
		testResults, err := r.runTest(ctx, prog, t, path)
		if err != nil {
			return results, err
		}
		results = append(results, testResults...)
	}
	return results, nil
}

func (r *Runner) runTest(ctx context.Context, prog *parser.Program, t parser.TestDef, file string) ([]TestResult, error) {
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
	exec := NewExecutor(r.client, r.opts.Timeout)
	for name, rec := range r.recipes {
		exec.RegisterRecipe(rec)
		_ = name
	}
	if vars != nil {
		for k, v := range vars {
			exec.SetVar(k, v)
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

	name := t.Name
	if row >= 0 && t.Examples != nil {
		name = fmt.Sprintf("%s [row %d]", t.Name, row+1)
	}

	return TestResult{
		TestName: name,
		File:     file,
		Passed:   runErr == nil,
		Duration: time.Since(start),
		Error:    runErr,
		Row:      row,
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
		if err := r.loadRecipeFile(nil, filepath.Join(r.cfg.Recipes, e.Name())); err != nil {
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
