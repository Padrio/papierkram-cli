package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// voucher upload, voucher document delete and voucher create --file
// (spec 9.3, 8.1, 8.4, 10.4).

// pdfContent is the content of the test files.
const pdfContent = "%PDF-1.4\n%%EOF\n"

// uploadPart parses the multipart body of an upload request and returns
// its only part with the part data.
func uploadPart(t *testing.T, r mockapi.Recorded) (*multipart.Part, string) {
	t.Helper()
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		t.Fatalf("Content-Type = %q (%v)", r.Header.Get("Content-Type"), err)
	}
	mr := multipart.NewReader(bytes.NewReader(r.Body), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("NextPart: %v", err)
	}
	data, _ := io.ReadAll(part)
	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("mehr als ein Teil: %v", err)
	}
	return part, string(data)
}

// voucherCreateArgs creates a valid voucher.
var voucherCreateArgs = []string{"voucher", "create", "--name", "Papier", "--provenance", "domestic",
	"--item", "name=Papier,amount=11.90,vat=19,category=Bürobedarf"}

func TestVoucherUpload(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t) // no TTY, no --yes: class create needs no confirmation
	wantExit(t, app, Run(app, []string{"voucher", "upload", "295", path, "-o", "json"}), 0)
	r := wantRequests(t, srv, 1)[0]
	if r.Method != "POST" || r.Path != "/expense/vouchers/295/documents" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	part, data := uploadPart(t, r)
	if part.FormName() != "file" || part.FileName() != "beleg.pdf" || part.Header.Get("Content-Type") != "application/pdf" || data != pdfContent {
		t.Errorf("Teil: name %q, Datei %q, Typ %q, Inhalt %q", part.FormName(), part.FileName(), part.Header.Get("Content-Type"), data)
	}
	if out, errs := bufs(app); !strings.Contains(out, `"type": "document"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
	assertNoToken(t, app)

	// alias, image type, table output
	path = writeFile(t, "Quittung.PNG", "\x89PNG")
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"beleg", "upload", "7", path}), 0)
	part, _ = uploadPart(t, wantRequests(t, srv, 1)[0])
	if part.FileName() != "Quittung.PNG" || part.Header.Get("Content-Type") != "image/png" {
		t.Errorf("Teil: Datei %q, Typ %q", part.FileName(), part.Header.Get("Content-Type"))
	}
	if out := mustOut(app); !strings.Contains(out, "FELD") || !strings.Contains(out, "document") {
		t.Errorf("stdout = %q", out)
	}
}

// Only the base name is sent, without control and invisible format
// characters: never a path, nothing that hides or breaks the name.
func TestUploadName(t *testing.T) {
	for in, want := range map[string]string{
		"beleg.pdf":                            "beleg.pdf",
		filepath.Join("a", "b", "beleg.pdf"):   "beleg.pdf",
		`C:\Users\anna\Belege\beleg.pdf`:       "beleg.pdf",
		"be\u200bleg\u202e\x07\r\n.pdf":        "beleg.pdf",
		" mein beleg.pdf ":                     "mein beleg.pdf",
		"Bürobedarf \"Oktober\".pdf":           "Bürobedarf \"Oktober\".pdf",
		filepath.Join("dir", "\u200b\x01\x02"): "datei",
	} {
		if got := uploadName(in); got != want {
			t.Errorf("uploadName(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

func TestVoucherUploadSendsSanitizedName(t *testing.T) {
	path := writeFile(t, "be\u200bleg.pdf", pdfContent)
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "upload", "295", path, "-o", "json"}), 0)
	if part, _ := uploadPart(t, wantRequests(t, srv, 1)[0]); part.FileName() != "beleg.pdf" {
		t.Errorf("Dateiname = %q", part.FileName())
	}
}

// uploadFileCases are files that cannot be uploaded: each is exit 2 before
// any request (no credit, no orphaned voucher on create --file).
func uploadFileCases(t *testing.T) map[string]struct{ path, want string } {
	t.Helper()
	dir := t.TempDir()
	cases := map[string]struct{ path, want string }{
		"fehlt":       {filepath.Join(dir, "fehlt.pdf"), "nicht gefunden"},
		"Verzeichnis": {dir, "ist ein Verzeichnis"},
		"leer":        {writeFile(t, "leer.pdf", ""), "ist leer"},
		"ohne Namen":  {"", "Dateiname fehlt"},
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		locked := writeFile(t, "gesperrt.pdf", pdfContent)
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		cases["nicht lesbar"] = struct{ path, want string }{locked, "keine Leseberechtigung"}
	}
	return cases
}

func TestVoucherUploadFileChecks(t *testing.T) {
	for name, tc := range uploadFileCases(t) {
		for _, extra := range [][]string{nil, {"--dry-run"}} {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, append([]string{"voucher", "upload", "295", tc.path}, extra...)), 2)
			wantRequests(t, srv, 0)
			if out, errs := bufs(app); out != "" || !strings.Contains(errs, tc.want) {
				t.Errorf("%s %v: stdout = %q, stderr = %q", name, extra, out, errs)
			}
		}
	}
}

func TestVoucherUploadArgs(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	for _, args := range [][]string{
		{"voucher", "upload"},
		{"voucher", "upload", "295"},
		{"voucher", "upload", "abc", path},
		{"voucher", "upload", "-1", path},
		{"voucher", "upload", "295", path, path},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}
}

func TestVoucherUploadDryRun(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "upload", "295", path, "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	want := fmt.Sprintf(`{"body":null,"credits":10,"dry_run":true,"method":"POST","upload":{"mime_type":"application/pdf","name":"beleg.pdf","size":%d},"url":"%s/expense/vouchers/295/documents"}`,
		len(pdfContent), srv.URL())
	if out := mustOut(app); canonJSON(t, []byte(out)) != want {
		t.Errorf("stdout = %s\nerwartet %s", out, want)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "upload", "295", path, "--dry-run"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, fmt.Sprintf("Datei:   beleg.pdf (%d Bytes, application/pdf)\n", len(pdfContent))) ||
		!strings.Contains(out, "Credits: 10\n") {
		t.Errorf("stdout = %q", out)
	}
}

func TestVoucherUploadReadOnly(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, append([]string{"voucher", "upload", "295", path}, extra...)), 9)
		wantRequests(t, srv, 0)
	}
}

func TestVoucherUploadError(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.On("POST", "/expense/vouchers/{voucher_id}/documents", http.StatusNotFound,
		[]byte(`{"type":"error","error_type":"not_found","message":"Beleg nicht gefunden"}`))
	wantExit(t, app, Run(app, []string{"voucher", "upload", "295", path}), 4)
	wantRequests(t, srv, 1)
}

func TestVoucherDocumentDeleteNeedsConfirmation(t *testing.T) {
	const path = "/expense/vouchers/295/documents/32"
	for _, extra := range [][]string{nil, {"--no-input"}} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append([]string{"voucher", "document", "delete", "295", "32"}, extra...)), 8)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "Beleg 295: Datei 32 löschen") || !strings.Contains(errs, "--yes") {
			t.Errorf("%v: stderr = %q", extra, errs)
		}
	}

	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "document", "delete", "295", "32", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != path {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 32,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	// in the terminal the document ID is typed in, like for delete
	app, srv = apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("32\n")
	wantExit(t, app, Run(app, []string{"voucher", "document", "delete", "295", "32"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != path {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out, errs := bufs(app); out != "voucher 295: Datei 32 gelöscht.\n" ||
		!strings.Contains(errs, "Beleg 295: Datei 32 löschen? Zur Bestätigung ID eingeben: ") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
	for _, answer := range []string{"295\n", "ja\n", "\n"} {
		app, srv := apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		wantExit(t, app, Run(app, []string{"voucher", "document", "delete", "295", "32"}), 8)
		wantRequests(t, srv, 0)
	}
}

func TestVoucherDocumentDeleteGuards(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "document", "delete", "295", "32", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	want := `{"body":null,"credits":1,"dry_run":true,"method":"DELETE","url":"` + srv.URL() + `/expense/vouchers/295/documents/32"}`
	if out := mustOut(app); canonJSON(t, []byte(out)) != want {
		t.Errorf("stdout = %s", out)
	}

	for _, extra := range [][]string{{"--yes"}, {"--dry-run"}} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, append([]string{"voucher", "document", "delete", "295", "32"}, extra...)), 9)
		wantRequests(t, srv, 0)
	}

	for _, args := range [][]string{
		{"voucher", "document", "delete", "295", "--yes"},
		{"voucher", "document", "delete", "295", "x", "--yes"},
		{"voucher", "document", "delete", "295", "32", "33", "--yes"},
		{"voucher", "document", "loeschen", "295", "32"},
		{"voucher", "document", "delete", "--voucher", "295", "32", "--yes"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
	}

	// the group only shows its help
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "document"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, "delete") {
		t.Errorf("stdout = %q", out)
	}
}

// voucher create --file: create, then upload (spec 9.3).
func TestVoucherCreateFile(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "POST" || reqs[0].Path != "/expense/vouchers" {
		t.Errorf("create = %s %s", reqs[0].Method, reqs[0].Path)
	}
	if reqs[1].Method != "POST" || reqs[1].Path != "/expense/vouchers/290/documents" {
		t.Errorf("upload = %s %s", reqs[1].Method, reqs[1].Path)
	}
	if part, data := uploadPart(t, reqs[1]); part.FormName() != "file" || part.FileName() != "beleg.pdf" || data != pdfContent {
		t.Errorf("Teil: name %q, Datei %q, Inhalt %q", part.FormName(), part.FileName(), data)
	}
	if out, errs := bufs(app); !strings.Contains(out, `"type": "expense_voucher"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}
}

