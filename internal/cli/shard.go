package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// shardBase is the directory a file's shard key is taken relative to. --shard-root pins it, and PROBE_SHARD_ROOT does when the flag is not
// given: a checkout and a tarball of the same tree then agree when the config is in a subfolder, since the key follows the directory given.
// With neither, it is the directory of the --config file, or the working directory (the empty string) when there is no --config.
func shardBase(cmd *cobra.Command) (string, error) {
	root, _ := cmd.Flags().GetString("shard-root")
	source := "--shard-root"
	if root == "" {
		root = os.Getenv("PROBE_SHARD_ROOT")
		source = "PROBE_SHARD_ROOT"
	}
	if root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("%s %q: %w", source, root, err)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("%s %q is not a directory", source, root)
		}
		return abs, nil
	}
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		return "", nil
	}
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return "", nil
	}
	return filepath.Dir(abs), nil
}
