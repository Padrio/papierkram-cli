package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// voucher pay <id> (spec 9.5, 8.1: class destructive).

const payPath = "/expense/vouchers/295/pay"

// Spec 9.5: --amount → value, --transaction → banking_transaction:{id},
// --date → payment_date, --reason → difference_reason.
func TestVoucherPayBody(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		path, body string
	}{
		{[]string{"295", "--amount", "19.99"}, payPath, `{"value":19.99}`},
		{[]string{"295", "--transaction", "457"}, payPath, `{"banking_transaction":{"id":457}}`},
		{[]string{"295", "--amount", "150", "--date", "2026-11-04", "--reason", "mahnung"}, payPath,
			`{"value":150,"payment_date":"2026-11-04","difference_reason":"mahnung"}`},
		{[]string{"295", "--transaction", "457", "--reason", "skonto", "--date", "2030-05-01"}, payPath,
			`{"banking_transaction":{"id":457},"difference_reason":"skonto","payment_date":"2030-05-01"}`},
		// R24: ID 0 is sent, as voucher and as transaction
		{[]string{"0", "--transaction", "0"}, "/expense/vouchers/0/pay", `{"banking_transaction":{"id":0}}`},
		{[]string{"295", "--amount", "0"}, payPath, `{"value":0}`},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(append([]string{"voucher", "pay"}, tc.args...), "--yes", "-o", "json")), 0)
		wantBody(t, srv, "POST", tc.path, tc.body)
		assertNoToken(t, app)
	}

	// decimals are sent as typed, never through float64
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "165.80", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; strings.TrimSpace(string(r.Body)) != `{"value":165.80}` {
		t.Errorf("body = %s", r.Body)
	}
}

// --date takes the relative dates of spec 6.1 on the day of App.Location.
func TestVoucherPayRelativeDate(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for word, day := range map[string]string{"heute": "2026-11-04", "today": "2026-11-04", "gestern": "2026-11-03", "yesterday": "2026-11-03"} {
		app, srv := apiApp(t)
		app.Location = loc
		// 23:30 UTC on 2026-11-03 is already 2026-11-04 in Berlin
		app.Now = func() time.Time { return time.Date(2026, 11, 3, 23, 30, 0, 0, time.UTC) }
		wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "10", "--date", word, "--yes", "-o", "json"}), 0)
		wantBody(t, srv, "POST", payPath, `{"value":10,"payment_date":"`+day+`"}`)
	}
}

// Spec 9.5: without --amount and --transaction nothing is sent (exit 2),
// before the configuration and the read-only gate.
func TestVoucherPayWithoutAmountOrTransaction(t *testing.T) {
	for _, args := range [][]string{
		{"voucher", "pay", "295", "--yes"},
		{"voucher", "pay", "295"},
		{"voucher", "pay", "295", "--date", "heute", "--reason", "skonto", "--yes"},
		{"voucher", "pay", "295", "--dry-run"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--amount") || !strings.Contains(errs, "--transaction") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--yes"}), 2)
	wantRequests(t, srv, 0)
}

func TestVoucherPayInvalidArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
		want string
	}{
		{[]string{"abc", "--amount", "1"}, 2, `ungültige ID "abc"`},
		{[]string{"-1", "--amount", "1"}, 2, ""},
		{[]string{"--amount", "1"}, 2, ""},
		{[]string{"295", "296", "--amount", "1"}, 2, ""},
		{[]string{"295", "--amount", "19,99"}, 5, "Dezimaltrennzeichen ist der Punkt"},
		{[]string{"295", "--amount", "viel"}, 5, `ungültiger Wert "viel" für --amount`},
		{[]string{"295", "--amount", ""}, 5, "--amount"},
		{[]string{"295", "--transaction", "abc"}, 5, `ungültiger Wert "abc" für --transaction`},
		{[]string{"295", "--transaction", "-1"}, 5, "--transaction"},
		{[]string{"295", "--amount", "1", "--date", "04.11.2026"}, 5, `ungültiger Wert "04.11.2026" für --date`},
		{[]string{"295", "--amount", "1", "--reason", "skonte"}, 5, `ungültiger Wert "skonte" für --reason – meintest du "skonto"?`},
		{[]string{"295", "--amount", "1", "--reason", "Skonto"}, 5, `"skonto"`},
		{[]string{"295", "--amount", "1", "--input", ""}, 2, "--input"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(append([]string{"voucher", "pay"}, tc.args...), "--yes")), tc.exit)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%v: stderr = %q, erwartet %q", tc.args, errs, tc.want)
		}
	}
	// the allowed reasons come from the spec enum
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "1", "--reason", "rabatt", "--yes"}), 5)
	if _, errs := bufs(app); !strings.Contains(errs, "erlaubt: sonstige, mahnung, teilzahlung, skonto, schmaelerung") {
		t.Errorf("stderr = %q", errs)
	}
}

