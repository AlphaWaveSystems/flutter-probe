//go:build !windows

package mcp

import (
	"os"
	"syscall"
)

// openNoBlock opens a picture file for reading without waiting: a named pipe swapped in for the file after it was checked would otherwise block the open for ever. The caller checks the open handle (a regular file).
func openNoBlock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
