//go:build windows

package safefile

import (
	"os"
	"syscall"
)

// openRead opens path for reading with all share modes, FILE_SHARE_DELETE
// included: os.Open shares only reading and writing, so while one process
// reads a file (time status, a second time stop) another could not remove
// or replace it (sharing violation), and the claim of time stop would
// fail (review, task 11). Without follow a symbolic link or other reparse
// point at path is opened itself, not followed (FILE_FLAG_OPEN_REPARSE_POINT);
// ReadRegular then rejects the handle as no regular file, like the Lstat
// before it and the os.SameFile check guard against a swapped file.
func openRead(path string, follow bool) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	attrs := uint32(syscall.FILE_ATTRIBUTE_NORMAL)
	if !follow {
		attrs |= syscall.FILE_FLAG_OPEN_REPARSE_POINT
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, attrs, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
