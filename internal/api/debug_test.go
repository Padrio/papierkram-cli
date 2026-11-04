package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// debugClient returns a client whose debug log goes to the returned buffer.
func debugClient(t *testing.T, rec *recorder, mod func(*Options)) (*Client, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	c := newTestClient(t, rec, func(o *Options) {
		o.Debug = &buf
		if mod != nil {
			mod(o)
		}
	})
	return c, &buf
}

func TestR26_DebugRedactsToken(t *testing.T) {
	const token = "test-token-r26"
	rec := &recorder{
		header: map[string]string{"Content-Type": "application/json"},
		body:   `{"echo":"` + token + `"}`,
	}
	c, buf := debugClient(t, rec, func(o *Options) { o.Token = token })

	if _, err := c.CallRaw(context.Background(), http.MethodGet, "/income/invoices?page=1&api_key="+token, nil, "", nil); err != nil {
		t.Fatalf("CallRaw GET: %v", err)
	}
	if _, err := c.CallRaw(context.Background(), http.MethodPost, "/income/invoices", nil, "application/json",
		strings.NewReader(`{"note":"`+token+`"}`)); err != nil {
		t.Fatalf("CallRaw POST: %v", err)
	}
	// The real request still carries the real token.
	if got := rec.last(t).Header.Get("Authorization"); got != "Bearer "+token {
		t.Errorf("sent Authorization = %q", got)
	}

	out := buf.String()
	if strings.Contains(out, "test-token-") {
		t.Fatalf("debug output contains the token:\n%s", out)
	}
	for _, want := range []string{
		"> Authorization: Bearer ***\n",
		"/api/v1/income/invoices?page=1&api_key=***\n",
		`< {"echo":"***"}`,
		`> {"note":"***"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output lacks %q:\n%s", want, out)
		}
	}
}

func TestDebugRequestAndResponse(t *testing.T) {
	rec := &recorder{
		header: map[string]string{"Content-Type": "application/json", "X-Remaining-Quota": "9000", "X-Consumed-Quota": "1000"},
		body:   "{\"type\":\"list\",\n\"page\":1}",
	}
	c, buf := debugClient(t, rec, nil)

	if _, err := c.Call(context.Background(), "UpdateInvoice", []int64{5}, nil, map[string]any{"name": "Ä"}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"> PUT http://",
		"/api/v1/income/invoices/5\n",
		"> Accept: application/json\n",
		"> Content-Type: application/json\n",
		"> User-Agent: papierkram-cli/",
		"> {\"name\":\"Ä\"}\n",
		"< X-Remaining-Quota: 9000\n",
		"< X-Consumed-Quota: 1000 (Kosten dieser Anfrage)\n",
		"< {\"type\":\"list\",\n< \"page\":1}\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output lacks %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`(?m)^< 200 OK \(\d+ms\)$`).MatchString(out) {
		t.Errorf("no status line with duration:\n%s", out)
	}
	if strings.Contains(out, "Versuch") {
		t.Errorf("first attempt must not be marked as retry:\n%s", out)
	}
}

func TestDebugNoQuotaLineWhenUnknown(t *testing.T) {
	c, buf := debugClient(t, &recorder{}, nil)
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if strings.Contains(buf.String(), "Quota") {
		t.Errorf("quota line without quota header:\n%s", buf)
	}
}

func TestDebugTruncatesLongBodies(t *testing.T) {
	rec := &recorder{body: strings.Repeat("a", 5000)}
	c, buf := debugClient(t, rec, nil)

	// JSON body {"x":"bbb…"} is 5000+8 bytes.
	if _, err := c.Call(context.Background(), "UpdateInvoice", []int64{5}, nil, map[string]any{"x": strings.Repeat("b", 5000)}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := len(rec.last(t).Body); got != 5008 {
		t.Fatalf("server received %d bytes, want the full 5008", got)
	}
	out := buf.String()
	if !strings.Contains(out, "< "+strings.Repeat("a", 4096)+"… (gekürzt, 5000 Bytes)\n") {
		t.Errorf("response body not truncated to 4096 bytes with marker:\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("a", 4097)) {
		t.Error("response body longer than 4096 bytes in debug output")
	}
	if !strings.Contains(out, `> {"x":"`+strings.Repeat("b", 4096-6)+"… (gekürzt, 5008 Bytes)\n") {
		t.Errorf("request body not truncated to 4096 bytes with marker:\n%s", out)
	}
}

func TestDebugPDFResponseSizeOnly(t *testing.T) {
	pdf := "%PDF-1.4 binary \x00\x01 stuff"
	rec := &recorder{header: map[string]string{"Content-Type": "application/pdf"}, body: pdf}
	c, buf := debugClient(t, rec, nil)

	if _, err := c.CallRaw(context.Background(), http.MethodGet, "/income/invoices/5/pdf", nil, "", nil); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	out := buf.String()
	if want := "< <binär, 24 Bytes>\n"; !strings.Contains(out, want) {
		t.Errorf("debug output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, "%PDF") {
		t.Errorf("PDF content in debug output:\n%s", out)
	}
}

// Image downloads (e.g. a voucher document) are binary by Content-Type.
func TestDebugImageResponseSizeOnly(t *testing.T) {
	rec := &recorder{header: map[string]string{"Content-Type": "image/jpeg"}, body: "\xff\xd8\xff\xe0"}
	c, buf := debugClient(t, rec, nil)
	if _, err := c.CallRaw(context.Background(), http.MethodGet, "/expense/vouchers/3/documents/1", nil, "", nil); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if want := "< <binär, 4 Bytes>\n"; !strings.Contains(buf.String(), want) {
		t.Errorf("debug output lacks %q:\n%s", want, buf)
	}
}

func TestDebugMultipartUploadSizeOnly(t *testing.T) {
	rec := &recorder{status: http.StatusCreated}
	c, buf := debugClient(t, rec, nil)

	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("%PDF-1.4 geheim"), 15); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	sent := len(rec.last(t).Body)
	out := buf.String()
	if want := "> <binär, " + strconv.Itoa(sent) + " Bytes>\n"; !strings.Contains(out, want) {
		t.Errorf("debug output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, "geheim") || strings.Contains(out, "beleg.pdf") {
		t.Errorf("upload content in debug output:\n%s", out)
	}
}

func TestDebugLogsEveryAttempt(t *testing.T) {
	srv, n := sequenceServer(t, 503, 503, 200)
	var buf bytes.Buffer
	s := &sleepRecorder{}
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: s.sleep, Debug: &buf, DebugInfo: "Schreibschutz: env"})

	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if n.Load() != 3 {
		t.Fatalf("requests = %d, want 3", n.Load())
	}
	out := buf.String()
	if got := strings.Count(out, "* Schreibschutz: env\n"); got != 1 {
		t.Errorf("info line %d times, want once per request:\n%s", got, out)
	}
	if got := strings.Count(out, "< 503 Service Unavailable ("); got != 2 {
		t.Errorf("503 lines = %d, want 2:\n%s", got, out)
	}
	for _, want := range []string{"/api/v1/income/invoices (Versuch 2)\n", "/api/v1/income/invoices (Versuch 3)\n", "< 200 OK ("} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output lacks %q:\n%s", want, out)
		}
	}
}

