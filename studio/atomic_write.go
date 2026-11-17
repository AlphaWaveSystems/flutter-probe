package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// replaceFileAtomically puts data at path so that the file is never seen half written: the data goes to a new file in the same folder (same file
// system, so the rename below is one step), is flushed to disk, and only then renamed over path. A failure at any point, a panic included, removes the
// new file and leaves path as it was, byte for byte. An existing file keeps its permission bits (only those: the owner, the group, extended attributes
// such as the macOS quarantine mark, and the setuid and sticky bits are not carried over); a new one is readable by others (0644), like the files
// Studio has always written.
//
// What it refuses, as the plain write it replaces did: a target that is not a regular file (a plain write to a FIFO or a device would not have replaced it
// with a file), and a read-only file ("permission denied", as the open for writing used to say: the rename would otherwise replace a file whose
// write bit was cleared on purpose). One change of behaviour that follows from the design: the folder must allow creating the temporary file, so a
// writable file in a folder that is not writable, which used to be saved in place, now fails and says so.
//
// The final name is the one the caller resolved, so a link the caller followed is not replaced: the file it points at is. On Windows os.Rename replaces an
// existing file (MoveFileEx with MOVEFILE_REPLACE_EXISTING), the same call the results export and the discovery-file writer (internal/secretfile) use; it fails
// if another process holds the file open without sharing delete, and the error is returned with the file untouched. A rename gives the file a new identity:
// a hard link to it is not updated. The folder is not flushed after the rename: the data is on disk, but a power cut can still undo the rename.
func replaceFileAtomically(path string, data []byte, tmpPattern string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		if info.Mode().Perm()&0o200 == 0 {
			return &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
		}
		mode = info.Mode().Perm()
	}
	tmp, err := atomicCreateTemp(filepath.Dir(path), tmpPattern)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot create a temporary file next to %s: the folder is not writable: %w", filepath.Base(path), err)
		}
		return err
	}
	name := tmp.Name()
	done := false
	defer func() {
		if !done { // an error or a panic: the new file does not stay
			tmp.Close()
			_ = os.Remove(name)
		}
	}()
	if _, err := atomicWrite(tmp, data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := atomicSync(tmp); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := atomicRename(name, path); err != nil {
		return err
	}
	done = true
	return nil
}

// The steps tests replace: the creation of the temporary file (os.CreateTemp: a random name, created exclusively, so a link planted at a guessable name
// is never followed), the write, the flush to disk and the rename.
var (
	atomicCreateTemp = os.CreateTemp
	atomicWrite      = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	atomicSync       = func(f *os.File) error { return f.Sync() }
	atomicRename     = os.Rename
)
