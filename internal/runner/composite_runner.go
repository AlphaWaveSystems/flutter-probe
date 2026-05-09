package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// CompositeDevice pairs a device alias with its live connection.
type CompositeDevice struct {
	Alias      string
	Client     probelink.ProbeClient
	DeviceCtx  *DeviceContext // nil if the device has no platform ops (e.g. pure WiFi)
	DeviceID   string
	DeviceName string
}

// CompositeDeviceResult holds the outcome for one device in a composite test.
type CompositeDeviceResult struct {
	Alias     string
	Passed    bool
	Error     error
	Artifacts []string
}

// CompositeTestResult is the outcome of a single composite test. It maps each
// device alias to an individual result, plus an overall pass/fail.
type CompositeTestResult struct {
	TestName      string
	File          string
	Passed        bool
	Skipped       bool
	Duration      time.Duration
	DeviceResults map[string]CompositeDeviceResult
	Row           int
}

// ToTestResult flattens the per-device results into a single TestResult for
// the existing reporter and runner pipeline.
func (c CompositeTestResult) ToTestResult() TestResult {
	var errParts []string
	var artifacts []string
	for alias, dr := range c.DeviceResults {
		if dr.Error != nil && !errors.Is(dr.Error, context.Canceled) {
			errParts = append(errParts, fmt.Sprintf("[%s] %v", alias, dr.Error))
		}
		artifacts = append(artifacts, dr.Artifacts...)
	}
	var runErr error
	if len(errParts) > 0 {
		runErr = fmt.Errorf("%s", strings.Join(errParts, "; "))
	}
	return TestResult{
		TestName:  c.TestName,
		File:      c.File,
		Passed:    c.Passed,
		Skipped:   c.Skipped,
		Duration:  c.Duration,
		Error:     runErr,
		Artifacts: artifacts,
		Row:       c.Row,
	}
}

// CompositeRunner executes composite tests across multiple devices.
type CompositeRunner struct {
	devices []CompositeDevice
	cfg     *config.Config
	recipes map[string]parser.RecipeDef
	opts    RunOptions
}

// NewCompositeRunner creates a CompositeRunner with the given devices.
func NewCompositeRunner(cfg *config.Config, devices []CompositeDevice, opts RunOptions) *CompositeRunner {
	return &CompositeRunner{
		devices: devices,
		cfg:     cfg,
		recipes: make(map[string]parser.RecipeDef),
		opts:    opts,
	}
}

// RegisterRecipe adds a shared recipe available to all devices.
func (cr *CompositeRunner) RegisterRecipe(r parser.RecipeDef) {
	cr.recipes[r.Name] = r
}

// Devices returns the composite devices configured for this runner.
func (cr *CompositeRunner) Devices() []CompositeDevice {
	return cr.devices
}

// HasAlias reports whether a device with the given alias is configured.
func (cr *CompositeRunner) HasAlias(alias string) bool {
	for _, d := range cr.devices {
		if d.Alias == alias {
			return true
		}
	}
	return false
}

