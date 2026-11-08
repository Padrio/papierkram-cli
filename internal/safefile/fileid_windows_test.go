//go:build windows

package safefile

import (
	"os"
	"path/filepath"
	"testing"
)

// The trap pinFileID closes: without it, an Lstat result compared after a
// swap reports the new file as the same (the ID is read by path at the
// comparison); pinned, the swap shows.
func TestPinFileIDKeepsTheFileOfTheLstat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timer.json")
	swap := func() {
		t.Helper()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("neu"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lstatOf := func() os.FileInfo {
		t.Helper()
		if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		return fi
	}
	lazy := lstatOf()
	swap()
	now, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(lazy, now) {
		t.Log("os.SameFile no longer reads the ID lazily; pinFileID is harmless")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	pinned := lstatOf()
	if !pinFileID(pinned) {
		t.Fatal("pinFileID failed on an existing file")
	}
	swap()
	if now, err = os.Lstat(path); err != nil {
		t.Fatal(err)
	}
	if os.SameFile(pinned, now) {
		t.Error("pinned FileInfo is the same file as the one swapped in")
	}
}
