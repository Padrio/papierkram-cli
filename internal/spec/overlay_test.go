package spec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/spec/patch"
)

const (
	snapshotPath   = "../../api/openapi.json"
	overlayPath    = "../../api/overlay.yaml"
	generatedPath  = "openapi.gen.json"
	snapshotSHA256 = "da826bc0bd364c1749ae088d42287b818b99e8c536ca5ced61193f2581e9ccea"
	operationCount = 89
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	if path == generatedPath {
		return Raw() // what the binary embeds
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func loadDoc(t *testing.T, path string) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(readFile(t, path))
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return doc
}

// rawJSON decodes a spec file generically so tests can inspect keys that
// kin-openapi does not model (patternProperties) or normalizes away.
func rawJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(readFile(t, path)))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return v
}

type operation struct {
	method, path string
	pathItem     *openapi3.PathItem
	op           *openapi3.Operation
}

func operations(doc *openapi3.T) []operation {
	var ops []operation
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			ops = append(ops, operation{method: method, path: path, pathItem: item, op: op})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].path != ops[j].path {
			return ops[i].path < ops[j].path
		}
		return ops[i].method < ops[j].method
	})
	return ops
}

func opByID(t *testing.T, doc *openapi3.T, id string) operation {
	t.Helper()
	for _, o := range operations(doc) {
		if o.op.OperationID == id {
			return o
		}
	}
	t.Fatalf("operation %s not found", id)
	return operation{}
}

func requestSchema(t *testing.T, doc *openapi3.T, id string) *openapi3.Schema {
	t.Helper()
	op := opByID(t, doc, id).op
	if op.RequestBody == nil || op.RequestBody.Value.Content["application/json"] == nil {
		t.Fatalf("%s has no application/json request body", id)
	}
	return op.RequestBody.Value.Content["application/json"].Schema.Value
}

func property(t *testing.T, s *openapi3.Schema, names ...string) *openapi3.Schema {
	t.Helper()
	for _, n := range names {
		if n == "[]" {
			s = s.Items.Value
			continue
		}
		ref, ok := s.Properties[n]
		if !ok {
			t.Fatalf("property %s not found (path %v)", n, names)
		}
		s = ref.Value
	}
	return s
}

func TestSnapshotUnchanged(t *testing.T) {
	sum := sha256.Sum256(readFile(t, snapshotPath))
	if got := hex.EncodeToString(sum[:]); got != snapshotSHA256 {
		t.Fatalf("api/openapi.json changed: sha256 %s, want %s (never edit the snapshot; fix the overlay)", got, snapshotSHA256)
	}
}

func TestGeneratedSpecUpToDate(t *testing.T) {
	got, err := patch.Apply(readFile(t, snapshotPath), readFile(t, overlayPath))
	if err != nil {
		t.Fatalf("apply overlay: %v", err)
	}
	if !bytes.Equal(got, Raw()) {
		t.Fatal("internal/spec/openapi.gen.json is stale; run `go generate ./internal/spec/`")
	}
}

func TestPatchedSpecLoadsAndValidates(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("patched spec does not validate: %v", err)
	}
}

