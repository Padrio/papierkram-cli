package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// Name resolution in the flags of the builder commands (spec v0.2 §4,
// task 3): order of the checks, write back of the IDs, --dry-run and
// commands --json.

// invoiceByName is invoice create with the customer flag value name.
func invoiceByName(customer string, extra ...string) []string {
	return append([]string{"invoice", "create", "--name", "X", "--customer", customer, "--payment-term", "1130",
		"--document-date", "heute", "--item", "name=Beratung,qty=2,unit=Stunde,price=100,vat=19", "-o", "json"}, extra...)
}

// mustermann answers the company list with ACME (1) and Mustermann GmbH
// (2).
func mustermann(t *testing.T, srv *mockapi.Server) {
	t.Helper()
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(1, "ACME"), company(2, "Mustermann GmbH")}))
}

// person is a person list entry.
func person(id int, first, last string) map[string]any {
	return map[string]any{"id": id, "first_name": first, "last_name": last}
}

// wantPaths checks method and path of the requests sent, in order.
func wantPaths(t *testing.T, srv *mockapi.Server, want ...string) []mockapi.Recorded {
	t.Helper()
	reqs := srv.Requests()
	var got []string
	for _, r := range reqs {
		got = append(got, r.Method+" "+r.Path)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Requests %q, erwartet %q", got, want)
	}
	return reqs
}

// wantErrText checks message and hint of the structured error on stderr
// (-o json); hint "" is not checked.
func wantErrText(t *testing.T, app *App, msg, hint string) {
	t.Helper()
	_, errs := bufs(app)
	var doc struct {
		Error struct {
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lastLine(errs)), &doc); err != nil {
		t.Fatalf("kein JSON-Fehler: %v\n%s", err, errs)
	}
	if doc.Error.Message != msg || hint != "" && doc.Error.Hint != hint {
		t.Errorf("Meldung %q, Hinweis %q\nerwartet %q, %q", doc.Error.Message, doc.Error.Hint, msg, hint)
	}
}

func TestNameFlagsAnnotated(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, tc := range []struct{ command, flag, resource string }{
		{"invoice create", "customer", "company"},
		{"invoice create", "contact-person", "person"},
		{"invoice create", "customer-project", "project"},
		{"invoice create", "payment-term", "payment-term"},
		{"invoice update", "customer", "company"},
		{"estimate create", "customer", "company"},
		{"voucher create", "creditor", "company"},
		{"voucher list", "creditor", "company"},
		{"project create", "team-member", "user"},
		{"project create", "default-proposition", "proposition"},
		{"task create", "proposition", "proposition"},
		{"task create", "project", "project"},
		{"task create", "user", "user"},
		{"time create", "task", "task"},
		{"time list", "user", "user"},
		{"time list", "task", "task"},
		{"invoice list", "company", "company"},
		{"invoice list", "project", "project"},
		{"person list", "company", "company"},
		{"person describe", "company", "company"},
		{"person delete", "company", "company"},
		{"person create", "company", "company"},
	} {
		f := d.find(t, tc.command).flag(t, tc.flag)
		if f.Resolves != tc.resource {
			t.Errorf("%s --%s: resolves %q, erwartet %q", tc.command, tc.flag, f.Resolves, tc.resource)
		}
		if f.Type != "id" || !strings.Contains(f.Description, "ID oder Name") {
			t.Errorf("%s --%s: Typ %q, Hilfe %q", tc.command, tc.flag, f.Type, f.Description)
		}
	}
	for _, tc := range []struct{ command, flag string }{
		{"time log", "task"}, {"time log", "user"}, {"time start", "user"}, {"local init", "company"},
		{"local init", "project"}, {"context create", "user"}, {"time list", "invoice"},
		{"transaction list", "bank-connection"}, {"invoice create", "from-estimate"},
		{"invoice create", "custom-template"}, {"voucher pay", "transaction"},
	} {
		if f := d.find(t, tc.command).flag(t, tc.flag); f.Resolves != "" {
			t.Errorf("%s --%s: resolves %q, erwartet keins", tc.command, tc.flag, f.Resolves)
		}
	}
	// every flag of the table on a builder command, and only those
	custom := []string{"time log", "time start", "local init", "context create"}
	n := 0
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			ref, named := registry.NameRefOf(f.Name)
			want := ""
			if named && f.Type == "id" && !slices.Contains(custom, c.Command) {
				want = ref.Resource
			}
			if f.Resolves != want {
				t.Errorf("%s --%s: resolves %q, erwartet %q", c.Command, f.Name, f.Resolves, want)
			}
			if f.Resolves != "" {
				n++
			}
		}
	}
	if n < 40 {
		t.Errorf("nur %d Flags mit resolves", n)
	}
}

