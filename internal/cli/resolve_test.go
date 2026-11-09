package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/namecache"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// nameRef is the table entry of flag; the test fails without it.
func nameRef(t *testing.T, flag string) registry.NameRef {
	t.Helper()
	ref, ok := registry.NameRefOf(flag)
	if !ok {
		t.Fatalf("keine Namensauflösung für --%s", flag)
	}
	return ref
}

// pagedList answers a list request with page "page" of pages (1-based)
// and total_pages total; a page beyond pages is empty.
func pagedList(t *testing.T, total int, pages ...[]map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil || p < 1 {
			p = 1
		}
		entries := []map[string]any{}
		if p <= len(pages) {
			entries = pages[p-1]
		}
		b, err := json.Marshal(map[string]any{
			"type": "list", "page": p, "page_size": 100, "total_pages": total,
			"total_entries": total * 100, "has_more": p < total, "entries": entries,
		})
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}
}

// company is a list entry with id and name, active unless state says
// otherwise.
func company(id int, name string, state ...string) map[string]any {
	m := map[string]any{"id": id, "name": name, "record_state": "active"}
	if len(state) > 0 {
		m["record_state"] = state[0]
	}
	return m
}

// newResolver is the resolver of a command of app, with the client of the
// environment of app.
func newResolver(t *testing.T, app *App) *nameResolver {
	t.Helper()
	client, r, err := app.Client(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &cobra.Command{Use: "test"}
	c.SetContext(t.Context())
	return app.newNameResolver(c, client, r)
}

// stderrOf returns stderr of app and empties it.
func stderrOf(app *App) string {
	_, errs := bufs(app)
	app.Err.(interface{ Reset() }).Reset()
	return errs
}

// wantResolveErr checks the exit code, message and hint of err.
func wantResolveErr(t *testing.T, err error, exit int) *clierr.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("kein Fehler, erwartet Exit %d", exit)
	}
	ce := clierr.From(err)
	if ce.Exit() != exit {
		t.Fatalf("Exit %d (%s: %s; %s), erwartet %d", ce.Exit(), ce.Code, ce.Message, ce.Hint, exit)
	}
	return ce
}

func TestIsIDValue(t *testing.T) {
	// "+12" stays an ID like before (spec.CoerceFlag); the caller trims
	// " 12 " before parsing
	for _, s := range []string{"3214", " 12 ", "0", "-1", "+1"} {
		if !isIDValue(s) {
			t.Errorf("isIDValue(%q) = false", s)
		}
	}
	for _, s := range []string{"1und1", "Mustermann", "", "12a", "1 2", " ", "+-1", "1+", "١٢"} {
		if isIDValue(s) {
			t.Errorf("isIDValue(%q) = true", s)
		}
	}
}

func TestNameKey(t *testing.T) {
	if got := nameKey("  Mustermann \t GmbH "); got != "mustermann gmbh" {
		t.Errorf("nameKey = %q", got)
	}
	if got := nameKey("Groß\u202eAG\n"); got != "groß ag" {
		t.Errorf("nameKey mit Steuerzeichen = %q", got)
	}
}

func TestResolveUniqueMatch(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(1, "ACME"), company(2, "Mustermann GmbH")}))
	got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := Resolution{Flag: "customer", Name: "Mustermann GmbH", Resource: "company", ID: 2, Credits: 1}
	if got != want {
		t.Errorf("Resolution %+v, erwartet %+v", got, want)
	}
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Path != "/contact/companies" || reqs[0].Query.Encode() != "page=1&page_size=100" {
		t.Errorf("Request %s?%s", reqs[0].Path, reqs[0].Query.Encode())
	}
	if errs := stderrOf(app); errs != "Hinweis: Kunde „Mustermann GmbH“ → 2 (1 Credit)\n" {
		t.Errorf("stderr %q", errs)
	}
	// the JSON form of a resolution (dry run, task 3)
	b, _ := json.Marshal(got)
	if string(b) != `{"flag":"customer","name":"Mustermann GmbH","resource":"company","id":2,"credits":1,"cached":false}` {
		t.Errorf("JSON %s", b)
	}
}

