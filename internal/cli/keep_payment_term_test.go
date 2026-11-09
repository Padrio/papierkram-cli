package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// Ruling E2E-A (live run 3, 2026-11-04): a PUT of an invoice without
// payment_term switches it to the account's default payment term and
// changes due_date. invoice update without --payment-term and without
// payment_term in --input therefore reads the invoice first (like
// --from-estimate reads the estimate) and sends its current
// payment_term.id along.

// invoiceWithTerm answers GET /income/invoices/459 with payment term id.
func invoiceWithTerm(srv interface {
	On(method, path string, status int, body []byte, header ...string)
}, term string) {
	srv.On("GET", "/income/invoices/459", http.StatusOK,
		[]byte(`{"id":459,"type":"invoice","name":"Beratung","payment_term":`+term+`}`))
}

func TestInvoiceUpdateKeepsPaymentTerm(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithTerm(srv, `{"id":31,"name":"Barzahlung","template_id":1,"type":"payment_term"}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "GET" || reqs[0].Path != "/income/invoices/459" || reqs[1].Method != "PUT" || reqs[1].Path != "/income/invoices/459" {
		t.Fatalf("requests = %s %s, %s %s", reqs[0].Method, reqs[0].Path, reqs[1].Method, reqs[1].Path)
	}
	if got, want := canonJSON(t, reqs[1].Body), canonJSON(t, []byte(`{"description":"Neu","payment_term":{"id":31}}`)); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: payment_term 31 der Rechnung mitgesendet") {
		t.Errorf("stderr = %q", errs)
	}
}

// Who names the payment term pays no read: per flag or in --input.
func TestInvoiceUpdateWithPaymentTermSendsNoRead(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "--payment-term", "1131", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/invoices/459", `{"description":"Neu","payment_term":{"id":1131}}`)

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"description":"Neu","payment_term":{"id":1132}}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/invoices/459", `{"description":"Neu","payment_term":{"id":1132}}`)

	// --input without payment_term (or with null, which Prune drops):
	// read
	for _, in := range []string{`{"description":"Neu"}`, `{"description":"Neu","payment_term":null}`} {
		app, srv = apiApp(t)
		invoiceWithTerm(srv, `{"id":31,"template_id":1}`)
		app.In = strings.NewReader(in)
		wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "-o", "json"}), 0)
		if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"description":"Neu","payment_term":{"id":31}}`)) {
			t.Errorf("%s: body = %s", in, reqs[1].Body)
		}
	}
}

// Errors of the flags and the body cost no read; estimate update has no
// such step (run 3 showed no reset there).
func TestInvoiceUpdateReadOnlyWhenNeeded(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "update", "459"},                                          // nothing to change: exit 2
		{"invoice", "update", "459", "--item", "name=A,qty=1,price=5,vat=19"}, // without --replace-items
		{"invoice", "update", "459", "--document-date", "1.1.2026"},           // exit 5
	} {
		app, srv := apiApp(t)
		if code := Run(app, args); code != 2 && code != 5 {
			t.Errorf("%v: exit %d", args, code)
		}
		wantRequests(t, srv, 0)
	}
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "update", "349", "--description", "Neu", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/estimates/349", `{"description":"Neu"}`)

	// read-only: exit 9 before the read
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "--dry-run"}), 9)
	wantRequests(t, srv, 0)
}

// --dry-run reads the invoice (like --from-estimate the estimate), shows
// the read as spent and counts it in the credits of the real call.
func TestInvoiceUpdateKeepPaymentTermDryRun(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithTerm(srv, `{"id":31,"template_id":1}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "--dry-run", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" || r.Path != "/income/invoices/459" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	var got struct {
		Method  string          `json:"method"`
		Credits int             `json:"credits"`
		Body    json.RawMessage `json:"body"`
		Read    *ReadInfo       `json:"read"`
	}
	out := mustOut(app)
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got.Method != "PUT" || got.Credits != 2 || canonJSON(t, got.Body) != canonJSON(t, []byte(`{"description":"Neu","payment_term":{"id":31}}`)) {
		t.Errorf("dry run = %s", out)
	}
	if got.Read == nil || got.Read.Method != "GET" || got.Read.URL != srv.URL()+"/income/invoices/459" || got.Read.Credits != 1 {
		t.Errorf("read = %+v", got.Read)
	}

	app, srv = apiApp(t)
	invoiceWithTerm(srv, `{"id":31,"template_id":1}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "--dry-run"}), 0)
	wantRequests(t, srv, 1)
	head := "Probelauf – nichts geändert\nGelesen: GET " + srv.URL() + "/income/invoices/459 (1 Credit, bereits verbraucht)\n" +
		"Methode: PUT\nURL:     " + srv.URL() + "/income/invoices/459\nCredits: 2 (erneutes Lesen und Ändern)\n"
	if text := mustOut(app); !strings.HasPrefix(text, head) {
		t.Errorf("stdout = %s", text)
	}

	// with --payment-term: no read, 1 credit
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--payment-term", "7", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, `"credits": 1`) || strings.Contains(out, `"read"`) {
		t.Errorf("stdout = %s", out)
	}
}

