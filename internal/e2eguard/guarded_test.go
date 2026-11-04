package e2eguard

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/cli"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

const (
	payBody     = `{"value":null,"banking_transaction":{"id":999999999}}`
	deliverBody = `{"send_via":"pdf"}`
	payPath     = "/expense/vouchers/7/pay"
	deliverPath = "/income/estimates/7/deliver"
)

// sendRaw sends method to rawURL (the full URL as given, not re-encoded)
// with body and the headers hdr (key, value pairs; a key may repeat).
func sendRaw(t *testing.T, tr *Transport, method, rawURL, body string, hdr ...string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Add(hdr[i], hdr[i+1])
	}
	return (&http.Client{Transport: tr}).Do(req)
}

// A pay or deliver request goes out only in its one canonical form:
// POST, exact path, no query, Content-Type application/json (charset
// utf-8 allowed), no Content-Encoding. Everything else is refused before
// sending (review round 2, R2-1): Rails merges query parameters into the
// body parameters, honours format suffixes and does not parse JSON sent
// as another type.
func TestTransportRefusesNonCanonicalGuardedRequests(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, u := newTestTransport(t, srv, 1000)
	base := u.String() // …/api/v1
	root := u.Scheme + "://" + u.Host
	jsonCT := []string{"Content-Type", "application/json"}
	cases := []struct {
		name, method, url, body string
		hdr                     []string
	}{
		// query, any
		{"pay ?value=5", "POST", base + payPath + "?value=5", payBody, jsonCT},
		{"pay ?banking_transaction[id]=471", "POST", base + payPath + "?banking_transaction%5Bid%5D=471", payBody, jsonCT},
		{"pay empty ?", "POST", base + payPath + "?", payBody, jsonCT},
		{"deliver ?send_via=email", "POST", base + deliverPath + "?send_via=email", deliverBody, jsonCT},
		{"deliver empty ?", "POST", base + deliverPath + "?", deliverBody, jsonCT},
		// path variants
		{"pay.json", "POST", base + payPath + ".json", payBody, jsonCT},
		{"deliver.json", "POST", base + deliverPath + ".json", deliverBody, jsonCT},
		{"pay/", "POST", base + payPath + "/", payBody, jsonCT},
		{"deliver/", "POST", base + deliverPath + "/", deliverBody, jsonCT},
		{"PAY", "POST", base + "/expense/vouchers/7/PAY", payBody, jsonCT},
		{"DELIVER", "POST", base + "/income/estimates/7/DELIVER", deliverBody, jsonCT},
		{"Deliver", "POST", base + "/income/invoices/7/Deliver", deliverBody, jsonCT},
		{"pay;x", "POST", base + payPath + ";x", payBody, jsonCT},
		{"deliver;x", "POST", base + deliverPath + ";x", deliverBody, jsonCT},
		{"p%61y", "POST", base + "/expense/vouchers/7/p%61y", payBody, jsonCT},
		{"pay/extra", "POST", base + payPath + "/extra", payBody, jsonCT},
		{"pay without /api/v1", "POST", root + payPath, payBody, jsonCT},
		{"pay at another resource", "POST", base + "/income/invoices/7/pay", payBody, jsonCT},
		{"deliver at vouchers", "POST", base + "/expense/vouchers/7/deliver", deliverBody, jsonCT},
		{"pay with non-numeric id", "POST", base + "/expense/vouchers/x/pay", payBody, jsonCT},
		{"double slash", "POST", base + "/expense//vouchers/7/pay", payBody, jsonCT},
		// method
		{"PUT pay", "PUT", base + payPath, payBody, jsonCT},
		{"GET deliver", "GET", base + deliverPath, "", nil},
		// Content-Type
		{"pay text/plain", "POST", base + payPath, payBody, []string{"Content-Type", "text/plain"}},
		{"deliver form", "POST", base + deliverPath, deliverBody, []string{"Content-Type", "application/x-www-form-urlencoded"}},
		{"pay no Content-Type", "POST", base + payPath, payBody, nil},
		{"pay charset latin1", "POST", base + payPath, payBody, []string{"Content-Type", "application/json; charset=latin1"}},
		{"pay other parameter", "POST", base + payPath, payBody, []string{"Content-Type", "application/json; x=1"}},
		{"pay two types", "POST", base + payPath, payBody, []string{"Content-Type", "application/json", "Content-Type", "text/plain"}},
		{"pay unparsable type", "POST", base + payPath, payBody, []string{"Content-Type", "application/json, text/plain"}},
		{"pay vnd json", "POST", base + payPath, payBody, []string{"Content-Type", "application/vnd.api+json"}},
		// Content-Encoding, any
		{"pay gzip", "POST", base + payPath, payBody, append([]string{"Content-Encoding", "gzip"}, jsonCT...)},
		{"deliver identity", "POST", base + deliverPath, deliverBody, append([]string{"Content-Encoding", "identity"}, jsonCT...)},
	}
	for _, c := range cases {
		resp, err := sendRaw(t, tr, c.method, c.url, c.body, c.hdr...)
		if err == nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrRefused) {
			t.Errorf("%s: err %v, want ErrRefused", c.name, err)
		}
	}
	if q := srv.Requests(); len(q) != 0 {
		var got []string
		for _, r := range q {
			got = append(got, r.Method+" "+r.Path+"?"+r.Query.Encode())
		}
		t.Errorf("server got %d requests, want 0: %v", len(q), got)
	}
}

