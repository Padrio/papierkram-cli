//go:build !unix

package cli

import "io/fs"

// ownedByEUID treats every file as the user's own: outside Unix there is
// no owner uid to compare, and the behaviour stays as before.
func ownedByEUID(fs.FileInfo) bool { return true }
