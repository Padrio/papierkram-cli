package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Berlin on every CI platform

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// apiApp is a test app talking to a fresh mock with an environment token.
func apiApp(t *testing.T) (*App, *mockapi.Server) {
	t.Helper()
	return mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-builder"})
}

// specEntry builds a registry entry for a spec operation that is not (yet)
// registered, so the runners can be tested with it directly.
func specEntry(t *testing.T, opID, resource, action string, class registry.Class, filters ...registry.Filter) registry.Entry {
	t.Helper()
	op, ok := spec.OperationByID(opID)
	if !ok {
		t.Fatalf("operation %s not in spec", opID)
	}
	return registry.Entry{
		Resource: resource, Action: action, OperationID: opID,
		Method: op.Method, Path: op.Path, Class: class,
		Credits: registry.ExpectedCredits(op.Method, op.Path),
		Filters: filters, Short: "Test " + opID,
	}
}

// runEntries runs args against the real root plus commands built by the
// builder for the given extra entries; an extra entry replaces a registered
// command of the same name, so the test runs exactly the entry it built.
func runEntries(t *testing.T, app *App, ctx context.Context, args []string, entries ...registry.Entry) int {
	t.Helper()
	root := NewRootCmd(app)
	for _, e := range entries {
		group, _, err := root.Find([]string{e.Resource})
		if err != nil || group == root {
			group = &cobra.Command{Use: e.Resource}
			root.AddCommand(group)
		}
		for _, c := range group.Commands() {
			if c.Name() == e.Action {
				group.RemoveCommand(c)
			}
		}
		group.AddCommand(app.newEntryCmd(e))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return executeContext(ctx, app, root, args)
}

func wantExit(t *testing.T, app *App, got, want int) {
	t.Helper()
	if got != want {
		out, errs := bufs(app)
		t.Fatalf("exit = %d, erwartet %d\nstdout: %s\nstderr: %s", got, want, out, errs)
	}
}

func wantRequests(t *testing.T, srv *mockapi.Server, n int) []mockapi.Recorded {
	t.Helper()
	reqs := srv.Requests()
	if len(reqs) != n {
		t.Fatalf("%d Requests, erwartet %d: %+v", len(reqs), n, reqs)
	}
	return reqs
}

func TestR03_EmptyBodyActionIsSuccess(t *testing.T) {
	archive := specEntry(t, "ArchiveCompany", "company", "archive", registry.ClassArchive)
	app, srv := apiApp(t)
	srv.On("POST", "/contact/companies/{id}/archive", http.StatusOK, nil)
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "archive", "12", "-o", "json"}, archive), 0)
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Method != "POST" || reqs[0].Path != "/contact/companies/12/archive" {
		t.Errorf("request = %s %s", reqs[0].Method, reqs[0].Path)
	}
	if out, errs := bufs(app); out != "{\n  \"id\": 12,\n  \"ok\": true\n}\n" || errs != "" {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}

	// table without --fields: a German line instead of a FELD/WERT table
	app, srv = apiApp(t)
	srv.On("POST", "/contact/companies/{id}/archive", http.StatusOK, nil)
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "archive", "12"}, archive), 0)
	if out, _ := bufs(app); out != "company 12 archiviert.\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestActionWithBodyRendersAPIObject(t *testing.T) {
	archive := specEntry(t, "ArchiveCompany", "company", "archive", registry.ClassArchive)
	app, srv := apiApp(t)
	srv.On("POST", "/contact/companies/12/archive", http.StatusOK, []byte(`{"id":12,"name":"Müller & Söhne","total":1190.00}`))
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "archive", "12", "-o", "json"}, archive), 0)
	want := "{\n  \"id\": 12,\n  \"name\": \"Müller & Söhne\",\n  \"total\": 1190.00\n}\n"
	if out, _ := bufs(app); out != want {
		t.Errorf("stdout = %q, erwartet %q", out, want)
	}
}

