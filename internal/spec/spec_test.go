package spec

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestEmbeddedSpecLoads(t *testing.T) {
	if len(Raw()) == 0 {
		t.Fatal("Raw() is empty")
	}
	d := Doc()
	if d == nil || d != Doc() {
		t.Fatal("Doc() must return one parsed document")
	}
}

// loadPanic returns the value load panics with, nil when it returns.
func loadPanic() (v any) {
	defer func() { v = recover() }()
	load()
	return nil
}

// useSpec makes the package index data instead of the embedded spec at
// the next call; the embedded spec is indexed again after the test.
func useSpec(t *testing.T, data []byte) {
	t.Helper()
	load()
	orig := raw
	t.Cleanup(func() {
		raw, docOnce, ops, loadErr = orig, sync.Once{}, nil, ""
		load()
	})
	raw, docOnce, ops, loadErr = data, sync.Once{}, nil, ""
}

// L59: a spec that does not load panics at the first call and with the
// same message at every later one; a sync.Once alone would let later
// calls go on with nil (or stale) indexes.
func TestLoadPanicRepeats(t *testing.T) {
	useSpec(t, []byte("{kein JSON"))
	first := loadPanic()
	msg, ok := first.(string)
	if !ok || !strings.HasPrefix(msg, "spec: embedded openapi.gen.json does not load") {
		t.Fatalf("first call: panic %v, want the load error", first)
	}
	for i := range 3 {
		if got := loadPanic(); got != first {
			t.Errorf("call %d: panic %v, want %q", i+2, got, msg)
		}
	}
	for name, call := range map[string]func(){
		"Doc":           func() { Doc() },
		"Operations":    func() { Operations() },
		"OperationByID": func() { OperationByID("GetInvoice") },
		"MatchPath":     func() { MatchPath("GET", "/income/invoices/1") },
	} {
		func() {
			defer func() {
				if got := recover(); got != first {
					t.Errorf("%s: panic %v, want %q", name, got, msg)
				}
			}()
			call()
		}()
	}
}

func TestOperationsSorted(t *testing.T) {
	ops := Operations()
	if len(ops) != 89 {
		t.Fatalf("got %d operations, want 89", len(ops))
	}
	for i := 1; i < len(ops); i++ {
		a, b := ops[i-1], ops[i]
		if a.Path > b.Path || (a.Path == b.Path && a.Method >= b.Method) {
			t.Errorf("not sorted: %s %s before %s %s", a.Method, a.Path, b.Method, b.Path)
		}
	}
	for _, o := range ops {
		if o.ID == "" || o.Op == nil || o.Op.OperationID != o.ID || o.Method != strings.ToUpper(o.Method) {
			t.Errorf("bad operation %+v", o)
		}
	}
}

func TestOperationByID(t *testing.T) {
	o, ok := OperationByID("DeliverInvoice")
	if !ok || o.Method != "POST" || o.Path != "/income/invoices/{id}/deliver" {
		t.Fatalf("got %+v, %v", o, ok)
	}
	if _, ok := OperationByID("Nope"); ok {
		t.Error("unknown id found")
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		method, path string
		id           string
		ids          []int64
		ok           bool
	}{
		{"POST", "/income/invoices/0/deliver", "DeliverInvoice", []int64{0}, true},
		{"POST", "/income/invoices/12/deliver", "DeliverInvoice", []int64{12}, true},
		{"post", "/income/invoices/12/deliver/", "DeliverInvoice", []int64{12}, true},
		{"POST", "/api/v1/income/invoices/12/deliver?x=1", "DeliverInvoice", []int64{12}, true},
		{"GET", "/income/invoices", "ListInvoices", nil, true},
		{"GET", "/income/invoices/?page=2", "ListInvoices", nil, true},
		{"GET", "/info", "GetInfo", nil, true},
		{"GET", "/expense/vouchers/5/pdf", "GetVoucherPdf", []int64{5}, true},
		{"DELETE", "/expense/vouchers/5/documents/9", "DeleteVoucherDocument", []int64{5, 9}, true},
		{"PUT", "/contact/companies/3/persons/4", "UpdatePerson", []int64{3, 4}, true},
		{"GET", "/contact/companies/7", "GetCompany", []int64{7}, true},
		{"GET", "/income/invoices/abc/pdf", "", nil, false},
		{"GET", "/income/invoices/-1/pdf", "", nil, false},
		{"GET", "/income/invoices/99999999999999999999/pdf", "", nil, false},
		{"DELETE", "/income/invoices", "", nil, false},
		{"GET", "/api/v1x/info", "", nil, false},
		{"GET", "/", "", nil, false},
	}
	for _, tt := range tests {
		op, ids, ok := MatchPath(tt.method, tt.path)
		if ok != tt.ok || op.ID != tt.id || !slices.Equal(ids, tt.ids) {
			t.Errorf("MatchPath(%s, %s) = %s %v %v, want %s %v %v", tt.method, tt.path, op.ID, ids, ok, tt.id, tt.ids, tt.ok)
		}
	}
}

