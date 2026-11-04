package api

import (
	"context"
	"net/http"
	"testing"
)

func TestR21_QuotaFromRemainingHeader(t *testing.T) {
	rec := &recorder{header: map[string]string{
		"X-Remaining-Quota":     "42",
		"X-Consumed-Quota":      "7",
		"X-RateLimit-Remaining": "999",
	}}
	var seen []Quota
	c := newTestClient(t, rec, func(o *Options) { o.OnQuota = func(q Quota) { seen = append(seen, q) } })

	resp, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := Quota{Remaining: 42, RequestCost: 7, Known: true, CostKnown: true}
	if resp.Quota != want {
		t.Errorf("Quota = %+v, want %+v", resp.Quota, want)
	}
	if len(seen) != 1 || seen[0] != want {
		t.Errorf("OnQuota calls = %+v", seen)
	}
}

func TestQuotaIgnoresRateLimitHeaderAlone(t *testing.T) {
	rec := &recorder{header: map[string]string{"X-RateLimit-Remaining": "999"}}
	called := false
	c := newTestClient(t, rec, func(o *Options) { o.OnQuota = func(Quota) { called = true } })
	resp, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Quota != (Quota{}) {
		t.Errorf("Quota = %+v, want zero", resp.Quota)
	}
	if called {
		t.Error("OnQuota called without quota headers")
	}
}

func TestParseQuota(t *testing.T) {
	cases := []struct {
		name      string
		remaining string
		consumed  string
		want      Quota
	}{
		{"none", "", "", Quota{}},
		{"both", "10000", "0", Quota{Remaining: 10000, RequestCost: 0, Known: true, CostKnown: true}},
		{"remaining only", "5", "", Quota{Remaining: 5, Known: true}},
		{"consumed only", "", "3", Quota{RequestCost: 3, CostKnown: true}},
		{"invalid remaining", "abc", "3", Quota{RequestCost: 3, CostKnown: true}},
		{"invalid consumed", "5", "x", Quota{Remaining: 5, Known: true}},
		{"whitespace", " 12 ", "", Quota{Remaining: 12, Known: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.remaining != "" {
				h.Set("X-Remaining-Quota", tc.remaining)
			}
			if tc.consumed != "" {
				h.Set("X-Consumed-Quota", tc.consumed)
			}
			if got := parseQuota(h); got != tc.want {
				t.Errorf("parseQuota = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestQuotaOnErrorResponse(t *testing.T) {
	rec := &recorder{status: http.StatusTooManyRequests, header: map[string]string{"X-Remaining-Quota": "0", "X-Consumed-Quota": "10000"}}
	var seen Quota
	c := newTestClient(t, rec, func(o *Options) { o.OnQuota = func(q Quota) { seen = q } })
	resp, _ := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	want := Quota{Remaining: 0, RequestCost: 10000, Known: true, CostKnown: true}
	if resp == nil || resp.Quota != want || seen != want {
		t.Errorf("resp quota %+v, OnQuota %+v, want %+v", resp, seen, want)
	}
}
