package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// invoice create --from-estimate (spec 9.4).

// fromEstimateArgs creates an invoice from estimate 349 (the spec example
// the mock answers with).
var fromEstimateArgs = []string{"invoice", "create", "--from-estimate", "349", "--payment-term", "1130"}

func fromEstimateCmd(extra ...string) []string {
	return append(append([]string{}, fromEstimateArgs...), extra...)
}

// indentJSON is raw with sorted keys, numbers as sent, indented by two
// spaces and with a final newline (the form of the golden files).
func indentJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(canonJSON(t, raw)), "", "  "); err != nil {
		t.Fatal(err)
	}
	b.WriteByte('\n')
	return b.Bytes()
}

// The body is built from the estimate of the spec example: references
// nested under customer, billing, every position with its VAT rate
// normalized ("19%" → 0.19), decimals exactly as the API sent them; the
// read-only fields of the estimate are not sent.
func TestInvoiceFromEstimateBody(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("-o", "json")), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "GET" || reqs[0].Path != "/income/estimates/349" {
		t.Errorf("read = %s %s", reqs[0].Method, reqs[0].Path)
	}
	if reqs[1].Method != "POST" || reqs[1].Path != "/income/invoices" {
		t.Errorf("create = %s %s", reqs[1].Method, reqs[1].Path)
	}
	golden.Assert(t, "invoice-from-estimate.body.json", indentJSON(t, reqs[1].Body))
	if out, errs := bufs(app); !strings.Contains(out, `"type": "invoice"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
	assertNoToken(t, app)
}

// estimateJSON answers the estimate read with body.
func estimateJSON(srv *mockapi.Server, body string) {
	srv.On("GET", "/income/estimates/{id}", http.StatusOK, []byte(body))
}

// Decimals stay exact (no float32 of the generated model), a proposition
// reference is kept when set, a null is sent only where the request allows
// it (description, billing.email, discount_percentage; not gross or the
// description of a position), missing values are not sent.
func TestInvoiceFromEstimateExactValues(t *testing.T) {
	app, srv := apiApp(t)
	estimateJSON(srv, `{"id":5,"name":"Angebot","description":null,"gross":null,"customer":{"id":0,"name":"A"},
		"contact_person":null,"project":{"id":7,"name":"P"},"billing":{"company":"Müller","email":null},
		"custom_template":{"id":3},"document_date":"2026-10-01","document_charges":[{"name":"Porto"}],
		"line_items":[
		  {"name":"Beratung","description":null,"quantity":1.5,"unit":"Stunde","price":19.99,"price_gross":23.79,
		   "discount_percentage":null,"proposition":{"id":9007199254740993},"type_of":"position","vat_rate":"7%",
		   "total_net":29.99,"document_charge":0.0,"position_in_group":"1"},
		  {"name":"Text","quantity":0,"unit":"","proposition":{"id":null},"vat_rate":"0%"},
		  {"name":"Ohne","quantity":1,"unit":"x","proposition":null,"vat_rate":"19,0 %"}]}`)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--from-estimate", "5", "--payment-term", "1", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2)
	want := `{"billing":{"company":"Müller","email":null},"customer":{"id":0,"project":{"id":7}},"description":null,"line_items":[` +
		`{"discount_percentage":null,"name":"Beratung","price":19.99,"price_gross":23.79,"proposition":{"id":9007199254740993},"quantity":1.5,"type_of":"position","unit":"Stunde","vat_rate":0.07},` +
		`{"name":"Text","quantity":0,"unit":"","vat_rate":0},` +
		`{"name":"Ohne","quantity":1,"unit":"x","vat_rate":0.19}],` +
		`"name":"Angebot","payment_term":{"id":1}}`
	if got := canonJSON(t, reqs[1].Body); got != canonJSON(t, []byte(want)) {
		t.Errorf("body = %s\nerwartet %s", got, want)
	}
}

// Without a customer ID, contact person and project are not sent either:
// they are references below the customer.
func TestInvoiceFromEstimateWithoutCustomer(t *testing.T) {
	app, srv := apiApp(t)
	estimateJSON(srv, `{"name":"A","customer":null,"contact_person":{"id":1},"project":{"id":2},
		"line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":"19%"}]}`)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--from-estimate", "5", "--payment-term", "1", "-o", "json"}), 0)
	want := `{"name":"A","payment_term":{"id":1},"line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":0.19}]}`
	if got := canonJSON(t, wantRequests(t, srv, 2)[1].Body); got != canonJSON(t, []byte(want)) {
		t.Errorf("body = %s\nerwartet %s", got, want)
	}
}

// --dry-run reads the estimate (that is the only way to show the body) and
// creates nothing: the credits are those of the create, the read is named
// on its own.
func TestInvoiceFromEstimateDryRun(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--dry-run", "-o", "json")), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" || r.Path != "/income/estimates/349" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	var got struct {
		DryRun  bool            `json:"dry_run"`
		Method  string          `json:"method"`
		URL     string          `json:"url"`
		Credits int             `json:"credits"`
		Body    json.RawMessage `json:"body"`
		Read    struct {
			Method  string `json:"method"`
			URL     string `json:"url"`
			Credits int    `json:"credits"`
		} `json:"read"`
	}
	out := mustOut(app)
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !got.DryRun || got.Method != "POST" || got.URL != srv.URL()+"/income/invoices" || got.Credits != 50 {
		t.Errorf("dry run = %s", out)
	}
	if got.Read.Method != "GET" || got.Read.URL != srv.URL()+"/income/estimates/349" || got.Read.Credits != 1 {
		t.Errorf("read = %+v", got.Read)
	}
	golden.Assert(t, "invoice-from-estimate.body.json", indentJSON(t, got.Body))

	// text: the read is named, the body shown
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--dry-run")), 0)
	wantRequests(t, srv, 1)
	text := mustOut(app)
	head := "Probelauf – nichts angelegt\nGelesen: GET " + srv.URL() + "/income/estimates/349 (1 Credit, bereits verbraucht)\n" +
		"Methode: POST\nURL:     " + srv.URL() + "/income/invoices\nCredits: 50\nBody:\n"
	if !strings.HasPrefix(text, head) || !strings.Contains(text, `"vat_rate": 0.19`) {
		t.Errorf("stdout = %s", text)
	}
}

// Every usage error is exit 2 before anything is resolved or read: a
// missing --payment-term (estimates have none), --input, an invalid ID.
func TestInvoiceFromEstimateUsage(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"invoice", "create", "--from-estimate", "349"}, "--payment-term"},
		{[]string{"invoice", "create", "--from-estimate", "349", "--dry-run"}, "--payment-term"},
		{fromEstimateCmd("--input", "rechnung.json"), "--input"},
		{[]string{"invoice", "create", "--from-estimate", "abc", "--payment-term", "1"}, `ungültige ID "abc"`},
		{[]string{"invoice", "create", "--from-estimate", "-1", "--payment-term", "1"}, `ungültige ID "-1"`},
		{[]string{"invoice", "create", "--from-estimate=", "--payment-term", "1"}, `ungültige ID ""`},
		{fromEstimateCmd("--item", "nam=A"), `unbekannter Schlüssel "nam"`},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), 2)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, tc.want) {
			t.Errorf("%v: stdout = %q, stderr = %q", tc.args, out, errs)
		}
	}
	// flags come before the read-only gate
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"invoice", "create", "--from-estimate", "349"}), 2)
	wantRequests(t, srv, 0)
}

// Invalid flag values are reported before the estimate is read: they
// cost no credit.
func TestInvoiceFromEstimateFlagValuesBeforeRead(t *testing.T) {
	for _, tc := range []struct {
		extra []string
		exit  int
	}{
		{[]string{"--document-date", "morgen früh"}, 5},
		{[]string{"--items", "fehlt.json"}, 2},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, fromEstimateCmd(tc.extra...)), tc.exit)
		wantRequests(t, srv, 0)
	}
}

func TestInvoiceFromEstimateReadOnly(t *testing.T) {
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, fromEstimateCmd(extra...)), 9)
		wantRequests(t, srv, 0)
	}
}

// A failed read is the normal error of the read: nothing is created.
func TestInvoiceFromEstimateReadFails(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/estimates/{id}", http.StatusNotFound,
		[]byte(`{"type":"error","error_type":"not_found","message":"Angebot nicht gefunden"}`))
	wantExit(t, app, Run(app, fromEstimateCmd("-o", "json")), 4)
	wantRequests(t, srv, 1)
	if out, errs := bufs(app); out != "" || !strings.Contains(errs, "Angebot 349") || !strings.Contains(errs, "keine Rechnung angelegt") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}

	// an answer that is no object
	app, srv = apiApp(t)
	estimateJSON(srv, `[]`)
	wantExit(t, app, Run(app, fromEstimateCmd()), 1)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "keine Rechnung angelegt") {
		t.Errorf("stderr = %q", errs)
	}
}

// R24: ID 0 is read, not dropped.
func TestInvoiceFromEstimateIDZero(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--from-estimate", "0", "--payment-term", "0", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Path != "/income/estimates/0" || !strings.Contains(string(reqs[1].Body), `"payment_term":{"id":0}`) {
		t.Errorf("read %s, body %s", reqs[0].Path, reqs[1].Body)
	}
}

// Every other flag overrides its field of the estimate (--customer: see
// TestInvoiceFromEstimateOtherCustomer).
func TestInvoiceFromEstimateFlagsOverride(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--name", "Rechnung Website", "--document-date", "2026-11-04",
		"--billing-zip", "10115", "--gross", "-o", "json")), 0)
	var body map[string]any
	if err := json.Unmarshal(wantRequests(t, srv, 2)[1].Body, &body); err != nil {
		t.Fatal(err)
	}
	billing, _ := body["billing"].(map[string]any)
	if body["name"] != "Rechnung Website" || body["document_date"] != "2026-11-04" || body["gross"] != true ||
		billing["zip"] != "10115" || billing["city"] != "Wiesbaden" || len(body["line_items"].([]any)) != 2 {
		t.Errorf("body = %v", body)
	}
}

// --item and --items replace the positions of the estimate completely.
func TestInvoiceFromEstimateItemsReplace(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--item", "name=Pauschale,qty=1,unit=Stück,price=500,vat=19", "-o", "json")), 0)
	var body struct {
		LineItems []map[string]any `json:"line_items"`
	}
	if err := json.Unmarshal(wantRequests(t, srv, 2)[1].Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.LineItems) != 1 || body.LineItems[0]["name"] != "Pauschale" {
		t.Errorf("line_items = %v", body.LineItems)
	}

	app, srv = apiApp(t)
	app.In = strings.NewReader(`[{"name":"Aus Datei","quantity":2,"unit":"h","price":10,"vat_rate":"7%"}]`)
	wantExit(t, app, Run(app, fromEstimateCmd("--items", "-", "-o", "json")), 0)
	if got := canonJSON(t, wantRequests(t, srv, 2)[1].Body); !strings.Contains(got, `"line_items":[{"name":"Aus Datei","price":10,"quantity":2,"unit":"h","vat_rate":0.07}]`) {
		t.Errorf("body = %s", got)
	}
}

// Without --from-estimate the registered invoice create (with the extra
// flag) behaves like the plain create of the builder tests: the same
// bodies and exit codes as TestWriteNestedRefsAndBilling,
// TestWriteItemObjectFields, TestWriteRequiredFieldsComeFromFlagsOrInput (R17) and the usage
// checks, and no read. The builder tests run the command without the
// extra; this runs the one users get.
func TestInvoiceCreateWithoutFromEstimateUnchanged(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
		body string // the POST body; "" means no request
		errs string // part of stderr
	}{
		{[]string{"--name", "Rechnung", "--payment-term", "4", "--contact-person", "7", "--customer", "12", "--customer-project", "3",
			"--billing-company", "Müller & Söhne", "--billing-zip", "10115", "--document-date", "2026-11-04", "--gross=false", "--item", item1},
			0, `{"name":"Rechnung","payment_term":{"id":4},"customer":{"id":12,"contact_person":{"id":7},"project":{"id":3}},
			"billing":{"company":"Müller & Söhne","zip":"10115"},"document_date":"2026-11-04","gross":false,
			"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19}]}`, ""},
		{[]string{"--name", "R", "--payment-term", "4", "--item", item1 + ",proposition=2"},
			0, `{"name":"R","payment_term":{"id":4},"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19,"proposition":{"id":2}}]}`, ""},
		{[]string{"--name", "R", "--payment-term", "4"}, 5, "", "line_items"},
		{[]string{"--name", "R", "--payment-term", "4", "--item", item1 + ",proposition=zwei"}, 5, "", "proposition"},
		{[]string{"--item", "nam=A", "--document-date", "nie"}, 2, "", `unbekannter Schlüssel "nam"`},
		{[]string{"--name", "R", "--payment-term", "4", "--document-date", "nie", "--item", item1}, 5, "", "--document-date"},
		{[]string{"--input", "-", "--items", "-"}, 2, "", "Standardeingabe"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(append([]string{"invoice", "create"}, tc.args...), "-o", "json")), tc.exit)
		if tc.body == "" {
			wantRequests(t, srv, 0)
		} else {
			wantBody(t, srv, "POST", "/income/invoices", tc.body)
		}
		if _, errs := bufs(app); !strings.Contains(errs, tc.errs) {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}

