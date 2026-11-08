package e2eguard

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

func quotaHeader(consumed, remaining string) http.Header {
	h := http.Header{}
	if consumed != "" {
		h.Set("X-Consumed-Quota", consumed)
	}
	if remaining != "" {
		h.Set("X-Remaining-Quota", remaining)
	}
	return h
}

// X-Consumed-Quota is the cost of the one request it answers, not a
// running total (measured 2026-11-04: three GET /info in a row answered
// 1, 1, 1 while X-Remaining-Quota went down). Every request counts with
// the larger of its documented cost and that header, so equal costs in a
// row are counted each time.
func TestMeterCountsConsumedPerRequest(t *testing.T) {
	m := NewMeter(1000, 0)
	m.Record("GET", "/api/v1/info", quotaHeader("1", "9990"))
	m.Record("GET", "/api/v1/info", quotaHeader("1", "9989"))
	m.Record("POST", "/api/v1/contact/companies", quotaHeader("10", "9979"))
	m.Record("POST", "/api/v1/projects", quotaHeader("10", "9969"))
	m.Record("GET", "/api/v1/users", quotaHeader("1", "9968"))
	if got := m.Spent(); got != 1+1+10+10+1 {
		t.Errorf("spent %d, want 23", got)
	}
	if m.Requests() != 5 || m.Unmeasured() != 0 {
		t.Errorf("requests %d unmeasured %d, want 5 and 0", m.Requests(), m.Unmeasured())
	}
	q := m.Quota()
	if !q.Known || q.Consumed != 1 || q.Remaining != 9968 {
		t.Errorf("quota %+v", q)
	}
}

// The larger value counts: a header below the documented cost (or 0)
// never lowers it, a header above it raises it (and is counted as
// costlier than documented).
func TestMeterCountsMaxOfDocumentedAndConsumed(t *testing.T) {
	m := NewMeter(1000, 0)
	m.Record("POST", "/api/v1/income/invoices", quotaHeader("0", "")) // 50 documented
	m.Record("POST", "/api/v1/income/estimates", quotaHeader("7", ""))
	m.Record("GET", "/api/v1/info", quotaHeader("3", ""))
	if got := m.Spent(); got != 50+50+3 {
		t.Errorf("spent %d, want 103", got)
	}
	if m.Unmeasured() != 0 || m.Costlier() != 1 {
		t.Errorf("unmeasured %d costlier %d, want 0 and 1", m.Unmeasured(), m.Costlier())
	}
}

// Without the header (or with garbage or a negative value) a request
// counts with its documented cost.
func TestMeterFallsBackToDocumentedCost(t *testing.T) {
	m := NewMeter(1000, 0)
	m.Record("POST", "/api/v1/expense/vouchers", http.Header{})
	m.Record("POST", "/api/v1/expense/vouchers/5/documents", quotaHeader("abc", ""))
	m.Record("GET", "/api/v1/users", quotaHeader("-4", "50"))
	m.Record("DELETE", "/api/v1/projects/3", nil)
	if got := m.Spent(); got != 50+10+1+1 {
		t.Errorf("spent %d, want 62", got)
	}
	if m.Unmeasured() != 4 || m.Requests() != 4 {
		t.Errorf("unmeasured %d requests %d, want 4 and 4", m.Unmeasured(), m.Requests())
	}
}

// A request that was sent but not answered (network error, broken
// response) may have been charged: it counts with its documented cost.
func TestMeterRecordUnanswered(t *testing.T) {
	m := NewMeter(1000, 0)
	m.RecordUnanswered("POST", "/api/v1/income/estimates")
	if m.Spent() != 50 || m.Requests() != 1 || m.Unmeasured() != 1 {
		t.Errorf("spent %d requests %d unmeasured %d, want 50, 1, 1", m.Spent(), m.Requests(), m.Unmeasured())
	}
	if m.Quota().Known {
		t.Errorf("quota %+v known without an answer", m.Quota())
	}
}