func TestDeleteOutputsDeletedObject(t *testing.T) {
	del := specEntry(t, "DeleteCompany", "company", "delete", registry.ClassDestructive)
	app, srv := apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "delete", "0", "-o", "json", "--yes"}, del), 0)
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Method != "DELETE" || reqs[0].Path != "/contact/companies/0" {
		t.Errorf("request = %s %s", reqs[0].Method, reqs[0].Path)
	}
	if out, _ := bufs(app); out != "{\n  \"id\": 0,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	app, _ = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "delete", "7", "--yes"}, del), 0)
	if out, _ := bufs(app); out != "company 7 gelöscht.\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestActionIDValidation(t *testing.T) {
	archive := specEntry(t, "ArchiveCompany", "company", "archive", registry.ClassArchive)
	for _, args := range [][]string{
		{"company", "archive"},
		{"company", "archive", "abc"},
		{"company", "archive", "1.5"},
		{"company", "archive", "-1"},
		{"company", "archive", "--", "-1"},
		{"company", "archive", "99999999999999999999"},
		{"company", "archive", "1", "2"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runEntries(t, app, nil, args, archive), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "Fehler: ") || strings.Contains(errs, "[flags]") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
}

func TestR20_SortAlwaysSendsDirection(t *testing.T) {
	for _, tc := range []struct {
		args []string
		dir  string
	}{
		{[]string{"user", "list", "--sort", "name"}, "asc"},
		{[]string{"user", "list", "--sort", "name", "--desc"}, "desc"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), 0)
		q := wantRequests(t, srv, 1)[0].Query
		if q.Get("order_by") != "name" || q.Get("order_direction") != tc.dir {
			t.Errorf("%v: query = %v", tc.args, q)
		}
	}
	for _, args := range [][]string{
		{"user", "list", "--desc"},
		{"user", "list", "--sort", ""},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
}

func TestR24_IDZeroIsSent(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"user", "describe", "0", "-o", "json"}), 0)
	if p := wantRequests(t, srv, 1)[0].Path; p != "/users/0" {
		t.Errorf("path = %q", p)
	}
}

func TestDescribeIDValidation(t *testing.T) {
	for _, args := range [][]string{
		{"user", "describe"},
		{"user", "describe", "x"},
		{"user", "describe", "--", "-3"},
		{"user", "describe", "1", "2"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
}

func timeListEntry(t *testing.T) registry.Entry {
	return specEntry(t, "ListTimeEntries", "time", "list", registry.ClassReadOnly,
		registry.Filter{Flag: "state", Param: "billing_state", Kind: "enum", Help: "Abrechnungsstatus"},
		registry.Filter{Flag: "since", Param: "start_time_range_start", Kind: "datetime_from", Help: "Beginn ab"},
		registry.Filter{Flag: "until", Param: "start_time_range_end", Kind: "datetime_to", Help: "Beginn bis"},
		registry.Filter{Flag: "user", Param: "user_id", Kind: "id", Help: "Benutzer"},
	)
}

func TestR30_UnknownBillingStateRejectedLocally(t *testing.T) {
	list := timeListEntry(t)
	app, srv := apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"time", "list", "--state", "billd"}, list), 5)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	for _, want := range []string{`ungültiger Wert "billd" für --state`, `meintest du "billed"`, "billed, unbilled, billable, unbillable, archived"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr enthält %q nicht:\n%s", want, errs)
		}
	}

	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"time", "list", "--state", "billed"}, list), 0)
	if q := wantRequests(t, srv, 1)[0].Query; q.Get("billing_state") != "billed" {
		t.Errorf("query = %v", q)
	}
}

func berlinApp(t *testing.T) (*App, *mockapi.Server) {
	t.Helper()
	app, srv := apiApp(t)
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	app.Location = loc
	app.Now = func() time.Time { return time.Date(2026, 10, 26, 0, 30, 0, 0, loc) }
	return app, srv
}

// Review Focus 4: date-only bounds are local midnight and 23:59:59, with
// the offset valid on that day (DST ends on 2026-10-25 in Berlin).
func TestListDateTimeFiltersUseLocalZone(t *testing.T) {
	list := timeListEntry(t)
	for _, tc := range []struct {
		args       []string
		start, end string
	}{
		{[]string{"--since", "2026-10-25", "--until", "2026-10-25"}, "2026-10-25T00:00:00+02:00", "2026-10-25T23:59:59+01:00"},
		{[]string{"--since", "gestern", "--until", "heute"}, "2026-10-25T00:00:00+02:00", "2026-10-26T23:59:59+01:00"},
		{[]string{"--since", "2026-10-25T08:00:00Z"}, "2026-10-25T08:00:00Z", ""},
	} {
		app, srv := berlinApp(t)
		wantExit(t, app, runEntries(t, app, nil, append([]string{"time", "list"}, tc.args...), list), 0)
		q := wantRequests(t, srv, 1)[0].Query
		if q.Get("start_time_range_start") != tc.start || q.Get("start_time_range_end") != tc.end {
			t.Errorf("%v: query = %v", tc.args, q)
		}
	}
	for _, bad := range []string{"25.10.2026", "2026-13-01", "morgen"} {
		app, srv := berlinApp(t)
		wantExit(t, app, runEntries(t, app, nil, []string{"time", "list", "--since", bad}, list), 5)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--since") || !strings.Contains(errs, "YYYY-MM-DD") {
			t.Errorf("%s: stderr = %q", bad, errs)
		}
	}
}

