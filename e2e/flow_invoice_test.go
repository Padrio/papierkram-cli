//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Spec 9.1, 9.2, 8.1, 17 (open points 1 and 16): the life cycle of an
// invoice of its own (create, deliver --via pdf, pdf, cancel, archive,
// unarchive) and the cancel of the delivered estimate (deliverPDF). Hard:
// deliver --via pdf, pdf (a file starting with %PDF), archive and
// unarchive exit 0. Measured: the states along the way and cancel, whose
// precondition is not known live.
//
// The invoice is delivered as PDF only (the transport refuses anything
// else), so it gets a number of the invoice sequence of the sandbox.
func TestE2E_InvoiceFlow(t *testing.T) {
	s := live(t)

	// 1. create (50)
	c, pt := s.customer(t), idOf(s.paymentTerms(t)[0])
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
		s.errorf(t, "invoice deliver --via pdf: Exit %d\n%s", r.code, r.stderr)
	} else {
		d := s.decode(t, r.stdout)
		s.measuref(t, "Statuswerte Rechnung", "nach deliver --via pdf: state %v, invoice_no %v, sent_on %v, sent_via %v",
			d["state"], d["invoice_no"], d["sent_on"], d["sent_via"])
	}

	// 3. pdf (1), hard: exit 0 and a PDF file
	s.need(t, 1)
	file := filepath.Join(t.TempDir(), "rechnung.pdf")
	r = s.run(t, "", "invoice", "pdf", id, "-O", file, "-o", "json")
	if x := lastExchange(r, "GET", "/pdf"); x != nil {
		s.measuref(t, "invoice pdf", "HTTP %d, Content-Type %q, %d Bytes", x.Status, x.RespHeader.Get("Content-Type"), len(x.RespBody))
	}
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

	// 5. archive, describe, unarchive (3), hard exit 0
	s.need(t, 3)
	if a := s.run(t, "", "invoice", "archive", id, "-o", "json"); a.code != 0 {
		s.errorf(t, "invoice archive: Exit %d\n%s", a.code, a.stderr)
	} else {
		m = s.ok(t, "", "invoice", "describe", id)
		s.measuref(t, "invoice archive", "nach archive: record_state %q", str(m["record_state"]))
		if u := s.run(t, "", "invoice", "unarchive", id, "-o", "json"); u.code != 0 {
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
// cancellation document, a reverse entry). When the answer is an object
// whose id differs from target, that is logged as ABWEICHUNG (the CLI
// shows the answer as is and says nothing about a new object) and the
// object is registered for the cleanup under its own name, or
// <prefix>folgeobjekt-<id> without one. remove touches it although the
// API chose its name, because the id comes verbatim from this answer.
func (s *liveSuite) followUp(t *testing.T, resource, action, target string, body []byte) {
	t.Helper()
	m := tryDecode(string(body))
	id := idOf(m)
	if id == 0 || ids(id) == target {
		return
	}
	of, _ := strconv.ParseInt(target, 10, 64)
	name := str(m["name"])
	if name == "" {
		name = s.prefix + "folgeobjekt-" + ids(id)
	}
	s.deviatef(t, "%s %s %s: Antwort nennt ein anderes Objekt %d (Folgeobjekt), zum Aufräumen registriert", resource, action, target, id)
	s.track(object{resource: resource, id: id, name: name, followUpOf: of})
}