func TestOperationIDsComplete(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	ops := operations(doc)
	if len(ops) != operationCount {
		t.Fatalf("got %d operations, want %d", len(ops), operationCount)
	}

	idPattern := regexp.MustCompile(`^[A-Z][A-Za-z]+$`)
	seen := map[string]string{}
	for _, o := range ops {
		id := o.op.OperationID
		where := o.method + " " + o.path
		if !idPattern.MatchString(id) {
			t.Errorf("%s: operationId %q does not match %s", where, id, idPattern)
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q used by %s and %s", id, prev, where)
		}
		seen[id] = where
		if want := expectedVerb(o.method, o.path); !strings.HasPrefix(id, want) {
			t.Errorf("%s: operationId %q should start with %q", where, id, want)
		}
	}
	if len(seen) != operationCount {
		t.Errorf("got %d unique operationIds, want %d", len(seen), operationCount)
	}

	// Spot checks, including every name spelled out in spec 5.4 and the plan.
	named := map[string]string{
		"GET /banking/bank_connections":                         "ListBankConnections",
		"GET /contact/companies":                                "ListCompanies",
		"GET /contact/companies/{id}":                           "GetCompany",
		"GET /tracker/time_entries":                             "ListTimeEntries",
		"GET /info":                                             "GetInfo",
		"GET /income/invoices":                                  "ListInvoices",
		"POST /income/invoices/{id}/deliver":                    "DeliverInvoice",
		"POST /income/estimates/{id}/deliver":                   "DeliverEstimate",
		"GET /income/invoices/{id}/pdf":                         "GetInvoicePdf",
		"GET /income/estimates/{id}/pdf":                        "GetEstimatePdf",
		"GET /expense/vouchers/{id}/pdf":                        "GetVoucherPdf",
		"POST /expense/vouchers/{voucher_id}/documents":         "UploadVoucherDocument",
		"DELETE /expense/vouchers/{voucher_id}/documents/{id}":  "DeleteVoucherDocument",
		"POST /expense/vouchers/{id}/cancel_with_reverse_entry": "CancelVoucherWithReverseEntry",
		"POST /expense/vouchers/{id}/pay":                       "PayVoucher",
		"POST /expense/vouchers/{id}/cancel":                    "CancelVoucher",
		"POST /contact/companies/{company_id}/persons":          "CreatePerson",
		"PUT /contact/companies/{company_id}/persons/{id}":      "UpdatePerson",
		"POST /settings/custom_attributes/{id}/unarchive":       "UnarchiveCustomAttribute",
		"GET /income/payment_terms/{id}":                        "GetPaymentTerm",
		"GET /users":                                            "ListUsers",
		"DELETE /tracker/time_entries/{id}":                     "DeleteTimeEntry",
		"POST /income/propositions/{id}/archive":                "ArchiveProposition",
		"GET /banking/transactions/{id}":                        "GetTransaction",
		"GET /contact/companies/{company_id}/persons":           "ListPersons",
		"POST /tracker/tasks":                                   "CreateTask",
		"PUT /projects/{id}":                                    "UpdateProject",
		"GET /banking/bank_connections/{id}":                    "GetBankConnection",
		"POST /expense/vouchers":                                "CreateVoucher",
		"GET /settings/custom_attributes":                       "ListCustomAttributes",
	}
	for where, want := range named {
		method, path, _ := strings.Cut(where, " ")
		item := doc.Paths.Value(path)
		if item == nil || item.GetOperation(method) == nil {
			t.Errorf("%s: operation missing", where)
			continue
		}
		if got := item.GetOperation(method).OperationID; got != want {
			t.Errorf("%s: operationId %q, want %q", where, got, want)
		}
	}
}

// expectedVerb derives the <Verb> part of <Verb><Resource>[<Action>] from the
// method and the shape of the path.
func expectedVerb(method, path string) string {
	last := path[strings.LastIndex(path, "/")+1:]
	switch {
	case method == "GET" && strings.HasSuffix(last, "}"), method == "GET" && last == "pdf", path == "/info":
		return "Get"
	case method == "GET":
		return "List"
	case method == "PUT":
		return "Update"
	case method == "DELETE":
		return "Delete"
	case last == "archive":
		return "Archive"
	case last == "unarchive":
		return "Unarchive"
	case last == "cancel", last == "cancel_with_reverse_entry":
		return "Cancel"
	case last == "pay":
		return "Pay"
	case last == "deliver":
		return "Deliver"
	case last == "documents":
		return "Upload"
	default:
		return "Create"
	}
}

func TestPDFResponsesBinary(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	for _, id := range []string{"GetInvoicePdf", "GetEstimatePdf", "GetVoucherPdf"} {
		resp := opByID(t, doc, id).op.Responses.Status(200)
		if resp == nil {
			t.Errorf("%s: no 200 response", id)
			continue
		}
		content := resp.Value.Content
		if len(content) != 1 || content["application/pdf"] == nil {
			t.Errorf("%s: content types %v, want exactly application/pdf", id, mapKeys(content))
			continue
		}
		s := content["application/pdf"].Schema.Value
		if !s.Type.Is("string") || s.Format != "binary" || s.Items != nil || len(s.AllOf) != 0 {
			t.Errorf("%s: schema type=%v format=%q, want string/binary", id, s.Type, s.Format)
		}
	}

	// No response anywhere may keep the literal `["application/pdf"]` key.
	for _, o := range operations(doc) {
		for code, r := range o.op.Responses.Map() {
			for ct := range r.Value.Content {
				if strings.ContainsAny(ct, `[]"`) {
					t.Errorf("%s %s %s: malformed content type %q", o.method, o.path, code, ct)
				}
			}
		}
	}
}

