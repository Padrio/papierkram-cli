package e2eguard

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

const testToken = "test-token-transport"

// newTestTransport is a Transport that allows exactly the host of srv.
func newTestTransport(t *testing.T, srv *mockapi.Server, budget int) (*Transport, *url.URL) {
	t.Helper()
	u, err := url.Parse(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	return &Transport{Base: http.DefaultTransport, Scheme: u.Scheme, Host: u.Host, Meter: NewMeter(budget, 0)}, u
}

func send(t *testing.T, tr *Transport, method, rawURL, body string) (*http.Response, error) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Transport: tr}).Do(req)
}

func TestTransportRecordsAndMeters(t *testing.T) {
	srv := mockapi.New(t)
	srv.On("GET", "/info", 200, []byte(`{"api":{"version":"1.0.0"}}`), "X-Consumed-Quota", "41", "X-Remaining-Quota", "959")
	tr, u := newTestTransport(t, srv, 100)
	resp, err := send(t, tr, "GET", u.String()+"/info", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "1.0.0") {
		t.Errorf("caller got body %q", body)
	}
	ex := tr.Exchanges()
	if len(ex) != 1 {
		t.Fatalf("%d exchanges", len(ex))
	}
	e := ex[0]
	if e.Method != "GET" || e.Path != "/api/v1/info" || e.Status != 200 || !strings.Contains(string(e.RespBody), "1.0.0") {
		t.Errorf("exchange %+v", e)
	}
	if e.ReqHeader.Get("Authorization") != "" || strings.Contains(strings.Join(e.ReqHeader.Values("Authorization"), ""), testToken) {
		t.Error("recorded request keeps the Authorization header")
	}
	if e.ReqHeader.Get("Accept") != "application/json" || e.RespHeader.Get("X-Consumed-Quota") != "41" {
		t.Errorf("headers req %v resp %v", e.ReqHeader, e.RespHeader)
	}
	if tr.Meter.Requests() != 1 || tr.Meter.Quota().Remaining != 959 {
		t.Errorf("meter requests %d quota %+v", tr.Meter.Requests(), tr.Meter.Quota())
	}
	if tr.Len() != 1 || len(tr.Since(1)) != 0 || len(tr.Since(0)) != 1 {
		t.Errorf("Len %d Since(1) %d Since(0) %d", tr.Len(), len(tr.Since(1)), len(tr.Since(0)))
	}
}

// The query of a request is recorded (filters such as company_id), the
// path stays without it.
func TestTransportRecordsQuery(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 100)
	resp, err := send(t, tr, "GET", u.String()+"/income/invoices?company_id=7&page=1", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	e := tr.Exchanges()[0]
	if e.Query != "company_id=7&page=1" || !strings.HasSuffix(e.Path, "/income/invoices") {
		t.Errorf("recorded path %q query %q", e.Path, e.Query)
	}
}

// The request body reaches the server unchanged and is recorded.
func TestTransportKeepsRequestBody(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 100)
	resp, err := send(t, tr, "POST", u.String()+"/income/invoices/7/deliver", `{"send_via":"pdf"}`)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	reqs := srv.Requests()
	if len(reqs) != 1 || string(reqs[0].Body) != `{"send_via":"pdf"}` {
		t.Fatalf("server got %+v", reqs)
	}
	if got := string(tr.Exchanges()[0].ReqBody); got != `{"send_via":"pdf"}` {
		t.Errorf("recorded body %q", got)
	}
}

// Only the sandbox host is ever contacted; nothing is sent elsewhere.
func TestTransportRefusesOtherTargets(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 100)
	for _, raw := range []string{
		"http://example.org/api/v1/info",
		"https://" + u.Host + "/api/v1/info",
		"http://other." + u.Host + "/api/v1/info",
	} {
		if _, err := send(t, tr, "GET", raw, ""); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: err %v, want ErrRefused", raw, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("server got %d requests", n)
	}
}

// Hard safety rule: deliver goes out only with send_via "pdf", never
// "email" (and not without a readable body: the API mails then).
func TestTransportRefusesDeliverOtherThanPDF(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 100)
	for _, body := range []string{
		`{"send_via":"email","email":{"recipient":"a@example.org","subject":"s","body":"b"}}`,
		`{"send_via":"EMAIL"}`,
		`{}`,
		``,
		`not json`,
		`{"send_via":"pdf","send_via":"email"}`,
		// keys that differ only by case: encoding/json would match either
		`{"send_via":"email","SEND_VIA":"pdf"}`,
		`{"send_via":"email","Send_Via":"pdf"}`,
		`{"SEND_VIA":"pdf"}`,
		`{"send_via":"pdf","Email":{"recipient":"a@example.org"}}`,
		`{"send_via":"pdf","email":null}`,
		`{"send_via":"pdf","x":{"a":1,"A":2}}`,
		`{"send_via":"pdf","x":[{"a":1,"a":2}]}`,
		`{"send_via":"pdf"} {"send_via":"email"}`,
		`["send_via","pdf"]`,
		`{"send_via":"pdf"`,
	} {
		for _, path := range []string{"/income/invoices/7/deliver", "/income/estimates/7/deliver", "/income/estimates/7/deliver/"} {
			if _, err := send(t, tr, "POST", u.String()+path, body); !errors.Is(err, ErrRefused) {
				t.Errorf("%s %q: err %v, want ErrRefused", path, body, err)
			}
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("server got %d requests", n)
	}
}

