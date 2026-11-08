//go:build e2e

package e2e

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// TestE2E_OpenMeasurements measures the open points 28, 10, 13 and 12 of
// docs/offene-punkte.md, one subtest each. All of them are measurements:
// a result that differs from what the CLI assumes is logged as ABWEICHUNG
// (deviatef) and never fails the run; only a read before the PUT of the
// round trip in no. 13 fails it (CLI behaviour, not a value of the API). A
// step that answers with 429 (exit 6) ends its subtest without a further
// request and stops all later tests (markQuota).
func TestE2E_OpenMeasurements(t *testing.T) {
	s := live(t)

	// 28. POST /income/invoices without document_date: the CLI treats the
	// field as optional for invoices (the estimate requires it, measured
	// 2026-11-04). Accepted: the invoice is registered for the cleanup and
	// its document_date logged; accepted without an id in the output (no
	// JSON, or e.g. {"ok":true}) is reported as such, the next run's
	// CleanupLeftovers removes it. Rejected (422 expected like the
	// estimate): exit, HTTP status and message of the API; a 422 is an
	// ABWEICHUNG.
	t.Run("28_InvoiceWithoutDocumentDate", func(t *testing.T) {
		c, pt := s.customer(t), idOf(s.paymentTerms(t)[0])
		s.need(t, 50)
		name := s.prefix + "rechnung-ohne-datum"
		r := s.run(t, "", "invoice", "create", "--name", name, "--customer", ids(c), "--payment-term", ids(pt),
			"--item", "name="+pos19+",qty=1,price=100,vat=19", "-o", "json")
		m := tryDecode(r.stdout)
		id := idOf(m)
		if id != 0 {
			s.track(object{resource: "invoice", id: id, name: name})
			s.logf(t, "angelegt: invoice %d (%s)", id, name)
		}
		if r.code == 0 {
			if id == 0 {
				what := "keine JSON-Ausgabe"
				if m != nil {
					what = "Ausgabe ohne id"
				}
				s.measuref(t, "Rechnung ohne document_date", "angenommen (Exit 0, %s), aber %s: id unbekannt, CleanupLeftovers des nächsten Laufs räumt sie auf",
					outcome(r, "POST", "/income/invoices"), what)
				return
			}
			s.measuref(t, "Rechnung ohne document_date", "angenommen (%s): document_date %v (%s)",
				outcome(r, "POST", "/income/invoices"), m["document_date"], jsonType(m["document_date"]))
			return
		}
		s.measuref(t, "Rechnung ohne document_date", "Exit %d, %s", r.code, outcome(r, "POST", "/income/invoices"))
		if r.code == 12 {
			s.logf(t, "Rechnung %s womöglich angelegt, id unbekannt: CleanupLeftovers des nächsten Laufs räumt sie auf", name)
		}
		if x := lastExchange(r, "POST", "/income/invoices"); x != nil && x.Status == 422 {
			s.deviatef(t, "POST /income/invoices ohne document_date: HTTP 422 (%s); das CLI behandelt das Feld bei Rechnungen als optional (Overlay wie beim Angebot ergänzen)",
				apiMessage(x.RespBody))
		}
	})

	// 10. due_date through UpdateInvoice: its request schema has no
	// due_date. papierkram api PUT (without --validate) sends the body as
	// given: due_date today + 45 days with the current payment term of
	// the draft invoice. Logged: HTTP status and due_date before and
	// after; a changed due_date is an ABWEICHUNG (invoice update --input
	// removes the field as read only). Should api end locally with exit 5
	// (nothing sent), that is logged as "lokal abgelehnt". After exit 12
	// (PUT sent, outcome unknown) the second describe still runs: due_date
	// before and after tells whether the PUT arrived.
	t.Run("10_InvoiceDueDatePut", func(t *testing.T) {
		inv := idOf(s.invoice(t))
		s.need(t, 3)
		path := "/income/invoices/" + ids(inv)
		before := s.ok(t, "", "invoice", "describe", ids(inv))
		pt := idOf(get(before, "payment_term"))
		if pt == 0 {
			s.fatalf(t, "Rechnung %d ohne payment_term.id: Messung nicht möglich", inv)
		}
		due := time.Now().In(s.loc).AddDate(0, 0, 45).Format(time.DateOnly)
		body := fmt.Sprintf(`{"due_date":%q,"payment_term":{"id":%d}}`, due, pt)
		r := s.run(t, body, "api", "PUT", path, "--input", "-", "-o", "json")
		x := lastExchange(r, "PUT", path)
		switch {
		case x == nil && r.code == 5:
			s.measuref(t, "due_date per PUT", "lokal abgelehnt (Exit 5, nichts gesendet): %s", firstLine(r.stderr))
			return
		case x == nil && r.code != 12:
			s.measuref(t, "due_date per PUT", "keine Antwort (Exit %d): %s", r.code, firstLine(r.stderr))
			return
		case s.quotaStop(t, r):
			return
		}
		after := s.ok(t, "", "invoice", "describe", ids(inv))
		b, a := str(before["due_date"]), str(after["due_date"])
		s.measuref(t, "due_date per PUT", "gesendet due_date %s mit payment_term %d → Exit %d, %s; due_date vorher %q, nachher %q",
			due, pt, r.code, outcome(r, "PUT", path), b, a)
		if a != b {
			s.deviatef(t, "due_date per PUT /income/invoices geändert (gesendet %s, vorher %q, nachher %q); das CLI entfernt das Feld aus --input als nur lesbar",
				due, b, a)
		}
	})

	// 13. supply_date: which format does the API answer with, and does
	// the documented round trip (describe -o json | update --input -)
	// send it back unchanged? Set with --supply-date (2: the update reads
	// the invoice for its payment term), describe (1), the round trip
	// with exactly that output (1: payment_term in the body, no read),
	// describe (1). A failed step or a changed supply_date is an
	// ABWEICHUNG. A read before the PUT of the round trip is a failure:
	// with payment_term in the body the CLI must not read the invoice.
	t.Run("13_InvoiceSupplyDate", func(t *testing.T) {
		inv := idOf(s.invoice(t))
		s.need(t, 5)
		path := "/income/invoices/" + ids(inv)
		const supply = "2026-11-01"
		r := s.run(t, "", "invoice", "update", ids(inv), "--supply-date", supply, "-o", "json")
		if r.code != 0 {
			if !s.quotaStop(t, r) {
				s.deviatef(t, "invoice update --supply-date %s: Exit %d, %s", supply, r.code, outcome(r, "PUT", path))
			}
			return
		}
		desc := s.run(t, "", "invoice", "describe", ids(inv), "-o", "json")
		if desc.code != 0 {
			s.quotaSkip(t, desc, []string{"invoice", "describe"})
			s.fatalf(t, "invoice describe: Exit %d\n%s", desc.code, desc.stderr)
		}
		got := s.decode(t, desc.stdout)["supply_date"]
		s.measuref(t, "supply_date", "gesendet %q, Antwort %q (%s, Form %s)", supply, str(got), jsonType(got), dateShape(str(got)))
		if !strings.HasPrefix(str(got), supply) {
			s.deviatef(t, "supply_date gesendet %s, Antwort %q", supply, str(got))
		}
		rt := s.run(t, desc.stdout, "invoice", "update", ids(inv), "--input", "-", "-o", "json")
		reads := countMethod(rt, "GET")
		if x := lastExchange(rt, "PUT", path); x != nil {
			s.measuref(t, "supply_date", "Rundlauf: gesendet supply_date %q, %d Leseanfrage(n) davor, HTTP %d",
				str(get(lastBody(x), "supply_date")), reads, x.Status)
		}
		if reads != 0 {
			s.errorf(t, "Rundlauf mit payment_term im Body: %d Leseanfrage(n) vor dem PUT, erwartet 0 (invoice update liest nur ohne payment_term)", reads)
		}
		if rt.code != 0 {
			if !s.quotaStop(t, rt) {
				s.deviatef(t, "Rundlauf mit supply_date %q gescheitert: Exit %d, %s", str(got), rt.code, outcome(rt, "PUT", path))
			}
			return
		}
		after := s.ok(t, "", "invoice", "describe", ids(inv))
		s.measuref(t, "supply_date", "nach dem Rundlauf %q (vorher %q)", str(after["supply_date"]), str(got))
		if str(after["supply_date"]) != str(got) {
			s.deviatef(t, "Rundlauf ändert supply_date: %q → %q", str(got), str(after["supply_date"]))
		}
	})

	// 12. entertainment_persons: set one person, then send [] (accepted
	// per TestE2E_Carries/04): does [] clear the list or leave it
	// unchanged? Rejecting the person ends the subtest (measured);
	// rejecting [] is an ABWEICHUNG.
	t.Run("12_VoucherEntertainmentPersons", func(t *testing.T) {
		v := idOf(s.voucher(t))
		s.need(t, 4)
		path := "/expense/vouchers/" + ids(v)
		guest := s.prefix + "gast"
		set := s.run(t, fmt.Sprintf(`{"entertainment_persons":[%q]}`, guest), "voucher", "update", ids(v), "--input", "-", "-o", "json")
		if set.code != 0 {
			s.measuref(t, "entertainment_persons", "[%q] abgelehnt: Exit %d, %s", guest, set.code, outcome(set, "PUT", path))
			return
		}
		m := s.ok(t, "", "voucher", "describe", ids(v))
		stored := slices.Contains(asSlice(m["entertainment_persons"]), any(guest))
		s.measuref(t, "entertainment_persons", "[%q] angenommen, danach %v", guest, m["entertainment_persons"])
		clr := s.run(t, `{"entertainment_persons":[]}`, "voucher", "update", ids(v), "--input", "-", "-o", "json")
		if clr.code != 0 {
			if !s.quotaStop(t, clr) {
				s.deviatef(t, "PUT mit entertainment_persons [] abgelehnt: Exit %d, %s", clr.code, outcome(clr, "PUT", path))
			}
			return
		}
		m = s.ok(t, "", "voucher", "describe", ids(v))
		after := asSlice(m["entertainment_persons"])
		meaning := "anders"
		switch {
		case !stored:
			meaning = "nicht unterscheidbar (Person vorher nicht gespeichert)"
		case len(after) == 0:
			meaning = "geleert"
		case slices.Contains(after, any(guest)):
			meaning = "unverändert"
		}
		s.measuref(t, "entertainment_persons", "[] angenommen: %s, danach %v", meaning, m["entertainment_persons"])
	})
}