func TestDeliverResponseHasNoContent(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	for _, id := range []string{"DeliverInvoice", "DeliverEstimate"} {
		responses := opByID(t, doc, id).op.Responses
		for _, code := range []int{200, 201} {
			r := responses.Status(code)
			if r == nil {
				t.Errorf("%s: no %d response", id, code)
				continue
			}
			if len(r.Value.Content) != 0 {
				t.Errorf("%s: %d response has content %v, want none", id, code, mapKeys(r.Value.Content))
			}
		}
		for code, r := range responses.Map() {
			if strings.HasPrefix(code, "2") && len(r.Value.Content) != 0 {
				t.Errorf("%s: %s response has content, want none", id, code)
			}
		}
	}
}

func TestOrderDirectionScalar(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	n := 0
	for _, o := range operations(doc) {
		p := o.op.Parameters.GetByInAndName("query", "order_direction")
		if p == nil {
			continue
		}
		n++
		s := p.Schema.Value
		if !s.Type.Is("string") || s.Items != nil || !reflect.DeepEqual(s.Enum, []any{"asc", "desc"}) {
			t.Errorf("%s %s: order_direction type=%v enum=%v items=%v, want string [asc desc]", o.method, o.path, s.Type, s.Enum, s.Items != nil)
		}
	}
	if n != 14 {
		t.Errorf("found order_direction on %d operations, want 14", n)
	}
}

func TestNoNullInEnums(t *testing.T) {
	enums := 0
	walk(rawJSON(t, generatedPath), "$", func(path, key string, v any) {
		if key != "enum" {
			return
		}
		values, ok := v.([]any)
		if !ok {
			t.Errorf("%s: enum is %T, want array", path, v)
			return
		}
		enums++
		seen := map[string]bool{}
		for _, e := range values {
			if e == nil {
				t.Errorf("%s: enum contains null", path)
				continue
			}
			k := toKey(e)
			if seen[k] {
				t.Errorf("%s: duplicate enum value %v", path, e)
			}
			seen[k] = true
		}
	})
	if enums == 0 {
		t.Fatal("no enums found; walker broken?")
	}
}

