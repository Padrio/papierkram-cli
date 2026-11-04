//go:build unix

package projectfile

import (
	"os"
	"syscall"
)

// openRead opens path for reading; without follow a symbolic link at path
// is not followed (O_NOFOLLOW), closing the race between the Lstat of
// readRegular and the open (review M4).
func openRead(path string, follow bool) (*os.File, error) {
	flags := os.O_RDONLY
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	return os.OpenFile(path, flags, 0)
}
