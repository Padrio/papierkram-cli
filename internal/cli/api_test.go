package cli

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// papierkram api (spec 7.2): escape hatch with the protection rules of the
// registry. Every test runs against mockapi with auto-fail, so a request
// to a path of the spec must match it; paths outside the spec are answered
// by explicit overrides.

func runAPI(app *App, args ...string) int {
	return Run(app, append([]string{"api"}, args...))
}

func wantStderrHas(t *testing.T, app *App, want ...string) {
	t.Helper()
	_, errs := bufs(app)
	for _, w := range want {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %q", w, errs)
		}
	}
}

func TestR27_ReadOnlyContextBlocksApiPost(t *testing.T) {
	setup := func() (*App, *mockapi.Server, *countingReader) {
		app, srv := mockApp(t, nil)
		writeCfg(t, app, roActive)
		if err := app.Keyring.Set("b", "test-token-b"); err != nil {
			t.Fatal(err)
		}
		in := &countingReader{r: strings.NewReader(`{"name":"P","customer":{"id":1}}`)}
		app.In = in
		return app, srv, in
	}
	app, srv, in := setup()
	wantExit(t, app, runAPI(app, "POST", "/projects", "--input", "-", "--yes"), 9)
	wantRequests(t, srv, 0)
	if in.n != 0 {
		t.Errorf("Body gelesen (%d Reads), obwohl der Schreibschutz greift", in.n)
	}
	wantStderrHas(t, app, `Kontext "b" ist schreibgeschützt (read_only = true)`)

	// also --dry-run and an unknown path
	for _, args := range [][]string{
		{"POST", "/projects", "-f", "name=P", "--dry-run"},
		{"POST", "/x/y", "-F", "a=@-", "--yes"},
		{"DELETE", "/projects/5", "--yes"},
	} {
		app, srv, in = setup()
		wantExit(t, app, runAPI(app, args...), 9)
		wantRequests(t, srv, 0)
		if in.n != 0 {
			t.Errorf("%v: Standardeingabe gelesen", args)
		}
	}

	// reading stays allowed
	app, srv, _ = setup()
	wantExit(t, app, runAPI(app, "GET", "/info", "-o", "json"), 0)
	wantRequests(t, srv, 1)
}

func TestR33_ApiDeliverEmailWithoutEmailObject(t *testing.T) {
	input := writeFile(t, "deliver.json", `{"send_via":"email","email":{"recipient":"kunde@example.org"}}`)
	for name, args := range map[string][]string{
		"ohne email":      {"-F", "send_via=email"},
		"ohne Betreff":    {"-F", "send_via=email", "-f", "email.recipient=kunde@example.org", "-f", "email.body=Hallo"},
		"leerer Text":     {"-F", "send_via=email", "-f", "email.recipient=a@example.org", "-f", "email.subject=S", "-f", "email.body= "},
		"input unvollst.": {"--input", input},
		"email ist null":  {"-F", "send_via=email", "-F", "email=null"},
		"Angebot":         {"-F", "send_via=email", "--estimate"},
	} {
		app, srv := apiApp(t)
		path := "/income/invoices/12/deliver"
		if args[len(args)-1] == "--estimate" {
			path, args = "/income/estimates/12/deliver", args[:len(args)-1]
		}
		wantExit(t, app, runAPI(app, append([]string{"POST", path, "--yes"}, args...)...), 5)
		wantRequests(t, srv, 0)
		_, errs := bufs(app)
		if !strings.Contains(errs, `send_via "email" verlangt`) || !strings.Contains(errs, "email.") {
			t.Errorf("%s: stderr = %q", name, errs)
		}
	}

	full := []string{"POST", "/income/invoices/12/deliver", "-F", "send_via=email",
		"-f", "email.recipient=kunde@example.org", "-f", "email.subject=Ihre Rechnung", "-f", "email.body=Hallo"}
	// complete: the send class needs the confirmation
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, full...), 8)
	wantRequests(t, srv, 0)

	app, srv = apiApp(t)
	srv.On("POST", "/income/invoices/12/deliver", http.StatusOK, nil)
	wantExit(t, app, runAPI(app, append(full, "--yes", "-o", "json")...), 0)
	wantBody(t, srv, "POST", "/income/invoices/12/deliver",
		`{"send_via":"email","email":{"recipient":"kunde@example.org","subject":"Ihre Rechnung","body":"Hallo"}}`)
	if out := mustOut(app); out != "{\n  \"ok\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	// pdf needs no mail
	app, srv = apiApp(t)
	srv.On("POST", "/income/invoices/12/deliver", http.StatusOK, nil)
	wantExit(t, app, runAPI(app, "POST", "/income/invoices/12/deliver", "-F", "send_via=pdf", "--yes"), 0)
	wantBody(t, srv, "POST", "/income/invoices/12/deliver", `{"send_via":"pdf"}`)
}

