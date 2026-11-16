package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"
)

// shardBase is the directory a file's shard is taken relative to: the directory of the --config file, or the working directory (the empty
// string) when there is no --config. Shards depend on the file's path relative to it, so a run from another directory, or with the
// checkout at another path, puts each file in the same shard.
func shardBase(cmd *cobra.Command) string {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		return ""
	}
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return ""
	}
	return filepath.Dir(abs)
}
