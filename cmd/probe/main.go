package main

import (
	"os"

	"github.com/alphawavesystems/flutter-probe/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