func TestR34_ApiDeleteWithoutYesNonTTY(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "DELETE", "/income/invoices/12"), 8)
	wantRequests(t, srv, 0)
	wantStderrHas(t, app, "Bestätigung erforderlich", "--yes")

	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "delete", "/income/invoices/12", "--yes", "-o", "json"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/income/invoices/12" || len(r.Body) != 0 {
		t.Errorf("request = %s %s %q", r.Method, r.Path, r.Body)
	}
	if out := mustOut(app); out != "{\n  \"ok\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

// A known path gets the confirmation of its command: delete wants the ID.
func TestAPIDeleteConfirmsLikeItsCommand(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("12\n")
	invoice12(srv)
	wantExit(t, app, runAPI(app, "DELETE", "/income/invoices/12", "-o", "json"), 0)
	reqs := wantRequests(t, srv, 2)
	if reqs[0].Method != "GET" || reqs[1].Method != "DELETE" {
		t.Errorf("requests = %+v", reqs)
	}
	wantStderrHas(t, app, "Rechnung 12 „Beratung Oktober“ (1.190,00 €) löschen? Zur Bestätigung ID eingeben")

	// --no-input never asks
	app, srv = apiApp(t)
	app.StdinTTY = true
	wantExit(t, app, runAPI(app, "DELETE", "/income/invoices/12", "--no-input"), 8)
	wantRequests(t, srv, 0)
}

