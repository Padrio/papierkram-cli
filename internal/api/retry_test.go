package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// sleepRecorder replaces time.Sleep and records the requested backoffs.
type sleepRecorder struct{ got []time.Duration }

func (s *sleepRecorder) sleep(d time.Duration) { s.got = append(s.got, d) }

// sequenceServer answers request n with statuses[n] (the last one repeats)
// and counts the requests.
func sequenceServer(t *testing.T, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		i := int(n.Add(1)) - 1
		w.WriteHeader(statuses[min(i, len(statuses)-1)])
		_, _ = io.WriteString(w, `{"type":"ok"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// closingServer reads the request body, then drops the connection without
// any response for the first `drops` requests and answers 200 afterwards.
func closingServer(t *testing.T, drops int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if n.Add(1) > drops {
			_, _ = io.WriteString(w, `{"type":"ok"}`)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func clientFor(srv *httptest.Server, s *sleepRecorder) *Client {
	return New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: s.sleep})
}

var wantBackoff = []time.Duration{500 * time.Millisecond, time.Second}

func TestRetryGet5xxThenSuccess(t *testing.T) {
	srv, n := sequenceServer(t, 503, 503, 200)
	s := &sleepRecorder{}
	resp, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Status != http.StatusOK || n.Load() != 3 {
		t.Errorf("status %d after %d requests, want 200 after 3", resp.Status, n.Load())
	}
	if !slices.Equal(s.got, wantBackoff) {
		t.Errorf("sleeps = %v, want %v", s.got, wantBackoff)
	}
}

func TestRetryGet5xxGivesUpAfterThreeRequests(t *testing.T) {
	srv, n := sequenceServer(t, 503)
	s := &sleepRecorder{}
	resp, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	ce := asCLIErr(t, err)
	if ce.Code != clierr.CodeServer || ce.Exit() != 1 || ce.HTTPStatus != 503 {
		t.Errorf("code %s exit %d status %d", ce.Code, ce.Exit(), ce.HTTPStatus)
	}
	if n.Load() != 3 || resp == nil || resp.Status != 503 {
		t.Errorf("%d requests, resp %+v", n.Load(), resp)
	}
	if !slices.Equal(s.got, wantBackoff) {
		t.Errorf("sleeps = %v, want %v", s.got, wantBackoff)
	}
}

func TestRetryGetNetworkErrorThenSuccess(t *testing.T) {
	srv, n := closingServer(t, 2)
	s := &sleepRecorder{}
	resp, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Status != http.StatusOK || n.Load() != 3 {
		t.Errorf("status %d after %d requests", resp.Status, n.Load())
	}
	if !slices.Equal(s.got, wantBackoff) {
		t.Errorf("sleeps = %v, want %v", s.got, wantBackoff)
	}
}

func TestRetryGetNetworkErrorGivesUp(t *testing.T) {
	srv, n := closingServer(t, 100)
	s := &sleepRecorder{}
	_, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	ce := asCLIErr(t, err)
	if ce.Code != clierr.CodeNetwork || ce.Exit() != 10 || n.Load() != 3 {
		t.Errorf("code %s exit %d after %d requests", ce.Code, ce.Exit(), n.Load())
	}
}

func TestR22_NoRetryOn429(t *testing.T) {
	srv, n := sequenceServer(t, 429, 200)
	s := &sleepRecorder{}
	_, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	ce := asCLIErr(t, err)
	if ce.Exit() != 6 || ce.Code != clierr.CodeQuota {
		t.Errorf("code %s exit %d, want quota 6", ce.Code, ce.Exit())
	}
	if n.Load() != 1 || len(s.got) != 0 {
		t.Errorf("%d requests, sleeps %v; want 1 request, no sleep", n.Load(), s.got)
	}
}

func TestNoRetryOn4xxOrMutation5xx(t *testing.T) {
	tests := []struct {
		name, method string
		status       int
	}{
		{"GET 400", http.MethodGet, 400},
		{"GET 401", http.MethodGet, 401},
		{"GET 404", http.MethodGet, 404},
		{"GET 422", http.MethodGet, 422},
		{"POST 503", http.MethodPost, 503},
		{"PUT 500", http.MethodPut, 500},
		{"DELETE 502", http.MethodDelete, 502},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, n := sequenceServer(t, tt.status, 200)
			s := &sleepRecorder{}
			_, err := clientFor(srv, s).CallRaw(context.Background(), tt.method, "/income/invoices/1", nil, "application/json", strings.NewReader("{}"))
			if ce := asCLIErr(t, err); ce.HTTPStatus != tt.status {
				t.Errorf("status %d", ce.HTTPStatus)
			}
			if n.Load() != 1 || len(s.got) != 0 {
				t.Errorf("%d requests, sleeps %v; want 1 request, no sleep", n.Load(), s.got)
			}
		})
	}
}

// The server reads the whole POST and drops the connection: the request may
// have been processed, so it is neither retried nor reported as "not sent".
func TestR23_NoRetryOnPostNetworkError(t *testing.T) {
	srv, n := closingServer(t, 100)
	s := &sleepRecorder{}
	_, err := clientFor(srv, s).Call(context.Background(), "CreateInvoice", nil, nil, map[string]any{"name": "x"})
	ce := asCLIErr(t, err)
	if n.Load() != 1 || len(s.got) != 0 {
		t.Errorf("%d requests, sleeps %v; want 1 request, no sleep", n.Load(), s.got)
	}
	if ce.Code != clierr.CodeOutcomeUnknown || ce.Exit() != 12 {
		t.Errorf("code %s exit %d, want outcome_unknown 12", ce.Code, ce.Exit())
	}
}

func TestR38_TimeoutAfterCreateIsOutcomeUnknown(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select { // sleep well beyond the client timeout
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	s := &sleepRecorder{}
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Timeout: 50 * time.Millisecond, Sleep: s.sleep})

	_, err := c.Call(context.Background(), "CreateInvoice", nil, nil, map[string]any{"name": "x"})
	ce := asCLIErr(t, err)
	if ce.Code != clierr.CodeOutcomeUnknown || ce.Exit() != 12 {
		t.Errorf("code %s exit %d, want outcome_unknown 12", ce.Code, ce.Exit())
	}
	if ce.Hint != "Ergebnis unbekannt – vor Wiederholung per describe/list prüfen" {
		t.Errorf("hint = %q", ce.Hint)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cause not wrapped: %v", ce.Cause)
	}
	if n.Load() != 1 || len(s.got) != 0 {
		t.Errorf("%d requests, sleeps %v; want 1 request, no sleep", n.Load(), s.got)
	}
}

func TestNoRetryAfterContextCanceled(t *testing.T) {
	srv, n := closingServer(t, 100)
	ctx, cancel := context.WithCancel(context.Background())
	s := &sleepRecorder{}
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: func(d time.Duration) { s.sleep(d); cancel() }})
	_, err := c.Call(ctx, "ListInvoices", nil, nil, nil)
	if ce := asCLIErr(t, err); ce.Code != clierr.CodeNetwork {
		t.Errorf("code %s", ce.Code)
	}
	if n.Load() != 1 || len(s.got) != 1 {
		t.Errorf("%d requests, sleeps %v; want stop after cancel", n.Load(), s.got)
	}
}
