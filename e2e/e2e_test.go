//go:build e2e

// Live E2E suite against a Papierkram SANDBOX account (spec 11.1, 17).
//
// Runs only with -tags e2e and PAPIERKRAM_E2E_SUBDOMAIN,
// PAPIERKRAM_E2E_TOKEN and PAPIERKRAM_E2E_SANDBOX_CONFIRM (exactly equal to
// the subdomain) set; otherwise every test skips without a request
// (e2eguard.Decide, TestMain). Choosing a sandbox is the caller's
// responsibility: never point it at real bookkeeping.
//
// Credit budget per run: creditBudget = 760 (spec 11.1: target < 800).
// Documented cost (registry.ExpectedCredits, spec 4.3) of every request of
// a full run without leftovers, by test in execution order (files
// alphabetically, source order within a file); a fixture counts where it
// is used first. No row is planned any more: every amount is the
// breakdown of tests that exist.
//
//	CleanupLeftovers: 1 list page per 100 entries of 9 resources
//	  (all pages; more with leftovers), at least                    9
//	QuotaHeadersPresent 1, InfoCreditCost 3                         4
//	DeliverPdfEmptyBody: customer, person, project (30), own
//	  estimate 50, deliver 1, readback 1                           82
//	VatRate019Is19Percent: estimate 50, article 10, 2 describes    62
//	PersonDefaultIsString: describe                                 1
//	UploadMimeTypes: supplier 10, voucher 50, 4 uploads × 10      100
//	VoucherStateValues: 3 list pages                                3
//	PartialUpdateSemantics: task 10, payment terms 1, invoice
//	  51, user 1, time entry 10, custom attribute 10,
//	  10 × (describe, update, describe), invoice update reads
//	  the invoice first (1)                                       114
//	ReplaceLineItems: describe, read + update, describe              4
//	InvoiceFlow: invoice 50, deliver 2, pdf 1, cancel +
//	  describe 2, archive + describe + unarchive 3, estimate
//	  cancel + describe 2                                          60
//	MasterDataArchive: archive + unarchive (2) of supplier,
//	  project, task, article, custom attribute, estimate
//	  (6 subtests)                                                 12
//	ReadCommands: person list 1, user describe 1, payment-term
//	  describe 1, bank-connection and transaction list 1 each and
//	  describe 1 each (only with data), auth status (GET /info)
//	  1; at most                                                    8
//	TimeFlow: time log 10, list 1, archive + unarchive 2, stop
//	  10, describe 1 (start, status, discard local: 0)             24
//	VoucherFlow: own creditor 10, create --file 60, describe 1,
//	  pdf 1, document delete 1, archive + unarchive 2, reverse +
//	  describe 2, cancel + describe 2                              79
//	Carries: 13 subtests 37 (2 time entries of 10 in no. 12,
//	  describe, update, describe in no. 6), no. 14 describe,
//	  read + update, describe 4                                    41
//	OpenMeasurements (4 subtests): no. 28 invoice create without
//	  document_date 50; no. 10 describe, api PUT, describe 3;
//	  no. 13 read + update, describe, round trip update,
//	  describe 5; no. 12 voucher update, describe, update,
//	  describe 4                                                   62
//	final cleanup: 1 delete per object (7 documents and 13 other
//	  objects = 20); the customer (and possibly the creditor of
//	  VoucherFlow) can only be archived: +1 each, 2; each of up
//	  to 3 final documents costs up to 3 more (cancel, delete,
//	  archive), 9                                                  31
//	                                                     total    696
//
// Not counted: a cancel or reverse (InvoiceFlow, VoucherFlow; 4 in all)
// whose answer names an object other than its target registers that
// follow-up object for the cleanup (followUp; none expected). Each costs
// up to 2 there (delete, archive; never cancel), together at most 8, taken
// from the margin of the reserve: worst case 704 in all, cleanup 62 of 64.
//
// Of the budget, cleanupReserve = 64 is kept for the final cleanup: the
// documented worst case (e2eguard.CleanupReserve) for the 7 documents
// (2 estimates, up to 3 invoices, 2 vouchers; 4 each) and up to 13 other
// objects (customer, supplier, creditor of VoucherFlow, person, project,
// task, article, custom attribute, time entry fixture, 2 midnight time
// entries, the entries of time log and time stop; 2 each), plus a margin
// of 10. Tests may use 760 - 64 = 696 (planned: 665, margin 31). The meter (e2eguard.Meter)
// counts every sent request, rejected ones too, with max(documented cost,
// X-Consumed-Quota), the header being the cost of that one request;
// before the cleanup the transport refuses every request that would cut
// into the reserve, and need() skips a test (and all later ones) when its
// cost no longer fits. TestMain releases the reserve for the cleanup and
// prints the total at the end.
//
// Quota: a CLI run that hits HTTP 429 (exit 6, e2eguard.QuotaHit) stops
// the tests like an exhausted budget: no further request in the tests,
// every later test skips, no coverage gate (AGENTS.md rule 7). The cleanup
// still tries once; a 429 there stops it without a further request. Such
// a run always ends with exit 1.
//
// Coverage: the transport also records which operation each answered
// request reached (e2eguard.Coverage). After the cleanup TestMain prints
// one ABDECKUNG line per finding against e2eguard.CoveragePlan and fails
// a full run (no -run, -skip or -list, budget and quota not exhausted, no
// failure so far)
// that missed an expected operation; pending ones never fail it.
//
// Measurements (MESSUNG in the log) answer the open points of spec 17 and
// the carries of task 44. A test fails hard only where the CLI relies on
// the behaviour:
//
//	hard: deliver --via pdf succeeds (2xx, exit 0); quota headers present;
//	  0.19 books 19 % (line item and article); person default as string
//	  accepted and stored; PDF upload accepted; PUT is a partial update;
//	  line_items replace all; carries 1 (ISO deadline), 2 (describe |
//	  update round trip keeps contact person and project), 3 (estimate
//	  PUT with sent_on/sent_to null), 4 (voucher PUT with
//	  entertainment_persons []),
//	  8 (PDF download Accept/Content-Type), 9 (price_gross 0.0 with
//	  gross=false), 11 (duration = wall clock difference on a normal day),
//	  12 (time log --from 23:00 --to 00:00 and --to 24:00: exit 0 and
//	  duration 3600; ended_at and the sent end measured); open point 13:
//	  the round trip with payment_term in the body sends no read before
//	  the PUT;
//	  invoice flow: deliver --via pdf, pdf (%PDF file), archive and
//	  unarchive exit 0; voucher flow: create --file (exit 0, upload
//	  2xx), exactly one document, pdf, document delete, archive and
//	  unarchive exit 0; time flow: time log, the entry in time list
//	  --since heute, archive and unarchive exit 0, the timer (start,
//	  status, stop, status, stop again exit 2, start, discard; all but
//	  stop without a request), the stopped entry from 10 to 8 minutes
//	  before the start of the test (Berlin, same day; the test fails
//	  before its first request when started before 01:15) with duration
//	  120; master data: archive and unarchive of
//	  company, project, task, proposition, custom attribute and
//	  estimate exit 0; read commands: the person of the run in person
//	  list, user and payment-term describe answer with the asked id,
//	  bank-connection and transaction list (and describe of the first
//	  entry, if any) exit 0, auth status with an integer
//	  quota.remaining, read_only false and token_source env.
//	measured only: empty deliver body, upload of png/jpg/txt, voucher
//	  state values, invoice states along its life cycle, invoice and
//	  estimate cancel, voucher document key shape, voucher pdf
//	  download, voucher reverse and cancel with the states after them,
//	  GET /info cost, carries 5 (due_time_unit values),
//	  6 (billable_duration on create and update; run 1: the create
//	  stored duration instead), 7 (sent_on after deliver pdf), 10 (pay body with value null and a
//	  transaction that does not exist: the suite never pays),
//	  13 (structure of GET /info); open points
//	  28 (invoice create without document_date), 10 (due_date through
//	  api PUT), 13 (supply_date format and round trip), 12
//	  (entertainment_persons set, then []: cleared or unchanged); the
//	  answer of archive and unarchive (record_state or empty), the number
//	  (total_entries) and key shape of bank connections and bank
//	  transactions (never their values; without an entry the read by
//	  id is reported not applicable).
//	A measurement that differs from what the CLI assumes (GET /info cost
//	  vs. registry, due_time_unit other than "day", billable_duration
//	  not stored as sent, a cancel or reverse answering with another
//	  object, an invoice without document_date rejected with 422, a
//	  due_date changed by PUT, supply_date not stored as sent or changed
//	  by the round trip, a failed step of that round trip, [] rejected
//	  for entertainment_persons) is logged as
//	  ABWEICHUNG and listed again in a summary at the end of the run;
//	  it does not fail the run.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
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

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// creditBudget is the most credits one run may use (spec 11.1: < 800).
const creditBudget = 760

