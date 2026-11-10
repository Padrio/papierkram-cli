package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Rulings T5-A/B: a pdf tool writes only a new file inside --files-dir.
// Absolute paths, "..", /dev/fd/1, an existing file and a link out of the
// directory are usage errors without a request; the file stays unchanged
// and stdout carries only JSON-RPC.
func TestMCPPdfFileTargets(t *testing.T) {
	app, srv, procErr := mcpApp(t)
	onPDF(srv, binaryPDF)
	dir, outside := t.TempDir(), t.TempDir()
	existing := filepath.Join(dir, "alt.pdf")
	secret := filepath.Join(outside, "geheim.pdf")
	for _, p := range []string{existing, secret} {
		if err := os.WriteFile(p, []byte("alt"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bad := []string{"/dev/fd/1", secret, "../" + filepath.Base(outside) + "/neu.pdf", "alt.pdf", "-", ""}
	if err := os.Symlink(secret, filepath.Join(dir, "verweis.pdf")); err == nil {
		bad = append(bad, "verweis.pdf")
	}
	s := serveStdio(t, context.Background(), app, "mcp", "serve", "--files-dir", dir)
	for _, target := range bad {
		res := callTool(t, s.cs, "invoice_pdf", map[string]any{"id": 459, "output_file": target})
		if e := errorOf(t, res); e["code"] != "usage" || e["exit_code"] != float64(2) {
			t.Errorf("%q: Fehler %v", target, e)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d Anfragen, erwartet 0", n)
	}
	for _, p := range []string{existing, secret} {
		if got, _ := os.ReadFile(p); string(got) != "alt" {
			t.Errorf("%s verändert: %q", p, got)
		}
	}
	// a new file in the directory (also in a subdirectory) is written
	if err := os.Mkdir(filepath.Join(dir, "ablage"), 0o700); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, s.cs, "invoice_pdf", map[string]any{"id": 459, "output_file": "ablage/neu.pdf"})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	want := filepath.Join(dir, "ablage", "neu.pdf")
	if got, _ := os.ReadFile(want); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	if !strings.Contains(contentText(t, res, 0), `"path":`) || !strings.Contains(contentText(t, res, 0), `"bytes":`) {
		t.Errorf("Ergebnis %q", contentText(t, res, 0))
	}
	// the same name again: not replaced (--no-clobber is fixed)
	res = callTool(t, s.cs, "invoice_pdf", map[string]any{"id": 459, "output_file": "ablage/neu.pdf"})
	if e := errorOf(t, res); e["exit_code"] != float64(2) || !strings.Contains(e["message"].(string), "--no-clobber") {
		t.Errorf("Fehler %v", e)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("%d Anfragen, erwartet 1", n)
	}
	if code := s.close(t); code != 0 {
		t.Errorf("Exit %d: %s", code, procErr)
	}
	s.checkWire(t)
}

// --files-dir must be an existing directory; relative to the working
// directory; the default is the working directory.
func TestMCPFilesDir(t *testing.T) {
	app, _, _ := mcpApp(t)
	wd, _ := app.Getwd()
	file := filepath.Join(wd, "datei")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(wd, "fehlt"), file, "fehlt"} {
		out, errb := &bytes.Buffer{}, &bytes.Buffer{}
		app.Out, app.Err = out, errb
		if code := Run(app, []string{"mcp", "serve", "--files-dir", dir}); code != 2 || out.Len() != 0 ||
			!strings.Contains(errb.String(), "--files-dir") {
			t.Errorf("%s: Exit %d, stdout %q, stderr %q", dir, code, out, errb)
		}
	}
	if err := os.Mkdir(filepath.Join(wd, "ablage"), 0o700); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{"": "", "ablage": filepath.Join(wd, "ablage"), filepath.Join(wd, "ablage") + "/": filepath.Join(wd, "ablage")} {
		got, err := app.mcpFilesDir(in)
		if err != nil || got != want {
			t.Errorf("%q: %q, %v; erwartet %q", in, got, err, want)
		}
	}
	app.Getwd = func() (string, error) { return "", os.ErrPermission }
	if _, err := app.mcpFilesDir("ablage"); err == nil {
		t.Error("ohne Arbeitsverzeichnis kein Fehler")
	}
	if _, err := app.mcpFilesDir(wd); err != nil {
		t.Errorf("absoluter Ordner braucht kein Arbeitsverzeichnis: %v", err)
	}
}

// Ruling T5-E: no implicit files directory. Without --files-dir the tools
// with files are missing (hint on stderr); the root of a file system and
// the home directory (also through a link) are refused.
func TestMCPFilesDirRequired(t *testing.T) {
	app, _, procErr := mcpApp(t)
	s := serveStdio(t, context.Background(), app, "mcp", "serve")
	tools, err := s.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if strings.HasSuffix(tool.Name, "_pdf") {
			t.Errorf("%s ohne --files-dir", tool.Name)
		}
	}
	if len(tools.Tools) == 0 {
		t.Error("keine Tools")
	}
	s.close(t)
	if !strings.Contains(procErr.String(), "Hinweis: ohne --files-dir keine Tools mit Dateien") {
		t.Errorf("stderr %q", procErr)
	}

	// with --files-dir the pdf tools exist, without the hint
	app, _, procErr = mcpApp(t)
	s = serveStdio(t, context.Background(), app, "mcp", "serve", "--files-dir", t.TempDir())
	tools, err = s.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, tool := range tools.Tools {
		if strings.HasSuffix(tool.Name, "_pdf") {
			n++
		}
	}
	s.close(t)
	if n != 3 || strings.Contains(procErr.String(), "Hinweis") {
		t.Errorf("%d pdf-Tools, stderr %q", n, procErr)
	}

	home := t.TempDir()
	link := filepath.Join(t.TempDir(), "heim")
	dirs := []string{string(filepath.Separator), home}
	if os.Symlink(home, link) == nil {
		dirs = append(dirs, link)
	}
	if runtime.GOOS != "windows" {
		dirs = append(dirs, "/tmp/../")
	}
	for _, dir := range dirs {
		app, _, errb := mcpApp(t)
		env := app.Env
		app.Env = func(k string) string {
			if k == "HOME" || k == "USERPROFILE" {
				return home
			}
			return env(k)
		}
		out := app.Out.(*bytes.Buffer)
		if code := Run(app, []string{"mcp", "serve", "--files-dir", dir}); code != 2 || out.Len() != 0 ||
			!strings.Contains(errb.String(), "Unterordner") {
			t.Errorf("%s: Exit %d, stdout %q, stderr %q", dir, code, out, errb)
		}
	}
}

// M1: global flags that mean nothing for the server are usage errors at
// the start, like --dry-run; nothing is written to stdout.
func TestMCPServeStartFlags(t *testing.T) {
	for _, args := range [][]string{{"--yes"}, {"-y"}, {"--jq", ".x"}, {"--fields", "id"}, {"-o", "json"}, {"--output", "yaml"}, {"--dry-run"}} {
		app, out, errb := newTestApp(t)
		if code := Run(app, append([]string{"mcp", "serve"}, args...)); code != 2 || out.Len() != 0 || !strings.Contains(errb.String(), "mcp serve") &&
			!strings.Contains(errb.String(), "--dry-run") {
			t.Errorf("%v: Exit %d, stdout %q, stderr %q", args, code, out, errb)
		}
	}
}

// M1: --timeout of the start applies to every call.
func TestMCPServeTimeout(t *testing.T) {
	app, srv, _ := mcpApp(t)
	srv.OnFunc(http.MethodGet, "/income/invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	s := serveStdio(t, context.Background(), app, "mcp", "serve", "--timeout", "50ms")
	start := time.Now()
	e := errorOf(t, callTool(t, s.cs, "invoice_describe", map[string]any{"id": 5}))
	if e["code"] != "network" || time.Since(start) > 1500*time.Millisecond {
		t.Errorf("Fehler %v nach %v", e, time.Since(start))
	}
	s.close(t)
}

// M2: the context resolved at the start is fixed: a later context use
// (active_context in config.toml) changes nothing for the running server.
func TestMCPContextPinned(t *testing.T) {
	app, srv, _ := mcpApp(t)
	writeCfg(t, app, twoCtx) // active_context = "a"
	for _, c := range []string{"a", "b"} {
		if err := app.Keyring.Set(c, "test-token-"+c); err != nil {
			t.Fatal(err)
		}
	}
	env := app.Env
	app.Env = func(k string) string {
		if k == "PAPIERKRAM_TOKEN" {
			return ""
		}
		return env(k)
	}
	cs := mcpSession(t, app, MCPOptions{})
	writeCfg(t, app, strings.Replace(twoCtx, `active_context = "a"`, `active_context = "b"`, 1))
	if res := callTool(t, cs, "info", nil); res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "Bearer test-token-a" {
		t.Errorf("Anfragen %+v", reqs)
	}
}

// M3: a signal ends the server only after the running call; the call is
// not cancelled and its request completes, but the client gets no answer
// (the connection is closed).
func TestMCPServeSignalWaitsForCall(t *testing.T) {
	app, srv, _ := mcpApp(t)
	arrived, release := make(chan struct{}), make(chan struct{})
	cancelled := make(chan bool, 1)
	srv.OnFunc(http.MethodGet, "/income/invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-r.Context().Done():
			cancelled <- true
		case <-release:
			cancelled <- false
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":5}`))
		}
	})
	ctx, signal := context.WithCancel(context.Background())
	s := serveStdio(t, ctx, app, "mcp", "serve")
	done := make(chan error, 1)
	go func() {
		_, err := s.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 5}})
		done <- err
	}()
	<-arrived
	signal()
	select {
	case code := <-s.exit:
		t.Fatalf("Server endet mit Exit %d vor dem Ende des Aufrufs", code)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if c := <-cancelled; c {
		t.Error("laufende Anfrage durch das Signal abgebrochen")
	}
	select {
	case code := <-s.exit:
		if code != 0 {
			t.Errorf("Exit %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Server endet nicht nach dem Aufruf")
	}
	// the call ran to its end, but the server closed the connection on
	// the signal: its answer does not reach the client (SDK, measured)
	if err := <-done; err == nil {
		t.Error("Antwort nach dem Signal erwartet keine, kam aber an")
	}
	_ = s.cs.Close()
}

// M5 (review focus 2, end to end): values that look like flags reach the
// API as values: sort=--yes, the company named "--context=x" is resolved
// by name, and neither --yes nor --context takes effect.
func TestMCPFlagLikeValuesEndToEnd(t *testing.T) {
	app, srv, _ := mcpApp(t)
	srv.OnFunc(http.MethodGet, "/contact/companies", pagedList(t, 1, []map[string]any{company(77, "--context=x")}))
	cs := mcpSession(t, app, MCPOptions{})
	res := callTool(t, cs, "invoice_list", map[string]any{"sort": "--yes", "company": "--context=x"})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || reqs[0].Path != "/contact/companies" || reqs[1].Path != "/income/invoices" {
		t.Fatalf("Anfragen %+v", reqs)
	}
	q := reqs[1].Query
	if q.Get("order_by") != "--yes" || q.Get("company_id") != "77" {
		t.Errorf("Query %s", q.Encode())
	}
	if len(res.Content) < 2 || !strings.Contains(contentText(t, res, 1), "--context=x") {
		t.Errorf("Hinweis zur Auflösung fehlt: %+v", res.Content)
	}
}
