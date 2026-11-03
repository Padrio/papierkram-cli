package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/api/gen"
	"github.com/Padrio/papierkram-cli/internal/buildinfo"
)

const testToken = "test-token-1"

// recorded is one request as seen by the test server.
type recorded struct {
	Method, Path, RawQuery string
	Header                 http.Header
	Body                   []byte
}

// recorder is an httptest handler that stores every request and answers
// with a fixed status, headers and body.
type recorder struct {
	mu     sync.Mutex
	reqs   []recorded
	status int
	header map[string]string
	body   string
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	rec.mu.Lock()
	rec.reqs = append(rec.reqs, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), b})
	rec.mu.Unlock()
	for k, v := range rec.header {
		w.Header().Set(k, v)
	}
	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, rec.body)
}

func (rec *recorder) last(t *testing.T) recorded {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) == 0 {
		t.Fatal("server received no request")
	}
	return rec.reqs[len(rec.reqs)-1]
}

func newTestClient(t *testing.T, rec *recorder, mod func(*Options)) *Client {
	t.Helper()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	o := Options{BaseURL: srv.URL + "/api/v1", Token: testToken}
	if mod != nil {
		mod(&o)
	}
	return New(o)
}

func TestCallSendsStandardHeaders(t *testing.T) {
	rec := &recorder{body: `{"type":"list"}`}
	c := newTestClient(t, rec, nil)

	resp, err := c.Call(context.Background(), "ListInvoices", nil, url.Values{"page": {"2"}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Status != http.StatusOK || string(resp.Body) != `{"type":"list"}` {
		t.Errorf("resp = %d %q", resp.Status, resp.Body)
	}
	got := rec.last(t)
	if got.Method != http.MethodGet || got.Path != "/api/v1/income/invoices" || got.RawQuery != "page=2" {
		t.Errorf("request = %s %s?%s", got.Method, got.Path, got.RawQuery)
	}
	for k, want := range map[string]string{
		"Authorization": "Bearer " + testToken,
		"Accept":        "application/json",
		"User-Agent":    buildinfo.UserAgent(),
	} {
		if v := got.Header.Get(k); v != want {
			t.Errorf("header %s = %q, want %q", k, v, want)
		}
	}
	if _, ok := got.Header["Content-Type"]; ok {
		t.Errorf("Content-Type sent without body: %q", got.Header.Get("Content-Type"))
	}
}

func TestCallCustomUserAgent(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(o *Options) { o.UserAgent = "custom/1" })
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if ua := rec.last(t).Header.Get("User-Agent"); ua != "custom/1" {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestNewNormalizesBaseURL(t *testing.T) {
	for _, suffix := range []string{"/api/v1", "/api/v1/"} {
		t.Run(suffix, func(t *testing.T) {
			rec := &recorder{}
			srv := httptest.NewServer(rec)
			defer srv.Close()
			c := New(Options{BaseURL: srv.URL + suffix, Token: testToken})

			if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
				t.Fatalf("Call: %v", err)
			}
			if p := rec.last(t).Path; p != "/api/v1/income/invoices" {
				t.Errorf("Call path = %s", p)
			}
			if _, err := c.CallRaw(context.Background(), http.MethodGet, "/income/invoices", nil, "", nil); err != nil {
				t.Fatalf("CallRaw: %v", err)
			}
			if p := rec.last(t).Path; p != "/api/v1/income/invoices" {
				t.Errorf("CallRaw path = %s", p)
			}
		})
	}
}

func TestCallJSONBody(t *testing.T) {
	rec := &recorder{status: http.StatusCreated, body: `{"id":1}`}
	c := newTestClient(t, rec, nil)

	resp, err := c.Call(context.Background(), "UpdateInvoice", []int64{5}, nil, map[string]any{"name": "Ä"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Status != http.StatusCreated {
		t.Errorf("status = %d", resp.Status)
	}
	got := rec.last(t)
	if got.Method != http.MethodPut || got.Path != "/api/v1/income/invoices/5" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if ct := got.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if string(got.Body) != `{"name":"Ä"}` {
		t.Errorf("body = %s", got.Body)
	}
}

// Operations with a request body must not send a Content-Type when the
// caller passes no body (the generated builder would add an empty one).
func TestCallBodyOperationWithoutBody(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, nil)
	if _, err := c.Call(context.Background(), "UpdateInvoice", []int64{5}, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	got := rec.last(t)
	if _, ok := got.Header["Content-Type"]; ok {
		t.Errorf("Content-Type sent without body: %q", got.Header.Get("Content-Type"))
	}
	if len(got.Body) != 0 {
		t.Errorf("body = %q", got.Body)
	}
}

func TestCallBuildErrors(t *testing.T) {
	c := New(Options{BaseURL: "http://127.0.0.1:1/api/v1", Token: testToken})
	ctx := context.Background()
	if _, err := c.Call(ctx, "NoSuchOp", nil, nil, nil); !errors.Is(err, gen.ErrUnknownOperation) {
		t.Errorf("unknown op: err = %v", err)
	}
	if _, err := c.Call(ctx, "ListInvoices", nil, nil, map[string]any{}); !errors.Is(err, gen.ErrUnexpectedBody) {
		t.Errorf("unexpected body: err = %v", err)
	}
	if _, err := c.Call(ctx, "UpdateInvoice", nil, nil, nil); !errors.Is(err, gen.ErrPathParams) {
		t.Errorf("path params: err = %v", err)
	}
}

func TestEmptySuccessBody(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rec := &recorder{status: status}
			c := newTestClient(t, rec, nil)
			resp, err := c.Call(context.Background(), "DeliverInvoice", []int64{1}, nil, nil)
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if resp.Status != status || len(resp.Body) != 0 {
				t.Errorf("resp = %d %q", resp.Status, resp.Body)
			}
		})
	}
}

func TestNon2xxReturnsResponseAndHTTPError(t *testing.T) {
	rec := &recorder{status: http.StatusNotFound, body: `{"type":"error"}`}
	c := newTestClient(t, rec, nil)
	resp, err := c.Call(context.Background(), "GetInvoice", []int64{9}, nil, nil)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("err = %v, want *HTTPError", err)
	}
	if he.Status != http.StatusNotFound || string(he.Body) != `{"type":"error"}` {
		t.Errorf("HTTPError = %d %q", he.Status, he.Body)
	}
	if resp == nil || resp.Status != http.StatusNotFound || string(resp.Body) != `{"type":"error"}` {
		t.Errorf("resp = %+v", resp)
	}
}