// The build fails when creditBudget reaches 800 (constant overflow).
const _ = uint(799 - creditBudget)

// cleanupReserve is the part of the budget kept for the final cleanup
// (see the header): 7 documents, 13 other objects, margin 10 = 64.
var cleanupReserve = e2eguard.CleanupReserve(7, 13, 10)

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
	s.coverage.BeginCleanup()
	if !s.cleanup() && code == 0 {
		s.stderrf("Aufräumen unvollständig: Objekte mit Präfix %s prüfen", s.prefix)
		code = 1
	}
	_ = os.RemoveAll(s.mainDir)
	if s.quota || s.quotaCleanup {
		// external stop (AGENTS.md rule 7): the run is incomplete, a
		// human decides; never green
		s.stderrf("Lauf durch das Kontingent (HTTP 429) abgebrochen: nicht wiederholen, bevor ein Mensch das Kontingent geprüft hat")
		code = max(code, 1)
	}
	rep := s.coverageSummary()
	// only a full run must cover every expected operation: not one
	// narrowed by -run, -skip or -list, stopped by the budget or the
	// quota, or failed already
	if len(rep.Missing) > 0 && fullRun() && !s.exhausted && code == 0 {
		s.stderrf("Abdeckung unvollständig: %d Operation(en) fehlen", len(rep.Missing))
		code = 1
	}
	s.deviationSummary()
	s.stderrf("%s", s.meter.Summary())
	os.Exit(code)
}

