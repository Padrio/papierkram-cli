package e2eguard

import (
	"errors"
	"net/http"
	"strings"
	"testing"
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

// The cost of the first request cannot be measured (no value before it):
// it counts with the documented cost. From then on the growth of
// X-Consumed-Quota is the cost.
func TestMeterCountsConsumedDeltas(t *testing.T) {
	m := NewMeter(100)
	m.Record("GET", "/api/v1/info", quotaHeader("500", "9500"))
	if m.Spent() != 1 {
		t.Fatalf("after the first GET: spent %d, want 1 (documented cost)", m.Spent())
	}
	m.Record("POST", "/api/v1/contact/companies", quotaHeader("510", "9490"))
	m.Record("GET", "/api/v1/info", quotaHeader("510", "9490")) // free?
	m.Record("POST", "/api/v1/income/invoices", quotaHeader("560", "9440"))
	if got := m.Spent(); got != 61 {
		t.Errorf("spent %d, want 1+10+0+50", got)
	}
	if m.Requests() != 4 || m.Unmeasured() != 1 {
		t.Errorf("requests %d unmeasured %d, want 4 and 1", m.Requests(), m.Unmeasured())
	}
	q := m.Quota()
	if !q.Known || q.Consumed != 560 || q.Remaining != 9440 {
		t.Errorf("quota %+v", q)
	}
}

// Without the header (or with garbage, or a counter that went down at the
// start of a month) a request counts with its documented cost.
func TestMeterFallsBackToDocumentedCost(t *testing.T) {
	m := NewMeter(1000)
	m.Record("POST", "/api/v1/expense/vouchers", http.Header{})
	m.Record("POST", "/api/v1/expense/vouchers/5/documents", quotaHeader("abc", ""))
	m.Record("GET", "/api/v1/users", quotaHeader("100", "50"))
	m.Record("DELETE", "/api/v1/projects/3", quotaHeader("7", "50"))
	if got := m.Spent(); got != 50+10+1+1 {
		t.Errorf("spent %d, want 62", got)
	}
	if m.Unmeasured() != 4 {
		t.Errorf("unmeasured %d, want 4", m.Unmeasured())
	}
}

func TestMeterReserve(t *testing.T) {
	m := NewMeter(60)
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
	m := NewMeter(20)
	m.Record("POST", "/api/v1/tracker/tasks", http.Header{})
	if !m.Fits(10) || m.Fits(11) {
		t.Errorf("spent 10 of 20: Fits(10)=%v Fits(11)=%v", m.Fits(10), m.Fits(11))
	}
	if m.Budget() != 20 {
		t.Errorf("Budget %d", m.Budget())
	}
}

func TestMeterSummary(t *testing.T) {
	m := NewMeter(450)
	m.Record("GET", "/api/v1/info", quotaHeader("10", "990"))
	s := m.Summary()
	for _, want := range []string{"1", "450", "990"} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary %q lacks %q", s, want)
		}
	}
}