func TestListDateFilter(t *testing.T) {
	list := specEntry(t, "ListInvoices", "invoice", "list", registry.ClassReadOnly,
		registry.Filter{Flag: "from", Param: "document_date_range_start", Kind: "date", Help: "ab"},
		registry.Filter{Flag: "to", Param: "document_date_range_end", Kind: "date", Help: "bis"},
	)
	app, srv := berlinApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "list", "--from", "gestern", "--to", "2026-12-31"}, list), 0)
	q := wantRequests(t, srv, 1)[0].Query
	if q.Get("document_date_range_start") != "2026-10-25" || q.Get("document_date_range_end") != "2026-12-31" {
		t.Errorf("query = %v", q)
	}
	app, srv = berlinApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "list", "--from", "1.1.2026"}, list), 5)
	wantRequests(t, srv, 0)
}

func TestListIDFilter(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"transaction", "list", "--bank-connection", "0"}), 0)
	if q := wantRequests(t, srv, 1)[0].Query; q.Get("bank_connection_id") != "0" {
		t.Errorf("query = %v", q)
	}
	for _, bad := range []string{"-1", "abc", "1.5"} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{"transaction", "list", "--bank-connection", bad}), 2)
		wantRequests(t, srv, 0)
	}
}

func TestListDefaultAndPagingQuery(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want url.Values
	}{
		{nil, url.Values{"page_size": {"25"}}},
		{[]string{"--page", "2", "--limit", "10"}, url.Values{"page": {"2"}, "page_size": {"10"}}},
		{[]string{"--limit", "100"}, url.Values{"page_size": {"100"}}},
		{[]string{"--limit", "1"}, url.Values{"page_size": {"1"}}},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append([]string{"user", "list"}, tc.args...)), 0)
		if q := wantRequests(t, srv, 1)[0].Query; q.Encode() != tc.want.Encode() {
			t.Errorf("%v: query = %s, erwartet %s", tc.args, q.Encode(), tc.want.Encode())
		}
	}
}

func TestListFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--limit", "101"},
		{"--limit", "0"},
		{"--page", "0"},
		{"--max-pages", "0", "--all"},
		{"--max-pages", "2"},
		{"--page", "2", "--all"},
		{"--limit", "x"},
		{"extra"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append([]string{"user", "list"}, args...)), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.HasPrefix(errs, "Fehler: ") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"user", "list", "--limit", "101"}), 2)
	if _, errs := bufs(app); !strings.Contains(errs, "--limit") || !strings.Contains(errs, "1–100") {
		t.Errorf("stderr = %q", errs)
	}
}

