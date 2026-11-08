//go:build e2e

package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// Shared objects of a run. Each is created on first use (so -run works
// for single tests), registered for the cleanup and named with the prefix
// of the run. Invoices, estimates and vouchers are created as drafts and
// never sent by mail.

// Fixed values of the fixtures, used by the measurements.
const (
	// pos19 is the name of the 19 % line item of estimate and invoice.
	pos19 = "pk-e2e-pos-19"
	// normalDay is the date of the time entry fixture: a Wednesday in
	// summer time, far from a clock change (carry 11).
	normalDay = "2026-06-10"
	// timeFrom, timeTo span 90 minutes; billable is the billable_duration
	// sent with them, in seconds (carry 6).
	timeFrom, timeTo = "09:00", "10:30"
	timeDuration     = 5400
	billable         = 1800
)

// create runs a create command, registers the new object, keeps the sent
// body (sentBody) and returns the API object.
func (s *liveSuite) create(t *testing.T, o object, args ...string) map[string]any {
	t.Helper()
	m, _ := s.createRun(t, o, args...)
	return m
}

// createRun is create that also returns the CLI run. A create that fails
// after the object exists (exit 11 of voucher create --file: the partial
// output names its id) still registers the object before the test fails,
// so the cleanup removes it. The sent body is that of the first POST (the
// create; an upload of --file follows it).
func (s *liveSuite) createRun(t *testing.T, o object, args ...string) (map[string]any, result) {
	t.Helper()
	r := s.run(t, "", append(args, "-o", "json")...)
	if r.code != 0 {
		if id := idOf(tryDecode(r.stdout)); id != 0 {
			o.id = id
			s.track(o)
			s.logf(t, "angelegt trotz Exit %d: %s %d (%s)", r.code, o.resource, o.id, o.name)
		}
		s.quotaSkip(t, r, args)
		s.fatalf(t, "papierkram %s: Exit %d\n%s", strings.Join(args, " "), r.code, r.stderr)
	}
	m := s.decode(t, r.stdout)
	o.id = idOf(m)
	if o.id == 0 {
		s.fatalf(t, "%s: Antwort ohne id", o.resource)
	}
	s.track(o)
	if x := firstExchange(r, "POST"); x != nil {
		s.sent[o.resource+"/"+ids(o.id)] = x.ReqBody
	}
	s.logf(t, "angelegt: %s %d (%s)", o.resource, o.id, o.name)
	return m, r
}

// user is the first user of the account (time entries need one).
func (s *liveSuite) user(t *testing.T) int64 {
	return idOf(s.fixture(t, "user", func() map[string]any {
		s.need(t, 1)
		es := entries(s.ok(t, "", "user", "list", "--limit", "100"))
		if len(es) == 0 {
			s.fatalf(t, "keine Benutzer im Account")
		}
		s.logf(t, "Benutzer %d (Rolle %s) für Zeiteinträge", idOf(es[0]), str(es[0]["role"]))
		return es[0]
	}))
}

// paymentTerms is the first page of payment terms (read only).
func (s *liveSuite) paymentTerms(t *testing.T) []map[string]any {
	m := s.fixture(t, "payment-terms", func() map[string]any {
		s.need(t, 1)
		return s.ok(t, "", "payment-term", "list", "--limit", "100")
	})
	es := entries(m)
	if len(es) == 0 {
		s.fatalf(t, "keine Zahlungsbedingungen im Account")
	}
	return es
}

func (s *liveSuite) customer(t *testing.T) int64 {
	return idOf(s.fixture(t, "customer", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "kunde"
		return s.create(t, object{resource: "company", name: name},
			"company", "create", "--name", name, "--contact-type", "customer")
	}))
}

func (s *liveSuite) supplier(t *testing.T) int64 {
	return idOf(s.fixture(t, "supplier", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "lieferant"
		return s.create(t, object{resource: "company", name: name},
			"company", "create", "--name", name, "--contact-type", "supplier")
	}))
}

// stornoSupplier is the creditor of TestE2E_VoucherFlow only: its voucher
// is reversed and cancelled, so this company can at most be archived in
// the cleanup, while s.supplier stays deletable (DeleteCompany 2xx,
// ruling FR-C).
func (s *liveSuite) stornoSupplier(t *testing.T) int64 {
	return idOf(s.fixture(t, "storno-supplier", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "lieferant-storno"
		return s.create(t, object{resource: "company", name: name},
			"company", "create", "--name", name, "--contact-type", "supplier")
	}))
}

// person is a contact person of the customer, created with --default
// (sent as the string "true", spec 4.4).
func (s *liveSuite) person(t *testing.T) map[string]any {
	c := s.customer(t)
	return s.fixture(t, "person", func() map[string]any {
		s.need(t, 10)
		first := s.prefix + "person"
		return s.create(t, object{resource: "person", name: first, company: c},
			"person", "create", "--company", ids(c), "--first-name", first, "--last-name", "Testperson", "--default")
	})
}

