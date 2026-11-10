package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// invoice paid-status (spec v0.2 §5, task 4): open items of the invoice
// list with a decimal sum. The test clock is 2026-06-10 in Berlin.

// paidEntry is an invoice list entry with state, outstanding_amount (nil:
// the key is left out) and due_date (nil: null).
func paidEntry(id int, state string, outstanding, due any) map[string]any {
	m := map[string]any{
		"id": id, "invoice_no": fmt.Sprintf("RE-%04d", id), "name": "Beratung", "state": state,
		"document_date": "2026-05-01", "due_date": due, "total_gross": "119.0",
		"billing": map[string]any{"company": "Mustermann GmbH"}, "record_state": "active",
	}
	if outstanding != nil {
		m["outstanding_amount"] = outstanding
	}
	return m
}

// paidApp is a test app whose invoice list has one page with entries.
func paidApp(t *testing.T, entries ...map[string]any) (*App, *mockapi.Server) {
	t.Helper()
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/income/invoices", pagedList(t, 1, entries))
	return app, srv
}

// paidReport runs paid-status -o json with args and decodes the report.
func paidReport(t *testing.T, app *App, args ...string) map[string]any {
	t.Helper()
	wantExit(t, app, Run(app, append(append([]string{"invoice", "paid-status"}, args...), "-o", "json")), 0)
	out, _ := bufs(app)
	dec := json.NewDecoder(strings.NewReader(out))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("kein JSON: %v\n%s", err, out)
	}
	wantContract(t, "paid-status", []byte(out))
	assertNoToken(t, app)
	return doc
}

// reportIDs are the IDs of the entries of a decoded report.
func reportIDs(doc map[string]any) []string {
	var ids []string
	for _, e := range doc["entries"].([]any) {
		ids = append(ids, string(e.(map[string]any)["id"].(json.Number)))
	}
	return ids
}

// num is the text of a number of a decoded report.
func reportNum(doc map[string]any, key string) string {
	n, _ := doc[key].(json.Number)
	return string(n)
}

func TestPaidStatusSelectsOpen(t *testing.T) {
	app, _ := paidApp(t,
		paidEntry(1, "draft", 50, "2026-06-01"),
		paidEntry(2, "canceled", 30, "2026-06-01"),
		paidEntry(3, "unpaid", "119.0", "2026-06-01"),
		paidEntry(4, "paid", "0.0", "2026-06-01"),
		paidEntry(5, "partially_paid", "50.5", "2026-06-20"),
		paidEntry(6, "deleted", "40.0", "2026-06-01"), // ruling T4-A
	)
	doc := paidReport(t, app)
	if ids := reportIDs(doc); !slices.Equal(ids, []string{"3", "5"}) {
		t.Errorf("Einträge %v", ids)
	}
	if reportNum(doc, "total_outstanding") != "169.50" || reportNum(doc, "count") != "2" || doc["complete"] != true || doc["as_of"] != "2026-06-10" {
		t.Errorf("Bericht %v", doc)
	}
	e := doc["entries"].([]any)[1].(map[string]any)
	if e["outstanding_amount"] != json.Number("50.5") || e["total_gross"] != json.Number("119.0") || e["customer"] != "Mustermann GmbH" ||
		e["state"] != "partially_paid" || e["invoice_no"] != "RE-0005" {
		t.Errorf("Eintrag %v", e)
	}
}

func TestPaidStatusOverdue(t *testing.T) {
	entries := []map[string]any{paidEntry(1, "unpaid", "10.0", "2026-06-09"), paidEntry(2, "unpaid", "20.0", "2026-06-10")}
	app, _ := paidApp(t, entries...)
	doc := paidReport(t, app)
	got := doc["entries"].([]any)
	if d := got[0].(map[string]any)["days_overdue"]; d != json.Number("1") {
		t.Errorf("days_overdue %v", d)
	}
	if d := got[1].(map[string]any)["days_overdue"]; d != json.Number("0") {
		t.Errorf("days_overdue (heute fällig) %v", d)
	}
	if reportNum(doc, "overdue_count") != "1" || reportNum(doc, "count") != "2" {
		t.Errorf("Bericht %v", doc)
	}

	app, _ = paidApp(t, entries...)
	doc = paidReport(t, app, "--overdue")
	if ids := reportIDs(doc); !slices.Equal(ids, []string{"1"}) || reportNum(doc, "count") != "1" || reportNum(doc, "overdue_count") != "1" ||
		reportNum(doc, "total_outstanding") != "10.00" {
		t.Errorf("--overdue: %v", doc)
	}
}