// pagedUsers answers GET /users with total pages of one user each and
// echoes the requested page size (25 without one, the CLI default); ids of
// page p are 9007199254740990+p, so exact merging is visible. The mock
// has validated page and page_size as integers already.
func pagedUsers(srv *mockapi.Server, total int) {
	srv.OnFunc("GET", "/users", func(w http.ResponseWriter, r *http.Request) {
		n, size := 1, 25
		if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
			n = p
		}
		if s, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
			size = s
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"type":"list","page":%d,"page_size":%d,"total_pages":%d,"total_entries":%d,"has_more":%t,`+
			`"entries":[{"id":%d,"full_name":"Nutzer <%d> & Co","email":"u%d@example.org","role":"owner","rate":19.90}]}`,
			n, size, total, total, n < total, int64(9007199254740990)+int64(n), n, n)
	})
}

// L202: the pagedUsers mock answers valid JSON with a numeric page_size,
// also for a request without page_size (25, the CLI default).
func TestPagedUsersMockValidJSON(t *testing.T) {
	_, srv := apiApp(t)
	pagedUsers(srv, 2)
	for query, size := range map[string]float64{"": 25, "?page=2": 25, "?page=1&page_size=50": 50} {
		req, err := http.NewRequest(http.MethodGet, srv.URL()+"/users"+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer test-token-builder")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			PageSize *float64 `json:"page_size"`
		}
		if err := json.Unmarshal(body, &page); err != nil || page.PageSize == nil || *page.PageSize != size {
			t.Errorf("%q: page_size %v (%v), want %v:\n%s", query, page.PageSize, err, size, body)
		}
	}
}

func TestListAllMergesPagesExactly(t *testing.T) {
	app, srv := apiApp(t)
	pagedUsers(srv, 3)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 3)
	for i, r := range reqs {
		if r.Query.Get("page") != fmt.Sprint(i+1) || r.Query.Get("page_size") != "100" {
			t.Errorf("request %d: query = %v", i, r.Query)
		}
	}
	golden.Assert(t, "builder/user-list-all.json", []byte(mustOut(app)))
	if _, errs := bufs(app); errs != "" {
		t.Errorf("stderr = %q", errs)
	}

	// --limit sets the page size of --all
	app, srv = apiApp(t)
	pagedUsers(srv, 2)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "--limit", "50", "-o", "json"}), 0)
	for _, r := range wantRequests(t, srv, 2) {
		if r.Query.Get("page_size") != "50" {
			t.Errorf("query = %v", r.Query)
		}
	}
	if out := mustOut(app); !strings.Contains(out, `"page_size": 50,`) || !strings.Contains(out, `"has_more": false`) {
		t.Errorf("stdout = %s", out)
	}
}

func mustOut(app *App) string {
	out, _ := bufs(app)
	return out
}

func TestListAllWarnsFromTenPages(t *testing.T) {
	// without TTY: warning, no prompt, all pages
	app, srv := apiApp(t)
	pagedUsers(srv, 10)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "-o", "json"}), 0)
	wantRequests(t, srv, 10)
	_, errs := bufs(app)
	if !strings.Contains(errs, "Warnung: --all lädt 10 Seiten (10 Credits)") || strings.Contains(errs, "Fortfahren") {
		t.Errorf("stderr = %q", errs)
	}

	// 9 pages: no warning
	app, srv = apiApp(t)
	pagedUsers(srv, 9)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "-o", "json"}), 0)
	wantRequests(t, srv, 9)
	if _, errs := bufs(app); errs != "" {
		t.Errorf("stderr = %q", errs)
	}

	// TTY, declined: only page 1 loaded, exit 8
	for _, answer := range []string{"n\n", "\n", ""} {
		app, srv = apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		pagedUsers(srv, 12)
		wantExit(t, app, Run(app, []string{"user", "list", "--all"}), 8)
		wantRequests(t, srv, 1)
		out, errs := bufs(app)
		if out != "" || !strings.Contains(errs, "Fortfahren? [j/N] ") {
			t.Errorf("%q: stdout = %q, stderr = %q", answer, out, errs)
		}
	}

	// TTY, confirmed
	for _, answer := range []string{"j\n", "ja\r\n", "Y\n", "yes\n"} {
		app, srv = apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		pagedUsers(srv, 12)
		wantExit(t, app, Run(app, []string{"user", "list", "--all", "-o", "json"}), 0)
		wantRequests(t, srv, 12)
	}

	// TTY with --yes or --no-input: no prompt
	for _, flag := range []string{"--yes", "--no-input"} {
		app, srv = apiApp(t)
		app.StdinTTY = true
		pagedUsers(srv, 12)
		wantExit(t, app, Run(app, []string{"user", "list", "--all", "-o", "json", flag}), 0)
		wantRequests(t, srv, 12)
		if _, errs := bufs(app); strings.Contains(errs, "Fortfahren") || !strings.Contains(errs, "12 Seiten") {
			t.Errorf("%s: stderr = %q", flag, errs)
		}
	}
}

func TestListAllMaxPagesCaps(t *testing.T) {
	app, srv := apiApp(t)
	pagedUsers(srv, 15)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "--max-pages", "2", "-o", "json"}), 0)
	wantRequests(t, srv, 2)
	out, errs := bufs(app)
	if !strings.Contains(out, `"has_more": true`) || !strings.Contains(out, `"total_pages": 15`) || strings.Count(out, `"id":`) != 2 {
		t.Errorf("stdout = %s", out)
	}
	// capped at 2 pages: no 10-page warning, but a cap warning
	if strings.Contains(errs, "15 Seiten (") || !strings.Contains(errs, "Warnung: --max-pages 2: 2 von 15 Seiten geladen; weitere Einträge vorhanden") ||
		strings.Contains(errs, "Ergebnis unvollständig") {
		t.Errorf("stderr = %q", errs)
	}

	// cap above the total changes nothing
	app, srv = apiApp(t)
	pagedUsers(srv, 3)
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "--max-pages", "5", "-o", "json"}), 0)
	wantRequests(t, srv, 3)
	if out, errs := bufs(app); !strings.Contains(out, `"has_more": false`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
}

func TestListAllEmptyList(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/users", http.StatusOK, []byte(`{"type":"list","page":1,"page_size":100,"total_pages":0,"total_entries":0,"has_more":false,"entries":[]}`))
	// page_size and the totals come from the API envelope, not the request
	wantExit(t, app, Run(app, []string{"user", "list", "--all", "--limit", "30", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	want := "{\n  \"type\": \"list\",\n  \"page\": 1,\n  \"page_size\": 100,\n  \"total_pages\": 0,\n  \"total_entries\": 0,\n  \"has_more\": false,\n  \"entries\": []\n}\n"
	if out := mustOut(app); out != want {
		t.Errorf("stdout = %q", out)
	}
}

func TestListAllInvalidEnvelopeIsNotUsageError(t *testing.T) {
	for _, body := range []string{
		`{"type":"list","entries":"kaputt"}`,
		// without total_pages --all must not claim a complete list
		`{"type":"list","page":1,"page_size":100,"total_pages":null,"total_entries":250,"has_more":true,"entries":[{"id":1}]}`,
		`{"type":"list","page":1,"page_size":100,"total_entries":250,"has_more":true,"entries":[{"id":1}]}`,
	} {
		app, srv := apiApp(t)
		srv.On("GET", "/users", http.StatusOK, []byte(body))
		wantExit(t, app, Run(app, []string{"user", "list", "--all"}), 1)
		wantRequests(t, srv, 1)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, "unerwartete Listenantwort") {
			t.Errorf("%s: stdout = %q, stderr = %q", body, out, errs)
		}
	}
}

// blockingReader blocks every Read until the test ends; the first Read
// calls onRead (e.g. to simulate Ctrl+C while the prompt waits).
type blockingReader struct {
	onRead func()
	done   chan struct{}
}

func (b *blockingReader) Read([]byte) (int, error) {
	if b.onRead != nil {
		b.onRead()
		b.onRead = nil
	}
	<-b.done
	return 0, io.EOF
}

func newBlockingReader(t *testing.T, onRead func()) *blockingReader {
	b := &blockingReader{onRead: onRead, done: make(chan struct{})}
	t.Cleanup(func() { close(b.done) })
	return b
}

// wantInterruptedPrompt checks the error of a prompt ended by Ctrl+C
// (task 17): exit 8, confirmation_required, the hint of the prompt.
func wantInterruptedPrompt(t *testing.T, app *App, hint string) {
	t.Helper()
	_, errs := bufs(app)
	lines := strings.Split(strings.TrimSpace(errs), "\n")
	var e struct {
		Error struct {
			Code     string `json:"code"`
			Message  string `json:"message"`
			Hint     string `json:"hint"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatalf("kein JSON-Fehler: %v\n%s", err, errs)
	}
	if e.Error.Code != "confirmation_required" || e.Error.ExitCode != 8 ||
		e.Error.Message != "Abgebrochen: Eingabe durch Unterbrechung beendet" || e.Error.Hint != hint {
		t.Errorf("Fehler = %+v", e.Error)
	}
}

