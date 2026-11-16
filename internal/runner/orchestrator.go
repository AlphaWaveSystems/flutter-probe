package runner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
	"golang.org/x/text/unicode/norm"
)

// DeviceRun tracks a parallel test execution on a single device.
type DeviceRun struct {
	DeviceID   string
	DeviceName string
	Platform   device.Platform
	AppID      string // package name / bundle ID (can differ per platform)
	Port       int    // host-side port for this device
	Files      []string
	Results    []TestResult
	Error      error
	Duration   time.Duration
}

// ParallelOrchestrator distributes test files across multiple devices
// and runs them concurrently.
type ParallelOrchestrator struct {
	cfg      *config.Config
	manager  *device.Manager
	devices  []DeviceRun
	opts     RunOptions
	portBase int
	verbose  bool

	// dial opens a device's agent connection. It is nil in production, which dials with probelink.DialWithOptions; a test sets it to reach a
	// fake agent. It is the only seam of the orchestrator.
	dial func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error)
}

// dialAgent opens a device's agent connection with po.dial, or with probelink.DialWithOptions when po.dial is nil. A failed dial returns a
// nil client, never a typed nil inside a non-nil interface.
func (po *ParallelOrchestrator) dialAgent(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
	if po.dial != nil {
		return po.dial(ctx, opts)
	}
	c, err := probelink.DialWithOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// NewParallelOrchestrator creates an orchestrator for parallel test execution.
func NewParallelOrchestrator(
	cfg *config.Config,
	manager *device.Manager,
	devices []DeviceRun,
	opts RunOptions,
	portBase int,
) *ParallelOrchestrator {
	return &ParallelOrchestrator{
		cfg:      cfg,
		manager:  manager,
		devices:  devices,
		opts:     opts,
		portBase: portBase,
		verbose:  opts.Verbose,
	}
}

// Run executes tests in parallel across all devices.
func (po *ParallelOrchestrator) Run(ctx context.Context) ([]TestResult, error) {
	fmt.Printf("  Parallel mode: %d devices, %d files\n\n", len(po.devices), po.totalFiles())

	for i, dr := range po.devices {
		fmt.Printf("  Device %d: %s (%d files, port %d)\n", i+1, po.deviceLabel(dr), len(dr.Files), dr.Port)
	}
	fmt.Println()

	var wg sync.WaitGroup
	for i := range po.devices {
		wg.Add(1)
		go func(dr *DeviceRun) {
			defer wg.Done()
			start := time.Now()

			// Retry up to 2 times, and only a failure to connect: a lane that has connected ran its files once, and a retry would run the
			// completed ones again. Its results so far are kept with its error.
			maxRetries := 2
			for attempt := 0; attempt <= maxRetries; attempt++ {
				dr.Results, dr.Error = po.runOnDevice(ctx, dr)
				var ce *connectError
				if dr.Error == nil || !errors.As(dr.Error, &ce) {
					break
				}
				if attempt < maxRetries {
					fmt.Printf("  \033[33m⚠\033[0m  [%s] attempt %d failed: %v — retrying in 5s...\n",
						po.shortID(dr.DeviceID), attempt+1, dr.Error)
					time.Sleep(5 * time.Second)
				}
			}

			addNotRun(dr)
			dr.Duration = time.Since(start)
			if dr.Error != nil {
				fmt.Printf("  \033[31m✗\033[0m  [%s] all attempts failed: %v\n",
					po.shortID(dr.DeviceID), dr.Error)
			}
		}(&po.devices[i])
	}
	wg.Wait()

	return po.mergeResults(), po.summaryError()
}

// connectError is a failure before a lane runs its first file: the port forward, the token, the dial or the handshake. Only these are retried.
// A failure after the lane has connected is a run error, and its results are kept.
type connectError struct{ err error }

func (e *connectError) Error() string { return e.err.Error() }
func (e *connectError) Unwrap() error { return e.err }

// runOnDevice connects to a single device and runs its assigned files.
func (po *ParallelOrchestrator) runOnDevice(ctx context.Context, dr *DeviceRun) ([]TestResult, error) {
	// Setup port forward for Android
	if dr.Platform == device.PlatformAndroid {
		devPort := po.cfg.Agent.AgentDevicePort()
		if err := po.manager.ForwardPort(ctx, dr.DeviceID, dr.Port, devPort); err != nil {
			return nil, &connectError{fmt.Errorf("[%s] port forward: %w", dr.DeviceID, err)}
		}
		defer po.manager.RemoveForward(ctx, dr.DeviceID, dr.Port)
	}

	// Read token
	var token string
	var err error
	tokenTimeout := po.cfg.Agent.TokenReadTimeout
	if tokenTimeout == 0 {
		tokenTimeout = 30 * time.Second
	}

	appID := dr.AppID
	if appID == "" {
		appID = po.cfg.Project.App
	}

	switch dr.Platform {
	case device.PlatformAndroid:
		token, err = po.manager.ReadTokenAndroid(ctx, dr.DeviceID, tokenTimeout, appID, nil)
	case device.PlatformIOS:
		token, err = po.manager.ReadTokenIOS(ctx, dr.DeviceID, tokenTimeout, appID)
	}
	if err != nil {
		return nil, &connectError{fmt.Errorf("[%s] token: %w", dr.DeviceID, err)}
	}

	// Connect WebSocket
	dialTimeout := po.cfg.Agent.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 30 * time.Second
	}

	client, err := po.dialAgent(ctx, probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        dr.Port,
		Token:       token,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		hint := ""
		if dr.Platform == device.PlatformAndroid {
			hint = device.ConnectHint(ctx, po.manager.ADB(), dr.DeviceID, dr.Port, po.cfg.Agent.AgentDevicePort(), appID)
		}
		return nil, &connectError{fmt.Errorf("[%s] connect: %w%s", dr.DeviceID, err, hint)}
	}
	defer client.Close()

	warning, err := probelink.CheckHandshake(ctx, client, po.cfg.CLIVersion)
	if err != nil {
		return nil, &connectError{fmt.Errorf("[%s] handshake: %w", dr.DeviceID, err)}
	}
	if warning != "" {
		fmt.Printf("  \033[33m⚠\033[0m  [%s] %s\n", po.shortID(dr.DeviceID), sysdialog.ScrubKnown(warning))
	}

	fmt.Printf("  \033[32m✓\033[0m  [%s] Connected\n", po.shortID(dr.DeviceID))

	// Create device context
	devCtx := po.deviceContext(ctx, dr, appID, tokenTimeout, dialTimeout)

	// Create runner for this device
	deviceOpts := po.opts
	deviceOpts.Files = dr.Files
	deviceOpts.DeviceID = dr.DeviceID
	deviceOpts.DeviceName = dr.DeviceName

	r := New(po.cfg, client, devCtx, deviceOpts)
	vc := visual.NewComparatorWithConfig(".", po.cfg.Visual.Threshold, po.cfg.Visual.PixelDelta)
	r.SetVisual(vc)

	return r.Run(ctx)
}

