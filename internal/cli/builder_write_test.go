package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// writeEntries are create/update entries built from spec operations; the
// registry gets them in Phase 7.
func writeEntries(t *testing.T) []registry.Entry {
	t.Helper()
	return []registry.Entry{
		specEntry(t, "CreateInvoice", "invoice", "create", registry.ClassCreate),
		specEntry(t, "UpdateInvoice", "invoice", "update", registry.ClassUpdate),
		specEntry(t, "CreateProject", "project", "create", registry.ClassCreate),
		specEntry(t, "UpdateProject", "project", "update", registry.ClassUpdate),
		specEntry(t, "CreateTask", "task", "create", registry.ClassCreate),
		specEntry(t, "UpdateCompany", "company", "update", registry.ClassUpdate),
		specEntry(t, "CreateVoucher", "voucher", "create", registry.ClassCreate),
		specEntry(t, "UpdatePerson", "person", "update", registry.ClassUpdate),
	}
}

// runWrite runs args with the write entries attached.
func runWrite(t *testing.T, app *App, args ...string) int {
	t.Helper()
	return runEntries(t, app, nil, args, writeEntries(t)...)
}

// canonJSON re-encodes JSON with sorted keys, numbers kept as text.
func canonJSON(t *testing.T, raw []byte) string {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		t.Fatalf("kein JSON: %v: %s", err, raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// wantBody checks the body of the only request against want (JSON).
func wantBody(t *testing.T, srv *mockapi.Server, method, path, want string) {
	t.Helper()
	r := wantRequests(t, srv, 1)[0]
	if r.Method != method || r.Path != path {
		t.Errorf("request = %s %s, erwartet %s %s", r.Method, r.Path, method, path)
	}
	if got, w := canonJSON(t, r.Body), canonJSON(t, []byte(want)); got != w {
		t.Errorf("body = %s\nerwartet %s", got, w)
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const item1 = `name=Beratung,qty=2,price=100,vat=19`

func TestR18_EmptyLineItemsSentWithReplace(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--replace-items", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"line_items":[]}`)

	// an empty list from --input is sent with --replace-items as well
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"name":"Neu","line_items":[]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "--replace-items", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"name":"Neu","line_items":[]}`)
}

func TestR32_UpdateItemRequiresReplaceItems(t *testing.T) {
	items := writeFile(t, "items.json", `[{"name":"A","quantity":1,"price":5,"vat_rate":19}]`)
	for _, tc := range []struct {
		name  string
		stdin string
		args  []string
	}{
		{"item", "", []string{"--item", item1}},
		{"items", "", []string{"--items", items}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			app.In = strings.NewReader(tc.stdin)
			args := append([]string{"invoice", "update", "12"}, tc.args...)
			wantExit(t, app, runWrite(t, app, args...), 2)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, "--replace-items") || !strings.Contains(errs, "ersetzt alle Positionen") {
				t.Errorf("stderr = %q", errs)
			}
		})
	}

	// with --replace-items the items replace all positions
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--item", item1, "--replace-items", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19}]}`)
}

func TestWriteFlagsOverrideInput(t *testing.T) {
	in := writeFile(t, "project.json", `{"name":"Alt","customer":{"id":1},"description":"bleibt"}`)
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "create", "--input", in, "--name", "Neu", "--customer", "2", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"Neu","customer":{"id":2},"description":"bleibt"}`)
}

func TestWriteUpdateSendsOnlyChangedFlags(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--name", "Neu", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/projects/5", `{"name":"Neu"}`)

	// a flag set to its zero value counts as changed
	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--flagged=false", "--description", "", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/projects/5", `{"flagged":false,"description":""}`)
}

func TestWriteUpdateWithoutChangesIsUsageError(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "update", "5"), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "keine Änderungen") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestWriteAttr(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--attr", "3=42", "--attr", "4=Text, mit Komma", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/projects/5", `{"custom_attributes":{"3":"42","4":"Text, mit Komma"}}`)

	// --attr merges into custom_attributes from --input
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"custom_attributes":{"1":"a","3":"alt"}}`)
	wantExit(t, app, runWrite(t, app, "company", "update", "8", "--input", "-", "--attr", "3=42", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/contact/companies/8", `{"custom_attributes":{"1":"a","3":"42"}}`)

	for _, bad := range []string{"x=1", "ohne-gleich"} {
		app, srv := apiApp(t)
		wantExit(t, app, runWrite(t, app, "project", "update", "5", "--attr", bad), 5)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--attr") {
			t.Errorf("%s: stderr = %q", bad, errs)
		}
	}
}

func TestWriteNestedRefsAndBilling(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "Rechnung", "--payment-term", "4",
		"--contact-person", "7", "--customer", "12", "--customer-project", "3",
		"--billing-company", "Müller & Söhne", "--billing-zip", "10115",
		"--document-date", "2026-11-04", "--gross=false", "--item", item1, "-o", "json"), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{
		"name":"Rechnung","payment_term":{"id":4},
		"customer":{"id":12,"contact_person":{"id":7},"project":{"id":3}},
		"billing":{"company":"Müller & Söhne","zip":"10115"},
		"document_date":"2026-11-04","gross":false,
		"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19}]}`)

	// a reference flag keeps the nested references from --input
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"customer":{"id":1,"contact_person":{"id":7}}}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "--customer", "2", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"customer":{"id":2,"contact_person":{"id":7}}}`)
}

func TestWriteRepeatableFlags(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "create", "--name", "P", "--customer", "1",
		"--team-member", "1", "--team-member", "0", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"P","customer":{"id":1},"team_members":[{"id":1},{"id":0}]}`)

	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "voucher", "create", "--name", "Bewirtung", "--provenance", "domestic",
		"--entertainment-person", "Anna", "--entertainment-person", "Ben, Jr.",
		"--item", "name=Essen,amount=59.5,vat=7,category=Bewirtungskosten", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/expense/vouchers", `{"name":"Bewirtung","provenance":"domestic",
		"entertainment_persons":["Anna","Ben, Jr."],
		"line_items":[{"name":"Essen","amount":59.5,"vat_rate":0.07,"category":"Bewirtungskosten"}]}`)
}

func TestWriteStringTypedFieldsSentAsString(t *testing.T) {
	// R7/R8-style: the spec type decides, for flags and --input alike
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "create", "--name", "P", "--customer", "1",
		"--budget-money", "5000", "--budget-time", "12.5", "--flagged", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"P","customer":{"id":1},"budget_money":"5000","budget_time":"12.5","flagged":true}`)

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"name":"T","project":{"id":3},"relative_costs":10,"complete":true}`)
	wantExit(t, app, runWrite(t, app, "task", "create", "--input", "-", "--flagged=ja", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"T","project":{"id":3},"relative_costs":"10","complete":"true","flagged":"true"}`)
}

