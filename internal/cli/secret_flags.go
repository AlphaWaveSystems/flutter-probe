package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// secretFlagEnv names the environment variable each secret flag of `probe test` falls back to. A secret on the command line can be read by
// other users in the process list and stays in shell history; a secret in the environment of the process does neither. The VS Code
// extension passes them this way. The relay token's variable is not PROBE_RELAY_TOKEN: that name is the agent's dart-define, which holds
// the relay session's agent token, a different token from the CLI's (cloud.RelaySession has both). A CI job that exports it for the app
// build would otherwise hand the CLI the wrong token.
var secretFlagEnv = map[string]string{
	"cloud-key":    "PROBE_CLOUD_KEY",
	"cloud-secret": "PROBE_CLOUD_SECRET",
	"relay-token":  "PROBE_RELAY_CLI_TOKEN",
}

// secretFlag returns the value of a secret flag: the flag when it was given (even empty), otherwise its environment variable.
func secretFlag(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	if cmd.Flags().Changed(name) {
		return v
	}
	if env, ok := secretFlagEnv[name]; ok {
		return os.Getenv(env)
	}
	return v
}