// The create request of create --file is that of create without it.
func TestVoucherCreateFileSendsSameCreate(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "-o", "json")), 0)
	plain := wantRequests(t, srv, 1)[0]
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 0)
	withFile := wantRequests(t, srv, 2)[0]
	if plain.Method != withFile.Method || plain.Path != withFile.Path || plain.Query.Encode() != withFile.Query.Encode() ||
		canonJSON(t, plain.Body) != canonJSON(t, withFile.Body) || plain.Header.Get("Content-Type") != withFile.Header.Get("Content-Type") {
		t.Errorf("create ohne --file: %s %s %s\nmit --file: %s %s %s", plain.Method, plain.Path, plain.Body, withFile.Method, withFile.Path, withFile.Body)
	}
}

// The file is checked before anything is sent: a typo in the path never
// leaves a voucher without its document.
func TestVoucherCreateFileChecksBeforeRequest(t *testing.T) {
	for name, tc := range uploadFileCases(t) {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", tc.path)), 2)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, tc.want) {
			t.Errorf("%s: stdout = %q, stderr = %q", name, out, errs)
		}
	}
	// the file check comes before the read-only gate (plan order: flags first)
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", filepath.Join(t.TempDir(), "fehlt.pdf"))), 2)
	wantRequests(t, srv, 0)
}

