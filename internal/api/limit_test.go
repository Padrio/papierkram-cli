package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// fillReader yields an endless stream of 'a' without allocating it.
type fillReader struct{}

func (fillReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// bigServer answers every request with status, contentType and a streamed
// body of size bytes (never held in memory by the server) and counts the
// requests.
func bigServer(t *testing.T, status int, contentType string, size int64) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(status)
		_, _ = io.CopyN(w, fillReader{}, size)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestMaxResponseSizeIs64MiB(t *testing.T) {
	if MaxResponseSize != 64<<20 {
		t.Errorf("MaxResponseSize = %d, want %d", MaxResponseSize, 64<<20)
	}
}

// A 2xx GET answer above the limit is exit 1 and never retried.
func TestResponseTooLargeGet(t *testing.T) {
	srv, n := bigServer(t, http.StatusOK, "application/octet-stream", MaxResponseSize+1)
	s := &sleepRecorder{}
	resp, err := clientFor(srv, s).Call(context.Background(), "ListInvoices", nil, nil, nil)
	ce := asCLIErr(t, err)
	if ce.Code != clierr.CodeError || ce.Exit() != 1 {
		t.Errorf("code %s exit %d, want error 1", ce.Code, ce.Exit())
	}
	if ce.Message != "Antwort größer als 64 MiB, abgebrochen" {
		t.Errorf("message = %q", ce.Message)
	}
	if ce.Hint != "Antwort zu groß für das CLI; Ergebnis nicht verwertbar" {
		t.Errorf("hint = %q", ce.Hint)
	}
	if ce.HTTPStatus != 0 {
		// ruling FR-D: a 2xx status is no error status (error.json)
		t.Errorf("http status = %d, want none", ce.HTTPStatus)
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Error("error does not wrap ErrResponseTooLarge")
	}
	if resp != nil {
		t.Errorf("resp = %+v, want nil", resp)
	}
	if n.Load() != 1 || len(s.got) != 0 {
		t.Errorf("%d requests, sleeps %v; want 1 request, no retry", n.Load(), s.got)
	}
}

// A PDF download above the limit fails the same way: nothing cut off is
// handed to the caller to save.
func TestResponseTooLargeDownload(t *testing.T) {
	srv, n := bigServer(t, http.StatusOK, "application/pdf", MaxResponseSize+1)
	resp, err := clientFor(srv, &sleepRecorder{}).Download(context.Background(), "GetInvoicePdf", []int64{5}, "application/pdf")
	if ce := asCLIErr(t, err); ce.Code != clierr.CodeError || ce.Message != "Antwort größer als 64 MiB, abgebrochen" {
		t.Errorf("code %s message %q", ce.Code, ce.Message)
	}
	if resp != nil || n.Load() != 1 {
		t.Errorf("resp %+v after %d requests, want nil after 1", resp, n.Load())
	}
}

// For a mutation the change was made: exit 1, do not repeat.
func TestResponseTooLargeMutation(t *testing.T) {
	tests := []struct {
		method string
		status int
		call   func(*Client) (*Response, error)
	}{
		{http.MethodPost, http.StatusCreated, func(c *Client) (*Response, error) {
			return c.Call(context.Background(), "CreateInvoice", nil, nil, map[string]any{"name": "x"})
		}},
		{http.MethodPut, http.StatusOK, func(c *Client) (*Response, error) {
			return c.Call(context.Background(), "UpdateInvoice", []int64{5}, nil, map[string]any{"name": "x"})
		}},
		{http.MethodDelete, http.StatusOK, func(c *Client) (*Response, error) {
			return c.CallRaw(context.Background(), http.MethodDelete, "/income/invoices/5", nil, "", nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			srv, n := bigServer(t, tt.status, "application/json", MaxResponseSize+1)
			s := &sleepRecorder{}
			resp, err := tt.call(clientFor(srv, s))
			ce := asCLIErr(t, err)
			// no http_status (ruling FR-D), whatever the 2xx status
			if ce.Code != clierr.CodeError || ce.Exit() != 1 || ce.HTTPStatus != 0 {
				t.Errorf("code %s exit %d status %d, want error 1 without status (answer %d)", ce.Code, ce.Exit(), ce.HTTPStatus, tt.status)
			}
			if ce.Message != "Antwort größer als 64 MiB; die Änderung wurde ausgeführt" {
				t.Errorf("message = %q", ce.Message)
			}
			if !strings.Contains(ce.Message, "Änderung wurde ausgeführt") {
				t.Errorf("message %q does not say the change was made", ce.Message)
			}
			if ce.Hint != "NICHT wiederholen; Ergebnis per describe/list prüfen" {
				t.Errorf("hint = %q", ce.Hint)
			}
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Error("error does not wrap ErrResponseTooLarge")
			}
			if resp != nil || n.Load() != 1 || len(s.got) != 0 {
				t.Errorf("resp %+v, %d requests, sleeps %v; want nil, 1, none", resp, n.Load(), s.got)
			}
		})
	}
}

// Above the limit a non-2xx status still decides, with the cut body.
func TestResponseTooLargeErrorStatus(t *testing.T) {
	tests := []struct {
		status   int
		code     clierr.Code
		exit     int
		requests int32
	}{
		{http.StatusNotFound, clierr.CodeNotFound, 4, 1},
		{http.StatusTooManyRequests, clierr.CodeQuota, 6, 1},
		{http.StatusServiceUnavailable, clierr.CodeServer, 1, 3}, // GET 5xx: retried as before
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			srv, n := bigServer(t, tt.status, "text/html", MaxResponseSize+1)
			resp, err := clientFor(srv, &sleepRecorder{}).Call(context.Background(), "ListInvoices", nil, nil, nil)
			ce := asCLIErr(t, err)
			if ce.Code != tt.code || ce.Exit() != tt.exit || ce.HTTPStatus != tt.status {
				t.Errorf("code %s exit %d status %d, want %s %d %d", ce.Code, ce.Exit(), ce.HTTPStatus, tt.code, tt.exit, tt.status)
			}
			if errors.Is(err, ErrResponseTooLarge) {
				t.Error("status error wraps ErrResponseTooLarge")
			}
			if resp == nil || len(resp.Body) != MaxResponseSize {
				t.Errorf("body not cut to MaxResponseSize: resp %v", resp != nil)
			}
			if n.Load() != tt.requests {
				t.Errorf("%d requests, want %d", n.Load(), tt.requests)
			}
		})
	}
}

