package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// contractDir holds the checked-in JSON contract schemas (spec 10.3, 10.4,
// 11), relative to the package directory.
const contractDir = "../../docs/schemas"

// contractSchema loads docs/schemas/<name>.json as an OpenAPI 3 Schema
// Object and checks that it is a valid one. Go tests and the testscript
// command jsonschema share it.
func contractSchema(name string) (*openapi3.Schema, error) {
	path := filepath.Join(contractDir, name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s openapi3.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("Vertrag %s ist kein Schema-Objekt: %w", path, err)
	}
	if err := s.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("Vertrag %s ist kein gültiges OpenAPI-3-Schema: %w", path, err)
	}
	return &s, nil
}

// loadContract is contractSchema for a Go test.
func loadContract(t testing.TB, name string) *openapi3.Schema {
	t.Helper()
	s, err := contractSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// checkContract validates the JSON document raw against the contract name.
func checkContract(name string, raw []byte) error {
	s, err := contractSchema(name)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("kein JSON: %w\n%s", err, raw)
	}
	if err := s.VisitJSON(v); err != nil {
		return fmt.Errorf("verletzt %s.json: %w\n%s", name, err, raw)
	}
	return nil
}

// wantContract fails t unless raw satisfies the contract name.
func wantContract(t *testing.T, name string, raw []byte) {
	t.Helper()
	if err := checkContract(name, raw); err != nil {
		t.Fatal(err)
	}
}

// lastLine is the last non-empty line of s: the structured error, which
// follows the text lines on stderr (spec 10.4).
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// contractRun is one run of the CLI for a contract test: setup prepares
// the mock (and may return extra env values), args are the arguments.
type contractRun struct {
	name  string
	exit  int
	env   map[string]string
	setup func(t *testing.T, srv *mockapi.Server) []string
	// prepare writes configuration, keychain or project file of the app
	prepare func(t *testing.T, app *App)
	args    []string
}

// run executes c and returns stdout and stderr; the exit code must match.
func (c contractRun) run(t *testing.T) (string, string) {
	t.Helper()
	env := map[string]string{"PAPIERKRAM_TOKEN": "test-token-contract"}
	for k, v := range c.env {
		env[k] = v
	}
	app, srv := mockApp(t, env)
	app.Now = func() time.Time { return contractNow }
	if c.prepare != nil {
		c.prepare(t, app)
	}
	args := c.args
	if c.setup != nil {
		args = append(append([]string{}, args...), c.setup(t, srv)...)
	}
	code := waitExit(t, func() int { return Run(app, args) })
	wantExit(t, app, code, c.exit)
	assertNoToken(t, app)
	return bufs(app)
}

// contractNow is the fixed time of the contract runs.
var contractNow = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

// apiError is an error body of the API (spec 4.1).
func apiError(msg string) []byte {
	return fmt.Appendf(nil, `{"type":"error","message":%q,"correlation_id":"corr-42"}`, msg)
}

// closeAfterRead answers a request by closing the connection after it was
// read completely: the request was sent, the outcome is unknown.
func closeAfterRead(w http.ResponseWriter, _ *http.Request) {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

// errorRuns produce every exit code the CLI has (1–12), each with -o json.
var errorRuns = []contractRun{
	{name: "error", exit: 1, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusTeapot, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "server_error", exit: 1, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusInternalServerError, apiError("kaputt"))
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	// Go accepts every three-digit status; outside 2xx it is an error
	// (proxies and gateways in front of the API send odd ones)
	{name: "error 101", exit: 1, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusSwitchingProtocols, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "server_error 600", exit: 1, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", 600, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "server_error 999", exit: 1, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", 999, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "usage", exit: 2, args: []string{"invoice", "describe", "-o", "json"}},
	{name: "auth", exit: 3, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusUnauthorized, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "auth ohne Token", exit: 3, env: map[string]string{"PAPIERKRAM_TOKEN": ""},
		args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "not_found", exit: 4, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusNotFound, apiError("Rechnung nicht gefunden"))
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "validation", exit: 5, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/contact/companies", http.StatusUnprocessableEntity, apiError("Name ist bereits vergeben"))
		return nil
	}, args: []string{"company", "create", "--name", "Muster GmbH", "--contact-type", "customer", "-o", "json"}},
	{name: "validation lokal", exit: 5, args: []string{"company", "create", "--name", "Muster GmbH", "-o", "json"}},
	{name: "quota", exit: 6, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/income/invoices/1", http.StatusTooManyRequests, nil)
		return nil
	}, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "conflict", exit: 7, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/contact/companies/12/archive", http.StatusConflict, nil)
		return nil
	}, args: []string{"company", "archive", "12", "-o", "json"}},
	{name: "confirmation_required", exit: 8, args: []string{"company", "delete", "12", "-o", "json"}},
	{name: "read_only", exit: 9, env: map[string]string{"PAPIERKRAM_READ_ONLY": "1"},
		args: []string{"company", "create", "--name", "Muster GmbH", "--contact-type", "customer", "-o", "json"}},
	// the base URL of a closed server: see networkRun
	{name: "network", exit: 10, args: []string{"invoice", "describe", "1", "-o", "json"}},
	{name: "partial", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/expense/vouchers/{voucher_id}/documents", http.StatusUnprocessableEntity, apiError("Dateityp nicht erlaubt"))
		return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
	}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
	{name: "outcome_unknown", exit: 12, setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("POST", "/contact/companies", closeAfterRead)
		return nil
	}, args: []string{"company", "create", "--name", "Muster GmbH", "--contact-type", "customer", "-o", "json"}},
	// a 2xx answer longer than 64 MiB (ruling FR-D): exit 1 without
	// http_status, for a read and a mutation
	{name: "zu groß Lesen", exit: 1, setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/income/invoices/{id}", tooLarge(http.StatusOK, "application/json"))
		return nil
	}, args: []string{"invoice", "describe", "459", "-o", "json"}},
	{name: "zu groß Änderung", exit: 1, setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("PUT", "/income/invoices/{id}", tooLarge(http.StatusOK, "application/json"))
		return nil
	}, args: []string{"invoice", "update", "459", "--payment-term", "1130", "--name", "x", "-o", "json"}},
}

