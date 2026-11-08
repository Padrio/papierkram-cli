package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/namecache"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/redact"
)

// The summary of time log in a terminal (spec 9.6): company, project,
// task and user by name. A name costs one request (1 credit) and is
// cached for namecache.TTL; the task brings the name of its project, the
// project the ID of its company.

// cacheDir is the base of the cache files: PAPIERKRAM_CACHE_DIR or the
// app's cache directory (os.UserCacheDir()); the cache packages append
// papierkram/, as config.Path does for the config directory.
func (a *App) cacheDir() string {
	if v := a.Env("PAPIERKRAM_CACHE_DIR"); v != "" {
		return v
	}
	return a.CacheDir
}

// names is the name cache of this invocation's account.
func (a *App) names() *namecache.Cache { return namecache.New(a.cacheDir(), a.Now) }

// cacheScope is the account of r for cache keys: context and base URL,
// since environment variables can point a context to another account.
// The URL is without userinfo: the scope is stored in the cache files.
func cacheScope(r *config.Resolved) string { return r.Context + " " + redact.URL(r.BaseURL) }

// cachedName is the cached value of an object: its name and, for a task
// its project, for a project its company.
type cachedName struct {
	Name    string `json:"name"`
	Project int64  `json:"project,omitempty"`
	Company int64  `json:"company,omitempty"`
}

// nameOps are the GET operations of the resources with names.
var nameOps = map[string]string{
	"task":    "GetTask",
	"project": "GetProject",
	"company": "GetCompany",
	"user":    "GetUser",
}

// nameLookup loads names for one summary: from this invocation (a task
// brings its project even when the cache cannot be written), the cache or
// the API.
type nameLookup struct {
	a      *App
	client *api.Client
	cache  *namecache.Cache
	scope  string
	seen   map[string]cachedName
}

func (l *nameLookup) put(resource string, id int64, n cachedName) {
	key := strconv.FormatInt(id, 10)
	l.seen[resource+"/"+key] = n
	l.cache.Put(l.scope, resource, key, n)
}

// name returns the name of resource id, loading it (1 credit) when it is
// neither known nor cached; a task also stores its project. A failed load
// is a warning and ok false; only an interrupt is an error (exit 10).
func (l *nameLookup) name(ctx context.Context, resource string, id int64) (cachedName, bool, error) {
	key := strconv.FormatInt(id, 10)
	if n, ok := l.seen[resource+"/"+key]; ok {
		return n, true, nil
	}
	var n cachedName
	if l.cache.Get(l.scope, resource, key, &n) {
		l.seen[resource+"/"+key] = n
		return n, true, nil
	}
	resp, err := l.client.Call(ctx, nameOps[resource], []int64{id}, nil, nil)
	if err != nil {
		if ctx.Err() != nil {
			return n, false, err
		}
		output.Warn(l.a.Err, fmt.Sprintf("Name von %s %d nicht geladen (%s); Zusammenfassung mit ID", resourceNoun(resource), id, clierr.From(err).Message))
		return n, false, nil
	}
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if dec.Decode(&obj) != nil || obj == nil {
		// like a failed request: the summary shows the ID (review M7)
		output.Warn(l.a.Err, fmt.Sprintf("Name von %s %d nicht geladen (unerwartete Antwort); Zusammenfassung mit ID", resourceNoun(resource), id))
		return n, false, nil
	}
	n.Name = objectName(obj)
	switch resource {
	case "task":
		if p, ok := obj["project"].(map[string]any); ok {
			n.Project = jsonInt(p["id"])
			// only a complete embed saves the project request: without
			// company_id the project is loaded for its company (review M7)
			if pn, co := objectName(p), jsonInt(p["company_id"]); n.Project != 0 && pn != "" && co != 0 {
				l.put("project", n.Project, cachedName{Name: pn, Company: co})
			}
		}
	case "project":
		n.Company = jsonInt(obj["company_id"])
	}
	l.put(resource, id, n)
	return n, true, nil
}

// jsonInt is an integer of a JSON object decoded with UseNumber, 0 when
// it is none.
func jsonInt(v any) int64 {
	if num, ok := v.(json.Number); ok {
		if i, err := num.Int64(); err == nil {
			return i
		}
	}
	return 0
}

// confirmTimeLog shows the summary of the time entry on stderr and asks
// (Enter = yes). Declining, the end of the input or an interrupt of the
// prompt is exit 8; an interrupt while a name loads is exit 10.
func (a *App) confirmTimeLog(ctx context.Context, client *api.Client, r *config.Resolved, p period, task, user int64, body map[string]any) error {
	l := &nameLookup{a: a, client: client, cache: a.names(), scope: cacheScope(r), seen: map[string]cachedName{}}
	type line struct{ label, value string }
	var lines []line
	named := func(n cachedName, ok bool, id int64) string {
		if ok && n.Name != "" {
			return fmt.Sprintf("%s (%d)", printable(n.Name), id)
		}
		return strconv.FormatInt(id, 10)
	}
	t, tok, err := l.name(ctx, "task", task)
	if err != nil {
		return err
	}
	if tok && t.Project != 0 {
		pr, pok, err := l.name(ctx, "project", t.Project)
		if err != nil {
			return err
		}
		if pok && pr.Company != 0 {
			co, cok, err := l.name(ctx, "company", pr.Company)
			if err != nil {
				return err
			}
			lines = append(lines, line{"Firma", named(co, cok, pr.Company)})
		}
		lines = append(lines, line{"Projekt", named(pr, pok, t.Project)})
	}
	lines = append(lines, line{"Aufgabe", named(t, tok, task)})
	u, uok, err := l.name(ctx, "user", user)
	if err != nil {
		return err
	}
	lines = append(lines, line{"Benutzer", named(u, uok, user)})
	if v, ok := body["comments"].(string); ok && v != "" {
		lines = append(lines, line{"Kommentar", printable(v)})
	}
	if v, ok := body["unbillable"].(bool); ok {
		billable := "ja"
		if v {
			billable = "nein"
		}
		lines = append(lines, line{"Abrechenbar", billable})
	}
	d := p.end - p.start // wall-clock minutes, the booked duration
	head := []line{
		{"Datum", p.date},
		{"Zeit", fmt.Sprintf("%s–%s (%d:%02d h)", hhmm(p.start), hhmm(p.end), d/60, d%60)},
	}
	var b strings.Builder
	b.WriteString("Neuer Zeiteintrag:\n")
	for _, l := range append(head, lines...) {
		fmt.Fprintf(&b, "  %-12s %s\n", l.label+":", l.value)
	}
	_, _ = fmt.Fprint(a.Err, b.String())
	answer, eof, err := a.ask(ctx, "Zeiteintrag anlegen? [J/n]: ")
	if err != nil {
		return err
	}
	if eof {
		// Enter is yes, but the end of the input (Ctrl-D) is no (review M2)
		_, _ = fmt.Fprintln(a.Err)
		return clierr.New(clierr.CodeConfirmation, "Abgebrochen: Eingabe beendet", "Nichts gesendet. Ohne Rückfrage mit --yes")
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "j", "ja", "y", "yes":
		return nil
	}
	return clierr.New(clierr.CodeConfirmation, "Abgebrochen durch Nutzer", "Nichts gesendet. Ohne Rückfrage mit --yes")
}
