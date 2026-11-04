package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// invoice|estimate deliver (spec 9.1, 10.4).

// sentDoc is the readback of a delivered invoice or estimate; sentOn is
// raw JSON ("null" or a quoted date).
func sentDoc(resource string, id int, sentOn string) []byte {
	return fmt.Appendf(nil, `{"id":%d,"type":"%s","%s_no":"RE-1001","name":"Beratung","state":"unpaid",`+
		`"sent_on":%s,"sent_via":"email","sent_to":"kunde@example.org","total_gross":1190.00}`, id, resource, resource, sentOn)
}

// deliverPaths are the deliver and readback paths of resource and id.
func deliverPaths(resource, id string) (post, get string) {
	get = "/income/" + resource + "s/" + id
	return get + "/deliver", get
}

func TestR01_DeliverEmptyBody(t *testing.T) {
	for _, resource := range []string{"invoice", "estimate"} {
		app, srv := apiApp(t)
		post, get := deliverPaths(resource, "459")
		srv.On("POST", post, http.StatusOK, nil) // live: 2xx with empty body
		srv.On("GET", get, http.StatusOK, sentDoc(resource, 459, `"2026-11-04"`))
		wantExit(t, app, Run(app, []string{resource, "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}), 0)
		reqs := wantRequests(t, srv, 2)
		if reqs[0].Method != "POST" || reqs[0].Path != post || canonJSON(t, reqs[0].Body) != `{"send_via":"pdf"}` {
			t.Errorf("%s: deliver = %s %s %s", resource, reqs[0].Method, reqs[0].Path, reqs[0].Body)
		}
		if reqs[1].Method != "GET" || reqs[1].Path != get {
			t.Errorf("%s: readback = %s %s", resource, reqs[1].Method, reqs[1].Path)
		}
		out, errs := bufs(app)
		want := fmt.Sprintf("{\n  \"id\": 459,\n  \"%s_no\": \"RE-1001\",\n  \"state\": \"unpaid\",\n  \"sent_on\": \"2026-11-04\",\n"+
			"  \"sent_via\": \"email\",\n  \"sent_to\": \"kunde@example.org\"\n}\n", resource)
		if out != want || errs != "" {
			t.Errorf("%s: stdout = %q, stderr = %q", resource, out, errs)
		}
		assertNoToken(t, app)
	}
}

func TestR02_DeliverReadbackFails(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
	srv.On("GET", "/income/invoices/459", http.StatusInternalServerError, []byte(`{"type":"error","message":"kaputt"}`))
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json"}), 0)
	posts := 0
	for _, r := range srv.Requests() {
		if r.Method == "POST" {
			posts++
		}
	}
	if posts != 1 {
		t.Errorf("%d POST, erwartet genau 1 (nie wiederholt)", posts)
	}
	out, errs := bufs(app)
	warning := "Versand erfolgreich, Nachlesen fehlgeschlagen: kaputt – NICHT erneut ausführen"
	if want := "{\n  \"id\": 459,\n  \"delivered\": true,\n  \"warnings\": [\n    \"" + warning + "\"\n  ]\n}\n"; out != want {
		t.Errorf("stdout = %q, erwartet %q", out, want)
	}
	if !strings.Contains(errs, "Warnung: "+warning) {
		t.Errorf("stderr = %q", errs)
	}

	// table: a German sentence, the warning on stderr
	app, srv = apiApp(t)
	srv.On("POST", "/income/estimates/{id}/deliver", http.StatusOK, nil)
	srv.On("GET", "/income/estimates/{id}", http.StatusNotFound, nil)
	wantExit(t, app, Run(app, []string{"estimate", "deliver", "7", "--via", "pdf", "--yes"}), 0)
	out, errs = bufs(app)
	if out != "estimate 7 versendet.\n" || !strings.Contains(errs, "NICHT erneut ausführen") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
}

func TestR04_DeliverWithoutVia(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "deliver", "459", "--yes"},
		{"estimate", "deliver", "349", "--yes"},
		{"invoice", "deliver", "459", "--to", "kunde@example.org", "--subject", "S", "--body", "B", "--yes"},
		{"invoice", "deliver", "459", "--dry-run"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--via") || !strings.Contains(errs, "email") || !strings.Contains(errs, "pdf") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
}

func TestR05_DeliverEmailMissingFields(t *testing.T) {
	full := map[string][]string{"--to": {"--to", "kunde@example.org"}, "--subject": {"--subject", "Rechnung"}, "--body": {"--body", "Hallo"}}
	for _, missing := range []string{"--to", "--subject", "--body"} {
		args := []string{"invoice", "deliver", "459", "--via", "email", "--yes"}
		for flag, kv := range full {
			if flag != missing {
				args = append(args, kv...)
			}
		}
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, missing) {
			t.Errorf("ohne %s: stderr = %q", missing, errs)
		}
	}

	// an empty value counts as missing; all missing flags are named
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "deliver", "349", "--via", "email", "--to", " ", "--yes"}), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--to, --subject, --body") {
		t.Errorf("stderr = %q", errs)
	}

	// --input with send_via email but without the mail: never the silent
	// default mail of the API (spec 4.5)
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"send_via":"email"}`)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--input", "-", "--yes"}), 2)
	wantRequests(t, srv, 0)
}

func TestR06_DeliverInputAddressInsteadOfRecipient(t *testing.T) {
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"send_via":"email","email":{"address":"kunde@example.org","subject":"Rechnung","body":"Hallo"}}`)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--input", "-", "--yes"}), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `unbekanntes Feld "email.address" – meintest du "recipient"?`) {
		t.Errorf("stderr = %q", errs)
	}
}