// The help names the flag and the fields that are not taken over.
func TestInvoiceFromEstimateHelp(t *testing.T) {
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--help"}), 0)
	out := mustOut(app)
	for _, want := range []string{"--from-estimate", "document_date", "custom_template", "document_charges",
		"supply_date", "start_new_item_group", "--payment-term"} {
		if !strings.Contains(out, want) {
			t.Errorf("Hilfe ohne %q:\n%s", want, out)
		}
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "create", "--help"}), 0)
	if strings.Contains(mustOut(app), "--from-estimate") {
		t.Error("estimate create hat --from-estimate")
	}
}

// schemaAt is the schema at the dotted path below s; allOf branches and
// array items are followed. nil when a segment does not exist.
func schemaAt(s *openapi3.Schema, path string) *openapi3.Schema {
	for _, k := range strings.Split(path, ".") {
		s = property(s, k)
		if s == nil {
			return nil
		}
	}
	return s
}

func property(s *openapi3.Schema, k string) *openapi3.Schema {
	if s == nil {
		return nil
	}
	if s.Items != nil && s.Items.Value != nil {
		s = s.Items.Value
	}
	if p := s.Properties[k]; p != nil && p.Value != nil {
		return p.Value
	}
	for _, b := range s.AllOf {
		if p := property(b.Value, k); p != nil {
			return p
		}
	}
	return nil
}

