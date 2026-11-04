// Package safefile reads and writes small local files that may be
// untrusted or that must never be half written: the project file of a
// cloned repository (projectfile), the timer state (timer) and the name
// cache (namecache). It is the one implementation of these rules; callers
// map its errors to their own messages.
package safefile

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	// ErrSymlink reports a symbolic link at a path that must not be one
	// (ReadRegular without follow).
	ErrSymlink = errors.New("symbolic link")
	// ErrNotRegular reports a device, FIFO, directory or other file that
	// is no regular file, or a file swapped between check and open.
	ErrNotRegular = errors.New("not a regular file")
	// ErrTooLarge reports a file larger than the limit of ReadRegular.
	ErrTooLarge = errors.New("file too large")
)

// ReadRegular reads the file at path when it is a regular file of at most
// limit bytes. A device (a checked-in link to /dev/zero), a FIFO (would
// block) or a directory is ErrNotRegular before it is opened; the opened
// handle is checked again, so a file swapped in between is never read
// beyond limit+1 bytes (ErrTooLarge). With follow a symbolic link to a
// regular file is read; without, a link is ErrSymlink and, where the
// platform allows, the open does not follow one either. Other errors are
// those of the file system (*fs.PathError, fs.ErrNotExist).
func ReadRegular(path string, follow bool, limit int64) ([]byte, error) {
	stat := os.Stat
	if !follow {
		stat = os.Lstat
	}
	st, err := stat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return nil, ErrSymlink
	}
	if !st.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	f, err := openRead(path, follow)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fst, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fst.Mode().IsRegular() || !os.SameFile(st, fst) {
		return nil, ErrNotRegular
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	return data, nil
}

// WriteOptions adjust WriteAtomic. ExactMode sets mode on the open file
// (for the mode of a replaced file; otherwise the umask applies at
// creation). NoSync skips the fsync before the rename: for a cache, whose
// loss is harmless and whose writes should be cheap (on darwin a sync is
// F_FULLFSYNC).
type WriteOptions struct {
	ExactMode, NoSync bool
}

// linkFile is os.Link; tests replace it to run the fallback of
// CreateAtomic for file systems without hard links.
var linkFile = os.Link

// renameFile is os.Rename for the fallback of CreateAtomic; tests make it
// fail.
var renameFile = os.Rename

// syncFile flushes f to disk; tests count its calls.
var syncFile = (*os.File).Sync

// WriteAtomic writes data to a new temporary file next to path with mode
// and renames it to path (see WriteOptions). The rename replaces the
// directory entry at path itself, so a symbolic link there is replaced
// and never followed; the temporary file is created exclusively.
func WriteAtomic(path string, data []byte, mode fs.FileMode, o WriteOptions) error {
	tmp, err := writeTemp(path, data, mode, o)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }() // no-op after the rename
	return os.Rename(tmp, path)
}

// CreateAtomic is WriteAtomic for a file that must not exist yet: the
// complete temporary file is linked to path, which fails with an error
// matching fs.ErrExist when anything is there (also a symbolic link,
// which is not followed), so two writers never both succeed and a reader
// never sees a partial file. Where the file system has no hard links,
// path is first claimed with O_EXCL and the file renamed over the claim:
// still exclusive, but a reader may see an empty file for a moment.
func CreateAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := writeTemp(path, data, mode, WriteOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	lerr := linkFile(tmp, path)
	if lerr == nil || errors.Is(lerr, fs.ErrExist) {
		return lerr
	}
	// no hard links: claim path exclusively (O_EXCL also fails on a
	// symbolic link), then rename the complete file over the claim. Of
	// concurrent writers exactly one wins; a reader may briefly see the
	// empty claim (review M6).
	claim, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	claimed, serr := claim.Stat()
	_ = claim.Close()
	if err := renameFile(tmp, path); err != nil {
		// remove the claim only while path is still it: another writer
		// may have taken path since (review N2)
		if now, err := os.Lstat(path); serr == nil && err == nil && os.SameFile(claimed, now) {
			_ = os.Remove(path)
		}
		return err
	}
	return nil
}

// writeTemp writes data to a new, exclusively created temporary file in
// the directory of path and returns its name; it is removed on error.
func writeTemp(path string, data []byte, mode fs.FileMode, o WriteOptions) (string, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	var f *os.File
	var err error
	for range 10 {
		name := filepath.Join(dir, "."+base+"."+rand.Text()[:12]+".tmp")
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil && o.ExactMode {
		_ = f.Chmod(mode) // best effort (file systems without Unix modes)
	}
	if err == nil && !o.NoSync {
		err = syncFile(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}