func TestInvoiceCreateCustomerByName(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, invoiceByName("Mustermann GmbH")), 0)
	reqs := wantPaths(t, srv, "GET /contact/companies", "POST /income/invoices")
	var body struct {
		Customer struct {
			ID int64 `json:"id"`
		} `json:"customer"`
	}
	if err := json.Unmarshal(reqs[1].Body, &body); err != nil || body.Customer.ID != 2 {
		t.Errorf("body %s (%v)", reqs[1].Body, err)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kunde „Mustermann GmbH“ → 2 (1 Credit)\n") {
		t.Errorf("stderr %q", errs)
	}
	assertNoToken(t, app)
}

func TestNameResolutionAfterReadOnlyGate(t *testing.T) {
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		mustermann(t, srv)
		wantExit(t, app, Run(app, invoiceByName("Mustermann GmbH", extra...)), 9)
		wantRequests(t, srv, 0)
	}
	// also a path flag of a simple action
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"person", "delete", "5", "--company", "Mustermann GmbH", "--yes", "-o", "json"}), 9)
	wantRequests(t, srv, 0)
}

func TestNameResolutionAfterFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "list", "--company", "Mustermann", "--limit", "500", "-o", "json"},
		{"invoice", "create", "--name", "X", "--customer", "Mustermann", "--item", "name=A,menge=2", "-o", "json"},
		{"person", "describe", "abc", "--company", "Mustermann", "-o", "json"},
		{"time", "list", "--user", "Max", "--invoice", "abc", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
}

func TestNameResolutionBeforeBody(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	args := invoiceByName("Mustermann GmbH")
	args[slices.Index(args, "heute")] = "2026-13-45"
	wantExit(t, app, Run(app, args), 5)
	wantPaths(t, srv, "GET /contact/companies")
}

func TestNoResolutionOutsideFlags(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "describe", "Mustermann", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, `ungültige ID "Mustermann": ganze Zahl ≥ 0 erwartet`, "")

	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, f := range d.find(t, "api").Flags {
		if f.Resolves != "" {
			t.Errorf("api --%s: resolves %q", f.Name, f.Resolves)
		}
	}
	for _, f := range d.GlobalFlags {
		if f.Resolves != "" {
			t.Errorf("--%s: resolves %q", f.Name, f.Resolves)
		}
	}
}