func TestVoucherCreateFileDryRun(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "--dry-run", "-o", "json")), 0)
	wantRequests(t, srv, 0)
	want := fmt.Sprintf(`{"body":{"line_items":[{"amount":11.90,"category":"Bürobedarf","name":"Papier","vat_rate":0.19}],"name":"Papier","provenance":"domestic"},`+
		`"credits":60,"dry_run":true,"method":"POST","upload":{"credits":10,"method":"POST","mime_type":"application/pdf","name":"beleg.pdf","size":%d,`+
		`"url":"%s/expense/vouchers/{voucher_id}/documents"},"url":"%s/expense/vouchers"}`,
		len(pdfContent), srv.URL(), srv.URL())
	if out := mustOut(app); canonJSON(t, []byte(out)) != want {
		t.Errorf("stdout = %s\nerwartet %s", out, want)
	}

	// text: the upload is visibly a second request
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "--dry-run")), 0)
	wantRequests(t, srv, 0)
	out := mustOut(app)
	second := fmt.Sprintf("\nDanach (zweite Anfrage, an den neuen Beleg):\nMethode: POST\nURL:     %s/expense/vouchers/{voucher_id}/documents\n"+
		"Credits: 10\nDatei:   beleg.pdf (%d Bytes, application/pdf)\n", srv.URL(), len(pdfContent))
	if !strings.HasPrefix(out, "Probelauf – nichts gesendet\nMethode: POST\nURL:     "+srv.URL()+"/expense/vouchers\nCredits: 60 (beide Anfragen)\n") ||
		!strings.HasSuffix(out, second) {
		t.Errorf("stdout = %s", out)
	}
}

func TestVoucherCreateFileReadOnly(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, append(append(append([]string{}, voucherCreateArgs...), "--file", path), extra...)), 9)
		wantRequests(t, srv, 0)
	}
}

// A failed create is the normal error; nothing is uploaded.
func TestVoucherCreateFileCreateFails(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.On("POST", "/expense/vouchers", http.StatusUnprocessableEntity,
		[]byte(`{"type":"error","error_type":"validation","message":"Lieferant fehlt"}`))
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 5)
	wantRequests(t, srv, 1)
	if out, errs := bufs(app); out != "" || !strings.Contains(errs, "Lieferant fehlt") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
}

