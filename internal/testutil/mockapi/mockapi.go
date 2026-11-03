// Package mockapi is a spec-validating httptest server for tests (spec 11,
// HTTP-Mock): every request is validated with kin-openapi against the
// embedded patched spec, and answered with the first 2xx example of the
// operation unless a test overrides the answer with On or OnFunc.
//
// A request that does not match the spec (path parameters, query, headers,
// body, a missing "Authorization: Bearer <token>") is a violation: it is
// answered with 500 and an API-style error body, and, unless WithoutAutoFail
// is given, the test fails at cleanup with all violations listed. Requests
// to paths or methods the spec does not know get 404 in the API error format
// and are not violations.
package mockapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/Padrio/papierkram-cli/internal/spec"
)

const apiPrefix = "/api/v1"

// Default quota headers of every response (spec 4.3); On, OnFunc can
// override them.
const (
	defaultRemainingQuota = "9000"
	defaultConsumedQuota  = "1000"
)

// fakePDF is the default body of operations that return application/pdf.
var fakePDF = []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")

// Recorded is one request as received by the mock. Path is relative to
// URL(), i.e. without /api/v1 (e.g. "/income/invoices/12").
type Recorded struct {
	Method, Path string
	Query        url.Values
	Header       http.Header
	Body         []byte
}

// Option configures a Server.
type Option func(*Server)

// WithoutAutoFail disables the t.Errorf at cleanup; the caller checks
// Violations itself (e.g. testscript scenarios).
func WithoutAutoFail() Option { return func(s *Server) { s.autoFail = false } }

// Server is the mock API. It is safe for concurrent use.
type Server struct {
	srv      *httptest.Server
	router   routers.Router
	autoFail bool
	corrID   atomic.Int64

	mu         sync.Mutex
	overrides  map[string]http.HandlerFunc // "METHOD path", path concrete or template
	requests   []Recorded
	violations []string
}

// New starts a mock server that is closed at t's cleanup.
func New(t testing.TB, opts ...Option) *Server {
	t.Helper()
	s := &Server{autoFail: true, overrides: map[string]http.HandlerFunc{}}
	for _, o := range opts {
		o(s)
	}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	// The router needs the server URL; build it on a shallow copy of the
	// shared doc so only the copy's Servers change.
	doc := *spec.Doc()
	doc.Servers = openapi3.Servers{{URL: "http://" + s.srv.Listener.Addr().String() + apiPrefix}}
	r, err := legacy.NewRouter(&doc, openapi3.DisableExamplesValidation())
	if err != nil {
		s.srv.Close()
		t.Fatalf("mockapi: router for the spec cannot be built: %v", err)
	}
	s.router = r
	s.srv.Start()
	t.Cleanup(func() {
		s.srv.Close() // waits for running handlers
		if v := s.Violations(); s.autoFail && len(v) > 0 {
			t.Errorf("mockapi: %d request(s) do not match the spec:\n  %s", len(v), strings.Join(v, "\n  "))
		}
	})
	return s
}

// URL returns the API root of the mock, ".../api/v1" without trailing slash.
func (s *Server) URL() string { return s.srv.URL + apiPrefix }

// On answers method and path with status, body and the headers hdr
// (key, value pairs) instead of the spec example. path is concrete
// ("/income/invoices/12") or a spec template ("/income/invoices/{id}"); a
// concrete override beats a template override. A non-empty body is sent as
// application/json unless hdr sets Content-Type. Requests are still
// validated first.
func (s *Server) On(method, path string, status int, body []byte, hdr ...string) {
	if len(hdr)%2 != 0 {
		panic(fmt.Sprintf("mockapi: On(%s %s): headers must be key, value pairs, got %d values", method, path, len(hdr)))
	}
	body = bytes.Clone(body)
	hdr = slices.Clone(hdr)
	s.OnFunc(method, path, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		if len(body) > 0 {
			h.Set("Content-Type", "application/json")
		}
		for i := 0; i < len(hdr); i += 2 {
			h.Set(hdr[i], hdr[i+1])
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

// OnFunc lets h answer method and path (concrete or template, see On). h
// runs only for requests that passed validation; it sees the full body and
// the default quota headers, which it may overwrite.
func (s *Server) OnFunc(method, path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[overrideKey(method, path)] = h
}

// Requests returns copies of all received requests in arrival order.
func (s *Server) Requests() []Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Recorded, len(s.requests))
	for i, r := range s.requests {
		out[i] = r.clone()
	}
	return out
}

// Violations returns the spec violations recorded so far, each as
// "METHOD /request/uri: cause".
func (s *Server) Violations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.violations)
}

