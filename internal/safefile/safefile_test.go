package safefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
}

func TestReadRegular(t *testing.T) {
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "f"), "12345")
	if data, err := ReadRegular(f, false, 5); err != nil || string(data) != "12345" {
		t.Errorf("regular file of exactly max: %q, %v", data, err)
	}
	if _, err := ReadRegular(f, false, 4); !errors.Is(err, ErrTooLarge) {
		t.Errorf("larger than max: %v", err)
	}
	if _, err := ReadRegular(dir, true, 5); !errors.Is(err, ErrNotRegular) {
		t.Errorf("directory: %v", err)
	}
	if _, err := ReadRegular(filepath.Join(dir, "missing"), false, 5); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: %v", err)
	}
	link := filepath.Join(dir, "link")
	symlink(t, f, link)
	if _, err := ReadRegular(link, false, 5); !errors.Is(err, ErrSymlink) {
		t.Errorf("link without follow: %v", err)
	}
	if data, err := ReadRegular(link, true, 5); err != nil || string(data) != "12345" {
		t.Errorf("link with follow: %q, %v", data, err)
	}
}

func TestWriteAtomicReplacesLinkWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	target := write(t, filepath.Join(dir, "target"), "keep")
	path := filepath.Join(dir, "state")
	symlink(t, target, path)
	if err := WriteAtomic(path, []byte("new"), 0o600, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Errorf("link target written: %q", got)
	}
	if st, err := os.Lstat(path); err != nil || !st.Mode().IsRegular() {
		t.Errorf("path is no regular file: %v, %v", st, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("content = %q", got)
	}
	assertNoTemp(t, dir)
}

func TestCreateAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if err := CreateAtomic(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one" {
		t.Errorf("content = %q", got)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
	// an existing file stays as it is
	if err := CreateAtomic(path, []byte("two"), 0o600); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second create: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one" {
		t.Errorf("content after second create = %q", got)
	}
	// a link at path (also a dangling one) is neither followed nor replaced
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	symlink(t, target, link)
	if err := CreateAtomic(link, []byte("x"), 0o600); !errors.Is(err, fs.ErrExist) {
		t.Errorf("create at a dangling link: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("link target created: %v", err)
	}
	assertNoTemp(t, dir)
}

// assertNoTemp checks that no temporary file is left in dir.
func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file left: %s", e.Name())
		}
	}
}

// Review M5: WriteAtomic syncs the file before the rename unless NoSync
// is set (a cache needs no durability); CreateAtomic always syncs.
func TestWriteAtomicSyncOption(t *testing.T) {
	calls := 0
	orig := syncFile
	syncFile = func(f *os.File) error { calls++; return orig(f) }
	t.Cleanup(func() { syncFile = orig })
	dir := t.TempDir()
	if err := WriteAtomic(filepath.Join(dir, "a"), []byte("x"), 0o600, WriteOptions{}); err != nil || calls != 1 {
		t.Errorf("default: %v, %d syncs", err, calls)
	}
	if err := WriteAtomic(filepath.Join(dir, "b"), []byte("x"), 0o600, WriteOptions{NoSync: true}); err != nil || calls != 1 {
		t.Errorf("NoSync: %v, %d syncs", err, calls)
	}
	if err := CreateAtomic(filepath.Join(dir, "c"), []byte("x"), 0o600); err != nil || calls != 2 {
		t.Errorf("CreateAtomic: %v, %d syncs", err, calls)
	}
}

// Review M6: of concurrent CreateAtomic calls on one path exactly one
// succeeds, the others get fs.ErrExist, with hard links and in the
// fallback for file systems without them; the file is complete.
func TestCreateAtomicConcurrent(t *testing.T) {
	for _, noLinks := range []bool{false, true} {
		if noLinks {
			orig := linkFile
			linkFile = func(string, string) error { return errors.New("hard links not supported") }
			t.Cleanup(func() { linkFile = orig })
		}
		path := filepath.Join(t.TempDir(), "state")
		const n = 16
		errs := make(chan error, n)
		var start sync.WaitGroup
		start.Add(1)
		for i := range n {
			go func() {
				start.Wait()
				errs <- CreateAtomic(path, []byte(strings.Repeat(string(rune('a'+i)), 1000)), 0o600)
			}()
		}
		start.Done()
		ok := 0
		for range n {
			switch err := <-errs; {
			case err == nil:
				ok++
			case !errors.Is(err, fs.ErrExist):
				t.Errorf("noLinks=%v: %v", noLinks, err)
			}
		}
		if ok != 1 {
			t.Errorf("noLinks=%v: %d writers succeeded, want 1", noLinks, ok)
		}
		data, _ := os.ReadFile(path)
		if len(data) != 1000 || strings.Count(string(data), string(data[:1])) != 1000 {
			t.Errorf("noLinks=%v: content %q…", noLinks, data[:min(len(data), 10)])
		}
		assertNoTemp(t, filepath.Dir(path))
	}
}
