//go:build e2e

// Live E2E suite against a Papierkram SANDBOX account (spec 11.1, 17).
//
// Runs only with -tags e2e and PAPIERKRAM_E2E_SUBDOMAIN,
// PAPIERKRAM_E2E_TOKEN and PAPIERKRAM_E2E_SANDBOX_CONFIRM (exactly equal to
// the subdomain) set; otherwise every test skips without a request
// (e2eguard.Decide, TestMain). Choosing a sandbox is the caller's
// responsibility: never point it at real bookkeeping.
//
// Credit budget per run: creditBudget = 490 (spec 11.1: target < 500).
// Documented cost (registry.ExpectedCredits, spec 4.3) of every request of
// a full run without leftovers, by test in file order; a fixture counts
// where it is used first:
//
//	CleanupLeftovers: 9 list pages (more with leftovers)            9
//	QuotaHeadersPresent 1, InfoCreditCost 3                         4
//	DeliverPdfEmptyBody: customer, person, project (30), own
//	  estimate 50, deliver 1, readback 1                           82
//	VatRate019Is19Percent: estimate 50, article 10, 2 describes    62
//	PersonDefaultIsString: describe                                 1
//	UploadMimeTypes: supplier 10, voucher 50, 4 uploads × 10      100
//	VoucherStateValues: 3 list pages                                3
//	PartialUpdateSemantics: task 10, payment terms 1, invoice
//	  51, user 1, time entry 10, custom attribute 10,
//	  10 × (describe, update, describe)                           113
//	ReplaceLineItems: describe, update, describe                    3
//	Carries (13 subtests, 2 time entries of 10 in no. 12)          36
//	final cleanup: 1 delete per object (13); a final document
//	  costs up to 3 more (cancel, delete, archive)                 13
//	                                                     total    426
//
// Of the budget, cleanupReserve = 46 is kept for the final cleanup: the
// documented worst case (e2eguard.CleanupReserve) for the 4 documents
// (2 estimates, invoice, voucher; 4 each) and up to 10 other objects
// (2 each), plus a margin of 10. Tests may use 490 - 46 = 444 (planned:
// 413). The meter (e2eguard.Meter) counts the real cost from
// X-Consumed-Quota; before the cleanup the transport refuses every
// request that would cut into the reserve, and need() skips a test (and
// all later ones) when its cost no longer fits. TestMain releases the
// reserve for the cleanup and prints the total at the end.
//
// Measurements (MESSUNG in the log) answer the open points of spec 17 and
// the carries of task 44. A test fails hard only where the CLI relies on
// the behaviour:
//
//	hard: deliver --via pdf succeeds (2xx, exit 0); quota headers present;
//	  0.19 books 19 % (line item and article); person default as string
//	  accepted and stored; PDF upload accepted; GET /info costs what the
//	  registry says; PUT is a partial update; line_items replace all;
//	  carries 1 (ISO deadline), 2 (describe | update round trip keeps
//	  contact person and project), 3 (estimate PUT with sent_on/sent_to
//	  null), 4 (voucher PUT with entertainment_persons []),
//	  5 (due_time_unit only "day": the table header says days),
//	  6 (billable_duration in seconds), 8 (PDF download Accept/Content-
//	  Type), 9 (price_gross 0.0 with gross=false), 11 (duration = wall
//	  clock difference on a normal day).
//	measured only: empty deliver body, upload of png/jpg/txt, voucher
//	  state values, carries 7 (sent_on after deliver pdf), 10 (pay
//	  with value null / amount and transaction), 12 (end 00:00/24:00),
//	  13 (structure of GET /info).
package e2e

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// creditBudget is the most credits one run may use (spec 11.1: < 500).
const creditBudget = 490

// The build fails when creditBudget reaches 500 (constant overflow).
const _ = uint(499 - creditBudget)

// cleanupReserve is the part of the budget kept for the final cleanup
// (see the header).
var cleanupReserve = e2eguard.CleanupReserve(4, 10, 10)

// suite is the live run; nil when the suite is skipped (skipReason).
var (
	suite      *liveSuite
	skipReason string
)

