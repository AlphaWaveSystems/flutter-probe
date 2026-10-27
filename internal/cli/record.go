package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/record"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record gestures and interactions, then generate a .probe file",
	Long: `Record user interactions on a running Flutter app and generate a
ProbeScript (.probe) test file from them. Tap, type, swipe, and scroll
events are captured in real time via the ProbeAgent and translated to
natural-language test steps.`,
	Example: `  probe record --output tests/flow.probe
  probe record --device emulator-5554 --output tests/onboarding.probe
  probe record --port 9999 --token-timeout 60s`,
	RunE: runRecord,
}

func init() {
	f := recordCmd.Flags()
	f.StringP("output", "o", "tests/recorded.probe", "output .probe file path")
	f.String("device", "", "target device serial or UDID (default: first available)")
	f.Duration("timeout", 5*time.Minute, "maximum recording duration")
	f.Int("port", 0, "ProbeAgent WebSocket port (default: 48686)")
	f.Int("agent-port", 0, "alias for --port")
	f.Duration("token-timeout", 0, "max time to wait for agent auth token (default: 30s)")
	rootCmd.AddCommand(recordCmd)
}

// recordedEvent captures a single action during recording.
type recordedEvent struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
	At     time.Time              `json:"at"`
}

// checkRecordAgent refuses an agent whose version does not match this CLI, and prints the version warning to w. probe
// record runs it before recording starts.
func checkRecordAgent(ctx context.Context, client probelink.ProbeClient, cliVersion string, w io.Writer) error {
	_, warning, err := probelink.CheckHandshakeVersion(ctx, client, cliVersion)
	if err != nil {
		return fmt.Errorf("agent handshake failed: %w", err)
	}
	if warning != "" {
		printAgentWarning(w, warning)
	}
	return nil
}

