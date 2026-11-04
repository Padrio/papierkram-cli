package cli

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// invoice and estimate commands (spec 7.1, 15 no. 30–37, 40–47). deliver
// has its own handler (deliver.go, Task 31); pdf is registered as Custom
// (Task 32) and not built yet.

// TestIncomeCommandsGolden renders list and describe of invoice and
// estimate from the spec examples of the mock in every output format.
func TestIncomeCommandsGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"invoice-list", []string{"invoice", "list"}, "/income/invoices", url.Values{"page_size": {"25"}}},
		{"invoice-describe", []string{"invoice", "describe", "459"}, "/income/invoices/459", url.Values{}},
		{"estimate-list", []string{"estimate", "list"}, "/income/estimates", url.Values{"page_size": {"25"}}},
		{"estimate-describe", []string{"estimate", "describe", "349"}, "/income/estimates/349", url.Values{}},
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

func TestIncomeAliases(t *testing.T) {
	for _, args := range [][]string{
		{"rechnung", "list", "-o", "json"},
		{"angebot", "describe", "349", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

func TestIncomeListFilters(t *testing.T) {
	for _, resource := range []string{"invoice", "estimate"} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{resource, "list", "--company", "3214", "--project", "0",
			"--from", "2026-01-01", "--to", "2026-03-31", "-o", "json"}), 0)
		q := wantRequests(t, srv, 1)[0].Query
		want := url.Values{
			"company_id": {"3214"}, "project_id": {"0"}, "page_size": {"25"},
			"document_date_range_start": {"2026-01-01"}, "document_date_range_end": {"2026-03-31"},
		}
		if q.Encode() != want.Encode() {
			t.Errorf("%s: query = %s, erwartet %s", resource, q.Encode(), want.Encode())
		}
	}

	// relative dates use the app clock
	app, srv := berlinApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "list", "--from", "gestern", "--to", "heute", "-o", "json"}), 0)
	if q := wantRequests(t, srv, 1)[0].Query; q.Get("document_date_range_start") != "2026-10-25" || q.Get("document_date_range_end") != "2026-10-26" {
		t.Errorf("query = %v", q)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "list", "--from", "1.1.2026"}), 5)
	wantRequests(t, srv, 0)
}

// Spec R17 at the CLI level (TestR17_LineItemsRequired covers the
// validation): create without positions is exit 5 before any request,
// also when the line items come empty from --input.
func TestIncomeCreateWithoutLineItems(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"invoice", "create", "--name", "Beratung", "--payment-term", "1130"}, ""},
		{[]string{"invoice", "create", "--input", "-"}, `{"name":"Beratung","payment_term":{"id":1130},"line_items":[]}`},
		{[]string{"estimate", "create", "--name", "Angebot Oktober"}, ""},
		{[]string{"estimate", "create", "--input", "-", "--dry-run"}, `{"name":"Angebot Oktober","line_items":[]}`},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.stdin)
		wantExit(t, app, Run(app, tc.args), 5)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, "line_items") {
			t.Errorf("%v: stdout = %q, stderr = %q", tc.args, out, errs)
		}
	}
}