// Exactly 64 MiB is still a complete answer.
func TestResponseExactlyMaxSize(t *testing.T) {
	srv, n := bigServer(t, http.StatusOK, "application/octet-stream", MaxResponseSize)
	resp, err := clientFor(srv, &sleepRecorder{}).Call(context.Background(), "ListInvoices", nil, nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(resp.Body) != MaxResponseSize || n.Load() != 1 {
		t.Errorf("body %d bytes after %d requests, want %d after 1", len(resp.Body), n.Load(), MaxResponseSize)
	}
	if !bytes.Equal(resp.Body[MaxResponseSize-3:], []byte("aaa")) {
		t.Error("body content changed")
	}
}

// --debug says that the answer was cut off at the limit.
func TestDebugShowsResponseCut(t *testing.T) {
	srv, _ := bigServer(t, http.StatusOK, "application/octet-stream", MaxResponseSize+1)
	var buf bytes.Buffer
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: (&sleepRecorder{}).sleep, Debug: &buf})
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err == nil {
		t.Fatal("Call succeeded")
	}
	out := buf.String()
	if !strings.Contains(out, "< gekürzt nach 64 MiB\n") {
		t.Errorf("debug output lacks the cut:\n%s", out)
	}
	if !strings.Contains(out, "< <binär, 67108864 Bytes>\n") {
		t.Errorf("debug output lacks the read size:\n%s", out)
	}
}

// Without the limit being reached, --debug says nothing about it.
func TestDebugNoCutAtMaxSize(t *testing.T) {
	srv, _ := bigServer(t, http.StatusOK, "application/octet-stream", 10)
	var buf bytes.Buffer
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Debug: &buf})
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if strings.Contains(buf.String(), "gekürzt nach") {
		t.Errorf("debug output reports a cut:\n%s", buf.String())
	}
}