// deviceContext is the device context of one device of a parallel run. Whether the device is physical is asked of the
// device, the same check a single-device run makes, so a physical device is never taken for an emulator.
func (po *ParallelOrchestrator) deviceContext(ctx context.Context, dr *DeviceRun, appID string, tokenTimeout, dialTimeout time.Duration) *DeviceContext {
	dc := NewDeviceContext(ctx, po.manager, dr.Platform, dr.DeviceID)
	dc.AppID = appID
	dc.Port = dr.Port
	dc.DevicePort = po.cfg.Agent.AgentDevicePort()
	// A lane has nobody to ask: its wipes follow the run's clear gate (see ApplyClearGate).
	dc.ApplyClearGate(po.opts.ClearGate)
	dc.GrantPermissionsOnClear = po.cfg.Defaults.GrantPermissionsOnClear
	dc.CrashEvidence = po.cfg.Defaults.CrashEvidence
	dc.ReconnectDelay = po.cfg.Agent.ReconnectDelay
	dc.RestartDelay = po.cfg.Device.RestartDelay
	dc.TokenReadTimeout = tokenTimeout
	dc.DialTimeout = dialTimeout
	dc.CLIVersion = po.cfg.CLIVersion
	return dc
}

// mergeResults combines results from all devices in file order.
func (po *ParallelOrchestrator) mergeResults() []TestResult {
	var all []TestResult
	for _, dr := range po.devices {
		all = append(all, dr.Results...)
	}
	return all
}