func TestMain(m *testing.M) {
	run, reason := e2eguard.Decide(os.Getenv)
	if !run {
		// no suite, no request: every test skips in live()
		skipReason = reason
		os.Exit(m.Run())
	}
	red := e2eguard.NewRedactor(os.Getenv(e2eguard.EnvToken))
	s, err := newLiveSuite(os.Getenv, red)
	if err != nil {
		stderrPlain(red.String("Vorbereitung gescheitert: " + err.Error()))
		os.Exit(1)
	}
	suite = s
	s.stderrf("Lauf %s gegen Sandbox %s, Credit-Budget %d", s.runID, s.sub, creditBudget)
	code := m.Run()
	s.meter.BeginCleanup()
	if !s.cleanup() && code == 0 {
		s.stderrf("Aufräumen unvollständig: Objekte mit Präfix %s prüfen", s.prefix)
		code = 1
	}
	_ = os.RemoveAll(s.mainDir)
	s.stderrf("%s", s.meter.Summary())
	os.Exit(code)
}

// leftoverKinds are the resources TestE2E_CleanupLeftovers searches, in
// removal order (children first). nameField is what Owned checks.
var leftoverKinds = []struct{ resource, nameField string }{
	{"invoice", "name"},
	{"estimate", "name"},
	{"voucher", "name"},
	{"time", "comments"},
	{"task", "name"},
	{"project", "name"},
	{"proposition", "name"},
	{"custom-attribute", "name"},
	{"company", "name"},
}

// listAll lists up to 3 pages of 100, newest first if the API sorts by
// id, otherwise in its order.
func (s *liveSuite) listAll(t *testing.T, resource string, extra ...string) []map[string]any {
	t.Helper()
	s.need(t, 3)
	base := append([]string{resource, "list", "--all", "--max-pages", "3", "--limit", "100", "-o", "json"}, extra...)
	r := s.run(t, "", append(base, "--sort", "id", "--desc")...)
	if r.code != 0 {
		s.logf(t, "%s list --sort id: Exit %d, ohne Sortierung erneut", resource, r.code)
		r = s.run(t, "", base...)
	}
	if r.code != 0 {
		s.fatalf(t, "%s list: Exit %d\n%s", resource, r.code, r.stderr)
	}
	return entries(s.decode(t, r.stdout))
}

// TestE2E_CleanupLeftovers removes objects of earlier runs: only those
// whose name starts with pk-e2e- (never anything else), and not those of
// this run (the cleanup at the end handles them).
func TestE2E_CleanupLeftovers(t *testing.T) {
	s := live(t)
	removed, kept := 0, 0
	for _, k := range leftoverKinds {
		for _, e := range s.listAll(t, k.resource) {
			name := str(e[k.nameField])
			if !e2eguard.Leftover(name, s.runID) {
				continue
			}
			if str(e["record_state"]) == "archived" {
				kept++
				continue
			}
			if k.resource == "company" {
				s.leftoverPersons(t, idOf(e))
			}
			s.need(t, 4)
			what, ok := s.remove(t.TempDir(), object{resource: k.resource, id: idOf(e), name: name})
			s.logf(t, "Rest %s %d (%s): %s", k.resource, idOf(e), name, what)
			if ok {
				removed++
			} else {
				kept++
				s.errorf(t, "Rest %s %d nicht entfernt: %s", k.resource, idOf(e), what)
			}
		}
	}
	s.logf(t, "Reste früherer Läufe: %d entfernt, %d archiviert/übrig", removed, kept)
}

// leftoverPersons removes the persons of a leftover company of the suite
// whose first name is owned by the suite.
func (s *liveSuite) leftoverPersons(t *testing.T, company int64) {
	t.Helper()
	for _, p := range s.listAll(t, "person", "--company", ids(company)) {
		name := str(p["first_name"])
		if !e2eguard.Owned(name) {
			continue
		}
		s.need(t, 1)
		what, _ := s.remove(t.TempDir(), object{resource: "person", id: idOf(p), company: company, name: name})
		s.logf(t, "Rest person %d (%s): %s", idOf(p), name, what)
	}
}

