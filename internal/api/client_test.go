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
	"github.com/Padrio/papierkram-cli/internal/clierr"
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

// Preview builds method and URL exactly like Call sends them, query
// included, without sending anything (--dry-run).
func TestPreviewMatchesCall(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken})
	q := url.Values{"search": {"Müller & Söhne"}, "page": {"2"}}

	method, u, err := c.Preview("ListPersons", []int64{7}, q)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(rec.reqs) != 0 {
		t.Fatalf("Preview hat gesendet: %+v", rec.reqs)
	}
	want := srv.URL + "/api/v1/contact/companies/7/persons?page=2&search=M%C3%BCller+%26+S%C3%B6hne"
	if method != http.MethodGet || u != want {
		t.Errorf("Preview = %s %s, erwartet GET %s", method, u, want)
	}
	if _, err := c.Call(context.Background(), "ListPersons", []int64{7}, q, nil); err != nil {
		t.Fatal(err)
	}
	if got := rec.last(t); srv.URL+got.Path+"?"+got.RawQuery != u {
		t.Errorf("Call sendet %s?%s, Preview zeigt %s", got.Path, got.RawQuery, u)
	}

	if method, u, err = c.Preview("DeleteInvoice", []int64{12}, nil); err != nil || method != http.MethodDelete || u != srv.URL+"/api/v1/income/invoices/12" {
		t.Errorf("Preview = %s %s, %v", method, u, err)
	}
	if _, _, err := c.Preview("NoSuchOp", nil, nil); !errors.Is(err, gen.ErrUnknownOperation) {
		t.Errorf("unknown op: err = %v", err)
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

	_, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("%PDF-1.4"), 8)
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
	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.zzunknown", strings.NewReader("x"), 1); err != nil {
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

	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Timeout: 50 * time.Millisecond,
		Sleep: func(time.Duration) {}})
	start := time.Now()
	_, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	// The GET was written completely; a safe method still counts as network.
	if ce := asCLIErr(t, err); ce.Code != clierr.CodeNetwork || ce.Exit() != 10 {
		t.Errorf("code %s exit %d, want network 10", ce.Code, ce.Exit())
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

// Download asks for the file type first and JSON (the API's error format)
// as fallback, and returns the body bytes unchanged; Call keeps JSON only.
func TestDownloadSendsAcceptAndKeepsBytes(t *testing.T) {
	pdf := "%PDF-1.4\x00\r\n\x1b[31m\xff%%EOF\n"
	rec := &recorder{body: pdf, header: map[string]string{"Content-Type": "application/pdf"}}
	c := newTestClient(t, rec, nil)

	resp, err := c.Download(context.Background(), "GetInvoicePdf", []int64{459}, "application/pdf")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(resp.Body) != pdf {
		t.Errorf("body = %q, want %q", resp.Body, pdf)
	}
	got := rec.last(t)
	if got.Method != http.MethodGet || got.Path != "/api/v1/income/invoices/459/pdf" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	if a := got.Header.Get("Accept"); a != "application/pdf, application/json;q=0.9" {
		t.Errorf("Accept = %q", a)
	}
	if a := got.Header.Get("Authorization"); a != "Bearer "+testToken {
		t.Errorf("Authorization = %q", a)
	}

	if _, err := c.Call(context.Background(), "GetInvoice", []int64{459}, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if a := rec.last(t).Header.Get("Accept"); a != "application/json" {
		t.Errorf("Call Accept = %q", a)
	}
}

// A JSON error answer of a download keeps the normal error mapping.
func TestDownloadMapsErrors(t *testing.T) {
	rec := &recorder{status: http.StatusNotFound, header: map[string]string{"Content-Type": "application/json"},
		body: `{"type":"error","message":"Rechnung nicht gefunden"}`}
	c := newTestClient(t, rec, nil)
	_, err := c.Download(context.Background(), "GetInvoicePdf", []int64{1}, "application/pdf")
	if ce := clierr.From(err); ce.Code != clierr.CodeNotFound || !strings.Contains(ce.Message, "Rechnung nicht gefunden") {
		t.Errorf("err = %v", err)
	}
}

// The upload body is streamed with a known length: the request carries
// Content-Length (no chunked encoding), and the file is still being read
// while the server already handles the request, so it is never read into
// memory first.
func TestUploadStreamsFileWithContentLength(t *testing.T) {
	started := make(chan struct{})
	var (
		length   int64
		encoding []string
		body     []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		length, encoding = r.ContentLength, r.TransferEncoding
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken})

	first := strings.Repeat("a", 64<<10) // fills the write buffer: the headers go out
	file := io.MultiReader(strings.NewReader(first), &gatedReader{gate: started, data: "%%EOF"})
	size := int64(len(first) + len("%%EOF"))
	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", file, size); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if length != int64(len(body)) || len(encoding) != 0 {
		t.Errorf("Content-Length = %d, Body %d Bytes, Transfer-Encoding %v", length, len(body), encoding)
	}
	if !strings.Contains(string(body), first+"%%EOF\r\n--") {
		t.Errorf("file content not sent completely")
	}
}

