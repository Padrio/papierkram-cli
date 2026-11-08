package spec

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

func resourceNames() []string {
	var names []string
	for _, r := range registry.Resources() {
		names = append(names, r.Name)
	}
	return names
}

func mustFields(t *testing.T, opID string) []Field {
	t.Helper()
	req := RequestSchema(opID)
	if req == nil {
		t.Fatalf("no request schema for %s", opID)
	}
	fs, err := Fields(req, resourceNames())
	if err != nil {
		t.Fatalf("Fields(%s): %v", opID, err)
	}
	return fs
}

func fieldByFlag(t *testing.T, fs []Field, flag string) Field {
	t.Helper()
	for _, f := range fs {
		if f.Flag == flag {
			return f
		}
	}
	t.Fatalf("no field with flag --%s", flag)
	return Field{}
}

func flagNames(fs []Field) []string {
	var out []string
	for _, f := range fs {
		if f.Flag != "" {
			out = append(out, f.Flag)
		}
	}
	return out
}

func TestFieldsCreateInvoice(t *testing.T) {
	fs := mustFields(t, "CreateInvoice")
	want := []string{
		"billing-city", "billing-company", "billing-contact-person", "billing-country",
		"billing-department", "billing-email", "billing-street", "billing-ust-idnr", "billing-zip",
		"contact-person", "custom-template", "customer", "customer-project", "description",
		"document-date", "flagged", "gross", "item", "name", "payment-term", "supply-date",
	}
	if got := flagNames(fs); !slices.Equal(got, want) {
		t.Fatalf("flags\n got %v\nwant %v", got, want)
	}

	checks := []struct {
		flag     string
		kind     FieldKind
		path     []string
		required bool
	}{
		{"customer", KindRef, []string{"customer"}, false},
		{"contact-person", KindNestedRef, []string{"customer", "contact_person"}, false},
		{"customer-project", KindNestedRef, []string{"customer", "project"}, false},
		{"payment-term", KindRef, []string{"payment_term"}, true},
		{"custom-template", KindRef, []string{"custom_template"}, false},
		{"billing-company", KindGroup, []string{"billing", "company"}, false},
		{"name", KindScalar, []string{"name"}, true},
		{"item", KindItems, []string{"line_items"}, true},
		{"document-date", KindScalar, []string{"document_date"}, true},
	}
	for _, c := range checks {
		f := fieldByFlag(t, fs, c.flag)
		if f.Kind != c.kind || !slices.Equal(f.Path, c.path) || f.Required != c.required {
			t.Errorf("--%s: got kind=%s path=%v required=%v, want %s %v %v", c.flag, f.Kind, f.Path, f.Required, c.kind, c.path, c.required)
		}
	}
	if f := fieldByFlag(t, fs, "item"); !f.Repeatable {
		t.Error("--item must be repeatable")
	}
	if f := fieldByFlag(t, fs, "flagged"); f.Type != "boolean" || !f.IsBool() || f.BoolString {
		t.Errorf("--flagged: %+v", f)
	}
	if f := fieldByFlag(t, fs, "customer"); f.Type != "integer" {
		t.Errorf("--customer type %q, want integer", f.Type)
	}
	if f := fieldByFlag(t, fs, "document-date"); f.Format != "date" || !strings.Contains(f.Help, "heute") {
		t.Errorf("--document-date: format %q help %q", f.Format, f.Help)
	}
}