func TestCategoryEnumHas94UniqueValues(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	orig := loadDoc(t, snapshotPath)
	origEnum := property(t, orig.Paths.Value("/expense/vouchers").Post.RequestBody.Value.Content["application/json"].Schema.Value,
		"line_items", "[]", "category").Enum
	var want []string
	for _, v := range origEnum {
		if v != nil && !slices.Contains(want, v.(string)) {
			want = append(want, v.(string))
		}
	}

	for _, id := range []string{"CreateVoucher", "UpdateVoucher"} {
		cat := property(t, requestSchema(t, doc, id), "line_items", "[]", "category")
		got := make([]string, 0, len(cat.Enum))
		for _, v := range cat.Enum {
			got = append(got, v.(string))
		}
		if len(got) != 94 {
			t.Errorf("%s: category has %d values, want 94", id, len(got))
		}
		// Same values in the same order as the snapshot, minus duplicates and null.
		if !slices.Equal(got, want) {
			t.Errorf("%s: category enum differs from the deduplicated snapshot enum", id)
		}
		if !cat.Nullable {
			t.Errorf("%s: category lost nullable", id)
		}
	}

	reason := requestSchema(t, doc, "PayVoucher").Properties["difference_reason"].Value
	if want := []any{"sonstige", "mahnung", "teilzahlung", "skonto", "schmaelerung"}; !reflect.DeepEqual(reason.Enum, want) {
		t.Errorf("difference_reason enum %v, want %v", reason.Enum, want)
	}
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

func TestPathParamsIntegerAndDeclared(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	for _, o := range operations(doc) {
		params := append(openapi3.Parameters{}, o.pathItem.Parameters...)
		params = append(params, o.op.Parameters...)
		names := pathParam.FindAllStringSubmatch(o.path, -1)
		for _, m := range names {
			p := params.GetByInAndName("path", m[1])
			if p == nil {
				t.Errorf("%s %s: path parameter %s not declared", o.method, o.path, m[1])
				continue
			}
			s := p.Schema.Value
			if !p.Required || !s.Type.Is("integer") || s.Format != "int64" {
				t.Errorf("%s %s: %s required=%v type=%v format=%q, want required integer/int64", o.method, o.path, m[1], p.Required, s.Type, s.Format)
			}
		}
		declared := 0
		for _, p := range params {
			if p.Value.In == "path" {
				declared++
			}
		}
		if declared != len(names) {
			t.Errorf("%s %s: %d path parameters declared, template has %d", o.method, o.path, declared, len(names))
		}
	}
}

func TestRequestDateFieldsHaveDateFormat(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	isDate := regexp.MustCompile(`_date$|^deadline$`)
	n := 0
	for _, o := range operations(doc) {
		if o.op.RequestBody == nil || o.op.RequestBody.Value.Content["application/json"] == nil {
			continue
		}
		for name, p := range o.op.RequestBody.Value.Content["application/json"].Schema.Value.Properties {
			if !isDate.MatchString(name) {
				continue
			}
			if name == "supply_date" {
				// Free text in the API (example: "01.05.2020 - 14.05.2020").
				if p.Value.Format != "" {
					t.Errorf("%s: supply_date has format %q, want none", o.op.OperationID, p.Value.Format)
				}
				continue
			}
			n++
			if p.Value.Format != "date" {
				t.Errorf("%s: %s format %q, want date", o.op.OperationID, name, p.Value.Format)
			}
		}
	}
	if n == 0 {
		t.Fatal("no date fields found")
	}
}

func TestBoolStringExtension(t *testing.T) {
	want := []string{
		"CreateTask.complete", "CreateTask.flagged",
		"UpdateTask.complete", "UpdateTask.flagged",
		"CreatePerson.default", "CreatePerson.flagged",
		"UpdatePerson.default", "UpdatePerson.flagged",
		"CreateCompany.people[].default", "CreateCompany.people[].flagged",
	}
	got := extensionPaths(t, "x-cli-bool-string")
	for _, g := range got {
		if g.value != true {
			t.Errorf("%s: x-cli-bool-string=%v, want true", g.path, g.value)
		}
	}
	assertPaths(t, "x-cli-bool-string", got, want)

	doc := loadDoc(t, generatedPath)
	for _, w := range want {
		id, path, _ := strings.Cut(w, ".")
		s := property(t, requestSchema(t, doc, id), strings.Split(strings.ReplaceAll(path, "[]", ".[]"), ".")...)
		if !s.Type.Is("string") {
			t.Errorf("%s: type %v, want string", w, s.Type)
		}
	}
	// relative_costs is a numeric string, not a boolean.
	for _, id := range []string{"CreateTask", "UpdateTask"} {
		if _, ok := property(t, requestSchema(t, doc, id), "relative_costs").Extensions["x-cli-bool-string"]; ok {
			t.Errorf("%s: relative_costs must not carry x-cli-bool-string", id)
		}
	}
}

func TestProvenanceEnumHint(t *testing.T) {
	want := []string{"CreateVoucher.provenance", "UpdateVoucher.provenance"}
	got := extensionPaths(t, "x-cli-enum-hints")
	assertPaths(t, "x-cli-enum-hints", got, want)
	for _, g := range got {
		if !reflect.DeepEqual(g.value, map[string]any{"non_eu": "foreign"}) {
			t.Errorf("%s: x-cli-enum-hints=%v, want map[non_eu:foreign]", g.path, g.value)
		}
	}
}

type extension struct {
	path  string
	value any
}

// extensionPaths finds every occurrence of an x-cli-* key below a request body
// schema and names it "<operationId>.<property path>" (arrays as "[]").
func extensionPaths(t *testing.T, key string) []extension {
	t.Helper()
	doc := loadDoc(t, generatedPath)
	var found []extension
	for _, o := range operations(doc) {
		if o.op.RequestBody == nil {
			continue
		}
		for _, mt := range o.op.RequestBody.Value.Content {
			collectExtension(mt.Schema.Value, key, o.op.OperationID, &found)
		}
	}
	// Also make sure the key appears nowhere else (responses, components).
	total := 0
	walk(rawJSON(t, generatedPath), "$", func(_, k string, _ any) {
		if k == key {
			total++
		}
	})
	if total != len(found) {
		t.Errorf("%s occurs %d times in the spec but %d times in request bodies", key, total, len(found))
	}
	return found
}

func collectExtension(s *openapi3.Schema, key, path string, found *[]extension) {
	if v, ok := s.Extensions[key]; ok {
		*found = append(*found, extension{path: path, value: v})
	}
	for name, p := range s.Properties {
		collectExtension(p.Value, key, path+"."+name, found)
	}
	if s.Items != nil {
		collectExtension(s.Items.Value, key, path+"[]", found)
	}
}

func assertPaths(t *testing.T, key string, got []extension, want []string) {
	t.Helper()
	paths := make([]string, 0, len(got))
	for _, g := range got {
		paths = append(paths, g.path)
	}
	sort.Strings(paths)
	want = slices.Clone(want)
	sort.Strings(want)
	if !slices.Equal(paths, want) {
		t.Errorf("%s occurrences:\n got  %v\nwant %v", key, paths, want)
	}
}

func TestQueryIDParamsInt64(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	n := 0
	for _, o := range operations(doc) {
		for _, p := range o.op.Parameters {
			if p.Value.In != "query" || !strings.HasSuffix(p.Value.Name, "_id") {
				continue
			}
			n++
			if s := p.Value.Schema.Value; !s.Type.Is("integer") || s.Format != "int64" {
				t.Errorf("%s %s: %s type=%v format=%q, want integer/int64", o.method, o.path, p.Value.Name, s.Type, s.Format)
			}
		}
	}
	if n != 14 {
		t.Errorf("found %d *_id query parameters, want 14", n)
	}
}

func TestSupplierRuleExamplesKept(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	cases := map[string]string{
		"CreateVoucher":  "Lieferant muss ein Lieferant sein",
		"CreateEstimate": "",
		"CreateInvoice":  "",
	}
	for id := range cases {
		o := opByID(t, doc, id).op
		for code, r := range o.Responses.Map() {
			if code == "201" {
				for name := range r.Value.Content["application/json"].Examples {
					if strings.Contains(name, "supplier") {
						t.Errorf("%s: error example %q still filed under 201", id, name)
					}
				}
			}
		}
		found := false
		for name, ex := range o.Responses.Status(422).Value.Content["application/json"].Examples {
			if strings.Contains(name, "supplier") {
				found = true
				if m, _ := ex.Value.Value.(map[string]any)["message"].(string); m == "" {
					t.Errorf("%s: example %q has no message", id, name)
				}
			}
		}
		if !found {
			t.Errorf("%s: supplier-rule example missing under 422", id)
		}
	}
	msg := opByID(t, doc, "CreateVoucher").op.Responses.Status(422).Value.Content["application/json"].Examples
	ok := false
	for _, ex := range msg {
		if ex.Value.Value.(map[string]any)["message"] == "Lieferant muss ein Lieferant sein" {
			ok = true
		}
	}
	if !ok {
		t.Error("CreateVoucher: message \"Lieferant muss ein Lieferant sein\" missing under 422")
	}
}

func TestNoPatternProperties(t *testing.T) {
	walk(rawJSON(t, generatedPath), "$", func(path, key string, _ any) {
		if key == "patternProperties" {
			t.Errorf("%s: patternProperties left over (not OpenAPI 3.0)", path)
		}
	})

	doc := loadDoc(t, generatedPath)
	for _, id := range []string{"CreateCompany", "UpdateCompany", "CreateProject", "UpdateProject"} {
		s := property(t, requestSchema(t, doc, id), "custom_attributes")
		if s.Extensions["x-cli-key-pattern"] != "^[0-9]+$" {
			t.Errorf("%s: custom_attributes x-cli-key-pattern=%v", id, s.Extensions["x-cli-key-pattern"])
		}
		ap := s.AdditionalProperties
		if ap.Schema == nil || len(ap.Schema.Value.AnyOf) != 4 {
			t.Errorf("%s: custom_attributes values must be described by additionalProperties (anyOf string/integer/number/boolean)", id)
		}
	}
}

// walk calls fn for every object member below v.
func walk(v any, path string, fn func(path, key string, v any)) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			p := path + "." + k
			fn(p, k, child)
			walk(child, p, fn)
		}
	case []any:
		for _, child := range x {
			walk(child, path+"[]", fn)
		}
	}
}