// addNotRun gives a not-run result to each file of a lane that never connected, so the report shows the files the lane did not reach. A
// lane that connected has its files accounted for by Run: the file it stopped at has its failure, and the files after it are not run. A
// file that ran and has no results (no tests, or none a tag let through) is not reported as not run.
func addNotRun(dr *DeviceRun) {
	var ce *connectError
	if dr.Error == nil || !errors.As(dr.Error, &ce) {
		return
	}
	dr.Results = append(dr.Results, notRunAll(dr.Files, firstFile(dr.Files), dr.Error)...)
}

// summaryError returns an error if any device failed to connect.
func (po *ParallelOrchestrator) summaryError() error {
	var errs []string
	for _, dr := range po.devices {
		if dr.Error != nil {
			errs = append(errs, fmt.Sprintf("[%s] %v", dr.DeviceID, dr.Error))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("device errors:\n  %s", joinLines(errs))
	}
	return nil
}

// PrintSummary prints a per-device summary table.
func (po *ParallelOrchestrator) PrintSummary() {
	fmt.Println("\n  \033[1m── Device Summary ──\033[0m")
	maxWall := time.Duration(0)
	for _, dr := range po.devices {
		if dr.Duration > maxWall {
			maxWall = dr.Duration
		}
		passed, failed := 0, 0
		for _, r := range dr.Results {
			if r.Passed {
				passed++
			} else if !r.Skipped {
				failed++
			}
		}
		status := "\033[32m✓\033[0m"
		if failed > 0 {
			status = "\033[31m✗\033[0m"
		}
		if dr.Error != nil {
			status = "\033[31m⚠\033[0m"
			fmt.Printf("  %s  %-20s  error: %v\n", status, po.shortID(dr.DeviceID), dr.Error)
		} else {
			fmt.Printf("  %s  %-20s  %d passed, %d failed  (%s)\n",
				status, po.shortID(dr.DeviceID), passed, failed, dr.Duration.Round(time.Millisecond))
		}
	}
	fmt.Println("  \033[1m────────────────────\033[0m")
}

func (po *ParallelOrchestrator) totalFiles() int {
	n := 0
	for _, dr := range po.devices {
		n += len(dr.Files)
	}
	return n
}

func (po *ParallelOrchestrator) deviceLabel(dr DeviceRun) string {
	if dr.DeviceName != "" {
		return dr.DeviceName
	}
	return dr.DeviceID
}

func (po *ParallelOrchestrator) shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func joinLines(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += "\n  "
		}
		result += s
	}
	return result
}

// ResolveAppID returns the correct app ID for a platform.
// Flutter uses camelCase bundle IDs on iOS (com.example.myApp) but
// snake_case package names on Android (com.example.my_app).
// If the configured app ID contains uppercase letters and the platform
// is Android, convert to snake_case.
func ResolveAppID(configAppID string, platform device.Platform) string {
	if platform != device.PlatformAndroid {
		return configAppID
	}
	// Check if the last segment has uppercase (camelCase iOS bundle ID)
	parts := strings.Split(configAppID, ".")
	last := parts[len(parts)-1]
	hasUpper := false
	for _, r := range last {
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return configAppID // already snake_case or no conversion needed
	}
	// Convert camelCase to snake_case in the last segment
	var result []byte
	for i, r := range last {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(r+'a'-'A'))
		} else {
			result = append(result, byte(r))
		}
	}
	parts[len(parts)-1] = string(result)
	return strings.Join(parts, ".")
}