// Spec 8.1/8.2: pay is destructive; without a terminal or with --no-input
// and without --yes it is exit 8 before anything is sent.
func TestVoucherPayNeedsConfirmation(t *testing.T) {
	for _, extra := range [][]string{nil, {"--no-input"}} {
		app, srv := apiApp(t)
		app.StdinTTY = extra != nil
		wantExit(t, app, Run(app, append([]string{"voucher", "pay", "295", "--amount", "19.99"}, extra...)), 8)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "Bestätigung erforderlich: Beleg 295 als bezahlt buchen") || !strings.Contains(errs, "--yes") {
			t.Errorf("%v: stderr = %q", extra, errs)
		}
	}
}

// Spec 8.2: in a terminal the confirmation names the voucher (summary GET
// of the generic mechanism) and the payment from the body.
func TestVoucherPayConfirmationSummary(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("ja\n")
	srv.On("GET", "/expense/vouchers/295", http.StatusOK, []byte(`{"id":295,"name":"Büromaterial","amount":165.80}`))
	srv.On("POST", payPath, http.StatusOK, nil)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "150", "--transaction", "457",
		"--date", "2026-11-04", "--reason", "skonto", "-o", "json"}), 0)
	reqs := wantRequests(t, srv, 2) // summary, pay
	if reqs[0].Method != "GET" || reqs[1].Method != "POST" || reqs[1].Path != payPath {
		t.Errorf("requests = %+v", reqs)
	}
	_, errs := bufs(app)
	for _, want := range []string{
		"Zahlung:   150,00 €\n", "Umsatz:    457\n", "Datum:     2026-11-04\n", "Grund:     skonto\n",
		"Beleg 295 „Büromaterial“ (165,80 €) wirklich als bezahlt buchen? [ja/nein]",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q:\n%s", want, errs)
		}
	}

	// only what is given is shown
	app, srv = apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("ja\n")
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--transaction", "457", "-o", "json"}), 0)
	wantRequests(t, srv, 2)
	if _, errs := bufs(app); !strings.Contains(errs, "Umsatz:    457") || strings.Contains(errs, "Zahlung:") || strings.Contains(errs, "Grund:") {
		t.Errorf("stderr = %q", errs)
	}

	// declined: only the summary was read
	app, srv = apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("nein\n")
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "150"}), 8)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
}

// Spec 8.4: --dry-run validates, shows the request and sends nothing.
func TestVoucherPayDryRun(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "19.99", "--transaction", "457", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	out := mustOut(app)
	for _, want := range []string{`"dry_run": true`, `"method": "POST"`, payPath + `"`, `"credits": 1`, `"value": 19.99`, `"id": 457`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout ohne %q:\n%s", want, out)
		}
	}
	// invalid values fail the dry run too
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "19,99", "--dry-run"}), 5)
	wantRequests(t, srv, 0)
}

// Spec 8.3: the read-only gate comes before the body is read or checked.
func TestVoucherPayReadOnly(t *testing.T) {
	missing := t.TempDir() + "/fehlt.json"
	for _, args := range [][]string{
		{"voucher", "pay", "295", "--amount", "19.99", "--yes"},
		{"voucher", "pay", "295", "--amount", "19,99", "--yes"},
		{"voucher", "pay", "295", "--transaction", "457", "--dry-run"},
		{"voucher", "pay", "295", "--input", missing, "--yes"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
	}
}

// --input like create (spec 6.3): flags override its fields; the merged
// body needs value or banking_transaction (exit 5) and is validated.
func TestVoucherPayInput(t *testing.T) {
	for _, tc := range []struct {
		in   string
		args []string
		body string
	}{
		{`{"value":100.0,"payment_date":"2030-05-01"}`, nil, `{"value":100.0,"payment_date":"2030-05-01"}`},
		{`{"value":"19.99"}`, nil, `{"value":19.99}`},
		{"\ufeff{\"value\":10,\"difference_reason\":\"teilzahlung\"}", []string{"--amount", "12.50"}, `{"value":12.50,"difference_reason":"teilzahlung"}`},
		{`{"payment_date":"2030-05-01"}`, []string{"--transaction", "457"}, `{"banking_transaction":{"id":457},"payment_date":"2030-05-01"}`},
		{`{"value":null,"banking_transaction":{"id":471}}`, nil, `{"value":null,"banking_transaction":{"id":471}}`},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.in)
		wantExit(t, app, Run(app, append([]string{"voucher", "pay", "295", "--input", "-", "--yes", "-o", "json"}, tc.args...)), 0)
		wantBody(t, srv, "POST", payPath, tc.body)
	}

	for _, tc := range []struct {
		in, want string
	}{
		{`{"payment_date":"2030-05-01"}`, "Hinweis: in --input value oder banking_transaction.id setzen oder --amount bzw. --transaction angeben\n"},
		{`{"value":null}`, "--amount"},
		{`{"banking_transaction":{}}`, "--transaction"},
		{`{"amount":5}`, `unbekanntes Feld "amount"`},
		{`{"value":5,"difference_reason":"rabatt"}`, "difference_reason"},
		{`{"value":"viel"}`, "value"},
		{`[1]`, "JSON-Objekt"},
	} {
		app, srv := apiApp(t)
		app.In = strings.NewReader(tc.in)
		wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--input", "-", "--yes"}), 5)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("%s: stderr = %q, erwartet %q", tc.in, errs, tc.want)
		}
	}
}