// Spec 4.3: X-Remaining-Quota and X-Consumed-Quota are not in the spec;
// the CLI shows and warns from them (8.5) and the meter counts with them.
func TestE2E_QuotaHeadersPresent(t *testing.T) {
	s := live(t)
	s.need(t, 1)
	r := s.run(t, "", "user", "list", "--limit", "1", "-o", "json")
	if r.code != 0 || len(r.exchanges) == 0 {
		s.fatalf(t, "user list: Exit %d\n%s", r.code, r.stderr)
	}
	h := r.exchanges[len(r.exchanges)-1].RespHeader
	for _, k := range []string{"X-Remaining-Quota", "X-Consumed-Quota"} {
		v := h.Get(k)
		s.measuref(t, "Quota-Header", "%s = %q", k, v)
		if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
			s.errorf(t, "%s fehlt oder ist keine Zahl: %q", k, v)
		}
	}
	s.measuref(t, "Quota-Header", "X-RateLimit-Remaining = %q (wird bewusst ignoriert)", h.Get("X-RateLimit-Remaining"))
}

// Spec 17: does GET /info cost credits? Three calls in a row: the growth
// of X-Consumed-Quota between them is the cost of the second and third.
func TestE2E_InfoCreditCost(t *testing.T) {
	s := live(t)
	s.need(t, 3)
	var consumed []int
	for range 3 {
		r := s.run(t, "", "info", "-o", "json")
		x := lastExchange(r, "GET", "/info")
		if r.code != 0 || x == nil {
			s.fatalf(t, "info: Exit %d\n%s", r.code, r.stderr)
		}
		n, err := strconv.Atoi(strings.TrimSpace(x.RespHeader.Get("X-Consumed-Quota")))
		if err != nil {
			s.fatalf(t, "X-Consumed-Quota fehlt; Kosten nicht messbar")
		}
		consumed = append(consumed, n)
	}
	d1, d2 := consumed[1]-consumed[0], consumed[2]-consumed[1]
	e, _ := registry.Lookup("info", "")
	s.measuref(t, "GET /info", "Kosten %d und %d Credits (X-Consumed-Quota %v); Registry: %d", d1, d2, consumed, e.Credits)
	if d1 != d2 {
		s.fatalf(t, "uneinheitliche Messung (%d, %d): anderer Verbrauch im Account während des Laufs?", d1, d2)
	}
	if d1 != e.Credits {
		s.errorf(t, "GET /info kostet %d Credits, die Registry sagt %d: Registry anpassen (Spezifikation 17)", d1, e.Credits)
	}
}

// deliverRun is the deliver --via pdf of the delivery estimate (shared by
// DeliverPdfEmptyBody and carry 7).
type deliverRun struct {
	res      result
	out      map[string]any
	estimate int64
}

var delivered *deliverRun

// deliverPDF delivers an estimate of its own as PDF, once per run. It is
// an estimate, not an invoice: an invoice would get a number of the
// invoice sequence. Never by mail (the transport refuses that as well).
func (s *liveSuite) deliverPDF(t *testing.T) *deliverRun {
	t.Helper()
	if delivered != nil {
		return delivered
	}
	if s.failed["deliver"] {
		s.fatalf(t, "deliver --via pdf ist bereits gescheitert")
	}
	s.failed["deliver"] = true
	name := s.prefix + "angebot-versand"
	args := s.estimateArgs(t, name)
	s.need(t, 53)
	est := s.create(t, object{resource: "estimate", name: name}, args...)
	r := s.run(t, "", "estimate", "deliver", ids(idOf(est)), "--via", "pdf", "--yes", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "estimate deliver --via pdf: Exit %d\n%s", r.code, r.stderr)
	}
	delivered = &deliverRun{res: r, out: s.decode(t, r.stdout), estimate: idOf(est)}
	s.failed["deliver"] = false
	return delivered
}

