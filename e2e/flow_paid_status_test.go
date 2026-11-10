//go:build e2e

package e2e

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// Spec v0.2 §4, §5, §7: invoice paid-status with a name at --customer
// (the name resolution of one flag) against the sandbox. Both runs share
// one directory, so they share the cache of the CLI. Hard: the first run
// exits 0 with paid-status.json, names the permanent customer on stderr
// ("→ <id> (") and sends GET /contact/companies and GET /income/invoices
// with company_id=<id>; the second (--overdue) exits 0 without GET
// /contact/companies and takes the name from the cache. Measured: count,
// overdue_count and complete.
func TestE2E_PaidStatus(t *testing.T) {
	s := live(t)
	id := s.permanentCustomer(t)
	dir := t.TempDir()

	// 1. company list (1 page per 100 companies, at most 5) and the
	// invoice list (1)
	s.need(t, 6)
	args := []string{"invoice", "paid-status", "--customer", e2eguard.PermanentCustomer, "-o", "json"}
	r := s.runIn(t, dir, time.Now, "", args...)
	if r.code != 0 {
		s.quotaSkip(t, r, args)
		s.fatalf(t, "invoice paid-status --customer %s: Exit %d\n%s", e2eguard.PermanentCustomer, r.code, r.stderr)
	}
	if v := s.checkContract(t, "paid-status", r.stdout); v != "" {
		s.errorf(t, "invoice paid-status %s", v)
	}
	if want := "→ " + ids(id) + " ("; !strings.Contains(r.stderr, want) {
		s.errorf(t, "invoice paid-status: Hinweis %q fehlt auf stderr\n%s", want, r.stderr)
	}
	if lastExchange(r, "GET", "/contact/companies") == nil {
		s.errorf(t, "invoice paid-status: keine Anfrage GET /contact/companies (Namensauflösung)")
	}
	if !invoicesOf(r, id) {
		s.errorf(t, "invoice paid-status: keine Anfrage GET /income/invoices mit company_id=%d", id)
	}
	out := s.decode(t, r.stdout)
	s.measuref(t, "paid-status per Name", "count %v, overdue_count %v, complete %v", out["count"], out["overdue_count"], out["complete"])

	// 2. the same with --overdue: the name from the cache, the invoice
	// list (1)
	s.need(t, 1)
	args = []string{"invoice", "paid-status", "--customer", e2eguard.PermanentCustomer, "--overdue", "-o", "json"}
	r = s.runIn(t, dir, time.Now, "", args...)
	if r.code != 0 {
		s.quotaSkip(t, r, args)
		s.fatalf(t, "invoice paid-status --overdue: Exit %d\n%s", r.code, r.stderr)
	}
	if lastExchange(r, "GET", "/contact/companies") != nil {
		s.errorf(t, "invoice paid-status --overdue: GET /contact/companies trotz Zwischenspeicher")
	}
	if !strings.Contains(r.stderr, "aus dem Zwischenspeicher") {
		s.errorf(t, "invoice paid-status --overdue: Name nicht aus dem Zwischenspeicher\n%s", r.stderr)
	}
	if !invoicesOf(r, id) {
		s.errorf(t, "invoice paid-status --overdue: keine Anfrage GET /income/invoices mit company_id=%d", id)
	}
}

// invoicesOf reports whether r sent GET /income/invoices with the filter
// company_id=company.
func invoicesOf(r result, company int64) bool {
	for _, x := range r.exchanges {
		if x.Method != "GET" || !strings.HasSuffix(strings.TrimRight(x.Path, "/"), "/income/invoices") {
			continue
		}
		if q, err := url.ParseQuery(x.Query); err == nil && q.Get("company_id") == ids(company) {
			return true
		}
	}
	return false
}