// Every exit code 1–12 ends with a structured error on stderr that
// satisfies docs/schemas/error.json (spec 10.4), and its exit_code is the
// exit code of the process.
func TestErrorContract(t *testing.T) {
	seen := map[int]bool{}
	for _, c := range errorRuns {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "network" {
				c = networkRun(t, c)
			}
			_, errs := c.run(t)
			line := lastLine(errs)
			wantContract(t, "error", []byte(line))
			var doc struct {
				Error struct {
					ExitCode int `json:"exit_code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(line), &doc); err != nil || doc.Error.ExitCode != c.exit {
				t.Errorf("exit_code = %d (%v), erwartet %d", doc.Error.ExitCode, err, c.exit)
			}
			seen[c.exit] = true
		})
	}
	for exit := 1; exit <= 12; exit++ {
		if !seen[exit] {
			t.Errorf("kein Fall für Exit %d", exit)
		}
	}
}

// networkRun points the run at a closed server (connection refused).
func networkRun(t *testing.T, c contractRun) contractRun {
	t.Helper()
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL + "/api/v1"
	closed.Close()
	c.setup = nil
	c.env = map[string]string{"PAPIERKRAM_BASE_URL": url}
	return c
}

// dryRunRuns cover every form of the --dry-run output (spec 8.4): a
// generic mutation, deliver, an upload, create --file (the upload as a
// second request), --from-estimate (the read already sent) and api.
var dryRunRuns = []contractRun{
	{name: "generisch", args: []string{"company", "create", "--name", "Muster GmbH", "--contact-type", "customer", "--dry-run", "-o", "json"}},
	{name: "ohne Body", args: []string{"company", "delete", "12", "--dry-run", "-o", "json"}},
	{name: "deliver", args: []string{"invoice", "deliver", "459", "--via", "email", "--to", "kunde@example.org",
		"--subject", "Rechnung", "--body", "Hallo", "--dry-run", "-o", "json"}},
	{name: "upload", setup: func(t *testing.T, _ *mockapi.Server) []string {
		return []string{writeFile(t, "beleg.pdf", pdfContent)}
	}, args: []string{"voucher", "upload", "295", "--dry-run", "-o", "json"}},
	// mime.TypeByExtension gives text types with a parameter
	// ("text/html; charset=utf-8"); no local type restriction (spec 9.3)
	{name: "upload html", setup: func(t *testing.T, _ *mockapi.Server) []string {
		return []string{writeFile(t, "quittung.html", "<html></html>")}
	}, args: []string{"voucher", "upload", "295", "--dry-run", "-o", "json"}},
	{name: "upload txt", setup: func(t *testing.T, _ *mockapi.Server) []string {
		return []string{writeFile(t, "beleg.txt", "Beleg")}
	}, args: []string{"voucher", "upload", "295", "--dry-run", "-o", "json"}},
	{name: "create --file html", setup: func(t *testing.T, _ *mockapi.Server) []string {
		return []string{"--file", writeFile(t, "quittung.html", "<html></html>")}
	}, args: append(slices.Clone(voucherCreateArgs), "--dry-run", "-o", "json")},
	{name: "create --file", setup: func(t *testing.T, _ *mockapi.Server) []string {
		return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
	}, args: append(slices.Clone(voucherCreateArgs), "--dry-run", "-o", "json")},
	{name: "from-estimate", args: fromEstimateCmd("--dry-run", "-o", "json")},
	{name: "Namensauflösung", setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH")}))
		return nil
	}, args: []string{"person", "delete", "1059", "--company", "Mustermann GmbH", "--dry-run", "-o", "json"}},
	{name: "Namensauflösung Felder", setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(2, "Mustermann GmbH")}))
		srv.OnFunc("GET", "/users", pagedList(t, 1, []map[string]any{person(1, "Max", "Mustermann")}))
		return nil
	}, args: []string{"project", "create", "--name", "P", "--customer", "mustermann gmbh", "--team-member", "Max Mustermann", "--dry-run", "-o", "json"}},
	{name: "api", args: []string{"api", "POST", "/contact/companies", "-f", "name=Muster GmbH", "-f", "contact_type=customer", "--dry-run", "-o", "json"}},
}

// Every form of --dry-run -o json satisfies docs/schemas/dry-run.json and
// sends nothing that changes data.
func TestDryRunContract(t *testing.T) {
	for _, c := range dryRunRuns {
		t.Run(c.name, func(t *testing.T) {
			out, _ := c.run(t)
			wantContract(t, "dry-run", []byte(out))
			var doc map[string]any
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatal(err)
			}
			switch c.name {
			case "upload", "create --file":
				if doc["upload"] == nil {
					t.Errorf("upload fehlt: %s", out)
				}
			case "upload html", "upload txt", "create --file html":
				if !strings.Contains(out, `; charset=utf-8"`) {
					t.Errorf("MIME-Typ ohne Parameter: %s", out)
				}
			case "from-estimate":
				if doc["read"] == nil {
					t.Errorf("read fehlt: %s", out)
				}
			case "Namensauflösung Felder":
				if r, _ := doc["resolved"].([]any); len(r) != 2 || doc["credits"] != 12.0 {
					t.Errorf("resolved/credits: %s", out)
				}
			}
		})
	}
}

// delete -o json prints {"id":N,"deleted":true} (spec 10.3).
func TestDeleteContract(t *testing.T) {
	c := contractRun{name: "delete", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("DELETE", "/contact/companies/12", http.StatusNoContent, nil)
		return nil
	}, args: []string{"company", "delete", "12", "--yes", "-o", "json"}}
	out, _ := c.run(t)
	wantContract(t, "delete", []byte(out))
}

