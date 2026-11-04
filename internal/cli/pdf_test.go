package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// invoice|estimate|voucher pdf <id> [-O datei|-] (spec 9.2).

// binaryPDF has the bytes a text path would break: NUL, CRLF, ESC, invalid
// UTF-8.
const binaryPDF = "%PDF-1.4\x00\r\n\x1b[31m\xff\xfe binär\r\n\x00%%EOF\n"

// pdfResources are the resources with pdf and the path of their download.
var pdfResources = []struct{ resource, path string }{
	{"invoice", "/income/invoices/{id}/pdf"},
	{"estimate", "/income/estimates/{id}/pdf"},
	{"voucher", "/expense/vouchers/{id}/pdf"},
}

// onPDF answers the download of every resource with body as application/pdf.
func onPDF(srv interface {
	On(method, path string, status int, body []byte, hdr ...string)
}, body string) {
	for _, r := range pdfResources {
		srv.On("GET", r.path, http.StatusOK, []byte(body), "Content-Type", "application/pdf")
	}
}

func stdoutBytes(app *App) []byte { return app.Out.(*bytes.Buffer).Bytes() }

func TestR29_PdfRefusesTTY(t *testing.T) {
	for _, r := range pdfResources {
		app, srv := apiApp(t)
		app.StdoutTTY = true
		wantExit(t, app, Run(app, []string{r.resource, "pdf", "7"}), 2)
		wantRequests(t, srv, 0)
		out, errs := bufs(app)
		if out != "" || !strings.Contains(errs, "-O datei.pdf") || !strings.Contains(errs, "-O -") {
			t.Errorf("%s: stdout = %q, stderr = %q", r.resource, out, errs)
		}
	}
	// with -o json the same: nothing goes to the terminal, no request
	app, srv := apiApp(t)
	app.StdoutTTY = true
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "7", "-o", "json"}), 2)
	wantRequests(t, srv, 0)
}

func TestPdfWritesFileExactly(t *testing.T) {
	for _, r := range pdfResources {
		app, srv := apiApp(t)
		onPDF(srv, binaryPDF)
		path := filepath.Join(t.TempDir(), "out.pdf")
		wantExit(t, app, Run(app, []string{r.resource, "pdf", "459", "-O", path}), 0)
		reqs := wantRequests(t, srv, 1)
		if want := strings.Replace(r.path, "{id}", "459", 1); reqs[0].Method != "GET" || reqs[0].Path != want {
			t.Errorf("%s: request = %s %s", r.resource, reqs[0].Method, reqs[0].Path)
		}
		if a := reqs[0].Header.Get("Accept"); !strings.HasPrefix(a, "application/pdf") {
			t.Errorf("%s: Accept = %q", r.resource, a)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != binaryPDF {
			t.Errorf("%s: Datei = %q, %v", r.resource, got, err)
		}
		out, errs := bufs(app)
		if want := fmt.Sprintf("PDF gespeichert: %s (%d Bytes)\n", path, len(binaryPDF)); out != "" || errs != want {
			t.Errorf("%s: stdout = %q, stderr = %q", r.resource, out, errs)
		}
		assertNoToken(t, app)
	}

	// -o json: the message is the output on stdout
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	path := filepath.Join(t.TempDir(), "rechnung.pdf")
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "--output-file", path, "-o", "json"}), 0)
	want := fmt.Sprintf("{\n  \"path\": %q,\n  \"bytes\": %d\n}\n", path, len(binaryPDF))
	if out, errs := bufs(app); out != want || errs != "" {
		t.Errorf("json: stdout = %q, stderr = %q", out, errs)
	}
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("json: Datei = %q", got)
	}
}

func TestPdfStdoutKeepsBinaryBytes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"-O -", []string{"-O", "-"}, fmt.Sprintf("PDF nach stdout geschrieben (%d Bytes)\n", len(binaryPDF))},
		{"ohne -O, kein Terminal", nil, fmt.Sprintf("PDF nach stdout geschrieben (%d Bytes)\n", len(binaryPDF))},
		// stdout carries the PDF: the message stays on stderr, also as JSON
		{"-O - -o json", []string{"-O", "-", "-o", "json"}, fmt.Sprintf("{\n  \"path\": \"-\",\n  \"bytes\": %d\n}\n", len(binaryPDF))},
		{"-o yaml", []string{"-o", "yaml"}, fmt.Sprintf("path: \"-\"\nbytes: %d\n", len(binaryPDF))},
	} {
		app, srv := apiApp(t)
		onPDF(srv, binaryPDF)
		wantExit(t, app, Run(app, append([]string{"voucher", "pdf", "295"}, tc.args...)), 0)
		wantRequests(t, srv, 1)
		if got := stdoutBytes(app); !bytes.Equal(got, []byte(binaryPDF)) {
			t.Errorf("%s: stdout = %q, erwartet %q", tc.name, got, binaryPDF)
		}
		if _, errs := bufs(app); errs != tc.wantErr {
			t.Errorf("%s: stderr = %q, erwartet %q", tc.name, errs, tc.wantErr)
		}
	}

	// -O - is explicit: allowed on a terminal too
	app, srv := apiApp(t)
	app.StdoutTTY = true
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"estimate", "pdf", "349", "-O", "-"}), 0)
	if got := stdoutBytes(app); !bytes.Equal(got, []byte(binaryPDF)) {
		t.Errorf("TTY -O -: stdout = %q", got)
	}
}

func TestPdfOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alte Fassung, deutlich länger als die neue"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path}), 0)
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "rechnung.pdf")
	if runtime.GOOS == "windows" {
		return
	}
	// an existing file keeps its mode (new files: umask, pdf_unix_test.go)
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("Modus vorhandene Datei = %v", fi.Mode().Perm())
	}
}

// Taking over the mode of the replaced file is best effort: when it fails,
// the paid download is still saved, with a warning.
func TestPdfChmodFailureKeepsDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := chmodFile
	chmodFile = func(string, fs.FileMode) error { return errors.New("Operation nicht erlaubt") }
	t.Cleanup(func() { chmodFile = orig })
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path}), 0)
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "rechnung.pdf")
	if _, errs := bufs(app); !strings.Contains(errs, "Warnung: ") || !strings.Contains(errs, "Operation nicht erlaubt") {
		t.Errorf("stderr = %q", errs)
	}
}

// A cancelled download (Ctrl+C) leaves the directory as it was: the
// temporary file is removed, an existing file is untouched.
func TestPdfCancelRemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, srv := apiApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.OnFunc("GET", "/income/invoices/{id}/pdf", func(w http.ResponseWriter, r *http.Request) {
		cancel() // the interrupt arrives while the download runs
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	wantExit(t, app, executeContext(ctx, app, NewRootCmd(app), []string{"invoice", "pdf", "459", "-O", path}), 10)
	wantRequests(t, srv, 1) // no retry after the interrupt
	if got, _ := os.ReadFile(path); string(got) != "alt" {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "rechnung.pdf")
	if _, errs := bufs(app); !strings.Contains(errs, "Abgebrochen") {
		t.Errorf("stderr = %q", errs)
	}
}

// A symbolic link as target is followed: the link stays, its target gets
// the PDF. A link to nowhere is a usage error before the request.
func TestPdfFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "ablage", "rechnung.pdf")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "aktuell.pdf")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("keine symbolischen Verweise: %v", err)
	}
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", link}), 0)
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Verweis ersetzt: %v, %v", fi, err)
	}
	if got, _ := os.ReadFile(real); string(got) != binaryPDF {
		t.Errorf("Ziel = %q", got)
	}
	wantDirFiles(t, filepath.Dir(real), "rechnung.pdf")
	if _, errs := bufs(app); !strings.Contains(errs, "PDF gespeichert: "+link+" ") {
		t.Errorf("stderr = %q", errs)
	}

	dangling := filepath.Join(dir, "weg.pdf")
	if err := os.Symlink(filepath.Join(dir, "fehlt", "x.pdf"), dangling); err != nil {
		t.Fatal(err)
	}
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", dangling}), 2)
	wantRequests(t, srv, 0)
}

// A write-protected file is not replaced: exit 1 before the request, on
// every platform (Windows refuses to replace it anyway).
func TestPdfReadOnlyFileNotReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path}), 1)
	wantRequests(t, srv, 0)
	if got, _ := os.ReadFile(path); string(got) != "alt" {
		t.Errorf("Datei = %q", got)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "schreibgeschützt") {
		t.Errorf("stderr = %q", errs)
	}
	wantDirFiles(t, dir, "rechnung.pdf")
}

// A failed download leaves an existing file untouched and no partial or
// temporary file behind.
func TestPdfFailedDownloadKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices/{id}/pdf", http.StatusNotFound, []byte(`{"type":"error","message":"Rechnung nicht gefunden"}`))
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "9", "-O", path}), 4)
	if got, _ := os.ReadFile(path); string(got) != "alt" {
		t.Errorf("Datei = %q", got)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Rechnung nicht gefunden") {
		t.Errorf("stderr = %q", errs)
	}
	wantDirFiles(t, dir, "rechnung.pdf")

	// new file: nothing is created
	app, srv = apiApp(t)
	srv.On("GET", "/income/invoices/{id}/pdf", http.StatusBadGateway, []byte("<html>Bad Gateway</html>"), "Content-Type", "text/html")
	neu := filepath.Join(dir, "neu.pdf")
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "9", "-O", neu}), 1)
	wantDirFiles(t, dir, "rechnung.pdf")
}

// wantDirFiles checks that dir holds exactly the given files.
func wantDirFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Errorf("Dateien in %s: %v, erwartet %v", dir, got, names)
	}
}