// gatedReader returns data only once gate is closed (the server handles
// the request), or fails after a timeout.
type gatedReader struct {
	gate <-chan struct{}
	data string
	done bool
}

func (g *gatedReader) Read(p []byte) (int, error) {
	if g.done {
		return 0, io.EOF
	}
	select {
	case <-g.gate:
	case <-time.After(5 * time.Second):
		return 0, errors.New("file read before the request was sent: upload is buffered")
	}
	g.done = true
	return copy(p, g.data), nil
}

// The file name is quoted like mime/multipart does: quotes and
// backslashes are escaped, line breaks cannot start a header of their own.
func TestUploadFilenameQuoted(t *testing.T) {
	rec := &recorder{status: http.StatusCreated}
	c := newTestClient(t, rec, nil)
	name := "a\"b\\c\r\nX-Evil: 1.pdf"
	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", name, strings.NewReader("x"), 1); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	_, params, _ := mime.ParseMediaType(rec.last(t).Header.Get("Content-Type"))
	part, err := multipart.NewReader(strings.NewReader(string(rec.last(t).Body)), params["boundary"]).NextPart()
	if err != nil {
		t.Fatalf("NextPart: %v", err)
	}
	if part.Header.Get("X-Evil") != "" || part.FormName() != "file" {
		t.Errorf("part header = %v", part.Header)
	}
	if want := multipart.FileContentDisposition("file", name); part.Header.Get("Content-Disposition") != want {
		t.Errorf("Content-Disposition = %q, want %q", part.Header.Get("Content-Disposition"), want)
	}
}

// A file that cannot be read completely is an error of the file (exit 1),
// not a network error: the request was never completely sent.
func TestUploadFileErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		r    io.Reader
		size int64
		want string
	}{
		"read error": {io.MultiReader(strings.NewReader("abc"), iotestErrReader{}), 10, "Datei beleg.pdf nicht lesbar: Lesefehler"},
		"shrunk":     {strings.NewReader("abc"), 10, "Datei beleg.pdf hat sich beim Hochladen verändert (3 statt 10 Bytes)"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recorder{status: http.StatusCreated}
			c := newTestClient(t, rec, nil)
			_, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", tc.r, tc.size)
			if err == nil {
				t.Fatalf("no error, want %q", tc.want)
			}
			if ce := clierr.From(err); ce.Code != clierr.CodeError || ce.Message != tc.want {
				t.Errorf("err = %v (%s), want %q", err, ce.Code, tc.want)
			}
		})
	}
}

type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) { return 0, errors.New("Lesefehler") }

// A file that grew after its size was taken is sent with that size only.
func TestUploadSendsGivenSize(t *testing.T) {
	rec := &recorder{status: http.StatusCreated}
	c := newTestClient(t, rec, nil)
	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("abcdef"), 3); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	_, params, _ := mime.ParseMediaType(rec.last(t).Header.Get("Content-Type"))
	part, _ := multipart.NewReader(strings.NewReader(string(rec.last(t).Body)), params["boundary"]).NextPart()
	if data, _ := io.ReadAll(part); string(data) != "abc" {
		t.Errorf("part data = %q", data)
	}
}

func TestMediaType(t *testing.T) {
	for name, want := range map[string]string{
		"beleg.pdf":       "application/pdf",
		"Quittung.PNG":    "image/png",
		"foto.jpg":        "image/jpeg",
		"beleg.zzunknown": "application/octet-stream",
		"ohne-endung":     "application/octet-stream",
		"punkt.am.ende.":  "application/octet-stream",
	} {
		if got := MediaType(name); got != want {
			t.Errorf("MediaType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTemplateURL(t *testing.T) {
	for _, base := range []string{"https://firma.papierkram.de/api/v1", "https://firma.papierkram.de/api/v1/"} {
		c := New(Options{BaseURL: base})
		if got := c.TemplateURL("/expense/vouchers/{voucher_id}/documents"); got != "https://firma.papierkram.de/api/v1/expense/vouchers/{voucher_id}/documents" {
			t.Errorf("%s: TemplateURL = %q", base, got)
		}
	}
}
