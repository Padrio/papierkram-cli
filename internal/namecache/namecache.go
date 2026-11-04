// Package namecache keeps small JSON values about API objects (their
// names) for a short time, so a command that shows names does not pay a
// credit per name on every call (spec 9.6: time log summary; completion
// reuses it). Entries are keyed by an account scope chosen by the caller
// (context and base URL), a resource and an ID, and live in
// <cacheDir>/papierkram/names/, one file per key.
//
// It is only a cache: every failure (no cache directory, a corrupt,
// foreign or unreadable file, a directory that cannot be written) is a
// miss or a skipped write, never an error of the command.
package namecache

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// TTL is how long an entry is used after it was stored.
const TTL = 60 * time.Second

// Cache is a name cache in one directory. A nil Cache or one without
// directory is disabled: Get always misses, Put does nothing.
type Cache struct {
	dir string
	now func() time.Time
}

// New returns the cache under <cacheDir>/papierkram/names; cacheDir is
// os.UserCacheDir() or PAPIERKRAM_CACHE_DIR (the caller decides, like
// config.Path for the config directory). An empty cacheDir disables the
// cache; a nil now means time.Now.
func New(cacheDir string, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	if cacheDir == "" {
		return &Cache{now: now}
	}
	return &Cache{dir: filepath.Join(cacheDir, "papierkram", "names"), now: now}
}

// file is the stored form of one entry. The key fields are repeated, so a
// hash collision or a moved file is a miss and never a wrong name.
type file struct {
	Scope    string          `json:"context"`
	Resource string          `json:"resource"`
	ID       string          `json:"id"`
	Stored   time.Time       `json:"stored"`
	Value    json.RawMessage `json:"value"`
}

// path is the file of a key: a hash, so scope and ID (a URL, a name) never
// become path elements.
func (c *Cache) path(scope, resource, id string) string {
	h := sha256.Sum256([]byte(scope + "\x00" + resource + "\x00" + id))
	return filepath.Join(c.dir, hex.EncodeToString(h[:16])+".json")
}

// Get decodes the entry of scope, resource and id into v and reports
// whether a valid entry was found: stored less than TTL ago (not in the
// future) and decodable into v.
func (c *Cache) Get(scope, resource, id string, v any) bool {
	if c == nil || c.dir == "" {
		return false
	}
	data, err := os.ReadFile(c.path(scope, resource, id))
	if err != nil {
		return false
	}
	var f file
	if json.Unmarshal(data, &f) != nil || f.Scope != scope || f.Resource != resource || f.ID != id {
		return false
	}
	age := c.now().Sub(f.Stored)
	if age < 0 || age >= TTL {
		return false
	}
	return json.Unmarshal(f.Value, v) == nil
}

// Put stores v for scope, resource and id, replacing an older entry
// atomically (temporary file and rename). Failures are ignored.
func (c *Cache) Put(scope, resource, id string, v any) {
	if c == nil || c.dir == "" {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	data, err := json.Marshal(file{Scope: scope, Resource: resource, ID: id, Stored: c.now(), Value: raw})
	if err != nil {
		return
	}
	if os.MkdirAll(c.dir, 0o700) != nil {
		return
	}
	target := c.path(scope, resource, id)
	tmp := target + "." + rand.Text()[:8] + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		_ = os.Remove(tmp)
		return
	}
	if os.Rename(tmp, target) != nil {
		_ = os.Remove(tmp)
	}
}