func TestPdfMissingDirectoryIsUsage(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "datei")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(dir, "fehlt", "rechnung.pdf"),
		filepath.Join(file, "rechnung.pdf"), // parent is a file
		dir,                                 // the target is a directory
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path}), 2)
		wantRequests(t, srv, 0)
	}
	// -O without a value
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", ""}), 2)
	wantRequests(t, srv, 0)
}

// A directory that cannot be written is exit 1 and costs no request.
func TestPdfUnwritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("Verzeichnisrechte nicht durchsetzbar")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", filepath.Join(dir, "r.pdf")}), 1)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "nicht gespeichert") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestPdfFieldsAndJQ(t *testing.T) {
	// PDF on stdout: --fields and --jq have nothing to work on
	for _, args := range [][]string{
		{"-O", "-", "--fields", "path"},
		{"--jq", ".bytes"},
		{"-O", "-", "-o", "json", "--jq", ".bytes"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append([]string{"invoice", "pdf", "459"}, args...)), 2)
		wantRequests(t, srv, 0)
	}

	// file: they apply to the message on stdout
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	path := filepath.Join(t.TempDir(), "r.pdf")
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path, "--jq", ".bytes"}), 0)
	if out, errs := bufs(app); out != fmt.Sprintf("%d\n", len(binaryPDF)) || errs != "" {
		t.Errorf("--jq: stdout = %q, stderr = %q", out, errs)
	}
	app, srv = apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path, "-o", "json", "--fields", "path"}), 0)
	if out := mustOut(app); out != fmt.Sprintf("{\n  \"path\": %q\n}\n", path) {
		t.Errorf("--fields: stdout = %q", out)
	}
}

func TestPdfDryRunIsUsage(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pdf", "295", "-O", "-", "--dry-run"}), 2)
	wantRequests(t, srv, 0)
}

// pdf only reads: it works under the hard read-only mode.
func TestPdfAllowedReadOnly(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-builder", "PAPIERKRAM_READ_ONLY": "1"})
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", "-"}), 0)
	wantRequests(t, srv, 1)
}

// An empty 2xx body is no error: a 0-byte file and a warning.
func TestPdfEmptyBodyWritesEmptyFile(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices/{id}/pdf", http.StatusOK, nil)
	path := filepath.Join(t.TempDir(), "leer.pdf")
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path, "-o", "json"}), 0)
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Errorf("Datei: %v, %v", fi, err)
	}
	out, errs := bufs(app)
	if out != fmt.Sprintf("{\n  \"path\": %q,\n  \"bytes\": 0\n}\n", path) || !strings.Contains(errs, "Warnung: ") || !strings.Contains(errs, "leer") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
}

// A JSON object with status 200 is no PDF: no file, exit 1.
func TestPdfJSONAnswerIsNoPDF(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices/{id}/pdf", http.StatusOK, []byte(`{"id":459}`))
	dir := t.TempDir()
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", filepath.Join(dir, "r.pdf")}), 1)
	wantDirFiles(t, dir)
	if _, errs := bufs(app); !strings.Contains(errs, "kein PDF") || !strings.Contains(errs, "application/json") {
		t.Errorf("stderr = %q", errs)
	}
}

// --debug shows the PDF only as its size (spec 5.5).
func TestPdfDebugShowsOnlySize(t *testing.T) {
	// the PDF type, an unknown binary type and none at all: only the size
	for _, ct := range []string{"application/pdf", "binary/octet-stream", ""} {
		app, srv := apiApp(t)
		srv.OnFunc("GET", "/income/invoices/{id}/pdf", func(w http.ResponseWriter, _ *http.Request) {
			w.Header()["Content-Type"] = nil // nil: net/http does not sniff a type
			if ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			_, _ = w.Write([]byte(binaryPDF))
		})
		path := filepath.Join(t.TempDir(), "r.pdf")
		wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path, "--debug"}), 0)
		_, errs := bufs(app)
		if !strings.Contains(errs, fmt.Sprintf("< <binär, %d Bytes>", len(binaryPDF))) || strings.Contains(errs, "%PDF") || strings.Contains(errs, "\x1b") {
			t.Errorf("%q: stderr = %q", ct, errs)
		}
		if got, _ := os.ReadFile(path); string(got) != binaryPDF {
			t.Errorf("%q: Datei = %q", ct, got)
		}
		assertNoToken(t, app)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("Pipe geschlossen") }

// A failed write to stdout after the download is exit 1, never 2.
func TestPdfStdoutWriteError(t *testing.T) {
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	app.Out = failWriter{}
	errb := app.Err.(*bytes.Buffer)
	if code := Run(app, []string{"invoice", "pdf", "459", "-O", "-"}); code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(errb.String(), "Pipe geschlossen") {
		t.Errorf("stderr = %q", errb)
	}
}