// The readback counts as "sent" only with a sent_on of today in
// App.Location (berlinApp: 2026-10-26 00:30, the day after the end of
// DST); an older sent_on is an earlier delivery, not this one.
func TestR37_DeliverTimeoutReadbackShowsSent(t *testing.T) {
	cases := []struct {
		name, sentOn string
		readback     int
		exit         int
		hint         string
	}{
		{"heute versendet", `"2026-10-26"`, http.StatusOK, 0, ""},
		{"heute, Datum mit Uhrzeit UTC", `"2026-10-25 23:45:00 +0000"`, http.StatusOK, 0, ""},
		{"heute, RFC 3339", `"2026-10-26T00:10:00+01:00"`, http.StatusOK, 0, ""},
		{"früher versendet", `"2026-10-25"`, http.StatusOK, 12, "zuletzt versendet am 2026-10-25"},
		{"gestern, Datum mit Uhrzeit UTC", `"2026-10-25 22:00:00 +0000"`, http.StatusOK, 12, "zuletzt versendet am 2026-10-25"},
		{"nicht versendet", "null", http.StatusOK, 12, "sent_on leer"},
		{"Nachlesen scheitert", "", http.StatusNotFound, 12, "Nachlesen fehlgeschlagen"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := berlinApp(t)
			srv.OnFunc("POST", "/income/invoices/459/deliver", func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done() // request fully read, no answer until the client gives up
			})
			if tc.readback == http.StatusOK {
				srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, tc.sentOn))
			} else {
				srv.On("GET", "/income/invoices/459", tc.readback, nil)
			}
			code := waitExit(t, func() int {
				return Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "kunde@example.org",
					"--subject", "Rechnung", "--body", "Hallo", "--yes", "--timeout", "300ms", "-o", "json"})
			})
			wantExit(t, app, code, tc.exit)
			reqs := srv.Requests()
			if len(reqs) != 2 || reqs[0].Method != "POST" || reqs[1].Method != "GET" {
				t.Fatalf("requests = %+v", reqs)
			}
			out, errs := bufs(app)
			if tc.exit == 0 {
				if !strings.Contains(out, `"sent_on": `+tc.sentOn) || !strings.Contains(out, `"warnings": [`) ||
					!strings.Contains(errs, "Warnung: ") || !strings.Contains(errs, "NICHT erneut ausführen") {
					t.Errorf("stdout = %s\nstderr = %s", out, errs)
				}
				return
			}
			if out != "" || !strings.Contains(errs, "vor Wiederholung per describe prüfen") || !strings.Contains(errs, tc.hint) ||
				!strings.Contains(errs, "papierkram invoice describe 459") || !strings.Contains(errs, `"exit_code":12`) {
				t.Errorf("stdout = %q\nstderr = %s", out, errs)
			}
		})
	}
}