// Spec 4.5/9.1: deliver answers live 2xx with an empty body (the spec
// claims 201 with an object). Hard: 2xx and exit 0; measured: the body.
func TestE2E_DeliverPdfEmptyBody(t *testing.T) {
	s := live(t)
	d := s.deliverPDF(t)
	x := lastExchange(d.res, "POST", "/deliver")
	if x == nil {
		s.fatalf(t, "kein deliver-Request aufgezeichnet")
	}
	s.measuref(t, "deliver --via pdf", "HTTP %d, Content-Type %q, Body %d Bytes: %q",
		x.Status, x.RespHeader.Get("Content-Type"), len(x.RespBody), apiMessage(x.RespBody))
	if x.Status < 200 || x.Status > 299 {
		s.errorf(t, "deliver: HTTP %d", x.Status)
	}
	if len(bytes.TrimSpace(x.RespBody)) == 0 {
		s.measuref(t, "deliver --via pdf", "leerer Body bestätigt")
	} else {
		s.measuref(t, "deliver --via pdf", "Body NICHT leer (Spezifikation 4.5 anpassen)")
	}
	s.logf(t, "Ausgabe: %v", d.out)
}

// Spec 6.2/11.1: vat_rate 0.19 books 19 %, in a line item (number) and in
// an article (string "0.19").
func TestE2E_VatRate019Is19Percent(t *testing.T) {
	s := live(t)
	t.Run("Position", func(t *testing.T) {
		est := idOf(s.estimate(t))
		s.need(t, 1)
		m := s.ok(t, "", "estimate", "describe", ids(est))
		items, _ := m["line_items"].([]any)
		found := false
		for _, it := range items {
			if str(get(it, "name")) != pos19 {
				continue
			}
			found = true
			net, _ := num(get(it, "total_net"))
			vat, _ := num(get(it, "total_vat"))
			s.measuref(t, "vat_rate 0.19 (Position)", "vat_rate %q, total_net %v, total_vat %v", str(get(it, "vat_rate")), net, vat)
			if str(get(it, "vat_rate")) != "19%" || !near(vat, net*0.19) {
				s.errorf(t, "Position mit vat_rate 0.19 nicht als 19 %% gebucht: vat_rate %q, total_vat %v bei total_net %v",
					str(get(it, "vat_rate")), vat, net)
			}
		}
		if !found {
			s.errorf(t, "Position %s fehlt im Angebot", pos19)
		}
	})
	t.Run("Artikel", func(t *testing.T) {
		p := idOf(s.proposition(t))
		s.need(t, 1)
		m := s.ok(t, "", "proposition", "describe", ids(p))
		rate, label := str(get(m, "vat_rate", "rate")), str(get(m, "vat_rate", "name"))
		s.measuref(t, "vat_rate \"0.19\" (Artikel)", "vat_rate %v", m["vat_rate"])
		if f, ok := num(rate); !ok || !near(f, 0.19) || label != "19%" {
			s.errorf(t, "Artikel mit vat_rate \"0.19\" nicht als 19 %% gebucht: rate %q, name %q", rate, label)
		}
	})
}

// near compares amounts to the cent.
func near(a, b float64) bool {
	d := a - b
	return d < 0.005 && d > -0.005
}

// Spec 4.4: person default is a string ("true"); the CLI sends --default
// as "true". Hard: accepted and stored as true; measured: the response
// type.
func TestE2E_PersonDefaultIsString(t *testing.T) {
	s := live(t)
	pers := s.person(t)
	s.need(t, 1)
	m := s.ok(t, "", "person", "describe", ids(idOf(pers)), "--company", ids(s.customer(t)))
	v := m["default"]
	s.measuref(t, "Person default", "Antwort %v (JSON-Typ %s)", v, jsonType(v))
	if str(v) != "true" {
		s.errorf(t, "default als String \"true\" gesendet, gespeichert ist %v", v)
	}
}

