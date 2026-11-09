package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// Name resolution (spec v0.2 §4): a flag of registry.NameRefs takes a
// name instead of an ID. The list of the resource is loaded page by page
// (resolvePageSize entries, at most resolveMaxPages pages) and the name
// compared by nameKey; archived objects never match. Exactly one active
// match is the ID; none is exit 4, several or a list longer than
// resolveMaxPages pages exit 2: never a choice. The cache of the ID
// completion (same file, scope and TTL) answers a unique match without a
// request; the resolution writes it with the whole list (Complete).

const (
	// resolvePageSize is the page size of the resolution: the API maximum.
	resolvePageSize = allPageSize
	// resolveMaxPages is the most pages one resolution loads (500 entries).
	resolveMaxPages = 5
)

// Resolution is one resolved flag value: the matched object's name and ID
// and the credits of the list requests (0 from the cache).
type Resolution struct {
	Flag     string `json:"flag"`
	Name     string `json:"name"`
	Resource string `json:"resource"`
	ID       int64  `json:"id"`
	Credits  int    `json:"credits"`
	Cached   bool   `json:"cached"`
}

var idValueRe = regexp.MustCompile(`^[+-]?[0-9]+$`)

// isIDValue reports whether a flag value is an ID (an integer with an
// optional sign, surrounding whitespace ignored) rather than a name: "+12"
// stays an ID as before (spec.CoerceFlag), negative values stay IDs and
// fail like before (exit 2). The caller trims before parsing.
func isIDValue(raw string) bool { return idValueRe.MatchString(strings.TrimSpace(raw)) }

// nameKey is the form names are compared in: cacheName in lower case.
func nameKey(s string) string { return strings.ToLower(cacheName(s)) }

// nameResolver resolves names with the client and configuration of one
// command, after its read-only gate.
type nameResolver struct {
	a      *App
	c      *cobra.Command
	client *api.Client
	r      *config.Resolved
}

func (a *App) newNameResolver(c *cobra.Command, client *api.Client, r *config.Resolved) *nameResolver {
	return &nameResolver{a: a, c: c, client: client, r: r}
}

// resolve returns the ID of the active object of ref.Resource named name
// and writes the hint on stderr. parent is the ID of ref.Parent (the
// company of a contact person), 0 without parent. Errors of the list
// requests keep their code.
func (n *nameResolver) resolve(ref registry.NameRef, name string, parent int64) (Resolution, error) {
	e, ok := registry.Lookup(ref.Resource, "list")
	hasParent := strings.Contains(e.Path, "{")
	if !ok || hasParent != (ref.Parent != "") || hasParent && parent <= 0 {
		return Resolution{}, clierr.New(clierr.CodeError,
			fmt.Sprintf("interner Fehler: Namensauflösung für --%s ohne Liste oder übergeordnete ID", ref.Flag), "")
	}
	shown, key := cacheName(name), nameKey(name)
	if key == "" {
		return Resolution{}, clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültiger Wert für --%s: leer", ref.Flag), "ID oder Name angeben")
	}
	listCmd := "papierkram " + ref.Resource + " list"
	var params []int64
	if hasParent {
		// the only list with a parent: the persons of a company
		params = []int64{parent}
		listCmd += " --company " + strconv.FormatInt(parent, 10)
	}

	// the cache: lists without parent only, never a path relative to the
	// working directory; only a unique match is taken from it, a miss
	// loads (an object created a moment ago, an error on fresh data)
	dir := n.a.cacheDir()
	cache := !hasParent && dir != ""
	path, scope := "", cacheScope(n.r)
	if cache {
		path = completionCachePath(dir, n.r.Context, ref.Resource)
		if f, ok := readCompletionCache(path, scope, ref.Resource, n.a.Now()); ok && f.Complete {
			if active, _ := nameMatches(f.Entries, key); len(active) == 1 {
				return n.found(ref, active[0], 0, true), nil
			}
		}
	}

	ids, credits, total, err := n.load(e, params)
	if err != nil {
		if credits == 0 {
			return Resolution{}, err
		}
		// pages loaded before the failure were paid for
		ce := *clierr.From(err)
		ce.Hint = withCredits(ce.Hint, credits)
		return Resolution{}, &ce
	}
	if total > resolveMaxPages {
		return Resolution{}, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("%s „%s“ nicht aufgelöst: die Liste hat mehr als %d Einträge (%d Seiten), die Namensauflösung lädt höchstens %d Seiten",
				ref.Label, shown, resolveMaxPages*resolvePageSize, total, resolveMaxPages),
			withCredits("ID statt Name angeben ("+listCmd+")", credits))
	}
	if cache {
		writeCompletionCache(path, completionCache{Scope: scope, Resource: ref.Resource, Stored: n.a.Now(), Complete: true, Entries: ids})
	}
	active, archived := nameMatches(ids, key)
	switch {
	case len(active) == 1:
		return n.found(ref, active[0], credits, false), nil
	case len(active) > 1:
		return Resolution{}, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("%s „%s“ ist mehrdeutig: %s", ref.Label, shown, idList(active)), withCredits("ID statt Name angeben", credits))
	case len(archived) > 0:
		return Resolution{}, clierr.New(clierr.CodeNotFound,
			fmt.Sprintf("%s „%s“ nicht gefunden", ref.Label, shown), withCredits(archivedHint(ref.Resource, archived), credits))
	}
	hint := "ID angeben (" + listCmd + ")"
	if similar := similarNames(ids, shown, key); len(similar) > 0 {
		hint = "Ähnlich: " + idListNameFirst(similar) + "; sonst " + hint
	}
	return Resolution{}, clierr.New(clierr.CodeNotFound, fmt.Sprintf("%s „%s“ nicht gefunden", ref.Label, shown), withCredits(hint, credits))
}