func TestDryRunShowsResolved(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, invoiceByName("Mustermann GmbH", "--dry-run")), 0)
	wantPaths(t, srv, "GET /contact/companies")
	out, _ := bufs(app)
	wantContract(t, "dry-run", []byte(out))
	var doc struct {
		Resolved json.RawMessage `json:"resolved"`
		Credits  int             `json:"credits"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if want := `[{"flag":"customer","name":"Mustermann GmbH","resource":"company","id":2,"credits":1,"cached":false}]`; canonJSON(t, doc.Resolved) != canonJSON(t, []byte(want)) {
		t.Errorf("resolved %s", doc.Resolved)
	}
	if doc.Credits != 51 {
		t.Errorf("credits %d", doc.Credits)
	}

	// the text form names the resolution and its spent credits
	app, srv = apiApp(t)
	mustermann(t, srv)
	args := invoiceByName("Mustermann GmbH", "--dry-run")
	o := slices.Index(args, "-o")
	wantExit(t, app, Run(app, slices.Delete(slices.Clone(args), o, o+2)), 0) // without -o json
	out, _ = bufs(app)
	for _, want := range []string{"Probelauf – nichts angelegt\n", "Aufgelöst: --customer „Mustermann GmbH“ → 2 (1 Credit, bereits verbraucht)\n", "Credits: 51\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout ohne %q:\n%s", want, out)
		}
	}

	// from the cache: 0 credits, the credits of the command alone
	app.Out.(interface{ Reset() }).Reset()
	wantExit(t, app, Run(app, args), 0)
	wantRequests(t, srv, 1) // only the one of the first run
	out, _ = bufs(app)
	if !strings.Contains(out, `"cached": true`) || !strings.Contains(out, `"credits": 50`) {
		t.Errorf("stdout %s", out)
	}
	wantContract(t, "dry-run", []byte(out))

	// without a name: no resolved
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, invoiceByName("3214", "--dry-run")), 0)
	if out, _ := bufs(app); strings.Contains(out, "resolved") || !strings.Contains(out, `"credits": 50`) {
		t.Errorf("stdout %s", out)
	}
}

func TestListFilterByName(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"invoice", "list", "--company", "mustermann gmbh", "-o", "json"}), 0)
	reqs := wantPaths(t, srv, "GET /contact/companies", "GET /income/invoices")
	if q := reqs[1].Query.Encode(); q != "company_id=2&page_size=25" {
		t.Errorf("Query %s", q)
	}

	// an ID filter and a name filter, --all
	app, srv = apiApp(t)
	srv.OnFunc("GET", "/projects", pagedList(t, 1, []map[string]any{{"id": 4710, "name": "Relaunch"}}))
	wantExit(t, app, Run(app, []string{"invoice", "list", "--company", "7", "--project", "Relaunch", "--all", "-o", "json"}), 0)
	reqs = wantPaths(t, srv, "GET /projects", "GET /income/invoices")
	if q := reqs[1].Query; q.Get("company_id") != "7" || q.Get("project_id") != "4710" {
		t.Errorf("Query %s", q.Encode())
	}

	// not found: exit 4 after the list, nothing listed
	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"invoice", "list", "--company", "Unbekannt", "-o", "json"}), 4)
	wantPaths(t, srv, "GET /contact/companies")
	wantErrText(t, app, "Firma „Unbekannt“ nicht gefunden", "ID angeben (papierkram company list) (1 Credit verbraucht)")
}

func TestPersonCompanyByName(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "list", "--company", "Mustermann GmbH", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies", "GET /contact/companies/2/persons")

	// describe and a simple action with the path flag
	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "describe", "1059", "--company", "Mustermann GmbH", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies", "GET /contact/companies/2/persons/1059")
	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "delete", "1059", "--company", "Mustermann GmbH", "--yes", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies", "DELETE /contact/companies/2/persons/1059")
	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "delete", "1059", "--company", "Mustermann GmbH", "--dry-run", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies")
	if out, _ := bufs(app); !strings.Contains(out, "/contact/companies/2/persons/1059") || !strings.Contains(out, `"resolved"`) || !strings.Contains(out, `"credits": 2`) {
		t.Errorf("stdout %s", out)
	}

	// ruling T3-D: without a possible prompt (--no-input, no terminal)
	// the missing confirmation is exit 8 before the resolution
	for _, extra := range [][]string{{"--no-input"}, nil} {
		app, srv = apiApp(t)
		mustermann(t, srv)
		wantExit(t, app, Run(app, append([]string{"person", "delete", "1059", "--company", "Mustermann GmbH", "-o", "json"}, extra...)), 8)
		wantRequests(t, srv, 0)
	}
	// in a terminal the prompt needs the resolved ID: list, summary,
	// declined, nothing deleted
	app, srv = apiApp(t)
	mustermann(t, srv)
	app.StdinTTY = true
	app.In = strings.NewReader("nein\n")
	wantExit(t, app, Run(app, []string{"person", "delete", "1059", "--company", "Mustermann GmbH", "-o", "json"}), 8)
	wantPaths(t, srv, "GET /contact/companies", "GET /contact/companies/2/persons/1059")

	// without the flag the preset still applies, without resolution
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "2843"})
	wantExit(t, app, Run(app, []string{"person", "list", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies/2843/persons")
}

func TestContactPersonByName(t *testing.T) {
	persons := pagedList(t, 1, []map[string]any{person(54, "Max", "Mustermann"), person(55, "Anna", "Groß")})
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies/{company_id}/persons", persons)
	wantExit(t, app, Run(app, append(invoiceByName("2"), "--contact-person", "anna groß")), 0)
	reqs := wantPaths(t, srv, "GET /contact/companies/2/persons", "POST /income/invoices")
	var body struct {
		Customer struct {
			ID            int64 `json:"id"`
			ContactPerson struct {
				ID int64 `json:"id"`
			} `json:"contact_person"`
		} `json:"customer"`
	}
	if err := json.Unmarshal(reqs[1].Body, &body); err != nil || body.Customer.ID != 2 || body.Customer.ContactPerson.ID != 55 {
		t.Errorf("body %s (%v)", reqs[1].Body, err)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kontaktperson „Anna Groß“ → 55 (1 Credit)\n") {
		t.Errorf("stderr %q", errs)
	}

	// the customer by name first, then its persons
	app, srv = apiApp(t)
	mustermann(t, srv)
	srv.OnFunc("GET", "/contact/companies/{company_id}/persons", persons)
	wantExit(t, app, Run(app, append(invoiceByName("Mustermann GmbH"), "--contact-person", "Anna Groß")), 0)
	wantPaths(t, srv, "GET /contact/companies", "GET /contact/companies/2/persons", "POST /income/invoices")
}

func TestContactPersonNameWithoutCustomer(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--contact-person", "Anna Groß", "--payment-term", "1", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, "Name für --contact-person braucht --customer (ID oder Name)", "sonst die ID der Kontaktperson angeben")

	// a customer ID < 1 has no persons
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "0", "--contact-person", "Anna Groß", "--payment-term", "1", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, `ungültiger Wert "0" für --customer: Name für --contact-person braucht eine ID ≥ 1 oder einen Namen`, "sonst die ID der Kontaktperson angeben")

	// the ID of the contact person needs no customer
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--contact-person", "55", "--payment-term", "1", "-o", "json"}), 0)
	wantPaths(t, srv, "PUT /income/invoices/459")
}

func TestTeamMemberNames(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/users", pagedList(t, 1, []map[string]any{person(1, "Max", "Mustermann"), person(3, "Erika", "Musterfrau")}))
	wantExit(t, app, Run(app, []string{"project", "create", "--name", "P", "--customer", "2", "--team-member", "Max Mustermann",
		"--team-member", "7", "--team-member", " 8 ", "--team-member", "erika musterfrau", "-o", "json"}), 0)
	reqs := wantPaths(t, srv, "GET /users", "POST /projects")
	if got, want := canonJSON(t, reqs[1].Body), canonJSON(t, []byte(`{"name":"P","customer":{"id":2},"team_members":[{"id":1},{"id":7},{"id":8},{"id":3}]}`)); got != want {
		t.Errorf("body %s, erwartet %s", got, want)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Benutzer „Erika Musterfrau“ → 3 (aus dem Zwischenspeicher, 0 Credits)\n") {
		t.Errorf("stderr %q", errs)
	}
}

func TestIDFlagStaysID(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, invoiceByName("3214")), 0)
	wantPaths(t, srv, "POST /income/invoices")

	// whitespace around an ID and a sign stay an ID
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, invoiceByName(" +3214 ")), 0)
	reqs := wantPaths(t, srv, "POST /income/invoices")
	if !strings.Contains(string(reqs[0].Body), `"customer":{"id":3214}`) {
		t.Errorf("body %s", reqs[0].Body)
	}

	// negative IDs fail like before: field flags exit 5 (body), list and
	// path flags exit 2
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, invoiceByName("-1")), 5)
	wantRequests(t, srv, 0)
	wantErrText(t, app, `ungültiger Wert "-1" für --customer: ID muss eine ganze Zahl ≥ 0 sein`, "")
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "list", "--company", "-1", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, "ungültiger Wert -1 für --company: ID muss ≥ 0 sein", "Verwendung: papierkram invoice list [Optionen]")
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "list", "--company", "-1", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, "ungültiger Wert -1 für --company: ID muss ≥ 0 sein", "")

	// an ID out of range is no name
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "list", "--company", "99999999999999999999", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, `ungültiger Wert "99999999999999999999" für --company: Wert außerhalb des Bereichs`, "")
}

func TestNonResolvingIDFlagMessage(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		msg, usage string
	}{
		{[]string{"time", "list", "--invoice", "abc"}, `ungültiger Wert "abc" für --invoice: ganze Zahl erwartet`, "papierkram time list [Optionen]"},
		{[]string{"transaction", "list", "--bank-connection", "Sparkasse"}, `ungültiger Wert "Sparkasse" für --bank-connection: ganze Zahl erwartet`, "papierkram transaction list [Optionen]"},
		{[]string{"time", "list", "--invoice", "99999999999999999999"}, `ungültiger Wert "99999999999999999999" für --invoice: Wert außerhalb des Bereichs`, "papierkram time list [Optionen]"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(tc.args, "-o", "json")), 2)
		wantRequests(t, srv, 0)
		wantErrText(t, app, tc.msg, "Verwendung: "+tc.usage)
	}
	// integer forms the integer flag took before stay valid
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "list", "--invoice", "0x10", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 1); reqs[0].Query.Get("invoice_id") != "16" {
		t.Errorf("Query %s", reqs[0].Query.Encode())
	}
}

func TestEmptyNameExit2(t *testing.T) {
	for _, args := range [][]string{
		invoiceByName("  "),
		{"invoice", "list", "--company", "", "-o", "json"},
		{"person", "list", "--company", " \t", "-o", "json"},
		{"project", "create", "--name", "P", "--customer", "2", "--team-member", "7", "--team-member", "", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		_, errs := bufs(app)
		if !strings.Contains(errs, "braucht eine ID oder einen Namen") {
			t.Errorf("%v: stderr %q", args, errs)
		}
	}
}

// Errors of the list requests keep their code; a name never matches
// silently.
func TestNameResolutionErrors(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH"), company(3, "mustermann gmbh")}))
	wantExit(t, app, Run(app, invoiceByName("Mustermann GmbH")), 2)
	wantPaths(t, srv, "GET /contact/companies")

	app, srv = apiApp(t)
	srv.On("GET", "/contact/companies", http.StatusTooManyRequests, nil)
	wantExit(t, app, Run(app, invoiceByName("Mustermann GmbH")), 6)
	wantPaths(t, srv, "GET /contact/companies")
}

// A missing --input or --items file is exit 2 with the flags, before the
// gate and the resolution (review T3-M2).
func TestMissingFileBeforeResolution(t *testing.T) {
	dir := t.TempDir()
	// the message quotes the path like %q (backslashes of Windows paths
	// doubled), CI fix
	missing := filepath.Join(dir, "fehlt.json")
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"--input", missing}, fmt.Sprintf("--input: Datei %q nicht gefunden", missing)},
		{[]string{"--items", missing}, fmt.Sprintf("--items: Datei %q nicht gefunden", missing)},
		{[]string{"--input", dir}, fmt.Sprintf("--input: %q ist ein Verzeichnis", dir)},
	} {
		app, srv := apiApp(t)
		mustermann(t, srv)
		args := append([]string{"invoice", "create", "--name", "X", "--customer", "Mustermann GmbH", "-o", "json"}, tc.args...)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		wantErrText(t, app, tc.msg, "Pfad prüfen oder - für die Standardeingabe angeben")
	}
	// "-" is stdin: no check of a file
	app, srv := apiApp(t)
	mustermann(t, srv)
	app.In = strings.NewReader(`{"payment_term":{"id":1130},"document_date":"2026-11-04"}`)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--name", "X", "--customer", "Mustermann GmbH", "--input", "-",
		"--item", "name=A,qty=1,unit=Stunde,price=5,vat=19", "--dry-run", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies")
}

// The path flag of create and update and a preset filter take names
// (review T3-M4).
func TestPersonWriteAndPresetFilterByName(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "create", "--company", "Mustermann GmbH", "--first-name", "Anna", "--last-name", "Groß", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies", "POST /contact/companies/2/persons")

	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"person", "update", "55", "--company", "mustermann gmbh", "--email", "anna@example.org", "-o", "json"}), 0)
	wantPaths(t, srv, "GET /contact/companies", "PUT /contact/companies/2/persons/55")

	// task list --project is the filter of the project preset; a name
	// replaces the preset like an ID
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "34"})
	srv.OnFunc("GET", "/projects", pagedList(t, 1, []map[string]any{{"id": 4710, "name": "Relaunch"}}))
	wantExit(t, app, Run(app, []string{"task", "list", "--project", "Relaunch", "-o", "json"}), 0)
	reqs := wantPaths(t, srv, "GET /projects", "GET /tracker/tasks")
	if q := reqs[1].Query.Get("project_id"); q != "4710" {
		t.Errorf("project_id %q", q)
	}
	if _, errs := bufs(app); strings.Contains(errs, "PAPIERKRAM_PROJECT") {
		t.Errorf("stderr %q", errs)
	}
}

// Ruling T3-E: with --customer, --customer-project by name is one of the
// customer's projects; a namesake of another customer is not chosen.
func TestCustomerProjectByName(t *testing.T) {
	app, srv := apiApp(t)
	mustermann(t, srv)
	srv.OnFunc("GET", "/projects", customerProjects(t))
	wantExit(t, app, Run(app, append(invoiceByName("Mustermann GmbH"), "--customer-project", "Relaunch")), 0)
	reqs := wantPaths(t, srv, "GET /contact/companies", "GET /projects", "POST /income/invoices")
	if q := reqs[1].Query.Get("company_id"); q != "2" {
		t.Errorf("company_id %q", q)
	}
	if !strings.Contains(string(reqs[2].Body), `"project":{"id":10}`) {
		t.Errorf("body %s", reqs[2].Body)
	}

	// without --customer all projects, as before: here ambiguous
	app, srv = apiApp(t)
	srv.OnFunc("GET", "/projects", customerProjects(t))
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer-project", "Relaunch", "--payment-term", "1", "-o", "json"}), 2)
	reqs = wantPaths(t, srv, "GET /projects")
	if reqs[0].Query.Has("company_id") {
		t.Errorf("Query %s", reqs[0].Query.Encode())
	}
	wantErrText(t, app, "Projekt „Relaunch“ ist mehrdeutig: 10 „Relaunch“, 11 „Relaunch“",
		"ID statt Name angeben oder mit --customer eingrenzen (1 Credit verbraucht)")

	// a customer ID < 1 has no projects
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "0", "--customer-project", "Relaunch", "--payment-term", "1", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
	wantErrText(t, app, `ungültiger Wert "0" für --customer: Name für --customer-project braucht eine ID ≥ 1 oder einen Namen`, "sonst die ID des Projekts angeben")
}

// nextRun is a fresh app on the environment, cache and config
// directory of app: the next invocation of the CLI.
func nextRun(t *testing.T, app *App) *App {
	t.Helper()
	next, _, _ := newTestApp(t)
	next.Env, next.CacheDir, next.ConfigDir = app.Env, app.CacheDir, app.ConfigDir
	return next
}

func TestMutationInvalidatesNameCache(t *testing.T) {
	// final review B-I1: a name resolved into the cache, then the object
	// archived by the same CLI: the next resolution loads the list again
	// instead of taking the archived ID from the cache
	app, srv := apiApp(t)
	before := pagedList(t, 1, []map[string]any{company(99, "ACME")})
	after := pagedList(t, 1, []map[string]any{company(99, "ACME", "archived"), company(120, "ACME")})
	archived := false
	srv.OnFunc("GET", "/contact/companies", func(w http.ResponseWriter, r *http.Request) {
		if archived {
			after(w, r)
			return
		}
		before(w, r)
	})
	srv.OnFunc("POST", "/contact/companies/99/archive", func(w http.ResponseWriter, _ *http.Request) {
		archived = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"ACME","record_state":"archived"}`))
	})
	create := []string{"project", "create", "--name", "P", "--customer", "ACME", "-o", "json"}
	wantExit(t, app, Run(app, append(create, "--dry-run")), 0)
	cache := completionCachePath(app.CacheDir, "", "company")
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("Cache nicht geschrieben: %v", err)
	}
	second := nextRun(t, app)
	wantExit(t, second, Run(second, []string{"company", "archive", "99", "-o", "json"}), 0)
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("Cache nach archive noch da: %v", err)
	}
	third := nextRun(t, app)
	wantExit(t, third, Run(third, create), 0)
	reqs := wantPaths(t, srv, "GET /contact/companies", "POST /contact/companies/99/archive", "GET /contact/companies", "POST /projects")
	var body struct {
		Customer struct {
			ID int64 `json:"id"`
		} `json:"customer"`
	}
	if err := json.Unmarshal(reqs[3].Body, &body); err != nil || body.Customer.ID != 120 {
		t.Errorf("body %s (%v)", reqs[3].Body, err)
	}
}

