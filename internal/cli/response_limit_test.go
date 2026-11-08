package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Answers longer than api.MaxResponseSize (spec 5.5) on the paths that
// interpret a client error themselves.

// fillReader yields an endless stream of 'a' without allocating it.
type fillReader struct{}

func (fillReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// tooLarge answers with status and a streamed body one byte longer than
// api.MaxResponseSize.
func tooLarge(status int, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		size := int64(api.MaxResponseSize + 1)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(status)
		_, _ = io.CopyN(w, fillReader{}, size)
	}
}

// pdf -O never saves a cut PDF: no file, no temporary file left.
func TestPdfTooLargeSavesNothing(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/income/invoices/{id}/pdf", tooLarge(http.StatusOK, "application/pdf"))
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pdf")
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path}), 1)
	wantRequests(t, srv, 1)
	wantStderr(t, app, "Antwort größer als 64 MiB, abgebrochen")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files left: %v", entries)
	}
}

// A mutation answered too large is exit 1 with the hint not to repeat;
// the request is sent once.
func TestMutationTooLargeIsExit1(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("PUT", "/income/invoices/{id}", tooLarge(http.StatusOK, "application/json"))
	wantExit(t, app, Run(app, []string{"invoice", "update", "459", "--payment-term", "1130", "--name", "x", "-o", "json"}), 1)
	wantRequests(t, srv, 1)
	wantStderr(t, app, "Fehler: Antwort größer als 64 MiB; die Änderung wurde ausgeführt",
		"Hinweis: NICHT wiederholen; Ergebnis per describe/list prüfen", `"code":"error"`)
	// ruling FR-D: no http_status (error.json allows only up to 499 for
	// code error; the message says the change was made)
	if _, errs := bufs(app); strings.Contains(errs, "http_status") {
		t.Errorf("stderr = %s", errs)
	}
}

// deliver keeps the hint not to repeat: the document was sent.
func TestDeliverTooLargeKeepsHint(t *testing.T) {
	app, srv := apiApp(t)
	srv.OnFunc("POST", "/income/invoices/{id}/deliver", tooLarge(http.StatusOK, "application/json"))
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "pdf", "--yes"}), 1)
	wantRequests(t, srv, 1)
	wantStderr(t, app, "die Änderung wurde ausgeführt", "Hinweis: NICHT wiederholen; Ergebnis per describe/list prüfen")
	if _, errs := bufs(app); strings.Contains(errs, "kann trotzdem erfolgt sein") {
		t.Errorf("stderr = %s", errs)
	}
}

// voucher create --file whose create answer is too large: the voucher
// exists, its ID is unknown, the file is not uploaded (exit 11).
func TestVoucherCreateFileCreateTooLarge(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.OnFunc("POST", "/expense/vouchers", tooLarge(http.StatusCreated, "application/json"))
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 11)
	wantRequests(t, srv, 1)
	wantStderr(t, app, "Beleg angelegt, Beleg-ID unbekannt: Antwort größer als 64 MiB; die Änderung wurde ausgeführt; Datei nicht hochgeladen",
		"papierkram voucher list", "NICHT den ganzen Befehl wiederholen", `"code":"partial"`)
	if out, _ := bufs(app); !strings.Contains(out, `"retry_command": null`) {
		t.Errorf("stdout = %s", out)
	}
}

// An upload answered too large was made: check with describe before the
// retry command (uncertain), never "Hochladen fehlgeschlagen".
func TestVoucherCreateFileUploadTooLarge(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.OnFunc("POST", "/expense/vouchers/290/documents", tooLarge(http.StatusOK, "application/json"))
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 11)
	wantRequests(t, srv, 2)
	wantStderr(t, app, "Beleg 290 angelegt, Ergebnis des Hochladens unbekannt", "papierkram voucher describe 290")
	if _, errs := bufs(app); strings.Contains(errs, "Hochladen fehlgeschlagen") {
		t.Errorf("stderr = %s", errs)
	}
}

func TestUploadUncertainTooLarge(t *testing.T) {
	ce := &clierr.Error{Code: clierr.CodeError, Cause: api.ErrResponseTooLarge}
	if !uploadUncertain(ce) {
		t.Error("too large upload answer is not uncertain")
	}
}

// time stop answered too large: the entry exists, so the hint says to
// check with time list and discard, never to stop again (T15-M4: no
// "sonst erneut papierkram time stop" either).
func TestTimerStopTooLarge(t *testing.T) {
	app, srv := logApp(t, nine(t))
	srv.OnFunc("POST", timeEntries, tooLarge(http.StatusCreated, "application/json"))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 1)
	wantStderr(t, app, "die Änderung wurde ausgeführt", "NICHT wiederholen",
		"Der Timer läuft weiter, der Eintrag wurde angelegt: mit papierkram time list prüfen, dann den Timer mit papierkram time discard verwerfen\n")
	if _, errs := bufs(app); strings.Contains(errs, "erneut papierkram time stop") {
		t.Errorf("stderr = %s", errs)
	}
	if after := timerFile(t, app, ""); string(after) != string(before) {
		t.Errorf("state changed: %s → %s", before, after)
	}
	if claimFile(t, app, "") != nil {
		t.Error("claim left")
	}
}