func TestDeliverEmail(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "deliver", "459"},
		{"invoice", "send", "459"},
		{"rechnung", "send", "459"},
	} {
		app, srv := apiApp(t)
		srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
		srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
		wantExit(t, app, Run(app, append(args, "--via", "email", "--to", "kunde@example.org",
			"--subject", "Rechnung {{rechnung.rechnungsnummer}}", "--body", "Guten Tag,\nanbei die Rechnung.", "--yes", "-o", "json")), 0)
		r := wantRequests(t, srv, 2)[0]
		want := `{"email":{"body":"Guten Tag,\nanbei die Rechnung.","recipient":"kunde@example.org",` +
			`"subject":"Rechnung {{rechnung.rechnungsnummer}}"},"send_via":"email"}`
		if got := canonJSON(t, r.Body); got != want {
			t.Errorf("%v: body = %s", args, got)
		}
	}
}

// Review focus 3: a mail text from a Windows editor (UTF-8 BOM, CRLF) is
// sent without the BOM and with its line breaks as they are.
func TestDeliverBodyFile(t *testing.T) {
	text := "Guten Tag,\r\n\r\nanbei die Rechnung.\nGrüße\r\n"
	for _, viaStdin := range []bool{false, true} {
		app, srv := apiApp(t)
		srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
		srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
		path := writeFile(t, "mail.txt", "\uFEFF"+text)
		if viaStdin {
			app.In = strings.NewReader("\uFEFF" + text)
			path = "-"
		}
		wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "kunde@example.org",
			"--subject", "Rechnung", "--body-file", path, "--yes", "-o", "json"}), 0)
		body := decodeBody(t, wantRequests(t, srv, 2)[0].Body)
		if got := body["email"].(map[string]any)["body"]; got != text {
			t.Errorf("stdin=%t: body = %q, erwartet %q", viaStdin, got, text)
		}
	}

	// not UTF-8 (Windows-1252 ü): exit 5, nothing sent
	app, srv := apiApp(t)
	path := writeFile(t, "mail.txt", "Gr\xfc\xdfe")
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org",
		"--subject", "S", "--body-file", path, "--yes"}), 5)
	wantRequests(t, srv, 0)

	// a missing file is a usage error, nothing sent
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org",
		"--subject", "S", "--body-file", "/gibt/es/nicht.txt", "--yes"}), 2)
	wantRequests(t, srv, 0)
}

func TestDeliverFlagConflicts(t *testing.T) {
	mail := []string{"--to", "k@example.org", "--subject", "S"}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{append([]string{"--via", "email", "--body", "B", "--body-file", "mail.txt"}, mail...), "--body-file"},
		{[]string{"--via", "pdf", "--to", "k@example.org"}, "--via pdf"},
		{[]string{"--via", "pdf", "--body-file", "mail.txt"}, "--via pdf"},
		{[]string{"--via", "fax"}, "email, pdf"},
		{append([]string{"--via", "email", "--input", "-", "--body-file", "-"}, mail...), "Standardeingabe"},
		{[]string{"--input", ""}, "--input"},
		{append([]string{"--via", "email", "--body-file", ""}, mail...), "--body-file"},
	} {
		app, srv := apiApp(t)
		in := &countingReader{r: strings.NewReader(`{"send_via":"pdf"}`)}
		app.In = in
		wantExit(t, app, Run(app, append([]string{"invoice", "deliver", "459", "--yes"}, tc.args...)), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) || in.n != 0 {
			t.Errorf("%v: stderr = %q, %d Reads", tc.args, errs, in.n)
		}
	}

	// --via pdf with a mail from --input: no mail fields for pdf either
	app, srv := apiApp(t)
	app.In = strings.NewReader(`{"send_via":"email","email":{"recipient":"k@example.org","subject":"S","body":"B"}}`)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--input", "-", "--via", "pdf", "--yes"}), 2)
	wantRequests(t, srv, 0)
}