// Review Focus 4: amounts as text or number with any number of digits are
// summed exactly.
func TestPaidStatusDecimalSum(t *testing.T) {
	for _, tc := range []struct {
		a, b any
		want string
	}{
		{"0.1", 0.2, "0.30"},
		{"1234567.89", "0.01", "1234567.90"},
		{0.1, 0.2, "0.30"},
	} {
		app, _ := paidApp(t, paidEntry(1, "unpaid", tc.a, "2026-06-01"), paidEntry(2, "unpaid", tc.b, "2026-06-01"))
		if got := reportNum(paidReport(t, app), "total_outstanding"); got != tc.want {
			t.Errorf("%v + %v = %s, erwartet %s", tc.a, tc.b, got, tc.want)
		}
	}

	app, _ := paidApp(t, paidEntry(1, "unpaid", "1.0", "2026-06-01"), paidEntry(7, "unpaid", "x", "2026-06-01"))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 1)
	_, errs := bufs(app)
	if !strings.Contains(lastLine(errs), `"code":"error"`) || !strings.Contains(lastLine(errs), "Rechnung 7") || !strings.Contains(lastLine(errs), "„x“") {
		t.Errorf("stderr %s", errs)
	}

	for _, tc := range []struct {
		in   []json.Number
		want string
	}{
		{[]json.Number{"0.1", "0.2"}, "0.30"},
		{[]json.Number{"1234567.89", "0.01"}, "1234567.90"},
		{nil, "0.00"},
		{[]json.Number{"226.0", "0.005"}, "226.01"},
		{[]json.Number{"1e2", "-0.5"}, "99.50"},
	} {
		got, err := sumDecimals(tc.in)
		if err != nil || got != json.Number(tc.want) {
			t.Errorf("sumDecimals(%v) = %s, %v; erwartet %s", tc.in, got, err, tc.want)
		}
	}
	if _, err := sumDecimals([]json.Number{"1", "x"}); err == nil {
		t.Error("sumDecimals mit x ohne Fehler")
	}
}

func TestPaidStatusNullDueDate(t *testing.T) {
	entries := []map[string]any{paidEntry(1, "unpaid", "10.0", nil), paidEntry(2, "unpaid", "20.0", "2026-06-01")}
	app, _ := paidApp(t, entries...)
	doc := paidReport(t, app)
	e := doc["entries"].([]any)[1].(map[string]any)
	if e["id"] != json.Number("1") || e["due_date"] != nil || e["days_overdue"] != json.Number("0") {
		t.Errorf("ohne Fälligkeit: %v", e)
	}
	app, _ = paidApp(t, entries...)
	if ids := reportIDs(paidReport(t, app, "--overdue")); !slices.Equal(ids, []string{"2"}) {
		t.Errorf("--overdue: %v", ids)
	}
}

func TestPaidStatusSorted(t *testing.T) {
	app, _ := paidApp(t,
		paidEntry(5, "unpaid", "1.0", "2026-06-20"),
		paidEntry(1, "unpaid", "1.0", nil),
		paidEntry(9, "unpaid", "1.0", "2026-06-01"),
		paidEntry(3, "unpaid", "1.0", "2026-06-01"),
		paidEntry(2, "unpaid", "1.0", nil),
	)
	if ids := reportIDs(paidReport(t, app)); !slices.Equal(ids, []string{"3", "9", "5", "1", "2"}) {
		t.Errorf("Reihenfolge %v", ids)
	}
}