// leaves are the dotted paths of the scalar fields of an object schema,
// objects followed; arrays are leaves.
func leaves(s *openapi3.Schema, prefix string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(s.Properties)) {
		p := s.Properties[k].Value
		if len(p.Properties) > 0 {
			out = append(out, leaves(p, prefix+k+".")...)
			continue
		}
		out = append(out, prefix+k)
	}
	return out
}

// covered reports whether path is one of the fields or below one of them.
func covered(path string, fields []string) bool {
	for _, f := range fields {
		if path == f || strings.HasPrefix(path, f+".") {
			return true
		}
	}
	return false
}

// The mapping of spec 9.4 matches both schemas: every source exists in the
// estimate, every target in the invoice request, and every field of the
// invoice request is either filled from the estimate or named as not
// taken over. A field gained or lost by either schema fails here.
func TestFromEstimateMappingMatchesSchemas(t *testing.T) {
	est := spec.ResponseSchema("GetEstimate")
	req := spec.RequestSchema("CreateInvoice")
	if est == nil || req == nil {
		t.Fatal("Schemas fehlen")
	}
	// the schemas of one position (schemaAt ends at the array)
	reqItem := schemaAt(req, "line_items").Items.Value
	estItem := schemaAt(est, "line_items").Items.Value
	if l := leaves(reqItem, ""); !slices.Contains(l, "vat_rate") || !slices.Contains(l, "start_new_item_group") {
		t.Fatalf("Positionsfelder des Rechnungs-Requests nicht gefunden: %v", l)
	}
	if l := leaves(estItem, ""); !slices.Contains(l, "vat_rate") || !slices.Contains(l, "position_in_group") {
		t.Fatalf("Positionsfelder des Angebots nicht gefunden: %v", l)
	}
	// prefix is the path of the level in estimateOnlyFields
	check := func(level, prefix string, from, to *openapi3.Schema, fields []estimateField, invoiceOnly []string) {
		var targets []string
		for _, f := range fields {
			if schemaAt(from, f.from) == nil {
				t.Errorf("%s: Angebot hat kein Feld %s", level, f.from)
			}
			ts := schemaAt(to, f.to)
			if ts == nil {
				t.Errorf("%s: Rechnungs-Request hat kein Feld %s", level, f.to)
				continue
			}
			targets = append(targets, f.to)
			// an object is copied field by field: each must exist in the estimate
			for _, k := range slices.Sorted(maps.Keys(ts.Properties)) {
				if schemaAt(from, f.from+"."+k) == nil {
					t.Errorf("%s: Angebot hat kein Feld %s.%s", level, f.from, k)
				}
			}
		}
		for _, f := range invoiceOnly {
			if schemaAt(to, f) == nil {
				t.Errorf("%s: nicht übertragbares Feld %s fehlt im Rechnungs-Request", level, f)
			}
			// the help says the estimate does not have it
			if !slices.Contains(estimateOnlyFields, prefix+f) && schemaAt(from, f) != nil {
				t.Errorf("%s: Angebot hat jetzt %s – übernehmen oder Hilfe korrigieren", level, f)
			}
		}
		for _, leaf := range leaves(to, "") {
			if leaf == "line_items" && level == "Rechnung" {
				continue // the positions: checked below
			}
			if !covered(leaf, targets) && !covered(leaf, invoiceOnly) {
				t.Errorf("%s: Feld %s des Rechnungs-Requests weder übertragen noch als nicht übertragbar genannt", level, leaf)
			}
		}
	}
	check("Rechnung", "", est, req, fromEstimateFields, invoiceOnlyFields)
	check("Position", "line_items.", estItem, reqItem, fromEstimateItemFields, invoiceOnlyItemFields)
	// the help names every field that is not taken over
	help := fromEstimateLong
	for _, f := range slices.Concat(invoiceOnlyFields, invoiceOnlyItemFields, estimateOnlyFields) {
		name := f[strings.LastIndex(f, ".")+1:] // positions: by the field name
		flag := "--" + strings.ReplaceAll(name, "_", "-")
		if !strings.Contains(help, name) && !strings.Contains(help, flag) {
			t.Errorf("Hilfe nennt %s nicht", f)
		}
	}
	for _, f := range estimateOnlyFields {
		if schemaAt(est, f) == nil {
			t.Errorf("Angebot hat kein Feld %s", f)
		}
	}
}

