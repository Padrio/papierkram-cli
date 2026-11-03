package api

import (
	"net/http"
	"strconv"
	"strings"
)

// Quota is the monthly credit state reported by the API (spec 4.3).
type Quota struct {
	Remaining, Consumed int
	// Known is true when X-Remaining-Quota was present and valid.
	Known bool
}

// parseQuota reads X-Remaining-Quota and X-Consumed-Quota. X-RateLimit-Remaining
// is deliberately ignored: Papierkram has no classic rate limit (R21).
func parseQuota(h http.Header) Quota {
	var q Quota
	if n, ok := headerInt(h, "X-Remaining-Quota"); ok {
		q.Remaining, q.Known = n, true
	}
	if n, ok := headerInt(h, "X-Consumed-Quota"); ok {
		q.Consumed = n
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