// Spec 9.3/17: which file types does the upload accept? The CLI has no
// local restriction. Hard only for PDF (voucher create --file beleg.pdf).
func TestE2E_UploadMimeTypes(t *testing.T) {
	s := live(t)
	v := idOf(s.voucher(t))
	dir := t.TempDir()
	files := map[string][]byte{
		"beleg.pdf": minimalPDF,
		"beleg.png": encodeImage(t, "png"),
		"beleg.jpg": encodeImage(t, "jpg"),
		"beleg.txt": []byte("pk-e2e Testbeleg\n"),
	}
	var summary []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, files[name], 0o600); err != nil {
			s.fatalf(t, "%v", err)
		}
		s.need(t, 10)
		r := s.run(t, "", "voucher", "upload", ids(v), path, "-o", "json")
		x := lastExchange(r, "POST", "/documents")
		status, msg := 0, ""
		if x != nil {
			status, msg = x.Status, apiMessage(x.RespBody)
		}
		res := fmt.Sprintf("%s: Exit %d, HTTP %d", name, r.code, status)
		if r.code != 0 {
			res += " (" + msg + ")"
		}
		summary = append(summary, res)
		if name == "beleg.pdf" && r.code != 0 {
			s.errorf(t, "PDF-Upload abgelehnt: %s", res)
		}
	}
	s.measuref(t, "Upload-MIME-Typen", "%s", strings.Join(summary, "; "))
}

// minimalPDF is a tiny valid PDF.
var minimalPDF = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
	"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n" +
	"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] >> endobj\n" +
	"trailer << /Root 1 0 R >>\n%%EOF\n")

// encodeImage is a 2x2 image as png or jpg.
func encodeImage(t *testing.T, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		suite.fatalf(t, "%v", err)
	}
	return b.Bytes()
}

// Spec 4.7/17: the state enums of the documents are not documented. The
// suite collects the values it sees (read only, first page of each list).
func TestE2E_VoucherStateValues(t *testing.T) {
	s := live(t)
	v := s.voucher(t)
	s.measuref(t, "Statuswerte", "neuer Beleg: state %q", str(v["state"]))
	for _, res := range []string{"voucher", "invoice", "estimate"} {
		s.need(t, 1)
		seen := map[string]int{}
		for _, e := range entries(s.ok(t, "", res, "list", "--limit", "100")) {
			seen[str(e["state"])]++
		}
		s.measuref(t, "Statuswerte", "%s: %v", res, seen)
	}
}

// partialCase is one resource of PartialUpdateSemantics: update sets one
// field, keep lists fields (key paths) that must not change.
type partialCase struct {
	name     string
	describe func(t *testing.T) []string
	update   func(t *testing.T) []string
	field    []string
	want     string
	keep     [][]string
}