func toKey(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestUploadFileRequired(t *testing.T) {
	op := opByID(t, loadDoc(t, generatedPath), "UploadVoucherDocument").op
	mt := op.RequestBody.Value.Content["multipart/form-data"]
	if mt == nil {
		t.Fatal("UploadVoucherDocument: no multipart/form-data request body")
	}
	if s := mt.Schema.Value; !slices.Equal(s.Required, []string{"file"}) {
		t.Errorf("UploadVoucherDocument: required = %v, want [file]", s.Required)
	}
}

// Spec R16, 4.4: the start and end times of a time entry are HH:MM; the
// pattern rejects ISO timestamps and single-digit hours before any request,
// and x-cli-pattern-format names the form in help and errors. The end may
// also be 24:00, the end of the day (open point 22, measured E2E run 2).
func TestTimeEntryTimesHHMM(t *testing.T) {
	format := map[string]string{
		"CreateTimeEntry.started_at_time": "HH:MM", "CreateTimeEntry.ended_at_time": "HH:MM oder 24:00",
		"UpdateTimeEntry.started_at_time": "HH:MM", "UpdateTimeEntry.ended_at_time": "HH:MM oder 24:00",
	}
	want := slices.Sorted(maps.Keys(format))
	got := extensionPaths(t, "x-cli-pattern-format")
	assertPaths(t, "x-cli-pattern-format", got, want)
	for _, g := range got {
		if g.value != format[g.path] {
			t.Errorf("%s: x-cli-pattern-format=%v, want %s", g.path, g.value, format[g.path])
		}
	}
	doc := loadDoc(t, generatedPath)
	for _, w := range want {
		id, name, _ := strings.Cut(w, ".")
		s := property(t, requestSchema(t, doc, id), name)
		if s.Pattern == "" {
			t.Errorf("%s: no pattern", w)
			continue
		}
		re := regexp.MustCompile(s.Pattern)
		end := name == "ended_at_time"
		for _, v := range []string{"00:00", "09:00", "10:20", "23:59", "24:00"} {
			if match := v != "24:00" || end; re.MatchString(v) != match {
				t.Errorf("%s: pattern %s matches %q = %v, want %v", w, s.Pattern, v, !match, match)
			}
		}
		for _, bad := range []string{"9:00", "24:01", "24:30", "25:00", "09:60", "09:00:00", "2026-10-25T09:00:00+02:00", "0900", ""} {
			if re.MatchString(bad) {
				t.Errorf("%s: pattern %s accepts %q", w, s.Pattern, bad)
			}
		}
	}
}

// billable_duration in a time entry body is in seconds like duration in
// the responses (examples: 10:00–12:00 = 7200); the description says so
// in the flag help, and that the API ignores the value on create and on
// update (measured E2E 2026-11-04, runs 1 and 2).
func TestTimeEntryBillableDurationInSeconds(t *testing.T) {
	doc := loadDoc(t, generatedPath)
	const want = "Abrechenbare Dauer in Sekunden; wirkungslos: Die API setzt beim Anlegen und Ändern die Dauer aus Beginn und Ende ein (gemessen 2026-11-04)"
	for _, id := range []string{"CreateTimeEntry", "UpdateTimeEntry"} {
		if d := property(t, requestSchema(t, doc, id), "billable_duration").Description; d != want {
			t.Errorf("%s: billable_duration description %q, want %q", id, d, want)
		}
	}
}