func waitExit(t *testing.T, run func() int) int {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- run() }()
	select {
	case code := <-done:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("Unterbrechung beendet die Eingabe nicht")
		return -1
	}
}

func TestListAllPromptInterrupted(t *testing.T) {
	app, srv := apiApp(t)
	pagedUsers(srv, 12)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.StdinTTY = true
	app.In = newBlockingReader(t, cancel)
	restored := false
	app.restoreTerminal = func() { restored = true }
	code := waitExit(t, func() int { return runEntries(t, app, ctx, []string{"user", "list", "--all", "-o", "json"}) })
	wantExit(t, app, code, 8)
	wantRequests(t, srv, 1)
	out, errs := bufs(app)
	if out != "" || !strings.Contains(errs, "Fortfahren? [j/N] \nFehler: Abgebrochen: Eingabe durch Unterbrechung beendet\nHinweis: Nichts gesendet\n") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
	wantInterruptedPrompt(t, app, "Nichts gesendet")
	if !restored {
		t.Error("Terminalzustand nicht wiederhergestellt")
	}
}

// An interrupt after a mutation was sent keeps outcome_unknown (exit 12)
// with its hint; the rewrite to "Abgebrochen" only applies to exit 10.
func TestMutationInterruptedAfterSendIsOutcomeUnknown(t *testing.T) {
	archive := specEntry(t, "ArchiveCompany", "company", "archive", registry.ClassArchive)
	for _, viaTimeout := range []bool{false, true} {
		app, srv := apiApp(t)
		ctx, cancel := context.WithCancel(context.Background())
		srv.OnFunc("POST", "/contact/companies/{id}/archive", func(_ http.ResponseWriter, r *http.Request) {
			if !viaTimeout {
				cancel()
			}
			<-r.Context().Done()
		})
		args := []string{"company", "archive", "12"}
		if viaTimeout {
			args = append(args, "--timeout", "50ms")
		}
		code := waitExit(t, func() int { return runEntries(t, app, ctx, args, archive) })
		cancel()
		wantExit(t, app, code, 12)
		wantRequests(t, srv, 1)
		if _, errs := bufs(app); !strings.Contains(errs, "Ergebnis unbekannt") || strings.Contains(errs, "Abgebrochen") {
			t.Errorf("timeout=%t: stderr = %q", viaTimeout, errs)
		}
	}
}