// --input supplies the body like on create; flags override its fields.
func TestDeliverInputMerge(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("POST", "/income/estimates/349/deliver", http.StatusOK, nil)
	srv.On("GET", "/income/estimates/349", http.StatusOK, sentDoc("estimate", 349, `"2026-11-04"`))
	app.In = strings.NewReader("\uFEFF{\"send_via\":\"email\",\"email\":{\"recipient\":\"alt@example.org\",\"subject\":\"Angebot\",\"body\":\"Text\"}}")
	wantExit(t, app, Run(app, []string{"estimate", "deliver", "349", "--input", "-", "--to", "neu@example.org", "--yes", "-o", "json"}), 0)
	r := wantRequests(t, srv, 2)[0]
	if got := canonJSON(t, r.Body); got != `{"email":{"body":"Text","recipient":"neu@example.org","subject":"Angebot"},"send_via":"email"}` {
		t.Errorf("body = %s", got)
	}

	// an unknown send_via from --input is a validation error (exit 5)
	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"send_via":"fax"}`)
	wantExit(t, app, Run(app, []string{"estimate", "deliver", "349", "--input", "-", "--yes"}), 5)
	wantRequests(t, srv, 0)
}

func TestDeliverDryRun(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org",
		"--subject", "S", "--body", "B", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	out := mustOut(app)
	// credits: the deliver and the readback (spec 8.4: the cost of the run)
	for _, want := range []string{`"dry_run": true`, `"method": "POST"`, `/income/invoices/459/deliver"`, `"credits": 2`, `"recipient": "k@example.org"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout ohne %q:\n%s", want, out)
		}
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"estimate", "deliver", "349", "--via", "pdf", "--dry-run"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, "Credits: 2\n") {
		t.Errorf("stdout = %s", out)
	}
}

// The read-only gate comes before the mail text is read (plan order).
func TestDeliverReadOnly(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "deliver", "459", "--via", "pdf", "--yes"},
		{"estimate", "deliver", "349", "--via", "pdf", "--dry-run"},
		{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org", "--subject", "S", "--body-file", "-", "--yes"},
		{"invoice", "deliver", "459", "--input", "-", "--yes"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		in := &countingReader{r: strings.NewReader(`{"send_via":"pdf"}`)}
		app.In = in
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
		if in.n != 0 {
			t.Errorf("%v: Eingabe gelesen (%d Reads), obwohl der Schreibschutz greift", args, in.n)
		}
	}
}

func TestDeliverNeedsConfirmation(t *testing.T) {
	for _, args := range [][]string{
		{"invoice", "deliver", "459", "--via", "pdf"},
		{"estimate", "deliver", "349", "--via", "pdf", "--no-input"},
	} {
		app, srv := apiApp(t) // no terminal
		wantExit(t, app, Run(app, args), 8)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}
}

