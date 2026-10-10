package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
	"github.com/alphawavesystems/flutter-probe/internal/license"
	"github.com/spf13/cobra"
)

const licenseFile = "license.key"

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Manage the FlutterProbe license key (paid features only)",
	Long: `Manage the license key stored in ~/.flutterprobe/license.key.

A key is only needed for hosted features. Everything that runs locally works
without one.`,
}

var licenseActivateCmd = &cobra.Command{
	Use:     "activate <key>",
	Short:   "Verify and store a license key",
	Example: `  probe license activate FP1.eyJ...`,
	Args:    cobra.ExactArgs(1),
	RunE:    runLicenseActivate,
}

var licenseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the stored license and its entitlements",
	Args:  cobra.NoArgs,
	RunE:  runLicenseStatus,
}

func init() {
	licenseCmd.AddCommand(licenseActivateCmd)
	licenseCmd.AddCommand(licenseStatusCmd)
	rootCmd.AddCommand(licenseCmd)
}

func runLicenseActivate(cmd *cobra.Command, args []string) error {
	key := strings.TrimSpace(args[0])
	claims, err := license.Verify(key)
	if err != nil {
		return fmt.Errorf("license not activated: %w", err)
	}
	dir, err := cloud.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, licenseFile), []byte(key+"\n"), 0600); err != nil {
		return fmt.Errorf("saving license: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "License activated for %s: %s\n", claims.Customer, describeClaims(claims))
	return nil
}

func runLicenseStatus(cmd *cobra.Command, args []string) error {
	claims, err := LoadLicense()
	switch {
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintln(cmd.OutOrStdout(), "No license. All local features are available; paid hosted features are off.")
		return nil
	case errors.Is(err, license.ErrExpired):
		fmt.Fprintf(cmd.OutOrStdout(), "License for %s expired on %s. Local features keep working.\n",
			claims.Customer, claims.Expires.Format("2006-01-02"))
		return nil
	case err != nil:
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Licensed to %s: %s\n", claims.Customer, describeClaims(claims))
	return nil
}

// LoadLicense reads and verifies the stored key. os.ErrNotExist when none.
func LoadLicense() (*license.Claims, error) {
	dir, err := cloud.ConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, licenseFile))
	if err != nil {
		return nil, err
	}
	return license.Verify(string(data))
}

func describeClaims(c *license.Claims) string {
	plans := make([]string, len(c.Plans))
	for i, p := range c.Plans {
		plans[i] = string(p)
	}
	s := strings.Join(plans, ", ")
	if c.Seats > 0 {
		s += fmt.Sprintf(", %d seats", c.Seats)
	}
	if !c.Expires.IsZero() {
		s += fmt.Sprintf(", valid until %s", c.Expires.UTC().Format("2006-01-02"))
	}
	if c.Expires.IsZero() {
		s += ", no expiry"
	}
	return s
}