// An unknown path is read_only for GET and destructive otherwise.
func TestAPIUnknownPathClass(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/x/y", "-f", "a=1"), 8)
	wantRequests(t, srv, 0)
	wantStderrHas(t, app, "Bestätigung erforderlich: POST /x/y senden")

	app, srv = apiApp(t)
	srv.On("POST", "/x/y", http.StatusCreated, []byte(`{"id":5,"preis":19.99}`))
	wantExit(t, app, runAPI(app, "POST", "/x/y", "-f", "a=1", "--yes", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/x/y", `{"a":"1"}`)
	if out := mustOut(app); out != "{\n  \"id\": 5,\n  \"preis\": 19.99\n}\n" {
		t.Errorf("stdout = %q", out)
	}

	// in a terminal: ja sends, nein is exit 8
	for answer, exit := range map[string]int{"ja\n": 0, "nein\n": 8} {
		app, srv = apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader(answer)
		srv.On("PUT", "/x/y", http.StatusNoContent, nil)
		wantExit(t, app, runAPI(app, "PUT", "/x/y"), exit)
		wantRequests(t, srv, 1-exit/8)
		wantStderrHas(t, app, "PUT /x/y wirklich senden? [ja/nein]")
	}

	// GET: no confirmation
	app, srv = apiApp(t)
	srv.On("GET", "/x/y", http.StatusOK, []byte(`{"a":1}`))
	wantExit(t, app, runAPI(app, "GET", "/x/y", "-o", "json"), 0)
	wantRequests(t, srv, 1)

	// hard read-only mode: unknown POST blocked, GET allowed
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, runAPI(app, "PATCH", "/x/y", "--yes"), 9)
	wantRequests(t, srv, 0)
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	srv.On("GET", "/x/y", http.StatusOK, []byte(`{}`))
	wantExit(t, app, runAPI(app, "GET", "/x/y", "-o", "json"), 0)
	wantRequests(t, srv, 1)

	// archive is not destructive: no confirmation
	app, srv = apiApp(t)
	srv.On("POST", "/income/invoices/{id}/archive", http.StatusOK, nil)
	wantExit(t, app, runAPI(app, "POST", "/income/invoices/12/archive"), 0)
	wantRequests(t, srv, 1)
}

func TestAPIPaginateMergesPages(t *testing.T) {
	app, srv := apiApp(t)
	pagedUsers(srv, 3)
	wantExit(t, app, runAPI(app, "GET", "/users", "--paginate", "-o", "json"), 0)
	for i, r := range wantRequests(t, srv, 3) {
		if r.Query.Get("page") != string(rune('1'+i)) || r.Query.Get("page_size") != "100" {
			t.Errorf("request %d: query = %v", i, r.Query)
		}
	}
	// the same envelope as user list --all
	golden.Assert(t, "builder/user-list-all.json", []byte(mustOut(app)))

	// a page size of the path or a field wins
	for _, args := range [][]string{{"GET", "/users?page_size=50", "--paginate"}, {"GET", "/users", "-F", "page_size=50", "--paginate"}} {
		app, srv = apiApp(t)
		pagedUsers(srv, 2)
		wantExit(t, app, runAPI(app, append(args, "-o", "json")...), 0)
		for _, r := range wantRequests(t, srv, 2) {
			if r.Query.Get("page_size") != "50" {
				t.Errorf("%v: query = %v", args, r.Query)
			}
		}
	}

	// --max-pages caps like --all
	app, srv = apiApp(t)
	pagedUsers(srv, 3)
	wantExit(t, app, runAPI(app, "GET", "/users", "--paginate", "--max-pages", "2", "-o", "json"), 0)
	wantRequests(t, srv, 2)
	wantStderrHas(t, app, "--max-pages 2: 2 von 3 Seiten geladen")
	if out := mustOut(app); !strings.Contains(out, `"has_more": true`) {
		t.Errorf("stdout = %s", out)
	}

	// from 10 pages: warning with the credits
	app, srv = apiApp(t)
	pagedUsers(srv, 10)
	wantExit(t, app, runAPI(app, "GET", "/users", "--paginate", "-o", "json"), 0)
	wantRequests(t, srv, 10)
	wantStderrHas(t, app, "--paginate lädt 10 Seiten (10 Credits)")

	for _, args := range [][]string{
		{"POST", "/users", "--paginate", "--yes"},
		{"GET", "/info", "--paginate"},
		{"GET", "/x/liste", "--paginate"},
		{"GET", "/users?page=2", "--paginate"},
		{"GET", "/users", "-f", "page=2", "--paginate"},
		{"GET", "/users", "--max-pages", "2"},
		{"GET", "/users", "--paginate", "--max-pages", "0"},
	} {
		app, srv = apiApp(t)
		wantExit(t, app, runAPI(app, args...), 2)
		wantRequests(t, srv, 0)
	}
}

func TestAPIPathSafety(t *testing.T) {
	for _, path := range []string{
		"", "users", "https://evil/x", "http://evil/x", "//evil/x", "../x", "/../x", "/income/../x",
		"/income/%2e%2e/x", "/income/%2E./x", "/income/.%2E/x", "/income/%252e%252e/x",
		`\\evil/x`, `/income\x`, "/income/%5c/x", "/income/%2f/x", "/income/%2F/x",
		"/a/./b", "/a/%2e/b", "/a//b", "/x#y", "/x\x00", "/x\ny", "/x%0a", "/x\u0085", "/%zz", "/x?a=%zz", "/x?a=\x01",
	} {
		for _, method := range []string{"GET", "POST"} {
			app, srv := apiApp(t)
			wantExit(t, app, runAPI(app, method, path, "--yes"), 2)
			wantRequests(t, srv, 0)
		}
	}

	// the prefix /api/v1 is the base, not part of the path
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/api/v1/info", "-o", "json"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/info" {
		t.Errorf("path = %s", r.Path)
	}
	// percent-encoded letters are sent as the path they stand for, and
	// the class is that of this path
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "DELETE", "/income/%69nvoices/12"), 8)
	wantRequests(t, srv, 0)
}

