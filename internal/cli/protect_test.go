package cli

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// invoiceActions are the invoice entries of the protection tests, built
// from the spec operations (the registry gets them in Phase 7).
func invoiceActions(t *testing.T) []registry.Entry {
	t.Helper()
	return append(writeEntries(t),
		specEntry(t, "ArchiveInvoice", "invoice", "archive", registry.ClassArchive),
		specEntry(t, "DeleteInvoice", "invoice", "delete", registry.ClassDestructive),
		specEntry(t, "CancelInvoice", "invoice", "cancel", registry.ClassDestructive),
		specEntry(t, "ListInvoices", "invoice", "list", registry.ClassReadOnly),
		specEntry(t, "GetInvoice", "invoice", "describe", registry.ClassReadOnly),
	)
}

func runProtect(t *testing.T, app *App, args ...string) int {
	t.Helper()
	return runEntries(t, app, nil, args, invoiceActions(t)...)
}

// invoice12 answers the summary GET of invoice 12.
func invoice12(srv *mockapi.Server) {
	srv.On("GET", "/income/invoices/12", http.StatusOK, []byte(`{"id":12,"name":"Beratung Oktober","total_gross":1190.00}`))
}

func TestR25_ArchiveInvoiceNoConfirmation(t *testing.T) {
	app, srv := apiApp(t) // no TTY, no --yes
	srv.On("POST", "/income/invoices/{id}/archive", http.StatusOK, nil)
	wantExit(t, app, runProtect(t, app, "invoice", "archive", "12"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/income/invoices/12/archive" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if _, errs := bufs(app); strings.Contains(errs, "Bestätigung") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestR35_ReadOnlyEnvOverridesWritableContext(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("a", "test-token-a"); err != nil {
		t.Fatal(err)
	}
	in := &countingReader{r: strings.NewReader(`{"name":"Neu"}`)}
	app.In = in
	wantExit(t, app, runProtect(t, app, "invoice", "create", "--input", "-", "--context", "a"), 9)
	wantRequests(t, srv, 0)
	if in.n != 0 {
		t.Errorf("Body gelesen (%d Reads), obwohl der Schreibschutz greift", in.n)
	}
	_, errs := bufs(app)
	for _, want := range []string{
		"Fehler: Schreibschutz aktiv (PAPIERKRAM_READ_ONLY) – Schreibzugriffe sind gesperrt",
		"Hinweis: Die Variable setzt, wer diesen Prozess startet; sie lässt sich nicht aufheben.",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q: %q", want, errs)
		}
	}
}

// The env lock answers before the token is checked: without any token the
// write is still exit 9, not exit 3.
func TestReadOnlyEnvBeforeTokenCheck(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "true"})
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12", "--yes"), 9)
	wantRequests(t, srv, 0)
}

// Every class but read_only is locked, reading stays allowed.
func TestReadOnlyEnvLocksAllMutationClasses(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "archive", "12"},
		{"invoice", "delete", "12", "--yes"},
		{"invoice", "cancel", "12", "--yes"},
		{"invoice", "update", "12", "--name", "N"},
		{"project", "create", "--name", "P", "--customer", "1"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, runProtect(t, app, args...), 9)
		wantRequests(t, srv, 0)
	}
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, runProtect(t, app, "invoice", "describe", "12", "-o", "json"), 0)
	wantRequests(t, srv, 1)
}

const roActive = `active_context = "b"

[contexts.a]
subdomain = "firma-a"

[contexts.b]
subdomain = "firma-b"
read_only = true
`

func TestReadOnlyContextAllowsSwitchingContext(t *testing.T) {
	setup := func() (*App, *mockapi.Server) {
		app, srv := mockApp(t, nil)
		writeCfg(t, app, roActive)
		for _, ctx := range []string{"a", "b"} {
			if err := app.Keyring.Set(ctx, "test-token-"+ctx); err != nil {
				t.Fatal(err)
			}
		}
		return app, srv
	}

	// active context b is read-only
	app, srv := setup()
	wantExit(t, app, runProtect(t, app, "project", "update", "5", "--name", "N"), 9)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	for _, want := range []string{`Fehler: Kontext "b" ist schreibgeschützt (read_only = true)`, "Hinweis: anderen Kontext mit --context wählen"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q: %q", want, errs)
		}
	}

	// --context a: allowed
	app, srv = setup()
	wantExit(t, app, runProtect(t, app, "project", "update", "5", "--name", "N", "--context", "a", "-o", "json"), 0)
	wantBody(t, srv, "PUT", "/projects/5", `{"name":"N"}`)
}

func TestDeleteWithoutTTYNeedsYes(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12"), 8)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	if !strings.Contains(errs, "Fehler: Bestätigung erforderlich") || !strings.Contains(errs, "Hinweis: --yes verwenden") {
		t.Errorf("stderr = %q", errs)
	}

	// --yes: sent without a summary request
	app, srv = apiApp(t)
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12", "--yes"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/income/invoices/12" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
}

