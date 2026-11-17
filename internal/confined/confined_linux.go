//go:build linux

package confined

import (
	"os"
	"strconv"
)

// openedPath asks the system for the path of the open file f.
func openedPath(f *os.File) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd())))
}
