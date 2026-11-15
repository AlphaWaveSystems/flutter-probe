//go:build windows

package mcp

import "os"

// openNoBlock opens a picture file for reading; Windows has no named-pipe swap for a path in this folder to guard against.
func openNoBlock(path string) (*os.File, error) { return os.Open(path) }
