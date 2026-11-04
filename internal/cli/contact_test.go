package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// company and person commands (spec 7.1, 9.7, 15 no. 5–16).

// TestContactCommandsGolden renders list and describe of company and person
// from the spec examples of the mock in every output format.
func TestContactCommandsGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"company-list", []string{"company", "list"}, "/contact/companies", url.Values{"page_size": {"25"}}},
		{"company-describe", []string{"company", "describe", "2843"}, "/contact/companies/2843", url.Values{}},
		{"person-list", []string{"person", "list", "--company", "2843"}, "/contact/companies/2843/persons", url.Values{"page_size": {"25"}}},
		{"person-describe", []string{"person", "describe", "1063", "--company", "2862"}, "/contact/companies/2862/persons/1063", url.Values{}},
	}
	for _, tc := range cases {
		for _, format := range []string{"table", "wide", "json", "yaml"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				app, srv := apiApp(t)
				wantExit(t, app, Run(app, append(tc.args, "-o", format)), 0)
				r := wantRequests(t, srv, 1)[0]
				if r.Method != "GET" || r.Path != tc.path || r.Query.Encode() != tc.query.Encode() {
					t.Errorf("request = %s %s?%s", r.Method, r.Path, r.Query.Encode())
				}
				out, errs := bufs(app)
				if errs != "" {
					t.Errorf("stderr = %q", errs)
				}
				golden.Assert(t, tc.name+"."+format, []byte(out))
				assertNoToken(t, app)
			})
		}
	}
}