func TestFieldsCreateProject(t *testing.T) {
	fs := mustFields(t, "CreateProject")
	want := []string{
		"attr", "budget-money", "budget-time", "budget-time-unit", "budget-type", "color",
		"customer", "default-proposition", "description", "end-date", "flagged", "name",
		"start-date", "team-member",
	}
	if got := flagNames(fs); !slices.Equal(got, want) {
		t.Fatalf("flags\n got %v\nwant %v", got, want)
	}
	tm := fieldByFlag(t, fs, "team-member")
	if tm.Kind != KindRefList || !tm.Repeatable || !slices.Equal(tm.Path, []string{"team_members"}) {
		t.Errorf("--team-member: %+v", tm)
	}
	attr := fieldByFlag(t, fs, "attr")
	if attr.Kind != KindAttrs || !attr.Repeatable || !slices.Equal(attr.Path, []string{"custom_attributes"}) {
		t.Errorf("--attr: %+v", attr)
	}
	if f := fieldByFlag(t, fs, "customer"); !f.Required || f.Kind != KindRef {
		t.Errorf("--customer: %+v", f)
	}
	if f := fieldByFlag(t, fs, "default-proposition"); f.Kind != KindRef || !slices.Equal(f.Path, []string{"default_proposition"}) {
		t.Errorf("--default-proposition: %+v", f)
	}
	color := fieldByFlag(t, fs, "color")
	if !slices.Equal(color.Enum, []string{"blue", "green", "lime", "pink", "purple", "orange"}) {
		t.Errorf("--color enum %v", color.Enum)
	}
	if !strings.HasSuffix(color.Help, "(erlaubt: blue, green, lime, pink, purple, orange)") {
		t.Errorf("--color help %q", color.Help)
	}
	if f := fieldByFlag(t, fs, "budget-money"); f.Type != "string" || f.IsBool() {
		t.Errorf("--budget-money: %+v", f)
	}
}

func TestFieldsObjectListsInputOnly(t *testing.T) {
	for _, tc := range []struct{ op, path string }{
		{"CreateCompany", "people"},
	} {
		fs := mustFields(t, tc.op)
		var found bool
		for _, f := range fs {
			if len(f.Path) == 1 && f.Path[0] == tc.path {
				found = true
				if f.Kind != KindObjectList || f.Flag != "" || !strings.Contains(f.Help, "--input") {
					t.Errorf("%s %s: %+v", tc.op, tc.path, f)
				}
			}
			if strings.HasPrefix(f.Flag, strings.ReplaceAll(tc.path, "_", "-")) {
				t.Errorf("%s: object list %s must not produce flag --%s", tc.op, tc.path, f.Flag)
			}
		}
		if !found {
			t.Errorf("%s: no object_list field for %s", tc.op, tc.path)
		}
	}
}

func TestFieldsTaskAndPersonBoolString(t *testing.T) {
	for _, op := range []string{"CreateTask", "UpdateTask"} {
		fs := mustFields(t, op)
		for _, flag := range []string{"complete", "flagged"} {
			f := fieldByFlag(t, fs, flag)
			if !f.BoolString || !f.IsBool() || f.Type != "string" {
				t.Errorf("%s --%s: %+v", op, flag, f)
			}
		}
		if f := fieldByFlag(t, fs, "relative-costs"); f.BoolString || f.IsBool() {
			t.Errorf("%s --relative-costs must stay a string flag: %+v", op, f)
		}
		for _, flag := range []string{"project", "proposition", "user"} {
			if f := fieldByFlag(t, fs, flag); f.Kind != KindRef {
				t.Errorf("%s --%s: %+v", op, flag, f)
			}
		}
	}
	for _, op := range []string{"CreatePerson", "UpdatePerson"} {
		fs := mustFields(t, op)
		for _, flag := range []string{"default", "flagged"} {
			if f := fieldByFlag(t, fs, flag); !f.BoolString || !f.IsBool() {
				t.Errorf("%s --%s: %+v", op, flag, f)
			}
		}
	}
}

func TestFieldsScalarList(t *testing.T) {
	for _, op := range []string{"CreateVoucher", "UpdateVoucher"} {
		f := fieldByFlag(t, mustFields(t, op), "entertainment-person")
		if f.Kind != KindScalarList || !f.Repeatable || f.Type != "string" ||
			!slices.Equal(f.Path, []string{"entertainment_persons"}) || !strings.HasSuffix(f.Help, "(wiederholbar)") {
			t.Errorf("%s --entertainment-person: %+v", op, f)
		}
	}
}

func TestFieldsBinaryInputOnly(t *testing.T) {
	fs := mustFields(t, "UploadVoucherDocument")
	if len(fs) != 1 {
		t.Fatalf("got %d fields, want 1: %+v", len(fs), fs)
	}
	f := fs[0]
	if f.Kind != KindBinary || f.Flag != "" || !f.Required || f.Format != "binary" || !slices.Equal(f.Path, []string{"file"}) {
		t.Errorf("file: %+v", f)
	}
	if _, err := CoerceFlag(CoerceContext{}, f, "x.pdf"); err == nil {
		t.Error("binary field must not be coerced from a flag")
	}
}