// The canonical forms still go out (charset utf-8 in any case allowed).
func TestTransportAllowsCanonicalGuardedRequests(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, u := newTestTransport(t, srv, 1000)
	for _, c := range []struct{ path, body, ct string }{
		{payPath, payBody, "application/json"},
		{payPath, payBody, "application/json; charset=utf-8"},
		{deliverPath, deliverBody, "application/json; charset=UTF-8"},
		{"/income/invoices/12/deliver", deliverBody, "Application/JSON"},
	} {
		resp, err := sendRaw(t, tr, "POST", u.String()+c.path, c.body, "Content-Type", c.ct)
		if err != nil {
			t.Errorf("%s (%s): %v", c.path, c.ct, err)
			continue
		}
		_ = resp.Body.Close()
	}
	if n := len(srv.Requests()); n != 4 {
		t.Errorf("server got %d requests, want 4", n)
	}
}

// The reviewer's bypass through the real CLI: papierkram api POST
// '/expense/vouchers/<id>/pay?value=5' --input - with the sentinel body
// must be refused before anything is sent; without the query it goes out.
func TestTransportRefusesPayQueryThroughCLI(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, _ := newTestTransport(t, srv, 1000)
	run := func(path string) (int, string) {
		env := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-cli"}
		var out, errb bytes.Buffer
		dir := t.TempDir()
		app := &cli.App{
			In: strings.NewReader(payBody), Out: &out, Err: &errb,
			Env: func(k string) string { return env[k] }, Now: time.Now, Location: time.UTC,
			ConfigDir: dir, CacheDir: dir,
			HTTPClient:   &http.Client{Transport: tr},
			Keyring:      auth.NewMemory(),
			Getwd:        func() (string, error) { return dir, nil },
			ReadPassword: func() ([]byte, error) { return nil, io.EOF },
			Sleep:        func(time.Duration) {},
		}
		code := cli.Run(app, []string{"--no-input", "api", "POST", path, "--input", "-", "--yes"})
		return code, errb.String()
	}
	code, stderr := run(payPath + "?value=5")
	if code == 0 || len(srv.Requests()) != 0 {
		t.Fatalf("pay?value=5: exit %d, %d requests sent; stderr:\n%s", code, len(srv.Requests()), stderr)
	}
	if !strings.Contains(stderr, "abgelehnt") {
		t.Errorf("stderr does not name the refusal:\n%s", stderr)
	}
	if code, stderr := run(payPath); code != 0 || len(srv.Requests()) != 1 {
		t.Errorf("canonical pay: exit %d, %d requests; stderr:\n%s", code, len(srv.Requests()), stderr)
	}
	if q := srv.Requests(); len(q) == 1 && len(q[0].Query) != 0 {
		t.Errorf("canonical pay sent a query: %v", q[0].Query)
	}
}

// sendMut is sendRaw with Content-Type application/json, then mut may
// change the request (e.g. write header keys in non-canonical form).
func sendMut(t *testing.T, tr *Transport, method, rawURL, body string, mut func(*http.Request)) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	if mut != nil {
		mut(req)
	}
	return (&http.Client{Transport: tr}).Do(req)
}