// Every operation's own template, filled with ids, must match back to itself:
// guards ambiguity between literal and parameter segments.
func TestMatchPathRoundTrip(t *testing.T) {
	for _, o := range Operations() {
		segs := []string{}
		var want []int64
		for _, s := range splitPath(o.Path) {
			if isParam(s) {
				id := int64(10 + len(want))
				want = append(want, id)
				s = strconv.FormatInt(id, 10)
			}
			segs = append(segs, s)
		}
		concrete := "/" + joinPath(segs)
		got, ids, ok := MatchPath(o.Method, concrete)
		if !ok || got.ID != o.ID || !slices.Equal(ids, want) {
			t.Errorf("%s %s -> %s %v %v, want %s %v", o.Method, concrete, got.ID, ids, ok, o.ID, want)
		}
	}
}

func TestRequestSchema(t *testing.T) {
	s := RequestSchema("CreateTask")
	c := s.Properties["complete"].Value
	if !c.Type.Is("string") || c.Extensions["x-cli-bool-string"] != true {
		t.Errorf("complete: type=%v ext=%v", c.Type, c.Extensions)
	}
	if RequestSchema("GetInvoice") != nil {
		t.Error("GetInvoice has no body, want nil")
	}
	if RequestSchema("Nope") != nil {
		t.Error("unknown op, want nil")
	}
	up := RequestSchema("UploadVoucherDocument")
	if up == nil || len(up.Properties) == 0 {
		t.Errorf("UploadVoucherDocument multipart schema missing: %+v", up)
	}
}

func TestResponseSchema(t *testing.T) {
	if s := ResponseSchema("GetInvoice"); s == nil || len(s.AllOf) == 0 && len(s.Properties) == 0 {
		t.Errorf("GetInvoice response schema empty: %+v", s)
	}
	if ResponseSchema("DeliverInvoice") != nil {
		t.Error("DeliverInvoice has no JSON response, want nil")
	}
	if ResponseSchema("GetInvoicePdf") != nil {
		t.Error("PDF is not application/json, want nil")
	}
	if ResponseSchema("Nope") != nil {
		t.Error("unknown op, want nil")
	}
}

// The PDF downloads answer with application/pdf (overlay), JSON operations
// with no file type.
func TestFileMediaType(t *testing.T) {
	for _, op := range []string{"GetInvoicePdf", "GetEstimatePdf", "GetVoucherPdf"} {
		if got := FileMediaType(op); got != "application/pdf" {
			t.Errorf("%s: %q, want application/pdf", op, got)
		}
	}
	for _, op := range []string{"GetInvoice", "DeliverInvoice", "Nope"} {
		if got := FileMediaType(op); got != "" {
			t.Errorf("%s: %q, want empty", op, got)
		}
	}
}