// --customer with another customer than the estimate's drops the
// estimate's contact person and project (they belong to its customer),
// with a hint; explicit flags still set them. The same customer keeps
// both.
func TestInvoiceFromEstimateOtherCustomer(t *testing.T) {
	customer := func(t *testing.T, raw []byte) string {
		t.Helper()
		var body struct {
			Customer json.RawMessage `json:"customer"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		return canonJSON(t, body.Customer)
	}
	for _, tc := range []struct {
		name  string
		extra []string
		want  string
		hint  bool
	}{
		{"anderer Kunde", []string{"--customer", "7"}, `{"id":7}`, true},
		{"gleicher Kunde", []string{"--customer", "3052"}, `{"contact_person":{"id":1116},"id":3052,"project":{"id":4316}}`, false},
		{"anderer Kunde mit Ansprechpartner", []string{"--customer", "7", "--contact-person", "8"}, `{"contact_person":{"id":8},"id":7}`, true},
		{"anderer Kunde mit beiden", []string{"--customer", "7", "--contact-person", "8", "--customer-project", "9"},
			`{"contact_person":{"id":8},"id":7,"project":{"id":9}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, fromEstimateCmd(append(tc.extra, "-o", "json")...)), 0)
			if got := customer(t, wantRequests(t, srv, 2)[1].Body); got != tc.want {
				t.Errorf("customer = %s, erwartet %s", got, tc.want)
			}
			_, errs := bufs(app)
			if !tc.hint {
				if errs != "" {
					t.Errorf("stderr = %q", errs)
				}
				return
			}
			if !strings.Contains(errs, "Kunde 7 statt 3052") || !strings.Contains(errs, "--customer-project") {
				t.Errorf("stderr = %q", errs)
			}
			if strings.Contains(tc.name, "Ansprechpartner") && strings.Contains(errs, "contact_person") {
				t.Errorf("Hinweis nennt das gesetzte contact_person: %q", errs)
			}
		})
	}
	// the dry run drops them as well
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--customer", "7", "--dry-run", "-o", "json")), 0)
	wantRequests(t, srv, 1)
	if out := mustOut(app); !strings.Contains(canonJSON(t, []byte(out)), `"customer":{"id":7}`) {
		t.Errorf("stdout = %s", out)
	}
}