// Review round 3 (R3-1): ambiguous requests count as guarded and are
// refused: segments decoded until stable (more than the cap: refused),
// backslash as separator, control characters, header keys in any case,
// method overrides in headers, query or body.
func TestTransportRefusesAmbiguousGuardedRequests(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, u := newTestTransport(t, srv, 1000)
	base := u.String()
	payAmount := `{"value":5}`
	nested := "p%" + strings.Repeat("25", 10) + "61y" // 11 decodes to "pay"
	set := func(k, v string) func(*http.Request) {
		return func(r *http.Request) { r.Header[k] = append(r.Header[k], v) }
	}
	cases := []struct {
		name, method, url, body string
		mut                     func(*http.Request)
	}{
		{"triple encoded %252570ay", "POST", base + "/expense/vouchers/7/%252570ay", payAmount, nil},
		{"double encoded d%2565liver", "POST", base + "/income/estimates/7/d%2565liver", `{"send_via":"email"}`, nil},
		{"decoding beyond the cap", "POST", base + "/expense/vouchers/7/" + nested, payAmount, nil},
		{"backslash 7%5Cpay", "POST", base + "/expense/vouchers/7%5Cpay", payAmount, nil},
		{"encoded slash 7%2Fpay", "POST", base + "/expense/vouchers/7%2Fpay", payAmount, nil},
		{"NUL pay%00", "POST", base + "/expense/vouchers/7/pay%00", payAmount, nil},
		{"control pay%01", "POST", base + "/expense/vouchers/7/pay%01", payAmount, nil},
		{"DEL pay%7F", "POST", base + "/expense/vouchers/7/pay%7F", payAmount, nil},
		{"NUL in another segment", "POST", base + "/expense/vouchers/7%00/pay", payBody, nil},
		{"lower-case second content-type", "POST", base + payPath, payBody, set("content-type", "text/plain")},
		{"lower-case content-encoding", "POST", base + payPath, payBody, set("content-encoding", "gzip")},
		{"X-HTTP-Method-Override", "POST", base + payPath, payBody, set("X-HTTP-Method-Override", "PUT")},
		{"x-http-method-override lower case", "POST", base + deliverPath, deliverBody, set("x-http-method-override", "PUT")},
		{"X-HTTP-Method", "POST", base + payPath, payBody, set("X-Http-Method", "PUT")},
		{"X-Method-Override", "POST", base + deliverPath, deliverBody, set("X-Method-Override", "PATCH")},
		{"_method in the query", "POST", base + payPath + "?_method=PUT", payBody, nil},
		{"_method in the deliver body", "POST", base + deliverPath, `{"send_via":"pdf","_method":"PUT"}`, nil},
		{"_METHOD in the deliver body", "POST", base + deliverPath, `{"send_via":"pdf","_METHOD":"PUT"}`, nil},
		{"_method in the pay body", "POST", base + payPath, `{"value":null,"banking_transaction":{"id":999999999},"_method":"PUT"}`, nil},
	}
	for _, c := range cases {
		resp, err := sendMut(t, tr, c.method, c.url, c.body, c.mut)
		if err == nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrRefused) {
			t.Errorf("%s: err %v, want ErrRefused", c.name, err)
		}
	}
	if q := srv.Requests(); len(q) != 0 {
		var got []string
		for _, r := range q {
			got = append(got, r.Method+" "+r.Path)
		}
		t.Errorf("server got %d requests, want 0: %v", len(q), got)
	}
}

// Paths that only contain the letters of pay or deliver stay unguarded:
// payment_terms is "paymentterms" after keeping a-z.
func TestTransportLeavesPaymentTermsAlone(t *testing.T) {
	srv := mockapi.New(t, mockapi.WithoutAutoFail())
	tr, u := newTestTransport(t, srv, 1000)
	for _, p := range []string{"/income/payment_terms", "/income/payment_terms/3"} {
		req, err := http.NewRequest("GET", u.String()+p, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := (&http.Client{Transport: tr}).Do(req)
		if err != nil {
			t.Errorf("GET %s: %v", p, err)
			continue
		}
		_ = resp.Body.Close()
	}
	if n := len(srv.Requests()); n != 2 {
		t.Errorf("server got %d requests, want 2", n)
	}
}