// Spec 10.3: the API object is rendered; an empty 2xx answer is success
// with {"id":N,"ok":true} (R03).
func TestVoucherPayOutput(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "100", "--yes", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	if out := mustOut(app); !strings.Contains(out, `"type": "expense_voucher"`) || !strings.Contains(out, `"amount": 100.0`) {
		t.Errorf("stdout = %s", out)
	}

	// table: the detail columns of a voucher
	app, srv = apiApp(t)
	srv.On("POST", payPath, http.StatusOK, []byte(`{"id":295,"voucher_no":"B-00012","name":"Büromaterial","state":"paid","amount":165.80}`))
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "165.80", "--yes"}), 0)
	if out := mustOut(app); !strings.Contains(out, "B-00012") || !strings.Contains(out, "paid") {
		t.Errorf("stdout = %q", out)
	}

	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"-o", "json"}, "{\n  \"id\": 295,\n  \"ok\": true\n}\n"},
		{nil, "voucher 295: Zahlung erfasst.\n"},
	} {
		app, srv := apiApp(t)
		srv.On("POST", payPath, http.StatusOK, nil)
		wantExit(t, app, Run(app, append([]string{"voucher", "pay", "295", "--amount", "100", "--yes"}, tc.flags...)), 0)
		if out, errs := bufs(app); out != tc.want || errs != "" {
			t.Errorf("%v: stdout = %q, stderr = %q", tc.flags, out, errs)
		}
	}
}

// A 5xx after the POST is never retried and tells to check before
// repeating; a 422 keeps the message of the API.
func TestVoucherPayErrors(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", payPath, http.StatusInternalServerError, []byte(`{"type":"error","message":"kaputt"}`))
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "100", "--yes"}), 1)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "vor Wiederholung per describe/list prüfen") {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	srv.On("POST", payPath, http.StatusUnprocessableEntity,
		[]byte(`{"type":"error","error_type":"unprocessable_entity","message":"Difference reason ist kein gültiger Wert"}`))
	wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "90", "--yes"}), 5)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "Difference reason ist kein gültiger Wert") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestVoucherPayHelp(t *testing.T) {
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"voucher", "pay", "--help"}), 0)
	out := mustOut(app)
	for _, want := range []string{
		"pay <id>",
		"--amount Dezimalzahl",
		"--transaction ID",
		"--date Datum",
		"(YYYY-MM-DD, heute, gestern)",
		"--reason Text",
		"(erlaubt: sonstige, mahnung, teilzahlung, skonto, schmaelerung)",
		"--input",
		"Kosten: 1 Credit",
		"--yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Hilfe ohne %q:\n%s", want, out)
		}
	}
}

// Like the generic create/update (runWriteThen: newWriteSpec before the
// flag checks and the read-only gate), a request schema without the pay
// fields makes the command unavailable (exit 1) before anything else.
func TestVoucherPayUnavailableFirst(t *testing.T) {
	// an operation whose request schema lacks value, banking_transaction, …
	e := specEntry(t, "DeliverInvoice", "invoice", "pay", "destructive")
	for _, tc := range []struct {
		env  map[string]string
		args []string
	}{
		{map[string]string{"PAPIERKRAM_TOKEN": "test-token-pay"}, []string{"invoice", "pay", "1", "--yes"}},
		{map[string]string{"PAPIERKRAM_TOKEN": "test-token-pay", "PAPIERKRAM_READ_ONLY": "1"}, []string{"invoice", "pay", "1", "--amount", "1", "--yes"}},
	} {
		app, srv := mockApp(t, tc.env)
		root := NewRootCmd(app)
		group, _, err := root.Find([]string{"invoice"})
		if err != nil {
			t.Fatal(err)
		}
		group.AddCommand(app.newPayCmd(e))
		wantExit(t, app, executeContext(context.Background(), app, root, tc.args), 1)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "Befehl nicht verfügbar") {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}
