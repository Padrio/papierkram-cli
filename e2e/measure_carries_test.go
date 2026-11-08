//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// TestE2E_Carries holds the additional live measurements carried over from
// the reviews of tasks 26 to 39 (task-44-carries.md), one subtest each.
// Which of them fail hard is listed in the header of e2e_test.go.
func TestE2E_Carries(t *testing.T) {
	s := live(t)

	// 1. Task deadline as ISO YYYY-MM-DD (the spec example says
	// "15.05.2020"); the CLI sends ISO (format date). Hard.
	t.Run("01_DeadlineISO", func(t *testing.T) {
		task := s.task(t)
		s.need(t, 2)
		s.ok(t, "", "task", "update", ids(task), "--deadline", "2026-12-31")
		m := s.ok(t, "", "task", "describe", ids(task))
		s.measuref(t, "deadline ISO", "gesendet 2026-12-31, gespeichert %q", str(m["deadline"]))
		if str(m["deadline"]) != "2026-12-31" {
			s.errorf(t, "deadline im ISO-Format nicht übernommen: %q", str(m["deadline"]))
		}
	})

	// 2. invoice describe -o json | invoice update --input -: supply_date
	// in the response format ("… +0200") accepted; contact person and
	// project kept. Run 2 (2026-11-04) sent customer as {id} and the API
	// cleared both; the CLI now moves them below customer (spec.Prune).
	// The keys of the sent body are logged (no values). Hard (documented
	// pipeline, spec 6.3a).
	t.Run("02_InvoiceRoundTrip", func(t *testing.T) {
		inv := idOf(s.invoice(t))
		s.need(t, 3)
		desc := s.run(t, "", "invoice", "describe", ids(inv), "-o", "json")
		if desc.code != 0 {
			s.quotaSkip(t, desc, []string{"invoice", "describe"})
			s.fatalf(t, "invoice describe: Exit %d\n%s", desc.code, desc.stderr)
		}
		before := s.decode(t, desc.stdout)
		r := s.run(t, desc.stdout, "invoice", "update", ids(inv), "--input", "-", "-o", "json")
		x := lastExchange(r, "PUT", "/income/invoices/"+ids(inv))
		if x != nil {
			var body map[string]any
			_ = json.Unmarshal(x.ReqBody, &body)
			s.measuref(t, "Rundlauf Rechnung", "gesendet supply_date %q, Schlüssel %s", str(body["supply_date"]), keyShape(body))
			s.measuref(t, "Rundlauf Rechnung", "gesendet customer.id %q, customer.contact_person.id %q, customer.project.id %q",
				str(get(body, "customer", "id")), str(get(body, "customer", "contact_person", "id")), str(get(body, "customer", "project", "id")))
		}
		if r.code != 0 {
			s.quotaSkip(t, r, []string{"invoice", "update"})
			s.fatalf(t, "describe | update --input -: Exit %d\n%s", r.code, r.stderr)
		}
		after := s.ok(t, "", "invoice", "describe", ids(inv))
		s.measuref(t, "Rundlauf Rechnung", "payment_term vorher %s, nachher %s (gesendet id %q)",
			paymentTermShape(before), paymentTermShape(after), str(get(lastBody(x), "payment_term", "id")))
		for _, k := range []string{"contact_person", "project"} {
			b, a := str(get(before, k, "id")), str(get(after, k, "id"))
			s.measuref(t, "Rundlauf Rechnung", "%s vorher %q, nachher %q", k, b, a)
			if b == "" {
				s.errorf(t, "Rechnung aus dem Angebot ohne %s: Rundlauf nicht prüfbar", k)
			} else if a != b {
				s.errorf(t, "Rundlauf verliert %s (%s → %q)", k, b, a)
			}
		}
	})

	// 3. Estimate PUT with sent_on/sent_to null (what the round trip
	// sends for a draft) accepted. Hard.
	t.Run("03_EstimateNullSentFields", func(t *testing.T) {
		est := idOf(s.estimate(t))
		s.need(t, 3)
		desc := s.run(t, "", "estimate", "describe", ids(est), "-o", "json")
		if desc.code != 0 {
			s.quotaSkip(t, desc, []string{"estimate", "describe"})
			s.fatalf(t, "estimate describe: Exit %d\n%s", desc.code, desc.stderr)
		}
		r := s.run(t, desc.stdout, "estimate", "update", ids(est), "--input", "-", "-o", "json")
		x := lastExchange(r, "PUT", "/income/estimates/"+ids(est))
		if x == nil {
			s.quotaSkip(t, r, []string{"estimate", "update"})
			s.fatalf(t, "nicht geprüft: kein PUT gesendet (Exit %d)\n%s", r.code, r.stderr)
		}
		body := s.decode(t, string(x.ReqBody))
		onSent, onNull := body["sent_on"]
		toSent, toNull := body["sent_to"]
		s.measuref(t, "Angebot sent_on/sent_to null", "gesendet sent_on %v (vorhanden %v), sent_to %v (vorhanden %v), HTTP %d",
			onSent, onNull, toSent, toNull, x.Status)
		if !onNull || onSent != nil || !toNull || toSent != nil {
			s.errorf(t, "nicht geprüft: der PUT enthielt nicht sent_on: null und sent_to: null")
		}
		if r.code != 0 {
			s.quotaSkip(t, r, []string{"estimate", "update"})
			s.errorf(t, "PUT mit sent_on/sent_to null abgelehnt: Exit %d\n%s", r.code, r.stderr)
			return
		}
		m := s.ok(t, "", "estimate", "describe", ids(est))
		s.measuref(t, "Angebot sent_on/sent_to null", "danach sent_on %v, sent_to %v", m["sent_on"], m["sent_to"])
	})

	// 4. Voucher PUT with entertainment_persons [] accepted. Hard; the
	// meaning (cleared or unchanged) is measured.
	t.Run("04_VoucherEmptyEntertainmentPersons", func(t *testing.T) {
		v := idOf(s.voucher(t))
		s.need(t, 2)
		r := s.run(t, `{"entertainment_persons":[]}`, "voucher", "update", ids(v), "--input", "-", "-o", "json")
		if r.code != 0 {
			s.quotaSkip(t, r, []string{"voucher", "update"})
			s.errorf(t, "PUT mit entertainment_persons [] abgelehnt: Exit %d\n%s", r.code, r.stderr)
			return
		}
		m := s.ok(t, "", "voucher", "describe", ids(v))
		s.measuref(t, "Beleg entertainment_persons []", "akzeptiert, danach %v", m["entertainment_persons"])
	})

	// 5. Payment term due_time_unit: which values occur? Every spec example
	// says "day"; the table shows ZIEL and EINHEIT side by side, so another
	// unit needs no code change. It is logged as ABWEICHUNG from the spec
	// examples. Measured.
	t.Run("05_PaymentTermUnits", func(t *testing.T) {
		seen := map[string]int{}
		for _, e := range s.paymentTerms(t) {
			seen[str(e["due_time_unit"])]++
		}
		s.measuref(t, "due_time_unit", "%v", seen)
		for u := range seen {
			if u != "day" && u != "" {
				s.deviatef(t, "due_time_unit %q kommt vor (Spec-Beispiele nur day); die Tabelle zeigt ZIEL und EINHEIT getrennt, keine Codeänderung nötig", u)
			}
		}
	})

	// 6. Time entry billable_duration: seconds in the request (overlay
	// section 15). Measured: run 1 (2026-11-04) stored duration (5400)
	// instead of the 1800 sent with the create, which the CLI cannot
	// change (docs/offene-punkte.md, point 15). It also measures whether
	// an update sets the value afterwards.
	t.Run("06_BillableDurationSeconds", func(t *testing.T) {
		te := idOf(s.timeEntry(t))
		var sent any = "nicht aufgezeichnet"
		if body := s.sentBody(t, "time", te); body != nil {
			sent = body["billable_duration"]
		}
		s.need(t, 3)
		m := s.ok(t, "", "time", "describe", ids(te))
		s.measuref(t, "billable_duration", "Anlegen: gesendet %v (%s), gespeichert %v, duration %v",
			sent, jsonType(sent), m["billable_duration"], m["duration"])
		if str(m["billable_duration"]) != fmt.Sprint(billable) {
			s.deviatef(t, "billable_duration: beim Anlegen gesendet %v, gespeichert %v (duration %v)", sent, m["billable_duration"], m["duration"])
		}
		r := s.run(t, "", "time", "update", ids(te), "--billable-duration", strconv.Itoa(billable), "-o", "json")
		if r.code != 0 {
			s.measuref(t, "billable_duration", "Ändern: Exit %d: %s", r.code, strings.TrimSpace(r.stderr))
			return
		}
		m = s.ok(t, "", "time", "describe", ids(te))
		s.measuref(t, "billable_duration", "Ändern: gesendet %d, gespeichert %v, duration %v", billable, m["billable_duration"], m["duration"])
		if str(m["billable_duration"]) != fmt.Sprint(billable) {
			s.deviatef(t, "billable_duration: per time update gesendet %d, gespeichert %v", billable, m["billable_duration"])
		}
	})

	// 7. deliver --via pdf: does the API set sent_on? (If not, a readback
	// after a timeout always ends with exit 12.) Measured.
	t.Run("07_DeliverPdfSentOn", func(t *testing.T) {
		d := s.deliverPDF(t)
		s.measuref(t, "deliver --via pdf sent_on", "Exit %d, sent_on %v, sent_via %v, state %v, estimate_no %v",
			d.res.code, d.out["sent_on"], d.out["sent_via"], d.out["state"], d.out["estimate_no"])
	})

	// 8. PDF download: Accept "application/pdf, application/json;q=0.9"
	// gives 200 application/pdf. Hard.
	t.Run("08_PDFDownloadAccept", func(t *testing.T) {
		est := idOf(s.estimate(t))
		s.need(t, 1)
		file := filepath.Join(t.TempDir(), "angebot.pdf")
		r := s.run(t, "", "estimate", "pdf", ids(est), "-O", file, "-o", "json")
		x := lastExchange(r, "GET", "/pdf")
		s.quotaSkip(t, r, []string{"estimate", "pdf"})
		if x == nil {
			s.fatalf(t, "kein PDF-Request: Exit %d\n%s", r.code, r.stderr)
		}
		s.measuref(t, "PDF-Download", "Accept %q → HTTP %d, Content-Type %q, %d Bytes",
			x.ReqHeader.Get("Accept"), x.Status, x.RespHeader.Get("Content-Type"), len(x.RespBody))
		data, err := os.ReadFile(file)
		switch {
		case r.code != 0:
			s.errorf(t, "estimate pdf: Exit %d\n%s", r.code, r.stderr)
		case err != nil:
			s.errorf(t, "estimate pdf: Datei nicht lesbar: %v", err)
		case x.ReqHeader.Get("Accept") != "application/pdf, application/json;q=0.9":
			s.errorf(t, "Accept %q", x.ReqHeader.Get("Accept"))
		case x.Status != 200 || !strings.HasPrefix(x.RespHeader.Get("Content-Type"), "application/pdf"):
			s.errorf(t, "Antwort HTTP %d, Content-Type %q", x.Status, x.RespHeader.Get("Content-Type"))
		case !bytes.HasPrefix(data, []byte("%PDF")):
			s.errorf(t, "Datei ist kein PDF (%d Bytes)", len(data))
		}
	})

	// 9. invoice create --from-estimate sends price_gross 0.0 with
	// gross=false: accepted? Hard (the invoice fixture is exactly that).
	t.Run("09_FromEstimatePriceGrossZero", func(t *testing.T) {
		s.invoice(t)
		x := lastExchange(invoiceResult, "POST", "/income/invoices")
		if x == nil {
			s.fatalf(t, "Anlage-Request der Rechnung nicht aufgezeichnet")
		}
		var body struct {
			Gross     any              `json:"gross"`
			LineItems []map[string]any `json:"line_items"`
		}
		_ = json.Unmarshal(x.ReqBody, &body)
		var pg []string
		for _, it := range body.LineItems {
			v, ok := it["price_gross"]
			if !ok {
				pg = append(pg, "fehlt")
				continue
			}
			pg = append(pg, fmt.Sprint(v))
		}
		s.measuref(t, "price_gross bei gross=false", "gesendet gross %v, price_gross %v → HTTP %d (akzeptiert)", body.Gross, pg, x.Status)
		// the question is only answered when exactly that was sent
		zero := len(pg) > 0
		for _, v := range pg {
			if f, err := strconv.ParseFloat(v, 64); err != nil || f != 0 {
				zero = false
			}
		}
		if body.Gross != false || !zero {
			s.errorf(t, "nicht geprüft: gesendet gross %v, price_gross %v statt gross false und price_gross 0 in jeder Position", body.Gross, pg)
		}
	})

	// 10. voucher pay: is value null next to banking_transaction accepted?
	// Measured with a transaction ID that does not exist
	// (e2eguard.NoTransactionID) and no amount (controller ruling: the
	// suite never pays; the transport lets only exactly this body through). amount together with a transaction is not
	// measured live (open point, docs/offene-punkte.md).
	t.Run("10_VoucherPayValueNull", func(t *testing.T) {
		v := idOf(s.voucher(t))
		s.need(t, 1)
		body := fmt.Sprintf(`{"value":null,"banking_transaction":{"id":%d}}`, e2eguard.NoTransactionID)
		r := s.run(t, body, "voucher", "pay", ids(v), "--input", "-", "--yes", "-o", "json")
		x := lastExchange(r, "POST", "/pay")
		if x == nil {
			s.measuref(t, "voucher pay", "value null + banking_transaction: nicht gesendet, Exit %d: %s", r.code, strings.TrimSpace(r.stderr))
			return
		}
		s.measuref(t, "voucher pay", "value null + banking_transaction: Body %s → HTTP %d: %s", x.ReqBody, x.Status, apiMessage(x.RespBody))
	})

	// 11. The API computes the duration as the wall clock difference
	// (measured on a normal day; the day of a clock change is only
	// documented, task 36: time log sends wall clock times). Hard.
	t.Run("11_DurationWallClock", func(t *testing.T) {
		te := idOf(s.timeEntry(t))
		s.need(t, 1)
		m := s.ok(t, "", "time", "describe", ids(te))
		s.measuref(t, "Dauer", "%s %s–%s → duration %v (erwartet %d), started_at %v, ended_at %v",
			normalDay, timeFrom, timeTo, m["duration"], timeDuration, m["started_at"], m["ended_at"])
		s.measuref(t, "Dauer", "Umstellungstag nicht gemessen (nur dokumentiert: 01:30–02:30 am 25.10. ist für die API vermutlich 1 h)")
		if str(m["duration"]) != fmt.Sprint(timeDuration) {
			s.errorf(t, "duration %v, erwartet %d (Wanduhr-Differenz)", m["duration"], timeDuration)
		}
	})

	// 12. Time entry ending at midnight (open point 22, decided): time log
	// --from 23:00 --to 00:00 and --to 24:00 send ended_at_time "24:00".
	// Hard: exit 0 and duration 3600 in the answer of time log (the API
	// object, no further request); measured: ended_at and the sent end.
	// Run 2 (through papierkram api) measured both ends as accepted, each
	// with duration 3600 and ended_at 00:00 of the next day.
	t.Run("12_MidnightEnd", func(t *testing.T) {
		task, user := s.task(t), s.user(t)
		for _, end := range []string{"00:00", "24:00"} {
			s.need(t, 11)
			comment := s.prefix + "mitternacht-" + strings.ReplaceAll(end, ":", "")
			r := s.run(t, "", "time", "log", "--task", ids(task), "--user", ids(user), "--date", normalDay,
				"--from", "23:00", "--to", end, "-m", comment, "-o", "json")
			m := tryDecode(r.stdout)
			if id := idOf(m); id != 0 {
				// registered before a failure too
				s.track(object{resource: "time", id: id, name: comment})
				s.logf(t, "angelegt: time %d (%s)", id, comment)
			}
			sent := "nicht gesendet"
			if x := lastExchange(r, "POST", "/tracker/time_entries"); x != nil {
				sent = fmt.Sprintf("%v", tryDecode(string(x.ReqBody))["ended_at_time"])
			}
			if r.code != 0 || m == nil {
				s.quotaSkip(t, r, []string{"time", "log"})
				s.errorf(t, "time log --from 23:00 --to %s: Exit %d, erwartet 0 (gesendet ended_at_time %s)\n%s", end, r.code, sent, r.stderr)
				continue
			}
			s.measuref(t, "Ende um Mitternacht", "time log --from 23:00 --to %s: gesendet ended_at_time %s, duration %v, started_at %v, ended_at %v",
				end, sent, m["duration"], m["started_at"], m["ended_at"])
			if str(m["duration"]) != "3600" {
				s.errorf(t, "time log --from 23:00 --to %s: duration %v, erwartet 3600", end, m["duration"])
			}
		}
	})

	// 13. GET /info: the spec has no response schema; log its structure.
	t.Run("13_InfoStructure", func(t *testing.T) {
		s.need(t, 1)
		m := s.ok(t, "", "info")
		var paths []string
		shape(m, "", 0, &paths)
		slices.Sort(paths)
		s.measuref(t, "GET /info Struktur", "%s", strings.Join(paths, ", "))
	})

	// 14. invoice update --customer with the invoice's own customer keeps
	// contact person and project (open point 29, task 8): the CLI reads
	// the invoice (with its payment term) and sends both below customer.
	// Run 2 (2026-11-04) measured that a PUT without them clears both.
	// The keys of the sent body are logged (no values). Hard.
	t.Run("14_InvoiceUpdateSameCustomer", func(t *testing.T) {
		inv := idOf(s.invoice(t))
		s.need(t, 4)
		before := s.ok(t, "", "invoice", "describe", ids(inv))
		cust := str(get(before, "customer", "id"))
		if cust == "" {
			s.fatalf(t, "Rechnung ohne customer.id: nicht prüfbar")
		}
		r := s.run(t, "", "invoice", "update", ids(inv), "--customer", cust, "-o", "json")
		if x := lastExchange(r, "PUT", "/income/invoices/"+ids(inv)); x != nil {
			s.measuref(t, "Rechnung gleicher Kunde", "gesendet Schlüssel %s", keyShape(lastBody(x)))
		}
		if r.code != 0 {
			s.quotaSkip(t, r, []string{"invoice", "update"})
			s.fatalf(t, "invoice update --customer (gleicher Kunde): Exit %d\n%s", r.code, r.stderr)
		}
		after := s.ok(t, "", "invoice", "describe", ids(inv))
		for _, k := range []string{"contact_person", "project"} {
			b, a := str(get(before, k, "id")), str(get(after, k, "id"))
			s.measuref(t, "Rechnung gleicher Kunde", "%s vorher %q, nachher %q", k, b, a)
			if b == "" {
				s.errorf(t, "Rechnung aus dem Angebot ohne %s: nicht prüfbar", k)
			} else if a != b {
				s.errorf(t, "invoice update --customer (gleicher Kunde) verliert %s (%s → %q)", k, b, a)
			}
		}
	})
}