func TestResolveCaseAndSpace(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(1, "ACME"), company(2, "Mustermann  GmbH")}))
	got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "  mustermann   gmbh ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 2 || got.Name != "Mustermann GmbH" {
		t.Errorf("Resolution %+v", got)
	}
	// a name of digits only stays an ID for the caller; here a name with
	// digits and letters matches as a name
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(5, "1und1")}))
	app.Now = func() time.Time { return time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC) }
	if got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "1UND1", 0); err != nil || got.ID != 5 {
		t.Errorf("1und1: %+v, %v", got, err)
	}
}

func TestResolveArchivedIgnored(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{
		company(7, "Mustermann GmbH", "archived"), company(8, "Mustermann GmbH"),
	}))
	got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	if err != nil || got.ID != 8 {
		t.Fatalf("aktiver Eintrag: %+v, %v", got, err)
	}

	app2, srv2 := apiApp(t)
	srv2.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(7, "Mustermann GmbH", "archived"), company(9, "ACME")}))
	_, err = newResolver(t, app2).resolve(nameRef(t, "customer"), "mustermann gmbh", 0)
	ce := wantResolveErr(t, err, 4)
	if ce.Code != clierr.CodeNotFound || ce.Message != "Kunde „mustermann gmbh“ nicht gefunden" {
		t.Errorf("Meldung %q (%s)", ce.Message, ce.Code)
	}
	if ce.Hint != "nur archiviert: 7 „Mustermann GmbH“; reaktivieren (papierkram company unarchive 7) oder ID angeben (1 Credit verbraucht)" {
		t.Errorf("Hinweis %q", ce.Hint)
	}
	for _, want := range []string{"archiviert", "7", "papierkram company unarchive 7"} {
		if !strings.Contains(ce.Hint, want) {
			t.Errorf("Hinweis %q ohne %q", ce.Hint, want)
		}
	}
	if errs := stderrOf(app2); errs != "" {
		t.Errorf("stderr %q", errs)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH"), company(3, "mustermann gmbh")}))
	_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	ce := wantResolveErr(t, err, 2)
	if ce.Code != clierr.CodeUsage || ce.Message != "Kunde „Mustermann GmbH“ ist mehrdeutig: 2 „Mustermann GmbH“, 3 „mustermann gmbh“" {
		t.Errorf("Meldung %q", ce.Message)
	}
	if ce.Hint != "ID statt Name angeben (1 Credit verbraucht)" {
		t.Errorf("Hinweis %q", ce.Hint)
	}
	if ce.HTTPStatus != 0 {
		t.Errorf("http_status %d", ce.HTTPStatus)
	}
}

func TestResolveNotFoundSuggests(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{
		company(1, "Mustermann Automobile GmbH"), company(2, "Musterfrau AG"), company(3, "ACME"),
	}))
	_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann", 0)
	ce := wantResolveErr(t, err, 4)
	if ce.Message != "Kunde „Mustermann“ nicht gefunden" {
		t.Errorf("Meldung %q", ce.Message)
	}
	if !strings.HasPrefix(ce.Hint, "Ähnlich: „Mustermann Automobile GmbH“ (1)") || !strings.HasSuffix(ce.Hint, "sonst ID angeben (papierkram company list) (1 Credit verbraucht)") {
		t.Errorf("Hinweis %q", ce.Hint)
	}
	if strings.Contains(ce.Hint, "ACME") {
		t.Errorf("Hinweis nennt Unähnliches: %q", ce.Hint)
	}

	// at most three: substring matches first, then close names
	app2, srv2 := apiApp(t)
	srv2.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{
		company(1, "Muster"), company(2, "Beta Muster"), company(3, "Gamma Muster"), company(4, "Must"), company(5, "Muster Delta"),
	}))
	_, err = newResolver(t, app2).resolve(nameRef(t, "customer"), "Muste", 0)
	ce = wantResolveErr(t, err, 4)
	if ce.Hint != "Ähnlich: „Muster“ (1), „Beta Muster“ (2), „Gamma Muster“ (3); sonst ID angeben (papierkram company list) (1 Credit verbraucht)" {
		t.Errorf("Hinweis %q", ce.Hint)
	}

	// nothing similar: only the way to the ID
	app3, srv3 := apiApp(t)
	srv3.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(3, "ACME")}))
	_, err = newResolver(t, app3).resolve(nameRef(t, "creditor"), "Xylophon", 0)
	ce = wantResolveErr(t, err, 4)
	if ce.Message != "Lieferant „Xylophon“ nicht gefunden" || ce.Hint != "ID angeben (papierkram company list) (1 Credit verbraucht)" {
		t.Errorf("Meldung %q, Hinweis %q", ce.Message, ce.Hint)
	}
}

