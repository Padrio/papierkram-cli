package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/namecache"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// ID completion (spec 7.3): the IDs of a resource come from its list
// request (page 1, page_size 100) and are shown as "id<TAB>name". The
// result is cached per context and resource for namecache.TTL (60 s) in
// <cacheDir>/papierkram/completion-<ctx>-<resource>.json; the context
// option completion_lookup = false switches requests and cache off.

// completionPageSize is the page size of the lookup: the API maximum, as
// many IDs as one request (one credit) can bring.
const completionPageSize = allPageSize

// completionTimeout bounds a lookup as a whole, independent of --timeout
// and the context timeout: the shell waits for it. A variable for tests.
var completionTimeout = 3 * time.Second

// completionCacheMax is the largest cache file that is read; 100 entries
// with names of at most 60 runes need far less.
const completionCacheMax = 256 << 10

// completionNameRunes is the longest name shown beside an ID.
const completionNameRunes = 60

// completionCache is the cache file of one context and resource. Scope is
// the account (cacheScope: context and base URL without userinfo); a file
// of another account, resource or age is a miss.
type completionCache struct {
	Scope    string         `json:"scope"`
	Resource string         `json:"resource"`
	Stored   time.Time      `json:"stored"`
	Entries  []completionID `json:"entries"`
}

// completionID is one object of the lookup.
type completionID struct {
	ID   int64  `json:"id"`
	Name string `json:"name,omitempty"`
}

// completionCachePath is the cache file of the context ctx (config.FileName,
// the form of the timer state) and resource below cacheDir.
func completionCachePath(cacheDir, ctx, resource string) string {
	return filepath.Join(cacheDir, "papierkram", "completion-"+config.FileName(ctx)+"-"+resource+".json")
}

// completeIDs returns the "id<TAB>name" candidates of resource whose ID
// starts with toComplete. The configuration is resolved like for every
// API command (resolveConfig, requireCredentials); a lookup that is off,
// fails or has no list without parent (person) gives none. Warnings of
// the resolution are silenced.
func (a *App) completeIDs(c *cobra.Command, resource, toComplete string) []string {
	e, ok := registry.Lookup(resource, "list")
	if resource == "" || !ok || strings.Contains(e.Path, "{") {
		return nil
	}
	defer a.silence()()
	r, err := a.resolveConfig()
	if err != nil || !r.CompletionLookup || requireCredentials(r) != nil {
		return nil
	}
	path, scope := completionCachePath(a.cacheDir(), r.Context, resource), cacheScope(r)
	ids, ok := readCompletionCache(path, scope, resource, a.Now())
	if !ok {
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		if ids, ok = a.lookupIDs(ctx, r, e); !ok {
			return nil
		}
		writeCompletionCache(path, completionCache{Scope: scope, Resource: resource, Stored: a.Now(), Entries: ids})
	}
	var out []string
	for _, id := range ids {
		s := strconv.FormatInt(id.ID, 10)
		if !strings.HasPrefix(s, toComplete) {
			continue
		}
		if name := completionName(id.Name); name != "" {
			s += "\t" + name
		}
		out = append(out, s)
	}
	return out
}

// lookupIDs sends the list request of e: the client of the API commands
// (clientOptions) with the completion's own timeout and without --debug
// output. Objects without a positive ID are left out.
func (a *App) lookupIDs(ctx context.Context, r *config.Resolved, e registry.Entry) ([]completionID, bool) {
	o := a.clientOptions(r)
	o.Timeout, o.Debug, o.DebugInfo = completionTimeout, nil, ""
	ctx, cancel := context.WithTimeout(ctx, completionTimeout)
	defer cancel()
	query := url.Values{"page_size": {strconv.Itoa(completionPageSize)}}
	resp, err := api.New(o).Call(ctx, e.OperationID, nil, query, nil)
	if err != nil {
		return nil, false
	}
	var page struct {
		Entries []map[string]any `json:"entries"`
	}
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if dec.Decode(&page) != nil {
		return nil, false
	}
	ids := []completionID{}
	for _, obj := range page.Entries {
		if id := jsonInt(obj["id"]); id > 0 {
			ids = append(ids, completionID{ID: id, Name: completionName(objectName(obj))})
		}
	}
	return ids, true
}

// completionName is a name as one line of printable text (output.Printable,
// whitespace collapsed) of at most completionNameRunes runes: it comes
// from the API or a cache file and ends up in the shell.
func completionName(s string) string {
	return output.Truncate(strings.Join(strings.Fields(output.Printable(s)), " "), completionNameRunes)
}

// readCompletionCache returns the IDs of the cache file at path when it
// is a regular file (no symbolic link) of scope and resource stored less
// than namecache.TTL ago. Anything else is a miss.
func readCompletionCache(path, scope, resource string, now time.Time) ([]completionID, bool) {
	data, err := safefile.ReadRegular(path, false, completionCacheMax)
	if err != nil {
		return nil, false
	}
	var f completionCache
	dec := json.NewDecoder(bytes.NewReader(data))
	if dec.Decode(&f) != nil || f.Scope != scope || f.Resource != resource || f.Entries == nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	if age := now.Sub(f.Stored); age < 0 || age >= namecache.TTL {
		return nil, false
	}
	ids := f.Entries[:0]
	for _, id := range f.Entries {
		if id.ID > 0 {
			ids = append(ids, id)
		}
	}
	return ids, true
}

// writeCompletionCache stores f at path, best effort: atomically, private
// and without fsync (a cache). A failure leaves the cache as it was.
func writeCompletionCache(path string, f completionCache) {
	data, err := json.Marshal(f)
	if err != nil || os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	_ = safefile.WriteAtomic(path, data, 0o600, safefile.WriteOptions{NoSync: true})
}