// Spec 8.2: the confirmation of the send class shows recipient and
// subject; with --via pdf it says that no mail is sent.
func TestDeliverConfirmationSummary(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"--via", "email", "--to", "kunde@example.org", "--subject", "Rechnung {{rechnung.rechnungsnummer}}", "--body", "B"},
			[]string{"Versand:   E-Mail", "Empfänger: kunde@example.org", "Betreff:   Rechnung {{rechnung.rechnungsnummer}}"}},
		{[]string{"--via", "pdf"}, []string{"Versand:   PDF"}},
	} {
		app, srv := apiApp(t)
		app.StdinTTY = true
		app.In = strings.NewReader("ja\n")
		srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
		srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
		wantExit(t, app, Run(app, append([]string{"invoice", "deliver", "459", "-o", "json"}, tc.args...)), 0)
		reqs := wantRequests(t, srv, 3) // summary, deliver, readback
		if reqs[1].Method != "POST" {
			t.Errorf("requests = %+v", reqs)
		}
		_, errs := bufs(app)
		for _, want := range append(tc.want, "Rechnung 459 „Beratung“ (1.190,00 €) wirklich versenden? [ja/nein]") {
			if !strings.Contains(errs, want) {
				t.Errorf("%v: stderr ohne %q:\n%s", tc.args, want, errs)
			}
		}
		if tc.args[1] == "pdf" && strings.Contains(errs, "Empfänger") {
			t.Errorf("pdf: stderr = %s", errs)
		}
	}

	// declined: nothing sent
	app, srv := apiApp(t)
	app.StdinTTY = true
	app.In = strings.NewReader("nein\n")
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "pdf"}), 8)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
}

func TestDeliverOutputFormats(t *testing.T) {
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{nil, "FELD        WERT\nID          459\nNR.         RE-1001\nSTATUS      unpaid\nVERSENDET   2026-11-04\nVERSANDART  email\nEMPFÄNGER   kunde@example.org\n"},
		{[]string{"--fields", "id,sent_on", "-o", "json"}, "{\n  \"id\": 459,\n  \"sent_on\": \"2026-11-04\"\n}\n"},
		{[]string{"--jq", ".invoice_no"}, "RE-1001\n"},
		{[]string{"-o", "yaml"}, "id: 459\ninvoice_no: RE-1001\nstate: unpaid\nsent_on: \"2026-11-04\"\nsent_via: email\nsent_to: kunde@example.org\n"},
	} {
		app, srv := apiApp(t)
		srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
		srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
		wantExit(t, app, Run(app, append([]string{"invoice", "deliver", "459", "--via", "pdf", "--yes"}, tc.flags...)), 0)
		if out := mustOut(app); out != tc.want {
			t.Errorf("%v: stdout = %q, erwartet %q", tc.flags, out, tc.want)
		}
	}
}

func TestDeliverHelp(t *testing.T) {
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "--help"}), 0)
	out := mustOut(app)
	for _, want := range []string{"deliver <id>", "Aliase", "send", "--via", "--to", "--subject", "--body-file", "--input", "Nachlesen"} {
		if !strings.Contains(out, want) {
			t.Errorf("Hilfe ohne %q:\n%s", want, out)
		}
	}
}

// An error answer to the deliver POST ends the command without readback;
// the POST is never repeated. 422 (e.g. a broken placeholder, spec 4.5)
// means not sent; after a 5xx the mail may have gone out, so the hint asks
// to check with describe and never invites a plain retry.
func TestDeliverPostError(t *testing.T) {
	for _, tc := range []struct {
		status, exit int
		hint         string
	}{
		{http.StatusUnprocessableEntity, 5, "Eingaben prüfen"},
		{http.StatusInternalServerError, 1, "vor Wiederholung per describe prüfen: papierkram invoice describe 459"},
		{http.StatusBadGateway, 1, "vor Wiederholung per describe prüfen: papierkram invoice describe 459"},
	} {
		app, srv := apiApp(t)
		srv.On("POST", "/income/invoices/459/deliver", tc.status, []byte(`{"type":"error","message":"Du hast einen Fehler in einem der verwendeten Platzhalter."}`))
		wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org",
			"--subject", "{{rechnung.nummer}}", "--body", "B", "--yes"}), tc.exit)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" {
			t.Errorf("%d: request = %s %s", tc.status, r.Method, r.Path)
		}
		out, errs := bufs(app)
		if out != "" || !strings.Contains(errs, "\nHinweis: ") || !strings.Contains(errs, tc.hint) || strings.Contains(errs, "erneut versuchen") {
			t.Errorf("%d: stdout = %q, stderr = %q", tc.status, out, errs)
		}
	}
}