// Spec 6.3a: PUT is a partial update (the CLI sends only given fields).
// Per resource: describe, update one field, describe; the field changed,
// the others (and the line items) stayed.
func TestE2E_PartialUpdateSemantics(t *testing.T) {
	s := live(t)
	val := s.prefix + "teilupdate"
	byID := func(res string, id func(*testing.T) int64, extra ...string) func(*testing.T) []string {
		return func(t *testing.T) []string { return append([]string{res, "describe", ids(id(t))}, extra...) }
	}
	upd := func(res string, id func(*testing.T) int64, args ...string) func(*testing.T) []string {
		return func(t *testing.T) []string { return append([]string{res, "update", ids(id(t))}, args...) }
	}
	idf := func(f func(*testing.T) map[string]any) func(*testing.T) int64 {
		return func(t *testing.T) int64 { return idOf(f(t)) }
	}
	personCompany := func(t *testing.T) []string { return []string{"--company", ids(s.customer(t))} }
	cases := []partialCase{
		{"company", byID("company", s.customer), upd("company", s.customer, "--notes", val),
			[]string{"notes"}, val, [][]string{{"name"}, {"contact_type"}}},
		{"person", func(t *testing.T) []string {
			return append([]string{"person", "describe", ids(idOf(s.person(t)))}, personCompany(t)...)
		}, func(t *testing.T) []string {
			return append([]string{"person", "update", ids(idOf(s.person(t))), "--comment", val}, personCompany(t)...)
		}, []string{"comment"}, val, [][]string{{"first_name"}, {"last_name"}, {"default"}}},
		{"project", byID("project", s.project), upd("project", s.project, "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"company_id"}}},
		{"task", byID("task", s.task), upd("task", s.task, "--name", val),
			[]string{"name"}, val, [][]string{{"project_id"}, {"deadline"}}},
		{"proposition", byID("proposition", idf(s.proposition)), upd("proposition", idf(s.proposition), "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"article_no"}, {"price"}, {"vat_rate", "rate"}}},
		{"estimate", byID("estimate", idf(s.estimate)), upd("estimate", idf(s.estimate), "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"customer", "id"}, {"line_items"}}},
		{"invoice", byID("invoice", idf(s.invoice)), upd("invoice", idf(s.invoice), "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"payment_term", "id"}, {"line_items"}}},
		{"voucher", byID("voucher", idf(s.voucher)), upd("voucher", idf(s.voucher), "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"provenance"}, {"creditor", "id"}, {"line_items"}}},
		{"time", byID("time", idf(s.timeEntry)), upd("time", idf(s.timeEntry), "--comments", val),
			[]string{"comments"}, val, [][]string{{"started_at"}, {"ended_at"}, {"task_id"}, {"billable_duration"}}},
		{"custom-attribute", byID("custom-attribute", s.customAttribute), upd("custom-attribute", s.customAttribute, "--name", val),
			[]string{"name"}, val, [][]string{{"data_type"}, {"attributable_type"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := c.describe(t) // creates the object if needed
			u := c.update(t)
			s.need(t, 3)
			before := s.ok(t, "", d...)
			s.ok(t, "", u...)
			after := s.ok(t, "", d...)
			var lost []string
			if got := str(get(after, c.field...)); got != c.want {
				lost = append(lost, fmt.Sprintf("%s nicht übernommen: %q statt %q", strings.Join(c.field, "."), got, c.want))
			}
			for _, k := range c.keep {
				b, a := fmt.Sprint(get(before, k...)), fmt.Sprint(get(after, k...))
				if b != a {
					lost = append(lost, fmt.Sprintf("%s: %s → %s", strings.Join(k, "."), b, a))
				}
			}
			if len(lost) > 0 {
				s.errorf(t, "Teil-Update %s: %s", c.name, strings.Join(lost, "; "))
			} else {
				s.measuref(t, "Teil-Update", "%s: nur %s geändert", c.name, strings.Join(c.field, "."))
			}
		})
	}
}

// customAttribute is a custom field definition for projects (only for
// PartialUpdateSemantics; removed at the end).
func (s *liveSuite) customAttribute(t *testing.T) int64 {
	return idOf(s.fixture(t, "custom-attribute", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "feld"
		return s.create(t, object{resource: "custom-attribute", name: name},
			"custom-attribute", "create", "--name", name, "--attributable-type", "Project", "--data-type", "string")
	}))
}

// Spec 6.3a: line_items in a PUT replace all line items (they have no id
// in the request).
func TestE2E_ReplaceLineItems(t *testing.T) {
	s := live(t)
	inv := idOf(s.invoice(t))
	s.need(t, 3)
	before := s.ok(t, "", "invoice", "describe", ids(inv))
	n, _ := before["line_items"].([]any)
	if len(n) < 2 {
		s.fatalf(t, "Rechnung hat %d Positionen, erwartet mindestens 2", len(n))
	}
	s.ok(t, "", "invoice", "update", ids(inv), "--replace-items", "--item", "name=pk-e2e-ersatz,qty=1,price=10,vat=19")
	after := s.ok(t, "", "invoice", "describe", ids(inv))
	items, _ := after["line_items"].([]any)
	var names []string
	for _, it := range items {
		names = append(names, str(get(it, "name")))
	}
	s.measuref(t, "Positionen ersetzen", "vorher %d, nachher %v", len(n), names)
	if len(names) != 1 || names[0] != "pk-e2e-ersatz" {
		s.errorf(t, "line_items ersetzen nicht alle Positionen: nachher %v", names)
	}
}