func TestPaidStatusQuery(t *testing.T) {
	app, srv := paidApp(t)
	paidReport(t, app, "--customer", "3214", "--from", "2026-01-01", "--to", "2026-03-31")
	r := wantRequests(t, srv, 1)[0]
	want := "company_id=3214&document_date_range_end=2026-03-31&document_date_range_start=2026-01-01&page=1&page_size=100"
	if r.Method != "GET" || r.Path != "/income/invoices" || r.Query.Encode() != want {
		t.Errorf("Anfrage %s %s?%s", r.Method, r.Path, r.Query.Encode())
	}
	// without filters: only the pages
	app, srv = paidApp(t)
	paidReport(t, app)
	if q := wantRequests(t, srv, 1)[0].Query.Encode(); q != "page=1&page_size=100" {
		t.Errorf("Query %s", q)
	}
	// an invalid date: exit 5 before any request
	app, srv = paidApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--from", "2026-13-01", "-o", "json"}), 5)
	wantRequests(t, srv, 0)
}

func TestPaidStatusCustomerByName(t *testing.T) {
	app, srv := paidApp(t, paidEntry(3, "unpaid", "119.0", "2026-06-01"))
	mustermann(t, srv)
	paidReport(t, app, "--customer", "Mustermann GmbH")
	reqs := wantPaths(t, srv, "GET /contact/companies", "GET /income/invoices")
	if q := reqs[1].Query.Encode(); q != "company_id=2&page=1&page_size=100" {
		t.Errorf("Query %s", q)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kunde „Mustermann GmbH“ → 2 (1 Credit)\n") {
		t.Errorf("stderr %q", errs)
	}

	// an empty name: exit 2, nothing sent
	app, srv = paidApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--customer", " ", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
}

func TestPaidStatusMaxPages(t *testing.T) {
	var pages [][]map[string]any
	for p := 1; p <= 7; p++ {
		pages = append(pages, []map[string]any{paidEntry(p, "unpaid", "1.0", "2026-06-01")})
	}
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/income/invoices", pagedList(t, 7, pages...))
	doc := paidReport(t, app)
	reqs := wantRequests(t, srv, 5)
	for i, r := range reqs {
		if r.Query.Get("page") != strconv.Itoa(i+1) {
			t.Errorf("Anfrage %d: Seite %s", i+1, r.Query.Get("page"))
		}
	}
	if doc["complete"] != false || reportNum(doc, "count") != "5" || reportNum(doc, "total_outstanding") != "5.00" {
		t.Errorf("Bericht %v", doc)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Warnung: --max-pages 5: 5 von 7 Seiten geladen; weitere Einträge vorhanden\n") {
		t.Errorf("stderr %q", errs)
	}

	app, srv = apiApp(t)
	srv.OnFunc("GET", "/income/invoices", pagedList(t, 7, pages...))
	if doc := paidReport(t, app, "--max-pages", "7"); doc["complete"] != true || reportNum(doc, "count") != "7" {
		t.Errorf("--max-pages 7: %v", doc)
	}
	wantRequests(t, srv, 7)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--max-pages", "0", "-o", "json"}), 2)
	wantRequests(t, srv, 0)

	// from 10 pages on it warns like invoice list --all (fetchPages)
	app, srv = apiApp(t)
	srv.OnFunc("GET", "/income/invoices", pagedList(t, 12, pages...))
	paidReport(t, app, "--max-pages", "12")
	if _, errs := bufs(app); !strings.Contains(errs, "Warnung: invoice paid-status lädt 12 Seiten (12 Credits); mit --max-pages begrenzen\n") {
		t.Errorf("stderr %q", errs)
	}
	wantRequests(t, srv, 8) // page 8 is empty: the end of the list
}