func runRecord(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	outFile, _ := cmd.Flags().GetString("output")
	deviceSerial, _ := cmd.Flags().GetString("device")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	portFlag, _ := cmd.Flags().GetInt("port")
	if portFlag == 0 {
		portFlag, _ = cmd.Flags().GetInt("agent-port")
	}
	tokenTimeout, _ := cmd.Flags().GetDuration("token-timeout")

	// Load config (respects --config flag for platform-specific configs)
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	// Apply CLI overrides
	if portFlag != 0 {
		cfg.Agent.Port = portFlag
	}
	if tokenTimeout != 0 {
		cfg.Agent.TokenReadTimeout = tokenTimeout
	}

	// Connect to device
	dm := device.NewManager()
	platform := device.Platform(cfg.Defaults.Platform)
	if deviceSerial == "" {
		devices, err := dm.List(ctx)
		if err != nil || len(devices) == 0 {
			return fmt.Errorf("no connected devices — start an emulator first")
		}
		deviceSerial = devices[0].ID
		platform = devices[0].Platform
	} else {
		// Detect platform from device list
		devices, _ := dm.List(ctx)
		for _, d := range devices {
			if d.ID == deviceSerial {
				platform = d.Platform
				break
			}
		}
	}

	agentPort := cfg.Agent.Port
	dialOpts := probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        agentPort,
		DialTimeout: cfg.Agent.DialTimeout,
	}

	if platform == device.PlatformIOS {
		// iOS: simulators share host loopback — no port forwarding needed
		fmt.Println("  Waiting for ProbeAgent token (iOS)...")
		token, err := dm.ReadTokenIOS(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
		if err != nil {
			return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
		}
		dialOpts.Token = token
	} else {
		// Android: forward port via ADB
		if err := dm.ForwardPort(ctx, deviceSerial, agentPort, cfg.Agent.AgentDevicePort()); err != nil {
			return fmt.Errorf("port forward: %w", err)
		}
		defer dm.RemoveForward(ctx, deviceSerial, agentPort) //nolint:errcheck

		fmt.Println("  Waiting for ProbeAgent token...")
		token, err := dm.ReadToken(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
		if err != nil {
			return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
		}
		dialOpts.Token = token
	}

	client, err := probelink.DialWithOptions(ctx, dialOpts)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer client.Close()

	// The obscured-field flag keeps a typed password out of a recording, and an agent built before it does not send
	// the flag. Refuse a mismatched agent before recording starts, as test does.
	if err := checkRecordAgent(ctx, client, cfg.CLIVersion, os.Stdout); err != nil {
		return err
	}

	var events []recordedEvent

	// Subscribe to recording notifications from the agent
	client.OnNotify = func(method string, params json.RawMessage) {
		if method == probelink.NotifyRecordedEvent {
			var p map[string]interface{}
			if err := json.Unmarshal(params, &p); err == nil {
				ev := recordedEvent{
					Method: method,
					Params: p,
					At:     time.Now(),
				}
				events = append(events, ev)
				printRecordedEvent(ev)
			}
		}
	}

	// Tell the agent to enter recording mode
	recCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err = client.Call(recCtx, probelink.MethodStartRecording, nil)
	if err != nil {
		return fmt.Errorf("start recording: %w", err)
	}

	fmt.Printf("\n  \033[32m●\033[0m  Recording on %s — interact with your app\n", deviceSerial)
	fmt.Println("  Press Ctrl+C or wait for timeout to stop and save.")

	// Wait for timeout or interrupt
	select {
	case <-recCtx.Done():
		// timeout
	case <-waitForInterrupt():
		// Ctrl+C
	}

	// Stop recording
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_, _ = client.Call(ctx2, probelink.MethodStopRecording, nil)

	// Generate .probe file
	probe := generateProbeScript(events)
	if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(outFile, []byte(probe), 0644); err != nil {
		return err
	}

	statusOK(os.Stdout, "Recorded %d events → %s", len(events), outFile)
	return nil
}

// printRecordedEvent prints a real-time feedback line for a recorded event.
func printRecordedEvent(e recordedEvent) {
	params := e.Params
	action, _ := params["action"].(string)
	sel := selectorFromParams(params)

	switch action {
	case "tap":
		statusOK(os.Stdout, "tap %s", sel)
	case "type":
		if obscuredEvent(params) {
			// The agent sends no value for an obscured field, so there is nothing to show or record for it.
			statusWarn(os.Stdout, "type into %s not recorded: the value of an obscured field is not saved. Add type \"${VAR}\" into it by hand.", sel)
			break
		}
		text, _ := params["text"].(string)
		if sel != "the element" {
			statusOK(os.Stdout, "type %q into %s", text, sel)
		} else {
			statusOK(os.Stdout, "type %q", text)
		}
	case "swipe":
		dir, _ := params["direction"].(string)
		statusOK(os.Stdout, "swipe %s", dir)
	case "long_press":
		statusOK(os.Stdout, "long press on %s", sel)
	default:
		statusOK(os.Stdout, "%s", action)
	}
}

// generateProbeScript converts recorded events into a .probe file string.
func generateProbeScript(events []recordedEvent) string {
	var sb strings.Builder

	// Header comment
	sb.WriteString(fmt.Sprintf("# Recorded on %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("test \"recorded flow\"\n")
	sb.WriteString("  open the app\n")

	for i, e := range events {
		// Insert wait step if >2s gap between actions
		if i > 0 {
			gap := e.At.Sub(events[i-1].At)
			if gap > 2*time.Second {
				secs := int(gap.Seconds())
				sb.WriteString(fmt.Sprintf("  wait %d seconds\n", secs))
			}
		}

		params := e.Params
		switch params["action"] {
		case "tap":
			sel := selectorFromParams(params)
			sb.WriteString(fmt.Sprintf("  tap %s\n", sel))
		case "type":
			sel := selectorFromParams(params)
			if obscuredEvent(params) {
				// The value of an obscured field is never written, so no type step is made for it. The comment names the
				// lookup and tells the person what to add by hand.
				sb.WriteString(fmt.Sprintf("  # type into %s was not recorded: the value of an obscured field is not saved. Add type \"${VAR}\" into %s by hand.\n", sel, sel))
				break
			}
			text, _ := params["text"].(string)
			if sel != "the element" {
				sb.WriteString(fmt.Sprintf("  type %q into the %s field\n", text, sel))
			} else {
				sb.WriteString(fmt.Sprintf("  type %q\n", text))
			}
		case "swipe":
			dir, _ := params["direction"].(string)
			sb.WriteString(fmt.Sprintf("  swipe %s\n", dir))
		case "scroll":
			dir, _ := params["direction"].(string)
			sb.WriteString(fmt.Sprintf("  scroll %s\n", dir))
		case "long_press":
			sel := selectorFromParams(params)
			sb.WriteString(fmt.Sprintf("  long press on %s\n", sel))
		case "see":
			sel := selectorFromParams(params)
			sb.WriteString(fmt.Sprintf("  see %s\n", sel))
		case "navigate":
			sb.WriteString("  go back\n")
		default:
			sb.WriteString(fmt.Sprintf("  # unknown event: %s\n", params["action"]))
		}
	}

	return sb.String()
}

// obscuredEvent reports whether a recorded type event came from an obscured field. The agent sends no value for such a
// field, so the event has no text that a recording may keep.
func obscuredEvent(params map[string]interface{}) bool {
	obscured, _ := params["obscured"].(bool)
	return obscured
}

// sanitizeText removes newlines and excess whitespace from text that will
// appear in generated .probe files (which use indentation-based syntax).
func sanitizeText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}

// selectorFromParams extracts the best selector string from event params.
// Handles both flat params (text/id) and nested selector maps from the agent.
func selectorFromParams(params map[string]interface{}) string {
	// Check for nested selector map from agent
	if sel, ok := params["selector"].(map[string]interface{}); ok {
		kind, _ := sel["kind"].(string)
		text := sanitizeText(sel["text"].(string))
		switch kind {
		case "id":
			if strings.HasPrefix(text, "#") {
				return text
			}
			return "#" + text
		case "text":
			if text != "" {
				return fmt.Sprintf("%q", text)
			}
		case "type":
			if text != "" {
				return fmt.Sprintf("the %s", text)
			}
		}
	}

	// Fallback: flat params
	if text, ok := params["text"].(string); ok && text != "" {
		return fmt.Sprintf("%q", sanitizeText(text))
	}
	if id, ok := params["id"].(string); ok && id != "" {
		return "#" + sanitizeText(id)
	}
	return "the element"
}

// waitForInterrupt returns a channel that closes on SIGINT/SIGTERM.
func waitForInterrupt() <-chan struct{} {
	ch := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		signal.Stop(sig)
		close(ch)
	}()
	return ch
}