func TestResolveMultiplePages(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 3,
		[]map[string]any{company(1, "A")}, []map[string]any{company(2, "B")}, []map[string]any{company(3, "Mustermann GmbH")}))
	got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 3 || got.Credits != 3 {
		t.Errorf("Resolution %+v", got)
	}
	reqs := wantRequests(t, srv, 3)
	for i, req := range reqs {
		if want := fmt.Sprintf("page=%d&page_size=100", i+1); req.Query.Encode() != want {
			t.Errorf("Request %d: %s, erwartet %s", i+1, req.Query.Encode(), want)
		}
	}
	if errs := stderrOf(app); errs != "Hinweis: Kunde „Mustermann GmbH“ → 3 (3 Credits)\n" {
		t.Errorf("stderr %q", errs)
	}
}

func TestResolveIncompleteList(t *testing.T) {
	app, srv := apiApp(t)
	pages := make([][]map[string]any, 6)
	for i := range pages {
		pages[i] = []map[string]any{company(100+i, fmt.Sprintf("Firma %d", i))}
	}
	pages[0] = append(pages[0], company(2, "Mustermann GmbH"))
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 6, pages...))
	_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	ce := wantResolveErr(t, err, 2)
	if !strings.Contains(ce.Message, "mehr als 500 Einträge") {
		t.Errorf("Meldung %q", ce.Message)
	}
	// page 1 shows total_pages 6: no further page (ruling T2-A)
	if ce.Hint != "ID statt Name angeben (papierkram company list) (1 Credit verbraucht)" {
		t.Errorf("Hinweis %q", ce.Hint)
	}
	wantRequests(t, srv, 1)
	// no cache of an incomplete list
	if _, err := os.Stat(completionCachePath(app.CacheDir, "", "company")); !os.IsNotExist(err) {
		t.Errorf("Cache-Datei geschrieben: %v", err)
	}

	// the list grows while loading: page 2 shows total_pages 6
	app2, srv2 := apiApp(t)
	srv2.OnFunc("GET", "/contact/companies", func(w http.ResponseWriter, r *http.Request) {
		total := 2
		if r.URL.Query().Get("page") == "2" {
			total = 6
		}
		b, _ := json.Marshal(map[string]any{"type": "list", "page": 1, "page_size": 100, "total_pages": total,
			"total_entries": total * 100, "has_more": true, "entries": []map[string]any{company(2, "Mustermann GmbH")}})
		_, _ = w.Write(b)
	})
	_, err = newResolver(t, app2).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	ce = wantResolveErr(t, err, 2)
	if !strings.Contains(ce.Message, "mehr als 500 Einträge (6 Seiten)") || !strings.HasSuffix(ce.Hint, "(2 Credits verbraucht)") {
		t.Errorf("Meldung %q, Hinweis %q", ce.Message, ce.Hint)
	}
	wantRequests(t, srv2, 2)
	if _, err := os.Stat(completionCachePath(app2.CacheDir, "", "company")); !os.IsNotExist(err) {
		t.Errorf("Cache-Datei geschrieben: %v", err)
	}
}