func TestAPIMethodAndArgs(t *testing.T) {
	for _, args := range [][]string{
		{}, {"GET"}, {"GET", "/info", "extra"},
		{"HEAD", "/info"}, {"OPTIONS", "/info"}, {"TRACE", "/info"}, {"FOO", "/info"}, {"", "/info"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runAPI(app, args...), 2)
		wantRequests(t, srv, 0)
	}
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "get", "/info", "-o", "json"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
		t.Errorf("method = %s", r.Method)
	}
}

func TestAPIFieldTyping(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/x/echo", http.StatusNoContent, nil)
	file := writeFile(t, "text.txt", "Zeile 1\r\nZeile 2 – äöü\n")
	app.In = strings.NewReader("aus stdin\n")
	wantExit(t, app, runAPI(app, "POST", "/x/echo", "--yes",
		"-f", "a=true", "-F", "b=true", "-F", "c=false", "-F", "d=null", "-F", "e=19.99", "-F", "f=007",
		"-F", "g=@"+file, "-F", "s=@-", "-F", "h.i=x", "-F", "h.j=-1.50e3", "-f", "k=19.99", "-F", "l=Text mit = Zeichen",
		"-F", "m=", "-F", "n=9007199254740993", "-f", "o=@kein-datei",
	), 0)
	r := wantRequests(t, srv, 1)[0]
	want := `{"a":"true","b":true,"c":false,"d":null,"e":19.99,"f":"007","g":"Zeile 1\r\nZeile 2 – äöü\n","s":"aus stdin\n",` +
		`"h":{"i":"x","j":-1.50e3},"k":"19.99","l":"Text mit = Zeichen","m":"","n":9007199254740993,"o":"@kein-datei"}`
	if got, w := canonJSON(t, r.Body), canonJSON(t, []byte(want)); got != w {
		t.Errorf("body = %s\nerwartet %s", got, w)
	}
	for _, exact := range []string{`"e":19.99`, `"n":9007199254740993`, `"j":-1.50e3`} {
		if !bytes.Contains(r.Body, []byte(exact)) {
			t.Errorf("body ohne %s: %s", exact, r.Body)
		}
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	// against the spec: the number stays exact and the body is valid
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/expense/vouchers/7/pay", "--yes", "-o", "json",
		"-F", "value=19.99", "-f", "payment_date=2026-11-04", "-F", "banking_transaction.id=3"), 0)
	if r := wantRequests(t, srv, 1)[0]; !bytes.Contains(r.Body, []byte(`"value":19.99`)) {
		t.Errorf("body = %s", r.Body)
	}
}

func TestAPIFieldErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-f", "a=1", "-f", "a=2"},
		{"-f", "a=1", "-F", "a=2"},
		{"-f", "a=1", "-f", "a.b=2"},
		{"-f", "a.b=1", "-f", "a=2"},
		{"-f", "ohne-gleich"},
		{"-f", "=x"},
		{"-f", "a..b=1"},
		{"-f", ".a=1"},
		{"-f", "a.=1"},
		{"-F", "a=@"},
		{"-F", "a=@-", "-F", "b=@-"},
		{"--input", "-", "-F", "a=@-"},
		{"--input", ""},
		{"-F", "a=@/gibt/es/nicht.txt"},
		{"--input", "/gibt/es/nicht.json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runAPI(app, append([]string{"POST", "/x/echo", "--yes"}, args...)...), 2)
		wantRequests(t, srv, 0)
	}
	// GET has no body
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/info", "--input", writeFile(t, "b.json", `{}`)), 2)
	wantRequests(t, srv, 0)
	// a field may not repeat a query parameter of the path
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/users?page=2", "-f", "page=3"), 2)
	wantRequests(t, srv, 0)
	// --input must be a JSON object (exit 5)
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/x/echo", "--yes", "--input", writeFile(t, "a.json", `[1]`)), 5)
	wantRequests(t, srv, 0)
}

