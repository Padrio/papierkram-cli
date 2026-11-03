package mockapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

const testToken = "test-token-mockapi"

// send issues a raw request against the mock. path is relative to URL().
// hdr are key,value pairs; without an Authorization pair a valid bearer
// token is sent.
func send(t *testing.T, s *mockapi.Server, method, path, body string, hdr ...string) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, s.URL()+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i+1] == "" {
			req.Header.Del(hdr[i])
			continue
		}
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, b
}

func client(s *mockapi.Server) *api.Client {
	return api.New(api.Options{BaseURL: s.URL(), Token: testToken})
}

// rawExample digs the first example (by key) of a response out of the raw
// spec bytes, independently of the kin-openapi model the mock uses.
func rawExample(t *testing.T, path, method, status string) any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(spec.Raw(), &doc); err != nil {
		t.Fatal(err)
	}
	get := func(m any, k string) any {
		mm, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("spec: %q not reachable", k)
		}
		return mm[k]
	}
	mt := get(get(get(get(get(get(doc, "paths"), path), method), "responses"), status), "content")
	exs := get(get(mt, "application/json"), "examples").(map[string]any)
	first := ""
	for k := range exs {
		if first == "" || k < first {
			first = k
		}
	}
	return get(exs[first], "value")
}

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, b)
	}
	return v
}

func assertErrorBody(t *testing.T, b []byte, errorType string) map[string]any {
	t.Helper()
	m, ok := decode(t, b).(map[string]any)
	if !ok || m["type"] != "error" || m["error_type"] != errorType {
		t.Fatalf("want API error body with error_type %q, got %s", errorType, b)
	}
	if msg, _ := m["message"].(string); msg == "" {
		t.Errorf("error body without message: %s", b)
	}
	if id, _ := m["correlation_id"].(string); !strings.HasPrefix(id, "mock-") {
		t.Errorf("correlation_id = %v, want mock-…", m["correlation_id"])
	}
	return m
}

func TestURL(t *testing.T) {
	s := mockapi.New(t)
	if !strings.HasSuffix(s.URL(), "/api/v1") || !strings.HasPrefix(s.URL(), "http://") {
		t.Errorf("URL() = %q, want http://…/api/v1", s.URL())
	}
}

// The real API client against the mock: list, quota, multipart upload and
// an empty-bodied action all pass validation.
func TestClientEndToEnd(t *testing.T) {
	s := mockapi.New(t)
	c := client(s)
	ctx := context.Background()

	resp, err := c.Call(ctx, "ListInvoices", nil, url.Values{"page": {"2"}, "page_size": {"10"}}, nil)
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	got := decode(t, resp.Body)
	want := rawExample(t, "/income/invoices", "get", "200")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body is not the spec example\ngot:  %s", resp.Body)
	}
	if env, _ := got.(map[string]any); env["type"] != "list" {
		t.Errorf("want list envelope, got %v", env["type"])
	}
	if resp.Quota != (api.Quota{Remaining: 9000, Consumed: 1000, Known: true}) {
		t.Errorf("quota = %+v", resp.Quota)
	}

	up, err := c.Upload(ctx, "UploadVoucherDocument", []int64{7}, "file", "beleg.pdf", bytes.NewReader([]byte("%PDF-1.4\x00\xff binary")))
	if err != nil {
		t.Fatalf("UploadVoucherDocument: %v", err)
	}
	if up.Status != http.StatusCreated {
		t.Errorf("upload status = %d, want 201", up.Status)
	}

	del, err := c.Call(ctx, "DeliverInvoice", []int64{12}, nil, map[string]any{"send_via": "pdf"})
	if err != nil {
		t.Fatalf("DeliverInvoice: %v", err)
	}
	if del.Status != http.StatusOK || len(del.Body) != 0 {
		t.Errorf("deliver = %d %q, want 200 with empty body", del.Status, del.Body)
	}

	if v := s.Violations(); len(v) != 0 {
		t.Errorf("unexpected violations: %v", v)
	}
	if n := len(s.Requests()); n != 3 {
		t.Errorf("recorded %d requests, want 3", n)
	}
}