// A failed read keeps its code and sends nothing, like --from-estimate;
// an answer without payment term id sends nothing either.
func TestInvoiceUpdateKeepPaymentTermReadFails(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices/459", http.StatusNotFound,
		[]byte(`{"type":"error","error_type":"not_found","message":"Rechnung nicht gefunden"}`))
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu", "-o", "json"}), 4)
	wantRequests(t, srv, 1)
	if out, errs := bufs(app); out != "" || !strings.Contains(errs, "Rechnung 459 nicht gelesen, nichts geändert") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}

	for _, answer := range []string{`[]`, `{"id":459,"payment_term":null}`, `{"id":459}`, `{"id":459} {}`} {
		app, srv := apiApp(t)
		srv.On("GET", "/income/invoices/459", http.StatusOK, []byte(answer))
		wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--description", "Neu"}), 1)
		wantRequests(t, srv, 1)
		if _, errs := bufs(app); !strings.Contains(errs, "nichts geändert") || !strings.Contains(errs, "--payment-term") {
			t.Errorf("%s: stderr = %q", answer, errs)
		}
	}
}

// commands --json names the credits of a run with the read; the help says
// when the read happens.
func TestInvoiceUpdateKeepPaymentTermContract(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"commands", "--json", "--jq", `.commands[] | select(.command == "invoice update") | .credits`}), 0)
	wantRequests(t, srv, 0)
	if out := strings.TrimSpace(mustOut(app)); out != "2" {
		t.Errorf("credits = %q, want 2", out)
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "update", "--help"}), 0)
	if out := mustOut(app); !strings.Contains(out, "Kosten: 1 Credit je Anfrage") || !strings.Contains(out, "liest das CLI die Rechnung vorher") {
		t.Errorf("help = %s", out)
	}
	// the flag says that it saves the read (review M5); commands --json
	// carries the same description
	out := mustOut(app)
	i := strings.Index(out, "--payment-term ID")
	if line, _, _ := strings.Cut(out[max(i, 0):], "\n"); i < 0 || !strings.Contains(line, "spart das Lesen der Rechnung (1 Credit)") {
		t.Errorf("--payment-term: %q", line)
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"commands", "--json", "--jq",
		`.commands[] | select(.command == "invoice update") | .flags[] | select(.name == "payment-term") | .description`}), 0)
	if out := mustOut(app); !strings.Contains(out, "spart das Lesen der Rechnung") {
		t.Errorf("commands --json: %q", out)
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "update", "--help"}), 0)
	if strings.Contains(mustOut(app), "spart das Lesen") {
		t.Error("estimate update nennt ein Lesen")
	}
}

// Open point 29 (task 8): a PUT of an invoice whose customer lacks
// contact_person and project clears both (measured 2026-11-04), also for
// the same customer. invoice update --customer without --contact-person
// and --customer-project therefore uses the read of the payment term: the
// same customer keeps both, another one drops them (dropForeignRefs).

// invoiceWithRefs answers GET /income/invoices/459 with customer 3214,
// contact person 55, project 66 and payment term 31 (refs: the top-level
// references of the answer, "" for none).
func invoiceWithRefs(srv interface {
	On(method, path string, status int, body []byte, header ...string)
}, refs string) {
	srv.On("GET", "/income/invoices/459", http.StatusOK,
		[]byte(`{"id":459,"type":"invoice","name":"Beratung","payment_term":{"id":31,"name":"Bar"},`+
			`"customer":{"type":"company","id":3214,"name":"Kunde"}`+refs+`}`))
}

const bothRefs = `,"contact_person":{"type":"person","id":55,"first_name":"Max"},"project":{"type":"project","id":66,"name":"P"}`