func TestWriteDateFlagsUseAppClock(t *testing.T) {
	app, srv := apiApp(t)
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	app.Location = loc
	// 23:30 UTC on 2026-11-03 is already 2026-11-04 in Berlin
	app.Now = func() time.Time { return time.Date(2026, 11, 3, 23, 30, 0, 0, time.UTC) }
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--start-date", "heute", "--end-date", "yesterday", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/projects/5", `{"start_date":"2026-11-04","end_date":"2026-11-03"}`)
}

func TestWriteInvalidFlagValuesAreValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		flag string
	}{
		{[]string{"project", "update", "5", "--start-date", "morgen"}, "--start-date"},
		{[]string{"project", "update", "5", "--customer", "abc"}, "--customer"},
		{[]string{"project", "update", "5", "--team-member", "-1"}, "--team-member"},
		{[]string{"project", "update", "5", "--flagged=vielleicht"}, "--flagged"},
		{[]string{"voucher", "create", "--name", "V", "--provenance", "non_eu", "--item", "name=A,amount=1,vat=19,category=Bewirtungskosten"}, "foreign"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runWrite(t, app, tc.args...), 5)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.flag) {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}

func TestWriteRequiredFieldsComeFromFlagsOrInput(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "create", "--name", "P"), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "customer") {
		t.Errorf("stderr = %q", errs)
	}

	// R17 via the builder: create without positions
	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4"), 5)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"customer":{"id":1}}`)
	wantExit(t, app, runWrite(t, app, "project", "create", "--input", "-", "--name", "P", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"P","customer":{"id":1}}`)
}

func TestWriteInputPruneHints(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"id":12,"invoice_no":"RE-1001","state":"draft","name":"Neu",
		"line_items":[{"name":"A","quantity":1,"price":19.99,"vat_rate":"19%","proposition":null,"total_net":19.99}]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "--replace-items", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"name":"Neu","line_items":[{"name":"A","quantity":1,"price":19.99,"vat_rate":0.19}]}`)
	_, errs := bufs(app)
	for _, want := range []string{
		"Hinweis: nur lesbare Felder entfernt: id, invoice_no, line_items[].total_net, state\n",
		"Hinweis: null ignoriert (Feld bleibt unverändert): line_items[].proposition\n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr enthält %q nicht:\n%s", want, errs)
		}
	}
}