// Only voucher create has --file.
func TestCreateFileOnlyOnVoucher(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "create", "--name", "R", "--file", "beleg.pdf"}), 2)
	wantRequests(t, srv, 0)
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "create", "--help"}), 0)
	if out := mustOut(app); !strings.Contains(out, "--file") {
		t.Errorf("Hilfe ohne --file:\n%s", out)
	}
}

func TestR39_VoucherFileUploadPartial(t *testing.T) {
	path := writeFile(t, "mein beleg.pdf", pdfContent)
	retry := "papierkram voucher upload 290 " + shellQuote(path)
	if !strings.HasPrefix(shellQuote(path), "'") {
		t.Fatalf("Pfad mit Leerzeichen nicht gequotet: %s", shellQuote(path))
	}
	cases := []struct {
		name    string
		answer  func(srv *mockapi.Server)
		warning string
	}{
		{"422", func(srv *mockapi.Server) {
			srv.On("POST", "/expense/vouchers/290/documents", http.StatusUnprocessableEntity,
				[]byte(`{"type":"error","error_type":"validation","message":"Dateityp nicht erlaubt"}`))
		}, "Beleg 290 angelegt, Datei nicht hochgeladen: Dateityp nicht erlaubt – nur das Hochladen nachholen: " + retry +
			"; NICHT den ganzen Befehl wiederholen (sonst ein zweiter Beleg)"},
		{"500", func(srv *mockapi.Server) {
			srv.On("POST", "/expense/vouchers/290/documents", http.StatusInternalServerError,
				[]byte(`{"type":"error","message":"kaputt"}`))
		}, "Ergebnis des Hochladens unbekannt: kaputt – die Datei ist möglicherweise schon angehängt; vor dem Nachholen per papierkram voucher describe 290 prüfen"},
		{"keine Antwort nach dem Senden", func(srv *mockapi.Server) {
			srv.OnFunc("POST", "/expense/vouchers/290/documents", func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done() // request fully read, no answer until the client gives up
			})
		}, "die Datei ist möglicherweise schon angehängt; vor dem Nachholen per papierkram voucher describe 290 prüfen"},
		{"Verbindung nach dem Senden geschlossen", func(srv *mockapi.Server) {
			srv.OnFunc("POST", "/expense/vouchers/290/documents", func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := http.NewResponseController(w).Hijack() // request fully read
				if err == nil {
					_ = conn.Close()
				}
			})
		}, "(Verbindung vom Server geschlossen) – die Datei ist möglicherweise schon angehängt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			tc.answer(srv)
			code := waitExit(t, func() int {
				return Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "--timeout", "300ms", "-o", "json"))
			})
			wantExit(t, app, code, 11)
			reqs := wantRequests(t, srv, 2) // never retried, no automatic delete
			if reqs[0].Path != "/expense/vouchers" || reqs[1].Method != "POST" || reqs[1].Path != "/expense/vouchers/290/documents" {
				t.Errorf("requests = %s %s, %s %s", reqs[0].Method, reqs[0].Path, reqs[1].Method, reqs[1].Path)
			}
			out, errs := bufs(app)
			var got struct {
				ID           int64    `json:"id"`
				Completed    []string `json:"completed"`
				Failed       []string `json:"failed"`
				RetryCommand string   `json:"retry_command"`
				Warnings     []string `json:"warnings"`
			}
			dec := json.NewDecoder(strings.NewReader(out))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&got); err != nil {
				t.Fatalf("stdout kein Ergebnisobjekt: %v\n%s", err, out)
			}
			if got.ID != 290 || strings.Join(got.Completed, ",") != "create" || strings.Join(got.Failed, ",") != "upload" ||
				got.RetryCommand != retry || len(got.Warnings) == 0 || !strings.Contains(strings.Join(got.Warnings, "\n"), tc.warning) {
				t.Errorf("stdout = %s\nerwartet Warnung %q", out, tc.warning)
			}
			if !strings.Contains(errs, `"code":"partial"`) || !strings.Contains(errs, `"exit_code":11`) || !strings.Contains(errs, retry) {
				t.Errorf("stderr = %s", errs)
			}
			if strings.Contains(out+errs, "))") {
				t.Errorf("doppelte Klammer im Text:\n%s\n%s", out, errs)
			}
			if timedOut := strings.HasPrefix(tc.name, "keine Antwort"); timedOut != strings.Contains(errs, "--timeout 120s") {
				t.Errorf("Hinweis auf --timeout nur bei Zeitüberschreitung: %s", errs)
			}
			assertNoToken(t, app)
		})
	}
}

