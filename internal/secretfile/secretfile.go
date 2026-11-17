// Package secretfile writes a file that holds a secret (a bearer token, a discovery file with one) for another local process to read, and removes it
// again. It is the one writer for every such file, so each gets the same refusals: a folder or file that is a link, a file that is not a regular
// file, one that other users can reach or that another user owns is not repaired and not used: a location someone else set up is not one to trust.
// The secret is written to a new 0600 file that is then renamed into place, so the write never follows a link at the final name.
package secretfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// permsEnforced is false where the file mode does not describe who can read a file.
var permsEnforced = runtime.GOOS != "windows"

// renameFile moves the finished temporary file into place; a variable so a test can make it fail.
var renameFile = os.Rename

// ownedByUs says whether a file or folder belongs to this user; a variable so a test can say it does not.
var ownedByUs = ownedByUser

// Options says what to write and how strictly to judge the folder.
type Options struct {
	Label   string // the prefix of every message, such as "mcp http" or "studio automation"
	Dir     string // the folder; created 0700 when it is not there
	Name    string // the file's name in it
	Content string // exactly what the file holds
	// SharedDir is set when the person named the file (the folder is theirs and may be a shared one such as their home folder): the folder must then
	// only be a real folder, owned by this user and not writable by others, instead of private (0700).
	SharedDir bool
}

// Write writes the file and returns its path. replaced reports that a file was already there: one left by a process that was killed, or the one a
// running process still depends on.
func Write(o Options) (path string, replaced bool, err error) {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return "", false, fmt.Errorf("%s: token directory: %w", o.Label, err)
	}
	di, err := os.Lstat(o.Dir)
	if err != nil {
		return "", false, fmt.Errorf("%s: token directory: %w", o.Label, err)
	}
	if di.Mode()&os.ModeSymlink != 0 || !di.IsDir() {
		return "", false, fmt.Errorf("%s: token directory %s is a symlink or not a directory; refusing to use it", o.Label, o.Dir)
	}
	if permsEnforced {
		if !ownedByUs(di) {
			return "", false, fmt.Errorf("%s: token directory %s belongs to another user; refusing to use it", o.Label, o.Dir)
		}
		if o.SharedDir {
			if di.Mode().Perm()&0o022 != 0 {
				return "", false, fmt.Errorf("%s: token directory %s can be written by other users (mode %04o); run `chmod go-w` on it or choose another folder", o.Label, o.Dir, di.Mode().Perm())
			}
		} else if di.Mode().Perm()&0o077 != 0 {
			return "", false, fmt.Errorf("%s: token directory %s is open to other users (mode %04o); run `chmod 700` on it or remove it", o.Label, o.Dir, di.Mode().Perm())
		}
	}

	path = filepath.Join(o.Dir, o.Name)
	if fi, err := os.Lstat(path); err == nil {
		replaced = true
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return "", false, fmt.Errorf("%s: token file %s is a symlink or not a regular file; refusing to use it. Remove it and start again", o.Label, path)
		}
		if permsEnforced && fi.Mode().Perm()&0o077 != 0 {
			return "", false, fmt.Errorf("%s: token file %s is open to other users (mode %04o); refusing to use it. Remove it and start again", o.Label, path, fi.Mode().Perm())
		}
		if permsEnforced && !ownedByUs(fi) {
			return "", false, fmt.Errorf("%s: token file %s belongs to another user; refusing to use it. Remove it and start again", o.Label, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("%s: token file: %w", o.Label, err)
	}

	removeStaleTemps(o.Dir, o.Name)
	tmp, err := os.CreateTemp(o.Dir, o.Name+".*") // created 0600, with O_EXCL
	if err != nil {
		return "", false, fmt.Errorf("%s: token file: %w", o.Label, err)
	}
	_, werr := tmp.WriteString(o.Content)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o600)
	}
	if werr == nil {
		werr = renameFile(tmp.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return "", false, fmt.Errorf("%s: token file: %w", o.Label, werr)
	}
	return path, replaced, nil
}

// Remove removes the file on shutdown, but only while it still holds this process's content: a second process started later has put its own there.
// It never blocks on what is at the name: only a regular file of exactly the content's size is opened (without waiting, as a pipe someone planted
// there would make a plain open wait for a writer), and at most one byte more than the content is read.
func Remove(path, content string) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() != int64(len(content)) {
		return
	}
	f, err := os.OpenFile(path, os.O_RDONLY|openNonblock, 0)
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() || !os.SameFile(fi, st) {
		return // the name was changed between the check and the open
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(len(content))+1))
	if err == nil && string(b) == content {
		_ = os.Remove(path)
	}
}

// staleTempAge is how old a temporary file of ours must be before a new write removes it: a write that is under way is a few milliseconds.
const staleTempAge = 10 * time.Second

// removeStaleTemps removes the temporary files an earlier write of this name left behind when the process died between making the file and renaming
// it into place (the file holds the secret, mode 0600). Only a regular file of this user, named like our temporary files and old enough, is removed.
func removeStaleTemps(dir, name string) {
	matches, _ := filepath.Glob(filepath.Join(dir, name+".*"))
	for _, m := range matches {
		fi, err := os.Lstat(m)
		if err != nil || !fi.Mode().IsRegular() || !ownedByUs(fi) || time.Since(fi.ModTime()) < staleTempAge {
			continue
		}
		_ = os.Remove(m)
	}
}