func TestNoInputWithTTYNeedsYes(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("12\n")
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12", "--no-input"), 8)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); strings.Contains(errs, "eingeben") || !strings.Contains(errs, "--yes verwenden") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestDeleteConfirmedByTypingID(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("12\n")
	invoice12(srv)
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12", "-o", "json"), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "GET" || reqs[0].Path != "/income/invoices/12" || reqs[1].Method != "DELETE" || reqs[1].Path != "/income/invoices/12" {
		t.Errorf("requests = %s %s, %s %s", reqs[0].Method, reqs[0].Path, reqs[1].Method, reqs[1].Path)
	}
	out, errs := bufs(app)
	if want := "Rechnung 12 „Beratung Oktober“ (1.190,00 €) löschen? Zur Bestätigung ID eingeben: "; !strings.Contains(errs, want) {
		t.Errorf("stderr = %q, erwartet %q", errs, want)
	}
	if out != "{\n  \"id\": 12,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestDeleteWrongIDCancels(t *testing.T) {
	for _, answer := range []string{"13\n", "ja\n", "\n", "", " 12x\n"} {
		app, srv := apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		invoice12(srv)
		wantExit(t, app, runProtect(t, app, "invoice", "delete", "12"), 8)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
			t.Errorf("%q: request = %s %s", answer, r.Method, r.Path)
		}
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, "Fehler: Abgebrochen durch Nutzer") {
			t.Errorf("%q: stdout = %q, stderr = %q", answer, out, errs)
		}
	}
}

func TestCancelConfirmedWithJa(t *testing.T) {
	for _, answer := range []string{"ja\n", "JA\r\n", "yes\n", "j\n", "Y\n"} {
		app, srv := apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		invoice12(srv)
		srv.On("POST", "/income/invoices/{id}/cancel", http.StatusOK, nil)
		wantExit(t, app, runProtect(t, app, "invoice", "cancel", "12"), 0)
		reqs := wantRequests(t, srv, 2)
		if reqs[1].Method != "POST" || reqs[1].Path != "/income/invoices/12/cancel" {
			t.Errorf("%q: request = %s %s", answer, reqs[1].Method, reqs[1].Path)
		}
		if _, errs := bufs(app); !strings.Contains(errs, "Rechnung 12 „Beratung Oktober“ (1.190,00 €) wirklich stornieren? [ja/nein]: ") {
			t.Errorf("%q: stderr = %q", answer, errs)
		}
	}
	for _, answer := range []string{"nein\n", "n\n", "\n", "12\n"} {
		app, srv := apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		invoice12(srv)
		wantExit(t, app, runProtect(t, app, "invoice", "cancel", "12"), 8)
		wantRequests(t, srv, 1)
	}
}