func TestFieldsCreateVoucher(t *testing.T) {
	fs := mustFields(t, "CreateVoucher")
	if f := fieldByFlag(t, fs, "creditor"); f.Kind != KindRef {
		t.Errorf("--creditor: %+v", f)
	}
	p := fieldByFlag(t, fs, "provenance")
	if !p.Required || !slices.Equal(p.Enum, []string{"domestic", "eu", "foreign"}) || p.Schema == nil {
		t.Errorf("--provenance: %+v", p)
	}
	if f := fieldByFlag(t, fs, "item"); !f.Required {
		t.Error("--item must be required for CreateVoucher")
	}
}

var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestFieldsAllOperations derives the fields of every operation with a request
// body and checks that no flag name is duplicated or shadows a global flag.
func TestFieldsAllOperations(t *testing.T) {
	var n int
	for _, o := range Operations() {
		req := RequestSchema(o.ID)
		if req == nil {
			continue
		}
		n++
		fs, err := Fields(req, resourceNames())
		if err != nil {
			t.Errorf("%s: %v", o.ID, err)
			continue
		}
		seen := map[string]bool{}
		for i, f := range fs {
			if i > 0 && fs[i-1].Flag > f.Flag {
				t.Errorf("%s: not sorted: --%s before --%s", o.ID, fs[i-1].Flag, f.Flag)
			}
			if len(f.Path) == 0 || f.Kind == "" || f.Help == "" {
				t.Errorf("%s: incomplete field %+v", o.ID, f)
			}
			if f.Flag == "" {
				if f.Kind != KindObjectList && f.Kind != KindBinary {
					t.Errorf("%s: field %v without flag", o.ID, f.Path)
				}
				continue
			}
			if !kebabRe.MatchString(f.Flag) {
				t.Errorf("%s: flag --%s is not kebab-case", o.ID, f.Flag)
			}
			if seen[f.Flag] {
				t.Errorf("%s: duplicate flag --%s", o.ID, f.Flag)
			}
			seen[f.Flag] = true
			if slices.Contains(ReservedFlags(), f.Flag) && !ownsReserved(f) {
				t.Errorf("%s: flag --%s shadows a global flag", o.ID, f.Flag)
			}
		}
	}
	if n != 24 {
		t.Errorf("checked %d operations with a request body, want 24", n)
	}
}

func ownsReserved(f Field) bool {
	return (f.Kind == KindItems && f.Flag == "item") || (f.Kind == KindAttrs && f.Flag == "attr")
}

func TestFieldsDeterministic(t *testing.T) {
	a := mustFields(t, "CreateInvoice")
	for range 5 {
		if b := mustFields(t, "CreateInvoice"); !reflect.DeepEqual(a, b) {
			t.Fatal("Fields is not deterministic")
		}
	}
}

func TestFieldsNilSchema(t *testing.T) {
	fs, err := Fields(nil, nil)
	if fs != nil || err != nil {
		t.Errorf("got %v, %v", fs, err)
	}
}

func obj(required []string, props map[string]*openapi3.Schema) *openapi3.Schema {
	s := openapi3.NewObjectSchema()
	s.Required = required
	for k, v := range props {
		s.Properties[k] = openapi3.NewSchemaRef("", v)
	}
	return s
}

func idRef() *openapi3.Schema {
	return obj(nil, map[string]*openapi3.Schema{"id": openapi3.NewInt64Schema()})
}