func TestAPIFieldsAsQuery(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader("name")
	wantExit(t, app, runAPI(app, "GET", "/users?page=2", "-f", "page_size=5", "-F", "order_direction=desc", "-F", "order_by=@-", "-o", "json"), 0)
	r := wantRequests(t, srv, 1)[0]
	if r.Query.Get("page") != "2" || r.Query.Get("page_size") != "5" || r.Query.Get("order_direction") != "desc" || r.Query.Get("order_by") != "name" {
		t.Errorf("query = %v", r.Query)
	}

	// DELETE without --input: query; with --input: body
	app, srv = apiApp(t)
	srv.On("DELETE", "/x/z", http.StatusNoContent, nil)
	wantExit(t, app, runAPI(app, "DELETE", "/x/z?a=1", "-F", "b=null", "-F", "c=2", "--yes"), 0)
	r = wantRequests(t, srv, 1)[0]
	if r.Query.Encode() != "a=1&b=&c=2" || len(r.Body) != 0 {
		t.Errorf("query = %v, body = %q", r.Query, r.Body)
	}
	app, srv = apiApp(t)
	srv.On("DELETE", "/x/z", http.StatusNoContent, nil)
	wantExit(t, app, runAPI(app, "DELETE", "/x/z", "--input", writeFile(t, "d.json", `{"a":1}`), "-F", "b=2", "--yes"), 0)
	wantBody(t, srv, "DELETE", "/x/z", `{"a":1,"b":2}`)
}

// --input is sent unpruned; -f/-F override single (also dotted) keys.
func TestAPIInputWithFields(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/x/echo", http.StatusOK, []byte(`{"ok":1}`))
	in := writeFile(t, "in.json", "\ufeff"+`{"name":"Alt","customer":{"id":1,"name":"Kunde"},"id":99,"budget_money":19.90,"created_at":"2026-01-01"}`)
	wantExit(t, app, runAPI(app, "POST", "/x/echo", "--input", in, "-f", "name=Neu", "-F", "customer.id=2", "--yes", "-o", "json"), 0)
	r := wantRequests(t, srv, 1)[0]
	want := `{"name":"Neu","customer":{"id":2,"name":"Kunde"},"id":99,"budget_money":19.90,"created_at":"2026-01-01"}`
	if got, w := canonJSON(t, r.Body), canonJSON(t, []byte(want)); got != w {
		t.Errorf("body = %s\nerwartet %s", got, w)
	}
	if !bytes.Contains(r.Body, []byte(`19.90`)) {
		t.Errorf("Zahl nicht exakt: %s", r.Body)
	}

	// --input - reads stdin
	app, srv = apiApp(t)
	srv.On("PUT", "/x/echo", http.StatusNoContent, nil)
	app.In = strings.NewReader(`{"a":"<b>&"}`)
	wantExit(t, app, runAPI(app, "PUT", "/x/echo", "--input", "-", "--yes"), 0)
	if r := wantRequests(t, srv, 1)[0]; string(r.Body) != `{"a":"<b>&"}` {
		t.Errorf("body = %s", r.Body)
	}
}

func TestAPIValidate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/projects", "-f", "name=P", "--validate"), 5)
	wantRequests(t, srv, 0)
	wantStderrHas(t, app, "customer")

	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/projects", "-f", "name=P", "-F", "customer.id=1", "--validate", "-o", "json"), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"P","customer":{"id":1}}`)

	// deliver without a body
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/income/invoices/12/deliver", "--validate", "--yes"), 5)
	wantRequests(t, srv, 0)

	// an operation without a request body takes none
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "DELETE", "/projects/5", "--input", writeFile(t, "x.json", `{"a":1}`), "--validate", "--yes"), 5)
	wantRequests(t, srv, 0)

	// unknown path: nothing to validate against
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/x/echo", "-f", "a=1", "--validate", "--yes"), 2)
	wantRequests(t, srv, 0)
}

