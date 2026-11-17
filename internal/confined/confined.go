// Package confined reads a file whose path the program does not control (an agent named it) without the check-then-use gap of resolving
// the path first and reading it later: the file is opened once, the opened file itself is checked, and the bytes are read from it.
package confined

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxSize is the largest file ReadFile reads.
const maxSize = 64 << 20

// ErrRefused is returned when the opened file is not one the caller allows.
var ErrRefused = errors.New("the file is not one that may be read")

// ReadFile opens p once (the last part of the path must not be a link, where the system can say so), checks that the opened file is a
// regular file with a single name (a hard link could give another file an allowed name) and that its real path, asked of the open file,
// is accepted by allowed, and then reads it from the same open file. It returns the bytes and that real path. Whatever happens to p
// meanwhile (a link swapped in, a folder renamed), what is read is what was checked.
func ReadFile(p string, allowed func(real string) bool) ([]byte, string, error) {
	f, err := openNoFollow(p)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrRefused, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("%w: not a regular file", ErrRefused)
	}
	if links(f, info) != 1 {
		return nil, "", fmt.Errorf("%w: the file has more than one name", ErrRefused)
	}
	real, err := openedPath(f)
	if err != nil {
		return nil, "", fmt.Errorf("%w: its real path cannot be read: %v", ErrRefused, err)
	}
	real = filepath.Clean(real)
	if !allowed(real) {
		return nil, "", ErrRefused
	}
	if info.Size() > maxSize {
		return nil, "", fmt.Errorf("%w: larger than %d bytes", ErrRefused, maxSize)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxSize {
		return nil, "", fmt.Errorf("%w: larger than %d bytes", ErrRefused, maxSize)
	}
	return data, real, nil
}

// Under reports whether p is dir or inside it. Both are expected clean and absolute.
func Under(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !hasDotDotPrefix(rel)
}

func hasDotDotPrefix(rel string) bool {
	return len(rel) >= 3 && rel[:2] == ".." && os.IsPathSeparator(rel[2])
}