func TestDebugLogsTransportError(t *testing.T) {
	srv, _ := closingServer(t, 1)
	var buf bytes.Buffer
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Debug: &buf})

	if _, err := c.Call(context.Background(), "CreateInvoice", nil, nil, map[string]any{"name": "x"}); err == nil {
		t.Fatal("expected an error")
	}
	if !regexp.MustCompile(`(?m)^< Fehler: .+ \(\d+ms\)$`).MatchString(buf.String()) {
		t.Errorf("no error line:\n%s", buf.String())
	}
}

// Without Options.Debug nothing is written, not even to stderr.
func TestDebugNilWritesNothing(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = stderr }()

	rec := &recorder{body: `{"type":"list"}`}
	c := newTestClient(t, rec, func(o *Options) { o.DebugInfo = "Schreibschutz: env" })
	_, callErr := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	os.Stderr = stderr
	_ = w.Close()
	got, _ := io.ReadAll(r)
	if callErr != nil {
		t.Fatalf("Call: %v", callErr)
	}
	if len(got) != 0 {
		t.Errorf("stderr = %q, want nothing", got)
	}
}

// A body without GetBody (e.g. a file stream) cannot be read for the log
// without consuming it; only its size is shown and it is sent unchanged.
func TestDebugUnreplayableBodyNotRead(t *testing.T) {
	rec := &recorder{}
	c, buf := debugClient(t, rec, nil)
	body := io.MultiReader(strings.NewReader(`{"a":1}`))
	if _, err := c.CallRaw(context.Background(), http.MethodPost, "/income/invoices", nil, "application/json", body); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if got := string(rec.last(t).Body); got != `{"a":1}` {
		t.Errorf("server received %q", got)
	}
	if want := "> <nicht protokolliert, Größe unbekannt>\n"; !strings.Contains(buf.String(), want) {
		t.Errorf("debug output lacks %q:\n%s", want, buf)
	}
}