func TestCallRaw(t *testing.T) {
	rec := &recorder{body: "ok"}
	c := newTestClient(t, rec, nil)

	_, err := c.CallRaw(context.Background(), http.MethodPost, "/income/invoices?a=1", url.Values{"b": {"2"}},
		"text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	got := rec.last(t)
	if got.Method != http.MethodPost || got.Path != "/api/v1/income/invoices" || got.RawQuery != "a=1&b=2" {
		t.Errorf("request = %s %s?%s", got.Method, got.Path, got.RawQuery)
	}
	if ct := got.Header.Get("Content-Type"); ct != "text/plain" || string(got.Body) != "hello" {
		t.Errorf("Content-Type %q body %q", ct, got.Body)
	}
	if a := got.Header.Get("Authorization"); a != "Bearer "+testToken {
		t.Errorf("Authorization = %q", a)
	}

	if _, err := c.CallRaw(context.Background(), http.MethodGet, "income/invoices", nil, "text/plain", nil); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	got = rec.last(t)
	if got.Path != "/api/v1/income/invoices" {
		t.Errorf("path = %s", got.Path)
	}
	if _, ok := got.Header["Content-Type"]; ok {
		t.Errorf("Content-Type sent without body")
	}
}

// A path that looks like a network-path reference must stay on the API host,
// so the token never leaves it.
func TestCallRawStaysOnBaseHost(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, nil)
	if _, err := c.CallRaw(context.Background(), http.MethodGet, "//other.example/x", nil, "", nil); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if p := rec.last(t).Path; !strings.HasPrefix(p, "/api/v1/") {
		t.Errorf("path = %s", p)
	}
}

func TestUploadMultipart(t *testing.T) {
	rec := &recorder{status: http.StatusCreated}
	c := newTestClient(t, rec, nil)

	_, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("%PDF-1.4"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	got := rec.last(t)
	if got.Method != http.MethodPost || got.Path != "/api/v1/expense/vouchers/3/documents" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	mt, params, err := mime.ParseMediaType(got.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("Content-Type = %q (%v)", got.Header.Get("Content-Type"), err)
	}
	mr := multipart.NewReader(strings.NewReader(string(got.Body)), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("NextPart: %v", err)
	}
	if part.FormName() != "file" || part.FileName() != "beleg.pdf" {
		t.Errorf("part name %q filename %q", part.FormName(), part.FileName())
	}
	if ct := part.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("part Content-Type = %q", ct)
	}
	data, _ := io.ReadAll(part)
	if string(data) != "%PDF-1.4" {
		t.Errorf("part data = %q", data)
	}
	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("more than one part: %v", err)
	}
}

func TestUploadUnknownExtension(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, nil)
	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.zzunknown", strings.NewReader("x")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	_, params, _ := mime.ParseMediaType(rec.last(t).Header.Get("Content-Type"))
	mr := multipart.NewReader(strings.NewReader(string(rec.last(t).Body)), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("NextPart: %v", err)
	}
	if ct := part.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("part Content-Type = %q", ct)
	}
}

func TestTimeoutApplied(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(block); srv.Close() })

	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Timeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("timeout not applied, took %s", d)
	}
}

func TestUsesGivenHTTPClient(t *testing.T) {
	rec := &recorder{}
	var used bool
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	c := newTestClient(t, rec, func(o *Options) { o.HTTP = hc })
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !used {
		t.Error("Options.HTTP not used")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
