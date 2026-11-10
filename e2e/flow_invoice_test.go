//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// Spec 9.1, 9.2, 8.1, 17 (open points 1 and 16): the life cycle of an
// invoice of its own (create, deliver --via pdf, pdf, cancel, archive,
// unarchive) and the cancel of the delivered estimate (deliverPDF). Hard:
// deliver --via pdf, pdf (a file starting with %PDF), archive and
// unarchive exit 0. Measured: the states along the way and cancel, whose
// precondition is not known live.
//
// The invoice is delivered as PDF only (the transport refuses anything
// else), so it gets a number of the invoice sequence of the sandbox: it
// books on the permanent customer (spec v0.2 §2), and the cleanup only
// archives it once delivered (archiveOnly). After the deliver, invoice
// paid-status of that customer (hard: exit 0 and paid-status.json)
// measures open point 30 (outstanding_amount in the list); after the
// cancel, the invoice list of that customer measures open point 31 (the
// cancellation invoice) and registers it for the cleanup when it is the
// only candidate.
func TestE2E_InvoiceFlow(t *testing.T) {
	s := live(t)

	// 1. create (50)
	c, pt := s.permanentCustomer(t), idOf(s.paymentTerms(t)[0])
	name := s.prefix + "rechnung-versand"
	s.need(t, 50)
	inv := s.create(t, object{resource: "invoice", name: name},
		"invoice", "create", "--name", name, "--customer", ids(c), "--payment-term", ids(pt),
		"--document-date", "today", "--item", "name="+pos19+",qty=1,price=100,vat=19")
	id := ids(idOf(inv))
	s.measuref(t, "Statuswerte Rechnung", "nach Anlegen: state %q", str(inv["state"]))

	// 2. deliver --via pdf: send and readback (2), hard exit 0
	s.need(t, 2)
	r := s.run(t, "", "invoice", "deliver", id, "--via", "pdf", "--yes", "-o", "json")
	if r.code != 0 {
		s.quotaSkip(t, r, []string{"invoice", "deliver"})
		s.errorf(t, "invoice deliver --via pdf: Exit %d\n%s", r.code, r.stderr)
	} else {
		s.archiveOnly("invoice", idOf(inv))
		d := s.decode(t, r.stdout)
		s.measuref(t, "Statuswerte Rechnung", "nach deliver --via pdf: state %v, invoice_no %v, sent_on %v, sent_via %v",
			d["state"], d["invoice_no"], d["sent_on"], d["sent_via"])

		// 2a. only after a deliver with exit 0: paid-status of the
		// permanent customer (1), hard exit 0 and the contract;
		// measured: the own invoice in it and the list entries
		s.need(t, 1)
		s.paidStatusAfterDeliver(t, c, idOf(inv))
	}

	// 3. pdf (1), hard: exit 0 and a PDF file
	s.need(t, 1)
	file := filepath.Join(t.TempDir(), "rechnung.pdf")
	r = s.run(t, "", "invoice", "pdf", id, "-O", file, "-o", "json")
	if x := lastExchange(r, "GET", "/pdf"); x != nil {
		s.measuref(t, "invoice pdf", "HTTP %d, Content-Type %q, %d Bytes", x.Status, x.RespHeader.Get("Content-Type"), len(x.RespBody))
	}
	s.quotaSkip(t, r, []string{"invoice", "pdf"})
	data, err := os.ReadFile(file)
	switch {
	case r.code != 0:
		s.errorf(t, "invoice pdf: Exit %d\n%s", r.code, r.stderr)
	case err != nil:
		s.errorf(t, "invoice pdf: Datei nicht lesbar: %v", err)
	case !bytes.HasPrefix(data, []byte("%PDF")):
		s.errorf(t, "invoice pdf: Datei ist kein PDF (%d Bytes)", len(data))
	}

	// 4. cancel and describe (2), measured
	s.need(t, 2)
	s.cancelMeasured(t, "invoice", "cancel", id)
	m := s.ok(t, "", "invoice", "describe", id)
	s.measuref(t, "Statuswerte Rechnung", "nach cancel: state %q", str(m["state"]))

	// 4a. the cancellation invoice in the list of the customer (1),
	// measured
	s.need(t, 1)
	s.findCancellation(t, c, idOf(inv))

	// 5. archive, describe, unarchive (3), hard exit 0
	s.need(t, 3)
	if a := s.run(t, "", "invoice", "archive", id, "-o", "json"); a.code != 0 {
		s.quotaSkip(t, a, []string{"invoice", "archive"})
		s.errorf(t, "invoice archive: Exit %d\n%s", a.code, a.stderr)
	} else {
		m = s.ok(t, "", "invoice", "describe", id)
		s.measuref(t, "invoice archive", "nach archive: record_state %q", str(m["record_state"]))
		if u := s.run(t, "", "invoice", "unarchive", id, "-o", "json"); u.code != 0 {
			s.quotaSkip(t, u, []string{"invoice", "unarchive"})
			s.errorf(t, "invoice unarchive: Exit %d\n%s", u.code, u.stderr)
		}
	}

	// 6. cancel the delivered estimate and describe (2), measured; the
	// estimate exists already in a full run (memoized)
	est := ids(s.deliverPDF(t).estimate)
	s.need(t, 2)
	s.cancelMeasured(t, "estimate", "cancel", est)
	m = s.ok(t, "", "estimate", "describe", est)
	s.measuref(t, "Statuswerte Angebot", "nach deliver --via pdf und cancel: state %q", str(m["state"]))
}