// A date-only sent_on is a calendar day of Papierkram (Europe/Berlin), not
// of the host: on a UTC or US host near midnight, "today" is the Berlin day.
func TestDeliverTimeoutSentOnIsABerlinDay(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// both are 2026-10-27 00:30 in Berlin (CET)
	utcNow := time.Date(2026, 10, 26, 23, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		loc    *time.Location
		sentOn string
		exit   int
	}{
		{"UTC, Berliner heute", time.UTC, `"2026-10-27"`, 0},
		{"UTC, Berliner gestern", time.UTC, `"2026-10-26"`, 12},
		{"New York, Berliner heute", ny, `"2026-10-27"`, 0},
		{"New York, Berliner gestern", ny, `"2026-10-26"`, 12},
		{"New York, Uhrzeit Berliner gestern", ny, `"2026-10-26 23:50:00 +0100"`, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			app.Location = tc.loc
			app.Now = func() time.Time { return utcNow.In(tc.loc) }
			srv.OnFunc("POST", "/income/invoices/459/deliver", func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			})
			srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, tc.sentOn))
			code := waitExit(t, func() int {
				return Run(app, []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "--timeout", "300ms", "-o", "json"})
			})
			wantExit(t, app, code, tc.exit)
		})
	}
}

// An empty mail text is missing, whether given by --body or --body-file
// (spec 9.1: exit 2).
func TestDeliverEmptyBodyFile(t *testing.T) {
	for _, content := range []string{"", "\uFEFF", " \r\n\t"} {
		app, srv := apiApp(t)
		path := writeFile(t, "mail.txt", content)
		wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--via", "email", "--to", "k@example.org",
			"--subject", "S", "--body-file", path, "--yes"}), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--body (oder --body-file)") {
			t.Errorf("%q: stderr = %q", content, errs)
		}
	}
}

// "email": null (e.g. from a copied GET answer) is no mail: --via pdf
// accepts it and sends no email key.
func TestDeliverPDFWithNullEmail(t *testing.T) {
	for _, args := range [][]string{
		{"--input", "-", "--via", "pdf"},
		{"--input", "-"},
	} {
		app, srv := apiApp(t)
		srv.On("POST", "/income/invoices/459/deliver", http.StatusOK, nil)
		srv.On("GET", "/income/invoices/459", http.StatusOK, sentDoc("invoice", 459, `"2026-11-04"`))
		app.In = strings.NewReader(`{"send_via":"pdf","email":null}`)
		wantExit(t, app, Run(app, append([]string{"invoice", "deliver", "459", "--yes", "-o", "json"}, args...)), 0)
		if got := canonJSON(t, wantRequests(t, srv, 2)[0].Body); got != `{"send_via":"pdf"}` {
			t.Errorf("%v: body = %s", args, got)
		}
	}
}

// The confirmation shows recipient, subject and name without control
// characters: a \r or an ANSI escape from an --input file or the API must
// not overwrite or hide what the user approves.
func TestDeliverConfirmationSanitized(t *testing.T) {
	app, srv := apiApp(t)
	app.StdinTTY = true
	path := writeFile(t, "in.json", `{"send_via":"email","email":{"recipient":"boese@example.org\rkunde@example.org",`+
		`"subject":"Rechnung\n\u001b[2KBetreff","body":"B"}}`)
	app.In = strings.NewReader("nein\n")
	srv.On("GET", "/income/invoices/459", http.StatusOK, []byte(`{"id":459,"name":"Beratung\r\u001b[31mRot","total_gross":"1190.00"}`))
	wantExit(t, app, Run(app, []string{"invoice", "deliver", "459", "--input", path}), 8)
	_, errs := bufs(app)
	if strings.ContainsAny(errs, "\r\x1b") {
		t.Errorf("Steuerzeichen in der Rückfrage: %q", errs)
	}
	for _, want := range []string{"Empfänger: boese@example.org kunde@example.org\n", "Betreff:   Rechnung  [2KBetreff\n", "„Beratung  [31mRot“"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q: %q", want, errs)
		}
	}
}
