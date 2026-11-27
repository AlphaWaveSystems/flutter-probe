package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/updatecheck"
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

var prefsCmd = &cobra.Command{
	Use:   "prefs",
	Short: "Show or change your own FlutterProbe settings on this computer",
	Long: `Settings that belong to you, kept in your user configuration folder and shared by probe and FlutterProbe Studio. They are never read from a
project: a probe.yaml cannot change them.

  clear-data-without-asking   off (default) | on
      Run a "clear app data" step on a simulator or an emulator without asking. A physical phone, and any device that cannot be told to be a
      simulator or emulator, still asks (or needs --yes).

  update-check   on (default) | off
      Check GitHub for a newer stable release on each run, and print one line on stderr when there is one. A failed check is silent.
      PROBE_NO_UPDATE_CHECK=1 turns the check off for one run and any run from that shell.`,
}

var prefsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show your settings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := userprefs.Load()
		path, _ := userprefs.Path()
		fmt.Fprintf(cmd.OutOrStdout(), "clear-data-without-asking: %s\nupdate-check: %s\nfile: %s\n",
			onOff(p.ClearDataWithoutAsking), updateCheckState(), path)
		return nil
	},
}

var prefsSetCmd = &cobra.Command{
	Use:   "set clear-data-without-asking|update-check on|off",
	Short: "Change a setting",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var on bool
		switch strings.ToLower(args[1]) {
		case "on", "true", "yes":
			on = true
		case "off", "false", "no":
		default:
			return UsageError{fmt.Errorf("the value must be on or off, not %q", args[1])}
		}
		switch args[0] {
		case "clear-data-without-asking":
			if err := userprefs.SetClearDataWithoutAsking(on); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "clear-data-without-asking: %s\n", onOff(on))
		case "update-check":
			if err := updatecheck.SetEnabled(on); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "update-check: %s\n", onOff(on))
		default:
			return UsageError{fmt.Errorf("unknown setting %q: the settings are clear-data-without-asking and update-check", args[0])}
		}
		return nil
	},
}

// updateCheckState is the update check as the person would see it: off when the environment variable or the stored choice says so.
func updateCheckState() string {
	if updatecheck.Enabled() && !environmentSwitchesUpdateCheckOff() {
		return "on"
	}
	return "off"
}

// environmentSwitchesUpdateCheckOff reports whether PROBE_NO_UPDATE_CHECK is set to something other than 0 or false.
func environmentSwitchesUpdateCheckOff() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PROBE_NO_UPDATE_CHECK")))
	return v != "" && v != "0" && v != "false"
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