func (r Recorded) clone() Recorded {
	r.Query = cloneValues(r.Query)
	r.Header = r.Header.Clone()
	r.Body = bytes.Clone(r.Body)
	return r
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = slices.Clone(vs)
	}
	return out
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	rel := relPath(r.URL.Path)
	s.mu.Lock()
	s.requests = append(s.requests, Recorded{
		Method: r.Method, Path: rel, Query: r.URL.Query(), Header: r.Header.Clone(), Body: bytes.Clone(body),
	})
	s.mu.Unlock()

	h := w.Header()
	h.Set("X-Remaining-Quota", defaultRemainingQuota)
	h.Set("X-Consumed-Quota", defaultConsumedQuota)

	if err != nil {
		s.violate(w, r, fmt.Errorf("request body unreadable: %w", err))
		return
	}
	if !hasBearer(r.Header.Get("Authorization")) {
		s.violate(w, r, errors.New(`header "Authorization: Bearer <token>" missing or empty`))
		return
	}

	vr := r.Clone(r.Context())
	vr.Body = io.NopCloser(bytes.NewReader(body))
	vr.URL, err = url.Parse(s.srv.URL + r.URL.RequestURI()) // router matches the absolute server URL
	var route *routers.Route
	var pathParams map[string]string
	if err == nil {
		route, pathParams, err = s.router.FindRoute(vr)
	}
	if err != nil {
		s.writeError(w, http.StatusNotFound, "not_found", "Nicht gefunden")
		return
	}
	if err := validate(vr, route, pathParams); err != nil {
		s.violate(w, r, err)
		return
	}

	if h := s.override(r.Method, rel, route.Path); h != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		h(w, r)
		return
	}
	writeDefault(w, route)
}

// validate checks the request against the operation: kin-openapi covers
// path, query, headers and body; unknown query parameters are checked here,
// because kin-openapi ignores them.
func validate(r *http.Request, route *routers.Route, pathParams map[string]string) error {
	opts := &openapi3filter.Options{
		AuthenticationFunc:                openapi3filter.NoopAuthenticationFunc, // bearer checked in serve
		RejectWhenRequestBodyNotSpecified: true,
		SkipSettingDefaults:               true,
	}
	opts.WithCustomSchemaErrorFunc(schemaErrorMessage)
	err := openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{
		Request: r, PathParams: pathParams, Route: route, Options: opts,
	})
	if err != nil {
		return err
	}
	declared := map[string]bool{}
	for _, ps := range []openapi3.Parameters{route.PathItem.Parameters, route.Operation.Parameters} {
		for _, p := range ps {
			if p.Value != nil && p.Value.In == openapi3.ParameterInQuery {
				declared[p.Value.Name] = true
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(r.URL.Query())) {
		if !declared[name] {
			return fmt.Errorf("query parameter %q is not defined for %s", name, route.Operation.OperationID)
		}
	}
	return nil
}

// schemaErrorMessage keeps schema errors to one line (kin-openapi appends
// the whole schema and value otherwise).
func schemaErrorMessage(err *openapi3.SchemaError) string {
	reason := err.Reason
	if reason == "" {
		reason = fmt.Sprintf("doesn't match schema %q", err.SchemaField)
	}
	if p := err.JSONPointer(); len(p) > 0 {
		return fmt.Sprintf("at %q: %s", "/"+strings.Join(p, "/"), reason)
	}
	return reason
}

func hasBearer(v string) bool {
	token, ok := strings.CutPrefix(v, "Bearer ")
	return ok && strings.TrimSpace(token) != ""
}

func (s *Server) violate(w http.ResponseWriter, r *http.Request, cause error) {
	msg := fmt.Sprintf("%s %s: %v", r.Method, r.URL.RequestURI(), cause)
	s.mu.Lock()
	s.violations = append(s.violations, msg)
	s.mu.Unlock()
	s.writeError(w, http.StatusInternalServerError, "mock_spec_violation",
		"Mock: Anfrage entspricht nicht der API-Spec: "+msg)
}

// writeError writes the API error format (spec 4.1).
func (s *Server) writeError(w http.ResponseWriter, status int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"type":           "error",
		"error_type":     errorType,
		"message":        message,
		"correlation_id": fmt.Sprintf("mock-%d", s.corrID.Add(1)),
	})
}