func TestPaidStatusMissingOutstanding(t *testing.T) {
	entries := []map[string]any{
		paidEntry(3, "unpaid", "119.0", "2026-06-01"),
		paidEntry(7, "unpaid", nil, "2026-06-05"),
		paidEntry(8, "draft", nil, "2026-06-05"), // not open whatever it owes: never read
	}
	app, srv := paidApp(t, entries...)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 1)
	wantRequests(t, srv, 1)
	wantErrText(t, app, "Rechnungsliste ohne outstanding_amount bei 1 Rechnung(en): offene Posten nicht bestimmbar",
		"mit --details je Rechnung nachlesen (1 Credit je Rechnung)")
	if out, _ := bufs(app); out != "" {
		t.Errorf("stdout %q", out)
	}

	app, srv = paidApp(t, entries...)
	srv.On("GET", "/income/invoices/7", http.StatusOK, []byte(`{"id":7,"state":"unpaid","outstanding_amount":"20.25"}`))
	doc := paidReport(t, app, "--details")
	wantPaths(t, srv, "GET /income/invoices", "GET /income/invoices/7")
	if ids := reportIDs(doc); !slices.Equal(ids, []string{"3", "7"}) || reportNum(doc, "total_outstanding") != "139.25" {
		t.Errorf("Bericht %v", doc)
	}
	if e := doc["entries"].([]any)[1].(map[string]any); e["outstanding_amount"] != json.Number("20.25") || e["customer"] != "Mustermann GmbH" {
		t.Errorf("Eintrag %v", e)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: --details liest 1 Rechnung einzeln (1 Credit)\n") {
		t.Errorf("stderr %q", errs)
	}

	// the single invoice lacks it too: exit 1
	app, srv = paidApp(t, entries...)
	srv.On("GET", "/income/invoices/7", http.StatusOK, []byte(`{"id":7}`))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--details", "-o", "json"}), 1)
	wantErrText(t, app, "Rechnung 7 ohne outstanding_amount, auch einzeln gelesen: offene Posten nicht bestimmbar", "")

	// --details without missing amounts reads nothing more
	app, srv = paidApp(t, entries[0])
	paidReport(t, app, "--details")
	wantRequests(t, srv, 1)
}

func TestPaidStatusGolden(t *testing.T) {
	entries := []map[string]any{
		paidEntry(1, "draft", 50, "2026-06-01"),
		paidEntry(3, "unpaid", "119.0", "2026-06-01"),
		paidEntry(4, "paid", "0.0", "2026-06-01"),
		paidEntry(5, "partially_paid", "50.5", "2026-06-20"),
	}
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			app, _ := paidApp(t, entries...)
			wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", format}), 0)
			out, errs := bufs(app)
			if errs != "" {
				t.Errorf("stderr %q", errs)
			}
			golden.Assert(t, "invoice-paid-status."+format, []byte(out))
		})
	}
	app, _ := paidApp(t, entries...)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status"}), 0)
	out, _ := bufs(app)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if got := lines[len(lines)-1]; got != "Summe offen: 169,50 € (2 Rechnungen, davon 1 überfällig; Stand 2026-06-10)" {
		t.Errorf("Summenzeile %q", got)
	}
	for _, h := range []string{"NR.", "KUNDE", "DATUM", "FÄLLIG", "ÜBERFÄLLIG", "BRUTTO", "OFFEN"} {
		if !strings.Contains(lines[0], h) {
			t.Errorf("Kopfzeile ohne %s: %q", h, lines[0])
		}
	}

	// no open item: the header, "Keine Einträge." and the sum 0
	app, _ = paidApp(t, entries[0])
	wantExit(t, app, Run(app, []string{"invoice", "paid-status"}), 0)
	out, errs := bufs(app)
	if !strings.HasSuffix(out, "Summe offen: 0,00 € (0 Rechnungen, davon 0 überfällig; Stand 2026-06-10)\n") || errs != "Keine Einträge.\n" {
		t.Errorf("leer: stdout %q, stderr %q", out, errs)
	}
	app, _ = paidApp(t, entries[1])
	wantExit(t, app, Run(app, []string{"invoice", "paid-status"}), 0)
	if out, _ := bufs(app); !strings.HasSuffix(out, "Summe offen: 119,00 € (1 Rechnung, davon 1 überfällig; Stand 2026-06-10)\n") {
		t.Errorf("eine Rechnung: %q", out)
	}
}

