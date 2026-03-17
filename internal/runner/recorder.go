package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// VideoRecorder captures device screen recordings at the OS level.
// It auto-detects the best available backend for each platform.
type VideoRecorder struct {
	manager    *device.Manager
	serial     string
	platform   device.Platform
	outputDir  string
	videoCfg   config.VideoConfig
	backend    string    // "scrcpy" | "screenrecord" | "screencap" | "simctl"
	cmd        *exec.Cmd // the recording process
	remotePath string    // on-device path (Android screenrecord)
	localPath  string    // final local path
	segments   []string  // for screenrecord chaining or screencap frames
	stopCh     chan struct{}
	frameIdx   int
	mu         sync.Mutex // protects cmd, segments, frameIdx, remotePath
}

// NewVideoRecorder creates a VideoRecorder, auto-detecting the best backend.
func NewVideoRecorder(manager *device.Manager, serial string, platform device.Platform, outputDir string, videoCfg config.VideoConfig) *VideoRecorder {
	vr := &VideoRecorder{
		manager:   manager,
		serial:    serial,
		platform:  platform,
		outputDir: outputDir,
		videoCfg:  videoCfg,
		stopCh:    make(chan struct{}),
	}

	switch platform {
	case device.PlatformIOS:
		vr.backend = "simctl"
	case device.PlatformAndroid:
		if _, err := exec.LookPath("scrcpy"); err == nil {
			vr.backend = "scrcpy"
		} else {
			// Try screenrecord — it's built into Android but may not work on all devices
			vr.backend = "screenrecord"
		}
	}

	fmt.Printf("    \033[36m🎬\033[0m  Video backend: %s\n", vr.backend)
	return vr
}

// Start begins recording in the background.
func (vr *VideoRecorder) Start(ctx context.Context, testName string) error {
	safeName := sanitizeName(testName)
	if err := os.MkdirAll(vr.outputDir, 0755); err != nil {
		return fmt.Errorf("video: mkdir: %w", err)
	}

	switch vr.backend {
	case "scrcpy":
		return vr.startScrcpy(ctx, safeName)
	case "screenrecord":
		return vr.startScreenrecord(ctx, safeName)
	case "screencap":
		return vr.startScreencap(ctx, safeName)
	case "simctl":
		return vr.startSimctl(ctx, safeName)
	default:
		return fmt.Errorf("video: unknown backend %q", vr.backend)
	}
}

// Stop stops the recording and returns the local path to the video file.
func (vr *VideoRecorder) Stop(ctx context.Context) (string, error) {
	switch vr.backend {
	case "scrcpy":
		return vr.stopScrcpy(ctx)
	case "screenrecord":
		return vr.stopScreenrecord(ctx)
	case "screencap":
		return vr.stopScreencap(ctx)
	case "simctl":
		return vr.stopSimctl(ctx)
	default:
		return "", fmt.Errorf("video: unknown backend %q", vr.backend)
	}
}

// --- scrcpy (best Android backend) ---

func (vr *VideoRecorder) startScrcpy(_ context.Context, safeName string) error {
	vr.localPath = filepath.Join(vr.outputDir, safeName+".mp4")
	vr.cmd = exec.Command("scrcpy",
		"-s", vr.serial,
		"--no-display",
		"--record="+vr.localPath,
	)
	if err := vr.cmd.Start(); err != nil {
		// Fall back to screenrecord
		vr.backend = "screenrecord"
		fmt.Printf("    \033[33m⚠\033[0m  scrcpy failed, falling back to screenrecord\n")
		return vr.startScreenrecord(context.Background(), safeName)
	}
	return nil
}

func (vr *VideoRecorder) stopScrcpy(_ context.Context) (string, error) {
	if vr.cmd == nil || vr.cmd.Process == nil {
		return "", fmt.Errorf("video: no scrcpy process")
	}
	_ = vr.cmd.Process.Signal(syscall.SIGINT)
	_ = vr.cmd.Wait()
	if err := verifyFile(vr.localPath); err != nil {
		return "", err
	}
	return vr.localPath, nil
}

