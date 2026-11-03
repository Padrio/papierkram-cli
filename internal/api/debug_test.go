package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
		"< X-Consumed-Quota: 1000\n",
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

// Non-UTF-8 bodies (e.g. an image download) are binary whatever the type.
func TestDebugInvalidUTF8SizeOnly(t *testing.T) {
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

	if _, err := c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("%PDF-1.4 geheim")); err != nil {
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