func TestPaidStatusCommandsJSON(t *testing.T) {
	c := decodeCommands(t, runCommandsJSON(t, "--json")).find(t, "invoice paid-status")
	if c.Class != "read_only" || c.Credits == nil || *c.Credits != 1 || c.OperationID != "ListInvoices" ||
		c.Method != "GET" || c.Path != "/income/invoices" || c.Blocked == nil || *c.Blocked {
		t.Errorf("Befehl %+v", c)
	}
	if f := c.flag(t, "max-pages"); f.Default == nil || *f.Default != "5" || f.Minimum == nil || *f.Minimum != 1 || f.Type != "integer" {
		t.Errorf("--max-pages %+v", f)
	}
	if f := c.flag(t, "details"); f.Credits == nil || *f.Credits != 1 || f.Type != "boolean" {
		t.Errorf("--details %+v", f)
	}
	if f := c.flag(t, "customer"); f.Resolves != "company" || f.Type != "id" || !strings.Contains(f.Description, "ID oder Name") {
		t.Errorf("--customer %+v", f)
	}
	for _, name := range []string{"from", "to"} {
		if f := c.flag(t, name); f.Type != "date" {
			t.Errorf("--%s %+v", name, f)
		}
	}
	if f := c.flag(t, "overdue"); f.Type != "boolean" {
		t.Errorf("--overdue %+v", f)
	}
	if len(c.Examples) < 2 {
		t.Errorf("Beispiele %v", c.Examples)
	}
}

func TestPaidStatusReadOnlyAllowed(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	srv.OnFunc("GET", "/income/invoices", pagedList(t, 1, []map[string]any{paidEntry(3, "unpaid", "1.0", nil)}))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 0)
	wantRequests(t, srv, 1)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--dry-run", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
}

// A page longer than the response limit ends like invoice list (exit 1,
// pageTooLarge) but with a hint of its own: paid-status has no --limit.
func TestPaidStatusPageTooLarge(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/income/invoices", tooLarge(http.StatusOK, "application/json"))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 1)
	_, errs := bufs(app)
	// M4: its own hint, paid-status has no --limit
	if !strings.Contains(lastLine(errs), `"hint":"`+hintPaidStatusTooLarge+`"`) {
		t.Errorf("stderr %s", lastLine(errs))
	}
	if !bytes.Contains([]byte(errs), []byte("64 MiB")) {
		t.Errorf("stderr %s", errs)
	}
}