// RunCompositeTest executes one composite test. It launches one goroutine per
// device and coordinates them through shared sync barriers.
//
// Failure semantics:
//   - If a device goroutine errors, it cancels the shared composite context.
//   - Other goroutines blocked at a sync barrier unblock via ctx.Done().
//   - Other goroutines executing steps fail on their next step via ctx.Err().
//   - The result shows FAIL for the failing device and CANCELLED for others.
func (cr *CompositeRunner) RunCompositeTest(ctx context.Context, test parser.CompositeTestDef, file string) CompositeTestResult {
	start := time.Now()
	result := CompositeTestResult{
		TestName:      test.Name,
		File:          file,
		Row:           -1,
		DeviceResults: make(map[string]CompositeDeviceResult),
	}

	// Validate: every alias declared in the test must be configured.
	for _, decl := range test.Devices {
		if !cr.HasAlias(decl.Alias) {
			result.Skipped = true
			for _, decl2 := range test.Devices {
				result.DeviceResults[decl2.Alias] = CompositeDeviceResult{
					Alias: decl2.Alias,
					Error: fmt.Errorf("device alias %q not configured (use --composite-device or probe.yaml composite.devices)", decl.Alias),
				}
			}
			result.Duration = time.Since(start)
			return result
		}
	}

	// Build barriers: one per unique sync label. Each barrier is initialized
	// with the number of devices so all goroutines must arrive before any proceeds.
	n := len(cr.devices)
	barriers := buildBarriers(test.Body, n)

	// Partition body steps by device alias. SyncStep nodes are appended to
	// every device's step list so each goroutine independently calls Arrive.
	deviceSteps := partitionSteps(test.Body, cr.devices)

	// Shared cancellable context — cancelled by the first device that fails.
	compCtx, compCancel := context.WithCancel(ctx)
	defer compCancel()

	type gorResult struct {
		alias string
		err   error
		arts  []string
	}
	resultCh := make(chan gorResult, n)

	for _, dev := range cr.devices {
		steps := deviceSteps[dev.Alias]
		go func(d CompositeDevice, steps []parser.Step) {
			exec := NewExecutor(d.Client, d.DeviceCtx, nil, cr.opts.Timeout, cr.opts.Verbose)
			exec.SetReconnectPolicy(cr.cfg.Agent.ReconnectAttempts, cr.cfg.Agent.ReconnectBackoff)
			for _, rec := range cr.recipes {
				exec.RegisterRecipe(rec)
			}

			err := runCompositeSteps(compCtx, exec, steps, barriers)

			// On failure: abort all barriers so other devices are not left
			// hanging, then cancel the shared context.
			if err != nil && !errors.Is(err, context.Canceled) {
				for _, b := range barriers {
					b.Abort()
				}
				compCancel()
			}

			resultCh <- gorResult{alias: d.Alias, err: err, arts: exec.Artifacts()}
		}(dev, steps)
	}

	// Collect all goroutine results.
	for i := 0; i < n; i++ {
		r := <-resultCh
		dr := CompositeDeviceResult{
			Alias:     r.alias,
			Passed:    r.err == nil,
			Artifacts: r.arts,
		}
		if r.err != nil {
			dr.Error = r.err
		}
		result.DeviceResults[r.alias] = dr
	}

	result.Passed = true
	for _, dr := range result.DeviceResults {
		if !dr.Passed {
			result.Passed = false
			break
		}
	}
	result.Duration = time.Since(start)
	return result
}

// runCompositeSteps executes a device's step sequence, intercepting SyncStep
// nodes to coordinate with the shared barrier map.
func runCompositeSteps(ctx context.Context, exec *Executor, steps []parser.Step, barriers map[string]*syncBarrier) error {
	for _, step := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s, ok := step.(parser.SyncStep); ok {
			b, ok := barriers[s.Label]
			if !ok {
				return fmt.Errorf("sync %q: unknown barrier (bug: barrier not pre-built)", s.Label)
			}
			if err := b.Arrive(ctx); err != nil {
				return fmt.Errorf("sync %q: %w", s.Label, err)
			}
			continue
		}
		if err := exec.RunStep(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

// buildBarriers scans a composite test body for SyncStep nodes and creates one
// syncBarrier per unique label, pre-initialized with the device count n.
func buildBarriers(body []parser.Step, n int) map[string]*syncBarrier {
	barriers := make(map[string]*syncBarrier)
	for _, step := range body {
		if s, ok := step.(parser.SyncStep); ok {
			if _, exists := barriers[s.Label]; !exists {
				barriers[s.Label] = newSyncBarrier(n)
			}
		}
	}
	return barriers
}

// partitionSteps distributes composite body steps into per-device step lists.
// DeviceStep nodes go to their target alias; SyncStep nodes go to all devices.
func partitionSteps(body []parser.Step, devices []CompositeDevice) map[string][]parser.Step {
	result := make(map[string][]parser.Step, len(devices))
	for _, d := range devices {
		result[d.Alias] = nil
	}
	for _, step := range body {
		switch s := step.(type) {
		case parser.DeviceStep:
			result[s.Alias] = append(result[s.Alias], s.Step)
		case parser.SyncStep:
			for alias := range result {
				result[alias] = append(result[alias], s)
			}
		}
	}
	return result
}
