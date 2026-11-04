package namecache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type entry struct {
	Name    string `json:"name"`
	Project int64  `json:"project,omitempty"`
}

func clock(t0 time.Time) (func() time.Time, *time.Time) {
	now := t0
	return func() time.Time { return now }, &now
}

func TestPutGetWithinTTL(t *testing.T) {
	dir := t.TempDir()
	now, cur := clock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	c := New(dir, now)
	c.Put("a https://a.example", "task", "545", entry{Name: "Support", Project: 4963})

	var got entry
	if !c.Get("a https://a.example", "task", "545", &got) || got != (entry{"Support", 4963}) {
		t.Fatalf("Get = %+v", got)
	}
	// files live under <cacheDir>/papierkram/
	matches, _ := filepath.Glob(filepath.Join(dir, "papierkram", "names", "*.json"))
	if len(matches) != 1 {
		t.Errorf("cache files = %v", matches)
	}

	*cur = cur.Add(TTL - time.Second)
	if !New(dir, now).Get("a https://a.example", "task", "545", &got) {
		t.Error("expired before the TTL")
	}
	*cur = cur.Add(time.Second)
	if New(dir, now).Get("a https://a.example", "task", "545", &got) {
		t.Error("still valid at the TTL")
	}
}

// A value stored in the future (clock set back) is not trusted.
func TestGetIgnoresFutureEntries(t *testing.T) {
	dir := t.TempDir()
	now, cur := clock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	New(dir, now).Put("a", "user", "1", entry{Name: "Max"})
	*cur = cur.Add(-time.Minute)
	var got entry
	if New(dir, now).Get("a", "user", "1", &got) {
		t.Errorf("entry from the future used: %+v", got)
	}
}

// Context, resource and ID all separate entries.
func TestKeysAreSeparate(t *testing.T) {
	dir := t.TempDir()
	now, _ := clock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	c := New(dir, now)
	c.Put("a", "task", "1", entry{Name: "A"})
	var got entry
	for _, k := range [][3]string{{"b", "task", "1"}, {"a", "user", "1"}, {"a", "task", "2"}, {"a\x00task", "1", ""}} {
		if c.Get(k[0], k[1], k[2], &got) {
			t.Errorf("Get(%q) = %+v, want miss", k, got)
		}
	}
}

// The cache is only a cache: corrupt, foreign or unreadable files are a
// miss, an unwritable directory is silently skipped, and an empty cache
// directory disables it.
func TestCacheFailuresAreMisses(t *testing.T) {
	now, _ := clock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	dir := t.TempDir()
	c := New(dir, now)
	c.Put("a", "task", "1", entry{Name: "A"})
	matches, _ := filepath.Glob(filepath.Join(dir, "papierkram", "names", "*.json"))
	if len(matches) != 1 {
		t.Fatalf("cache files = %v", matches)
	}
	for _, content := range []string{"{kaputt", "", `{"context":"x","resource":"task","id":"1","stored":"2026-11-04T10:00:00Z","value":{"name":"B"}}`, `{"context":"a","resource":"task","id":"1","stored":"2026-11-04T10:00:00Z","value":"kein Objekt"}`} {
		if err := os.WriteFile(matches[0], []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		var got entry
		if c.Get("a", "task", "1", &got) {
			t.Errorf("content %q: hit %+v", content, got)
		}
	}

	// unwritable: a file where the directory should be
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, "papierkram"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := New(blocked, now)
	b.Put("a", "task", "1", entry{Name: "A"})
	var got entry
	if b.Get("a", "task", "1", &got) {
		t.Error("hit from an unwritable cache")
	}

	// disabled
	var none *Cache
	none.Put("a", "task", "1", entry{Name: "A"})
	if none.Get("a", "task", "1", &got) || New("", now).Get("a", "task", "1", &got) {
		t.Error("hit from a disabled cache")
	}
	New("", now).Put("a", "task", "1", entry{Name: "A"})
}

// Cached names may be customer names: the directory is 0700, the files
// 0600.
func TestCacheFilesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix modes")
	}
	dir := t.TempDir()
	now, _ := clock(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	New(dir, now).Put("a", "company", "1", entry{Name: "Muster GmbH"})
	names := filepath.Join(dir, "papierkram", "names")
	if st, err := os.Stat(names); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, %v", st.Mode().Perm(), err)
	}
	matches, _ := filepath.Glob(filepath.Join(names, "*.json"))
	for _, m := range matches {
		if st, err := os.Stat(m); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, %v", st.Mode().Perm(), err)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(names, "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}
