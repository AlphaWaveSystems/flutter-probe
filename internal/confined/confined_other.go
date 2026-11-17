//go:build !darwin && !linux && !windows

package confined

import (
	"errors"
	"io/fs"
	"os"
)

// On other systems the path of an open file cannot be asked for: nothing is read.
func openNoFollow(p string) (*os.File, error) { return nil, errors.New("not supported on this system") }

func links(*os.File, fs.FileInfo) uint64 { return 0 }

func openedPath(*os.File) (string, error) { return "", errors.New("not supported on this system") }