// The partial result in the table format: German lines on stdout with ID
// and the command to retry.
func TestVoucherCreateFilePartialText(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.On("POST", "/expense/vouchers/290/documents", http.StatusUnprocessableEntity,
		[]byte(`{"type":"error","error_type":"validation","message":"Dateityp nicht erlaubt"}`))
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path)), 11)
	wantRequests(t, srv, 2)
	out, errs := bufs(app)
	want := "voucher 290 angelegt, Datei nicht hochgeladen (erledigt: create; fehlgeschlagen: upload).\n" +
		"Nachholen: papierkram voucher upload 290 " + shellQuote(path) + "\n"
	if out != want || !strings.Contains(errs, "Warnung: Beleg 290 angelegt") || !strings.Contains(errs, "Fehler: ") {
		t.Errorf("stdout = %q\nstderr = %q", out, errs)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"beleg.pdf":               "beleg.pdf",
		"/tmp/Belege/2026-10.pdf": "/tmp/Belege/2026-10.pdf",
		"mein beleg.pdf":          "'mein beleg.pdf'",
		`a"b.pdf`:                 `'a"b.pdf'`,
		"it's.pdf":                `'it'\''s.pdf'`,
		"$HOME.pdf":               "'$HOME.pdf'",
		"Bürobedarf.pdf":          "'Bürobedarf.pdf'",
		"":                        "''",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, erwartet %s", in, got, want)
		}
	}
}