func TestFieldsCollisionPrefixesParent(t *testing.T) {
	req := obj(nil, map[string]*openapi3.Schema{
		"contact_person": openapi3.NewStringSchema(),
		"customer": obj(nil, map[string]*openapi3.Schema{
			"id":             openapi3.NewInt64Schema(),
			"contact_person": idRef(),
		}),
	})
	fs, err := Fields(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := flagNames(fs), []string{"contact-person", "customer", "customer-contact-person"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if f := fieldByFlag(t, fs, "customer-contact-person"); f.Kind != KindNestedRef {
		t.Errorf("prefixed field: %+v", f)
	}
	if f := fieldByFlag(t, fs, "contact-person"); f.Kind != KindScalar {
		t.Errorf("unprefixed field: %+v", f)
	}
}

func TestFieldsCollisionError(t *testing.T) {
	tests := map[string]*openapi3.Schema{
		"global flag": obj(nil, map[string]*openapi3.Schema{"output": openapi3.NewStringSchema()}),
		"still colliding": obj(nil, map[string]*openapi3.Schema{
			"contact_person":          openapi3.NewStringSchema(),
			"customer_contact_person": openapi3.NewStringSchema(),
			"customer": obj(nil, map[string]*openapi3.Schema{
				"id":             openapi3.NewInt64Schema(),
				"contact_person": idRef(),
			}),
		}),
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Fields(req, nil); err == nil {
				t.Error("want error")
			}
		})
	}
}

func TestFieldsHugeEnumHelp(t *testing.T) {
	big := openapi3.NewStringSchema()
	for i := range 25 {
		big.Enum = append(big.Enum, strings.Repeat("x", i+1))
	}
	fs, err := Fields(obj(nil, map[string]*openapi3.Schema{"category": big}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if h := fs[0].Help; !strings.HasSuffix(h, "(siehe Completion)") || strings.Contains(h, "erlaubt") {
		t.Errorf("help %q", h)
	}
}

// The flag help names the form of a pattern field (x-cli-pattern-format):
// --started-at-time of a time entry takes HH:MM, --ended-at-time also
// 24:00 (open point 22).
func TestFieldsPatternFormatHelp(t *testing.T) {
	fs, err := Fields(RequestSchema("CreateTimeEntry"), nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range fs {
		switch f.Flag {
		case "started-at-time", "ended-at-time":
			n++
			want := map[string]string{"started-at-time": " (HH:MM)", "ended-at-time": " (HH:MM oder 24:00)"}[f.Flag]
			if !strings.HasSuffix(f.Help, want) {
				t.Errorf("--%s: help %q", f.Flag, f.Help)
			}
		case "billable-duration":
			// the unit of duration in the responses, and that the API
			// ignores it (measured 2026-11-04)
			if !strings.HasPrefix(f.Help, "Abrechenbare Dauer in Sekunden; wirkungslos:") {
				t.Errorf("--%s: help %q", f.Flag, f.Help)
			}
		case "entry-date":
			if !strings.Contains(f.Help, "YYYY-MM-DD") {
				t.Errorf("--%s: help %q", f.Flag, f.Help)
			}
		}
	}
	if n != 2 {
		t.Errorf("%d time flags found, want 2", n)
	}
}

// HelpSuffix is the part of Help after the field's own text, so commands
// with their own flag texts (voucher pay) add the same date forms and enum
// values as the generic flags.
func TestHelpSuffix(t *testing.T) {
	byPath := map[string]Field{}
	for _, f := range mustFields(t, "PayVoucher") {
		byPath[strings.Join(f.Path, ".")] = f
	}
	for path, want := range map[string]string{
		"payment_date":        " (YYYY-MM-DD, heute, gestern)",
		"difference_reason":   " (erlaubt: sonstige, mahnung, teilzahlung, skonto, schmaelerung)",
		"value":               "",
		"banking_transaction": "",
	} {
		f, ok := byPath[path]
		if !ok {
			t.Fatalf("%s fehlt", path)
		}
		if got := HelpSuffix(f); got != want {
			t.Errorf("HelpSuffix(%s) = %q, erwartet %q", path, got, want)
		}
		if !strings.HasSuffix(f.Help, want) {
			t.Errorf("Help(%s) = %q endet nicht auf %q", path, f.Help, want)
		}
	}
	for _, f := range mustFields(t, "CreateVoucher") {
		if f.Flag == "category" && HelpSuffix(f) != " (siehe Completion)" {
			t.Errorf("category: HelpSuffix = %q", HelpSuffix(f))
		}
	}
}