func TestContactAliases(t *testing.T) {
	for _, args := range [][]string{
		{"firma", "list", "-o", "json"},
		{"kontakt", "describe", "2843", "-o", "json"},
		{"kontaktperson", "list", "--company", "1", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

// companyJSON is a minimal company list entry; typ "" is a null
// contact_type.
func companyJSON(id int, typ string) string {
	ct := "null"
	if typ != "" {
		ct = strconv.Quote(typ)
	}
	return fmt.Sprintf(`{"id":%d,"name":"Firma %d","contact_type":%s,"type":"company"}`, id, id, ct)
}

// mixedCompanies are three pages with customers and suppliers mixed, as the
// API sends them whatever contact_type the request has (spec 4.6).
var mixedCompanies = [][]string{
	{companyJSON(1, "customer"), companyJSON(2, "supplier"), companyJSON(3, "customer")},
	{companyJSON(4, "supplier"), companyJSON(5, "customer"), companyJSON(6, "")},
	{companyJSON(7, "customer"), companyJSON(8, "supplier")},
}

// companyPages answers GET /contact/companies page by page from pages.
func companyPages(srv *mockapi.Server, pages [][]string) {
	total := 0
	for _, p := range pages {
		total += len(p)
	}
	srv.OnFunc("GET", "/contact/companies", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil {
			n = 1
		}
		var entries []string
		if n >= 1 && n <= len(pages) {
			entries = pages[n-1]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"type":"list","page":%d,"page_size":%s,"total_pages":%d,"total_entries":%d,"has_more":%t,"entries":[%s]}`,
			n, r.URL.Query().Get("page_size"), len(pages), total, n < len(pages), strings.Join(entries, ","))
	})
}

// filteredEnvelope is the expected JSON of company list --type.
func filteredEnvelope(page, size, pages, total int, more bool, entries ...string) string {
	return fmt.Sprintf(`{"type":"list","page":%d,"page_size":%d,"total_pages":%d,"total_entries":%d,"has_more":%t,"entries":[%s]}`,
		page, size, pages, total, more, strings.Join(entries, ","))
}

func TestR19_CompanyListTypeFiltersAllPages(t *testing.T) {
	c := func(id int, typ string) string { return companyJSON(id, typ) }
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--type", "supplier"},
			filteredEnvelope(1, 25, 1, 3, false, c(2, "supplier"), c(4, "supplier"), c(8, "supplier"))},
		{[]string{"--type", "customer", "--limit", "3"},
			filteredEnvelope(1, 3, 2, 4, true, c(1, "customer"), c(3, "customer"), c(5, "customer"))},
		{[]string{"--type", "customer", "--limit", "3", "--page", "2"},
			filteredEnvelope(2, 3, 2, 4, false, c(7, "customer"))},
		{[]string{"--type", "customer", "--limit", "3", "--page", "3"},
			filteredEnvelope(3, 3, 2, 4, false)},
		{[]string{"--type", "customer", "--all"},
			filteredEnvelope(1, 100, 1, 4, false, c(1, "customer"), c(3, "customer"), c(5, "customer"), c(7, "customer"))},
	} {
		app, srv := apiApp(t)
		companyPages(srv, mixedCompanies)
		wantExit(t, app, Run(app, append([]string{"company", "list", "-o", "json"}, tc.args...)), 0)
		for i, r := range wantRequests(t, srv, 3) {
			// all pages at the API maximum; contact_type is never sent
			want := url.Values{"page": {strconv.Itoa(i + 1)}, "page_size": {"100"}}
			if r.Query.Encode() != want.Encode() {
				t.Errorf("%v: request %d: query = %s, erwartet %s", tc.args, i+1, r.Query.Encode(), want.Encode())
			}
		}
		out, errs := bufs(app)
		if got, want := canonJSON(t, []byte(out)), canonJSON(t, []byte(tc.want)); got != want {
			t.Errorf("%v:\nstdout   = %s\nerwartet = %s", tc.args, got, want)
		}
		if errs != "" {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}

// The envelope keeps the order of the API envelope (spec 10.3).
func TestCompanyListTypeEnvelopeOrder(t *testing.T) {
	app, srv := apiApp(t)
	companyPages(srv, [][]string{{companyJSON(1, "customer")}})
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "supplier", "-o", "json"}), 0)
	want := "{\n  \"type\": \"list\",\n  \"page\": 1,\n  \"page_size\": 25,\n  \"total_pages\": 0,\n  \"total_entries\": 0,\n  \"has_more\": false,\n  \"entries\": []\n}\n"
	if out := mustOut(app); out != want {
		t.Errorf("stdout = %q", out)
	}
	wantRequests(t, srv, 1)
}

func TestCompanyListTypeSortAndTable(t *testing.T) {
	app, srv := apiApp(t)
	companyPages(srv, mixedCompanies)
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "supplier", "--sort", "name", "--desc"}), 0)
	for _, r := range wantRequests(t, srv, 3) {
		if r.Query.Get("order_by") != "name" || r.Query.Get("order_direction") != "desc" || r.Query.Has("contact_type") {
			t.Errorf("query = %v", r.Query)
		}
	}
	out := mustOut(app)
	if !strings.Contains(out, "Firma 8") || strings.Contains(out, "Firma 1 ") || strings.Count(out, "supplier") != 3 {
		t.Errorf("stdout = %s", out)
	}
}

func TestCompanyListTypeValidation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "kunde"}), 5)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	for _, want := range []string{`ungültiger Wert "kunde" für --type`, "customer, supplier"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr enthält %q nicht:\n%s", want, errs)
		}
	}

	for _, args := range [][]string{
		{"--max-pages", "2"},
		{"--type", "supplier", "--all", "--page", "2"},
		{"--type", "supplier", "--max-pages", "0"},
		{"--type", "supplier", "--limit", "0"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append([]string{"company", "list"}, args...)), 2)
		wantRequests(t, srv, 0)
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "list", "--max-pages", "2"}), 2)
	if _, errs := bufs(app); !strings.Contains(errs, "--max-pages gilt nur zusammen mit --all oder --type") {
		t.Errorf("stderr = %q", errs)
	}
}

// --max-pages caps --type without --all: the result covers the loaded
// pages only and says so (has_more, warning).
func TestCompanyListTypeMaxPages(t *testing.T) {
	app, srv := apiApp(t)
	companyPages(srv, mixedCompanies)
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "supplier", "--max-pages", "2", "-o", "json"}), 0)
	wantRequests(t, srv, 2)
	out, errs := bufs(app)
	want := filteredEnvelope(1, 25, 1, 2, true, companyJSON(2, "supplier"), companyJSON(4, "supplier"))
	if canonJSON(t, []byte(out)) != canonJSON(t, []byte(want)) {
		t.Errorf("stdout = %s", out)
	}
	// unloaded pages may or may not hold matches: no claim of more entries
	if !strings.Contains(errs, "Warnung: --max-pages 2: 2 von 3 Seiten geladen; Ergebnis unvollständig, möglicherweise weitere Treffer") ||
		strings.Contains(errs, "weitere Einträge vorhanden") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestCompanyListTypeWarnsFromTenPages(t *testing.T) {
	pages := make([][]string, 10)
	for i := range pages {
		pages[i] = []string{companyJSON(i+1, "customer")}
	}
	// without TTY: warning, all pages
	app, srv := apiApp(t)
	companyPages(srv, pages)
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "customer", "-o", "json"}), 0)
	wantRequests(t, srv, 10)
	if _, errs := bufs(app); !strings.Contains(errs, "Warnung: --type lädt 10 Seiten (10 Credits); mit --max-pages begrenzen") {
		t.Errorf("stderr = %q", errs)
	}

	// TTY, declined: only page 1, exit 8
	app, srv = apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("n\n")
	companyPages(srv, pages)
	wantExit(t, app, Run(app, []string{"company", "list", "--type", "customer"}), 8)
	wantRequests(t, srv, 1)
	out, errs := bufs(app)
	if out != "" || !strings.Contains(errs, "Fortfahren? [j/N] ") || !strings.Contains(errs, "Abgebrochen: --type nicht fortgesetzt") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}

	// TTY with --yes or --no-input: warning, no prompt, all pages
	for _, flag := range []string{"--yes", "--no-input"} {
		app, srv = apiApp(t)
		app.StdinTTY = true
		companyPages(srv, pages)
		wantExit(t, app, Run(app, []string{"company", "list", "--type", "customer", "-o", "json", flag}), 0)
		wantRequests(t, srv, 10)
		out, errs := bufs(app)
		if strings.Contains(errs, "Fortfahren") || !strings.Contains(errs, "Warnung: --type lädt 10 Seiten") || !strings.Contains(out, `"total_entries": 10`) {
			t.Errorf("%s: stdout = %s, stderr = %q", flag, out, errs)
		}
	}
}

func TestCompanyListTypeHelp(t *testing.T) {
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "list", "--help"}), 0)
	out := mustOut(app)
	for _, want := range []string{"--type Text", "customer", "supplier", "lädt alle Seiten", "lokal",
		"--page und --limit gelten für das gefilterte Ergebnis", "papierkram company list --type customer",
		"Mit --max-pages zählen total_entries und total_pages nur die Treffer der geladenen Seiten"} {
		if !strings.Contains(out, want) {
			t.Errorf("Hilfe enthält %q nicht:\n%s", want, out)
		}
	}
	// the flag lines say what --page and --limit mean with --type
	for _, flag := range []string{"--page Zahl", "--limit Zahl"} {
		if line := helpLine(out, flag); !strings.Contains(line, "mit --type: ") || !strings.Contains(line, "gefilterten Ergebnisses") {
			t.Errorf("Hilfezeile %s = %q", flag, line)
		}
	}
	// lists without a local filter keep the plain text
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"user", "list", "--help"}), 0)
	if line := helpLine(mustOut(app), "--page Zahl"); !strings.Contains(line, "nur diese Seite laden (ab 1)") || strings.Contains(line, "--type") {
		t.Errorf("user list --page = %q", line)
	}
}

// helpLine returns the help line of a flag ("--page Zahl").
func helpLine(help, flag string) string {
	for line := range strings.SplitSeq(help, "\n") {
		if strings.Contains(line, flag) {
			return line
		}
	}
	return ""
}

func TestCompanyCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "create", "--name", "Müller & Söhne", "--contact-type", "customer",
		"--email", "info@example.org", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/contact/companies", `{"name":"Müller & Söhne","contact_type":"customer","email":"info@example.org"}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "company"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "create", "--name", "X", "--contact-type", "kunde"}), 5)
	wantRequests(t, srv, 0)
}

func TestCompanyUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "update", "2843", "--email", "neu@example.org", "--contact-type", "supplier", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/contact/companies/2843", `{"email":"neu@example.org","contact_type":"supplier"}`)
}

