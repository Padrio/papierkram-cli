//go:build unix

package cli

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByEUID reports whether fi (from Stat or Lstat) belongs to the
// effective user.
func ownedByEUID(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && uint64(st.Uid) == uint64(os.Geteuid())
}