// paidStatusAfterDeliver runs invoice paid-status for customer (an ID:
// no name resolution) from today (Berlin: the date of the invoice), so
// the list stays one page however many runs booked on the permanent
// customer, after the deliver of invoice. Hard:
// exit 0 and docs/schemas/paid-status.json. Measured (open point 30):
// whether the invoice is an open item, with state and outstanding_amount,
// and how many entries of the list the API sent carry outstanding_amount.
// The invoice missing from the open items is an ABWEICHUNG. The list
// entries are measured before the hard checks: a list without
// outstanding_amount ends paid-status with exit 1.
func (s *liveSuite) paidStatusAfterDeliver(t *testing.T, customer, invoice int64) {
	t.Helper()
	r := s.run(t, "", "invoice", "paid-status", "--customer", ids(customer), "--from", s.todayBerlin(), "-o", "json")
	s.measureInvoiceList(t, "paid-status nach Versand", r, invoice)
	if r.code != 0 {
		s.quotaSkip(t, r, []string{"invoice", "paid-status"})
		s.errorf(t, "invoice paid-status --customer %d: Exit %d\n%s", customer, r.code, r.stderr)
		return
	}
	if v := s.checkContract(t, "paid-status", r.stdout); v != "" {
		s.errorf(t, "invoice paid-status %s", v)
	}
	out := s.decode(t, r.stdout)
	for _, e := range entries(out) {
		if idOf(e) == invoice {
			s.measuref(t, "paid-status nach Versand", "eigene Rechnung %d enthalten: ja, state %q, outstanding_amount %v, days_overdue %v",
				invoice, str(e["state"]), e["outstanding_amount"], e["days_overdue"])
			return
		}
	}
	s.measuref(t, "paid-status nach Versand", "eigene Rechnung %d enthalten: nein (count %v)", invoice, out["count"])
	s.deviatef(t, "invoice paid-status: versendete Rechnung %d fehlt in den offenen Posten", invoice)
}

// measureInvoiceList logs, for the invoice list pages r received, how many
// entries carry outstanding_amount (not null) and how many do not, and
// state, outstanding_amount and its JSON type of the entry of invoice.
func (s *liveSuite) measureInvoiceList(t *testing.T, topic string, r result, invoice int64) {
	t.Helper()
	with, without, pages := 0, 0, 0
	own := "nicht in der Liste"
	for _, x := range r.exchanges {
		if x.Method != "GET" || !strings.HasSuffix(strings.TrimRight(x.Path, "/"), "/income/invoices") {
			continue
		}
		pages++
		for _, e := range entries(tryDecode(string(x.RespBody))) {
			v, ok := e["outstanding_amount"]
			if ok && v != nil {
				with++
			} else {
				without++
			}
			if idOf(e) == invoice {
				own = fmt.Sprintf("state %q, outstanding_amount %v (%s)", str(e["state"]), v, jsonType(v))
			}
		}
	}
	s.measuref(t, topic, "Rechnungsliste (%d Seite(n)): %d Einträge mit outstanding_amount, %d ohne; eigene Rechnung %d: %s",
		pages, with, without, invoice, own)
}

