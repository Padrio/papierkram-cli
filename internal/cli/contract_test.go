package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

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
	args  []string
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
		{name: "ohne ID", exit: 11, setup: func(t *testing.T, srv *mockapi.Server) []string {
			srv.On("POST", "/expense/vouchers", http.StatusCreated, []byte(`{"name":"Papier"}`))
			return []string{"--file", writeFile(t, "beleg.pdf", pdfContent)}
		}, args: append(slices.Clone(voucherCreateArgs), "-o", "json")},
	}
	for _, c := range runs {
		t.Run(c.name, func(t *testing.T) {
			out, errs := c.run(t)
			wantContract(t, "partial", []byte(out))
			wantContract(t, "error", []byte(lastLine(errs)))
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
				}, "number must be at least 300"},
				"server_error mit 404":       {withCode("server_error", 1, 404), `Error at "/http_status": number must be at least 500`},
				"server_error ohne Status":   {withCode("server_error", 1, 0), `property "http_status" is missing`},
				"server_error mit 503":       {withCode("server_error", 1, 503), ""},
				"error mit 500":              {withCode("error", 1, 500), `Error at "/http_status": number must be at most 499`},
				"error mit 418":              {withCode("error", 1, 418), ""},
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
