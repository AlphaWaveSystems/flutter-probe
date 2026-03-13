package cli

import (
	"context"
	"fmt"

	"github.com/flutterprobe/probe/internal/device"
	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Manage connected emulators and simulators",
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connected Android emulators and iOS simulators",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		dm := device.NewManager()
		devices, err := dm.List(ctx)
		if err != nil {
			return fmt.Errorf("listing devices: %w", err)
		}
		if len(devices) == 0 {
			fmt.Println("  No devices connected.")
			fmt.Println("  Run 'probe device start --platform android' to launch an emulator.")
			return nil
		}
		fmt.Printf("  %-22s %-12s %s\n", "SERIAL", "STATE", "NAME")
		fmt.Printf("  %-22s %-12s %s\n", "------", "-----", "----")
		for _, d := range devices {
			fmt.Printf("  %-22s %-12s %s\n", d.ID, d.State, d.Name)
		}
		return nil
	},
}

var deviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start an Android emulator or iOS simulator",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		platform, _ := cmd.Flags().GetString("platform")
		avd, _ := cmd.Flags().GetString("avd")

		dm := device.NewManager()

		switch platform {
		case "android":
			if avd == "" {
				// List available AVDs and pick first
				adb := device.NewADB()
				avds, err := adb.ListAVDs(ctx)
				if err != nil || len(avds) == 0 {
					return fmt.Errorf("no AVDs found — create one with Android Studio")
				}
				avd = avds[0]
			}
			fmt.Printf("  Starting emulator %q...\n", avd)
			d, err := dm.Start(ctx, avd)
			if err != nil {
				return err
			}
			fmt.Printf("  \033[32m✓\033[0m  Emulator %s (%s) is online\n", d.Name, d.ID)
		case "ios":
			return fmt.Errorf("iOS simulator support is in Phase 2 (Q3 2026)")
		default:
			return fmt.Errorf("unknown platform %q — use android or ios", platform)
		}
		return nil
	},
}

func init() {
	deviceStartCmd.Flags().StringP("platform", "p", "android", "platform: android | ios")
	deviceStartCmd.Flags().String("avd", "", "AVD name to start (default: first available)")
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceStartCmd)
}