// FileSchema is the schema of the file answer FileMediaType names: the
// binary string of the PDF downloads; nil for JSON operations.
func TestFileSchema(t *testing.T) {
	for _, op := range []string{"GetInvoicePdf", "GetEstimatePdf", "GetVoucherPdf"} {
		s := FileSchema(op)
		if s == nil || !s.Type.Is("string") || s.Format != "binary" {
			t.Errorf("%s: %+v, want binary string", op, s)
		}
	}
	for _, op := range []string{"GetInvoice", "DeliverInvoice", "Nope"} {
		if s := FileSchema(op); s != nil {
			t.Errorf("%s: %+v, want nil", op, s)
		}
	}
}

// JSON content keys in any spelling (parameters, +json) are no file type.
func TestFileMediaTypeSkipsJSONVariants(t *testing.T) {
	resp := func(cts ...string) *openapi3.Response {
		r := openapi3.NewResponse()
		r.Content = openapi3.Content{}
		for _, ct := range cts {
			r.Content[ct] = openapi3.NewMediaType()
		}
		return r
	}
	for _, tc := range []struct {
		resps []*openapi3.Response
		want  string
	}{
		{[]*openapi3.Response{resp("application/json; charset=utf-8")}, ""},
		{[]*openapi3.Response{resp("application/problem+json")}, ""},
		{[]*openapi3.Response{resp("Application/JSON", "application/pdf")}, "application/pdf"},
		{[]*openapi3.Response{resp(), resp("Application/PDF")}, "application/pdf"},
	} {
		if got := fileMediaType(tc.resps); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.resps, got, tc.want)
		}
	}
}

// The entries of GET /contact/companies have contact_type through allOf
// (CompanyFlat → CompanyOnlyReference); the API ignores it as a filter, so
// company list --type filters by this property locally (spec 9.7).
func TestListEntryProperty(t *testing.T) {
	ct := ListEntryProperty("ListCompanies", "contact_type")
	if ct == nil {
		t.Fatal("ListCompanies: contact_type not found in the entries")
	}
	var enum []string
	for _, v := range ct.Enum {
		if s, ok := v.(string); ok {
			enum = append(enum, s)
		}
	}
	if !slices.Equal(enum, []string{"customer", "supplier"}) {
		t.Errorf("contact_type enum = %v", ct.Enum)
	}
	if ListEntryProperty("ListCompanies", "nope") != nil {
		t.Error("unknown property, want nil")
	}
	if ListEntryProperty("GetCompany", "contact_type") != nil {
		t.Error("GetCompany is no list, want nil")
	}
	if ListEntryProperty("Nope", "id") != nil {
		t.Error("unknown op, want nil")
	}
}

func TestQueryParams(t *testing.T) {
	q := QueryParams("ListInvoices")
	for _, name := range []string{"company_id", "order_direction"} {
		if q[name] == nil || q[name].In != "query" {
			t.Errorf("ListInvoices: query param %s missing (have %d)", name, len(q))
		}
	}
	if len(QueryParams("GetInvoice")) != 0 {
		t.Error("GetInvoice has no query params")
	}
	if QueryParams("Nope") != nil {
		t.Error("unknown op, want nil")
	}
}

// L59: the operation's parameter wins over the path item's of the same
// name; path and header parameters and nil entries are left out. The
// embedded spec declares only path parameters on path items, so the rule
// is checked on the merge itself.
func TestQueryParamsOperationWins(t *testing.T) {
	param := func(name, in, desc string) *openapi3.ParameterRef {
		return &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: name, In: in, Description: desc}}
	}
	pathItem := openapi3.Parameters{param("page", "query", "path item"), param("sort", "query", "path item"), param("id", "path", "path item"), nil, {}}
	operation := openapi3.Parameters{param("page", "query", "operation"), param("x-trace", "header", "operation"), param("filter", "query", "operation")}
	got := queryParams(pathItem, operation)
	want := map[string]string{"page": "operation", "sort": "path item", "filter": "operation"}
	if len(got) != len(want) {
		t.Errorf("params = %v, want %v", slices.Sorted(maps.Keys(got)), want)
	}
	for name, from := range want {
		if p := got[name]; p == nil || p.Description != from {
			t.Errorf("%s: %+v, want the one of the %s", name, p, from)
		}
	}
}

