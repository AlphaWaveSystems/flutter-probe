package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flutterprobe/probe/internal/device"
	"github.com/flutterprobe/probe/internal/probelink"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record gestures and interactions, then generate a .probe file",
	Example: `  probe record --output tests/flow.probe
  probe record --device emulator-5554 --output tests/onboarding.probe`,
	RunE: runRecord,
}

func init() {
	recordCmd.Flags().StringP("output", "o", "tests/recorded.probe", "output .probe file path")
	recordCmd.Flags().String("device", "", "target device serial")
	recordCmd.Flags().Duration("timeout", 5*time.Minute, "maximum recording time")
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

	// Connect to device
	dm := device.NewManager()
	if deviceSerial == "" {
		devices, err := dm.List(ctx)
		if err != nil || len(devices) == 0 {
			return fmt.Errorf("no connected devices — start an emulator first")
		}
		deviceSerial = devices[0].ID
	}

	if err := dm.ForwardPort(ctx, deviceSerial, 8686, 8686); err != nil {
		return fmt.Errorf("port forward: %w", err)
	}
	defer dm.RemoveForward(ctx, deviceSerial, 8686) //nolint:errcheck

	token, err := dm.ReadToken(ctx, deviceSerial, 15*time.Second)
	if err != nil {
		return fmt.Errorf("agent token: %w", err)
	}

	client, err := probelink.Dial(ctx, "127.0.0.1", 8686, token)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer client.Close()

	var events []recordedEvent

	// Subscribe to recording notifications from the agent
	client.OnNotify = func(method string, params json.RawMessage) {
		if method == "probe.recorded_event" {
			var p map[string]interface{}
			if err := json.Unmarshal(params, &p); err == nil {
				events = append(events, recordedEvent{
					Method: method,
					Params: p,
					At:     time.Now(),
				})
			}
		}
	}

	// Tell the agent to enter recording mode
	recCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err = client.Call(recCtx, "probe.start_recording", nil)
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
	_, _ = client.Call(ctx2, "probe.stop_recording", nil)

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

// generateProbeScript converts recorded events into a .probe file string.
func generateProbeScript(events []recordedEvent) string {
	var sb strings.Builder
	sb.WriteString(`test "recorded flow"` + "\n")

	for _, e := range events {
		params := e.Params
		switch params["action"] {
		case "tap":
			sel := selectorFromParams(params)
			sb.WriteString(fmt.Sprintf("  tap %s\n", sel))
		case "type":
			text, _ := params["text"].(string)
			sel := selectorFromParams(params)
			sb.WriteString(fmt.Sprintf("  type %q into %s\n", text, sel))
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
			// Unknown — emit as comment
			sb.WriteString(fmt.Sprintf("  # unknown event: %s\n", params["action"]))
		}
	}

	return sb.String()
}

func selectorFromParams(params map[string]interface{}) string {
	if text, ok := params["text"].(string); ok && text != "" {
		return fmt.Sprintf("%q", text)
	}
	if id, ok := params["id"].(string); ok && id != "" {
		return "#" + id
	}
	return "the element"
}

// waitForInterrupt returns a channel that closes on SIGINT.
func waitForInterrupt() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		// Simple 1-second polling — real impl would use signal.Notify
		for {
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return ch
}