// lastBody is the decoded request body of x; nil without x or JSON.
func lastBody(x *e2eguard.Exchange) map[string]any {
	if x == nil {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(x.ReqBody, &m)
	return m
}

// keyShape is the key structure of a JSON object without its values:
// sorted keys, nested objects in braces, arrays of objects by their first
// element in brackets (customer{contact_person{id},id}).
func keyShape(v any) string {
	switch x := v.(type) {
	case map[string]any:
		parts := make([]string, 0, len(x))
		for _, k := range slices.Sorted(maps.Keys(x)) {
			parts = append(parts, k+keyShape(x[k]))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		return "[" + keyShape(x[0]) + "]"
	}
	return ""
}

// shape lists the key paths of v with their JSON types (arrays by their
// first element), down to depth 5.
func shape(v any, path string, depth int, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if depth >= 5 || len(x) == 0 {
			*out = append(*out, path+": object")
			return
		}
		for k, e := range x {
			p := k
			if path != "" {
				p = path + "." + k
			}
			shape(e, p, depth+1, out)
		}
	case []any:
		if len(x) == 0 || depth >= 5 {
			*out = append(*out, path+": array")
			return
		}
		shape(x[0], path+"[]", depth+1, out)
	default:
		*out = append(*out, path+": "+jsonType(v))
	}
}
