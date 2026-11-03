package registry

import (
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// actionClass is the action class of spec 8.1 per action name.
var actionClass = map[string]Class{
	"":                ClassReadOnly, // info
	"list":            ClassReadOnly,
	"describe":        ClassReadOnly,
	"pdf":             ClassReadOnly,
	"create":          ClassCreate,
	"upload":          ClassCreate,
	"update":          ClassUpdate,
	"archive":         ClassArchive,
	"unarchive":       ClassArchive,
	"delete":          ClassDestructive,
	"cancel":          ClassDestructive,
	"reverse":         ClassDestructive,
	"pay":             ClassDestructive,
	"document delete": ClassDestructive,
	"deliver":         ClassSend,
}

var filterKinds = []string{"id", "date", "datetime_from", "datetime_to", "enum"}

func TestRegistryEntriesMatchSpec(t *testing.T) {
	entries := Entries()
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	resources := map[string]bool{}
	for _, r := range Resources() {
		resources[r.Name] = true
	}
	seenOp := map[string]bool{}
	seenCmd := map[string]bool{}
	for _, e := range entries {
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		op, ok := spec.OperationByID(e.OperationID)
		if !ok {
			t.Errorf("%s: operation %q not in spec", name, e.OperationID)
			continue
		}
		if e.Method != op.Method || e.Path != op.Path {
			t.Errorf("%s: %s %s, spec has %s %s", name, e.Method, e.Path, op.Method, op.Path)
		}
		if want := ExpectedCredits(e.Method, e.Path); e.Credits != want {
			t.Errorf("%s: credits %d, want %d", name, e.Credits, want)
		}
		want, ok := actionClass[e.Action]
		if !ok {
			t.Errorf("%s: unknown action %q", name, e.Action)
		} else if e.Class != want {
			t.Errorf("%s: class %q, want %q", name, e.Class, want)
		}
		if seenOp[e.OperationID] {
			t.Errorf("%s: duplicate operation %s", name, e.OperationID)
		}
		seenOp[e.OperationID] = true
		if seenCmd[e.Resource+"\x00"+e.Action] {
			t.Errorf("duplicate command %q", name)
		}
		seenCmd[e.Resource+"\x00"+e.Action] = true
		if !resources[e.Resource] {
			t.Errorf("%s: resource %q not in Resources()", name, e.Resource)
		}
		if e.Short == "" {
			t.Errorf("%s: no short help", name)
		}
		if len(e.Examples) < 2 {
			t.Errorf("%s: %d examples, want at least 2", name, len(e.Examples))
		}
		for _, ex := range e.Examples {
			if !strings.HasPrefix(ex+" ", "papierkram "+name+" ") {
				t.Errorf("%s: example %q does not start with the command", name, ex)
			}
		}
		params := spec.QueryParams(e.OperationID)
		flags := map[string]bool{}
		for _, f := range e.Filters {
			if params[f.Param] == nil {
				t.Errorf("%s: filter %s maps to unknown query parameter %q", name, f.Flag, f.Param)
			}
			if !slices.Contains(filterKinds, f.Kind) {
				t.Errorf("%s: filter %s has unknown kind %q", name, f.Flag, f.Kind)
			}
			if f.Flag == "" || strings.HasPrefix(f.Flag, "-") || f.Help == "" || flags[f.Flag] {
				t.Errorf("%s: bad filter %+v", name, f)
			}
			flags[f.Flag] = true
		}
		if len(e.Filters) > 0 && e.Action != "list" {
			t.Errorf("%s: filters on a non-list command", name)
		}
		if e.Class == ClassReadOnly && e.Action != "pdf" && len(e.Columns.Table) == 0 {
			t.Errorf("%s: no table columns", name)
		}
		checkColumns(t, name, op, e.Columns)
	}
}

// checkColumns verifies that every column path exists in the response:
// in the schema of a list entry or the object, or, for operations without a
// response schema (GET /info), in the spec examples.
func checkColumns(t *testing.T, name string, op spec.Operation, cols output.Columns) {
	t.Helper()
	all := append(slices.Clone(cols.Table), cols.Wide...)
	s := spec.ResponseSchema(op.ID)
	if s != nil && strings.HasSuffix(name, " list") {
		entries := property(s, "entries")
		if entries == nil || entries.Items == nil {
			t.Errorf("%s: list response without entries[]", name)
			return
		}
		s = entries.Items.Value
	}
	for _, c := range all {
		if c.Header == "" || c.Header != strings.ToUpper(c.Header) {
			t.Errorf("%s: column %s needs an upper-case header, got %q", name, c.Path, c.Header)
		}
		if s != nil {
			if !schemaHasPath(s, c.Path) {
				t.Errorf("%s: column path %q not in response schema", name, c.Path)
			}
			continue
		}
		if !examplesHavePath(op, c.Path) {
			t.Errorf("%s: column path %q not in response examples", name, c.Path)
		}
	}
}

// property finds a property of s, also inside allOf/oneOf/anyOf.
func property(s *openapi3.Schema, key string) *openapi3.Schema {
	if s == nil {
		return nil
	}
	if p := s.Properties[key]; p != nil {
		return p.Value
	}
	for _, refs := range []openapi3.SchemaRefs{s.AllOf, s.OneOf, s.AnyOf} {
		for _, r := range refs {
			if p := property(r.Value, key); p != nil {
				return p
			}
		}
	}
	return nil
}

func schemaHasPath(s *openapi3.Schema, path string) bool {
	for seg := range strings.SplitSeq(path, ".") {
		if s = property(s, seg); s == nil {
			return false
		}
	}
	return true
}

func examplesHavePath(op spec.Operation, path string) bool {
	r := op.Op.Responses.Status(200)
	if r == nil || r.Value == nil {
		return false
	}
	mt := r.Value.Content.Get("application/json")
	if mt == nil || len(mt.Examples) == 0 {
		return false
	}
	for _, ex := range mt.Examples {
		v := any(ex.Value.Value)
		for seg := range strings.SplitSeq(path, ".") {
			m, ok := v.(map[string]any)
			if !ok {
				return false
			}
			if v, ok = m[seg]; !ok {
				return false
			}
		}
	}
	return true
}

func TestReadOnlyEntriesRegistered(t *testing.T) {
	// Spec 15 no. 1–4, 50–51, 59, 88–89.
	want := []struct{ resource, action, opID string }{
		{"bank-connection", "list", "ListBankConnections"},
		{"bank-connection", "describe", "GetBankConnection"},
		{"transaction", "list", "ListTransactions"},
		{"transaction", "describe", "GetTransaction"},
		{"payment-term", "list", "ListPaymentTerms"},
		{"payment-term", "describe", "GetPaymentTerm"},
		{"info", "", "GetInfo"},
		{"user", "list", "ListUsers"},
		{"user", "describe", "GetUser"},
	}
	for _, w := range want {
		e, ok := Lookup(w.resource, w.action)
		if !ok || e.OperationID != w.opID {
			t.Errorf("Lookup(%q, %q) = %q, %v; want %s", w.resource, w.action, e.OperationID, ok, w.opID)
		}
		e, ok = ByOperationID(w.opID)
		if !ok || e.Resource != w.resource || e.Action != w.action {
			t.Errorf("ByOperationID(%s) = %q %q, %v", w.opID, e.Resource, e.Action, ok)
		}
	}
	if _, ok := Lookup("invoice", "fly"); ok {
		t.Error("unknown command found")
	}
	if _, ok := ByOperationID("Nope"); ok {
		t.Error("unknown operation found")
	}
}

func TestTransactionBankConnectionFilter(t *testing.T) {
	e, _ := Lookup("transaction", "list")
	if len(e.Filters) != 1 {
		t.Fatalf("filters = %+v, want one", e.Filters)
	}
	f := e.Filters[0]
	if f.Flag != "bank-connection" || f.Param != "bank_connection_id" || f.Kind != "id" || f.Help == "" {
		t.Fatalf("filter = %+v, want --bank-connection → bank_connection_id (id)", f)
	}
}

func TestResources(t *testing.T) {
	want := map[string][]string{
		"company":          {"firma", "kontakt"},
		"person":           {"kontaktperson"},
		"invoice":          {"rechnung"},
		"estimate":         {"angebot"},
		"voucher":          {"beleg", "expense"},
		"proposition":      {"artikel", "leistung"},
		"payment-term":     {"zahlungsbedingung"},
		"project":          {"projekt"},
		"task":             {"aufgabe"},
		"time":             {"zeit", "time-entry"},
		"custom-attribute": {"datenfeld"},
		"bank-connection":  {"bankverbindung"},
		"transaction":      {"umsatz"},
		"user":             {"benutzer"},
		"info":             nil,
	}
	rs := Resources()
	if len(rs) != len(want) {
		t.Fatalf("%d resources, want %d", len(rs), len(want))
	}
	for _, r := range rs {
		aliases, ok := want[r.Name]
		if !ok {
			t.Errorf("unexpected resource %q", r.Name)
			continue
		}
		if !slices.Equal(r.Aliases, aliases) {
			t.Errorf("%s: aliases %v, want %v", r.Name, r.Aliases, aliases)
		}
		if r.Short == "" {
			t.Errorf("%s: no short help", r.Name)
		}
	}
	if err := checkResources(rs); err != nil {
		t.Fatal(err)
	}
}

func TestCheckResourcesRejectsDuplicates(t *testing.T) {
	tests := map[string][]Resource{
		"duplicate name":       {{Name: "a"}, {Name: "a"}},
		"alias twice":          {{Name: "a", Aliases: []string{"x"}}, {Name: "b", Aliases: []string{"x"}}},
		"alias is other name":  {{Name: "a", Aliases: []string{"b"}}, {Name: "b"}},
		"alias is own name":    {{Name: "a", Aliases: []string{"a"}}},
		"alias before a name":  {{Name: "b", Aliases: []string{"a"}}, {Name: "a"}},
		"alias repeated local": {{Name: "a", Aliases: []string{"x", "x"}}},
	}
	for name, rs := range tests {
		if err := checkResources(rs); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestNewIndex(t *testing.T) {
	if _, err := newIndex(resources, table); err != nil {
		t.Fatalf("registry table is invalid: %v", err)
	}
	rs := []Resource{{Name: "a"}, {Name: "b"}}
	tests := map[string][]Entry{
		"duplicate command":   {{Resource: "a", Action: "list", OperationID: "X"}, {Resource: "a", Action: "list", OperationID: "Y"}},
		"duplicate operation": {{Resource: "a", Action: "list", OperationID: "X"}, {Resource: "b", Action: "list", OperationID: "X"}},
		"unknown resource":    {{Resource: "c", Action: "list", OperationID: "X"}},
		"no operation":        {{Resource: "a", Action: "list"}},
	}
	for name, es := range tests {
		if _, err := newIndex(rs, es); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestEntriesAndResourcesAreCopies(t *testing.T) {
	es := Entries()
	es[0].Resource = "changed"
	es[0].Examples[0] = "changed"
	es[0].Columns.Table[0].Path = "changed"
	rs := Resources()
	rs[0].Aliases[0] = "changed"
	e, ok := Lookup(table[0].Resource, table[0].Action)
	if !ok || e.Examples[0] == "changed" || e.Columns.Table[0].Path == "changed" {
		t.Fatal("Entries() shares memory with the registry")
	}
	e.Examples[0] = "changed"
	if again, _ := Lookup(table[0].Resource, table[0].Action); again.Examples[0] == "changed" {
		t.Fatal("Lookup() shares memory with the registry")
	}
	if Resources()[0].Aliases[0] == "changed" {
		t.Fatal("Resources() shares memory with the registry")
	}
	a, b := Entries(), Entries()
	for i := range a {
		if a[i].OperationID != b[i].OperationID {
			t.Fatal("Entries() order is not stable")
		}
	}
}

func TestExpectedCredits(t *testing.T) {
	tests := []struct {
		method, path string
		want         int
	}{
		{"POST", "/income/invoices", 50},
		{"POST", "/income/estimates", 50},
		{"POST", "/expense/vouchers", 50},
		{"post", "/income/invoices", 50},
		{"POST", "/income/invoices/", 50},
		{"POST", "/api/v1/income/invoices?x=1", 50},
		{"POST", "/contact/companies/12/persons", 10},
		{"GET", "/income/invoices", 1},
		{"PUT", "/income/invoices/{id}", 1},
		{"POST", "/income/invoices/{id}/deliver", 1},
		{"POST", "/income/invoices/{id}/cancel", 1},
		{"POST", "/contact/companies", 10},
		{"POST", "/contact/companies/{company_id}/persons", 10},
		{"POST", "/expense/vouchers/{voucher_id}/documents", 10},
		{"DELETE", "/expense/vouchers/{voucher_id}/documents/{id}", 1},
		{"POST", "/income/propositions", 10},
		{"POST", "/projects", 10},
		{"POST", "/settings/custom_attributes", 10},
		{"POST", "/tracker/tasks", 10},
		{"POST", "/tracker/time_entries", 10},
		{"POST", "/contact/companies/{id}/archive", 1},
		{"POST", "/contact/companies/{id}/unarchive", 1},
		{"POST", "/expense/vouchers/{id}/pay", 1},
		{"POST", "/expense/vouchers/{id}/cancel_with_reverse_entry", 1},
		{"GET", "/info", 1},
		{"GET", "/users/{id}", 1},
		{"DELETE", "/projects/{id}", 1},
	}
	for _, tt := range tests {
		if got := ExpectedCredits(tt.method, tt.path); got != tt.want {
			t.Errorf("ExpectedCredits(%s, %s) = %d, want %d", tt.method, tt.path, got, tt.want)
		}
	}
}

// TestExpectedCreditsSpecMatrix checks the rule against all 89 operations:
// spec 15 has exactly three 50-credit and eight 10-credit operations, all
// POST.
func TestExpectedCreditsSpecMatrix(t *testing.T) {
	want := map[int][]string{
		50: {"POST /expense/vouchers", "POST /income/estimates", "POST /income/invoices"},
		10: {
			"POST /contact/companies", "POST /contact/companies/{company_id}/persons",
			"POST /expense/vouchers/{voucher_id}/documents", "POST /income/propositions",
			"POST /projects", "POST /settings/custom_attributes", "POST /tracker/tasks",
			"POST /tracker/time_entries",
		},
	}
	got := map[int][]string{}
	ops := spec.Operations()
	for _, o := range ops {
		c := ExpectedCredits(o.Method, o.Path)
		if c != 1 {
			got[c] = append(got[c], o.Method+" "+o.Path)
		}
	}
	for c := range want {
		slices.Sort(got[c])
		slices.Sort(want[c])
	}
	if len(got) != len(want) || !slices.Equal(got[50], want[50]) || !slices.Equal(got[10], want[10]) {
		t.Fatalf("credits other than 1:\n got %v\nwant %v", got, want)
	}
	if len(ops) != 89 {
		t.Fatalf("%d operations, want 89", len(ops))
	}
}