func TestUnknownFieldIsViolation(t *testing.T) {
	s := mockapi.New(t, mockapi.WithoutAutoFail())
	resp, b := send(t, s, http.MethodPost, "/income/invoices", `{"bogus":1}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	assertErrorBody(t, b, "mock_spec_violation")
	v := s.Violations()
	if len(v) != 1 {
		t.Fatalf("violations = %v, want exactly one", v)
	}
	if !strings.Contains(v[0], "POST") || !strings.Contains(v[0], "/income/invoices") || !strings.Contains(v[0], "bogus") {
		t.Errorf("violation lacks method, path or cause: %q", v[0])
	}
}

func TestParameterViolations(t *testing.T) {
	for name, path := range map[string]string{
		"unknown query parameter": "/income/invoices?colour=red",
		"wrong query type":        "/income/invoices?page=zwei",
		"bad query enum":          "/income/invoices?order_by=name&order_direction=up",
		"non-integer path id":     "/income/invoices/abc",
	} {
		t.Run(name, func(t *testing.T) {
			s := mockapi.New(t, mockapi.WithoutAutoFail())
			resp, _ := send(t, s, http.MethodGet, path, "")
			if resp.StatusCode != http.StatusInternalServerError || len(s.Violations()) != 1 {
				t.Errorf("status %d, violations %v; want 500 and one violation", resp.StatusCode, s.Violations())
			}
		})
	}
}

func TestBodyViolations(t *testing.T) {
	for name, tc := range map[string]struct{ method, path, body string }{
		"invalid JSON":           {http.MethodPut, "/projects/3", `{"name":`},
		"missing required field": {http.MethodPost, "/income/invoices/12/deliver", `{}`},
		"body on GET":            {http.MethodGet, "/projects/3", `{"name":"x"}`},
	} {
		t.Run(name, func(t *testing.T) {
			s := mockapi.New(t, mockapi.WithoutAutoFail())
			resp, _ := send(t, s, tc.method, tc.path, tc.body)
			if resp.StatusCode != http.StatusInternalServerError || len(s.Violations()) != 1 {
				t.Errorf("status %d, violations %v; want 500 and one violation", resp.StatusCode, s.Violations())
			}
		})
	}
}

func TestMissingAuthorizationIsViolation(t *testing.T) {
	for name, value := range map[string]string{
		"missing":     "",
		"empty token": "Bearer ",
		"basic":       "Basic dXNlcjpwdw==",
	} {
		t.Run(name, func(t *testing.T) {
			s := mockapi.New(t, mockapi.WithoutAutoFail())
			resp, b := send(t, s, http.MethodGet, "/info", "", "Authorization", value)
			if resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", resp.StatusCode)
			}
			assertErrorBody(t, b, "mock_spec_violation")
			v := s.Violations()
			if len(v) != 1 || !strings.Contains(v[0], "Authorization") {
				t.Errorf("violations = %v, want one about Authorization", v)
			}
			if strings.Contains(strings.Join(v, ""), "dXNlcjpwdw") {
				t.Error("violation leaks the credential")
			}
		})
	}
}

// fakeTB records Errorf calls and cleanups so the auto-fail at cleanup can
// be observed without failing this test.
type fakeTB struct {
	testing.TB
	mu       sync.Mutex
	errs     []string
	cleanups []func()
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Cleanup(fn func()) {
	f.cleanups = append(f.cleanups, fn)
}
func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}
func (f *fakeTB) runCleanups() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func TestAutoFailAtCleanup(t *testing.T) {
	ft := &fakeTB{TB: t}
	s := mockapi.New(ft)
	send(t, s, http.MethodPost, "/income/invoices", `{"bogus":1}`)
	send(t, s, http.MethodGet, "/info", "", "Authorization", "")
	if len(ft.errs) != 0 {
		t.Fatalf("Errorf before cleanup: %v", ft.errs)
	}
	ft.runCleanups()
	if len(ft.errs) != 1 {
		t.Fatalf("Errorf calls = %d, want 1: %v", len(ft.errs), ft.errs)
	}
	if !strings.Contains(ft.errs[0], "bogus") || !strings.Contains(ft.errs[0], "Authorization") {
		t.Errorf("cleanup error does not list all violations: %s", ft.errs[0])
	}

	clean := &fakeTB{TB: t}
	cs := mockapi.New(clean)
	send(t, cs, http.MethodGet, "/info", "")
	clean.runCleanups()
	if len(clean.errs) != 0 {
		t.Errorf("Errorf without violations: %v", clean.errs)
	}

	opted := &fakeTB{TB: t}
	optedSrv := mockapi.New(opted, mockapi.WithoutAutoFail())
	send(t, optedSrv, http.MethodGet, "/info", "", "Authorization", "")
	opted.runCleanups()
	if len(opted.errs) != 0 {
		t.Errorf("WithoutAutoFail still failed: %v", opted.errs)
	}
}

func TestOverrides(t *testing.T) {
	s := mockapi.New(t)
	s.On(http.MethodGet, "/income/invoices/{id}", http.StatusOK, []byte(`{"type":"invoice","id":1}`))
	s.On(http.MethodGet, "/income/invoices/12", http.StatusNotFound,
		[]byte(`{"type":"error","error_type":"not_found","message":"Nicht gefunden","correlation_id":"x"}`),
		"X-Remaining-Quota", "5", "X-Extra", "ja")

	resp, b := send(t, s, http.MethodGet, "/income/invoices/12", "")
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(b), "not_found") {
		t.Errorf("concrete override: %d %s", resp.StatusCode, b)
	}
	if resp.Header.Get("X-Remaining-Quota") != "5" || resp.Header.Get("X-Extra") != "ja" {
		t.Errorf("override headers not applied: %v", resp.Header)
	}
	if resp.Header.Get("X-Consumed-Quota") != "1000" {
		t.Errorf("default X-Consumed-Quota missing on override: %v", resp.Header)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	resp, b = send(t, s, http.MethodGet, "/income/invoices/13", "")
	if resp.StatusCode != http.StatusOK || string(b) != `{"type":"invoice","id":1}` {
		t.Errorf("template override: %d %s", resp.StatusCode, b)
	}

	// Other methods on the same path keep the default.
	resp, _ = send(t, s, http.MethodDelete, "/income/invoices/13", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204 default", resp.StatusCode)
	}

	// A later On for the same key replaces the earlier one; a prefixed
	// path is accepted.
	s.On(http.MethodGet, "/api/v1/income/invoices/12", http.StatusOK, []byte(`{"id":12}`))
	if _, b = send(t, s, http.MethodGet, "/income/invoices/12", ""); string(b) != `{"id":12}` {
		t.Errorf("replaced override: %s", b)
	}
}

func TestOverrideStillValidates(t *testing.T) {
	s := mockapi.New(t, mockapi.WithoutAutoFail())
	var called atomic.Bool
	s.OnFunc(http.MethodPost, "/income/invoices", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
	})
	s.On(http.MethodPost, "/projects", http.StatusCreated, []byte(`{}`))
	r1, _ := send(t, s, http.MethodPost, "/income/invoices", `{"bogus":1}`)
	r2, _ := send(t, s, http.MethodPost, "/projects", `{"bogus":1}`)
	if called.Load() || r1.StatusCode != http.StatusInternalServerError || r2.StatusCode != http.StatusInternalServerError {
		t.Errorf("override ran for an invalid request (called=%v, %d, %d)", called.Load(), r1.StatusCode, r2.StatusCode)
	}
	if n := len(s.Violations()); n != 2 {
		t.Errorf("violations = %d, want 2", n)
	}
}

func TestOnFunc(t *testing.T) {
	s := mockapi.New(t)
	s.OnFunc(http.MethodPut, "/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Remaining-Quota", "1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(b)
	})
	resp, b := send(t, s, http.MethodPut, "/projects/3", `{"name":"Neu"}`)
	if resp.StatusCode != http.StatusAccepted || string(b) != `{"name":"Neu"}` {
		t.Errorf("OnFunc: %d %s (handler must see the full body)", resp.StatusCode, b)
	}
	if resp.Header.Get("X-Remaining-Quota") != "1" || resp.Header.Get("X-Consumed-Quota") != "1000" {
		t.Errorf("quota headers: %v", resp.Header)
	}
}

func TestDefaults(t *testing.T) {
	s := mockapi.New(t)

	resp, b := send(t, s, http.MethodGet, "/income/invoices/5/pdf", "", "Accept", "application/pdf")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(b), []byte("%%EOF")) {
		t.Errorf("pdf body is not a PDF: %q", b)
	}

	resp, b = send(t, s, http.MethodPost, "/contact/companies", `{"name":"ACME GmbH","contact_type":"customer"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("create status = %d, want 201", resp.StatusCode)
	}
	if !reflect.DeepEqual(decode(t, b), rawExample(t, "/contact/companies", "post", "201")) {
		t.Errorf("create body is not the 201 example: %s", b)
	}

	resp, b = send(t, s, http.MethodDelete, "/projects/4", "")
	if resp.StatusCode != http.StatusNoContent || len(b) != 0 {
		t.Errorf("delete: %d %q, want 204 empty", resp.StatusCode, b)
	}

	if resp.Header.Get("X-Remaining-Quota") != "9000" || resp.Header.Get("X-Consumed-Quota") != "1000" {
		t.Errorf("quota headers missing: %v", resp.Header)
	}
}

func TestUnknownPathIs404(t *testing.T) {
	s := mockapi.New(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/nope"},
		{http.MethodGet, "/income/invoices/1/nope"},
		{http.MethodPatch, "/income/invoices/1"},
	} {
		resp, b := send(t, s, tc.method, tc.path, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404", tc.method, tc.path, resp.StatusCode)
			continue
		}
		m := assertErrorBody(t, b, "not_found")
		if m["message"] != "Nicht gefunden" {
			t.Errorf("message = %v", m["message"])
		}
		if resp.Header.Get("X-Remaining-Quota") != "9000" {
			t.Errorf("quota header missing on 404")
		}
	}
	if v := s.Violations(); len(v) != 0 {
		t.Errorf("404 recorded violations: %v", v)
	}
}

func TestRequestsCaptured(t *testing.T) {
	s := mockapi.New(t)
	send(t, s, http.MethodPost, "/projects?", `{"name":"P1","customer":{"id":1}}`, "X-Test", "eins")
	send(t, s, http.MethodGet, "/tracker/time_entries?billing_state=billed&page=1", "")

	reqs := s.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	if reqs[0].Method != http.MethodPost || reqs[0].Path != "/projects" || string(reqs[0].Body) != `{"name":"P1","customer":{"id":1}}` {
		t.Errorf("first request: %+v", reqs[0])
	}
	if reqs[0].Header.Get("X-Test") != "eins" || reqs[0].Header.Get("Authorization") != "Bearer "+testToken {
		t.Errorf("headers not captured: %v", reqs[0].Header)
	}
	if reqs[1].Path != "/tracker/time_entries" || reqs[1].Query.Get("billing_state") != "billed" || reqs[1].Query.Get("page") != "1" {
		t.Errorf("second request: %+v", reqs[1])
	}

	// Requests returns copies.
	reqs[0].Body[0] = 'X'
	reqs[0].Header.Set("X-Test", "zwei")
	reqs[1].Query.Set("page", "9")
	again := s.Requests()
	if again[0].Body[0] != '{' || again[0].Header.Get("X-Test") != "eins" || again[1].Query.Get("page") != "1" {
		t.Error("Requests() exposes internal state")
	}
}

func TestConcurrentRequests(t *testing.T) {
	s := mockapi.New(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			send(t, s, http.MethodGet, "/info", "")
			_ = s.Requests()
			_ = s.Violations()
		})
	}
	wg.Wait()
	if n := len(s.Requests()); n != 8 {
		t.Errorf("requests = %d, want 8", n)
	}
}

func TestOnOddHeaderPairsPanics(t *testing.T) {
	s := mockapi.New(t)
	defer func() {
		if recover() == nil {
			t.Error("On with an odd header list did not panic")
		}
	}()
	s.On(http.MethodGet, "/info", http.StatusOK, nil, "X-Only-Key") //nolint:staticcheck // SA5012: the odd count is the point of this test
}
