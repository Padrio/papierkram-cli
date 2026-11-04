package registry

import (
	"bytes"
	"encoding/json"
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

var filterKinds = []string{"id", "date", "datetime_from", "datetime_to", "enum", "local_enum"}

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
			if f.Kind == "local_enum" {
				checkLocalEnum(t, name, e.OperationID, f, params)
			} else if params[f.Param] == nil {
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

// checkLocalEnum checks a filter the CLI applies itself: the API has no
// such query parameter (else it would be an "enum" filter), and the list
// entries have the property with a value list.
func checkLocalEnum(t *testing.T, name, opID string, f Filter, params map[string]*openapi3.Parameter) {
	t.Helper()
	if params[f.Param] != nil {
		t.Errorf("%s: filter %s is local, but %q is a query parameter of the API", name, f.Flag, f.Param)
	}
	if p := spec.ListEntryProperty(opID, f.Param); p == nil || len(p.Enum) == 0 {
		t.Errorf("%s: filter %s: list entries have no enum property %q", name, f.Flag, f.Param)
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
			for _, p := range []string{c.Path, c.Fallback} {
				if p != "" && !schemaHasPath(s, p) {
					t.Errorf("%s: column path %q not in response schema", name, p)
				}
			}
			continue
		}
		if c.Fallback != "" {
			t.Errorf("%s: column %s: fallback without a response schema", name, c.Path)
		}
		if !examplesHavePath(t, op, c.Path) {
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

// jsonExamples returns the values of the application/json examples of the
// 200 response; an example without a value is reported and skipped.
func jsonExamples(t *testing.T, op spec.Operation) []any {
	t.Helper()
	r := op.Op.Responses.Status(200)
	if r == nil || r.Value == nil {
		return nil
	}
	mt := r.Value.Content.Get("application/json")
	if mt == nil {
		return nil
	}
	var out []any
	for key, ex := range mt.Examples {
		if ex == nil || ex.Value == nil {
			t.Errorf("%s: example %q has no value", op.ID, key)
			continue
		}
		out = append(out, ex.Value.Value)
	}
	return out
}

func examplesHavePath(t *testing.T, op spec.Operation, path string) bool {
	t.Helper()
	examples := jsonExamples(t, op)
	if len(examples) == 0 {
		return false
	}
	for _, v := range examples {
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

// TestInfoTemplatesColumn renders the spec example of GET /info with the
// info columns: the templates are listed as "id name", untruncated.
func TestInfoTemplatesColumn(t *testing.T) {
	op, _ := spec.OperationByID("GetInfo")
	e, _ := Lookup("info", "")
	found := false
	for _, v := range jsonExamples(t, op) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "Invoice type 2") {
			continue
		}
		found = true
		var out, errOut bytes.Buffer
		o := output.Options{Format: output.FormatTable, Out: &out, Err: &errOut}
		if err := output.Render(o, raw, e.Columns); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "RECHNUNGSVORLAGEN  249 Invoice type 1, 250 Invoice type 2\n") {
			t.Errorf("templates not listed:\n%s", out.String())
		}
	}
	if !found {
		t.Fatal("no GET /info example with templates")
	}
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

func TestCompanyPersonEntriesRegistered(t *testing.T) {
	// Spec 15 no. 5–16.
	want := []struct {
		resource, action, opID string
		class                  Class
		credits                int
	}{
		{"company", "list", "ListCompanies", ClassReadOnly, 1},
		{"company", "create", "CreateCompany", ClassCreate, 10},
		{"company", "describe", "GetCompany", ClassReadOnly, 1},
		{"company", "update", "UpdateCompany", ClassUpdate, 1},
		{"company", "delete", "DeleteCompany", ClassDestructive, 1},
		{"company", "archive", "ArchiveCompany", ClassArchive, 1},
		{"company", "unarchive", "UnarchiveCompany", ClassArchive, 1},
		{"person", "list", "ListPersons", ClassReadOnly, 1},
		{"person", "create", "CreatePerson", ClassCreate, 10},
		{"person", "describe", "GetPerson", ClassReadOnly, 1},
		{"person", "update", "UpdatePerson", ClassUpdate, 1},
		{"person", "delete", "DeletePerson", ClassDestructive, 1},
	}
	for _, w := range want {
		e, ok := Lookup(w.resource, w.action)
		if !ok || e.OperationID != w.opID || e.Class != w.class || e.Credits != w.credits || e.Custom {
			t.Errorf("Lookup(%q, %q) = %+v, %v; want %s %s %d credits, generic", w.resource, w.action, e, ok, w.opID, w.class, w.credits)
		}
	}
}

func TestInvoiceEstimateEntriesRegistered(t *testing.T) {
	// Spec 15 no. 30–49. deliver (Task 31) and pdf (Task 32) have their
	// own handlers: registered as Custom, not built generically.
	want := []struct {
		resource, action, opID string
		class                  Class
		credits                int
		custom                 bool
	}{
		{"estimate", "list", "ListEstimates", ClassReadOnly, 1, false},
		{"estimate", "create", "CreateEstimate", ClassCreate, 50, false},
		{"estimate", "describe", "GetEstimate", ClassReadOnly, 1, false},
		{"estimate", "update", "UpdateEstimate", ClassUpdate, 1, false},
		{"estimate", "delete", "DeleteEstimate", ClassDestructive, 1, false},
		{"estimate", "archive", "ArchiveEstimate", ClassArchive, 1, false},
		{"estimate", "unarchive", "UnarchiveEstimate", ClassArchive, 1, false},
		{"estimate", "cancel", "CancelEstimate", ClassDestructive, 1, false},
		{"estimate", "deliver", "DeliverEstimate", ClassSend, 1, true},
		{"estimate", "pdf", "GetEstimatePdf", ClassReadOnly, 1, true},
		{"invoice", "list", "ListInvoices", ClassReadOnly, 1, false},
		{"invoice", "create", "CreateInvoice", ClassCreate, 50, false},
		{"invoice", "describe", "GetInvoice", ClassReadOnly, 1, false},
		{"invoice", "update", "UpdateInvoice", ClassUpdate, 1, false},
		{"invoice", "delete", "DeleteInvoice", ClassDestructive, 1, false},
		{"invoice", "archive", "ArchiveInvoice", ClassArchive, 1, false},
		{"invoice", "unarchive", "UnarchiveInvoice", ClassArchive, 1, false},
		{"invoice", "cancel", "CancelInvoice", ClassDestructive, 1, false},
		{"invoice", "deliver", "DeliverInvoice", ClassSend, 1, true},
		{"invoice", "pdf", "GetInvoicePdf", ClassReadOnly, 1, true},
	}
	for _, w := range want {
		e, ok := Lookup(w.resource, w.action)
		if !ok || e.OperationID != w.opID || e.Class != w.class || e.Credits != w.credits || e.Custom != w.custom {
			t.Errorf("Lookup(%q, %q) = %+v, %v; want %s %s %d credits, custom %v", w.resource, w.action, e, ok, w.opID, w.class, w.credits, w.custom)
		}
	}
}

func TestVoucherEntriesRegistered(t *testing.T) {
	// Spec 15 no. 17–29. upload, document delete (Task 33), pay (Task 35)
	// and pdf (Task 32) have their own handlers: registered as Custom, not
	// built generically.
	want := []struct {
		resource, action, opID string
		class                  Class
		credits                int
		custom                 bool
	}{
		{"voucher", "upload", "UploadVoucherDocument", ClassCreate, 10, true},
		{"voucher", "document delete", "DeleteVoucherDocument", ClassDestructive, 1, true},
		{"voucher", "list", "ListVouchers", ClassReadOnly, 1, false},
		{"voucher", "create", "CreateVoucher", ClassCreate, 50, false},
		{"voucher", "describe", "GetVoucher", ClassReadOnly, 1, false},
		{"voucher", "update", "UpdateVoucher", ClassUpdate, 1, false},
		{"voucher", "delete", "DeleteVoucher", ClassDestructive, 1, false},
		{"voucher", "archive", "ArchiveVoucher", ClassArchive, 1, false},
		{"voucher", "unarchive", "UnarchiveVoucher", ClassArchive, 1, false},
		{"voucher", "cancel", "CancelVoucher", ClassDestructive, 1, false},
		{"voucher", "reverse", "CancelVoucherWithReverseEntry", ClassDestructive, 1, false},
		{"voucher", "pay", "PayVoucher", ClassDestructive, 1, true},
		{"voucher", "pdf", "GetVoucherPdf", ClassReadOnly, 1, true},
	}
	for _, w := range want {
		e, ok := Lookup(w.resource, w.action)
		if !ok || e.OperationID != w.opID || e.Class != w.class || e.Credits != w.credits || e.Custom != w.custom {
			t.Errorf("Lookup(%q, %q) = %+v, %v; want %s %s %d credits, custom %v", w.resource, w.action, e, ok, w.opID, w.class, w.credits, w.custom)
		}
	}
}

func TestPropositionEntriesRegistered(t *testing.T) {
	// Spec 15 no. 52–58, all built generically.
	want := []struct {
		action, opID string
		class        Class
		credits      int
	}{
		{"list", "ListPropositions", ClassReadOnly, 1},
		{"create", "CreateProposition", ClassCreate, 10},
		{"describe", "GetProposition", ClassReadOnly, 1},
		{"update", "UpdateProposition", ClassUpdate, 1},
		{"delete", "DeleteProposition", ClassDestructive, 1},
		{"archive", "ArchiveProposition", ClassArchive, 1},
		{"unarchive", "UnarchiveProposition", ClassArchive, 1},
	}
	for _, w := range want {
		e, ok := Lookup("proposition", w.action)
		if !ok || e.OperationID != w.opID || e.Class != w.class || e.Credits != w.credits || e.Custom || len(e.Filters) != 0 {
			t.Errorf("Lookup(proposition, %q) = %+v, %v; want %s %s %d credits, generic, no filters", w.action, e, ok, w.opID, w.class, w.credits)
		}
	}
}

// Spec 10.3: the default columns of proposition. List entries
// (PropositionFlat) have no vat_rate, so only the object responses show
// it.
func TestPropositionColumns(t *testing.T) {
	for _, action := range []string{"list", "describe", "create", "update", "archive", "unarchive"} {
		want := []string{"id", "article_no", "name", "price", "vat_rate.name", "proposition_type"}
		if action == "list" {
			want = slices.DeleteFunc(want, func(p string) bool { return p == "vat_rate.name" })
		}
		e, _ := Lookup("proposition", action)
		var got []string
		for _, c := range e.Columns.Table {
			got = append(got, c.Path)
		}
		if !slices.Equal(got, want) {
			t.Errorf("proposition %s: table columns %v, want %v", action, got, want)
		}
	}
}

// Spec 10.3 [Builder]: payment-term shows the due period due_time with its
// unit due_time_unit (a free string, "day" in the spec examples), so the
// period is never shown in a wrong unit; wide shows the cash discount
// period with its unit.
func TestPaymentTermColumns(t *testing.T) {
	for _, action := range []string{"list", "describe"} {
		e, _ := Lookup("payment-term", action)
		var got []string
		for _, c := range e.Columns.Table {
			got = append(got, c.Header+" "+c.Path)
		}
		if want := []string{"ID id", "NAME name", "ZIEL due_time", "EINHEIT due_time_unit"}; !slices.Equal(got, want) {
			t.Errorf("payment-term %s: table columns %v, want %v", action, got, want)
		}
		var wide []string
		for _, c := range e.Columns.Wide {
			wide = append(wide, c.Header+" "+c.Path)
		}
		if want := []string{"SKONTO cash_discount", "SKONTOFRIST cash_discount_time", "SKONTO-EINHEIT cash_discount_unit"}; !slices.Equal(wide, want) {
			t.Errorf("payment-term %s: wide columns %v, want %v", action, wide, want)
		}
	}
}

// Spec 4.2, 7.1: voucher list filters by creditor, project and document
// date.
func TestVoucherListFilters(t *testing.T) {
	want := []struct{ flag, param, kind string }{
		{"creditor", "creditor_id", "id"},
		{"project", "project_id", "id"},
		{"from", "document_date_range_start", "date"},
		{"to", "document_date_range_end", "date"},
	}
	e, _ := Lookup("voucher", "list")
	if len(e.Filters) != len(want) {
		t.Fatalf("voucher list: filters = %+v, want %d", e.Filters, len(want))
	}
	for i, w := range want {
		if f := e.Filters[i]; f.Flag != w.flag || f.Param != w.param || f.Kind != w.kind || f.Help == "" {
			t.Errorf("voucher list: filter %d = %+v, want --%s → %s (%s)", i, f, w.flag, w.param, w.kind)
		}
	}
}

// Spec 10.3: the default columns of voucher; wide adds the supplier where
// the response has it (not in list entries).
func TestVoucherColumns(t *testing.T) {
	want := []string{"id", "voucher_no", "name", "state", "document_date", "due_date", "amount"}
	for _, action := range []string{"list", "describe", "create", "update", "archive", "unarchive", "cancel", "reverse"} {
		e, _ := Lookup("voucher", action)
		var got []string
		for _, c := range e.Columns.Table {
			got = append(got, c.Path)
		}
		if !slices.Equal(got, want) {
			t.Errorf("voucher %s: table columns %v, want %v", action, got, want)
		}
		hasCreditor := slices.ContainsFunc(e.Columns.Wide, func(c output.Column) bool { return c.Path == "creditor.name" })
		if hasCreditor == (action == "list") {
			t.Errorf("voucher %s: wide columns %v, creditor.name only outside list", action, e.Columns.Wide)
		}
	}
}

// Spec 4.2, 7.1: invoice and estimate list filter by company, project and
// document date.
func TestInvoiceEstimateListFilters(t *testing.T) {
	want := []struct{ flag, param, kind string }{
		{"company", "company_id", "id"},
		{"project", "project_id", "id"},
		{"from", "document_date_range_start", "date"},
		{"to", "document_date_range_end", "date"},
	}
	for _, resource := range []string{"invoice", "estimate"} {
		e, _ := Lookup(resource, "list")
		if len(e.Filters) != len(want) {
			t.Fatalf("%s list: filters = %+v, want %d", resource, e.Filters, len(want))
		}
		for i, w := range want {
			if f := e.Filters[i]; f.Flag != w.flag || f.Param != w.param || f.Kind != w.kind || f.Help == "" {
				t.Errorf("%s list: filter %d = %+v, want --%s → %s (%s)", resource, i, f, w.flag, w.param, w.kind)
			}
		}
	}
}

// Spec 10.3: the default columns of invoice and estimate.
func TestInvoiceEstimateColumns(t *testing.T) {
	want := map[string][]string{
		"invoice":  {"id", "invoice_no", "name", "state", "document_date", "due_date", "total_gross", "billing.company"},
		"estimate": {"id", "estimate_no", "name", "state", "document_date", "total_gross", "billing.company"},
	}
	for resource, paths := range want {
		for _, action := range []string{"list", "describe"} {
			e, _ := Lookup(resource, action)
			var got []string
			for _, c := range e.Columns.Table {
				got = append(got, c.Path)
			}
			if !slices.Equal(got, paths) {
				t.Errorf("%s %s: table columns %v, want %v", resource, action, got, paths)
			}
		}
	}
}

// Spec 10.3: one column customer_no/supplier_no, the supplier number when
// there is no customer number.
func TestCompanyNumberColumn(t *testing.T) {
	for _, action := range []string{"list", "describe"} {
		e, _ := Lookup("company", action)
		var paths []string
		for _, c := range append(e.Columns.Table, e.Columns.Wide...) {
			if c.Path == "customer_no" || c.Path == "supplier_no" || c.Fallback != "" {
				paths = append(paths, c.Header+" "+c.Path+"|"+c.Fallback)
			}
		}
		if len(paths) != 1 || paths[0] != "NR. customer_no|supplier_no" {
			t.Errorf("company %s: number columns %v, want one NR. customer_no|supplier_no", action, paths)
		}
	}
}

// company list --type: the API ignores contact_type (spec 4.6), so the
// filter is local (spec 9.7).
func TestCompanyListTypeFilter(t *testing.T) {
	e, _ := Lookup("company", "list")
	if len(e.Filters) != 1 {
		t.Fatalf("filters = %+v, want one", e.Filters)
	}
	f := e.Filters[0]
	if f.Flag != "type" || f.Param != "contact_type" || f.Kind != "local_enum" || !strings.Contains(f.Help, "alle Seiten") {
		t.Fatalf("filter = %+v, want --type → contact_type (local_enum), help names loading all pages", f)
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
		if r.Noun == "" {
			t.Errorf("%s: no German noun for confirmations", r.Name)
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
	for _, reserved := range []string{"help", "completion", "context", "auth", "version", "api", "commands", "schema", "local", "gen-docs"} {
		tests["reserved name "+reserved] = []Resource{{Name: reserved}}
		tests["reserved alias "+reserved] = []Resource{{Name: "a", Aliases: []string{reserved}}}
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

// Spec 15 no. 60–80: project, custom-attribute and task, all built
// generically.
func TestProjectTaskCustomAttributeEntriesRegistered(t *testing.T) {
	for resource, op := range map[string]string{"project": "Project", "custom-attribute": "CustomAttribute", "task": "Task"} {
		want := []struct {
			action, opID string
			class        Class
			credits      int
		}{
			{"list", "List" + op + "s", ClassReadOnly, 1},
			{"create", "Create" + op, ClassCreate, 10},
			{"describe", "Get" + op, ClassReadOnly, 1},
			{"update", "Update" + op, ClassUpdate, 1},
			{"delete", "Delete" + op, ClassDestructive, 1},
			{"archive", "Archive" + op, ClassArchive, 1},
			{"unarchive", "Unarchive" + op, ClassArchive, 1},
		}
		for _, w := range want {
			e, ok := Lookup(resource, w.action)
			if !ok || e.OperationID != w.opID || e.Class != w.class || e.Credits != w.credits || e.Custom {
				t.Errorf("Lookup(%s, %q) = %+v, %v; want %s %s %d credits, generic", resource, w.action, e, ok, w.opID, w.class, w.credits)
			}
		}
	}
}

// Spec 4.2, 7.1: project list filters by company, task list by project
// and proposition; custom-attribute list has no filter.
func TestProjectTaskCustomAttributeListFilters(t *testing.T) {
	type filter struct{ flag, param, kind string }
	for resource, want := range map[string][]filter{
		"project":          {{"company", "company_id", "id"}},
		"task":             {{"project", "project_id", "id"}, {"proposition", "proposition_id", "id"}},
		"custom-attribute": nil,
	} {
		e, _ := Lookup(resource, "list")
		if len(e.Filters) != len(want) {
			t.Errorf("%s list: filters = %+v, want %d", resource, e.Filters, len(want))
			continue
		}
		for i, w := range want {
			if f := e.Filters[i]; f.Flag != w.flag || f.Param != w.param || f.Kind != w.kind || f.Help == "" {
				t.Errorf("%s list: filter %d = %+v, want --%s → %s (%s)", resource, i, f, w.flag, w.param, w.kind)
			}
		}
	}
}

// Spec 10.3: the default columns of project, task and custom-attribute.
func TestProjectTaskCustomAttributeColumns(t *testing.T) {
	want := map[string][]string{
		"project":          {"id", "name", "company_id", "record_state", "start_date", "end_date"},
		"task":             {"id", "name", "project_id", "complete", "deadline"},
		"custom-attribute": {"id", "name", "attributable_type", "data_type"},
	}
	for resource, paths := range want {
		for _, action := range []string{"list", "describe", "create", "update", "archive", "unarchive"} {
			e, _ := Lookup(resource, action)
			var got []string
			for _, c := range e.Columns.Table {
				got = append(got, c.Path)
			}
			if !slices.Equal(got, paths) {
				t.Errorf("%s %s: table columns %v, want %v", resource, action, got, paths)
			}
		}
	}
}

// Spec 9.8: project describe without ID, task list and task create take
// the project from the project file; no other command has a preset.
func TestProjectFilePresets(t *testing.T) {
	want := map[string]string{"project describe": "project", "task list": "project", "task create": "project"}
	for _, e := range Entries() {
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		if e.Preset != want[name] {
			t.Errorf("%s: Preset %q, want %q", name, e.Preset, want[name])
		}
	}
}

// A preset is a project file key (spec 9.8) and only exists where the
// builder applies it: the positional ID of describe, a list filter flag of
// that name, or create (whose field flag the CLI tests check).
func TestPresetsAreApplicable(t *testing.T) {
	keys := []string{"company", "project", "task", "user"}
	for _, e := range Entries() {
		if e.Preset == "" {
			continue
		}
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		if !slices.Contains(keys, e.Preset) {
			t.Errorf("%s: Preset %q is no project file key %v", name, e.Preset, keys)
		}
		switch e.Action {
		case "describe":
			if !strings.HasSuffix(e.Path, "{id}") {
				t.Errorf("%s: Preset on describe without positional {id}", name)
			}
		case "list":
			if !slices.ContainsFunc(e.Filters, func(f Filter) bool { return f.Flag == e.Preset && f.Kind == "id" }) {
				t.Errorf("%s: Preset %q without id filter --%s", name, e.Preset, e.Preset)
			}
		case "create":
		default:
			t.Errorf("%s: Preset on action %q", name, e.Action)
		}
	}
}
