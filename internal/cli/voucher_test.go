package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// voucher commands (spec 7.1, 15 no. 19–27). pay, pdf, upload and document
// delete are registered as Custom (Tasks 32, 33, 35) and not built yet.

// TestVoucherCommandsGolden renders list and describe of voucher from the
// spec examples of the mock in every output format.
func TestVoucherCommandsGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"voucher-list", []string{"voucher", "list"}, "/expense/vouchers", url.Values{"page_size": {"25"}}},
		{"voucher-describe", []string{"voucher", "describe", "295"}, "/expense/vouchers/295", url.Values{}},
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

func TestVoucherAliases(t *testing.T) {
	for _, args := range [][]string{
		{"beleg", "list", "-o", "json"},
		{"expense", "describe", "295", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

func TestVoucherListFilters(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "list", "--creditor", "2904", "--project", "0",
		"--from", "2026-01-01", "--to", "2026-03-31", "-o", "json"}), 0)
	q := wantRequests(t, srv, 1)[0].Query
	want := url.Values{
		"creditor_id": {"2904"}, "project_id": {"0"}, "page_size": {"25"},
		"document_date_range_start": {"2026-01-01"}, "document_date_range_end": {"2026-03-31"},
	}
	if q.Encode() != want.Encode() {
		t.Errorf("query = %s, erwartet %s", q.Encode(), want.Encode())
	}

	// relative dates use the app clock
	app, srv = berlinApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "list", "--from", "gestern", "--to", "heute", "-o", "json"}), 0)
	if q := wantRequests(t, srv, 1)[0].Query; q.Get("document_date_range_start") != "2026-10-25" || q.Get("document_date_range_end") != "2026-10-26" {
		t.Errorf("query = %v", q)
	}

	// an invalid date is a validation error, a non-numeric ID a flag error
	for _, tc := range []struct {
		args []string
		exit int
	}{
		{[]string{"voucher", "list", "--to", "31.03.2026"}, 5},
		{[]string{"voucher", "list", "--creditor", "abc"}, 2},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), tc.exit)
		wantRequests(t, srv, 0)
	}
}

func TestVoucherCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "create", "--name", "Bewirtung Kunde", "--provenance", "domestic",
		"--creditor", "2904", "--document-date", "2026-11-04", "--due-date", "2026-11-30",
		"--entertainment-reason", "Projektbesprechung", "--entertainment-person", "Anna Muster",
		"--item", "name=Restaurant,amount=150.80,vat=19,category=Bewirtungskosten",
		"--item", `name="Trinkgeld, bar",amount=15,vat=0%,category=Bewirtungskosten`, "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/expense/vouchers", `{"name":"Bewirtung Kunde","provenance":"domestic","creditor":{"id":2904},`+
		`"document_date":"2026-11-04","due_date":"2026-11-30","entertainment_reason":"Projektbesprechung",`+
		`"entertainment_persons":["Anna Muster"],"line_items":[`+
		`{"name":"Restaurant","amount":150.80,"vat_rate":0.19,"category":"Bewirtungskosten"},`+
		`{"name":"Trinkgeld, bar","amount":15,"vat_rate":0,"category":"Bewirtungskosten"}]}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "expense_voucher"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
}

// Spec R17 at the CLI level (TestR17_LineItemsRequired covers the
// validation): create without positions is exit 5 before any request.
func TestVoucherCreateWithoutLineItems(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"voucher", "create", "--name", "Bewirtung", "--provenance", "domestic"}, ""},
		{[]string{"voucher", "create", "--input", "-", "--dry-run"}, `{"name":"Bewirtung","provenance":"domestic","line_items":[]}`},
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

// Spec R28 at the CLI level (TestR28_CategoryTypoThreeSuggestions covers
// the validation): a category with a typo is exit 5 with the three closest
// values, before any request, for --item, --input and --dry-run alike.
func TestVoucherCategoryTypoSuggestsThreeValues(t *testing.T) {
	const want = `ungültiger Wert "Bürobedraf" für line_items[0].category – meintest du "Bürobedarf", "Büroartikel" oder "Betriebsbedarf"?`
	for _, tc := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"voucher", "create", "--name", "Papier", "--provenance", "domestic",
			"--item", "name=Papier,amount=11.90,vat=19,category=Bürobedraf"}, ""},
		{[]string{"voucher", "create", "--name", "Papier", "--provenance", "domestic",
			"--item", "name=Papier,amount=11.90,vat=19,category=Bürobedraf", "--dry-run"}, ""},
		{[]string{"voucher", "create", "--input", "-"},
			`{"name":"Papier","provenance":"domestic","line_items":[{"name":"Papier","amount":11.9,"vat_rate":0.19,"category":"Bürobedraf"}]}`},
		{[]string{"voucher", "update", "295", "--item", "name=Papier,amount=11.90,vat=19,category=Bürobedraf", "--replace-items"}, ""},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.stdin)
		wantExit(t, app, Run(app, tc.args), 5)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, want) {
			t.Errorf("%v: stdout = %q, stderr = %q", tc.args, out, errs)
		}
	}
}

// Spec R12 at the CLI level (TestR12_ProvenanceNonEuSuggestsForeign covers
// the validation): provenance non_eu is exit 5 with the suggestion foreign
// from x-cli-enum-hints, before any request, as flag and from --input.
func TestVoucherProvenanceNonEuSuggestsForeign(t *testing.T) {
	for _, tc := range []struct {
		args        []string
		stdin, want string
	}{
		{[]string{"voucher", "create", "--name", "Hosting", "--provenance", "non_eu",
			"--item", "name=Server,amount=20,vat=0,category=Bürobedarf"}, "",
			`ungültiger Wert "non_eu" für --provenance – meintest du "foreign"?`},
		{[]string{"voucher", "update", "295", "--provenance", "non_eu", "--dry-run"}, "",
			`ungültiger Wert "non_eu" für --provenance – meintest du "foreign"?`},
		{[]string{"voucher", "create", "--input", "-"},
			`{"name":"Hosting","provenance":"non_eu","line_items":[{"name":"Server","amount":20,"vat_rate":0,"category":"Bürobedarf"}]}`,
			`ungültiger Wert "non_eu" für provenance – meintest du "foreign"?`},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.stdin)
		wantExit(t, app, Run(app, tc.args), 5)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, tc.want) {
			t.Errorf("%v: stdout = %q, stderr = %q", tc.args, out, errs)
		}
	}
}

// VAT 1 is ambiguous (1 % or 100 %?): exit 5, nothing sent.
func TestVoucherAmbiguousVATRejected(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "create", "--name", "V", "--provenance", "domestic",
		"--item", "name=A,amount=1,vat=1,category=Bürobedarf"}), 5)
	wantRequests(t, srv, 0)
}

func TestVoucherUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "update", "295", "--name", "Bewirtung Kunde", "--due-date", "2026-12-15",
		"--provenance", "eu", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/expense/vouchers/295", `{"name":"Bewirtung Kunde","due_date":"2026-12-15","provenance":"eu"}`)

	// positions only with --replace-items; nothing to change is exit 2
	for _, args := range [][]string{
		{"voucher", "update", "295", "--item", "name=A,amount=5,vat=19,category=Bürobedarf"},
		{"voucher", "update", "295"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "update", "295", "--item", "name=A,amount=5,vat=7,category=Bürobedarf",
		"--replace-items", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/expense/vouchers/295", `{"line_items":[{"name":"A","amount":5,"vat_rate":0.07,"category":"Bürobedarf"}]}`)
}

// describe -o json | update --input - sends a spec-conformant PUT (the mock
// validates it): the supplier becomes {id}, read-only fields go, positions
// only with --replace-items, then with category and vat_rate 0.19.
func TestVoucherDescribeUpdateRoundTrip(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "describe", "295", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	described := mustOut(app)

	app, srv = apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, Run(app, []string{"voucher", "update", "295", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/expense/vouchers/295", `{"name":"Restaurant visit","description":"Took customer for dinner.",`+
		`"document_date":"2020-06-14","due_date":"2020-06-30","entertainment_persons":[],"entertainment_reason":"sales meeting",`+
		`"flagged":true,"provenance":"domestic","creditor":{"id":2904}}`)
	_, errs := bufs(app)
	if !strings.Contains(errs, itemsDropHint) || !strings.Contains(errs, "nur lesbare Felder entfernt") {
		t.Errorf("stderr = %q", errs)
	}
	for _, field := range []string{"id", "type", "state", "record_state", "voucher_no", "amount", "invoice_amount", "documents", "creditor.name"} {
		if !strings.Contains(errs, field) {
			t.Errorf("Hinweis nennt %q nicht: %q", field, errs)
		}
	}

	app, srv = apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, Run(app, []string{"voucher", "update", "295", "--input", "-", "--replace-items", "-o", "json"}), 0)
	body := decodeBody(t, wantRequests(t, srv, 1)[0].Body)
	items, _ := body["line_items"].([]any)
	if len(items) != 2 {
		t.Fatalf("line_items = %v", body["line_items"])
	}
	for i, want := range []struct {
		name   string
		amount json.Number
	}{{"restaurant bill", "150.8"}, {"tip", "15.0"}} {
		item, _ := items[i].(map[string]any)
		if item["name"] != want.name || item["amount"] != want.amount || item["vat_rate"] != json.Number("0.19") ||
			item["category"] != "Bewirtungskosten" {
			t.Errorf("Position %d = %v", i, item)
		}
	}
}

func TestVoucherDeleteNeedsConfirmation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "delete", "295"}), 8)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "delete", "295", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/expense/vouchers/295" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 295,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

// archive is not destructive (spec 8.1): no confirmation, also without TTY.
func TestVoucherArchiveWithoutConfirmation(t *testing.T) {
	for _, action := range []string{"archive", "unarchive"} {
		app, srv := apiApp(t) // no TTY, no --yes
		wantExit(t, app, Run(app, []string{"voucher", action, "7", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/expense/vouchers/7/"+action {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
	}
}

// cancel and reverse (cancel_with_reverse_entry) are destructive (spec
// 8.1): without TTY and --yes exit 8, nothing sent; with --yes the POST
// goes out.
func TestVoucherCancelAndReverseNeedConfirmation(t *testing.T) {
	for _, tc := range []struct{ action, path string }{
		{"cancel", "/expense/vouchers/7/cancel"},
		{"reverse", "/expense/vouchers/7/cancel_with_reverse_entry"},
	} {
		for _, extra := range [][]string{nil, {"--no-input"}} {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, append([]string{"voucher", tc.action, "7"}, extra...)), 8)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
				t.Errorf("%s %v: stderr = %q", tc.action, extra, errs)
			}
		}

		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{"voucher", tc.action, "7", "--yes", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != tc.path {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
		if out := mustOut(app); !strings.Contains(out, `"type": "expense_voucher"`) {
			t.Errorf("%s: stdout = %s", tc.action, out)
		}
	}
}

// pay, pdf, upload and document delete are Custom entries: registered, but
// no generic command until their handlers exist (Tasks 32, 33, 35).
func TestVoucherCustomActionsNotBuilt(t *testing.T) {
	for _, args := range [][]string{
		{"voucher", "pay", "7", "--yes"},
		{"voucher", "pdf", "7"},
		{"voucher", "upload", "7", "beleg.pdf"},
		{"voucher", "document", "delete", "7", "3", "--yes"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher"}), 0)
	help := mustOut(app)
	var actions []string
	for _, a := range []string{"list", "describe", "create", "update", "delete", "archive", "unarchive", "cancel", "reverse"} {
		if strings.Contains(help, "  "+a+" ") {
			actions = append(actions, a)
		}
	}
	if len(actions) != 9 {
		t.Errorf("Gruppenhilfe %v:\n%s", actions, help)
	}
	for _, a := range []string{"pay", "pdf", "upload", "document"} {
		if strings.Contains(help, "  "+a+" ") {
			t.Errorf("Gruppenhilfe nennt %s:\n%s", a, help)
		}
	}
}

// The registered mutations run the protection layer (Task 24).
func TestVoucherMutationsDryRun(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		method, path  string
		credits, body string
	}{
		{[]string{"voucher", "create", "--name", "V", "--provenance", "foreign", "--item", "name=A,amount=5,vat=19,category=Bürobedarf"},
			"POST", "/expense/vouchers", `"credits": 50`, `"category": "Bürobedarf"`},
		{[]string{"voucher", "update", "295", "--name", "Neu"},
			"PUT", "/expense/vouchers/295", `"credits": 1`, `"name": "Neu"`},
		{[]string{"voucher", "cancel", "295"},
			"POST", "/expense/vouchers/295/cancel", `"credits": 1`, `"body": null`},
		{[]string{"voucher", "reverse", "295"},
			"POST", "/expense/vouchers/295/cancel_with_reverse_entry", `"credits": 1`, `"body": null`},
		{[]string{"voucher", "delete", "295"},
			"DELETE", "/expense/vouchers/295", `"credits": 1`, `"body": null`},
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

func TestVoucherMutationsReadOnly(t *testing.T) {
	for _, args := range [][]string{
		{"voucher", "create", "--name", "V", "--provenance", "domestic", "--item", "name=A,amount=5,vat=19,category=Bürobedarf"},
		{"voucher", "update", "7", "--name", "Neu"},
		{"voucher", "delete", "7", "--yes"},
		{"voucher", "archive", "7"},
		{"voucher", "unarchive", "7"},
		{"voucher", "cancel", "7", "--yes"},
		{"voucher", "reverse", "7", "--yes"},
		{"voucher", "reverse", "7", "--dry-run"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
	}
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"voucher", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
}