func TestResolveCacheHit(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(1, "ACME"), company(2, "Mustermann GmbH")}))
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	app.Now = func() time.Time { return now }
	n := newResolver(t, app)
	if _, err := n.resolve(nameRef(t, "customer"), "Mustermann GmbH", 0); err != nil {
		t.Fatal(err)
	}
	stderrOf(app)
	now = now.Add(namecache.TTL - time.Second)
	got, err := n.resolve(nameRef(t, "customer"), "mustermann gmbh", 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Resolution{Flag: "customer", Name: "Mustermann GmbH", Resource: "company", ID: 2, Cached: true}); got != want {
		t.Errorf("Resolution %+v, erwartet %+v", got, want)
	}
	wantRequests(t, srv, 1)
	if errs := stderrOf(app); errs != "Hinweis: Kunde „Mustermann GmbH“ → 2 (aus dem Zwischenspeicher, 0 Credits)\n" {
		t.Errorf("stderr %q", errs)
	}
	// the cache is shared by all flags of the resource
	if got, err := n.resolve(nameRef(t, "creditor"), "ACME", 0); err != nil || got.ID != 1 || !got.Cached {
		t.Errorf("creditor: %+v, %v", got, err)
	}
	wantRequests(t, srv, 1)
	// only a unique hit comes from the cache: a miss loads the list (an
	// object created a moment ago is found, an error rests on fresh data)
	_, err = n.resolve(nameRef(t, "customer"), "Unbekannt", 0)
	_ = wantResolveErr(t, err, 4)
	wantRequests(t, srv, 2)

	// stored 61 s ago: loaded again
	now = now.Add(namecache.TTL + time.Second)
	if got, err := n.resolve(nameRef(t, "customer"), "Mustermann GmbH", 0); err != nil || got.Cached || got.Credits != 1 {
		t.Errorf("nach TTL: %+v, %v", got, err)
	}
	wantRequests(t, srv, 3)
}

// writeCacheFile writes a fresh completion cache file of resource for the
// direct mode of srv with the raw JSON fields extra (e.g. `"complete":true`).
func writeCacheFile(t *testing.T, app *App, srv *mockapi.Server, resource, entries, extra string) string {
	t.Helper()
	path := completionCachePath(app.CacheDir, "", resource)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	stored, _ := json.Marshal(app.Now())
	content := fmt.Sprintf(`{"scope":%q,"resource":%q,"stored":%s,"entries":%s%s}`, " "+srv.URL(), resource, stored, entries, extra)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The completion stores a list of more than one page as incomplete; the
// resolver does not trust it and loads.
func TestResolveCacheIncompleteIgnored(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 2,
		[]map[string]any{company(1, "ACME")}, []map[string]any{company(2, "Mustermann GmbH")}))
	c := &cobra.Command{Use: "test"}
	if got := app.completeIDs(c, "company", ""); len(got) != 1 || got[0] != "1\tACME" {
		t.Fatalf("Vervollständigung %q", got)
	}
	wantRequests(t, srv, 1)
	data, err := os.ReadFile(completionCachePath(app.CacheDir, "", "company"))
	if err != nil || strings.Contains(string(data), `"complete"`) {
		t.Fatalf("Cache %s, %v", data, err)
	}
	got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
	if err != nil || got.ID != 2 || got.Cached || got.Credits != 2 {
		t.Errorf("Resolution %+v, %v", got, err)
	}
	wantRequests(t, srv, 3)
	// a hand-written "complete" cache without entries field is a miss too
	writeCacheFile(t, app, srv, "company", "null", `,"complete":true`)
	if _, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0); err != nil {
		t.Fatal(err)
	}
	wantRequests(t, srv, 5)
}