// --- screenrecord (built-in Android, 180s limit) ---

func (vr *VideoRecorder) startScreenrecord(_ context.Context, safeName string) error {
	vr.localPath = filepath.Join(vr.outputDir, safeName+".mp4")
	vr.remotePath = "/sdcard/probe_recording.mp4"

	adbBin := vr.manager.ADB().Bin()
	vr.cmd = exec.Command(adbBin, "-s", vr.serial,
		"shell", "screenrecord", "--size", vr.videoCfg.Resolution, vr.remotePath)
	if err := vr.cmd.Start(); err != nil {
		// Fall back to screencap
		vr.backend = "screencap"
		fmt.Printf("    \033[33m⚠\033[0m  screenrecord failed, falling back to screencap\n")
		return vr.startScreencap(context.Background(), safeName)
	}

	// Start chaining goroutine: restart periodically to avoid Android's 180s limit
	cycleInterval := vr.videoCfg.ScreenrecordCycle
	go func() {
		segIdx := 0
		for {
			select {
			case <-vr.stopCh:
				return
			case <-time.After(cycleInterval):
				vr.mu.Lock()
				// Stop current recording
				if vr.cmd != nil && vr.cmd.Process != nil {
					_ = vr.cmd.Process.Signal(syscall.SIGINT)
					_ = vr.cmd.Wait()
				}
				// Pull segment
				segPath := filepath.Join(vr.outputDir, fmt.Sprintf("segment_%03d.mp4", segIdx))
				_ = vr.manager.ADB().Pull(context.Background(), vr.serial, vr.remotePath, segPath)
				vr.segments = append(vr.segments, segPath)
				segIdx++
				// Start new segment
				newRemote := fmt.Sprintf("/sdcard/probe_recording_%d.mp4", segIdx)
				vr.remotePath = newRemote
				vr.cmd = exec.Command(adbBin, "-s", vr.serial,
					"shell", "screenrecord", "--size", vr.videoCfg.Resolution, newRemote)
				_ = vr.cmd.Start()
				vr.mu.Unlock()
			}
		}
	}()

	return nil
}

func (vr *VideoRecorder) stopScreenrecord(ctx context.Context) (string, error) {
	close(vr.stopCh)
	// Wait briefly for the chaining goroutine to exit after receiving stopCh
	time.Sleep(200 * time.Millisecond)
	vr.mu.Lock()
	if vr.cmd != nil && vr.cmd.Process != nil {
		_ = vr.cmd.Process.Signal(syscall.SIGINT)
		_ = vr.cmd.Wait()
	}
	// Pull final segment
	finalSeg := filepath.Join(vr.outputDir, fmt.Sprintf("segment_%03d.mp4", len(vr.segments)))
	if err := vr.manager.ADB().Pull(ctx, vr.serial, vr.remotePath, finalSeg); err == nil {
		vr.segments = append(vr.segments, finalSeg)
	}
	vr.mu.Unlock()

	if len(vr.segments) == 1 {
		// Single segment — just rename
		if err := os.Rename(vr.segments[0], vr.localPath); err != nil {
			return vr.segments[0], nil
		}
		return vr.localPath, nil
	}

	// Try ffmpeg concat
	if _, err := exec.LookPath("ffmpeg"); err == nil && len(vr.segments) > 1 {
		concatFile := filepath.Join(vr.outputDir, "concat.txt")
		var lines []string
		for _, seg := range vr.segments {
			lines = append(lines, fmt.Sprintf("file '%s'", seg))
		}
		if err := os.WriteFile(concatFile, []byte(strings.Join(lines, "\n")), 0644); err == nil {
			cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0",
				"-i", concatFile, "-c", "copy", vr.localPath)
			if err := cmd.Run(); err == nil {
				os.Remove(concatFile)
				for _, seg := range vr.segments {
					os.Remove(seg)
				}
				return vr.localPath, nil
			}
			os.Remove(concatFile)
		}
	}

	// ffmpeg not available — return first segment
	if len(vr.segments) > 0 {
		fmt.Printf("    \033[33m⚠\033[0m  ffmpeg not found, keeping %d video segments as separate files\n", len(vr.segments))
		return vr.segments[0], nil
	}
	return "", fmt.Errorf("video: no segments captured")
}

