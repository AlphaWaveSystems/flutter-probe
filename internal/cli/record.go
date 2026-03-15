package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/flutterprobe/probe/internal/config"
	"github.com/flutterprobe/probe/internal/device"
	"github.com/flutterprobe/probe/internal/probelink"
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
	f.Duration("token-timeout", 0, "max time to wait for agent auth token (default: 30s)")
	rootCmd.AddCommand(recordCmd)
}

// recordedEvent captures a single action during recording.
type recordedEvent struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
	At     time.Time              `json:"at"`
}

func runRecord(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	outFile, _ := cmd.Flags().GetString("output")
	deviceSerial, _ := cmd.Flags().GetString("device")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	portFlag, _ := cmd.Flags().GetInt("port")
	tokenTimeout, _ := cmd.Flags().GetDuration("token-timeout")

	// Load config
	cfgDir, _ := os.Getwd()
	cfg, err := config.Load(cfgDir)
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
		if err := dm.ForwardPort(ctx, deviceSerial, agentPort, agentPort); err != nil {
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

	fmt.Printf("\n  \033[32m✓\033[0m  Recorded %d events → %s\n", len(events), outFile)
	return nil
}

// printRecordedEvent prints a real-time feedback line for a recorded event.
func printRecordedEvent(e recordedEvent) {
	params := e.Params
	action, _ := params["action"].(string)
	sel := selectorFromParams(params)

	switch action {
	case "tap":
		fmt.Printf("  \033[32m✓\033[0m  tap %s\n", sel)
	case "type":
		text, _ := params["text"].(string)
		if sel != "the element" {
			fmt.Printf("  \033[32m✓\033[0m  type %q into %s\n", text, sel)
		} else {
			fmt.Printf("  \033[32m✓\033[0m  type %q\n", text)
		}
	case "swipe":
		dir, _ := params["direction"].(string)
		fmt.Printf("  \033[32m✓\033[0m  swipe %s\n", dir)
	case "long_press":
		fmt.Printf("  \033[32m✓\033[0m  long press on %s\n", sel)
	default:
		fmt.Printf("  \033[32m✓\033[0m  %s\n", action)
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
			text, _ := params["text"].(string)
			sel := selectorFromParams(params)
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

// selectorFromParams extracts the best selector string from event params.
// Handles both flat params (text/id) and nested selector maps from the agent.
func selectorFromParams(params map[string]interface{}) string {
	// Check for nested selector map from agent
	if sel, ok := params["selector"].(map[string]interface{}); ok {
		kind, _ := sel["kind"].(string)
		text, _ := sel["text"].(string)
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
		return fmt.Sprintf("%q", text)
	}
	if id, ok := params["id"].(string); ok && id != "" {
		return "#" + id
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