// The resolver writes the cache of the completion: the IDs complete
// without a request, names untruncated in the file, archived marked.
func TestResolveWritesCacheForCompletion(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{
		company(1, "ACME"), company(2, "Mustermann GmbH"), company(3, "Alt", "archived"),
	}))
	if _, err := newResolver(t, app).resolve(nameRef(t, "customer"), "ACME", 0); err != nil {
		t.Fatal(err)
	}
	c := &cobra.Command{Use: "test"}
	got := app.completeIDs(c, "company", "")
	if want := []string{"1\tACME", "2\tMustermann GmbH", "3\tAlt"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Vervollständigung %q, erwartet %q", got, want)
	}
	wantRequests(t, srv, 1)
	data, err := os.ReadFile(completionCachePath(app.CacheDir, "", "company"))
	if err != nil {
		t.Fatal(err)
	}
	var f completionCache
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if !f.Complete || len(f.Entries) != 3 || !f.Entries[2].Archived || f.Entries[0].Archived {
		t.Errorf("Cache %s", data)
	}
}

func TestResolvePersonByParent(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies/2843/persons", pagedList(t, 1, []map[string]any{
		{"id": 11, "first_name": "Anna", "last_name": "Groß"},
		{"id": 12, "first_name": "Ben", "last_name": "Groß"},
	}))
	n := newResolver(t, app)
	got, err := n.resolve(nameRef(t, "contact-person"), "anna  groß", 2843)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Resolution{Flag: "contact-person", Name: "Anna Groß", Resource: "person", ID: 11, Credits: 1}); got != want {
		t.Errorf("Resolution %+v, erwartet %+v", got, want)
	}
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Path != "/contact/companies/2843/persons" || reqs[0].Query.Encode() != "page=1&page_size=100" {
		t.Errorf("Request %s?%s", reqs[0].Path, reqs[0].Query.Encode())
	}
	if errs := stderrOf(app); errs != "Hinweis: Kontaktperson „Anna Groß“ → 11 (1 Credit)\n" {
		t.Errorf("stderr %q", errs)
	}
	// persons are never cached: a second call loads again
	if entries, _ := os.ReadDir(filepath.Join(app.CacheDir, "papierkram")); len(entries) != 0 {
		t.Errorf("Cache-Dateien: %v", entries)
	}
	if _, err := n.resolve(nameRef(t, "contact-person"), "Anna Groß", 2843); err != nil {
		t.Fatal(err)
	}
	wantRequests(t, srv, 2)
	// not found names the list of the company
	_, err = n.resolve(nameRef(t, "contact-person"), "Carla", 2843)
	if ce := wantResolveErr(t, err, 4); !strings.HasSuffix(ce.Hint, "ID angeben (papierkram person list --company 2843) (1 Credit verbraucht)") {
		t.Errorf("Hinweis %q", ce.Hint)
	}
	// without parent: a programming error, no request
	_, err = n.resolve(nameRef(t, "contact-person"), "Anna Groß", 0)
	if ce := wantResolveErr(t, err, 1); ce.Code != clierr.CodeError {
		t.Errorf("Code %s", ce.Code)
	}
	wantRequests(t, srv, 3)
}

func TestResolveUserName(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/users", pagedList(t, 1, []map[string]any{
		{"id": 4, "first_name": "Erika", "last_name": "Mustermann"},
		{"id": 5, "first_name": "Max", "last_name": "Mustermann"},
	}))
	got, err := newResolver(t, app).resolve(nameRef(t, "team-member"), "Max Mustermann", 0)
	if err != nil || got.ID != 5 || got.Resource != "user" {
		t.Errorf("Resolution %+v, %v", got, err)
	}
	if errs := stderrOf(app); errs != "Hinweis: Benutzer „Max Mustermann“ → 5 (1 Credit)\n" {
		t.Errorf("stderr %q", errs)
	}
}