// Data the report cannot use ends with exit 1 naming the invoice, before
// anything is printed; a due date with time counts as its Berlin day.
func TestPaidStatusInvalidData(t *testing.T) {
	withTime := paidEntry(3, "unpaid", "1.0", "2026-06-09T23:30:00+02:00")
	app, _ := paidApp(t, withTime)
	doc := paidReport(t, app)
	if e := doc["entries"].([]any)[0].(map[string]any); e["days_overdue"] != json.Number("1") {
		t.Errorf("Datum mit Uhrzeit: %v", e)
	}

	// total_gross is a display column (ruling T4-D): null or missing stays
	// null in JSON and empty in the table
	noGross := paidEntry(4, "unpaid", "1.0", nil)
	delete(noGross, "total_gross")
	nullGross := paidEntry(5, "unpaid", "2.0", nil)
	nullGross["total_gross"] = nil
	app, _ = paidApp(t, noGross, nullGross)
	doc = paidReport(t, app)
	for _, e := range doc["entries"].([]any) {
		if g, ok := e.(map[string]any)["total_gross"]; !ok || g != nil {
			t.Errorf("total_gross: %v", e)
		}
	}
	if reportNum(doc, "total_outstanding") != "3.00" {
		t.Errorf("Bericht %v", doc)
	}
	app, _ = paidApp(t, nullGross)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status"}), 0)
	if out, _ := bufs(app); !strings.Contains(out, "\nRE-0005  Mustermann GmbH  2026-05-01                              2,00 €\n") {
		t.Errorf("Tabelle ohne Brutto: %q", out)
	}

	badGross := paidEntry(4, "unpaid", "1.0", nil)
	badGross["total_gross"] = "x"
	for _, tc := range []struct {
		name  string
		entry any
		want  string
	}{
		{"due_date", paidEntry(3, "unpaid", "1.0", "morgen"), "Rechnung 3: ungültiges due_date „morgen“ in der Rechnungsliste"},
		{"total_gross ungültig", badGross, "Rechnung 4: ungültiger Betrag „x“ in total_gross der Rechnungsliste"},
		// M2: a number big.Rat cannot hold is no panic
		{"Betrag zu groß", paidEntry(8, "unpaid", "1e1000001", nil), "Rechnung 8: ungültiger Betrag „1e1000001“ in outstanding_amount der Rechnungsliste"},
		{"kein Objekt", 42, "unerwartete Listenantwort der Papierkram-API"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			srv.OnFunc("GET", "/income/invoices", func(w http.ResponseWriter, _ *http.Request) {
				b, _ := json.Marshal(map[string]any{"type": "list", "page": 1, "page_size": 100, "total_pages": 1,
					"total_entries": 1, "has_more": false, "entries": []any{tc.entry}})
				_, _ = w.Write(b)
			})
			wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 1)
			out, errs := bufs(app)
			if out != "" || !strings.Contains(lastLine(errs), tc.want) || !strings.Contains(lastLine(errs), `"code":"error"`) {
				t.Errorf("stdout %q, stderr %s", out, errs)
			}
		})
	}
}

// --details: an error of the single read ends the command with its exit
// code; an answer that is no object is exit 1.
func TestPaidStatusDetailsErrors(t *testing.T) {
	entries := []map[string]any{paidEntry(7, "unpaid", nil, nil)}
	app, srv := paidApp(t, entries...)
	srv.On("GET", "/income/invoices/7", http.StatusNotFound, apiError("Rechnung nicht gefunden"))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--details", "-o", "json"}), 4)

	app, srv = paidApp(t, entries...)
	srv.On("GET", "/income/invoices/7", http.StatusOK, []byte(`[1]`))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--details", "-o", "json"}), 1)
	if _, errs := bufs(app); !strings.Contains(lastLine(errs), "unerwartete Antwort der Papierkram-API zu Rechnung 7") {
		t.Errorf("stderr %s", errs)
	}
}

// Without a token exit 3 before any request; a customer name not found is
// exit 4 after the company list, no invoice list.
func TestPaidStatusClientAndNameErrors(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": ""})
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "json"}), 3)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	mustermann(t, srv)
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--customer", "Unbekannt", "-o", "json"}), 4)
	wantPaths(t, srv, "GET /contact/companies")
}

// -o wide adds ID, NAME and STATUS; --fields picks the columns of the
// entries and keeps the sum line.
func TestPaidStatusWideAndFields(t *testing.T) {
	app, _ := paidApp(t, paidEntry(3, "unpaid", "119.0", "2026-06-01"))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "-o", "wide"}), 0)
	out, _ := bufs(app)
	lines := strings.Split(out, "\n")
	for _, h := range []string{"NR.", "OFFEN", "ID", "NAME", "STATUS"} {
		if !strings.Contains(lines[0], h) {
			t.Errorf("Kopfzeile ohne %s: %q", h, lines[0])
		}
	}
	if !strings.Contains(lines[1], "unpaid") || !strings.Contains(lines[1], "Beratung") {
		t.Errorf("Zeile %q", lines[1])
	}

	app, _ = paidApp(t, paidEntry(3, "unpaid", "119.0", "2026-06-01"))
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--fields", "invoice_no,outstanding_amount"}), 0)
	if out, _ := bufs(app); out != "NR.      OFFEN\nRE-0003  119,00 €\nSumme offen: 119,00 € (1 Rechnung, davon 1 überfällig; Stand 2026-06-10)\n" {
		t.Errorf("--fields: %q", out)
	}
}

