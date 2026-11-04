//go:build !unix

package projectfile

import "os"

// openRead opens path for reading. Without O_NOFOLLOW the Lstat of
// readRegular and the os.SameFile check on the handle guard against a
// swapped file.
func openRead(path string, _ bool) (*os.File, error) {
	return os.Open(path)
}