// Regression of the first live run (2026-11-04): 66 requests whose
// X-Consumed-Quota was each one's own cost, three rejected creates (422)
// charged in full. The API used 303 credits (10000 → 9697); the meter of
// that run, which took the growth of X-Consumed-Quota as the cost,
// counted 151 (8 by documented cost). Replayed, it must count 303.
//
// The request list is reconstructed from the log and the code of the
// suite, not recorded: requests of equal cost are partly merged under one
// path (e.g. describe, update and describe of company, person and project
// as 9 × GET /contact/companies/1), and every header equals the
// documented cost. The test therefore guards against the old growth logic
// only; the max rule is TestMeterCountsMaxOfDocumentedAndConsumed's.
func TestMeterReplaysFirstLiveRun(t *testing.T) {
	type req struct {
		method, path string
		n            int
	}
	run := []req{
		{"GET", "/api/v1/income/invoices", 9}, // leftover lists of 9 resources
		{"GET", "/api/v1/users", 1},
		{"GET", "/api/v1/info", 3},
		{"POST", "/api/v1/contact/companies", 1},
		{"POST", "/api/v1/contact/companies/1/persons", 1},
		{"POST", "/api/v1/projects", 1},
		{"POST", "/api/v1/income/estimates", 2}, // 422
		{"POST", "/api/v1/propositions", 1},
		{"GET", "/api/v1/propositions/1", 1},
		{"GET", "/api/v1/contact/companies/1/persons/1", 1},
		{"POST", "/api/v1/contact/companies", 1},
		{"POST", "/api/v1/expense/vouchers", 1},   // 422
		{"GET", "/api/v1/contact/companies/1", 9}, // describe, update, describe of company, person, project
		{"POST", "/api/v1/tracker/tasks", 1},
		{"GET", "/api/v1/tracker/tasks/1", 6}, // task, proposition
		{"GET", "/api/v1/users", 1},
		{"POST", "/api/v1/tracker/time_entries", 1},
		{"GET", "/api/v1/tracker/time_entries/1", 3},
		{"POST", "/api/v1/custom_attributes", 1},
		{"GET", "/api/v1/custom_attributes/1", 3},
		{"PUT", "/api/v1/tracker/tasks/1", 5}, // carries 1, 5, 6, 11
		{"POST", "/api/v1/tracker/time_entries", 2},
		{"GET", "/api/v1/info", 1},
		{"DELETE", "/api/v1/projects/4", 10}, // cleanup
	}
	m := NewMeter(760, 64)
	n := 0
	for _, r := range run {
		cost := registry.ExpectedCredits(r.method, r.path)
		for range r.n {
			m.Record(r.method, r.path, quotaHeader(strconv.Itoa(cost), ""))
			n++
		}
	}
	if n != 66 || m.Requests() != 66 {
		t.Fatalf("%d requests replayed, meter %d, want 66", n, m.Requests())
	}
	if m.Spent() != 303 || m.Unmeasured() != 0 {
		t.Errorf("spent %d unmeasured %d, want 303 and 0 (API: 10000 → 9697)", m.Spent(), m.Unmeasured())
	}
	// budget 760, reserve 64: tests may use 696, so 393 are left
	if !m.Fits(393) || m.Fits(394) {
		t.Errorf("spent 303 of 760 - 64: Fits(393)=%v Fits(394)=%v", m.Fits(393), m.Fits(394))
	}
}

func TestMeterReserve(t *testing.T) {
	m := NewMeter(60, 0)
	if err := m.Reserve("POST", "/api/v1/income/invoices"); err != nil {
		t.Fatalf("50 of 60: %v", err)
	}
	m.Record("POST", "/api/v1/income/invoices", http.Header{})
	if err := m.Reserve("POST", "/api/v1/projects"); err != nil {
		t.Fatalf("50+10 of 60: %v", err)
	}
	m.Record("POST", "/api/v1/projects", http.Header{})
	err := m.Reserve("GET", "/api/v1/info")
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("61 of 60: %v, want ErrBudget", err)
	}
	if !strings.Contains(err.Error(), "60") {
		t.Errorf("error %q does not name the budget", err)
	}
}

func TestMeterFits(t *testing.T) {
	m := NewMeter(20, 0)
	m.Record("POST", "/api/v1/tracker/tasks", http.Header{})
	if !m.Fits(10) || m.Fits(11) {
		t.Errorf("spent 10 of 20: Fits(10)=%v Fits(11)=%v", m.Fits(10), m.Fits(11))
	}
	if m.Budget() != 20 {
		t.Errorf("Budget %d", m.Budget())
	}
}

