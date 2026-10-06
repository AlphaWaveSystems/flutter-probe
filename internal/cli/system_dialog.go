package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// `probe system-dialog` drives OS-level dialogs (permission alerts, the
// StoreKit sign-in sheet, ...) with no Flutter app or test file involved —
// handy for one-time provisioning such as signing a sandbox tester in on a
// fresh simulator. The same driver backs the ProbeScript system-dialog steps.

var systemDialogCmd = &cobra.Command{
	Use:     "system-dialog",
	Aliases: []string{"sysdialog"},
	Short:   "Drive OS system dialogs (permission alerts, sign-in sheets) on a simulator/emulator",
	Long: `Drive OS-level system dialogs that live outside the Flutter app: iOS permission
alerts and the StoreKit "Sign in to Apple Account" sheet, Android permission
dialogs. iOS simulators are driven through a small XCUITest runner
(` + "`probe ios-driver install`" + `); Android through uiautomator.

Secrets are never taken from the command line (they would show up in ps and
shell history): use --env NAME or --stdin with the type command.`,
}

func sysDialogFlags(c *cobra.Command) {
	c.Flags().String("device", "", "simulator UDID or Android serial (default: the only booted/connected device)")
	c.Flags().String("title", "", "only match a dialog whose text contains this")
}

// openSystemDriver resolves --device and returns a ready driver.
func openSystemDriver(cmd *cobra.Command) (sysdialog.Driver, error) {
	ctx := cmd.Context()
	dm := device.NewManager()
	id, _ := cmd.Flags().GetString("device")
	var platform device.Platform
	devices, _ := dm.List(ctx)
	if id == "" {
		var ready []device.Device
		for _, d := range devices {
			if d.State == "online" || d.State == "booted" || d.State == "device" {
				ready = append(ready, d)
			}
		}
		if len(ready) != 1 {
			return nil, fmt.Errorf("found %d booted/connected devices — pick one with --device (probe device list)", len(ready))
		}
		id, platform = ready[0].ID, ready[0].Platform
	} else {
		for _, d := range devices {
			if d.ID == id {
				platform = d.Platform
			}
		}
		if platform == "" {
			return nil, fmt.Errorf("device %q not found (probe device list)", id)
		}
	}
	return sysdialog.ForDevice(ctx, dm, id, platform, sysdialog.IOSOptions{
		Version:     Version,
		AutoInstall: true,
		Logf:        func(f string, a ...any) { statusInfo(os.Stderr, f, a...) },
	})
}

func withDriver(run func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		d, err := openSystemDriver(cmd)
		if err != nil {
			return err
		}
		defer d.Close()
		return run(cmd.Context(), d, cmd, args)
	}
}

var sysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the system dialogs currently showing (title, buttons, fields)",
	Args:  cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		ds, err := d.Dialogs(ctx)
		if err != nil {
			return err
		}
		if len(ds) == 0 {
			fmt.Println("  No system dialog is showing.")
			return nil
		}
		for _, x := range ds {
			fmt.Printf("  • %s\n", x.Title)
			if len(x.Buttons) > 0 {
				fmt.Printf("      buttons: %s\n", strings.Join(x.Buttons, " | "))
			}
			if len(x.Fields) > 0 {
				fmt.Printf("      fields:  %s\n", strings.Join(x.Fields, " | "))
			}
		}
		return nil
	}),
}

var sysSeeCmd = &cobra.Command{
	Use:   "see",
	Short: "Exit 0 if a (matching) system dialog is showing, 1 if not",
	Args:  cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		title, _ := cmd.Flags().GetString("title")
		ok, err := d.See(ctx, title)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no system dialog showing")
		}
		fmt.Println("  system dialog is showing")
		return nil
	}),
}

var sysWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for a system dialog to appear (or --gone)",
	Args:  cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		title, _ := cmd.Flags().GetString("title")
		gone, _ := cmd.Flags().GetBool("gone")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		ok, err := d.Wait(ctx, title, !gone, timeout)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("timed out after %s waiting for the dialog", timeout)
		}
		return nil
	}),
}

var sysTapCmd = &cobra.Command{
	Use:   "tap <button>",
	Short: `Tap a button in the system dialog, e.g. probe system-dialog tap "Allow"`,
	Args:  cobra.ExactArgs(1),
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		tapped, err := d.Tap(ctx, args[0], title)
		if err != nil {
			return err
		}
		statusOK(os.Stdout, "tapped %q", tapped)
		return nil
	}),
}

var sysTypeCmd = &cobra.Command{
	Use:   "type",
	Short: "Type a secret into a system field (value from --env NAME or --stdin; never argv)",
	Example: `  PROBE_SANDBOX_PASSWORD=... probe system-dialog type --field Password --env PROBE_SANDBOX_PASSWORD
  pass show sandbox | probe system-dialog type --field Password --stdin`,
	Args: cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		field, _ := cmd.Flags().GetString("field")
		envName, _ := cmd.Flags().GetString("env")
		useStdin, _ := cmd.Flags().GetBool("stdin")
		title, _ := cmd.Flags().GetString("title")
		var value string
		switch {
		case envName != "":
			var err error
			if value, _, err = sysdialog.ResolveSecret("$" + envName); err != nil {
				return err
			}
		case useStdin:
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return fmt.Errorf("reading the value from stdin: %w", err)
			}
			value = strings.TrimRight(line, "\r\n")
		default:
			return fmt.Errorf("give the value with --env NAME or --stdin (it is never accepted on the command line)")
		}
		if field == "" {
			return fmt.Errorf("--field is required")
		}
		if err := d.Type(ctx, field, value, title); err != nil {
			return err
		}
		statusOK(os.Stdout, "typed into %q (value hidden)", field)
		return nil
	}),
}