func TestAPIDryRun(t *testing.T) {
	in := writeFile(t, "inv.json", `{"name":"Beratung","total":19.99}`)
	for _, tc := range []struct {
		args    []string
		url     string
		body    string
		credits string
	}{
		{[]string{"POST", "/income/invoices", "--input", in}, "/income/invoices", `{"name":"Beratung","total":19.99}`, "50"},
		{[]string{"POST", "/contact/companies", "-f", "name=ACME"}, "/contact/companies", `{"name":"ACME"}`, "10"},
		{[]string{"POST", "/x/companies", "-f", "a=1"}, "/x/companies", `{"a":"1"}`, "10"},
		{[]string{"POST", "/x/y", "-F", "a=1"}, "/x/y", `{"a":1}`, "1"},
		{[]string{"DELETE", "/income/invoices/12?x=1", "-f", "y=2"}, "/income/invoices/12?x=1&y=2", `null`, "1"},
		{[]string{"POST", "/income/invoices/12/deliver", "-F", "send_via=pdf"}, "/income/invoices/12/deliver", `{"send_via":"pdf"}`, "1"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runAPI(app, append(tc.args, "--dry-run", "-o", "json")...), 0)
		wantRequests(t, srv, 0)
		want := `{"dry_run":true,"method":"` + tc.args[0] + `","url":"` + srv.URL() + tc.url + `","body":` + tc.body + `,"credits":` + tc.credits + `}`
		if got, w := canonJSON(t, []byte(mustOut(app))), canonJSON(t, []byte(want)); got != w {
			t.Errorf("%v:\n%s\nerwartet %s", tc.args, got, w)
		}
		if !strings.Contains(mustOut(app), "19.99") && strings.Contains(tc.body, "19.99") {
			t.Errorf("Zahl nicht exakt: %s", mustOut(app))
		}
		assertNoToken(t, app)
	}

	// table: German lines
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/income/invoices", "--input", in, "--dry-run"), 0)
	wantRequests(t, srv, 0)
	for _, want := range []string{"Probelauf – nichts gesendet", "Methode: POST", "Credits: 50", `"total": 19.99`} {
		if !strings.Contains(mustOut(app), want) {
			t.Errorf("stdout ohne %q: %s", want, mustOut(app))
		}
	}

	// the URL never shows userinfo of the base URL
	app, srv = apiApp(t)
	withUser := strings.Replace(srv.URL(), "http://", "http://alice:s3cret@", 1)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": withUser, "PAPIERKRAM_TOKEN": "test-token-builder"})
	wantExit(t, app, runAPI(app, "POST", "/x/y", "--dry-run", "-o", "json"), 0)
	if out, errs := bufs(app); strings.Contains(out+errs, "s3cret") || strings.Contains(out+errs, "alice") || !strings.Contains(out, srv.URL()+"/x/y") {
		t.Errorf("stdout = %s, stderr = %s", out, errs)
	}

	// GET reads: --dry-run is a usage error
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/info", "--dry-run"), 2)
	wantRequests(t, srv, 0)
	wantStderrHas(t, app, "--dry-run")
}

func TestAPIBinaryResponse(t *testing.T) {
	// not a terminal: the bytes go to stdout unchanged
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/income/invoices/5/pdf"), 0)
	wantRequests(t, srv, 1)
	if out := stdoutBytes(app); !bytes.HasPrefix(out, []byte("%PDF-")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Errorf("stdout = %q", out)
	}

	// terminal: a known file answer is refused before the request
	app, srv = apiApp(t)
	app.StdoutTTY = true
	wantExit(t, app, runAPI(app, "GET", "/income/invoices/5/pdf"), 2)
	wantRequests(t, srv, 0)
	wantStderrHas(t, app, "papierkram invoice pdf 5 -O")

	png := []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 0xff}
	app, srv = apiApp(t)
	srv.On("GET", "/x/bild", http.StatusOK, png, "Content-Type", "image/png")
	wantExit(t, app, runAPI(app, "GET", "/x/bild"), 0)
	if out := stdoutBytes(app); !bytes.Equal(out, png) {
		t.Errorf("stdout = %q", out)
	}

	// unknown answer in a terminal: the request was sent, so even GET is
	// exit 1, never 2 (global rule), with the hint to redirect
	app, srv = apiApp(t)
	app.StdoutTTY = true
	srv.On("GET", "/x/bild", http.StatusOK, png, "Content-Type", "image/png")
	wantExit(t, app, runAPI(app, "GET", "/x/bild"), 1)
	wantRequests(t, srv, 1)
	if out := stdoutBytes(app); len(out) != 0 {
		t.Errorf("stdout = %q", out)
	}
	wantStderrHas(t, app, "Befehl ausgeführt", "kein JSON", "umleiten", "pdf <id> -O")

	// a mutation was executed: exit 1, never 2
	app, srv = apiApp(t)
	app.StdoutTTY = true
	srv.On("POST", "/x/bild", http.StatusOK, png, "Content-Type", "image/png")
	wantExit(t, app, runAPI(app, "POST", "/x/bild", "--yes"), 1)
	wantRequests(t, srv, 1)
	wantStderrHas(t, app, "Befehl ausgeführt", "umleiten", "pdf <id> -O")

	// --jq cannot apply to a file
	app, srv = apiApp(t)
	srv.On("GET", "/x/bild", http.StatusOK, png, "Content-Type", "image/png")
	wantExit(t, app, runAPI(app, "GET", "/x/bild", "--jq", ".a"), 1)
	if out := stdoutBytes(app); len(out) != 0 {
		t.Errorf("stdout = %q", out)
	}
}

