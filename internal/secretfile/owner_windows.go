//go:build windows

package secretfile

import "os"

// ownedByUs: the file mode does not say who owns a file on Windows, so the check is not made (permsEnforced is false there as well).
func ownedByUser(os.FileInfo) bool { return true }

// openNonblock: no pipe can be at a regular file name here.
const openNonblock = 0