// The API rejects an estimate without document_date with 422 "Datum muss
// ausgefüllt werden" (measured live 2026-11-04) and still charges the 50
// credits of the create. The CLI checks it before the request (exit 5),
// with flags as with --input, and null counts as missing.
func TestEstimateCreateRequiresDocumentDate(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"estimate", "create", "--name", "A", "--item", "name=A,qty=1,price=5,vat=19"}, ""},
		{[]string{"estimate", "create", "--name", "A", "--item", "name=A,qty=1,price=5,vat=19", "--dry-run"}, ""},
		{[]string{"estimate", "create", "--input", "-"}, `{"name":"A","line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`},
		{[]string{"estimate", "create", "--input", "-"}, `{"name":"A","document_date":null,"line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.stdin)
		wantExit(t, app, Run(app, append(tc.args, "-o", "json")), 5)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, "document_date") {
			t.Errorf("%v %s: stdout = %q, stderr = %q", tc.args, tc.stdin, out, errs)
		}
	}

	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "create", "--help"}), 0)
	wantRequests(t, srv, 0)
	out, _ := bufs(app)
	i := strings.Index(out, "--document-date Datum")
	if line, _, _ := strings.Cut(out[max(i, 0):], "\n"); i < 0 || !strings.Contains(line, "(Pflicht)") {
		t.Errorf("Hilfe nennt --document-date nicht als Pflicht:\n%s", out)
	}
}

func TestInvoiceCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--name", "Beratung Oktober", "--customer", "3214",
		"--contact-person", "1205", "--payment-term", "1130", "--document-date", "2026-11-04",
		"--item", "name=Beratung,qty=2,price=100,vat=19", "--item", `name="Fahrt, vor Ort",qty=1,price=40,vat=7%`, "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{"name":"Beratung Oktober","customer":{"id":3214,"contact_person":{"id":1205}},`+
		`"payment_term":{"id":1130},"document_date":"2026-11-04","line_items":[`+
		`{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19},{"name":"Fahrt, vor Ort","quantity":1,"price":40,"vat_rate":0.07}]}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "invoice"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
}

func TestEstimateCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "create", "--name", "Angebot Website", "--customer", "3052",
		"--customer-project", "4316", "--document-date", "2026-11-04", "--item", "name=Konzeption,qty=3,unit=Tag,price=800,vat=19", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/income/estimates", `{"name":"Angebot Website","customer":{"id":3052,"project":{"id":4316}},"document_date":"2026-11-04",`+
		`"line_items":[{"name":"Konzeption","quantity":3,"unit":"Tag","price":800,"vat_rate":0.19}]}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "estimate"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
}

func TestIncomeUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--name", "Beratung November", "--payment-term", "1131", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/invoices/459", `{"name":"Beratung November","payment_term":{"id":1131}}`)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "update", "349", "--description", "Gültig bis Jahresende", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/estimates/349", `{"description":"Gültig bis Jahresende"}`)

	// positions only with --replace-items; nothing to change is exit 2
	for _, args := range [][]string{
		{"invoice", "update", "459", "--item", "name=A,qty=1,price=5,vat=19"},
		{"estimate", "update", "349"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "update", "349", "--item", "name=A,qty=1,price=5,vat=7", "--replace-items", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/estimates/349", `{"line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.07}]}`)
}

// describeJSON runs describe -o json and returns stdout, the input of the
// round trip (spec 6.3a: describe -o json | update --input -).
func describeJSON(t *testing.T, resource, id string) string {
	t.Helper()
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{resource, "describe", id, "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	out, _ := bufs(app)
	if !strings.Contains(out, `"line_items"`) || !strings.Contains(out, `"vat_rate": "19%"`) {
		t.Fatalf("%s describe ohne Positionen mit vat_rate \"19%%\": %s", resource, out)
	}
	return out
}

// roundTripPruned are read-only fields the round trip removes from a
// describe -o json of invoice or estimate and names in its hint (spec 6.3,
// R14).
var roundTripPruned = []string{"id", "type", "state", "record_state", "total_gross", "customer.name",
	"document_charges"}

// Spec R14 at the CLI level (TestR14_UpdateInputPrunesReadOnly covers the
// pruning): describe -o json | update --input - sends a spec-conformant PUT
// (the mock validates it) without read-only fields and, without
// --replace-items, without positions. Contact person and project of the
// answer go below customer: without them the live API clears both
// (measured 2026-11-04).
func TestIncomeDescribeUpdateRoundTrip(t *testing.T) {
	// the billing block is writable and kept; nulls of nullable fields too
	const billing = `"billing":{"company":"Mustermann Automobile GmbH","email":null,"ust_idnr":"11/234/34567",` +
		`"street":"Dotzheimer Str. 36","zip":"65185","city":"Wiesbaden","country":"Deutschland","contact_person":"John Doe","department":"Purchasing"}`
	for _, tc := range []struct{ resource, id, path, want string }{
		{"invoice", "459", "/income/invoices/459", `{"name":"My invoice","description":"Notes for the invoice","document_date":"2020-05-01",` +
			`"supply_date":"2026-08-28 13:41:34 +0200","gross":false,"custom_template":{"id":220},` + billing +
			`,"customer":{"id":3214,"contact_person":{"id":1205},"project":{"id":4576}},"payment_term":{"id":1130}}`},
		{"estimate", "349", "/income/estimates/349", `{"name":"My estimate","description":"Notes for the estimate","document_date":"2020-05-01",` +
			`"sent_on":null,"sent_to":null,"gross":false,` + billing + `,"customer":{"id":3052,"contact_person":{"id":1116},"project":{"id":4316}}}`},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			described := describeJSON(t, tc.resource, tc.id)
			app, srv := apiApp(t)
			app.In = strings.NewReader(described)
			wantExit(t, app, Run(app, []string{tc.resource, "update", tc.id, "--input", "-", "-o", "json"}), 0)
			// the mock validates the PUT against the spec (auto-fail)
			wantBody(t, srv, "PUT", tc.path, tc.want)
			_, errs := bufs(app)
			if !strings.Contains(errs, itemsDropHint) || !strings.Contains(errs, "nur lesbare Felder entfernt") ||
				!strings.Contains(errs, "Hinweis: Referenzen übernommen: contact_person → customer.contact_person, project → customer.project\n") {
				t.Errorf("stderr = %q", errs)
			}
			for _, field := range roundTripPruned {
				if !strings.Contains(errs, field) {
					t.Errorf("Hinweis nennt %q nicht: %q", field, errs)
				}
			}
		})
	}
}

// Spec R31 at the CLI level (TestR31_DescribeRoundTripKeepsItems covers
// the pruning): with --replace-items the round trip sends the positions
// with their writable fields kept and vat_rate "19%" normalized to 0.19.
func TestIncomeDescribeUpdateRoundTripReplaceItems(t *testing.T) {
	for _, tc := range []struct{ resource, id string }{
		{"invoice", "459"},
		{"estimate", "349"},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			described := describeJSON(t, tc.resource, tc.id)
			app, srv := apiApp(t)
			app.In = strings.NewReader(described)
			wantExit(t, app, Run(app, []string{tc.resource, "update", tc.id, "--input", "-", "--replace-items", "-o", "json"}), 0)
			body := decodeBody(t, wantRequests(t, srv, 1)[0].Body)
			items, _ := body["line_items"].([]any)
			if len(items) != 2 {
				t.Fatalf("line_items = %v", body["line_items"])
			}
			for i, want := range []struct {
				name string
				vat  json.Number
			}{{"Büroartikel", "0.19"}, {"Heizung", "0.07"}} {
				item, _ := items[i].(map[string]any)
				if item["name"] != want.name || item["vat_rate"] != want.vat ||
					item["description"] != "Beschreibung einer Rechnungsposition" || item["unit"] != "Stunde" ||
					item["quantity"] != json.Number("1.0") || item["price"] != json.Number("100.0") || item["type_of"] != "position" ||
					item["price_gross"] != json.Number("0.0") || item["discount_percentage"] != json.Number("0.0") {
					t.Errorf("Position %d = %v", i, item)
				}
				for _, key := range []string{"total_net", "total_vat", "total_gross", "document_charge", "position_in_group"} {
					if _, ok := item[key]; ok {
						t.Errorf("Position %d enthält %q: %v", i, key, item)
					}
				}
			}
			if _, errs := bufs(app); strings.Contains(errs, "nicht gesendet") {
				t.Errorf("stderr = %q", errs)
			}
		})
	}
}

// decodeBody decodes a request body with numbers kept as text.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	var body map[string]any
	if err := d.Decode(&body); err != nil {
		t.Fatalf("Body kein JSON-Objekt: %v: %s", err, raw)
	}
	return body
}

func TestIncomeDeleteNeedsConfirmation(t *testing.T) {
	for _, tc := range []struct{ resource, path string }{
		{"invoice", "/income/invoices/459"},
		{"estimate", "/income/estimates/349"},
	} {
		id := tc.path[strings.LastIndexByte(tc.path, '/')+1:]
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{tc.resource, "delete", id}), 8)
		wantRequests(t, srv, 0)

		app, srv = apiApp(t)
		wantExit(t, app, Run(app, []string{tc.resource, "delete", id, "--yes", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != tc.path {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
		if out := mustOut(app); out != "{\n  \"id\": "+id+",\n  \"deleted\": true\n}\n" {
			t.Errorf("stdout = %q", out)
		}
	}
}

// archive is not destructive (spec 8.1): no confirmation, also without TTY.
func TestIncomeArchiveWithoutConfirmation(t *testing.T) {
	for _, resource := range []string{"invoice", "estimate"} {
		for _, action := range []string{"archive", "unarchive"} {
			app, srv := apiApp(t) // no TTY, no --yes
			wantExit(t, app, Run(app, []string{resource, action, "7", "-o", "json"}), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/income/"+resource+"s/7/"+action {
				t.Errorf("request = %s %s", r.Method, r.Path)
			}
		}
	}
}

// cancel is destructive (spec 8.1): without TTY and --yes exit 8, nothing
// sent; with --yes the POST goes out.
func TestIncomeCancelNeedsConfirmation(t *testing.T) {
	for _, resource := range []string{"invoice", "estimate"} {
		for _, extra := range [][]string{nil, {"--no-input"}} {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, append([]string{resource, "cancel", "7"}, extra...)), 8)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
				t.Errorf("%s %v: stderr = %q", resource, extra, errs)
			}
		}

		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{resource, "cancel", "7", "--yes", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/income/"+resource+"s/7/cancel" {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
	}
}

// All invoice and estimate actions are built, the Custom ones (deliver,
// pdf) by their handlers.
func TestIncomeCustomActionsBuilt(t *testing.T) {
	for _, resource := range []string{"invoice", "estimate"} {
		app, _ := apiApp(t)
		wantExit(t, app, Run(app, []string{resource}), 0)
		help := mustOut(app)
		var actions []string
		for _, a := range []string{"list", "describe", "create", "update", "delete", "archive", "unarchive", "cancel", "deliver", "pdf"} {
			if strings.Contains(help, "  "+a+" ") {
				actions = append(actions, a)
			}
		}
		if len(actions) != 10 {
			t.Errorf("%s: Gruppenhilfe %v:\n%s", resource, actions, help)
		}
	}
}

// The registered mutations run the protection layer (Task 24).
func TestIncomeMutationsDryRun(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		method, path  string
		credits, body string
	}{
		{[]string{"invoice", "create", "--name", "R", "--payment-term", "1130", "--item", "name=A,qty=1,price=5,vat=19"},
			"POST", "/income/invoices", `"credits": 50`, `"vat_rate": 0.19`},
		{[]string{"estimate", "create", "--name", "A", "--document-date", "2026-11-04", "--item", "name=A,qty=1,price=5,vat=19"},
			"POST", "/income/estimates", `"credits": 50`, `"name": "A"`},
		{[]string{"invoice", "update", "459", "--name", "Neu"},
			"PUT", "/income/invoices/459", `"credits": 1`, `"name": "Neu"`},
		{[]string{"estimate", "cancel", "349"},
			"POST", "/income/estimates/349/cancel", `"credits": 1`, `"body": null`},
		{[]string{"invoice", "delete", "459"},
			"DELETE", "/income/invoices/459", `"credits": 1`, `"body": null`},
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

func TestIncomeMutationsReadOnly(t *testing.T) {
	var cases [][]string
	for _, resource := range []string{"invoice", "estimate"} {
		cases = append(cases,
			[]string{resource, "create", "--name", "X", "--payment-term", "1", "--item", "name=A,qty=1,price=5,vat=19"},
			[]string{resource, "update", "7", "--name", "Neu"},
			[]string{resource, "delete", "7", "--yes"},
			[]string{resource, "archive", "7"},
			[]string{resource, "unarchive", "7"},
			[]string{resource, "cancel", "7", "--yes"},
			[]string{resource, "cancel", "7", "--dry-run"},
		)
	}
	for _, args := range cases {
		if args[0] == "estimate" && slices.Contains(args, "--payment-term") {
			args = slices.Delete(args, 4, 6) // estimates have no payment term
		}
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
	}
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"invoice", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
}