func TestWriteInputUnknownFieldNoRequest(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"name":"P","customer":{"id":1},"note":"x"}`)
	wantExit(t, app, runWrite(t, app, "project", "create", "--input", "-"), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `unbekanntes Feld "note"`) {
		t.Errorf("stderr = %q", errs)
	}

	for _, tc := range []struct{ in, want string }{
		{`[1,2]`, "JSON-Objekt"},
		{`{"name":`, "--input"},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.in)
		wantExit(t, app, runWrite(t, app, "project", "create", "--input", "-"), 5)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%s: stderr = %q", tc.in, errs)
		}
	}

	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "create", "--input", filepath.Join(t.TempDir(), "fehlt.json")), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--input: Datei") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestWriteInputBOMAndDecimals(t *testing.T) {
	in := writeFile(t, "invoice.json", "\uFEFF{\r\n  \"name\": \"Rechnung\",\r\n  \"payment_term\": {\"id\": 4},\r\n"+
		"  \"line_items\": [{\"name\": \"A\", \"quantity\": 3, \"price\": 19.99, \"vat_rate\": 0.19}]\r\n}\r\n")
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--input", in, "-o", "json"), 0)
	r := wantRequests(t, srv, 1)[0]
	if !bytes.Contains(r.Body, []byte(`"price":19.99`)) {
		t.Errorf("body = %s", r.Body)
	}

	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4",
		"--item", "name=A,qty=1.5,price=19.99,vat=19", "-o", "json"), 0)
	r = wantRequests(t, srv, 1)[0]
	if !bytes.Contains(r.Body, []byte(`"price":19.99`)) || !bytes.Contains(r.Body, []byte(`"quantity":1.5`)) {
		t.Errorf("body = %s", r.Body)
	}
}

func TestWriteItemObjectFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4",
		"--item", item1+",proposition=2", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{"name":"R","payment_term":{"id":4},
		"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19,"proposition":{"id":2}}]}`)

	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4",
		"--item", item1+",proposition=zwei"), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "proposition") {
		t.Errorf("stderr = %q", errs)
	}

	// an object with more than an id cannot be given as key=value
	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "voucher", "create", "--name", "V", "--provenance", "eu",
		"--item", "name=A,amount=1,vat=19,category=Bewirtungskosten,billing=5"), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `"billing"`) || !strings.Contains(errs, "--items") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestWriteItemsFile(t *testing.T) {
	items := writeFile(t, "items.json", `[{"name":"A","quantity":1,"price":5,"vat_rate":"7%","total_net":5}]`)
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4", "--items", items, "-o", "json"), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{"name":"R","payment_term":{"id":4},
		"line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.07}]}`)
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: nur lesbare Felder entfernt: line_items[].total_net") {
		t.Errorf("stderr = %q", errs)
	}

	// --item replaces the positions of --input (flags win)
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"name":"R","payment_term":{"id":4},"line_items":[{"name":"Alt","quantity":1,"price":1,"vat_rate":0.19}]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--input", "-", "--item", item1, "-o", "json"), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{"name":"R","payment_term":{"id":4},
		"line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19}]}`)

	// unknown keys in --items are exit 5 like --input
	bad := writeFile(t, "bad.json", `[{"nam":"A"}]`)
	app, srv = apiApp(t)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--name", "R", "--payment-term", "4", "--items", bad), 5)
	wantRequests(t, srv, 0)
}