// quotaStop reports whether r hit the monthly quota (e2eguard.QuotaHit:
// exit 6, or a 429 of any of its requests) and then logs that the
// measurement stops: no further request after an exhausted quota.
func (s *liveSuite) quotaStop(t *testing.T, r result) bool {
	t.Helper()
	if !e2eguard.QuotaHit(r.code, r.exchanges) {
		return false
	}
	s.measuref(t, "Kontingent", "HTTP 429 (Exit 6): Messung abgebrochen, keine weitere Anfrage")
	return true
}

// outcome is the answer of the last request of r with method and path
// suffix: "HTTP n" (with the message of the API for a non-2xx status),
// or "keine Antwort" with the first line of stderr (not sent, or sent
// without an answer: exit 12).
func outcome(r result, method, suffix string) string {
	x := lastExchange(r, method, suffix)
	switch {
	case x == nil:
		return "keine Antwort: " + firstLine(r.stderr)
	case x.Status/100 == 2:
		return fmt.Sprintf("HTTP %d", x.Status)
	}
	return fmt.Sprintf("HTTP %d: %s", x.Status, apiMessage(x.RespBody))
}

// firstLine is the first non-empty line of s.
func firstLine(s string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// countMethod is the number of requests of r with method.
func countMethod(r result, method string) int {
	n := 0
	for _, x := range r.exchanges {
		if x.Method == method {
			n++
		}
	}
	return n
}

// dateShape is the form of a date value with every digit as 9
// ("9999-99-99 99:99:99 +9999"); "leer" for an empty one.
func dateShape(v string) string {
	if v == "" {
		return "leer"
	}
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '9'
		}
		return r
	}, v)
}
