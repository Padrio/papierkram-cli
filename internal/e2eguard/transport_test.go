package e2eguard

import (
	"errors"
	"io"
	"net/http"
	"net/url"
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
