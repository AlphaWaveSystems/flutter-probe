package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
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

			// Retry up to 2 times on connection failure
			maxRetries := 2
			for attempt := 0; attempt <= maxRetries; attempt++ {
				dr.Results, dr.Error = po.runOnDevice(ctx, dr)
				if dr.Error == nil {
					break
				}
				if attempt < maxRetries {
					fmt.Printf("  \033[33m⚠\033[0m  [%s] attempt %d failed: %v — retrying in 5s...\n",
						po.shortID(dr.DeviceID), attempt+1, dr.Error)
					time.Sleep(5 * time.Second)
				}
			}

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

// runOnDevice connects to a single device and runs its assigned files.
func (po *ParallelOrchestrator) runOnDevice(ctx context.Context, dr *DeviceRun) ([]TestResult, error) {
	// Setup port forward for Android
	if dr.Platform == device.PlatformAndroid {
		devPort := po.cfg.Agent.AgentDevicePort()
		if err := po.manager.ForwardPort(ctx, dr.DeviceID, dr.Port, devPort); err != nil {
			return nil, fmt.Errorf("[%s] port forward: %w", dr.DeviceID, err)
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
		token, err = po.manager.ReadTokenAndroid(ctx, dr.DeviceID, tokenTimeout, appID)
	case device.PlatformIOS:
		token, err = po.manager.ReadTokenIOS(ctx, dr.DeviceID, tokenTimeout, appID)
	}
	if err != nil {
		return nil, fmt.Errorf("[%s] token: %w", dr.DeviceID, err)
	}

	// Connect WebSocket
	dialTimeout := po.cfg.Agent.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 30 * time.Second
	}

	client, err := probelink.DialWithOptions(ctx, probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        dr.Port,
		Token:       token,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("[%s] connect: %w", dr.DeviceID, err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("[%s] ping: %w", dr.DeviceID, err)
	}

	fmt.Printf("  \033[32m✓\033[0m  [%s] Connected\n", po.shortID(dr.DeviceID))

	// Create device context
	devCtx := &DeviceContext{
		Manager:                 po.manager,
		Serial:                  dr.DeviceID,
		Platform:                dr.Platform,
		AppID:                   appID,
		Port:                    dr.Port,
		DevicePort:              po.cfg.Agent.AgentDevicePort(),
		AllowClearData:          true,
		GrantPermissionsOnClear: po.cfg.Defaults.GrantPermissionsOnClear,
		ReconnectDelay:          po.cfg.Agent.ReconnectDelay,
		RestartDelay:            po.cfg.Device.RestartDelay,
		TokenReadTimeout:        tokenTimeout,
		DialTimeout:             dialTimeout,
	}

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

// mergeResults combines results from all devices in file order.
func (po *ParallelOrchestrator) mergeResults() []TestResult {
	var all []TestResult
	for _, dr := range po.devices {
		all = append(all, dr.Results...)
	}
	return all
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

// ShardFiles returns the subset of files for shard index/total.
// Uses deterministic hash so the same files always go to the same shard.
func ShardFiles(files []string, index, total int) []string {
	if total <= 1 {
		return files
	}
	// Sort files first for deterministic ordering
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)

	var result []string
	for _, f := range sorted {
		h := sha256.Sum256([]byte(f))
		bucket := int(h[0]) % total
		if bucket == index {
			result = append(result, f)
		}
	}
	return result
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
