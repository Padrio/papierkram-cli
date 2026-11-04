package e2eguard

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// ErrBudget is returned by Meter.Reserve when a request would exceed the
// credit budget of the run.
var ErrBudget = errors.New("e2e: Credit-Budget des Laufs erschöpft")

// Quota is the last quota state the API reported: Remaining from
// X-Remaining-Quota, Consumed from X-Consumed-Quota, which is the cost of
// that one request, not a running total (measured 2026-11-04).
type Quota struct {
	Remaining, Consumed int
	// Known is true once a response carried X-Consumed-Quota.
	Known bool
}

// Documented worst case of removing one object in the final cleanup
// (spec 4.3: 1 credit per request): a document (invoice, estimate,
// voucher) may need delete, cancel, delete, archive; any other object
// delete, archive.
const (
	CleanupCostDocument = 4
	CleanupCostOther    = 2
)

// CleanupReserve is the share of the budget kept for the final cleanup:
// the documented worst case for docs documents and others other objects,
// plus margin.
func CleanupReserve(docs, others, margin int) int {
	return docs*CleanupCostDocument + others*CleanupCostOther + margin
}

// Meter counts the credits of a run. Every request that was sent counts,
// whatever its status (a rejected create is charged in full, measured
// 2026-11-04): with the larger of its documented cost
// (registry.ExpectedCredits) and its X-Consumed-Quota, which is the cost
// of that one request (spec 4.3; measured: not a running total). Without
// a valid header, or without an answer (RecordUnanswered), the documented
// cost counts.
//
// A fixed part of the budget is reserved for the final cleanup: until
// BeginCleanup, tests (Fits) and requests (Reserve) may only use budget
// minus reserve, so the cleanup always gets to run. It is safe for
// concurrent use.
type Meter struct {
	mu         sync.Mutex
	budget     int
	reserve    int
	cleanup    bool
	spent      int
	requests   int
	unmeasured int
	costlier   int
	quota      Quota
}

// NewMeter returns a meter with the given credit budget, of which reserve
// (clamped to 0..budget) is kept for the cleanup.
func NewMeter(budget, reserve int) *Meter {
	return &Meter{budget: budget, reserve: min(max(reserve, 0), budget)}
}

// Budget is the credit budget of the run.
func (m *Meter) Budget() int { return m.budget }

// Reserved is the part of the budget kept for the cleanup.
func (m *Meter) Reserved() int { return m.reserve }

// BeginCleanup releases the cleanup reserve: from now on requests may use
// the whole budget.
func (m *Meter) BeginCleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup = true
}

// limit is the credit limit of the current phase; m.mu must be held.
func (m *Meter) limit() int {
	if m.cleanup {
		return m.budget
	}
	return m.budget - m.reserve
}

// Reserve checks before a request is sent that its documented cost still
// fits the limit of the phase (before the cleanup: budget minus reserve);
// ErrBudget otherwise (the request must not be sent).
func (m *Meter) Reserve(method, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cost := registry.ExpectedCredits(method, path)
	if m.spent+cost > m.limit() {
		if m.cleanup {
			return fmt.Errorf("%w: %s %s kostet %d, verbraucht %d von %d", ErrBudget, method, path, cost, m.spent, m.budget)
		}
		return fmt.Errorf("%w: %s %s kostet %d, verbraucht %d von %d, davon %d für das Aufräumen reserviert",
			ErrBudget, method, path, cost, m.spent, m.budget, m.reserve)
	}
	return nil
}

// Record counts one answered request, of any status, from its response
// headers: max(documented cost, X-Consumed-Quota); the documented cost
// when the header is missing, invalid or negative.
func (m *Meter) Record(method, path string, h http.Header) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++
	cost := registry.ExpectedCredits(method, path)
	consumed, ok := headerInt(h, "X-Consumed-Quota")
	ok = ok && consumed >= 0
	switch {
	case !ok:
		m.unmeasured++
	case consumed > cost:
		m.costlier++
		cost = consumed
	}
	m.spent += cost
	if ok {
		m.quota.Consumed, m.quota.Known = consumed, true
	}
	if r, ok := headerInt(h, "X-Remaining-Quota"); ok {
		m.quota.Remaining = r
	}
}

// RecordUnanswered counts a request that was sent (or may have been) but
// got no usable answer, e.g. a network error: the API may have charged
// it, so its documented cost counts.
func (m *Meter) RecordUnanswered(method, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++
	m.unmeasured++
	m.spent += registry.ExpectedCredits(method, path)
}

// Fits reports whether n more credits fit the limit of the phase (before
// the cleanup: budget minus reserve).
func (m *Meter) Fits(n int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent+n <= m.limit()
}

// Spent is the number of credits counted so far.
func (m *Meter) Spent() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent
}

// Requests is the number of counted requests (answered or not).
func (m *Meter) Requests() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// Unmeasured is the number of requests counted with their documented
// cost because X-Consumed-Quota was missing or invalid, or no answer came.
func (m *Meter) Unmeasured() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unmeasured
}

// Costlier is the number of requests whose X-Consumed-Quota was above
// their documented cost (the registry may be too low for them).
func (m *Meter) Costlier() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.costlier
}

// Quota is the last reported quota state.
func (m *Meter) Quota() Quota {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.quota
}

// Summary is the closing line of a run.
func (m *Meter) Summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	rest := "unbekannt"
	if m.quota.Known {
		rest = strconv.Itoa(m.quota.Remaining)
	}
	return fmt.Sprintf("Credits: %d von %d verbraucht (Reserve fürs Aufräumen %d; %d Anfragen, davon %d ohne X-Consumed-Quota nach dokumentierten Kosten gezählt, %d teurer als dokumentiert); Restkontingent laut API: %s",
		m.spent, m.budget, m.reserve, m.requests, m.unmeasured, m.costlier, rest)
}

func headerInt(h http.Header, key string) (int, bool) {
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
