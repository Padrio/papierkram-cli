//go:build windows

package safefile

import "os"

// pinFileID loads the file ID (volume and index) of fi, a FileInfo of
// os.Lstat or os.Stat, now. On Windows such a FileInfo keeps only the path
// and os.SameFile reads the ID by that path on its first comparison
// (os/types_windows.go, loadFileId); after a swap that would be the ID of
// the new file, so the check of ReadRegular would never see the swap.
// Comparing fi with itself loads the ID and caches it in fi. false: the ID
// could not be read (the file is gone or no longer accessible).
func pinFileID(fi os.FileInfo) bool { return os.SameFile(fi, fi) }