func TestInvoiceUpdateSameCustomerKeepsRefs(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "GET" || reqs[1].Method != "PUT" {
		t.Fatalf("requests = %s, %s", reqs[0].Method, reqs[1].Method)
	}
	want := `{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}},"payment_term":{"id":31}}`
	if got := canonJSON(t, reqs[1].Body); got != canonJSON(t, []byte(want)) {
		t.Errorf("body = %s, want %s", got, want)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kontaktperson 55 und Projekt 66 der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)\n") {
		t.Errorf("stderr = %q", errs)
	}

	// only the references the invoice has are sent and named
	app, srv = apiApp(t)
	invoiceWithRefs(srv, `,"project":{"type":"project","id":66,"name":"P"}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":3214,"project":{"id":66}},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt 66 der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)\n") || strings.Contains(errs, "Kontaktperson") {
		t.Errorf("stderr = %q", errs)
	}

	// an invoice without both: nothing taken over, no hint
	app, srv = apiApp(t)
	invoiceWithRefs(srv, "")
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":3214},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	if _, errs := bufs(app); strings.Contains(errs, "der Rechnung mitgesendet") || strings.Contains(errs, "nicht übernommen") {
		t.Errorf("stderr = %q", errs)
	}
}

// open point 29 with name resolution (task 3): --customer by name is
// resolved before the read, which compares the resolved ID with the
// customer of the invoice.
func TestInvoiceUpdateCustomerNameKeepsRefs(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(3214, "Mustermann GmbH"), company(99, "ACME")}))
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "Mustermann", "-o", "json"}), 4)
	wantRequests(t, srv, 1) // a part of the name is no name

	app, srv = apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(3214, "Mustermann GmbH"), company(99, "ACME")}))
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "Mustermann GmbH", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 3)
	for i, want := range []string{"GET /contact/companies", "GET /income/invoices/459", "PUT /income/invoices/459"} {
		if got := reqs[i].Method + " " + reqs[i].Path; got != want {
			t.Errorf("Request %d: %s, erwartet %s", i+1, got, want)
		}
	}
	want := `{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}},"payment_term":{"id":31}}`
	if got := canonJSON(t, reqs[2].Body); got != canonJSON(t, []byte(want)) {
		t.Errorf("body = %s, want %s", got, want)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kontaktperson 55 und Projekt 66 der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)\n") {
		t.Errorf("stderr = %q", errs)
	}

	// another customer by name drops them
	app, srv = apiApp(t)
	srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(3214, "Mustermann GmbH"), company(99, "ACME")}))
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "acme", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 3); canonJSON(t, reqs[2].Body) != canonJSON(t, []byte(`{"customer":{"id":99},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[2].Body)
	}
}

func TestInvoiceUpdateOtherCustomerDropsRefs(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "99", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":99},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	_, errs := bufs(app)
	if !strings.Contains(errs, "Hinweis: Kunde 99 statt 3214 aus der Rechnung: contact_person und project nicht übernommen (setzen per --contact-person bzw. --customer-project)\n") {
		t.Errorf("stderr = %q", errs)
	}
	if strings.Contains(errs, "gleicher Kunde") {
		t.Errorf("stderr = %q", errs)
	}
}

// A reference flag wins over the invoice; with both flags (or without
// --customer) nothing is read for the references.
func TestInvoiceUpdateRefFlagsWin(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--contact-person", "77", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":3214,"contact_person":{"id":77},"project":{"id":66}},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt 66 der Rechnung mitgesendet") || strings.Contains(errs, "Kontaktperson 55") {
		t.Errorf("stderr = %q", errs)
	}

	// another customer with one flag: the flag stays, the hint names the
	// other reference only
	app, srv = apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "99", "--contact-person", "77", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":99,"contact_person":{"id":77}},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kunde 99 statt 3214 aus der Rechnung: project nicht übernommen (setzen per --customer-project)\n") {
		t.Errorf("stderr = %q", errs)
	}

	for _, args := range [][]string{
		{"invoice", "update", "459", "--customer", "3214", "--contact-person", "77", "--customer-project", "88", "--payment-term", "7", "-o", "json"},
		{"invoice", "update", "459", "--description", "Neu", "--payment-term", "7", "-o", "json"},
		{"invoice", "update", "459", "--contact-person", "77", "--payment-term", "7", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		if reqs := wantRequests(t, srv, 1); reqs[0].Method != "PUT" {
			t.Errorf("%v: request = %s", args, reqs[0].Method)
		}
	}

	// a customer change against --input needs no read for the
	// references: they belong to the old customer (dry run: 1 credit)
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"customer":{"id":3214},"payment_term":{"id":7}}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "--customer", "99", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, `"credits": 1`) || strings.Contains(out, `"read"`) {
		t.Errorf("stdout = %s", out)
	}

	// references in --input count as given
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}}}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "--customer", "3214", "--payment-term", "7", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/invoices/459", `{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}},"payment_term":{"id":7}}`)
}

