package main

import (
	"os"
	"path/filepath"
)

// replaceFileAtomically puts data at path so that the file is never seen half written: the data goes to a new file in the same folder (same file
// system, so the rename below is one step), is flushed to disk, and only then renamed over path. A failure at any point removes the new file and leaves
// path as it was, byte for byte. An existing file keeps its permissions; a new one is readable by others (0644), like the files Studio has always written.
//
// The final name is the one the caller resolved, so a link the caller followed is not replaced: the file it points at is. On Windows os.Rename replaces an
// existing file (MoveFileEx with MOVEFILE_REPLACE_EXISTING), the same call the results export and the discovery-file writer (internal/secretfile) use; it fails
// if another process holds the file open without sharing delete, and the error is returned with the file untouched. A rename gives the file a new identity:
// a hard link to it is not updated, and the owner and any extended attributes are not carried over.
func replaceFileAtomically(path string, data []byte, tmpPattern string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), tmpPattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := atomicWrite(tmp, data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := atomicSync(tmp); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := atomicRename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// The three steps tests make fail: the write, the flush to disk and the rename.
var (
	atomicWrite  = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	atomicSync   = func(f *os.File) error { return f.Sync() }
	atomicRename = os.Rename
)