func TestTransportStopsAtBudget(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 1)
	resp, err := send(t, tr, "GET", u.String()+"/users", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if _, err := send(t, tr, "GET", u.String()+"/users", ""); !errors.Is(err, ErrBudget) {
		t.Fatalf("second request: %v, want ErrBudget", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("server got %d requests, want 1", n)
	}
}

// Ruling (fix round 1): the suite never pays. A pay request goes out only
// with exactly {"value": null, "banking_transaction": {"id": NoTransactionID}};
// everything else is refused before sending.
func TestTransportPayOnlyWithSentinel(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, u := newTestTransport(t, srv, 100)
	for _, body := range []string{
		`{"value":1,"banking_transaction":{"id":999999999}}`,
		`{"value":"1"}`,
		`{"value":0}`,
		`{"value":null}`,
		`{"value":null,"value":5}`,
		`{}`,
		``,
		`not json`,
		`null`,
		// the documented way to pay with a bank transaction (no value)
		`{"banking_transaction":{"id":999999999}}`,
		`{"banking_transaction":{"id":471}}`,
		`{"value":null,"banking_transaction":{"id":471}}`,
		`{"value":null,"banking_transaction":{"id":"999999999"}}`,
		`{"value":null,"banking_transaction":{"id":999999999.0}}`,
		`{"value":null,"banking_transaction":{"id":999999999,"x":1}}`,
		`{"value":null,"banking_transaction":null}`,
		`{"value":null,"banking_transaction":{"id":999999999},"payment_date":"2026-01-01"}`,
		`{"payment_date":"2026-01-01"}`,
		`{"value":null,"banking_transaction":{"id":999999999},"Value":5}`,
		`{"value":null,"banking_transaction":{"id":999999999,"ID":471}}`,
		`{"value":null,"banking_transaction":{"id":999999999},"banking_transaction":{"id":471}}`,
		`{"value":null,"banking_transaction":{"id":999999999}} {}`,
	} {
		if _, err := send(t, tr, "POST", u.String()+"/expense/vouchers/7/pay", body); !errors.Is(err, ErrRefused) {
			t.Errorf("pay %q: err %v, want ErrRefused", body, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("server got %d requests", n)
	}
	for _, body := range []string{
		`{"value":null,"banking_transaction":{"id":999999999}}`,
		` { "banking_transaction" : { "id" : 999999999 } , "value" : null } `,
	} {
		resp, err := send(t, tr, "POST", u.String()+"/expense/vouchers/7/pay", body)
		if err != nil {
			t.Errorf("pay %q: %v", body, err)
			continue
		}
		_ = resp.Body.Close()
	}
	if n := len(srv.Requests()); n != 2 {
		t.Errorf("server got %d requests, want 2", n)
	}
}

func TestNoTransactionID(t *testing.T) {
	if NoTransactionID != 999999999 {
		t.Errorf("NoTransactionID = %d", NoTransactionID)
	}
}

// A deliver as PDF with an ordinary body still goes out.
func TestTransportAllowsPlainDeliverPDF(t *testing.T) {
	srv := mockapi.New(t)
	tr, u := newTestTransport(t, srv, 100)
	resp, err := send(t, tr, "POST", u.String()+"/income/estimates/7/deliver", ` {"send_via" : "pdf"} `)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

// The base transport of the suite ignores the proxy variables of the
// shell: a (TLS-terminating) proxy would see the token.
func TestDirectTransportIgnoresProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("NO_PROXY", "")
	b := DirectTransport()
	if b.Proxy != nil {
		req, _ := http.NewRequest("GET", "https://example-sandbox.papierkram.de/api/v1/info", nil)
		u, err := b.Proxy(req)
		t.Fatalf("Proxy is set (would use %v, %v)", u, err)
	}
	if b == http.DefaultTransport {
		t.Error("DirectTransport returns the shared DefaultTransport")
	}
	if http.DefaultTransport.(*http.Transport).Proxy == nil {
		t.Error("DirectTransport changed http.DefaultTransport")
	}
}

// A rejected request is charged like any other (measured 2026-11-04: a
// create answered with 422 costs its 50 credits): the meter counts it.
func TestTransportMetersErrorResponses(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	srv.On("POST", "/income/estimates", 422, []byte(`{"type":"error","message":"Datum muss ausgefüllt werden"}`), "X-Consumed-Quota", "50")
	tr, u := newTestTransport(t, srv, 1000)
	body := `{"name":"A","document_date":"2026-11-04","line_items":[{"name":"A","quantity":1,"price":5,"vat_rate":0.19}]}`
	resp, err := send(t, tr, "POST", u.String()+"/income/estimates", body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 422 || tr.Meter.Spent() != 50 || tr.Meter.Requests() != 1 {
		t.Errorf("status %d spent %d requests %d, want 422, 50, 1", resp.StatusCode, tr.Meter.Spent(), tr.Meter.Requests())
	}
}

// failingBase fails every request after it was handed over, like a
// connection that breaks before the answer.
type failingBase struct{ calls int }

func (f *failingBase) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls++
	return nil, errors.New("connection reset")
}

// A request that got no answer may still have been charged: it counts
// with its documented cost.
func TestTransportMetersUnansweredRequests(t *testing.T) {
	base := &failingBase{}
	tr := &Transport{Base: base, Scheme: "https", Host: "sandbox.example", Meter: NewMeter(1000, 0)}
	if _, err := send(t, tr, "POST", "https://sandbox.example/api/v1/income/estimates", `{"name":"A"}`); err == nil {
		t.Fatal("no error")
	}
	if base.calls != 1 || tr.Meter.Spent() != 50 || tr.Meter.Requests() != 1 || tr.Meter.Unmeasured() != 1 {
		t.Errorf("calls %d spent %d requests %d unmeasured %d, want 1, 50, 1, 1",
			base.calls, tr.Meter.Spent(), tr.Meter.Requests(), tr.Meter.Unmeasured())
	}
}

// Coverage counts every answered request, whatever its status; a refused
// (host, budget, pay body) or unanswered request does not count.
func TestTransportRecordsCoverage(t *testing.T) {
	srv := mockapi.New(t)
	srv.On("GET", "/info", 200, []byte(`{}`))
	srv.On("GET", "/users/4", 404, []byte(`{"message":"nicht gefunden"}`))
	tr, u := newTestTransport(t, srv, 2)
	cov := NewCoverage()
	tr.Coverage = cov
	for _, r := range []struct{ method, url, body string }{
		{"GET", u.String() + "/info", ""},
		{"GET", u.String() + "/users/4", ""},
	} {
		resp, err := send(t, tr, r.method, r.url, r.body)
		if err != nil {
			t.Fatalf("%s %s: %v", r.method, r.url, err)
		}
		_ = resp.Body.Close()
	}
	if _, err := send(t, tr, "GET", "http://example.org/api/v1/users", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("other host: %v, want ErrRefused", err)
	}
	if _, err := send(t, tr, "POST", u.String()+"/expense/vouchers/7/pay", `{"value":5}`); !errors.Is(err, ErrRefused) {
		t.Errorf("pay: %v, want ErrRefused", err)
	}
	if _, err := send(t, tr, "GET", u.String()+"/payment_terms", ""); !errors.Is(err, ErrBudget) {
		t.Errorf("budget: %v, want ErrBudget", err)
	}
	failing := &Transport{Base: &failingBase{}, Scheme: "https", Host: "sandbox.example", Meter: NewMeter(1000, 0), Coverage: cov}
	if _, err := send(t, failing, "POST", "https://sandbox.example/api/v1/income/estimates", `{"name":"A"}`); err == nil {
		t.Fatal("unanswered request: no error")
	}

	plan := map[string]Expectation{
		"GetInfo":          {Test: "TestA"},
		"GetUser":          {Test: "TestA", Mode: ModeAnswered, Note: "n"},
		"ListUsers":        {Test: "TestA", Mode: ModeAnswered, Note: "n"},
		"PayVoucher":       {Test: "TestA", Mode: ModeAnswered, Note: "n"},
		"ListPaymentTerms": {Test: "TestA", Mode: ModeAnswered, Note: "n"},
		"CreateEstimate":   {Test: "TestA", Mode: ModeAnswered, Note: "n"},
	}
	r := cov.Report(plan)
	if want := []string{"GetInfo", "GetUser"}; !slices.Equal(r.Covered, want) {
		t.Errorf("Covered %v, want %v", r.Covered, want)
	}
	if want := []string{"CreateEstimate", "ListPaymentTerms", "ListUsers", "PayVoucher"}; !slices.Equal(r.Missing, want) {
		t.Errorf("Missing %v, want %v", r.Missing, want)
	}
	if len(r.Unknown) != 0 {
		t.Errorf("Unknown %v", r.Unknown)
	}
}