// fullRun reports whether no test was selected, skipped or only listed
// by name (-run, -skip, -list; e2eguard.FullRun): only then does the
// coverage gate of TestMain apply.
func fullRun() bool {
	return e2eguard.FullRun(func(name string) string {
		if f := flag.Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	})
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

// listAll lists every page of resource (100 entries each, 1 credit per
// page, need() before each), newest first if the API sorts by id,
// otherwise in its order. It follows has_more/total_pages to the end, so
// no leftover is missed in a full sandbox.
func (s *liveSuite) listAll(t *testing.T, resource string, extra ...string) []map[string]any {
	t.Helper()
	return s.listPages(t, resource, nil, extra...)
}

// listPages is listAll; first, when set, gets the output of the first
// page that answered (exit 0).
func (s *liveSuite) listPages(t *testing.T, resource string, first func(t *testing.T, resource, stdout string), extra ...string) []map[string]any {
	t.Helper()
	sorted := true
	var out []map[string]any
	for page := 1; ; page++ {
		s.need(t, 1)
		args := append([]string{resource, "list", "--page", strconv.Itoa(page), "--limit", "100", "-o", "json"}, extra...)
		if sorted {
			args = append(args, "--sort", "id", "--desc")
		}
		r := s.run(t, "", args...)
		if r.code != 0 && sorted && page == 1 {
			s.logf(t, "%s list --sort id: Exit %d, ohne Sortierung erneut", resource, r.code)
			sorted, page = false, 0
			continue
		}
		if r.code != 0 {
			s.fatalf(t, "%s list Seite %d: Exit %d\n%s", resource, page, r.code, r.stderr)
		}
		if first != nil && page == 1 {
			first(t, resource, r.stdout)
		}
		m := s.decode(t, r.stdout)
		es := entries(m)
		out = append(out, es...)
		total, _ := strconv.Atoi(str(m["total_pages"]))
		if len(es) == 0 || (str(m["has_more"]) != "true" && page >= total) {
			s.logf(t, "%s: %d Seite(n), %d Einträge durchsucht", resource, page, len(out))
			return out
		}
	}
}

// TestE2E_CleanupLeftovers removes objects of earlier runs: only those
// whose name starts with pk-e2e- (never anything else), and not those of
// this run (the cleanup at the end handles them). The first page of each
// list is checked against docs/schemas/list.json (checkListContract).
func TestE2E_CleanupLeftovers(t *testing.T) {
	s := live(t)
	removed, kept := 0, 0
	for _, k := range leftoverKinds {
		for _, e := range s.listPages(t, k.resource, s.checkListContract) {
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
			switch {
			case ok:
				removed++
			case s.quota:
				s.skipf(t, "Rest %s %d nicht entfernt: Kontingent erschöpft (HTTP 429), Test übersprungen", k.resource, idOf(e))
			default:
				kept++
				s.errorf(t, "Rest %s %d nicht entfernt: %s", k.resource, idOf(e), what)
			}
		}
	}
	s.logf(t, "Reste früherer Läufe: %d entfernt, %d archiviert/übrig", removed, kept)
}

// listContract is the path of the contract of list -o json, relative to
// the package directory.
const listContract = "../docs/schemas/list.json"

// checkListContract checks the first page of a list that
// TestE2E_CleanupLeftovers loads anyway (no credit more) against
// docs/schemas/list.json (review T13-M2): the envelope of the live API
// as the CLI prints it. A violation is a measurement (ABWEICHUNG), naming
// only the JSON path and the rule, never values of the entries.
func (s *liveSuite) checkListContract(t *testing.T, resource, stdout string) {
	t.Helper()
	raw, err := os.ReadFile(listContract)
	if err != nil {
		s.errorf(t, "Vertrag %s nicht lesbar: %v", listContract, err)
		return
	}
	var schema openapi3.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		s.errorf(t, "Vertrag %s ist kein Schema-Objekt: %v", listContract, err)
		return
	}
	if err := schema.Validate(context.Background()); err != nil {
		s.errorf(t, "Vertrag %s ist kein gültiges OpenAPI-3-Schema: %v", listContract, err)
		return
	}
	var doc any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		s.deviatef(t, "%s list Seite 1: keine JSON-Ausgabe, Vertrag list.json nicht prüfbar", resource)
		return
	}
	err = schema.VisitJSON(doc)
	var se *openapi3.SchemaError
	switch {
	case err == nil:
		s.logf(t, "%s list Seite 1 erfüllt list.json", resource)
	case errors.As(err, &se):
		s.deviatef(t, "%s list Seite 1 verletzt list.json bei /%s: %s", resource, strings.Join(se.JSONPointer(), "/"), se.Reason)
	default:
		s.deviatef(t, "%s list Seite 1 verletzt list.json (%T)", resource, err)
	}
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