// Without an ID in the create answer nothing can be uploaded: exit 11 with
// id null and the hint to look the voucher up.
func TestVoucherCreateFileWithoutID(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	for _, format := range []string{"json", "table"} {
		app, srv := apiApp(t)
		srv.On("POST", "/expense/vouchers", http.StatusCreated, nil)
		wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", format)), 11)
		wantRequests(t, srv, 1)
		out, errs := bufs(app)
		// no placeholder: <id> would be a shell redirection that empties the file
		for _, bad := range []string{"<", ">", "papierkram voucher upload"} {
			if strings.Contains(out, bad) || strings.Contains(errs, bad) {
				t.Errorf("%s: Ausgabe enthält %q:\nstdout: %s\nstderr: %s", format, bad, out, errs)
			}
		}
		if !strings.Contains(errs, "Beleg-ID unbekannt") || !strings.Contains(errs, "papierkram voucher list") {
			t.Errorf("%s: stderr = %s", format, errs)
		}
		if format == "json" {
			if canonJSON(t, []byte(out)) != `{"completed":["create"],"failed":["upload"],"id":null,"retry_command":null,"warnings":[`+
				`"Beleg angelegt, Beleg-ID unbekannt (API lieferte keine ID des angelegten Objekts): Datei nicht hochgeladen – Beleg per papierkram voucher list suchen und die Datei mit voucher upload an ihn hochladen; NICHT den ganzen Befehl wiederholen (sonst ein zweiter Beleg)"]}` {
				t.Errorf("stdout = %s", out)
			}
			if !strings.Contains(errs, `"code":"partial"`) {
				t.Errorf("stderr = %s", errs)
			}
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// Every 4xx answer to the upload, also one without its own error code
// (413, 415), is a clean failure: nothing attached, no describe check.
func TestVoucherCreateFileUpload4xxIsClean(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	for _, status := range []int{http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity} {
		app, srv := apiApp(t)
		srv.On("POST", "/expense/vouchers/290/documents", status, []byte(`{"type":"error","message":"abgelehnt"}`))
		wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 11)
		wantRequests(t, srv, 2)
		out, errs := bufs(app)
		if !strings.Contains(out, "Beleg 290 angelegt, Datei nicht hochgeladen: abgelehnt") || strings.Contains(out+errs, "möglicherweise") {
			t.Errorf("%d: stdout = %s\nstderr = %s", status, out, errs)
		}
	}
}

// An upload that fails without an HTTP status because the file shrank
// after the check was never completely sent: a clean failure.
func TestVoucherCreateFileUploadWithoutStatusIsClean(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, _, _ := newTestApp(t)
	srv := mockapi.New(t, mockapi.WithoutAutoFail()) // the cut upload body is no valid request
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-upload"})
	srv.OnFunc("POST", "/expense/vouchers", func(w http.ResponseWriter, _ *http.Request) {
		if err := os.Truncate(path, 3); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":290,"type":"expense_voucher"}`)
	})
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json")), 11)
	out, errs := bufs(app)
	if !strings.Contains(out, "hat sich beim Hochladen verändert") || strings.Contains(out+errs, "möglicherweise") {
		t.Errorf("stdout = %s\nstderr = %s", out, errs)
	}
}

func TestUploadUncertain(t *testing.T) {
	for _, tc := range []struct {
		err  *clierr.Error
		want bool
	}{
		{&clierr.Error{Code: clierr.CodeOutcomeUnknown}, true},
		{&clierr.Error{Code: clierr.CodeNetwork}, true},
		{&clierr.Error{Code: clierr.CodeServer, HTTPStatus: 502}, true},
		{&clierr.Error{Code: clierr.CodeError, HTTPStatus: 413}, false},
		{&clierr.Error{Code: clierr.CodeError, HTTPStatus: 415}, false},
		{&clierr.Error{Code: clierr.CodeValidation, HTTPStatus: 422}, false},
		{&clierr.Error{Code: clierr.CodeNotFound, HTTPStatus: 404}, false},
		{&clierr.Error{Code: clierr.CodeError}, false}, // file not readable
	} {
		if got := uploadUncertain(tc.err); got != tc.want {
			t.Errorf("uploadUncertain(%s, %d) = %v", tc.err.Code, tc.err.HTTPStatus, got)
		}
	}
}

// The retry command names the absolute path, so it works from any
// directory and never starts with "-".
func TestVoucherCreateFileRetryAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("-x.pdf", []byte(pdfContent), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	app, srv := apiApp(t)
	srv.On("POST", "/expense/vouchers/290/documents", http.StatusUnprocessableEntity, []byte(`{"type":"error","message":"abgelehnt"}`))
	wantExit(t, app, Run(app, append(append([]string{}, voucherCreateArgs...), "--file=-x.pdf", "-o", "json")), 11)
	out := mustOut(app)
	want := "papierkram voucher upload 290 " + shellQuote(filepath.Join(wd, "-x.pdf"))
	var res struct {
		RetryCommand string `json:"retry_command"`
	}
	// decode instead of matching the raw text: JSON escapes the
	// backslashes of a Windows path
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.RetryCommand != want || strings.Contains(out, " -x.pdf") {
		t.Errorf("stdout = %s\nerwartet %s", out, want)
	}
}

// A timeout names --timeout: large files on a slow line need more than
// the default; the outcome stays unknown (exit 12 for upload alone).
func TestVoucherUploadTimeoutHint(t *testing.T) {
	path := writeFile(t, "beleg.pdf", pdfContent)
	app, srv := apiApp(t)
	srv.OnFunc("POST", "/expense/vouchers/295/documents", func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	code := waitExit(t, func() int { return Run(app, []string{"voucher", "upload", "295", path, "--timeout", "300ms"}) })
	wantExit(t, app, code, 12)
	if _, errs := bufs(app); !strings.Contains(errs, "größere Dateien brauchen ggf. --timeout 120s") {
		t.Errorf("stderr = %s", errs)
	}
}

// The run of create --file costs both requests.
func TestCreateUploadRun(t *testing.T) {
	create, ok1 := registry.Lookup("voucher", "create")
	upload, ok2 := registry.Lookup("voucher", "upload")
	if !ok1 || !ok2 {
		t.Fatal("voucher create/upload nicht registriert")
	}
	run := (&createUpload{entry: upload}).run(create)
	want := create
	want.Credits = 60
	if !reflect.DeepEqual(run, want) {
		t.Errorf("run = %+v", run)
	}
}

// create --file asks no confirmation of its own: an upload class that
// needs one (or a different one than create) must fail the build loudly
// instead of being skipped.
func TestCreateFileUploadNeedsNoOwnConfirmation(t *testing.T) {
	create, _ := registry.Lookup("voucher", "create")
	upload, _ := registry.Lookup("voucher", "upload")
	if err := checkCreateFile(create, upload); err != nil {
		t.Fatalf("Registry: %v", err)
	}
	for _, class := range []registry.Class{registry.ClassDestructive, registry.ClassSend, registry.ClassReadOnly} {
		up := upload
		up.Class = class
		if checkCreateFile(create, up) == nil {
			t.Errorf("Upload der Klasse %s wird nicht abgelehnt", class)
		}
	}
	upload.Class = registry.ClassDestructive
	defer func() {
		if recover() == nil {
			t.Error("addCreateFile baut --file trotz bestätigungspflichtigem Upload")
		}
	}()
	app, _ := apiApp(t)
	app.addCreateFileWith(create, upload, &cobra.Command{RunE: func(*cobra.Command, []string) error { return nil }})
}