func TestMutationInvalidatesNameCacheOutcomeUnknown(t *testing.T) {
	// B-I1 after exit 12: the change may have arrived, the cache goes; a
	// path outside the registry (papierkram api) drops every cache of the
	// context
	app, srv := apiApp(t)
	srv.OnFunc("POST", "/contact/companies/99/archive", closeAfterRead)
	writeCacheFile(t, app, srv, "company", `[{"id":99,"name":"ACME"}]`, `,"complete":true`)
	other := writeCacheFile(t, app, srv, "project", `[{"id":5,"name":"Web"}]`, `,"complete":true`)
	wantExit(t, app, Run(app, []string{"company", "archive", "99", "-o", "json"}), 12)
	if _, err := os.Stat(completionCachePath(app.CacheDir, "", "company")); !os.IsNotExist(err) {
		t.Errorf("Cache nach Exit 12 noch da: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("Cache einer anderen Ressource gelöscht: %v", err)
	}
	srv.On("POST", "/unbekannt", 200, []byte(`{}`))
	next := nextRun(t, app)
	wantExit(t, next, Run(next, []string{"api", "POST", "/unbekannt", "--yes", "-o", "json"}), 0)
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Errorf("Cache nach api POST außerhalb der Spec noch da: %v", err)
	}
}