// A null the invoice request cannot take is left out with the hint of the
// --input path (pruneHints): vat_rate, price or name of the estimate never
// disappear silently. A missing reference (proposition: null) is no
// value and needs no hint; nullable fields keep their null.
func TestInvoiceFromEstimateDroppedNullsHint(t *testing.T) {
	app, srv := apiApp(t)
	estimateJSON(srv, `{"name":null,"gross":null,"billing":{"email":null},"customer":{"id":1},
		"line_items":[{"name":"A","quantity":1,"unit":"x","price":null,"vat_rate":null,"proposition":null},
		              {"name":"B","quantity":null,"unit":"x","vat_rate":"19%","proposition":{"id":null}}]}`)
	wantExit(t, app, Run(app, fromEstimateCmd("--name", "R", "-o", "json")), 0)
	want := `{"billing":{"email":null},"customer":{"id":1},"line_items":[{"name":"A","price":null,"unit":"x","quantity":1},` +
		`{"name":"B","unit":"x","vat_rate":0.19}],"name":"R","payment_term":{"id":1130}}`
	if got := canonJSON(t, wantRequests(t, srv, 2)[1].Body); got != canonJSON(t, []byte(want)) {
		t.Errorf("body = %s\nerwartet %s", got, want)
	}
	_, errs := bufs(app)
	if errs != "Hinweis: null ignoriert (Feld bleibt unverändert): gross, line_items[].proposition.id, line_items[].quantity, line_items[].vat_rate, name\n" {
		t.Errorf("stderr = %q", errs)
	}

	// the spec example has nulls only in nullable fields and references
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, fromEstimateCmd("--dry-run")), 0)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); errs != "" {
		t.Errorf("stderr = %q", errs)
	}
}