// voucher create --file whose upload fails is exit 11 with the partial
// result on stdout (spec 9.3): with the voucher ID and the command that
// uploads the file alone, or both null when the API sent no ID.
func TestPartialContract(t *testing.T) {
	runs := []contractRun{
		{name: "mit ID", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/expense/vouchers/{voucher_id}/documents", http.StatusUnprocessableEntity, apiError("Dateityp nicht erlaubt"))
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
		{name: "Upload mit 600", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/expense/vouchers/{voucher_id}/documents", 600, nil)
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
		{name: "ohne ID", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/expense/vouchers", http.StatusCreated, []byte(`{"name":"Papier"}`))
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
		// a 2xx answer longer than 64 MiB (ruling FR-D): no http_status
		{name: "Upload zu groß", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.OnFunc("POST", "/expense/vouchers/{voucher_id}/documents", tooLarge(http.StatusOK, "application/json"))
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
		{name: "Anlegen zu groß", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.OnFunc("POST", "/expense/vouchers", tooLarge(http.StatusCreated, "application/json"))
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
	}
	for _, c := range runs {
		t.Run(c.name, func(t *testing.T) {
			out, errs := c.run(t)
			wantContract(t, "partial", []byte(out))
			wantContract(t, "error", []byte(lastLine(errs)))
			if strings.Contains(c.name, "zu groß") && strings.Contains(lastLine(errs), "http_status") {
				t.Errorf("http_status bei zu großer 2xx-Antwort: %s", lastLine(errs))
			}
		})
	}
}

// deliver -o json prints the document read back (spec 9.1), with warnings
// when the answer was lost but the readback shows it sent today (spec
// 10.4), or {"id","delivered","warnings"} when only the readback failed.
func TestDeliverContract(t *testing.T) {
	runs := []contractRun{
		{name: "Rechnung", setup: func(_ *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
			srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
			return nil
		}, args: []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}},
		{name: "Angebot", setup: func(_ *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/income/estimates/349/deliver", http.StatusOK, nil)
			srv.On("GET", "/income/estimates/349", http.StatusOK, sentDoc("estimate", 349, "null"))
			return nil
		}, args: []string{"estimate", "deliver", "349", "--via", "pdf", "--yes", "-o", "json"}},
		{name: "Antwort verloren, heute versendet", setup: func(_ *testing.T, srv *mockapi.Server) []string {
			srv.OnFunc("POST", "/income/invoices/459/deliver", closeAfterRead)
			srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
			return nil
		}, args: []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}},
		{name: "Nachlesen gescheitert", setup: func(_ *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
			srv.On("GET", "/income/invoices/459", http.StatusInternalServerError, apiError("kaputt"))
			return nil
		}, args: []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}},
	}
	for _, c := range runs {
		t.Run(c.name, func(t *testing.T) {
			out, _ := c.run(t)
			wantContract(t, "deliver", []byte(out))
		})
	}
}

// listPages answers the invoice list with two pages of one entry each, so
// --all merges them (page 1, has_more false, total_pages as sent).
func listPages(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	if page == "" {
		page = "1"
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"type":"list","page":%s,"page_size":1,"total_pages":2,"total_entries":2,"has_more":%t,"entries":[{"id":%s,"name":"Beratung"}]}`,
		page, page == "1", page)
}

// outputRuns cover the plain JSON outputs of spec 10.3 with their
// contract: list (API envelope, --all, local filter, api --paginate), the
// empty answer of an action, an update, a create and papierkram api, auth
// status with every source of context, token and read-only, and version.
// check tests what the contract cannot (review T13-M4: one function per
// run instead of a switch on its name); nil checks nothing more.
var outputRuns = []struct {
	schema string
	contractRun
	check func(t *testing.T, doc map[string]any, out string)
}{
	{"list", contractRun{name: "list", args: []string{"invoice", "list", "-o", "json"}}, nil},
	{"list", contractRun{name: "list --all", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/income/invoices", listPages)
		return nil
	}, args: []string{"invoice", "list", "--all", "-o", "json"}}, wantMerged},
	{"list", contractRun{name: "list lokal gefiltert", args: []string{"company", "list", "--type", "customer", "-o", "json"}}, nil},
	{"list", contractRun{name: "api --paginate", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/income/invoices", listPages)
		return nil
	}, args: []string{"api", "GET", "/income/invoices", "--paginate", "-o", "json"}}, wantMerged},
	{"ok", contractRun{name: "archive leer", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/contact/companies/12/archive", http.StatusOK, nil)
		return nil
	}, args: []string{"company", "archive", "12", "-o", "json"}}, wantOKID(12)},
	// review T13-M5: update and papierkram api with an empty answer
	{"ok", contractRun{name: "update leer", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("PUT", "/contact/companies/12", http.StatusOK, nil)
		return nil
	}, args: []string{"company", "update", "12", "--name", "Muster GmbH", "-o", "json"}}, wantOKID(12)},
	{"ok", contractRun{name: "create leer", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/contact/companies", http.StatusCreated, nil)
		return nil
	}, args: []string{"company", "create", "--name", "Muster GmbH", "--contact-type", "customer", "-o", "json"}}, wantOKWithoutID},
	{"ok", contractRun{name: "api POST leer", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("POST", "/contact/companies/12/archive", http.StatusOK, nil)
		return nil
	}, args: []string{"api", "POST", "/contact/companies/12/archive", "-o", "json"}}, wantOKWithoutID},
	{"auth-status", contractRun{name: "auth status", setup: func(_ *testing.T, srv *mockapi.Server) []string {
		srv.On("GET", "/info", http.StatusOK, []byte(`{}`), "X-Remaining-Quota", "8000", "X-Consumed-Quota", "1")
		return nil
	}, args: []string{"auth", "status", "-o", "json"}}, func(t *testing.T, doc map[string]any, out string) {
		if q := objOf(doc, "quota"); q["known"] != true || q["request_cost"] != 1.0 {
			t.Errorf("Kontingent fehlt: %s", out)
		}
		wantSources(t, doc, "", "env", "")
	}},
	{"auth-status", contractRun{name: "auth status ohne Kontingent", env: map[string]string{"PAPIERKRAM_READ_ONLY": "1"},
		setup: func(_ *testing.T, srv *mockapi.Server) []string {
			srv.On("GET", "/info", http.StatusOK, []byte(`{}`), "X-Remaining-Quota", "", "X-Consumed-Quota", "")
			return nil
		}, args: []string{"auth", "status", "-o", "json"}}, func(t *testing.T, doc map[string]any, out string) {
		if q := objOf(doc, "quota"); q["known"] != false || q["request_cost"] != nil {
			t.Errorf("Kontingent bekannt: %s", out)
		}
		wantSources(t, doc, "", "env", "env")
	}},
	// without a token auth status still prints what it knows (exit 3)
	{"auth-status", contractRun{name: "auth status ohne Token", exit: 3, env: map[string]string{"PAPIERKRAM_TOKEN": ""},
		args: []string{"auth", "status", "-o", "json"}}, func(t *testing.T, doc map[string]any, _ string) {
		wantSources(t, doc, "", "", "")
	}},
	// review T13-M1: the other sources, each once (token of the context
	// from the keychain or in plain text, context by flag, environment,
	// project file or active_context, read-only of the context)
	{"auth-status", contractRun{name: "auth status Schlüsselbund --context", env: map[string]string{"PAPIERKRAM_TOKEN": ""},
		prepare: authContexts, args: []string{"auth", "status", "--context", "b", "-o", "json"}},
		func(t *testing.T, doc map[string]any, _ string) {
			wantSources(t, doc, "flag", "keychain", "")
			if doc["context"] != "b" || doc["subdomain"] != "firma-b" {
				t.Errorf("Kontext: %v", doc)
			}
		}},
	{"auth-status", contractRun{name: "auth status PAPIERKRAM_CONTEXT", env: map[string]string{"PAPIERKRAM_TOKEN": "", "PAPIERKRAM_CONTEXT": "b"},
		prepare: authContexts, args: []string{"auth", "status", "-o", "json"}},
		func(t *testing.T, doc map[string]any, _ string) { wantSources(t, doc, "env", "keychain", "") }},
	{"auth-status", contractRun{name: "auth status active_context", env: map[string]string{"PAPIERKRAM_TOKEN": ""},
		prepare: authContexts, args: []string{"auth", "status", "-o", "json"}},
		func(t *testing.T, doc map[string]any, _ string) {
			wantSources(t, doc, "config", "keychain", "")
			if doc["context"] != "a" {
				t.Errorf("Kontext: %v", doc)
			}
		}},
	{"auth-status", contractRun{name: "auth status Projektdatei, Klartext, Schreibschutz des Kontexts", env: map[string]string{"PAPIERKRAM_TOKEN": ""},
		prepare: func(t *testing.T, app *App) {
			authContexts(t, app)
			writeProjectFile(t, app, "context = \"c\"\n")
		}, args: []string{"auth", "status", "-o", "json"}},
		func(t *testing.T, doc map[string]any, _ string) {
			wantSources(t, doc, "projectfile", "config", "context")
			if doc["context"] != "c" || doc["read_only"] != true {
				t.Errorf("Kontext: %v", doc)
			}
		}},
	{"version", contractRun{name: "version", args: []string{"version", "-o", "json"}}, nil},
	// invoice paid-status (spec v0.2 §5): an open item with due date and
	// one without, number and text amounts, a cut list (complete false)
	{"paid-status", contractRun{name: "invoice paid-status", setup: func(t *testing.T, srv *mockapi.Server) []string {
		srv.OnFunc("GET", "/income/invoices", pagedList(t, 7, []map[string]any{
			paidEntry(3, "unpaid", "119.0", "2026-06-01"), paidEntry(5, "partially_paid", 50.5, nil),
		}, []map[string]any{paidEntry(6, "unpaid", "1.0", "2026-12-01")}))
		return nil
	}, args: []string{"invoice", "paid-status", "-o", "json"}}, func(t *testing.T, doc map[string]any, out string) {
		if doc["complete"] != false || doc["count"] != 3.0 || doc["total_outstanding"] != 170.5 {
			t.Errorf("Bericht: %s", out)
		}
	}},
}

// authContexts writes the contexts a and b (token in the keychain) and c
// (token in plain text, read-only) with a active.
func authContexts(t *testing.T, app *App) {
	t.Helper()
	writeCfg(t, app, twoWritableCtx+"[contexts.c]\nsubdomain = \"firma-c\"\ntoken = \"test-token-c\"\nread_only = true\n")
	for _, ctx := range []string{"a", "b"} {
		if err := app.Keyring.Set(ctx, "test-token-"+ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// wantSources checks the three sources of an auth status document.
func wantSources(t *testing.T, doc map[string]any, context, token, readOnly string) {
	t.Helper()
	if doc["context_source"] != context || doc["token_source"] != token || doc["read_only_source"] != readOnly {
		t.Errorf("Quellen = %v/%v/%v, erwartet %q/%q/%q", doc["context_source"], doc["token_source"], doc["read_only_source"], context, token, readOnly)
	}
}

// wantMerged checks a list merged from the two pages of listPages.
func wantMerged(t *testing.T, doc map[string]any, out string) {
	t.Helper()
	if doc["page"] != 1.0 || doc["has_more"] != false || len(doc["entries"].([]any)) != 2 {
		t.Errorf("nicht zusammengeführt: %s", out)
	}
}

// wantOKID checks the empty answer of an update or action: {"id":id,"ok":true}.
func wantOKID(id float64) func(t *testing.T, doc map[string]any, out string) {
	return func(t *testing.T, doc map[string]any, out string) {
		t.Helper()
		if doc["id"] != id || len(doc) != 2 {
			t.Errorf("erwartet id %v: %s", id, out)
		}
	}
}

// wantOKWithoutID checks the empty answer of a create or papierkram api.
func wantOKWithoutID(t *testing.T, doc map[string]any, out string) {
	t.Helper()
	if _, ok := doc["id"]; ok {
		t.Errorf("id ohne Objekt: %s", out)
	}
}

// The JSON outputs of list, an empty answer, auth status and version
// satisfy their contracts in docs/schemas/ (spec 10.3).
func TestOutputContract(t *testing.T) {
	for _, c := range outputRuns {
		t.Run(c.name, func(t *testing.T) {
			out, _ := c.run(t)
			wantContract(t, c.schema, []byte(out))
			var doc map[string]any
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatal(err)
			}
			if c.check != nil {
				c.check(t, doc, out)
			}
		})
	}
}

// contractMutation changes a valid document so that its contract must
// reject it; want is part of the error kin-openapi reports for the rule.
// An empty want marks a variant the contract must still accept.
type contractMutation struct {
	fn   func(d map[string]any)
	want string
}

// The contracts are strict: each change of a valid document is rejected
// for the rule it breaks (like TestCommandsJSONMatchesContract).
func TestContractSchemasStrict(t *testing.T) {
	cases := []struct {
		name   string
		valid  string
		mutate map[string]contractMutation
	}{
		{"error", `{"error":{"code":"not_found","message":"Rechnung nicht gefunden","hint":"ID prüfen","http_status":404,"correlation_id":"corr-42","exit_code":4}}`,
			map[string]contractMutation{
				"unbekanntes Feld oben":   {func(d map[string]any) { d["status"] = 404.0 }, `property "status" is unsupported`},
				"unbekanntes Fehlerfeld":  {func(d map[string]any) { errOf(d)["details"] = "x" }, `property "details" is unsupported`},
				"code fehlt":              {func(d map[string]any) { delete(errOf(d), "code") }, `property "code" is missing`},
				"exit_code fehlt":         {func(d map[string]any) { delete(errOf(d), "exit_code") }, `property "exit_code" is missing`},
				"unbekannter code":        {func(d map[string]any) { errOf(d)["code"] = "gone" }, "value is not one of the allowed values"},
				"exit_code passt nicht":   {func(d map[string]any) { errOf(d)["exit_code"] = 5.0 }, `Error at "/exit_code": value is not one of the allowed values [4]`},
				"http_status passt nicht": {func(d map[string]any) { errOf(d)["http_status"] = 500.0 }, `Error at "/http_status": value is not one of the allowed values [404]`},
				"http_status 2xx": {func(d map[string]any) {
					e := errOf(d)
					e["code"], e["exit_code"], e["http_status"] = "error", 1.0, 200.0
				}, `Error at "/error/http_status": Doesn't match schema "not"`},
				"server_error mit 404":       {withCode("server_error", 1, 404), `Error at "/http_status": number must be at least 500`},
				"server_error ohne Status":   {withCode("server_error", 1, 0), `property "http_status" is missing`},
				"server_error mit 503":       {withCode("server_error", 1, 503), ""},
				"error mit 500":              {withCode("error", 1, 500), `Error at "/http_status": number must be at most 499`},
				"error mit 418":              {withCode("error", 1, 418), ""},
				"error mit 101":              {withCode("error", 1, 101), ""},
				"error mit 99":               {withCode("error", 1, 99), "number must be at least 100"},
				"server_error mit 999":       {withCode("server_error", 1, 999), ""},
				"server_error mit 1000":      {withCode("server_error", 1, 1000), "number must be at most 999"},
				"partial mit 600":            {withCode("partial", 11, 600), ""},
				"error ohne Status":          {withCode("error", 1, 0), ""},
				"usage mit Status":           {withCode("usage", 2, 400), `Doesn't match schema "not"`},
				"confirmation mit Status":    {withCode("confirmation_required", 8, 409), `Doesn't match schema "not"`},
				"read_only mit Status":       {withCode("read_only", 9, 403), `Doesn't match schema "not"`},
				"network mit Status":         {withCode("network", 10, 502), `Doesn't match schema "not"`},
				"outcome_unknown mit Status": {withCode("outcome_unknown", 12, 504), `Doesn't match schema "not"`},
				"partial mit 413":            {withCode("partial", 11, 413), ""},
				"partial ohne Status":        {withCode("partial", 11, 0), ""},
				"auth ohne Status":           {withCode("auth", 3, 0), ""},
				"hint leer":                  {func(d map[string]any) { errOf(d)["hint"] = "" }, "minimum string length is 1"},
				"correlation_id null":        {func(d map[string]any) { errOf(d)["correlation_id"] = nil }, "Value is not nullable"},
				"exit_code als Text":         {func(d map[string]any) { errOf(d)["exit_code"] = "4" }, `Error at "/exit_code": value is not one of the allowed values [4]`},
			}},
		{"dry-run", `{"dry_run":true,"method":"POST","url":"https://firma.papierkram.de/api/v1/expense/vouchers","body":{"name":"Papier"},` +
			`"upload":{"method":"POST","url":"https://firma.papierkram.de/api/v1/expense/vouchers/{voucher_id}/documents","credits":10,"name":"beleg.pdf","size":15,"mime_type":"application/pdf"},"credits":60}`,
			map[string]contractMutation{
				"unbekanntes Feld":           {func(d map[string]any) { d["sent"] = false }, `property "sent" is unsupported`},
				"dry_run false":              {func(d map[string]any) { d["dry_run"] = false }, "value is not one of the allowed values"},
				"GET":                        {func(d map[string]any) { d["method"] = "GET" }, "value is not one of the allowed values"},
				"body fehlt":                 {func(d map[string]any) { delete(d, "body") }, `property "body" is missing`},
				"body als Text":              {func(d map[string]any) { d["body"] = "{}" }, "value must be an object"},
				"URL relativ":                {func(d map[string]any) { d["url"] = "/expense/vouchers" }, "regular expression"},
				"credits 0":                  {func(d map[string]any) { d["credits"] = 0.0 }, "number must be at least 1"},
				"upload ohne mime_type":      {func(d map[string]any) { delete(objOf(d, "upload"), "mime_type") }, `property "mime_type" is missing`},
				"upload halb zweite Anfrage": {func(d map[string]any) { delete(objOf(d, "upload"), "credits") }, `Error at "/credits": property "credits" is missing`},
				"upload unbekanntes Feld":    {func(d map[string]any) { objOf(d, "upload")["path"] = "/tmp/x" }, `property "path" is unsupported`},
				"read ohne credits":          {func(d map[string]any) { d["read"] = map[string]any{"method": "GET", "url": "https://x/api/v1/a"} }, `property "credits" is missing`},
				"MIME-Typ mit Leerzeichen":   {withMIME("text plain"), "regular expression"},
				"MIME-Typ ohne Subtyp":       {withMIME("text"), "regular expression"},
				"MIME-Parameter ohne Wert":   {withMIME("text/html; charset"), "regular expression"},
				"MIME-Typ mit Semikolon":     {withMIME("text/html;"), "regular expression"},
				"MIME-Typ mit Parameter":     {withMIME("text/plain; charset=utf-8"), ""},
				"MIME-Typ mit Pluszeichen":   {withMIME("image/svg+xml"), ""},
				"MIME-Parameter in Quotes":   {withMIME(`text/plain;charset="utf-8"`), ""},
				"MIME-Typ unbekannte Datei":  {withMIME("application/octet-stream"), ""},
				"read mit POST": {func(d map[string]any) {
					d["read"] = map[string]any{"method": "POST", "url": "https://x/api/v1/a", "credits": 1.0}
				}, "value is not one of the allowed values"},
				"resolved":                  {withResolved(nil), ""},
				"resolved aus Cache":        {withResolved(func(r map[string]any) { r["credits"], r["cached"] = 0.0, true }), ""},
				"resolved leer":             {func(d map[string]any) { d["resolved"] = []any{} }, "minimum number of items is 1"},
				"resolved unbekanntes Feld": {withResolved(func(r map[string]any) { r["url"] = "https://x/api/v1/a" }), `property "url" is unsupported`},
				"resolved credits negativ":  {withResolved(func(r map[string]any) { r["credits"] = -1.0 }), "number must be at least 0"},
				"resolved id fehlt":         {withResolved(func(r map[string]any) { delete(r, "id") }), `property "id" is missing`},
				"resolved cached fehlt":     {withResolved(func(r map[string]any) { delete(r, "cached") }), `property "cached" is missing`},
				"resolved Flag mit --":      {withResolved(func(r map[string]any) { r["flag"] = "--customer" }), "regular expression"},
				"resolved Name leer":        {withResolved(func(r map[string]any) { r["name"] = "" }), "minimum string length is 1"},
				"resolved kein Array":       {func(d map[string]any) { d["resolved"] = map[string]any{} }, "value must be an array"},
			}},
		{"delete", `{"id":12,"deleted":true}`,
			map[string]contractMutation{
				"unbekanntes Feld": {func(d map[string]any) { d["ok"] = true }, `property "ok" is unsupported`},
				"deleted false":    {func(d map[string]any) { d["deleted"] = false }, "value is not one of the allowed values"},
				"id fehlt":         {func(d map[string]any) { delete(d, "id") }, `property "id" is missing`},
				"id als Text":      {func(d map[string]any) { d["id"] = "12" }, "value must be an integer"},
			}},
		{"partial", `{"id":290,"completed":["create"],"failed":["upload"],"retry_command":"papierkram voucher upload 290 '/tmp/mein beleg.pdf'","warnings":["Beleg 290 angelegt, Datei nicht hochgeladen"]}`,
			map[string]contractMutation{
				"unbekanntes Feld":         {func(d map[string]any) { d["exit_code"] = 11.0 }, `property "exit_code" is unsupported`},
				"retry_command fehlt":      {func(d map[string]any) { delete(d, "retry_command") }, `property "retry_command" is missing`},
				"id null mit Befehl":       {func(d map[string]any) { d["id"] = nil }, `Error at "/retry_command": value is not one of the allowed values [null]`},
				"Befehl null mit id":       {func(d map[string]any) { d["retry_command"] = nil }, `Error at "/id": value is not one of the allowed values [null]`},
				"Befehl ohne papierkram":   {func(d map[string]any) { d["retry_command"] = "voucher upload 290 x" }, "regular expression"},
				"completed leer":           {func(d map[string]any) { d["completed"] = []any{} }, "minimum number of items is 1"},
				"unbekannter Schritt":      {func(d map[string]any) { d["failed"] = []any{"mail"} }, "value is not one of the allowed values"},
				"warnings leer":            {func(d map[string]any) { d["warnings"] = []any{} }, "minimum number of items is 1"},
				"Schritt doppelt erledigt": {func(d map[string]any) { d["completed"] = []any{"create", "create"} }, "duplicate items found"},
			}},
		{"deliver", `{"id":459,"invoice_no":"RE-1001","state":"unpaid","sent_on":"2026-11-04","sent_via":"email","sent_to":"kunde@example.org"}`,
			map[string]contractMutation{
				"unbekanntes Feld":        {func(d map[string]any) { d["name"] = "Beratung" }, `property "name" is unsupported`},
				"sent_on fehlt":           {func(d map[string]any) { delete(d, "sent_on") }, `Error at "/sent_on": property "sent_on" is missing`},
				"ohne Belegnummer":        {func(d map[string]any) { delete(d, "invoice_no") }, `Error at "/estimate_no": property "estimate_no" is missing`},
				"beide Belegnummern":      {func(d map[string]any) { d["estimate_no"] = "AN-1" }, `Doesn't match schema "not"`},
				"delivered zum Nachlesen": {func(d map[string]any) { d["delivered"] = true }, `Doesn't match schema "not"`},
				"warnings leer":           {func(d map[string]any) { d["warnings"] = []any{} }, "minimum number of items is 1"},
				"id als Text":             {func(d map[string]any) { d["id"] = "459" }, "value must be an integer"},
				"delivered ohne Warnung": {func(d map[string]any) {
					for _, k := range []string{"invoice_no", "state", "sent_on", "sent_via", "sent_to"} {
						delete(d, k)
					}
					d["delivered"] = true
				}, `Error at "/warnings": property "warnings" is missing`},
				"delivered false": {func(d map[string]any) {
					for _, k := range []string{"invoice_no", "state", "sent_on", "sent_via", "sent_to"} {
						delete(d, k)
					}
					d["delivered"], d["warnings"] = false, []any{"x"}
				}, "value is not one of the allowed values"},
			}},
		{"list", `{"type":"list","page":1,"page_size":2,"total_pages":3,"total_entries":5,"has_more":true,"entries":[{"id":441,"name":"Beratung","billing":{"company":"Muster GmbH"}}]}`,
			map[string]contractMutation{
				"unbekanntes Feld":       {func(d map[string]any) { d["next_page"] = 2.0 }, `property "next_page" is unsupported`},
				"entries fehlt":          {func(d map[string]any) { delete(d, "entries") }, `property "entries" is missing`},
				"total_pages fehlt":      {func(d map[string]any) { delete(d, "total_pages") }, `property "total_pages" is missing`},
				"has_more als Text":      {func(d map[string]any) { d["has_more"] = "false" }, "value must be a boolean"},
				"page als Text":          {func(d map[string]any) { d["page"] = "1" }, "value must be an integer"},
				"page_size null":         {func(d map[string]any) { d["page_size"] = nil }, "Value is not nullable"},
				"Eintrag kein Objekt":    {func(d map[string]any) { d["entries"] = []any{441.0} }, "value must be an object"},
				"entries leer":           {func(d map[string]any) { d["entries"] = []any{} }, ""},
				"Eintrag beliebige Form": {func(d map[string]any) { d["entries"] = []any{map[string]any{"x": nil}} }, ""},
			}},
		{"ok", `{"id":12,"ok":true}`,
			map[string]contractMutation{
				"unbekanntes Feld": {func(d map[string]any) { d["deleted"] = true }, `property "deleted" is unsupported`},
				"ok false":         {func(d map[string]any) { d["ok"] = false }, "value is not one of the allowed values"},
				"ok fehlt":         {func(d map[string]any) { delete(d, "ok") }, `property "ok" is missing`},
				"id als Text":      {func(d map[string]any) { d["id"] = "12" }, "value must be an integer"},
				"id negativ":       {func(d map[string]any) { d["id"] = -1.0 }, "number must be at least 0"},
				"id null":          {func(d map[string]any) { d["id"] = nil }, "Value is not nullable"},
				"ohne id":          {func(d map[string]any) { delete(d, "id") }, ""},
			}},
		{"auth-status", `{"context":"firma-gmbh","context_source":"config","subdomain":"firma-gmbh","base_url":"https://firma-gmbh.papierkram.de/api/v1",` +
			`"token_source":"keychain","read_only":false,"read_only_source":"","quota":{"remaining":8000,"request_cost":1,"known":true}}`,
			map[string]contractMutation{
				"unbekanntes Feld":           {func(d map[string]any) { d["token"] = "test-token-x" }, `property "token" is unsupported`},
				"unbekanntes Kontingentfeld": {func(d map[string]any) { objOf(d, "quota")["used"] = 2.0 }, `property "used" is unsupported`},
				"quota fehlt":                {func(d map[string]any) { delete(d, "quota") }, `property "quota" is missing`},
				"request_cost fehlt":         {func(d map[string]any) { delete(objOf(d, "quota"), "request_cost") }, `property "request_cost" is missing`},
				"request_cost null":          {func(d map[string]any) { objOf(d, "quota")["request_cost"] = nil }, ""},
				"remaining null":             {func(d map[string]any) { objOf(d, "quota")["remaining"] = nil }, "Value is not nullable"},
				"read_only als Text":         {func(d map[string]any) { d["read_only"] = "false" }, "value must be a boolean"},
				"unbekannte Token-Quelle":    {func(d map[string]any) { d["token_source"] = "flag" }, "value is not one of the allowed values"},
				"unbekannte Kontext-Quelle":  {func(d map[string]any) { d["context_source"] = "keychain" }, "value is not one of the allowed values"},
				"unbekannte Schutz-Quelle":   {func(d map[string]any) { d["read_only_source"] = "config" }, "value is not one of the allowed values"},
				"ohne Kontext": {func(d map[string]any) {
					d["context"], d["context_source"], d["token_source"] = "", "", "env"
				}, ""},
				"Schreibschutz aus env": {func(d map[string]any) { d["read_only"], d["read_only_source"] = true, "env" }, ""},
			}},
		{"paid-status", `{"entries":[{"id":3,"invoice_no":"RE-0003","name":"Beratung","customer":"Mustermann GmbH","state":"unpaid",` +
			`"document_date":"2026-05-01","due_date":"2026-06-01","days_overdue":9,"total_gross":119.0,"outstanding_amount":119.0}],` +
			`"total_outstanding":119.00,"count":1,"overdue_count":1,"as_of":"2026-06-10","complete":true}`,
			map[string]contractMutation{
				"unbekanntes Feld":                 {func(d map[string]any) { d["currency"] = "EUR" }, `property "currency" is unsupported`},
				"unbekanntes Eintragsfeld":         {withPaidEntry(func(e map[string]any) { e["billing"] = map[string]any{} }), `property "billing" is unsupported`},
				"as_of fehlt":                      {func(d map[string]any) { delete(d, "as_of") }, `property "as_of" is missing`},
				"as_of kein Datum":                 {func(d map[string]any) { d["as_of"] = "10.06.2026" }, "regular expression"},
				"complete fehlt":                   {func(d map[string]any) { delete(d, "complete") }, `property "complete" is missing`},
				"total_outstanding als String":     {func(d map[string]any) { d["total_outstanding"] = "119.00" }, "value must be a number"},
				"count negativ":                    {func(d map[string]any) { d["count"] = -1.0 }, "number must be at least 0"},
				"overdue_count als Zahl mit Komma": {func(d map[string]any) { d["overdue_count"] = 1.5 }, "value must be an integer"},
				"outstanding_amount als String":    {withPaidEntry(func(e map[string]any) { e["outstanding_amount"] = "119.0" }), "value must be a number"},
				"outstanding_amount fehlt":         {withPaidEntry(func(e map[string]any) { delete(e, "outstanding_amount") }), `property "outstanding_amount" is missing`},
				"days_overdue negativ":             {withPaidEntry(func(e map[string]any) { e["days_overdue"] = -1.0 }), "number must be at least 0"},
				"customer null":                    {withPaidEntry(func(e map[string]any) { e["customer"] = nil }), "Value is not nullable"},
				"Nummer und Daten null": {withPaidEntry(func(e map[string]any) {
					e["invoice_no"], e["document_date"], e["due_date"] = nil, nil, nil
				}), ""},
				"due_date kein Datum": {withPaidEntry(func(e map[string]any) { e["due_date"] = "morgen" }), "regular expression"},
				"entries leer":        {func(d map[string]any) { d["entries"] = []any{} }, ""},
			}},
		{"version", `{"version":"dev","commit":"none","date":"unknown","go_version":"go1.27.1"}`,
			map[string]contractMutation{
				"unbekanntes Feld": {func(d map[string]any) { d["os"] = "darwin" }, `property "os" is unsupported`},
				"commit fehlt":     {func(d map[string]any) { delete(d, "commit") }, `property "commit" is missing`},
				"version als Zahl": {func(d map[string]any) { d["version"] = 1.0 }, "value must be a string"},
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schema := loadContract(t, c.name)
			if err := checkContract(c.name, []byte(c.valid)); err != nil {
				t.Fatalf("gültiges Dokument abgelehnt: %v", err)
			}
			for name, m := range c.mutate {
				var doc map[string]any
				if err := json.Unmarshal([]byte(c.valid), &doc); err != nil {
					t.Fatal(err)
				}
				m.fn(doc)
				err := schema.VisitJSON(doc)
				switch {
				case m.want == "" && err != nil:
					t.Errorf("%s: gültige Variante abgelehnt: %v", name, err)
				case m.want == "":
				case err == nil:
					t.Errorf("%s: Vertrag lässt das Dokument durch", name)
				case !strings.Contains(err.Error(), m.want):
					t.Errorf("%s: abgelehnt, aber nicht wegen %q:\n%v", name, m.want, err)
				}
			}
		})
	}
}