// One read for both purposes; --dry-run reads and shows it with 2
// credits, also with --payment-term.
func TestInvoiceUpdateRefsOneRead(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "-o", "json"}), 0)
	if n := countGET(srv.Requests()); n != 1 {
		t.Errorf("%d GET, want 1", n)
	}

	for _, extra := range [][]string{nil, {"--payment-term", "7"}} {
		app, srv := apiApp(t)
		invoiceWithRefs(srv, bothRefs)
		args := append([]string{"invoice", "update", "459", "--customer", "3214", "--dry-run", "-o", "json"}, extra...)
		wantExit(t, app, Run(app, args), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" || r.Path != "/income/invoices/459" {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
		var got struct {
			Credits int             `json:"credits"`
			Body    json.RawMessage `json:"body"`
			Read    *ReadInfo       `json:"read"`
		}
		out := mustOut(app)
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		term := "31"
		if extra != nil {
			term = "7"
		}
		want := `{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}},"payment_term":{"id":` + term + `}}`
		if got.Credits != 2 || got.Read == nil || got.Read.Credits != 1 || canonJSON(t, got.Body) != canonJSON(t, []byte(want)) {
			t.Errorf("%v: dry run = %s", extra, out)
		}
	}
}

func countGET(reqs []mockapi.Recorded) int {
	n := 0
	for _, r := range reqs {
		if r.Method == "GET" {
			n++
		}
	}
	return n
}

// A failed read sends nothing (exit 4); read-only stops before the read.
func TestInvoiceUpdateRefsReadFails(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices/459", http.StatusNotFound,
		[]byte(`{"type":"error","error_type":"not_found","message":"Rechnung nicht gefunden"}`))
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--payment-term", "7", "-o", "json"}), 4)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "Rechnung 459 nicht gelesen, nichts geändert") {
		t.Errorf("stderr = %q", errs)
	}

	// an answer that is no object, or one Prune cannot take (a field of
	// neither schema), sends nothing either; the hint names the flags
	for _, answer := range []string{`[]`, `{"id":459,"customer":{"id":3214},"neu":1}`} {
		app, srv := apiApp(t)
		srv.On("GET", "/income/invoices/459", http.StatusOK, []byte(answer))
		wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--payment-term", "7"}), 1)
		wantRequests(t, srv, 1)
		if _, errs := bufs(app); !strings.Contains(errs, "nichts geändert") || !strings.Contains(errs, "per --contact-person/--customer-project angeben") {
			t.Errorf("%s: stderr = %q", answer, errs)
		}
	}

	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--customer", "3214", "--payment-term", "7"}), 9)
	wantRequests(t, srv, 0)
}

// T8-M1: a reference that --input brings (and that is sent) is not named
// as dropped when the invoice belongs to another customer: the hint names
// only what is really left out.
func TestInvoiceUpdateForeignRefsHintSkipsInputRefs(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	app.In = strings.NewReader(`{"customer":{"id":99,"contact_person":{"id":5}}}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "--customer", "99", "--payment-term", "7", "-o", "json"}), 0)
	if reqs := wantRequests(t, srv, 2); canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(`{"customer":{"id":99,"contact_person":{"id":5}},"payment_term":{"id":7}}`)) {
		t.Errorf("body = %s", reqs[1].Body)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kunde 99 statt 3214 aus der Rechnung: project nicht übernommen (setzen per --customer-project)\n") {
		t.Errorf("stderr = %q", errs)
	}
}

// T8-M3: --input with the same customer as --customer and without
// references reads once and sends both references of the invoice.
func TestInvoiceUpdateInputSameCustomerKeepsRefs(t *testing.T) {
	app, srv := apiApp(t)
	invoiceWithRefs(srv, bothRefs)
	app.In = strings.NewReader(`{"customer":{"id":3214}}`)
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--input", "-", "--customer", "3214", "--payment-term", "7", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2)
	if n := countGET(reqs); n != 1 {
		t.Errorf("%d GET, want 1", n)
	}
	want := `{"customer":{"id":3214,"contact_person":{"id":55},"project":{"id":66}},"payment_term":{"id":7}}`
	if reqs[1].Method != "PUT" || canonJSON(t, reqs[1].Body) != canonJSON(t, []byte(want)) {
		t.Errorf("%s body = %s, want %s", reqs[1].Method, reqs[1].Body, want)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Kontaktperson 55 und Projekt 66 der Rechnung mitgesendet") {
		t.Errorf("stderr = %q", errs)
	}
}
