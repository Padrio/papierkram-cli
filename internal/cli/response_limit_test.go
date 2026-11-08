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
		"Hinweis: NICHT wiederholen; Ergebnis per describe/list prüfen", `"code":"error"`, `"http_status":200`)
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
	ce := &clierr.Error{Code: clierr.CodeError, HTTPStatus: 200, Cause: api.ErrResponseTooLarge}
	if !uploadUncertain(ce) {
		t.Error("too large upload answer is not uncertain")
	}
}

// time stop answered too large: the entry exists, so the hint says to
// check with time list and discard, never just to stop again.
func TestTimerStopTooLarge(t *testing.T) {
	app, srv := logApp(t, nine(t))
	srv.OnFunc("POST", timeEntries, tooLarge(http.StatusCreated, "application/json"))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	wantExit(t, app, at(app, morning(t), "time", "stop"), 1)
	wantStderr(t, app, "die Änderung wurde ausgeführt", "NICHT wiederholen",
		"erst mit papierkram time list prüfen, ob der Eintrag angelegt wurde", "papierkram time discard")
	if _, errs := bufs(app); strings.Contains(errs, "Der Timer läuft weiter: erneut papierkram time stop") {
		t.Errorf("stderr = %s", errs)
	}
	if after := timerFile(t, app, ""); string(after) != string(before) {
		t.Errorf("state changed: %s → %s", before, after)
	}
	if claimFile(t, app, "") != nil {
		t.Error("claim left")
	}
}