// withResolved adds a resolution to a decoded dry-run document, changed
// by fn when not nil.
func withResolved(fn func(r map[string]any)) func(d map[string]any) {
	return func(d map[string]any) {
		r := map[string]any{"flag": "customer", "name": "Mustermann GmbH", "resource": "company", "id": 2.0, "credits": 1.0, "cached": false}
		if fn != nil {
			fn(r)
		}
		d["resolved"] = []any{r}
	}
}

// withPaidEntry changes the first entry of a decoded paid-status report.
func withPaidEntry(fn func(e map[string]any)) func(d map[string]any) {
	return func(d map[string]any) { fn(d["entries"].([]any)[0].(map[string]any)) }
}

// withMIME sets the MIME type of the upload of a decoded dry-run document.
func withMIME(v string) func(d map[string]any) {
	return func(d map[string]any) { objOf(d, "upload")["mime_type"] = v }
}

// withCode sets code, exit code and HTTP status (0: none) of a decoded
// error document.
func withCode(code string, exit, status int) func(d map[string]any) {
	return func(d map[string]any) {
		e := errOf(d)
		e["code"], e["exit_code"] = code, float64(exit)
		delete(e, "http_status")
		if status != 0 {
			e["http_status"] = float64(status)
		}
	}
}

// errOf is the error object of a decoded error document.
func errOf(d map[string]any) map[string]any { return objOf(d, "error") }

