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

// Quota is the last quota state the API reported.
type Quota struct {
	Remaining, Consumed int
	// Known is true once a response carried X-Consumed-Quota.
	Known bool
}

// Meter counts the credits of a run from the quota headers (spec 4.3:
// X-Consumed-Quota, X-Remaining-Quota). The cost of a request is the growth
// of X-Consumed-Quota since the previous response. Where that cannot be
// measured (first request, header missing or invalid, counter went down
// at the start of a month) the documented cost of registry.ExpectedCredits
// counts instead. It is safe for concurrent use.
type Meter struct {
	mu         sync.Mutex
	budget     int
	spent      int
	requests   int
	unmeasured int
	quota      Quota
}

// NewMeter returns a meter with the given credit budget.
func NewMeter(budget int) *Meter { return &Meter{budget: budget} }

// Budget is the credit budget of the run.
func (m *Meter) Budget() int { return m.budget }

// Reserve checks before a request is sent that its documented cost still
// fits the budget; ErrBudget otherwise (the request must not be sent).
func (m *Meter) Reserve(method, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cost := registry.ExpectedCredits(method, path)
	if m.spent+cost > m.budget {
		return fmt.Errorf("%w: %s %s kostet %d, verbraucht %d von %d", ErrBudget, method, path, cost, m.spent, m.budget)
	}
	return nil
}

// Record counts one answered request from its response headers.
func (m *Meter) Record(method, path string, h http.Header) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++
	cost := registry.ExpectedCredits(method, path)
	consumed, ok := headerInt(h, "X-Consumed-Quota")
	switch {
	case ok && m.quota.Known && consumed >= m.quota.Consumed:
		cost = consumed - m.quota.Consumed
	default:
		m.unmeasured++
	}
	m.spent += cost
	if ok {
		m.quota.Consumed, m.quota.Known = consumed, true
	}
	if r, ok := headerInt(h, "X-Remaining-Quota"); ok {
		m.quota.Remaining = r
	}
}

// Fits reports whether n more credits fit the budget.
func (m *Meter) Fits(n int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent+n <= m.budget
}

// Spent is the number of credits counted so far.
func (m *Meter) Spent() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent
}

// Requests is the number of answered requests.
func (m *Meter) Requests() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests
}

// Unmeasured is the number of requests counted with their documented cost.
func (m *Meter) Unmeasured() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unmeasured
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
	return fmt.Sprintf("Credits: %d von %d verbraucht (%d Anfragen, davon %d nach dokumentierten Kosten gezählt); Restkontingent laut API: %s",
		m.spent, m.budget, m.requests, m.unmeasured, rest)
}

func headerInt(h http.Header, key string) (int, bool) {
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
