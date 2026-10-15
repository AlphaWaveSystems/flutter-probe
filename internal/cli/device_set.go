package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/devsettings"
	"github.com/spf13/cobra"
)

var deviceSetCmd = &cobra.Command{
	Use:   "set <setting> <value>",
	Short: "Change an OS setting of one named emulator or simulator",
	Long: `Change an OS-level setting of one specific device. --device is required (a serial, a UDID or a unique device name); "booted" is not accepted, so a change never lands on whichever device is running.

Settings:
  appearance <light|dark|auto>       dark mode (iOS: light or dark)
  font_scale <value>                 Android: a factor such as 1.3; iOS: a content size category such as accessibility-large
  locale <tag>                       app language such as de or pt-BR; needs --app (Android 13 or later; on iOS the app restarts)
  orientation <name>                 Android: portrait, landscape, reverse-portrait, reverse-landscape (turns auto-rotate off)
  open_url <link>                    open a deep link (with --app on Android, restricted to that app)

iOS physical phones are not supported: these settings use simctl.`,
	Example: `  probe device set appearance dark --device emulator-5554
  probe device set font_scale accessibility-large --device "StudioE2EWeb-iPhone17"
  probe device set locale de-DE --device emulator-5554 --app com.example.app
  probe device set open_url "myapp://item/1" --device emulator-5554`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("device")
		app, _ := cmd.Flags().GetString("app")
		ctx := context.Background()
		d, err := resolveNamedDevice(ctx, id)
		if err != nil {
			return err
		}
		req := devsettings.Request{Platform: devsettings.Platform(d.Platform), Device: d.ID, Setting: devsettings.Setting(args[0]), Value: args[1], App: app}
		if err := devsettings.Apply(ctx, nil, req); err != nil {
			return err
		}
		statusOK(os.Stdout, "%s set to %s on %s (%s)", args[0], args[1], d.Name, d.ID)
		return nil
	},
}

// resolveNamedDevice finds one device by exact ID or exact name. An empty value, "booted" and an ambiguous name are errors.
func resolveNamedDevice(ctx context.Context, id string) (device.Device, error) {
	if id == "" || strings.EqualFold(id, "booted") {
		return device.Device{}, fmt.Errorf("--device is required: name one specific emulator or simulator (see 'probe device list')")
	}
	devices, err := device.NewManager().List(ctx)
	if err != nil {
		return device.Device{}, fmt.Errorf("listing devices: %w", err)
	}
	return pickDevice(devices, id)
}

func pickDevice(devices []device.Device, id string) (device.Device, error) {
	var byName []device.Device
	for _, d := range devices {
		if d.ID == id {
			return checkSettable(d)
		}
		if d.Name == id {
			byName = append(byName, d)
		}
	}
	switch len(byName) {
	case 1:
		return checkSettable(byName[0])
	case 0:
		return device.Device{}, fmt.Errorf("no device %q (see 'probe device list')", id)
	}
	return device.Device{}, fmt.Errorf("%d devices are named %q: use the serial or UDID", len(byName), id)
}

func checkSettable(d device.Device) (device.Device, error) {
	// "online" is the manager's word; adb reports "device" and simctl "booted".
	if d.State != "online" && d.State != "booted" && d.State != "device" {
		return device.Device{}, fmt.Errorf("%s (%s) is %s: start it first", d.Name, d.ID, d.State)
	}
	if d.Platform == device.PlatformIOS && d.Transport != "" {
		return device.Device{}, fmt.Errorf("%s is a physical iOS device: these settings use simctl and work on simulators only", d.Name)
	}
	return d, nil
}

func init() {
	deviceSetCmd.Flags().String("device", "", "serial, UDID or unique name of the device (required)")
	deviceSetCmd.Flags().String("app", "", "application id (Android) or bundle id (iOS), for locale and open_url")
	deviceCmd.AddCommand(deviceSetCmd)
}