// withCredits appends the credits the list requests used to the hint of
// a failed resolution: "… (2 Credits verbraucht)".
func withCredits(hint string, credits int) string {
	note := "(" + creditsText(credits) + " verbraucht)"
	if hint == "" {
		return note
	}
	return hint + " " + note
}

// found is the resolution of the match m and its hint on stderr.
func (n *nameResolver) found(ref registry.NameRef, m completionID, credits int, cached bool) Resolution {
	cost := creditsText(credits)
	if cached {
		cost = "aus dem Zwischenspeicher, 0 Credits"
	}
	_, _ = fmt.Fprintf(n.a.Err, "Hinweis: %s „%s“ → %d (%s)\n", ref.Label, m.Name, m.ID, cost)
	return Resolution{Flag: ref.Flag, Name: m.Name, Resource: ref.Resource, ID: m.ID, Credits: credits, Cached: cached}
}

// load reads the list of e (params: the parent ID) from page 1 up to
// total_pages, at most resolveMaxPages pages, through fetchPage. It
// returns the entries, the credits of the pages loaded (also with an
// error) and the largest total_pages seen; it stops as soon as that
// exceeds resolveMaxPages, an empty page ends the list early. Not fetchPages:
// its --max-pages warning names a flag the user did not give.
func (n *nameResolver) load(e registry.Entry, params []int64) ([]completionID, int, int64, error) {
	ctx := n.c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	fetch := func(q url.Values) (*api.Response, error) {
		return n.client.Call(ctx, e.OperationID, params, q, nil)
	}
	query := url.Values{"page_size": {strconv.Itoa(resolvePageSize)}}
	ids := []completionID{}
	credits, total := 0, int64(1)
	for p := int64(1); p <= min(total, resolveMaxPages); p++ {
		page, err := fetchPage(fetch, query, p)
		if err != nil {
			return nil, credits, 0, err
		}
		credits += e.Credits
		t, valid := totalPages(page.TotalPages)
		if !valid {
			return nil, credits, 0, unexpectedList(fmt.Errorf("total_pages ist %s", orMissing(page.TotalPages)))
		}
		total = max(total, t)
		if total > resolveMaxPages {
			// the resolution fails anyway: no further page (credits)
			return ids, credits, total, nil
		}
		if len(page.Entries) == 0 {
			break
		}
		objs := make([]map[string]any, 0, len(page.Entries))
		for _, raw := range page.Entries {
			var obj map[string]any
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if err := dec.Decode(&obj); err != nil {
				return nil, credits, 0, unexpectedList(err)
			}
			objs = append(objs, obj)
		}
		ids = append(ids, idEntries(objs)...)
	}
	return ids, credits, total, nil
}

// nameMatches splits the entries named key (nameKey) into active and
// archived ones.
func nameMatches(ids []completionID, key string) (active, archived []completionID) {
	for _, id := range ids {
		if nameKey(id.Name) != key {
			continue
		}
		if id.Archived {
			archived = append(archived, id)
		} else {
			active = append(active, id)
		}
	}
	return active, archived
}

// similarNames are up to three active entries close to the name: those
// whose name contains it first (list order), then the nearest of the rest
// (spec.Suggest).
func similarNames(ids []completionID, shown, key string) []completionID {
	const most = 3
	var out, rest []completionID
	for _, id := range ids {
		switch {
		case id.Archived || id.Name == "":
		case strings.Contains(nameKey(id.Name), key):
			out = append(out, id)
		default:
			rest = append(rest, id)
		}
	}
	if len(out) >= most {
		return out[:most]
	}
	names := make([]string, len(rest))
	for i, id := range rest {
		names[i] = id.Name
	}
	for _, s := range spec.Suggest(shown, names, most-len(out)) {
		for _, id := range rest {
			if id.Name == s {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

// idList is "2 „A“, 3 „B“".
func idList(ids []completionID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d „%s“", id.ID, id.Name)
	}
	return strings.Join(parts, ", ")
}

// idListNameFirst is "„A“ (2), „B“ (3)".
func idListNameFirst(ids []completionID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("„%s“ (%d)", id.Name, id.ID)
	}
	return strings.Join(parts, ", ")
}

// archivedHint names the archived matches and, when the resource has an
// unarchive command, how to reactivate one.
func archivedHint(resource string, archived []completionID) string {
	hint := "nur archiviert: " + idList(archived)
	if _, ok := registry.Lookup(resource, "unarchive"); !ok {
		return hint + "; ID angeben"
	}
	id := "<id>"
	if len(archived) == 1 {
		id = strconv.FormatInt(archived[0].ID, 10)
	}
	return hint + "; reaktivieren (papierkram " + resource + " unarchive " + id + ") oder ID angeben"
}
