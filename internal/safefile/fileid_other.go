//go:build !windows

package safefile

import "os"

// pinFileID is a no-op: os.Lstat and os.Stat fill device and inode at
// once, so os.SameFile compares the file of that moment.
func pinFileID(os.FileInfo) bool { return true }