// Errors of the list request pass through with their code.
func TestResolveErrorsPassThrough(t *testing.T) {
	for _, tc := range []struct {
		status int
		exit   int
	}{{http.StatusTooManyRequests, 6}, {http.StatusUnauthorized, 3}} {
		app, srv := apiApp(t)
		srv.On("GET", "/contact/companies", tc.status, []byte(`{"error":"x"}`))
		_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "ACME", 0)
		_ = wantResolveErr(t, err, tc.exit)
		wantRequests(t, srv, 1)
		if _, err := os.Stat(completionCachePath(app.CacheDir, "", "company")); !os.IsNotExist(err) {
			t.Errorf("%d: Cache geschrieben", tc.status)
		}
	}
	app, _, _ := newTestApp(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": "http://127.0.0.1:1/api/v1", "PAPIERKRAM_TOKEN": "test-token-net"})
	_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "ACME", 0)
	_ = wantResolveErr(t, err, 10)
	assertNoToken(t, app)
}

// Without a cache directory the resolver neither reads nor writes a cache.
func TestResolveWithoutCacheDir(t *testing.T) {
	app, srv := apiApp(t)
	app.CacheDir = ""
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH")}))
	n := newResolver(t, app)
	for range 2 {
		if got, err := n.resolve(nameRef(t, "customer"), "Mustermann GmbH", 0); err != nil || got.Cached {
			t.Errorf("Resolution %+v, %v", got, err)
		}
	}
	wantRequests(t, srv, 2)
	if wd, _ := app.Getwd(); wd != "" {
		if entries, _ := os.ReadDir(wd); len(entries) != 0 {
			t.Errorf("Dateien im Arbeitsverzeichnis: %v", entries)
		}
	}
}

// The cache is per context and account: a cache of another base URL is
// not used.
func TestResolveCacheScope(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH")}))
	other := mockapi.New(t)
	path := writeCacheFile(t, app, other, "company", `[{"id":9,"name":"Mustermann GmbH"}]`, `,"complete":true`)
	if got, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0); err != nil || got.ID != 2 {
		t.Errorf("Resolution %+v, %v", got, err)
	}
	wantRequests(t, srv, 1)
	// the resolver replaced the file with the list of this account
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), srv.URL()) {
		t.Errorf("Cache %s, %v", data, err)
	}
}

// A list answer without a valid total_pages is unexpected (exit 1, like
// list --all); the credit of the page is named, no cache is written.
func TestResolveInvalidTotalPages(t *testing.T) {
	for _, total := range []string{``, `,"total_pages":null`, `,"total_pages":"x"`, `,"total_pages":-1`} {
		app, srv := apiApp(t)
		srv.On("GET", "/contact/companies", http.StatusOK,
			[]byte(`{"type":"list","page":1,"page_size":100`+total+`,"entries":[{"id":2,"name":"Mustermann GmbH"}]}`))
		_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "Mustermann GmbH", 0)
		ce := wantResolveErr(t, err, 1)
		if ce.Code != clierr.CodeError || !strings.Contains(ce.Message, "total_pages") || ce.Hint != "(1 Credit verbraucht)" {
			t.Errorf("%q: %s %q, Hinweis %q", total, ce.Code, ce.Message, ce.Hint)
		}
		wantRequests(t, srv, 1)
		if _, err := os.Stat(completionCachePath(app.CacheDir, "", "company")); !os.IsNotExist(err) {
			t.Errorf("%q: Cache geschrieben", total)
		}
	}
}

// A read error after loaded pages names their credits; its code stays.
func TestResolveReadErrorNamesCredits(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "3" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"quota"}`))
			return
		}
		pagedList(t, 3, []map[string]any{company(1, "A")}, []map[string]any{company(2, "B")})(w, r)
	})
	_, err := newResolver(t, app).resolve(nameRef(t, "customer"), "C", 0)
	ce := wantResolveErr(t, err, 6)
	if !strings.HasSuffix(ce.Hint, "(2 Credits verbraucht)") || ce.HTTPStatus != http.StatusTooManyRequests {
		t.Errorf("Hinweis %q, http_status %d", ce.Hint, ce.HTTPStatus)
	}
	wantRequests(t, srv, 3)
}
