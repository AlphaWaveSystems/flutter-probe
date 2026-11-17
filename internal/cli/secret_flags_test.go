package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func secretFlagCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for name := range secretFlagEnv {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().String("cloud-device", "", "")
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// Each secret flag reads its environment variable when the flag is not given, so the VS Code extension (or a CI job) can keep the secret
// off the command line.
func TestSecretFlagsFallBackToTheirEnvironmentVariable(t *testing.T) {
	want := map[string]string{
		"cloud-key":    "PROBE_CLOUD_KEY",
		"cloud-secret": "PROBE_CLOUD_SECRET",
		"relay-token":  "PROBE_RELAY_CLI_TOKEN",
	}
	if len(secretFlagEnv) != len(want) {
		t.Fatalf("secret flags: want %v, got %v", want, secretFlagEnv)
	}
	for flag, env := range want {
		if secretFlagEnv[flag] != env {
			t.Errorf("--%s must fall back to %s, falls back to %q", flag, env, secretFlagEnv[flag])
		}
		t.Setenv(env, "from-env-"+flag)
		if got := secretFlag(secretFlagCmd(t), flag); got != "from-env-"+flag {
			t.Errorf("--%s not given: want the value of %s, got %q", flag, env, got)
		}
	}
}

// A flag that was given wins over the environment, even when it was given empty: the command line is the explicit choice.
func TestSecretFlagGivenWinsOverTheEnvironment(t *testing.T) {
	t.Setenv("PROBE_CLOUD_SECRET", "from-env")
	if got := secretFlag(secretFlagCmd(t, "--cloud-secret", "from-flag"), "cloud-secret"); got != "from-flag" {
		t.Errorf("want the flag's value, got %q", got)
	}
	if got := secretFlag(secretFlagCmd(t, "--cloud-secret", ""), "cloud-secret"); got != "" {
		t.Errorf("a flag given empty must stay empty, got %q", got)
	}
}

// A flag that is not a secret flag never reads the environment.
func TestOtherFlagsDoNotReadTheEnvironment(t *testing.T) {
	t.Setenv("PROBE_CLOUD_DEVICE", "from-env")
	if got := secretFlag(secretFlagCmd(t), "cloud-device"); got != "" {
		t.Errorf("--cloud-device has no environment variable, got %q", got)
	}
}

// probe test has the three flags and reads each one only through secretFlag: a plain GetString would skip the environment.
func TestProbeTestReadsSecretFlagsThroughTheEnvironmentFallback(t *testing.T) {
	src, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	for flag := range secretFlagEnv {
		if testCmd.Flags().Lookup(flag) == nil {
			t.Errorf("probe test has no --%s", flag)
		}
		if strings.Contains(string(src), `GetString("`+flag+`")`) {
			t.Errorf("test.go reads --%s with GetString; read it with secretFlag so its environment variable works", flag)
		}
		if !strings.Contains(string(src), `secretFlag(cmd, "`+flag+`")`) {
			t.Errorf("test.go does not read --%s through secretFlag", flag)
		}
	}
}
