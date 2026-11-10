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
	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/namecache"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/safefile"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// ID completion (spec 7.3): the IDs of a resource come from its list
// request (page 1, page_size 100) and are shown as "id<TAB>name". The
// result is cached per context and resource for namecache.TTL (60 s) in
// <cacheDir>/papierkram/completion-<ctx>-<resource>.json; the context
// option completion_lookup = false switches requests and cache off. The
// name resolution (resolve.go) reads and writes the same file.

// completionPageSize is the page size of the lookup: the API maximum, as
// many IDs as one request (one credit) can bring.
const completionPageSize = allPageSize

// completionTimeout bounds a lookup as a whole, independent of --timeout
// and the context timeout: the shell waits for it. A variable for tests.
var completionTimeout = 3 * time.Second

// completionCacheMax is the largest cache file that is read: the name
// resolution stores up to resolveMaxPages * resolvePageSize (500) entries
// with untruncated names.
const completionCacheMax = 1 << 20

// completionNameRunes is the longest name shown beside an ID.
const completionNameRunes = 60

// completionCache is the cache file of one context and resource. Scope is
// the account (cacheScope: context and base URL without userinfo); a file
// of another account, resource or age is a miss. Complete is set when
// Entries hold the whole list (one page of the completion, all pages of
// the name resolution); a file without it (v0.1) is incomplete.
type completionCache struct {
	Scope    string         `json:"scope"`
	Resource string         `json:"resource"`
	Stored   time.Time      `json:"stored"`
	Complete bool           `json:"complete,omitempty"`
	Entries  []completionID `json:"entries"`
}