func TestListInterruptAbortsRequest(t *testing.T) {
	app, srv := apiApp(t)
	started := make(chan struct{})
	srv.OnFunc("GET", "/users", func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	done := make(chan int)
	go func() { done <- runEntries(t, app, ctx, []string{"user", "list"}) }()
	select {
	case code := <-done:
		wantExit(t, app, code, 10)
	case <-time.After(10 * time.Second):
		t.Fatal("Abbruch beendet die Anfrage nicht")
	}
	// an interrupt is no network failure: no hint to check the
	// subdomain and connection
	if _, errs := bufs(app); !strings.Contains(errs, "Fehler: Abgebrochen") || strings.Contains(errs, "Subdomain") || strings.Contains(errs, "Hinweis:") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestResourceGroupsAndAliases(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"benutzer", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)

	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"user"}), 0)
	if out := mustOut(app); !strings.Contains(out, "describe") || !strings.Contains(out, "list") {
		t.Errorf("Gruppenhilfe = %s", out)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"user", "bogus"}), 2)
	wantRequests(t, srv, 0)

	// a resource without entries gets no command; built from a fixed
	// resource list so the check does not depend on which resources the
	// registry has entries for yet
	app, _ = apiApp(t)
	user, _ := registry.Lookup("user", "list")
	empty := &cobra.Command{Use: "papierkram"}
	buildResourceCommands(app, empty, []registry.Resource{
		{Name: "user", Aliases: []string{"benutzer"}, Short: "Benutzer", Noun: "Benutzer"},
		{Name: "leer", Aliases: []string{"nichts"}, Short: "ohne Einträge", Noun: "Leer"},
	}, []registry.Entry{user})
	for _, name := range []string{"leer", "nichts"} {
		if c, _, err := empty.Find([]string{name}); err == nil && c != empty {
			t.Errorf("%s ohne Registry-Einträge hat einen Befehl", name)
		}
	}
	if c, _, err := empty.Find([]string{"benutzer", "list"}); err != nil || c.Name() != "list" {
		t.Errorf("user list fehlt: %v", err)
	}

	root := NewRootCmd(app)
	if c, _, err := root.Find([]string{"info"}); err != nil || c.Name() != "info" || c.HasSubCommands() {
		t.Errorf("info ist kein eigener Befehl: %v", err)
	}
}

func TestEntryHelpGerman(t *testing.T) {
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"transaction", "list", "--help"}), 0)
	out := mustOut(app)
	for _, want := range []string{"Bankumsätze auflisten", "papierkram transaction list --bank-connection 237",
		"--bank-connection ID", "--limit Zahl", "(Standard: 25)", "--max-pages", "--sort Text", "--desc", "Credit"} {
		if !strings.Contains(out, want) {
			t.Errorf("Hilfe enthält %q nicht:\n%s", want, out)
		}
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"user", "describe", "--help"}), 0)
	if out := mustOut(app); !strings.Contains(out, "papierkram user describe <id> [Optionen]") {
		t.Errorf("Hilfe:\n%s", out)
	}
}

