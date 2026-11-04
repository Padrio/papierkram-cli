//go:build unix

package projectfile

import (
	"os"
	"syscall"
)

// openRead opens path for reading; without follow a symbolic link at path
// is not followed (O_NOFOLLOW), closing the race between the Lstat of
// readRegular and the open (review M4). O_NONBLOCK keeps the open of a
// FIFO swapped in after the Stat from waiting for a writer (review N1);
// for a regular file it has no effect, and readRegular rejects anything
// else by the check on the handle.
func openRead(path string, follow bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	return os.OpenFile(path, flags, 0)
}