// completionID is one object of the lookup: its ID, its name (cacheName,
// untruncated) and whether it is archived.
type completionID struct {
	ID       int64  `json:"id"`
	Name     string `json:"name,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

// completionCachePath is the cache file of the context ctx (config.FileName,
// the form of the timer state) and resource below cacheDir.
func completionCachePath(cacheDir, ctx, resource string) string {
	return filepath.Join(cacheDir, "papierkram", "completion-"+config.FileName(ctx)+"-"+resource+".json")
}

// completeIDs returns the "id<TAB>name" candidates of resource whose ID
// starts with toComplete. The configuration is resolved like for every
// API command (resolveWith, requireCredentials), in two steps so that the
// keychain is read only for a request (review I1): first without it –
// a lookup that is off or has no base URL ends here, a cache hit returns
// –, then on a miss with it, under the deadline of the request. A lookup
// that fails or has no list without parent (person) gives none. A local
// --context of the command (local init) chooses the account. Warnings are
// silenced.
func (a *App) completeIDs(c *cobra.Command, resource, toComplete string) []string {
	e, ok := registry.Lookup(resource, "list")
	if resource == "" || !ok || strings.Contains(e.Path, "{") {
		return nil
	}
	defer a.silence()()
	flagContext := a.Flags.Context
	if f := c.LocalNonPersistentFlags().Lookup("context"); f != nil && f.Changed {
		flagContext = f.Value.String()
	}
	r, err := a.resolveWith(flagContext, nil)
	if err != nil || !r.CompletionLookup || r.BaseURL == "" {
		return nil
	}
	// without a cache directory there is no cache (never a path relative
	// to the working directory)
	dir, scope := a.cacheDir(), cacheScope(r)
	path := completionCachePath(dir, r.Context, resource)
	var cached completionCache
	ok = false
	if dir != "" {
		cached, ok = readCompletionCache(path, scope, resource, a.Now())
	}
	ids := cached.Entries
	if !ok {
		parent := c.Context()
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, completionTimeout)
		defer cancel()
		var store auth.Store
		if a.Keyring != nil {
			store = deadlineStore{Store: a.Keyring, ctx: ctx}
		}
		if r, err = a.resolveWith(flagContext, store); err != nil || requireCredentials(r) != nil {
			return nil
		}
		var complete bool
		if ids, complete, ok = a.lookupIDs(ctx, r, e); !ok {
			return nil
		}
		if dir != "" {
			writeCompletionCache(path, completionCache{Scope: scope, Resource: resource, Stored: a.Now(), Complete: complete, Entries: ids})
		}
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

// deadlineStore reads the keychain until ctx ends: a keychain that waits
// (a locked Secret Service collection showing its unlock dialog) is given
// up, its answer is dropped. Resolve then fails and there are no IDs.
type deadlineStore struct {
	auth.Store
	ctx context.Context
}

func (s deadlineStore) Get(name string) (string, error) {
	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		t, err := s.Store.Get(name)
		done <- result{t, err}
	}()
	select {
	case r := <-done:
		return r.token, r.err
	case <-s.ctx.Done():
		return "", s.ctx.Err()
	}
}

// lookupIDs sends the list request of e within ctx: the client of the API
// commands (clientOptions) with the completion's own timeout and without
// --debug output. Unlike clientFor it prints no warning about a base URL
// without https (warnInsecure): stderr of a completion would garble the
// shell prompt. complete is set when page 1 is the whole list
// (total_pages at most 1; missing or invalid: not complete).
func (a *App) lookupIDs(ctx context.Context, r *config.Resolved, e registry.Entry) (ids []completionID, complete, ok bool) {
	o := a.clientOptions(r)
	o.Timeout, o.Debug, o.DebugInfo = completionTimeout, nil, ""
	query := url.Values{"page_size": {strconv.Itoa(completionPageSize)}}
	resp, err := api.New(o).Call(ctx, e.OperationID, nil, query, nil)
	if err != nil {
		return nil, false, false
	}
	var page struct {
		TotalPages json.RawMessage  `json:"total_pages"`
		Entries    []map[string]any `json:"entries"`
	}
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if dec.Decode(&page) != nil {
		return nil, false, false
	}
	total, valid := totalPages(page.TotalPages)
	return idEntries(page.Entries), valid && total <= 1, true
}

// totalPages is the total_pages of a list answer; valid is false when it
// is missing, null, no integer or negative.
func totalPages(raw json.RawMessage) (total int64, valid bool) {
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &total) != nil || total < 0 {
		return 0, false
	}
	return total, true
}

// idEntries are the cache entries of list entries decoded with UseNumber:
// objects without a positive ID are left out, the name is cacheName of
// objectName (untruncated), record_state "archived" marks archived ones.
// Never nil (a cache file with entries null is a miss).
func idEntries(entries []map[string]any) []completionID {
	ids := []completionID{}
	for _, obj := range entries {
		if id := jsonInt(obj["id"]); id > 0 {
			state, _ := obj["record_state"].(string)
			ids = append(ids, completionID{ID: id, Name: cacheName(objectName(obj)), Archived: state == "archived"})
		}
	}
	return ids
}

// cacheName is a name as one line of printable text (output.Printable,
// whitespace collapsed), untruncated: the form stored in the cache and
// compared by the name resolution.
func cacheName(s string) string {
	return strings.Join(strings.Fields(output.Printable(s)), " ")
}

// completionName is cacheName cut to completionNameRunes runes: it comes
// from the API or a cache file and ends up in the shell.
func completionName(s string) string {
	return output.Truncate(cacheName(s), completionNameRunes)
}

// readCompletionCache returns the cache file at path when it is a regular
// file (no symbolic link) of scope and resource stored less than
// namecache.TTL ago, with the entries of a positive ID and their names as
// cacheName (a file edited by hand). Anything else is a miss.
func readCompletionCache(path, scope, resource string, now time.Time) (completionCache, bool) {
	data, err := safefile.ReadRegular(path, false, completionCacheMax)
	if err != nil {
		return completionCache{}, false
	}
	var f completionCache
	dec := json.NewDecoder(bytes.NewReader(data))
	if dec.Decode(&f) != nil || f.Scope != scope || f.Resource != resource || f.Entries == nil {
		return completionCache{}, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return completionCache{}, false
	}
	if age := now.Sub(f.Stored); age < 0 || age >= namecache.TTL {
		return completionCache{}, false
	}
	ids := f.Entries[:0]
	for _, id := range f.Entries {
		if id.ID > 0 {
			id.Name = cacheName(id.Name)
			ids = append(ids, id)
		}
	}
	f.Entries = ids
	return f, true
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

// removeCompletionCaches deletes the completion caches of the context ctx
// (auth logout, context delete, context rename for the old name): the
// fetched names of the account go with its token. Exact file names per
// resource of the registry, never a prefix pattern (the caches of a-b
// share the prefix of a); silent and best effort.
func (a *App) removeCompletionCaches(ctx string) {
	dir := a.cacheDir()
	if dir == "" {
		return
	}
	for _, r := range registry.Resources() {
		_ = os.Remove(completionCachePath(dir, ctx, r.Name))
	}
}

// dropCompletionCache deletes the cache of the context ctx for the
// resource a mutation (method and path relative to the API root) may have
// changed, after every such request whatever its outcome (final review
// B-I1): the name resolution must not take an archived, renamed or
// deleted object, nor miss a new one of the same name, from a cache
// written before. A path of no registry command (papierkram api) drops
// all caches of the context. Silent and best effort.
func (a *App) dropCompletionCache(ctx, method, path string) {
	dir := a.cacheDir()
	if dir == "" {
		return
	}
	if op, _, ok := spec.MatchPath(method, path); ok {
		if e, found := registry.ByOperationID(op.ID); found {
			_ = os.Remove(completionCachePath(dir, ctx, e.Resource))
			return
		}
	}
	a.removeCompletionCaches(ctx)
}