const itemsDropHint = "Hinweis: line_items aus --input nicht gesendet – Positionen bleiben unverändert; zum Ersetzen --replace-items angeben\n"

func TestWriteUpdateInputItemsOnlyWithReplace(t *testing.T) {
	// spec 6.3a: positions are touched only with --replace-items; without
	// it line_items from --input (empty or not) are left out with a hint
	for _, items := range []string{`[]`, `[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]`} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(`{"name":"Neu","line_items":` + items + `}`)
		wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "-o", "json"), 0)
		wantBody(t, srv, "PUT", "/income/invoices/12", `{"name":"Neu"}`)
		if _, errs := bufs(app); !strings.Contains(errs, itemsDropHint) {
			t.Errorf("%s: stderr = %q", items, errs)
		}
	}

	// with --replace-items they are sent as given
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"name":"Neu","line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "--replace-items", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"name":"Neu","line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`)
	if _, errs := bufs(app); strings.Contains(errs, "nicht gesendet") {
		t.Errorf("stderr = %q", errs)
	}

	// only line_items from --input: nothing left to change
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"line_items":[]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-"), 2)
	wantRequests(t, srv, 0)
}

func TestWriteDescribeRoundTripKeepsPositions(t *testing.T) {
	get := specEntry(t, "GetInvoice", "invoice", "describe", registry.ClassReadOnly)
	entries := append(writeEntries(t), get)
	app, _ := apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "describe", "12", "-o", "json"}, entries...), 0)
	described, _ := bufs(app)
	if !strings.Contains(described, `"line_items"`) {
		t.Fatalf("GetInvoice-Beispiel ohne line_items: %s", described)
	}

	// describe | jq '.name="Neu"' | update --input -: no positions sent
	app, srv := apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "update", "12", "--input", "-", "--name", "Neu", "-o", "json"}, entries...), 0)
	r := wantRequests(t, srv, 1)[0]
	var body map[string]any
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["line_items"]; ok || body["name"] != "Neu" {
		t.Errorf("body = %s", r.Body)
	}
	if _, errs := bufs(app); !strings.Contains(errs, itemsDropHint) || !strings.Contains(errs, "nur lesbare Felder entfernt") {
		t.Errorf("stderr = %q", errs)
	}

	// with --replace-items the positions are sent (R31)
	app, srv = apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "update", "12", "--input", "-", "--replace-items", "-o", "json"}, entries...), 0)
	r = wantRequests(t, srv, 1)[0]
	body = nil
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	if l, _ := body["line_items"].([]any); len(l) == 0 {
		t.Errorf("body ohne Positionen: %s", r.Body)
	}
}

// TestWriteCreateSendsInputItems: on create line_items from --input are
// always sent.
func TestWriteCreateSendsInputItems(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"name":"R","payment_term":{"id":4},"line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "create", "--input", "-", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/income/invoices", `{"name":"R","payment_term":{"id":4},"line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`)
	if _, errs := bufs(app); strings.Contains(errs, "nicht gesendet") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestWriteItemFlagCombinations(t *testing.T) {
	items := writeFile(t, "items.json", `[{"name":"A","quantity":1,"price":5,"vat_rate":19}]`)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"invoice", "create", "--name", "R", "--payment-term", "4", "--items", items, "--item", item1}, "--items"},
		{[]string{"invoice", "create", "--input", "-", "--items", "-"}, "Standardeingabe"},
		{[]string{"invoice", "create", "--replace-items"}, "unbekanntes Flag: --replace-items"},
		{[]string{"project", "create", "--item", item1}, "unbekanntes Flag: --item"},
	} {
		app, srv := apiApp(t)
		reader := &countingReader{r: strings.NewReader(`{"name":"R"}`)}
		app.In = reader
		wantExit(t, app, runWrite(t, app, tc.args...), 2)
		wantRequests(t, srv, 0)
		if reader.n > 0 {
			t.Errorf("%v: Standardeingabe gelesen", tc.args)
		}
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}