// --debug shows the body of --input (buffered), never the token.
func TestAPIDebugShowsBody(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/x/echo", http.StatusNoContent, nil)
	in := writeFile(t, "d.json", `{"name":"Debug-Körper"}`)
	wantExit(t, app, runAPI(app, "POST", "/x/echo", "--input", in, "--yes", "--debug"), 0)
	wantRequests(t, srv, 1)
	wantStderrHas(t, app, "> POST "+srv.URL()+"/x/echo", `> {"name":"Debug-Körper"}`, "Bearer ***")
	assertNoToken(t, app)
}

// The JSON answer of a known list renders with the columns of its command.
func TestAPITableUsesColumnsOfCommand(t *testing.T) {
	app, srv := apiApp(t)
	pagedUsers(srv, 1) // echoes page_size
	wantExit(t, app, runAPI(app, "GET", "/users?page_size=10"), 0)
	if out := mustOut(app); !strings.HasPrefix(out, "ID") || !strings.Contains(out, "9007199254740991") {
		t.Errorf("stdout = %q", out)
	}
	app, srv = apiApp(t)
	pagedUsers(srv, 1)
	wantExit(t, app, runAPI(app, "GET", "/users?page_size=10", "--fields", "id", "-o", "json"), 0)
	if out := mustOut(app); strings.Contains(out, "email") {
		t.Errorf("stdout = %q", out)
	}
}

// A failed deliver keeps the hint of the deliver command: a 5xx does not
// prove that no mail went out.
func TestAPIDeliverServerErrorHint(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/income/invoices/12/deliver", http.StatusInternalServerError,
		[]byte(`{"type":"error","error_type":"server_error","message":"Fehler","correlation_id":"c-1"}`))
	wantExit(t, app, runAPI(app, "POST", "/income/invoices/12/deliver", "-F", "send_via=pdf", "--yes"), 1)
	wantRequests(t, srv, 1)
	wantStderrHas(t, app, "der Versand kann trotzdem erfolgt sein", "papierkram invoice describe 12")
}

// Review I1: the deliver rule cannot be bypassed through the query, wrong
// types or a missing send_via. A send-class operation takes no query
// (exit 2), its body is always validated against the request schema like
// the deliver command (exit 5), and send_via must be set.
func TestAPIDeliverRuleBypasses(t *testing.T) {
	for name, tc := range map[string]struct {
		path string
		args []string
		exit int
		want string
	}{
		"Query send_via":           {"/income/invoices/12/deliver?send_via=email", nil, 2, "keine Query"},
		"Query und Body":           {"/income/invoices/12/deliver?send_via=email", []string{"-f", "email.recipient=a@example.org"}, 2, "keine Query"},
		"Query mit pdf":            {"/income/invoices/12/deliver?x=1", []string{"-F", "send_via=pdf"}, 2, "keine Query"},
		"email ist Text":           {"/income/invoices/12/deliver", []string{"-F", "send_via=email", "-f", "email=foo"}, 5, "email"},
		"Felder keine Texte":       {"/income/invoices/12/deliver", []string{"-F", "send_via=email", "-F", "email.recipient=1", "-F", "email.subject=true", "-F", "email.body=2"}, 5, "email."},
		"ohne Body":                {"/income/invoices/12/deliver", nil, 5, "send_via"},
		"send_via null":            {"/income/invoices/12/deliver", []string{"-F", "send_via=null"}, 5, "send_via"},
		"send_via fehlt":           {"/income/estimates/12/deliver", []string{"-f", "email.recipient=a@example.org", "-f", "email.subject=S", "-f", "email.body=B"}, 5, "send_via"},
		"unbekanntes Feld":         {"/income/invoices/12/deliver", []string{"-F", "send_via=pdf", "-f", "bogus=1"}, 5, "bogus"},
		"ungültiger Wert send_via": {"/income/invoices/12/deliver", []string{"-f", "send_via=fax"}, 5, "send_via"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runAPI(app, append([]string{"POST", tc.path, "--yes"}, tc.args...)...), tc.exit)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%s: stderr ohne %q: %q", name, tc.want, errs)
		}
	}
}