type stepRecording struct{ recorder *record.Recorder }

var _ runner.StepRecorder = (*stepRecording)(nil)

func recordingEvent(ev runner.StepEvent) record.Event {
	result := record.Event{File: ev.File, TestName: ev.TestName, Line: ev.Line, Description: ev.Description, Status: string(ev.Status), Duration: ev.Duration, Attempt: ev.Attempt, Depth: ev.Depth, At: ev.At}
	if ev.Error != nil {
		result.Error = ev.Error.Error()
	}
	if ev.Target != nil {
		result.Target = &record.Target{Verb: ev.Target.Verb, Text: ev.Target.Text, Lookup: ev.Target.Lookup}
	}
	return result
}

func (r *stepRecording) BeginStep(client probelink.ProbeClient, ev runner.StepEvent) error {
	return r.recorder.BeginStep(client, recordingEvent(ev))
}
func (r *stepRecording) EndStep(client probelink.ProbeClient, ev runner.StepEvent) error {
	return r.recorder.EndStep(client, recordingEvent(ev))
}

type deviceRecording struct {
	dir      string
	recorder *record.Recorder
}
type runRecording struct {
	base, runID string
	mu          sync.Mutex
	recorders   []deviceRecording
}

func newRunRecording(base string) *runRecording {
	return &runRecording{base: base, runID: time.Now().UTC().Format("20060102-150405")}
}

// NewDevice starts a recorder for one device whose platform and OS version are not known here.
func (r *runRecording) NewDevice(device string) runner.StepRecorder {
	return r.NewDeviceInfo(device, "", "")
}

// NewDeviceInfo starts a recorder for one device. platform and osVersion come from the run's metadata, the same values
// the report carries, so the manifest and the report agree.
func (r *runRecording) NewDeviceInfo(device, platform, osVersion string) runner.StepRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	dir := filepath.Join(r.base, r.runID, deviceSlug(device))
	recorder := record.NewRecorder(record.Options{Dir: dir, RunID: r.runID, Device: device, Platform: platform, OSVersion: osVersion, Screenshots: "every_step", CLIVersion: Version})
	r.recorders = append(r.recorders, deviceRecording{dir, recorder})
	return &stepRecording{recorder: recorder}
}

// compositeRecorder is the recorder factory for a composite run. Each device's recorder takes the platform and OS version
// the composite device list carries, as a single-device run takes them from its run metadata.
func (r *runRecording) compositeRecorder() func(runner.CompositeDevice) runner.StepRecorder {
	return func(d runner.CompositeDevice) runner.StepRecorder {
		return r.NewDeviceInfo(d.DeviceName, string(d.Platform), d.OSVersion)
	}
}

// Finish returns written device directories and advisory warnings, never a run error.
func (r *runRecording) Finish() (dirs, warnings []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[string]bool)
	for _, entry := range r.recorders {
		// Close records its write errors in Warnings as well.
		_ = entry.recorder.Close()
		warnings = append(warnings, entry.recorder.Warnings()...)
		if info, err := os.Stat(entry.dir); err == nil && info.IsDir() && !seen[entry.dir] {
			dirs = append(dirs, entry.dir)
			seen[entry.dir] = true
		}
	}
	r.recorders = nil
	return dirs, warnings
}

func (r *runRecording) printFinish(w io.Writer) {
	dirs, warnings := r.Finish()
	for _, dir := range dirs {
		fmt.Fprintf(w, "Recording written to %s\n", dir)
	}
	for _, warning := range warnings {
		fmt.Fprintf(w, "Recording warning: %s\n", warning)
	}
	if len(dirs) > 0 {
		fmt.Fprintln(w, "Recordings can contain screens and request or response bodies. They stay on this machine; keep them out of version control.")
	}
}

var deviceSlugSeparators = regexp.MustCompile(`[^a-z0-9]+`)

func deviceSlug(device string) string {
	slug := strings.Trim(deviceSlugSeparators.ReplaceAllString(strings.ToLower(device), "-"), "-")
	if slug == "" {
		return "device"
	}
	return slug
}