// ---- File Distribution ----

// DistributeFiles assigns files to N buckets using round-robin.
func DistributeFiles(files []string, n int) [][]string {
	if n <= 0 {
		n = 1
	}
	buckets := make([][]string, n)
	for i, f := range files {
		buckets[i%n] = append(buckets[i%n], f)
	}
	return buckets
}

// ShardFiles returns the subset of files for shard index/total. A file's shard comes from its path relative to base, the directory of the
// config (the working directory when base is empty), so the same file lands in the same shard however the run spells it: relative, with
// ./, absolute, with a trailing separator, under a symbolic link to the checkout, or with Windows separators.
func ShardFiles(files []string, base string, index, total int) []string {
	if total <= 1 {
		return files
	}
	// Sort files first for deterministic ordering
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)

	var result []string
	for _, f := range sorted {
		key, _ := ShardKey(f, base)
		h := sha256.Sum256([]byte(key))
		bucket := int(h[0]) % total
		if bucket == index {
			result = append(result, f)
		}
	}
	return result
}

// ShardKey is the path a file is hashed by. It is the file's path relative to the repository root, the nearest ancestor of base that holds
// a .git, or relative to base when there is no repository. A file inside neither keeps its absolute path, and outside reports it, so the
// caller can say so. A typed relative path is taken from the working directory, and base is the config directory (the working directory
// when empty). Every path has its symbolic links resolved where they exist, is cleaned, has forward slashes, is in Unicode NFC, and is
// lower-cased: the same checkout hashes the same wherever it is and on any file system. Two files can collide; they then share a shard.
func ShardKey(file, base string) (key string, outside bool) {
	if base == "" {
		base = "."
	}
	if b, err := filepath.Abs(base); err == nil {
		base = b
	}
	base = resolvePath(base)
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	abs = resolvePath(abs)
	root := gitRepoRoot(base)
	if root == "" {
		root = base
	}
	if rel, ok := relInside(root, abs); ok {
		return normalizeShardKey(rel), false
	}
	if rel, ok := relInside(base, abs); ok {
		return normalizeShardKey(rel), false
	}
	return normalizeShardKey(abs), true
}

// relInside returns the path of p relative to root, and whether p is inside root.
func relInside(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// gitRepoRoot returns the nearest directory at or above dir that holds a .git, or "" when there is none.
func gitRepoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// normalizeShardKey makes a key the same on every platform: cleaned, with forward slashes, in Unicode NFC (macOS names files in NFD), and
// lower-cased (case-insensitive file systems).
func normalizeShardKey(key string) string {
	k := strings.ReplaceAll(filepath.ToSlash(filepath.Clean(key)), `\`, "/")
	return strings.ToLower(norm.NFC.String(k))
}

// resolvePath resolves the symbolic links of p. A path that does not exist (a file that is named but not there) resolves through its
// nearest existing parent, so it is spelled the same way as a path that exists under the same checkout.
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolvePath(parent), filepath.Base(p))
}

// ParseShard parses "N/M" format into zero-based index and total.
// Returns (0, 0, nil) if empty string.
func ParseShard(s string) (index, total int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	var n, m int
	_, err = fmt.Sscanf(s, "%d/%d", &n, &m)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid shard format %q — expected N/M (e.g. 1/3)", s)
	}
	if n < 1 || n > m {
		return 0, 0, fmt.Errorf("shard %d/%d out of range — N must be 1..M", n, m)
	}
	return n - 1, m, nil // convert to 0-based
}