// M6 (ruling T4-E): from warnPages invoices to read, --details warns like
// list --all; without a terminal it proceeds, in one a "no" is exit 8
// before any single read.
func TestPaidStatusDetailsCostBrake(t *testing.T) {
	var entries []map[string]any
	for id := 1; id <= warnPages; id++ {
		entries = append(entries, paidEntry(id, "unpaid", nil, nil))
	}
	details := func(t *testing.T, srv *mockapi.Server) {
		srv.OnFunc("GET", "/income/invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"outstanding_amount":"1.0"}`))
		})
	}
	warn := "Warnung: --details liest 10 Rechnungen einzeln (10 Credits); mit --customer, --from oder --to eingrenzen\n"

	app, srv := paidApp(t, entries...)
	details(t, srv)
	doc := paidReport(t, app, "--details")
	wantRequests(t, srv, 1+warnPages)
	if reportNum(doc, "count") != "10" || reportNum(doc, "total_outstanding") != "10.00" {
		t.Errorf("Bericht %v", doc)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: --details liest 10 Rechnungen einzeln (10 Credits)\n"+warn) {
		t.Errorf("stderr %q", errs)
	}

	app, srv = paidApp(t, entries...)
	details(t, srv)
	app.StdinTTY = true
	app.In = strings.NewReader("n\n")
	wantExit(t, app, Run(app, []string{"invoice", "paid-status", "--details", "-o", "json"}), 8)
	wantPaths(t, srv, "GET /income/invoices")
	_, errs := bufs(app)
	if !strings.Contains(errs, warn) || !strings.Contains(errs, "Fortfahren? [j/N] ") {
		t.Errorf("stderr %q", errs)
	}
	wantErrText(t, app, "Abgebrochen: --details nicht fortgesetzt (nur die Rechnungsliste geladen)",
		"Mit --customer, --from oder --to eingrenzen oder mit --yes ohne Rückfrage laden")

	// one below the limit: no warning
	app, srv = paidApp(t, entries[:warnPages-1]...)
	details(t, srv)
	paidReport(t, app, "--details")
	if _, errs := bufs(app); strings.Contains(errs, "Warnung") {
		t.Errorf("stderr %q", errs)
	}
}

// M7: today is the Berlin day, also on a host in UTC shortly before
// midnight UTC (00:30 in Berlin).
func TestPaidStatusBerlinDayOnUTCHost(t *testing.T) {
	app, _ := paidApp(t, paidEntry(3, "unpaid", "1.0", "2026-06-09"))
	app.Location = time.UTC
	app.Now = func() time.Time { return time.Date(2026, 6, 9, 22, 30, 0, 0, time.UTC) }
	doc := paidReport(t, app)
	e := doc["entries"].([]any)[0].(map[string]any)
	if doc["as_of"] != "2026-06-10" || e["due_date"] != "2026-06-09" || e["days_overdue"] != json.Number("1") {
		t.Errorf("Bericht %v", doc)
	}
}

// M8: the filter flags carry the annotations of the filter flags of
// invoice list (type, values, bounds, help).
func TestPaidStatusFilterFlagsLikeInvoiceList(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	ps, il := d.find(t, "invoice paid-status"), d.find(t, "invoice list")
	for flag, listFlag := range map[string]string{"customer": "company", "from": "from", "to": "to"} {
		a, b := ps.flag(t, flag), il.flag(t, listFlag)
		if a.Type != b.Type || !slices.Equal(a.Enum, b.Enum) || !sameBound(a.Minimum, b.Minimum) || !sameBound(a.Maximum, b.Maximum) ||
			a.Resolves != b.Resolves {
			t.Errorf("--%s %+v\nweicht ab von invoice list --%s %+v", flag, a, listFlag, b)
		}
		if flag != "customer" && a.Description != b.Description {
			t.Errorf("--%s Hilfe %q, invoice list %q", flag, a.Description, b.Description)
		}
	}
}

func sameBound(a, b *float64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