// --- screencap (simplest Android fallback) ---

func (vr *VideoRecorder) startScreencap(_ context.Context, safeName string) error {
	vr.localPath = filepath.Join(vr.outputDir, safeName+".mp4")
	framesDir := filepath.Join(vr.outputDir, "frames")
	if err := os.MkdirAll(framesDir, 0755); err != nil {
		return err
	}

	// Determine capture interval from configured framerate
	captureInterval := time.Second
	if vr.videoCfg.Framerate > 0 {
		captureInterval = time.Second / time.Duration(vr.videoCfg.Framerate)
	}

	go func() {
		adbBin := vr.manager.ADB().Bin()
		for {
			select {
			case <-vr.stopCh:
				return
			default:
				vr.mu.Lock()
				idx := vr.frameIdx
				vr.mu.Unlock()
				framePath := filepath.Join(framesDir, fmt.Sprintf("frame_%04d.png", idx))
				cmd := exec.Command(adbBin, "-s", vr.serial, "exec-out", "screencap", "-p")
				data, err := cmd.Output()
				if err == nil && len(data) > 0 {
					_ = os.WriteFile(framePath, data, 0644)
					vr.mu.Lock()
					vr.segments = append(vr.segments, framePath)
					vr.frameIdx++
					vr.mu.Unlock()
				}
				time.Sleep(captureInterval)
			}
		}
	}()

	return nil
}

func (vr *VideoRecorder) stopScreencap(ctx context.Context) (string, error) {
	close(vr.stopCh)
	time.Sleep(100 * time.Millisecond) // let goroutine exit

	if len(vr.segments) == 0 {
		return "", fmt.Errorf("video: no frames captured")
	}

	// Try ffmpeg stitching
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		framesDir := filepath.Join(vr.outputDir, "frames")
		pattern := filepath.Join(framesDir, "frame_%04d.png")
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
			"-framerate", fmt.Sprintf("%d", vr.videoCfg.Framerate),
			"-i", pattern,
			"-c:v", "libx264", "-pix_fmt", "yuv420p",
			vr.localPath)
		if err := cmd.Run(); err == nil {
			// Clean up frames
			os.RemoveAll(framesDir)
			return vr.localPath, nil
		}
	}

	// No ffmpeg — return the frames directory
	fmt.Printf("    \033[33m⚠\033[0m  ffmpeg not found, keeping %d PNG frames in %s\n", len(vr.segments), filepath.Join(vr.outputDir, "frames"))
	return vr.segments[0], nil
}

// --- simctl (iOS) ---

func (vr *VideoRecorder) startSimctl(_ context.Context, safeName string) error {
	vr.localPath = filepath.Join(vr.outputDir, safeName+".mov")
	vr.cmd = exec.Command("xcrun", "simctl", "io", vr.serial, "recordVideo", "--codec=h264", "--force", vr.localPath)
	if err := vr.cmd.Start(); err != nil {
		return fmt.Errorf("video: simctl recordVideo: %w", err)
	}
	return nil
}

func (vr *VideoRecorder) stopSimctl(_ context.Context) (string, error) {
	if vr.cmd == nil || vr.cmd.Process == nil {
		return "", fmt.Errorf("video: no simctl process")
	}
	_ = vr.cmd.Process.Signal(syscall.SIGINT)
	_ = vr.cmd.Wait()
	if err := verifyFile(vr.localPath); err != nil {
		return "", err
	}
	return vr.localPath, nil
}

// verifyFile checks that the file exists and has content.
func verifyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("video: output file missing: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("video: output file is empty: %s", path)
	}
	return nil
}