// findCancellation looks for the cancellation invoice of invoice (open
// point 31) in the invoice list of customer of today (Berlin), newest
// first: candidates are invoices with a higher id that this run did not
// register. Measured: name, state, invoice_no, type and key shape of
// each. Exactly one is registered for the cleanup as follow-up of
// invoice (archiveOnly: it has a number) with the type of its entry, when
// that is the type of an invoice (e2eguard.IsFollowUpType, as followUp
// does; remove checks it again); none is a measurement, more than one an
// ABWEICHUNG, and none of them is registered (never a choice).
//
// The heuristic rests on assumptions the live run has to measure (open
// point 31):
//
//	(a) the cancellation is an object of its own (not only a state);
//	(b) it has the company_id of the cancelled invoice;
//	(c) its document_date is today;
//	(d) ids rise monotonically, so it has a higher id;
//	(e) no other invoice of the permanent customer appears today (no
//	    parallel run, no human);
//	(f) the list can be sorted by id (otherwise: unsorted, the list is
//	    limited to one day and one customer anyway, 1 credit more);
//	(g) it can be archived;
//	(h) refuted in live run 6: it carries the name of the original
//	    invoice (with the pk-e2e- prefix); the search does not depend on
//	    the name, and the leftover search of a later run archives it as
//	    a numbered leftover.
func (s *liveSuite) findCancellation(t *testing.T, customer, invoice int64) {
	t.Helper()
	day := s.todayBerlin()
	args := []string{"invoice", "list", "--company", ids(customer), "--from", day, "--to", day, "--limit", "100", "-o", "json"}
	r := s.run(t, "", append(args, "--sort", "id", "--desc")...)
	if r.code != 0 {
		s.quotaSkip(t, r, args)
		// (f): without sorting, the candidates are found by id anyway
		s.measuref(t, "Storno-Rechnung", "invoice list --sort id: Exit %d, ohne Sortierung erneut", r.code)
		s.need(t, 1)
		r = s.run(t, "", args...)
	}
	if r.code != 0 {
		s.quotaSkip(t, r, args)
		s.deviatef(t, "Storno-Suche: invoice list Exit %d, nicht gemessen", r.code)
		return
	}
	var found []map[string]any
	for _, e := range entries(s.decode(t, r.stdout)) {
		if idOf(e) > invoice && !s.tracked("invoice", idOf(e)) {
			found = append(found, e)
			s.measuref(t, "Storno-Rechnung", "Kandidat %d: name %q, state %q, invoice_no %v, type %q, Schlüssel %s",
				idOf(e), str(e["name"]), str(e["state"]), e["invoice_no"], str(e["type"]), keyShape(e))
		}
	}
	switch len(found) {
	case 0:
		s.measuref(t, "Storno-Rechnung", "kein Folgeobjekt in der Liste (Rechnung %d)", invoice)
	case 1:
		f := found[0]
		typ := str(f["type"])
		if !e2eguard.IsFollowUpType("invoice", typ) {
			s.deviatef(t, "Storno-Suche: Kandidat %d mit Typ %q, nicht angefasst", idOf(f), typ)
			return
		}
		name := str(f["name"])
		if name == "" {
			name = s.prefix + "folgeobjekt-" + ids(idOf(f))
		}
		s.track(object{resource: "invoice", id: idOf(f), name: name, followUpOf: invoice, followUpType: typ, archiveOnly: true})
		s.measuref(t, "Storno-Rechnung", "Storno-Rechnung gefunden: %d zu Rechnung %d, zum Archivieren registriert", idOf(f), invoice)
	default:
		var all []string
		for _, f := range found {
			all = append(all, ids(idOf(f)))
		}
		s.deviatef(t, "Storno-Suche: %d Kandidaten (%s) für Rechnung %d, keiner registriert", len(found), strings.Join(all, ", "), invoice)
	}
}

// cancelPaths are the path suffixes of the status changes cancelMeasured
// runs, by CLI action.
var cancelPaths = map[string]string{"cancel": "/cancel", "reverse": "/cancel_with_reverse_entry"}

// cancelMeasured runs the status change action (cancel or reverse) of
// resource id (1 credit) and logs exit, HTTP status and API message:
// whether it is allowed in a state is not known live, so the outcome is a
// measurement, never a failure. A 2xx answer naming another object is a
// follow-up object (followUp).
func (s *liveSuite) cancelMeasured(t *testing.T, resource, action, id string) {
	t.Helper()
	r := s.run(t, "", resource, action, id, "--yes", "-o", "json")
	status, msg := 0, strings.TrimSpace(r.stderr)
	if x := lastExchange(r, "POST", cancelPaths[action]); x != nil {
		status, msg = x.Status, apiMessage(x.RespBody)
		if x.Status >= 200 && x.Status <= 299 {
			s.followUp(t, resource, action, id, x.RespBody)
		}
	}
	s.measuref(t, resource+" "+action, "Exit %d, HTTP %d, Meldung %q", r.code, status, msg)
}

// followUp handles the answer body of a status change of resource target:
// a cancel or reverse may create a new object with an id of its own (a
// cancellation document, a reverse entry; e2eguard.JudgeFollowUp). Such
// an object is logged as ABWEICHUNG (the CLI shows the answer as is and
// says nothing about a new object). Only when its type is that of
// resource (same ID space) is it registered for the cleanup, under its
// own name or <prefix>folgeobjekt-<id>; remove touches it although the API
// chose its name, because the id comes verbatim from this answer. Any
// other follow-up is left alone.
func (s *liveSuite) followUp(t *testing.T, resource, action, target string, body []byte) {
	t.Helper()
	of, err := strconv.ParseInt(target, 10, 64)
	if err != nil || of <= 0 {
		s.logf(t, "%s %s: Ziel-ID %q ist keine ID, Folgeobjekt nicht geprüft", resource, action, target)
		return
	}
	f, ok := e2eguard.JudgeFollowUp(resource, of, body)
	if !ok {
		return
	}
	if !f.Track {
		s.deviatef(t, "%s %s %s: Folgeobjekt %d Typ %q, nicht angefasst", resource, action, target, f.ID, f.Type)
		return
	}
	name := f.Name
	if name == "" {
		name = s.prefix + "folgeobjekt-" + ids(f.ID)
	}
	s.deviatef(t, "%s %s %s: Antwort nennt ein anderes Objekt %d (Folgeobjekt), zum Aufräumen registriert", resource, action, target, f.ID)
	s.track(object{resource: resource, id: f.ID, name: name, followUpOf: of, followUpType: f.Type})
}