type countingReader struct {
	r interface{ Read([]byte) (int, error) }
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.n++
	return c.r.Read(p)
}

func TestWriteOutput(t *testing.T) {
	// create: the API object
	app, srv := apiApp(t)
	srv.On("POST", "/projects", http.StatusCreated, []byte(`{"id":9007199254740993,"name":"P","budget_money":"19.99"}`))
	wantExit(t, app, runWrite(t, app, "project", "create", "--name", "P", "--customer", "1", "-o", "json"), 0)
	if out, _ := bufs(app); out != "{\n  \"id\": 9007199254740993,\n  \"name\": \"P\",\n  \"budget_money\": \"19.99\"\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	// update with an empty body: {"id":N,"ok":true}
	app, srv = apiApp(t)
	srv.On("PUT", "/projects/{id}", http.StatusOK, nil)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--name", "N", "-o", "json"), 0)
	if out, _ := bufs(app); out != "{\n  \"id\": 5,\n  \"ok\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
	app, srv = apiApp(t)
	srv.On("PUT", "/projects/{id}", http.StatusOK, nil)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--name", "N"), 0)
	if out, _ := bufs(app); out != "project 5 aktualisiert.\n" {
		t.Errorf("stdout = %q", out)
	}

	// create with an empty body: {"ok":true} plus a warning
	app, srv = apiApp(t)
	srv.On("POST", "/projects", http.StatusCreated, nil)
	wantExit(t, app, runWrite(t, app, "project", "create", "--name", "P", "--customer", "1", "-o", "json"), 0)
	if out, errs := bufs(app); out != "{\n  \"ok\": true\n}\n" || !strings.Contains(errs, "API lieferte kein Objekt") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
}

func TestWritePathParamFlags(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "person", "update", "5", "--company", "7", "--first-name", "Anna", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/contact/companies/7/persons/5", `{"first_name":"Anna"}`)

	create := specEntry(t, "CreatePerson", "person", "create", registry.ClassCreate)
	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "create", "--company", "7", "--first-name", "A", "--last-name", "B", "-o", "json"}, create), 0)
	wantBody(t, srv, "POST", "/contact/companies/7/persons", `{"first_name":"A","last_name":"B"}`)

	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "create", "7", "--first-name", "A"}, create), 2)
	wantRequests(t, srv, 0)
}

// TestWriteCommandsForAllSpecOperations builds the command of every
// create/update operation: every flag field gets its flag, the builder flags
// exist when the schema has the kind, and nothing collides.
func TestWriteCommandsForAllSpecOperations(t *testing.T) {
	app, _, _ := newTestApp(t)
	n := 0
	for _, op := range spec.Operations() {
		action := ""
		switch {
		case strings.HasPrefix(op.ID, "Create"):
			action = "create"
		case strings.HasPrefix(op.ID, "Update"):
			action = "update"
		default:
			continue
		}
		e := specEntry(t, op.ID, "x", action, registry.ClassCreate)
		if !generic(e) {
			continue
		}
		n++
		cmd := app.newEntryCmd(e)
		fields, err := spec.Fields(spec.RequestSchema(op.ID), resourceNames())
		if err != nil {
			t.Fatalf("%s: %v", op.ID, err)
		}
		for _, f := range fields {
			if f.Flag != "" && cmd.Flags().Lookup(f.Flag) == nil {
				t.Errorf("%s: Flag --%s fehlt", op.ID, f.Flag)
			}
			if f.Kind == spec.KindItems {
				for _, fl := range []string{"items", "replace-items"} {
					has := cmd.Flags().Lookup(fl) != nil
					if want := fl == "items" || action == "update"; has != want {
						t.Errorf("%s: --%s vorhanden = %v", op.ID, fl, has)
					}
				}
			}
		}
		if cmd.Flags().Lookup("input") == nil {
			t.Errorf("%s: --input fehlt", op.ID)
		}
	}
	if n < 15 {
		t.Errorf("nur %d create/update-Operationen geprüft", n)
	}
}