// Errors found after the read name the estimate they come from.
func TestInvoiceFromEstimateErrorOrigin(t *testing.T) {
	// a value of the estimate that cannot be sent
	app, srv := apiApp(t)
	estimateJSON(srv, `{"name":"A","line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":"abc"}]}`)
	wantExit(t, app, Run(app, fromEstimateCmd()), 5)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "aus Angebot 349: line_items[0].vat_rate") || !strings.Contains(errs, "--item/--items") {
		t.Errorf("stderr = %q", errs)
	}
	// replaced positions are not checked
	app, srv = apiApp(t)
	estimateJSON(srv, `{"name":"A","line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":"abc"}]}`)
	wantExit(t, app, Run(app, fromEstimateCmd("--item", item1, "-o", "json")), 0)
	wantRequests(t, srv, 2)

	// the body validation names the invoice built from the estimate
	app, srv = apiApp(t)
	estimateJSON(srv, `{"name":null,"line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":"19%"}]}`)
	wantExit(t, app, Run(app, fromEstimateCmd()), 5)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "Rechnung aus Angebot 349: ") || !strings.Contains(errs, "--name") {
		t.Errorf("stderr = %q", errs)
	}
}

// Data after the JSON object of the read is an error like any other
// broken answer: nothing is created.
func TestInvoiceFromEstimateTrailingData(t *testing.T) {
	app, srv := apiApp(t)
	estimateJSON(srv, `{"name":"A","line_items":[{"name":"X","quantity":1,"unit":"x","vat_rate":"19%"}]} {"name":"B"}`)
	wantExit(t, app, Run(app, fromEstimateCmd()), 1)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "kein JSON-Objekt") {
		t.Errorf("stderr = %q", errs)
	}
}