func (s *Server) override(method, concrete, template string) http.HandlerFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h := s.overrides[overrideKey(method, concrete)]; h != nil {
		return h
	}
	return s.overrides[overrideKey(method, template)]
}

func overrideKey(method, path string) string {
	return strings.ToUpper(method) + " " + relPath(path)
}

// relPath strips a query, the /api/v1 prefix and a trailing slash.
func relPath(p string) string {
	p, _, _ = strings.Cut(p, "?")
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok && (rest == "" || rest[0] == '/') {
		p = rest
	}
	return "/" + strings.Trim(p, "/")
}

// cannedResponse is the default answer of an operation.
type cannedResponse struct {
	status      int
	contentType string
	body        []byte
}

var (
	defaultsOnce sync.Once
	defaults     map[string]cannedResponse // by operationId
)

func writeDefault(w http.ResponseWriter, route *routers.Route) {
	defaultsOnce.Do(loadDefaults)
	c := defaults[route.Operation.OperationID]
	if c.contentType != "" {
		w.Header().Set("Content-Type", c.contentType)
	}
	w.WriteHeader(c.status)
	_, _ = w.Write(c.body)
}

// loadDefaults derives the default response of every operation from its
// first 2xx response. Example bodies are taken verbatim from the raw spec
// so decimals like 1190.00 and large IDs keep their exact spelling.
func loadDefaults() {
	// Path items also hold "parameters" (an array), so operations are
	// decoded one by one.
	var raw struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(spec.Raw(), &raw); err != nil {
		panic(fmt.Sprintf("mockapi: spec does not parse: %v", err))
	}
	type rawOperation struct {
		Responses map[string]struct {
			Content map[string]rawMediaType `json:"content"`
		} `json:"responses"`
	}
	defaults = map[string]cannedResponse{}
	for _, o := range spec.Operations() {
		var code string
		for _, c := range slices.Sorted(maps.Keys(o.Op.Responses.Map())) {
			if len(c) == 3 && c[0] == '2' {
				code = c
				break
			}
		}
		if code == "" {
			defaults[o.ID] = cannedResponse{status: http.StatusOK, contentType: "application/json", body: []byte("{}")}
			continue
		}
		c := cannedResponse{}
		_, _ = fmt.Sscan(code, &c.status)
		resp := o.Op.Responses.Value(code).Value
		switch {
		case c.status == http.StatusNoContent || resp == nil || len(resp.Content) == 0:
			// no body
		case resp.Content["application/pdf"] != nil:
			c.contentType, c.body = "application/pdf", fakePDF
		case resp.Content["application/json"] != nil:
			var ro rawOperation
			if err := json.Unmarshal(raw.Paths[o.Path][strings.ToLower(o.Method)], &ro); err != nil {
				panic(fmt.Sprintf("mockapi: operation %s does not parse: %v", o.ID, err))
			}
			body := jsonExample(ro.Responses[code].Content["application/json"], resp.Content["application/json"].Schema)
			c.contentType, c.body = "application/json", body
		}
		defaults[o.ID] = c
	}
}

// rawMediaType is a response media type of the raw spec.
type rawMediaType struct {
	Examples map[string]struct {
		Value json.RawMessage `json:"value"`
	} `json:"examples"`
	Example json.RawMessage `json:"example"`
}

// jsonExample picks the example body: the first of examples (by key), else
// example, else the schema example, else "{}".
func jsonExample(mt rawMediaType, schema *openapi3.SchemaRef) []byte {
	var ex json.RawMessage
	for _, k := range slices.Sorted(maps.Keys(mt.Examples)) {
		if v := mt.Examples[k].Value; len(v) > 0 && string(v) != "null" {
			ex = v
			break
		}
	}
	if ex == nil && len(mt.Example) > 0 && string(mt.Example) != "null" {
		ex = mt.Example
	}
	if ex == nil && schema != nil && schema.Value != nil && schema.Value.Example != nil {
		ex, _ = json.Marshal(schema.Value.Example)
	}
	var buf bytes.Buffer
	if ex == nil || json.Compact(&buf, ex) != nil {
		return []byte("{}")
	}
	return buf.Bytes()
}