// Spec 17: does GET /info cost credits? Three calls in a row; the cost
// of each is its own X-Consumed-Quota (the cost of that one request, not a
// running total: measured 2026-11-04). The difference of
// X-Remaining-Quota is only logged. A measurement: a cost other than the
// registry's is an ABWEICHUNG (the registry is then adjusted), not a
// failure.
func TestE2E_InfoCreditCost(t *testing.T) {
	s := live(t)
	s.need(t, 3)
	var consumed []int
	var remaining []string
	for range 3 {
		r := s.run(t, "", "info", "-o", "json")
		x := lastExchange(r, "GET", "/info")
		if r.code != 0 || x == nil {
			s.fatalf(t, "info: Exit %d\n%s", r.code, r.stderr)
		}
		remaining = append(remaining, strings.TrimSpace(x.RespHeader.Get("X-Remaining-Quota")))
		n, err := strconv.Atoi(strings.TrimSpace(x.RespHeader.Get("X-Consumed-Quota")))
		if err != nil {
			s.deviatef(t, "GET /info: X-Consumed-Quota fehlt, Kosten nicht messbar")
			return
		}
		consumed = append(consumed, n)
	}
	e, _ := registry.Lookup("info", "")
	s.measuref(t, "GET /info", "Kosten laut X-Consumed-Quota %v; Registry: %d", consumed, e.Credits)
	s.measuref(t, "GET /info", "X-Remaining-Quota %v (nur zur Information)", remaining)
	if d := e2eguard.CostDeviation("GET /info", consumed, e.Credits); d != "" {
		s.deviatef(t, "%s", d)
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
// TestE2E_InvoiceFlow cancels this estimate: a test after it finds it
// cancelled.
func (s *liveSuite) deliverPDF(t *testing.T) *deliverRun {
	t.Helper()
	if delivered != nil {
		return delivered
	}
	if s.exhausted {
		s.skipf(t, "%s: kein deliver --via pdf", s.stopReason())
	}
	if s.failed["deliver"] {
		s.fatalf(t, "deliver --via pdf ist bereits gescheitert")
	}
	done := false
	defer func() {
		if !done && t.Failed() {
			s.failed["deliver"] = true
		}
	}()
	name := s.prefix + "angebot-versand"
	args := s.estimateArgs(t, name)
	s.need(t, 53)
	est := s.create(t, object{resource: "estimate", name: name}, args...)
	r := s.run(t, "", "estimate", "deliver", ids(idOf(est)), "--via", "pdf", "--yes", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "estimate deliver --via pdf: Exit %d\n%s", r.code, r.stderr)
	}
	delivered = &deliverRun{res: r, out: s.decode(t, r.stdout), estimate: idOf(est)}
	done = true
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
		// the question is only asked when 0.19 (a number) was sent
		var sent any = "nicht aufgezeichnet"
		if body := s.sentBody(t, "estimate", est); body != nil {
			sent = "fehlt"
			for _, it := range asSlice(body["line_items"]) {
				if str(get(it, "name")) == pos19 {
					sent = get(it, "vat_rate")
				}
			}
		}
		s.measuref(t, "vat_rate 0.19 (Position)", "gesendet vat_rate %v (%s)", sent, jsonType(sent))
		if str(sent) != "0.19" || jsonType(sent) != "number" {
			s.fatalf(t, "nicht geprüft: Position %s wurde nicht mit vat_rate 0.19 (Zahl) angelegt, sondern %v", pos19, sent)
		}
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
		var sent any = "nicht aufgezeichnet"
		if body := s.sentBody(t, "proposition", p); body != nil {
			sent = body["vat_rate"]
		}
		s.measuref(t, "vat_rate \"0.19\" (Artikel)", "gesendet vat_rate %v (%s)", sent, jsonType(sent))
		if str(sent) != "0.19" || jsonType(sent) != "string" {
			s.fatalf(t, "nicht geprüft: Artikel wurde nicht mit vat_rate \"0.19\" (String) angelegt, sondern %v", sent)
		}
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
// type (run 1, 2026-11-04: boolean).
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
		// payment_term of an invoice is a copy of the chosen template (in
		// every spec example its id is template_id + 1). Run 3 showed a PUT
		// without payment_term switching to the default template and
		// changing due_date; invoice update now reads the invoice and
		// sends its payment_term.id (ruling E2E-A). Hard: template and
		// due_date stay.
		{"invoice", byID("invoice", idf(s.invoice)), upd("invoice", idf(s.invoice), "--description", val),
			[]string{"description"}, val, [][]string{{"name"}, {"payment_term", "template_id"}, {"due_date"}, {"line_items"}}},
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
			// describe, update, describe; invoice update reads the
			// invoice first (keeps its payment term)
			n := 3
			if c.name == "invoice" {
				n = 4
			}
			s.need(t, n)
			before := s.ok(t, "", d...)
			s.ok(t, "", u...)
			after := s.ok(t, "", d...)
			if other := changedKeys(before, after, c.field[0]); len(other) > 0 {
				s.measuref(t, "Teil-Update", "%s: außerdem geändert (nur Schlüssel): %s", c.name, strings.Join(other, ", "))
			}
			if c.name == "invoice" {
				s.measuref(t, "Teil-Update", "invoice: payment_term vorher %s, nachher %s", paymentTermShape(before), paymentTermShape(after))
			}
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

// changedKeys are the top-level keys whose value differs between before
// and after, without skip and the timestamps; sorted. Only the keys are
// logged, never the values.
func changedKeys(before, after map[string]any, skip string) []string {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if k == skip || k == "updated_at" || k == "created_at" {
			continue
		}
		if fmt.Sprint(before[k]) != fmt.Sprint(after[k]) {
			out = append(out, k)
		}
	}
	return out
}

// paymentTermShape is id, template_id and name of the payment_term of an
// invoice answer.
func paymentTermShape(m map[string]any) string {
	return fmt.Sprintf("id %s, template_id %s, name %q",
		str(get(m, "payment_term", "id")), str(get(m, "payment_term", "template_id")), str(get(m, "payment_term", "name")))
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
	s.need(t, 4) // describe, update (reads the invoice first), describe
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