func TestWriteMultipartEntryNotGeneric(t *testing.T) {
	upload := specEntry(t, "UploadVoucherDocument", "voucher", "create", registry.ClassCreate)
	if generic(upload) {
		t.Error("multipart-Eintrag würde vom generischen Builder gebaut")
	}
	if !generic(specEntry(t, "CreateVoucher", "voucher", "create", registry.ClassCreate)) {
		t.Error("CreateVoucher nicht generisch")
	}
}

// TestReservedFlagsCoverRootFlags keeps the reserved flags of spec.Fields
// in sync with the global flags and the builder flags (Task 19 carry): a
// top-level request field named like one of them has no longer name to
// fall back to, so Fields must report the collision. Only --item and
// --attr belong to their own field kinds.
func TestReservedFlagsCoverRootFlags(t *testing.T) {
	app, _, _ := newTestApp(t)
	root := NewRootCmd(app)
	var names []string
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	names = append(names, builderFlags...)
	for _, name := range names {
		prop := strings.ReplaceAll(name, "-", "_")
		req := openapi3.NewObjectSchema().WithProperty(prop, openapi3.NewStringSchema())
		if _, err := spec.Fields(req, nil); err == nil {
			t.Errorf("--%s fehlt in den reservierten Flags von spec.Fields (Feld %q kollidiert unbemerkt)", name, prop)
		}
	}
}

// runGerman runs args against the real root with entries attached and the
// German templates applied afterwards, as in the full command tree.
func runGerman(t *testing.T, app *App, args []string, entries ...registry.Entry) int {
	t.Helper()
	root := NewRootCmd(app)
	for _, e := range entries {
		group := &cobra.Command{Use: e.Resource}
		if g, _, err := root.Find([]string{e.Resource}); err == nil && g != root {
			group = g
		} else {
			root.AddCommand(group)
		}
		group.AddCommand(app.newEntryCmd(e))
	}
	applyGerman(root)
	return executeContext(context.Background(), app, root, args)
}

func TestWriteHelpGerman(t *testing.T) {
	entries := append(writeEntries(t), specEntry(t, "UpdateTimeEntry", "time", "update", registry.ClassUpdate))
	for _, args := range [][]string{{"invoice", "update", "--help"}, {"time", "update", "--help"}} {
		app, _ := apiApp(t)
		wantExit(t, app, runGerman(t, app, args, entries...), 0)
		out, _ := bufs(app)
		for _, bad := range []string{"(Standard: )", "help for", "[flags]"} {
			if strings.Contains(out, bad) {
				t.Errorf("%v: Hilfe enthält %q:\n%s", args, bad, out)
			}
		}
		if !strings.Contains(out, "Hilfe zu update") {
			t.Errorf("%v: deutsche Hilfe-Zeile fehlt:\n%s", args, out)
		}
	}

	app, _ := apiApp(t)
	wantExit(t, app, runGerman(t, app, []string{"invoice", "update", "--help"}, entries...), 0)
	out, _ := bufs(app)
	for _, want := range []string{
		"--flagged Wert für flagged (true/false)",
		"--customer ID ID für customer",
		"--document-date Datum Wert für document_date (YYYY-MM-DD, heute, gestern)",
		"--item Texte Position als key=wert,key=wert (wiederholbar)",
		"--items Text Positionen als JSON-Array aus Datei oder - für die Standardeingabe",
		"--input Text Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder",
		"--replace-items alle Positionen ersetzen (nötig für --item/--items; ohne Positionen werden alle gelöscht)",
	} {
		if !slices.Contains(helpLines(out), want) {
			t.Errorf("Hilfe enthält die Zeile %q nicht:\n%s", want, out)
		}
	}

	app, _ = apiApp(t)
	wantExit(t, app, runGerman(t, app, []string{"time", "update", "--help"}, entries...), 0)
	if out, _ := bufs(app); !slices.Contains(helpLines(out), "--billable-duration Zahl Abrechenbare Dauer in Sekunden; wirkungslos: Die API setzt beim Anlegen und Ändern die Dauer aus Beginn und Ende ein (gemessen 2026-11-04)") {
		t.Errorf("Hilfe:\n%s", out)
	}
}