// Reads above the limit (ruling T15-A): list names --limit, api on a list
// names page_size, every other read gets no size advice.
func TestReadTooLargeHints(t *testing.T) {
	const neutral = "Hinweis: Antwort zu groß für das CLI; Ergebnis nicht verwertbar"
	for _, tc := range []struct {
		name, path string
		args       []string
		hint       string
	}{
		{"list", "/income/invoices", []string{"invoice", "list", "-o", "json"}, "Hinweis: mit --limit kleinere Seiten anfordern"},
		{"list --all", "/income/invoices", []string{"invoice", "list", "--all", "-o", "json"}, "Hinweis: mit --limit kleinere Seiten anfordern"},
		{"api list", "/income/invoices", []string{"api", "GET", "/income/invoices", "-o", "json"},
			"Hinweis: mit kleinerem page_size kleinere Seiten anfordern (z. B. -f page_size=25)"},
		{"api --paginate", "/income/invoices", []string{"api", "GET", "/income/invoices", "--paginate", "-o", "json"},
			"Hinweis: mit kleinerem page_size kleinere Seiten anfordern (z. B. -f page_size=25)"},
		{"describe", "/income/invoices/{id}", []string{"invoice", "describe", "459", "-o", "json"}, neutral},
		{"api describe", "/income/invoices/{id}", []string{"api", "GET", "/income/invoices/459", "-o", "json"}, neutral},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			srv.OnFunc("GET", tc.path, tooLarge(http.StatusOK, "application/json"))
			wantExit(t, app, Run(app, tc.args), 1)
			wantRequests(t, srv, 1)
			wantStderr(t, app, "Fehler: Antwort größer als 64 MiB, abgebrochen", tc.hint, `"code":"error"`)
			if _, errs := bufs(app); strings.Contains(errs, "--fields") {
				t.Errorf("stderr = %s", errs)
			}
		})
	}
}

// T15-M5: the read of invoice update answered too large changes nothing;
// the hint says how to do without the read: the payment term per
// --payment-term (or, when only the references were read, their flags).
func TestInvoiceUpdateReadTooLarge(t *testing.T) {
	for _, tc := range []struct {
		args []string
		hint string
	}{
		{[]string{"--description", "x"}, "Zahlungsbedingung per --payment-term <id> angeben (papierkram payment-term list)"},
		{[]string{"--customer", "3214", "--payment-term", "7"}, "Kontaktperson und Projekt per --contact-person/--customer-project angeben"},
		{[]string{"--customer", "3214"}, "Zahlungsbedingung per --payment-term <id> angeben (papierkram payment-term list); Kontaktperson und Projekt per --contact-person/--customer-project angeben"},
	} {
		app, srv := apiApp(t)
		srv.OnFunc("GET", "/income/invoices/{id}", tooLarge(http.StatusOK, "application/json"))
		wantExit(t, app, Run(app, append([]string{"invoice", "update", "459", "-o", "json"}, tc.args...)), 1)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
			t.Errorf("%v: request = %s", tc.args, r.Method)
		}
		wantStderr(t, app, "Rechnung 459 nicht gelesen, nichts geändert: Antwort größer als 64 MiB, abgebrochen", tc.hint)
	}
}

// Review T15-M1: invoice create --from-estimate whose estimate is
// answered too large creates nothing: exit 1 naming that no invoice was
// created, no POST (--dry-run reads as well and stops the same way).
func TestFromEstimateReadTooLarge(t *testing.T) {
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		app, srv := apiApp(t)
		srv.OnFunc("GET", "/income/estimates/{id}", tooLarge(http.StatusOK, "application/json"))
		wantExit(t, app, Run(app, append([]string{"invoice", "create", "--from-estimate", "349", "--payment-term", "1130", "-o", "json"}, extra...)), 1)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" || r.Path != "/income/estimates/349" {
			t.Errorf("%v: request = %s %s", extra, r.Method, r.Path)
		}
		wantStderr(t, app, "Fehler: Angebot 349 nicht gelesen, keine Rechnung angelegt: Antwort größer als 64 MiB, abgebrochen", `"code":"error"`)
		if out := mustOut(app); out != "" {
			t.Errorf("%v: stdout = %q", extra, out)
		}
	}
}

// Review T15-M1: deliver answered 2xx whose readback is too large is sent:
// exit 0 with {"id","delivered","warnings"} and the warning not to run it
// again, never a second POST.
func TestDeliverReadbackTooLarge(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
	srv.OnFunc("GET", "/income/invoices/{id}", tooLarge(http.StatusOK, "application/json"))
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}), 0)
	rs := wantRequests(t, srv, 2)
	if rs[0].Method != "POST" || rs[1].Method != "GET" {
		t.Errorf("requests = %s, %s", rs[0].Method, rs[1].Method)
	}
	const warning = "Versand erfolgreich, Nachlesen fehlgeschlagen: Antwort größer als 64 MiB, abgebrochen – NICHT erneut ausführen"
	out, errs := bufs(app)
	wantContract(t, "deliver", []byte(out))
	if !strings.Contains(out, `"delivered": true`) || !strings.Contains(out, warning) || errs != "Warnung: "+warning+"\n" {
		t.Errorf("stdout = %s\nstderr = %q", out, errs)
	}
}