// The summary request uses all path parameters; persons are named by first
// and last name, objects without an amount show none.
func TestConfirmSummaryNestedObject(t *testing.T) {
	del := specEntry(t, "DeletePerson", "person", "delete", registry.ClassDestructive)
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("5\n")
	srv.On("GET", "/contact/companies/7/persons/5", http.StatusOK, []byte(`{"id":5,"first_name":"Anna","last_name":"Groß"}`))
	wantExit(t, app, runEntries(t, app, nil, []string{"person", "delete", "5", "--company", "7"}, del), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Path != "/contact/companies/7/persons/5" || reqs[1].Method != "DELETE" {
		t.Errorf("requests = %+v", reqs)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Kontaktperson 5 „Anna Groß“ löschen? Zur Bestätigung ID eingeben: ") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestGermanAmount(t *testing.T) {
	for in, want := range map[string]string{
		"1190.00": "1.190,00 €", "226.0": "226,00 €", "5": "5,00 €", "-1234567.891": "-1.234.567,891 €",
		"999": "999,00 €", "1000": "1.000,00 €", "abc": "abc",
	} {
		if got := germanAmount(in); got != want {
			t.Errorf("germanAmount(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// A failing summary request does not abort: the prompt shows what is known.
func TestConfirmSummaryFetchFails(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("12\n")
	srv.On("GET", "/income/invoices/12", http.StatusInternalServerError, []byte(`{"message":"kaputt"}`))
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12"), 0)
	reqs := srv.Requests()
	if last := reqs[len(reqs)-1]; last.Method != "DELETE" {
		t.Errorf("letzter Request = %s %s", last.Method, last.Path)
	}
	_, errs := bufs(app)
	for _, want := range []string{"Warnung: ", "Rechnung 12 löschen? Zur Bestätigung ID eingeben: "} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q: %q", want, errs)
		}
	}
}

func TestConfirmPromptInterrupted(t *testing.T) {
	app, srv := apiApp(t)
	invoice12(srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.StdinTTY = true
	app.In = newBlockingReader(t, cancel)
	code := waitExit(t, func() int {
		return runEntries(t, app, ctx, []string{"invoice", "delete", "12"}, invoiceActions(t)...)
	})
	wantExit(t, app, code, 10)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Fehler: Abgebrochen: Eingabe durch Unterbrechung beendet") {
		t.Errorf("stderr = %q", errs)
	}
}

// Prompts share one buffered reader: a second prompt on piped stdin gets
// the second line instead of losing it to the first prompt's buffer.
func TestPromptsShareStdinReader(t *testing.T) {
	app, _ := apiApp(t)
	app.In = strings.NewReader("ja\n12\nRest")
	for _, want := range []string{"ja", "12", "Rest", ""} {
		got, err := app.prompt(context.Background(), "? ")
		if err != nil || got != want {
			t.Fatalf("prompt = %q, %v; erwartet %q", got, err, want)
		}
	}
}

func TestDryRunJSON(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runProtect(t, app, "project", "create", "--name", "P", "--customer", "1", "--dry-run", "-o", "json"), 0)
	wantRequests(t, srv, 0)
	want := "{\n  \"dry_run\": true,\n  \"method\": \"POST\",\n  \"url\": \"" + srv.URL() + "/projects\",\n" +
		"  \"body\": {\n    \"customer\": {\n      \"id\": 1\n    },\n    \"name\": \"P\"\n  },\n  \"credits\": 10\n}\n"
	if out, errs := bufs(app); out != want || errs != "" {
		t.Errorf("stdout = %q\nerwartet %q\nstderr = %q", out, want, errs)
	}

	// credits come from the entry; actions without a body show null; no
	// confirmation is needed for destructive classes
	del := specEntry(t, "DeleteInvoice", "invoice", "delete", registry.ClassDestructive)
	del.Credits = 7
	app, srv = apiApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "delete", "12", "--dry-run", "-o", "json"}, del), 0)
	wantRequests(t, srv, 0)
	want = "{\n  \"dry_run\": true,\n  \"method\": \"DELETE\",\n  \"url\": \"" + srv.URL() + "/income/invoices/12\",\n  \"body\": null,\n  \"credits\": 7\n}\n"
	if out := mustOut(app); out != want {
		t.Errorf("stdout = %q\nerwartet %q", out, want)
	}

	// --jq works on the dry-run object
	app, srv = apiApp(t)
	wantExit(t, app, runProtect(t, app, "invoice", "archive", "12", "--dry-run", "--jq", ".credits"), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); out != "1\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestDryRunText(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runProtect(t, app, "project", "create", "--name", "P", "--customer", "1", "--dry-run"), 0)
	wantRequests(t, srv, 0)
	want := "Probelauf – nichts gesendet\n" +
		"Methode: POST\n" +
		"URL:     " + srv.URL() + "/projects\n" +
		"Credits: 10\n" +
		"Body:\n{\n  \"customer\": {\n    \"id\": 1\n  },\n  \"name\": \"P\"\n}\n"
	if out := mustOut(app); out != want {
		t.Errorf("stdout = %q\nerwartet %q", out, want)
	}

	app, srv = apiApp(t)
	wantExit(t, app, runProtect(t, app, "invoice", "delete", "12", "--dry-run"), 0)
	wantRequests(t, srv, 0)
	want = "Probelauf – nichts gesendet\nMethode: DELETE\nURL:     " + srv.URL() + "/income/invoices/12\nCredits: 1\nBody:    –\n"
	if out := mustOut(app); out != want {
		t.Errorf("stdout = %q\nerwartet %q", out, want)
	}
}

// --dry-run validates locally: an invalid body is exit 5 without request.
func TestDryRunValidates(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runProtect(t, app, "project", "create", "--name", "P", "--dry-run"), 5)
	wantRequests(t, srv, 0)
}

func TestDryRunBlockedUnderReadOnly(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, runProtect(t, app, "project", "create", "--name", "P", "--customer", "1", "--dry-run", "-o", "json"), 9)
	wantRequests(t, srv, 0)
	if out := mustOut(app); out != "" {
		t.Errorf("stdout = %q", out)
	}
}

func TestDryRunOnReadCommandIsUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "list", "--dry-run"},
		{"invoice", "describe", "12", "--dry-run"},
		{"user", "list", "--dry-run"},
		{"info", "--dry-run"},
		{"context", "use", "b", "--dry-run"},
	} {
		app, srv := apiApp(t)
		writeCfg(t, app, twoCtx)
		wantExit(t, app, runProtect(t, app, args...), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--dry-run") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
		if b, err := os.ReadFile(cfgFile(t, app)); err != nil || string(b) != twoCtx {
			t.Errorf("%v: config geändert: %s", args, b)
		}
	}
}