// Path parameters before the last one come from required flags (person
// commands, Task 25).
func TestPathParamFlags(t *testing.T) {
	get := specEntry(t, "GetPerson", "person", "describe", registry.ClassReadOnly)
	list := specEntry(t, "ListPersons", "person", "list", registry.ClassReadOnly)
	del := specEntry(t, "DeletePerson", "person", "delete", registry.ClassDestructive)

	app, srv := apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "describe", "5", "--company", "0", "-o", "json"}, get, list, del), 0)
	if p := wantRequests(t, srv, 1)[0].Path; p != "/contact/companies/0/persons/5" {
		t.Errorf("path = %q", p)
	}

	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "list", "--company", "7", "-o", "json"}, get, list, del), 0)
	if p := wantRequests(t, srv, 1)[0].Path; p != "/contact/companies/7/persons" {
		t.Errorf("path = %q", p)
	}

	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "delete", "5", "--company", "7", "-o", "json", "--yes"}, get, list, del), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/contact/companies/7/persons/5" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 5,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	for _, args := range [][]string{
		{"person", "describe", "5"},
		{"person", "list"},
		{"person", "describe", "5", "--company", "-1"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runEntries(t, app, nil, args, get, list, del), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--company") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
}

func TestAfterRequestErrorsAreNeverUsage(t *testing.T) {
	err := afterRequest(usageErr("kaputt"))
	ce := cliErr(t, err, "error")
	if ce.Exit() != 1 || !strings.HasPrefix(ce.Message, "Befehl ausgeführt, Ausgabe fehlgeschlagen") {
		t.Errorf("err = %+v", ce)
	}
	if afterRequest(nil) != nil {
		t.Error("nil bleibt nil")
	}
}

// Every error after a sent request gets the prefix, not only usage errors
// (M1): a 2xx answer that is no JSON, a YAML error, a write error on stdout.
func TestAfterRequestPrefixesEveryOutputError(t *testing.T) {
	for _, in := range []error{
		usageErr("kaputt"),
		clierr.New(clierr.CodeError, "Antwort ist kein gültiges JSON", "Hinweis bleibt"),
		errors.New("write |1: broken pipe"),
	} {
		ce := cliErr(t, afterRequest(in), "error")
		if ce.Exit() != 1 || ce.Message != "Befehl ausgeführt, Ausgabe fehlgeschlagen: "+clierr.From(in).Message {
			t.Errorf("%v: err = %+v", in, ce)
		}
		if !errors.Is(ce, in) {
			t.Errorf("%v: cause lost", in)
		}
		if h := clierr.From(in).Hint; ce.Hint != h {
			t.Errorf("%v: hint = %q, want %q", in, ce.Hint, h)
		}
		// applied twice, the prefix stays single
		if again := clierr.From(afterRequest(ce)); again.Message != ce.Message {
			t.Errorf("twice: %q", again.Message)
		}
	}
}

func TestMutationOutputFailureSaysExecuted(t *testing.T) {
	archive := specEntry(t, "ArchiveCompany", "company", "archive", registry.ClassArchive)
	// a 2xx answer that is not JSON
	app, srv := apiApp(t)
	srv.On("POST", "/contact/companies/{id}/archive", http.StatusOK, []byte("<html>ok</html>"), "Content-Type", "text/html")
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "archive", "12", "-o", "json"}, archive), 1)
	if _, errs := bufs(app); !strings.Contains(errs, `"message":"Befehl ausgeführt, Ausgabe fehlgeschlagen: `) {
		t.Errorf("stderr = %q", errs)
	}
	// stdout fails
	app, srv = apiApp(t)
	app.Out = failWriter{}
	srv.On("POST", "/contact/companies/{id}/archive", http.StatusOK, []byte(`{"id":12}`))
	wantExit(t, app, runEntries(t, app, nil, []string{"company", "archive", "12", "-o", "yaml"}, archive), 1)
	if errs := app.Err.(interface{ String() string }).String(); !strings.Contains(errs, "Befehl ausgeführt, Ausgabe fehlgeschlagen: ") {
		t.Errorf("stderr = %q", errs)
	}
}
