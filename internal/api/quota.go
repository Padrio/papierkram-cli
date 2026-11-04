package api

import (
	"net/http"
	"strconv"
	"strings"
)

// Quota is the credit state reported with a response (spec 4.3):
// Remaining is the monthly credit left (X-Remaining-Quota); RequestCost
// is what the answered request cost (X-Consumed-Quota, measured live
// 2026-11-04: the cost of that one request, not a monthly consumption).
type Quota struct {
	Remaining, RequestCost int
	// Known is true when X-Remaining-Quota was present and valid.
	Known bool
	// CostKnown is true when X-Consumed-Quota was present and valid.
	CostKnown bool
}

// parseQuota reads X-Remaining-Quota and X-Consumed-Quota. X-RateLimit-Remaining
// is deliberately ignored: Papierkram has no classic rate limit (R21).
func parseQuota(h http.Header) Quota {
	var q Quota
	if n, ok := headerInt(h, "X-Remaining-Quota"); ok {
		q.Remaining, q.Known = n, true
	}
	if n, ok := headerInt(h, "X-Consumed-Quota"); ok {
		q.RequestCost, q.CostKnown = n, true
	}
	return q
}

func headerInt(h http.Header, key string) (int, bool) {
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