// objOf is the object under key of a decoded document.
func objOf(d map[string]any, key string) map[string]any { return d[key].(map[string]any) }

// authStatusSources are the values of the source fields of auth status
// (config.Resolved), "" for none.
var authStatusSources = map[string][]string{
	"context_source":   {"", config.SourceFlag, config.SourceEnv, config.SourceProjectFile, config.SourceConfig},
	"token_source":     {"", config.SourceEnv, config.SourceKeychain, config.SourceConfig},
	"read_only_source": {"", config.SourceEnv, config.SourceContext},
}

// Review T13-M1: the enums of the source fields in auth-status.json are
// the config.Source* constants each field takes; every such constant
// belongs to a field (a new one fails here until the schema and this
// table name it), and the real runs of outputRuns produce every value.
func TestAuthStatusSourceEnums(t *testing.T) {
	schema := loadContract(t, "auth-status")
	used := map[string]bool{}
	for field, want := range authStatusSources {
		ref := schema.Properties[field]
		if ref == nil || ref.Value == nil {
			t.Fatalf("%s fehlt im Schema", field)
		}
		var enum []string
		for _, v := range ref.Value.Enum {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("%s: Enum-Wert %v kein Text", field, v)
			}
			enum = append(enum, s)
		}
		if !sameSet(enum, want) {
			t.Errorf("%s: Enum %q, config.Source* %q", field, enum, want)
		}
		for _, v := range want {
			used[v] = true
		}
	}

	consts := sourceConstants(t)
	if len(consts) < 6 {
		t.Fatalf("nur %d Source-Konstanten gefunden: %v", len(consts), consts)
	}
	for name, v := range consts {
		if !used[v] {
			t.Errorf("config.%s = %q gehört zu keinem Feld von auth status", name, v)
		}
	}

	seen := map[string][]string{}
	for _, c := range outputRuns {
		if c.schema != "auth-status" {
			continue
		}
		out, _ := c.run(t)
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for field := range authStatusSources {
			if v, _ := doc[field].(string); !slices.Contains(seen[field], v) {
				seen[field] = append(seen[field], v)
			}
		}
	}
	for field, want := range authStatusSources {
		if !sameSet(seen[field], want) {
			t.Errorf("%s: Läufe zeigen %q, erwartet alle von %q", field, seen[field], want)
		}
	}
}

// sameSet reports whether a and b hold the same strings.
func sameSet(a, b []string) bool {
	a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

// sourceConstants are the string constants Source* of package config
// (name to value), read from its source so a new one is not missed.
func sourceConstants(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../config/*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if !strings.HasPrefix(n.Name, "Source") || i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("config.%s ist kein Text-Literal", n.Name)
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					out[n.Name] = v
				}
			}
		}
	}
	return out
}