// The token is also removed from Go's transport error text, which quotes
// the full URL including the query.
func TestDebugRedactsTokenInTransportError(t *testing.T) {
	const token = "test-token-err"
	srv, _ := closingServer(t, 1)
	var buf bytes.Buffer
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: token, Debug: &buf})

	if _, err := c.CallRaw(context.Background(), http.MethodPost, "/income/invoices?api_key="+token, nil, "", nil); err == nil {
		t.Fatal("expected an error")
	}
	out := buf.String()
	if strings.Contains(out, "test-token-") {
		t.Fatalf("debug output contains the token:\n%s", out)
	}
	if !regexp.MustCompile(`(?m)^< Fehler: .*api_key=\*\*\*.* \(\d+ms\)$`).MatchString(out) {
		t.Errorf("no redacted error line:\n%s", out)
	}
}

// A token with characters that change under URL encoding is removed both
// in its encoded form (query) and literally (bodies).
func TestDebugRedactsURLEncodedToken(t *testing.T) {
	const token = "test-token-a+b/c"
	rec := &recorder{header: map[string]string{"Content-Type": "application/json"}, body: `{"echo":"` + token + `"}`}
	c, buf := debugClient(t, rec, func(o *Options) { o.Token = token })

	if _, err := c.CallRaw(context.Background(), http.MethodPost, "/income/invoices", url.Values{"api_key": {token}},
		"application/json", strings.NewReader(`{"note":"`+token+`"}`)); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if got := rec.last(t).RawQuery; got != "api_key=test-token-a%2Bb%2Fc" {
		t.Fatalf("query = %q, want the encoded token", got)
	}
	out := buf.String()
	if strings.Contains(out, "test-token-") {
		t.Fatalf("debug output contains the token:\n%s", out)
	}
	for _, want := range []string{"/api/v1/income/invoices?api_key=***\n", `> {"note":"***"}`, `< {"echo":"***"}`} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output lacks %q:\n%s", want, out)
		}
	}
}

// A request body that cannot be copied for the log says so instead of
// looking like an empty body; the error text is redacted as well.
func TestDebugUnreadableRequestBody(t *testing.T) {
	tests := map[string]func() (io.ReadCloser, error){
		"GetBody fails": func() (io.ReadCloser, error) { return nil, errors.New("kaputt " + testToken) },
		"read fails": func() (io.ReadCloser, error) {
			return io.NopCloser(iotest.ErrReader(errors.New("kaputt " + testToken))), nil
		},
	}
	for name, getBody := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			c := New(Options{BaseURL: "http://x/api/v1", Token: testToken, Debug: &buf})
			req, err := http.NewRequest(http.MethodPost, "http://x/api/v1/income/invoices", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.GetBody = getBody
			c.debugRequest(req, 0)
			if want := "> <nicht lesbar: kaputt ***>\n"; !strings.Contains(buf.String(), want) {
				t.Errorf("debug output lacks %q:\n%s", want, buf.String())
			}
		})
	}
}