// helpLines returns the help lines with whitespace runs collapsed.
func helpLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		lines = append(lines, strings.Join(strings.Fields(l), " "))
	}
	return lines
}

func TestWriteFlagOnlyUsageErrorsBeforeConfig(t *testing.T) {
	// no token: anything after a.Client would be exit 3
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"voucher", "create", "--item", "name=A,billing=5"}, `Feld "billing" ist ein Objekt`},
		{[]string{"invoice", "create", "--item", "nam=A"}, `unbekannter Schlüssel "nam"`},
		{[]string{"invoice", "create", "--item", `name="offen`}, "--item"},
		{[]string{"invoice", "create", "--input", ""}, "--input: Dateiname fehlt"},
		{[]string{"invoice", "create", "--input=", "--name", "R"}, "--input: Dateiname fehlt"},
		{[]string{"invoice", "create", "--items", ""}, "--items: Dateiname fehlt"},
	} {
		app, srv := mockApp(t, nil)
		wantExit(t, app, runWrite(t, app, tc.args...), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}

func TestWriteNullLineItemsWithReplace(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"name":"Neu","line_items":null}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "--replace-items"), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "Fehler: line_items: null ist nicht erlaubt – für leere Positionsliste [] angeben oder Positionen per --item/--items setzen\n") {
		t.Errorf("stderr = %q", errs)
	}
}

// Positions per --item/--items win over line_items: null of --input, like
// every flag over --input (task 17, L213).
func TestWriteNullLineItemsFlagsWin(t *testing.T) {
	items := writeFile(t, "items.json", `[{"name":"B","quantity":1,"price":5,"vat_rate":0.19}]`)
	for _, flags := range [][]string{
		{"--item", item1},
		{"--items", items},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(`{"name":"Neu","line_items":null}`)
		args := append([]string{"invoice", "update", "12", "--input", "-", "--replace-items", "-o", "json"}, flags...)
		wantExit(t, app, runWrite(t, app, args...), 0)
		want := `{"name":"Neu","line_items":[{"name":"Beratung","quantity":2,"price":100,"vat_rate":0.19}]}`
		if flags[0] == "--items" {
			want = `{"name":"Neu","line_items":[{"name":"B","quantity":1,"price":5,"vat_rate":0.19}]}`
		}
		wantBody(t, srv, "PUT", "/income/invoices/12", want)
		if _, errs := bufs(app); errs != "" {
			t.Errorf("%v: stderr = %q", flags, errs)
		}
	}
}

func TestWriteDroppedInputItemsNotPruned(t *testing.T) {
	// positions that are not sent produce neither prune hints nor errors
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"name":"Neu","line_items":[{"nam":"x","total_net":5,"proposition":null}]}`)
	wantExit(t, app, runWrite(t, app, "invoice", "update", "12", "--input", "-", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/income/invoices/12", `{"name":"Neu"}`)
	if _, errs := bufs(app); errs != itemsDropHint {
		t.Errorf("stderr = %q", errs)
	}
}

func TestWriteEmptyBoolFlagIsInvalid(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runWrite(t, app, "project", "update", "5", "--flagged="), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `ungültiger Wert "" für --flagged`) {
		t.Errorf("stderr = %q", errs)
	}
}