// Review I2: a path the server may route to a spec operation although it
// is not that operation's exact form is refused (exit 2, 0 requests):
// segments hold only letters, digits, _ and -, and a path that matches a
// spec operation only without case or with a non-digit ID is ambiguous.
func TestAPIAmbiguousPaths(t *testing.T) {
	for _, tc := range []struct{ method, path, want string }{
		{"POST", "/income/invoices/12-x/deliver", "Pfad nicht eindeutig: gemeint ist /income/invoices/12/deliver"},
		{"POST", "/income/invoices/12/Deliver", "Pfad nicht eindeutig: gemeint ist /income/invoices/12/deliver"},
		{"POST", "/income/invoices/12/deliver.json", "nur Buchstaben, Ziffern, _ und -"},
		{"POST", "/income/invoices/12;x/deliver", "nur Buchstaben, Ziffern, _ und -"},
		{"POST", "/income/invoices/12%3Bx/deliver", "nur Buchstaben, Ziffern, _ und -"},
		{"DELETE", "/income/invoices/abc", "Pfad nicht eindeutig: gemeint ist /income/invoices/{id}"},
		{"GET", "/Income/Invoices", "Pfad nicht eindeutig: gemeint ist /income/invoices"},
		{"GET", "/income/invoices/1e3", "Pfad nicht eindeutig: gemeint ist /income/invoices/1"},
		{"GET", "/x/a%23b", "nur Buchstaben"},
		{"GET", "/x/a%2541", "nur Buchstaben"},
		{"GET", "/x/a+b", "nur Buchstaben"},
		{"GET", "/x/a~b", "nur Buchstaben"},
		{"GET", "/x/%C3%A4", "nur Buchstaben"},
		{"GET", "/x/ä", "nur Buchstaben"},
		{"GET", "/x/a%00b", "Steuerzeichen"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, runAPI(app, tc.method, tc.path, "-F", "send_via=email", "--yes"), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%s %s: stderr ohne %q: %q", tc.method, tc.path, tc.want, errs)
		}
	}
	// a path outside the spec with allowed characters stays unknown
	app, srv := apiApp(t)
	srv.On("GET", "/x_1/A-b", http.StatusOK, []byte(`{}`))
	wantExit(t, app, runAPI(app, "GET", "/x_1/A-b", "-o", "json"), 0)
	wantRequests(t, srv, 1)
}

// Review M4: a trailing slash is sent as given and classified like the
// path without it; %-encoded letters are sent decoded.
func TestAPITrailingSlashAndEncodedLetters(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, runAPI(app, "DELETE", "/income/invoices/12/", "--dry-run", "-o", "json"), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, `"url": "`+srv.URL()+`/income/invoices/12/"`) {
		t.Errorf("stdout = %s", out)
	}
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "DELETE", "/income/invoices/12/"), 8) // destructive like delete
	wantRequests(t, srv, 0)
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "POST", "/income/invoices/12/deliver/", "-F", "send_via=email", "--yes"), 5)
	wantRequests(t, srv, 0)
	app, srv = apiApp(t)
	wantExit(t, app, runAPI(app, "GET", "/%69nfo", "-o", "json"), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/info" {
		t.Errorf("path = %s", r.Path)
	}
}
