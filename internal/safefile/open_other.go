//go:build !unix

package safefile

import "os"

// openRead opens path for reading. Without O_NOFOLLOW the Lstat of
// ReadRegular and the os.SameFile check on the handle guard against a
// swapped file.
func openRead(path string, _ bool) (*os.File, error) {
	return os.Open(path)
}
