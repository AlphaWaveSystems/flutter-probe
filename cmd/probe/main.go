package main

import (
	"os"

	"github.com/flutterprobe/probe/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
