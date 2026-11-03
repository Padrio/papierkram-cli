package spec

import (
	"slices"
	"strings"
	"testing"
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
				want = append(want, int64(10+len(want)))
				s = "1" + string(rune('0'+len(want)-1))
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

func splitPath(p string) []string { return strings.Split(strings.Trim(p, "/"), "/") }
func joinPath(s []string) string  { return strings.Join(s, "/") }

func TestMatchPathCandidatesSorted(t *testing.T) {
	load()
	for key, ts := range byShape {
		for i := 1; i < len(ts); i++ {
			if ts[i-1].op.Path > ts[i].op.Path {
				t.Errorf("%s: candidates not sorted by path: %s before %s", key, ts[i-1].op.Path, ts[i].op.Path)
			}
		}
	}
}