var sysDismissCmd = &cobra.Command{
	Use:   "dismiss",
	Short: "Dismiss the system dialog via Cancel / Don't Allow / Not Now (no-op if none)",
	Args:  cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		title, _ := cmd.Flags().GetString("title")
		did, err := d.Dismiss(ctx, title)
		if err != nil {
			return err
		}
		if did {
			statusOK(os.Stdout, "dialog dismissed")
		} else {
			fmt.Println("  No system dialog was showing.")
		}
		return nil
	}),
}

var sysSandboxCmd = &cobra.Command{
	Use:   "sign-in-sandbox",
	Short: "Sign a StoreKit sandbox tester in through the system sheet (idempotent)",
	Long: `Waits briefly for the "Sign in to Apple Account" sheet, types the tester from
PROBE_SANDBOX_USER / PROBE_SANDBOX_PASSWORD, confirms, and waits for the sheet
to close. If no sheet appears it does nothing and exits 0, so it is safe to run
before every test run. The account lives in the simulator's data and survives
app reinstalls.`,
	Args: cobra.NoArgs,
	RunE: withDriver(func(ctx context.Context, d sysdialog.Driver, cmd *cobra.Command, _ []string) error {
		userEnv, _ := cmd.Flags().GetString("user-env")
		passEnv, _ := cmd.Flags().GetString("password-env")
		wait, _ := cmd.Flags().GetDuration("wait")
		user, _, err := sysdialog.ResolveSecret("$" + userEnv)
		if err != nil {
			return err
		}
		pass, _, err := sysdialog.ResolveSecret("$" + passEnv)
		if err != nil {
			return err
		}
		done, err := sysdialog.SignInSandbox(ctx, d, sysdialog.SandboxOptions{User: user, Password: pass, WaitForSheet: wait})
		if err != nil {
			return err
		}
		if done {
			statusOK(os.Stdout, "signed in the sandbox tester")
		} else {
			fmt.Println("  No sign-in sheet appeared — nothing to do.")
		}
		return nil
	}),
}

var iosDriverCmd = &cobra.Command{
	Use:   "ios-driver",
	Short: "Manage the iOS system-dialog driver (XCUITest runner)",
}

var iosDriverInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download the iOS driver for this probe version from the GitHub release",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		url, _ := cmd.Flags().GetString("url")
		var dir string
		var err error
		if url != "" {
			dir, err = sysdialog.InstallIOSDriverFrom(cmd.Context(), url, sysdialog.IOSDriverRoot()+"/"+Version)
		} else {
			dir, err = sysdialog.InstallIOSDriver(cmd.Context(), Version)
		}
		if err != nil {
			return err
		}
		statusOK(os.Stdout, "iOS driver installed at %s", dir)
		return nil
	},
}

var iosDriverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the iOS driver is installed and whether a runner is up for --device",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir, err := sysdialog.FindIOSDriverDir(Version)
		if err != nil {
			fmt.Printf("  installed: no (%v)\n", err)
		} else {
			fmt.Printf("  installed: yes (%s)\n", dir)
		}
		if id, _ := cmd.Flags().GetString("device"); id != "" {
			port := sysdialog.PortFor(id)
			fmt.Printf("  runner for %s: port %d, running: %v\n", id, port, sysdialog.Healthy(cmd.Context(), port))
		}
		return nil
	},
}

var iosDriverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running iOS driver for --device",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		id, _ := cmd.Flags().GetString("device")
		if id == "" {
			return fmt.Errorf("--device is required")
		}
		port := sysdialog.PortFor(id)
		if !sysdialog.Healthy(cmd.Context(), port) {
			fmt.Println("  No runner is running for that device.")
			return nil
		}
		return sysdialog.Shutdown(cmd.Context(), port)
	},
}

func init() {
	for _, c := range []*cobra.Command{sysListCmd, sysSeeCmd, sysWaitCmd, sysTapCmd, sysTypeCmd, sysDismissCmd, sysSandboxCmd} {
		sysDialogFlags(c)
		systemDialogCmd.AddCommand(c)
	}
	sysWaitCmd.Flags().Bool("gone", false, "wait for the dialog to disappear instead")
	sysWaitCmd.Flags().Duration("timeout", 15*time.Second, "how long to wait")
	sysTypeCmd.Flags().String("field", "", "field label (e.g. Password)")
	sysTypeCmd.Flags().String("env", "", "name of the environment variable holding the value")
	sysTypeCmd.Flags().Bool("stdin", false, "read the value from the first line of stdin")
	sysSandboxCmd.Flags().String("user-env", "PROBE_SANDBOX_USER", "env var with the sandbox tester's Apple Account")
	sysSandboxCmd.Flags().String("password-env", "PROBE_SANDBOX_PASSWORD", "env var with the sandbox tester's password")
	sysSandboxCmd.Flags().Duration("wait", 8*time.Second, "how long to wait for the sheet before concluding there is none")
	rootCmd.AddCommand(systemDialogCmd)

	iosDriverInstallCmd.Flags().String("url", "", "install from this zip URL instead of the GitHub release")
	iosDriverStatusCmd.Flags().String("device", "", "simulator UDID")
	iosDriverStopCmd.Flags().String("device", "", "simulator UDID")
	iosDriverCmd.AddCommand(iosDriverInstallCmd, iosDriverStatusCmd, iosDriverStopCmd)
	rootCmd.AddCommand(iosDriverCmd)
}