func splitPath(p string) []string { return strings.Split(strings.Trim(p, "/"), "/") }
func joinPath(s []string) string  { return strings.Join(s, "/") }

// tieSpec has templates with equal literal counts that fit one concrete
// path: /a/1/2 fits /a/1/{y} and /a/{x}/2 (and /a/{x}/{y} with fewer
// literals), /b/c/c fits /b/{x}/c and /b/c/{y} only loosely.
const tieSpec = `{"openapi":"3.0.3","info":{"title":"t","version":"1"},"paths":{
"/a/{x}/2":{"get":{"operationId":"A2","responses":{"200":{"description":"ok"}}}},
"/a/1/{y}":{"get":{"operationId":"A1","responses":{"200":{"description":"ok"}}}},
"/a/{x}/{y}":{"get":{"operationId":"AXY","responses":{"200":{"description":"ok"}}}},
"/b/{x}/c":{"get":{"operationId":"BXC","responses":{"200":{"description":"ok"}}}},
"/b/c/{y}":{"get":{"operationId":"BCY","responses":{"200":{"description":"ok"}}}}}}`

// L66: on a tie the lexically smallest template wins, whatever the map
// order the candidates were indexed in (reindexed many times), through
// the public MatchPath and MatchPathLoose.
func TestMatchPathTieBreakDeterministic(t *testing.T) {
	useSpec(t, []byte(tieSpec))
	for range 30 {
		docOnce, ops = sync.Once{}, nil
		if op, ids, ok := MatchPath("GET", "/a/1/2"); !ok || op.ID != "A1" || !slices.Equal(ids, []int64{2}) {
			t.Fatalf("MatchPath /a/1/2 = %s %v %v, want A1 [2] (/a/1/{y} < /a/{x}/2)", op.ID, ids, ok)
		}
		if op, ids, ok := MatchPath("GET", "/a/3/4"); !ok || op.ID != "AXY" || !slices.Equal(ids, []int64{3, 4}) {
			t.Fatalf("MatchPath /a/3/4 = %s %v %v, want AXY [3 4]", op.ID, ids, ok)
		}
		if op, canonical, ok := MatchPathLoose("GET", "/b/c/c"); !ok || op.ID != "BCY" || canonical != "/b/c/{y}" {
			t.Fatalf("MatchPathLoose /b/c/c = %s %q %v, want BCY /b/c/{y} (/b/c/{y} < /b/{x}/c)", op.ID, canonical, ok)
		}
	}
}

func TestMatchPathLoose(t *testing.T) {
	tests := []struct {
		method, path  string
		id, canonical string
		ok            bool
	}{
		{"POST", "/income/invoices/12-x/deliver", "DeliverInvoice", "/income/invoices/12/deliver", true},
		{"POST", "/income/invoices/12/Deliver", "DeliverInvoice", "/income/invoices/12/deliver", true},
		{"post", "/Income/INVOICES/7x/deliver", "DeliverInvoice", "/income/invoices/7/deliver", true},
		{"GET", "/income/invoices/abc", "GetInvoice", "/income/invoices/{id}", true},
		{"GET", "/Income/Invoices", "ListInvoices", "/income/invoices", true},
		{"DELETE", "/contact/companies/1a/persons/2b", "DeletePerson", "/contact/companies/1/persons/2", true},
		{"GET", "/x/y", "", "", false},
		{"PATCH", "/income/invoices/12", "", "", false},
		{"POST", "/income/invoices/12/deliver/x", "", "", false},
	}
	for _, tt := range tests {
		op, canonical, ok := MatchPathLoose(tt.method, tt.path)
		if ok != tt.ok || op.ID != tt.id || canonical != tt.canonical {
			t.Errorf("MatchPathLoose(%s %s) = %s %q %v, want %s %q %v", tt.method, tt.path, op.ID, canonical, ok, tt.id, tt.canonical, tt.ok)
		}
	}
}