func TestMeterSummary(t *testing.T) {
	m := NewMeter(450, 40)
	m.Record("GET", "/api/v1/info", quotaHeader("10", "990"))
	s := m.Summary()
	for _, want := range []string{"1", "450", "990", "40"} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary %q lacks %q", s, want)
		}
	}
}

// The cleanup reserve: the documented worst case of the final cleanup
// (document: delete, cancel, delete, archive; other object: delete,
// archive) plus a margin.
func TestCleanupReserve(t *testing.T) {
	if got := CleanupReserve(4, 10, 10); got != 4*4+10*2+10 {
		t.Errorf("CleanupReserve(4, 10, 10) = %d, want 46", got)
	}
	// the reserve of the live suite (e2e/e2e_test.go)
	if got := CleanupReserve(7, 13, 10); got != 64 {
		t.Errorf("CleanupReserve(7, 13, 10) = %d, want 64", got)
	}
	if got := CleanupReserve(0, 0, 0); got != 0 {
		t.Errorf("CleanupReserve(0, 0, 0) = %d", got)
	}
}

// Before the cleanup, tests (Fits) and requests (Reserve) stop at budget
// minus reserve: the reserve stays for the cleanup, which may then use
// the whole budget.
func TestMeterKeepsCleanupReserve(t *testing.T) {
	m := NewMeter(100, 30)
	for range 6 {
		m.Record("POST", "/api/v1/projects", http.Header{}) // 10 each
	}
	if !m.Fits(10) || m.Fits(11) {
		t.Errorf("spent 60, budget 100, reserve 30: Fits(10)=%v Fits(11)=%v", m.Fits(10), m.Fits(11))
	}
	if err := m.Reserve("POST", "/api/v1/projects"); err != nil {
		t.Fatalf("60+10 of 70 for tests: %v", err)
	}
	m.Record("POST", "/api/v1/projects", http.Header{})
	err := m.Reserve("GET", "/api/v1/info")
	if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "Aufräumen") {
		t.Fatalf("71 of 70 before the cleanup: %v, want ErrBudget naming the reserve", err)
	}
	m.BeginCleanup()
	for i := range 30 {
		if err := m.Reserve("DELETE", "/api/v1/projects/1"); err != nil {
			t.Fatalf("cleanup request %d: %v", i+1, err)
		}
		m.Record("DELETE", "/api/v1/projects/1", http.Header{})
	}
	if err := m.Reserve("DELETE", "/api/v1/projects/1"); !errors.Is(err, ErrBudget) {
		t.Errorf("101 of 100 in the cleanup: %v, want ErrBudget", err)
	}
	if m.Fits(1) {
		t.Error("Fits after the budget is used up")
	}
}

// A reserve outside 0..budget is clamped.
func TestMeterClampsReserve(t *testing.T) {
	if m := NewMeter(10, 50); m.Fits(1) || m.Reserved() != 10 {
		t.Errorf("reserve 50 of 10: Fits(1)=%v Reserved=%d", m.Fits(1), m.Reserved())
	}
	if m := NewMeter(10, -5); !m.Fits(10) || m.Reserved() != 0 {
		t.Errorf("reserve -5: Fits(10)=%v Reserved=%d", m.Fits(10), m.Reserved())
	}
}

// The cost of a request is its own X-Consumed-Quota: three GET /info
// answering 1, 1, 1 cost 1 each (run 1 read the growth, 0, and reported
// a false ABWEICHUNG). Only a cost other than the registry's, or
// differing costs, is a deviation.
func TestCostDeviation(t *testing.T) {
	for _, tc := range []struct {
		consumed   []int
		documented int
		want       string // part of the deviation; "" for none
	}{
		{[]int{1, 1, 1}, 1, ""},
		{[]int{0, 0, 0}, 1, "kostet 0 Credits, die Registry sagt 1"},
		{[]int{2, 2, 2}, 1, "kostet 2 Credits, die Registry sagt 1"},
		{[]int{1, 2, 1}, 1, "uneinheitlich"},
		{nil, 1, "keine Messung"},
	} {
		got := CostDeviation("GET /info", tc.consumed, tc.documented)
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("CostDeviation(%v, %d) = %q, want %q", tc.consumed, tc.documented, got, tc.want)
		}
	}
}
