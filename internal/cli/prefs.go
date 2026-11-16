package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

var prefsCmd = &cobra.Command{
	Use:   "prefs",
	Short: "Show or change your own FlutterProbe settings on this computer",
	Long: `Settings that belong to you, kept in your user configuration folder and shared by probe and FlutterProbe Studio. They are never read from a
project: a probe.yaml cannot change them.

  clear-data-without-asking   off (default) | on
      Run a "clear app data" step on a simulator or an emulator without asking. A physical phone, and any device that cannot be told to be a
      simulator or emulator, still asks (or needs --yes).`,
}

var prefsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show your settings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := userprefs.Load()
		path, _ := userprefs.Path()
		fmt.Fprintf(cmd.OutOrStdout(), "clear-data-without-asking: %s\nfile: %s\n", onOff(p.ClearDataWithoutAsking), path)
		return nil
	},
}

var prefsSetCmd = &cobra.Command{
	Use:   "set clear-data-without-asking on|off",
	Short: "Change a setting",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "clear-data-without-asking" {
			return UsageError{fmt.Errorf("unknown setting %q: the setting is clear-data-without-asking", args[0])}
		}
		var on bool
		switch strings.ToLower(args[1]) {
		case "on", "true", "yes":
			on = true
		case "off", "false", "no":
		default:
			return UsageError{fmt.Errorf("the value must be on or off, not %q", args[1])}
		}
		if err := userprefs.SetClearDataWithoutAsking(on); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "clear-data-without-asking: %s\n", onOff(on))
		return nil
	},
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func init() {
	prefsCmd.AddCommand(prefsShowCmd, prefsSetCmd)
}
