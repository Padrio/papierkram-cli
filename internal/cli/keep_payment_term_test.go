package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
