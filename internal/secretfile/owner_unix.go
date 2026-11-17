//go:build !windows

package secretfile

import (
	"os"
	"syscall"
)

// openNonblock makes an open of a file that turns out to be a pipe return at once instead of waiting for a writer.
const openNonblock = syscall.O_NONBLOCK

// ownedByUs says whether the file or folder belongs to the user running this process. Where the owner cannot be read it is taken as ours.
func ownedByUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return !ok || int(st.Uid) == os.Getuid()
}