func (s *liveSuite) project(t *testing.T) int64 {
	c := s.customer(t)
	return idOf(s.fixture(t, "project", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "projekt"
		return s.create(t, object{resource: "project", name: name},
			"project", "create", "--name", name, "--customer", ids(c))
	}))
}

func (s *liveSuite) task(t *testing.T) int64 {
	p := s.project(t)
	return idOf(s.fixture(t, "task", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "aufgabe"
		return s.create(t, object{resource: "task", name: name},
			"task", "create", "--name", name, "--project", ids(p))
	}))
}

// proposition is an article with vat_rate 19 (sent as "0.19", spec 6.2).
func (s *liveSuite) proposition(t *testing.T) map[string]any {
	return s.fixture(t, "proposition", func() map[string]any {
		s.need(t, 10)
		name := s.prefix + "artikel"
		return s.create(t, object{resource: "proposition", name: name},
			"proposition", "create", "--name", name, "--article-no", s.prefix+"art",
			"--time-unit", "hour", "--proposition-type", "service", "--price", "100", "--vat-rate", "19")
	})
}

// estimateArgs creates a net estimate (gross=false) for the customer with
// its contact person and project, dated today (the API requires
// document_date, measured 2026-11-04), and two line items: 19 % and 7 %.
func (s *liveSuite) estimateArgs(t *testing.T, name string) []string {
	c, pers, p := s.customer(t), idOf(s.person(t)), s.project(t)
	return []string{"estimate", "create", "--name", name, "--customer", ids(c),
		"--contact-person", ids(pers), "--customer-project", ids(p),
		"--document-date", "today",
		"--item", "name=" + pos19 + ",qty=2,price=100,vat=19",
		"--item", "name=pk-e2e-pos-7,qty=1,price=50,vat=7"}
}

// estimate is the draft estimate the invoice is built from.
func (s *liveSuite) estimate(t *testing.T) map[string]any {
	args := s.estimateArgs(t, s.prefix+"angebot")
	return s.fixture(t, "estimate", func() map[string]any {
		s.need(t, 50)
		return s.create(t, object{resource: "estimate", name: s.prefix + "angebot"}, args...)
	})
}

// invoiceResult is the CLI run that created the invoice (carry 9 reads
// its request body).
var invoiceResult result

// invoice is a draft invoice created with --from-estimate (spec 9.4),
// dated today: the invoice date never comes from the estimate, and the API
// requires it (measured live run 5, 2026-11-08; docs/offene-punkte.md,
// point 28).
func (s *liveSuite) invoice(t *testing.T) map[string]any {
	e := idOf(s.estimate(t))
	pt := idOf(s.paymentTerms(t)[0])
	return s.fixture(t, "invoice", func() map[string]any {
		s.need(t, 51)
		name := s.prefix + "rechnung"
		args := []string{"invoice", "create", "--from-estimate", ids(e), "--payment-term", ids(pt), "--name", name, "--document-date", "today", "-o", "json"}
		invoiceResult = s.run(t, "", args...)
		if invoiceResult.code != 0 {
			s.fatalf(t, "invoice create --from-estimate: Exit %d\n%s", invoiceResult.code, invoiceResult.stderr)
		}
		m := s.decode(t, invoiceResult.stdout)
		s.track(object{resource: "invoice", id: idOf(m), name: name})
		s.logf(t, "angelegt: invoice %d (%s)", idOf(m), name)
		return m
	})
}

// voucher is a domestic voucher of the supplier with one 19 % line item.
func (s *liveSuite) voucher(t *testing.T) map[string]any {
	sup := s.supplier(t)
	return s.fixture(t, "voucher", func() map[string]any {
		s.need(t, 50)
		name := s.prefix + "beleg"
		return s.create(t, object{resource: "voucher", name: name},
			"voucher", "create", "--name", name, "--creditor", ids(sup), "--provenance", "domestic",
			"--document-date", "today",
			"--item", "name=pk-e2e-pos-19,amount=119,vat=19,category=Bürobedarf")
	})
}

// timeEntry is a time entry of 90 minutes on a normal day with a
// billable_duration of 30 minutes in seconds (carries 6 and 11).
func (s *liveSuite) timeEntry(t *testing.T) map[string]any {
	task, user := s.task(t), s.user(t)
	return s.fixture(t, "time-entry", func() map[string]any {
		s.need(t, 10)
		comment := s.prefix + "zeit"
		return s.create(t, object{resource: "time", name: comment},
			"time", "create", "--entry-date", normalDay, "--started-at-time", timeFrom, "--ended-at-time", timeTo,
			"--task", ids(task), "--user", ids(user), "--billable-duration", strconv.Itoa(billable), "--comments", comment)
	})
}