// A Latin-1 error page from a proxy is shown as text, not hidden as binary.
func TestDebugLatin1ErrorBodyAsText(t *testing.T) {
	rec := &recorder{
		status: http.StatusBadGateway,
		header: map[string]string{"Content-Type": "text/html; charset=iso-8859-1"},
		body:   "<h1>Gateway-Fehler f\xfcr Proxy</h1>",
	}
	c, buf := debugClient(t, rec, func(o *Options) { o.Sleep = func(time.Duration) {} })
	if _, err := c.CallRaw(context.Background(), http.MethodGet, "/income/invoices", nil, "", nil); err == nil {
		t.Fatal("expected an error")
	}
	out := buf.String()
	if want := "< <h1>Gateway-Fehler f\uFFFDr Proxy</h1>\n"; !strings.Contains(out, want) {
		t.Errorf("debug output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, "<binär") {
		t.Errorf("text error body shown as binary:\n%s", out)
	}
}

func TestDebugBodyClassification(t *testing.T) {
	tests := []struct{ contentType, data, want string }{
		// text: JSON (also +json) and text/*; everything else only as size
		{"application/json; charset=utf-8", `{"a":1}`, "< {\"a\":1}\n"},
		{"application/problem+json", `{"a":1}`, "< {\"a\":1}\n"},
		{"text/plain", "hallo", "< hallo\n"},
		{"text/html; charset=iso-8859-1", "abc\xff", "< abc\uFFFD\n"},
		{"", "%PDF-1.4\x00\x1b[31m", "< <binär, 14 Bytes>\n"},
		{"application/x-unknown", "abc", "< <binär, 3 Bytes>\n"},
		{"binary/octet-stream", "%PDF", "< <binär, 4 Bytes>\n"},
		{"application/x-pdf", "%PDF", "< <binär, 4 Bytes>\n"},
		{"kein; gültiger=", "abc", "< <binär, 3 Bytes>\n"},
		{"application/pdf", "%PDF", "< <binär, 4 Bytes>\n"},
		{"multipart/mixed; boundary=x", "abcd", "< <binär, 4 Bytes>\n"},
		{"image/png", "abcd", "< <binär, 4 Bytes>\n"},
		{"Application/Octet-Stream", "abcd", "< <binär, 4 Bytes>\n"},
	}
	c := New(Options{BaseURL: "http://x/api/v1", Token: testToken})
	for _, tt := range tests {
		var b strings.Builder
		c.writeBody(&b, "< ", tt.contentType, []byte(tt.data))
		if b.String() != tt.want {
			t.Errorf("%q: got %q, want %q", tt.contentType, b.String(), tt.want)
		}
	}
}

func TestDebugResponseHeaderLines(t *testing.T) {
	rec := &recorder{
		status: http.StatusTooManyRequests,
		header: map[string]string{"Content-Type": "application/json", "Retry-After": "3600", "Set-Cookie": "s=1"},
		body:   `{"type":"error"}`,
	}
	c, buf := debugClient(t, rec, nil)
	if _, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil); err == nil {
		t.Fatal("expected an error")
	}
	out := buf.String()
	for _, want := range []string{"< 429 Too Many Requests (", "< Content-Type: application/json\n", "< Retry-After: 3600\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Set-Cookie") {
		t.Errorf("other response headers must not be dumped:\n%s", out)
	}
}

// Controller ruling N1: userinfo of the base URL appears in no debug
// line, dry-run URL or error, also when the base URL cannot be parsed;
// the request may still use it.
func TestNoCredentialsFromBaseURL(t *testing.T) {
	leaks := func(t *testing.T, what, s string) {
		t.Helper()
		if strings.Contains(s, "alice") || strings.Contains(s, "s3cret") {
			t.Errorf("%s shows credentials: %s", what, s)
		}
	}
	srv, _ := closingServer(t, 1)
	base := strings.Replace(srv.URL, "http://", "http://alice:s3cret@", 1) + "/api/v1"
	var buf bytes.Buffer
	c := New(Options{BaseURL: base, Token: testToken, Debug: &buf})
	_, err := c.Call(context.Background(), "CreateInvoice", nil, nil, map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("expected an error")
	}
	leaks(t, "debug", buf.String())
	leaks(t, "error", err.Error())
	if !strings.Contains(buf.String(), "> POST "+srv.URL+"/api/v1/income/invoices") {
		t.Errorf("request line:\n%s", buf.String())
	}
	_, u, err := c.Preview("GetInvoice", []int64{1}, nil)
	leaks(t, "Preview", u)
	if err != nil || u != srv.URL+"/api/v1/income/invoices/1" {
		t.Errorf("Preview = %q, %v", u, err)
	}
	leaks(t, "TemplateURL", c.TemplateURL("/expense/vouchers/{voucher_id}/documents"))

	bad := New(Options{BaseURL: "http://alice:s3cret@127.0.0.1:badport/api/v1", Token: testToken, Debug: &buf})
	_, u, err = bad.Preview("GetInvoice", []int64{1}, nil)
	leaks(t, "Preview (bad)", u+fmt.Sprint(err))
	leaks(t, "TemplateURL (bad)", bad.TemplateURL("/expense/vouchers/{voucher_id}/documents"))
	_, err = bad.Call(context.Background(), "GetInvoice", []int64{1}, nil, nil)
	leaks(t, "Call (bad)", fmt.Sprint(err))
	_, err = bad.CallRaw(context.Background(), "GET", "/info", nil, "", nil)
	leaks(t, "CallRaw (bad)", fmt.Sprint(err))
	leaks(t, "debug (bad)", buf.String())
}