func TestCompanyDeleteNeedsConfirmation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "delete", "2843"}), 8)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"company", "delete", "2843", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/contact/companies/2843" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 2843,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestCompanyArchiveWithoutConfirmation(t *testing.T) {
	for _, action := range []string{"archive", "unarchive"} {
		app, srv := apiApp(t) // no TTY, no --yes
		wantExit(t, app, Run(app, []string{"company", action, "2843", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/contact/companies/2843/"+action {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
	}
}

// R9 at the CLI level: person default and flagged are switches, sent as
// the strings "true"/"false" (TestR09_PersonDefaultFlaggedAsString covers
// the coercion).
func TestPersonCreateSendsBoolFlagsAsStrings(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "create", "--company", "1", "--first-name", "Anna", "--last-name", "Groß",
		"--flagged", "--default=false", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/contact/companies/1/persons", `{"first_name":"Anna","last_name":"Groß","flagged":"true","default":"false"}`)
}

// The API answers default as a boolean (measured 2026-11-04) but takes a
// string: the round trip describe -o json | update --input - sends it as
// "true" again.
func TestPersonUpdateInputSendsBooleanDefaultAsString(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"default":true,"flagged":false}`)
	wantExit(t, app, Run(app, []string{"person", "update", "5", "--company", "7", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/contact/companies/7/persons/5", `{"default":"true","flagged":"false"}`)
}

func TestPersonUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "update", "5", "--company", "7", "--position", "Einkauf", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/contact/companies/7/persons/5", `{"position":"Einkauf"}`)
}

func TestPersonDeleteNeedsConfirmation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "delete", "5", "--company", "7"}), 8)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "delete", "5", "--company", "7", "--yes"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/contact/companies/7/persons/5" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "person 5 gelöscht.\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestPersonCommandsRequireCompany(t *testing.T) {
	for _, args := range [][]string{
		{"person", "list"},
		{"person", "describe", "5"},
		{"person", "create", "--first-name", "Anna", "--last-name", "Groß"},
		{"person", "update", "5", "--first-name", "Anna"},
		{"person", "delete", "5", "--yes"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "Fehler: Pflichtangabe --company fehlt") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
}

// The registered mutations run the protection layer (Task 24): --dry-run
// sends nothing, the read-only lock is exit 9 before any request.
func TestContactMutationsDryRun(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		method, path  string
		credits, body string
	}{
		{[]string{"person", "create", "--company", "7", "--first-name", "Anna", "--last-name", "Groß"},
			"POST", "/contact/companies/7/persons", `"credits": 10`, `"first_name": "Anna"`},
		{[]string{"company", "create", "--name", "Müller & Söhne", "--contact-type", "customer"},
			"POST", "/contact/companies", `"credits": 10`, `"contact_type": "customer"`},
		{[]string{"company", "update", "2843", "--email", "neu@example.org"},
			"PUT", "/contact/companies/2843", `"credits": 1`, `"email": "neu@example.org"`},
		{[]string{"person", "delete", "5", "--company", "7"},
			"DELETE", "/contact/companies/7/persons/5", `"credits": 1`, `"body": null`},
		{[]string{"company", "archive", "2843"},
			"POST", "/contact/companies/2843/archive", `"credits": 1`, `"body": null`},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(tc.args, "--dry-run", "-o", "json")), 0)
		wantRequests(t, srv, 0)
		out := mustOut(app)
		for _, want := range []string{`"dry_run": true`, `"method": "` + tc.method + `"`, tc.path + `"`, tc.credits, tc.body} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: stdout ohne %q:\n%s", tc.args, want, out)
			}
		}
	}
}

func TestContactMutationsReadOnly(t *testing.T) {
	for _, args := range [][]string{
		{"company", "create", "--name", "X", "--contact-type", "customer"},
		{"company", "update", "2843", "--email", "neu@example.org"},
		{"company", "delete", "2843", "--yes"},
		{"company", "archive", "2843"},
		{"company", "unarchive", "2843"},
		{"person", "create", "--company", "7", "--first-name", "Anna", "--last-name", "Groß"},
		{"person", "update", "5", "--company", "7", "--position", "Einkauf"},
		{"person", "delete", "5", "--company", "7", "--yes"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
	}
	// reading stays allowed
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"person", "list", "--company", "7", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
}