// final review B-M2: --customer-project by name without --customer takes
// the customer of --input as parent (file or stdin, read once); without
// any customer it searches all projects as before and says so.
func TestCustomerProjectParentFromInput(t *testing.T) {
	const input = `{"customer":{"id":2}}`
	for _, tc := range []struct {
		name  string
		path  string
		stdin string
	}{
		{"Datei", writeFile(t, "rechnung.json", input), ""},
		{"Standardeingabe", "-", input},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			app.In = strings.NewReader(tc.stdin)
			srv.OnFunc("GET", "/projects", customerProjects(t))
			wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", tc.path, "--customer-project", "Relaunch",
				"--payment-term", "1", "-o", "json"}), 0)
			reqs := wantPaths(t, srv, "GET /projects", "PUT /income/invoices/459")
			if q := reqs[0].Query.Get("company_id"); q != "2" {
				t.Errorf("company_id %q", q)
			}
			if !strings.Contains(string(reqs[1].Body), `"project":{"id":10}`) || !strings.Contains(string(reqs[1].Body), `"id":2`) {
				t.Errorf("body %s", reqs[1].Body)
			}
		})
	}
	// no customer anywhere: all projects, named in the hint
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/projects", pagedList(t, 1, []map[string]any{{"id": 10, "name": "Relaunch"}}))
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer-project", "Relaunch", "--payment-term", "1", "-o", "json"}), 0)
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt „Relaunch“ → 10 (1 Credit; ohne Kunde unter den Projekten aller Kunden gesucht)\n") {
		t.Errorf("stderr %q", errs)
	}
	wantPaths(t, srv, "GET /projects", "PUT /income/invoices/459")
	for _, ref := range registry.NameRefs() {
		if ref.ParentFilter != "" && allParentsNote[ref.Flag] == "" {
			t.Errorf("--%s: kein Hinweis für die Suche ohne --%s", ref.Flag, ref.Parent)
		}
	}
}
