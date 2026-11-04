package spec

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// getExample returns a response example of a GET operation from the embedded
// spec, decoded like --input (json.Number). name "" takes the first example
// in name order.
func getExample(t *testing.T, opID, name string) map[string]any {
	t.Helper()
	o, ok := OperationByID(opID)
	if !ok {
		t.Fatalf("operation %s not found", opID)
	}
	mt := o.Op.Responses.Status(200).Value.Content["application/json"]
	if mt == nil || len(mt.Examples) == 0 {
		t.Fatalf("%s has no 200 example", opID)
	}
	if name == "" {
		names := make([]string, 0, len(mt.Examples))
		for n := range mt.Examples {
			names = append(names, n)
		}
		sort.Strings(names)
		name = names[0]
	}
	ex := mt.Examples[name]
	if ex == nil || ex.Value == nil {
		t.Fatalf("%s: example %q missing", opID, name)
	}
	b, err := json.Marshal(ex.Value.Value)
	if err != nil {
		t.Fatal(err)
	}
	v, err := DecodeJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s example is %T, want object", opID, v)
	}
	return m
}

func prune(t *testing.T, opID string, body map[string]any) PruneResult {
	t.Helper()
	res, err := Prune(body, RequestSchema(opID), ResourceResponseSchema(opID))
	if err != nil {
		t.Fatalf("Prune(%s): %v", opID, err)
	}
	return res
}

func TestR14_UpdateInputPrunesReadOnly(t *testing.T) {
	in := getExample(t, "GetInvoice", "")
	before := marshal(t, in)
	res := prune(t, "UpdateInvoice", in)

	if after := marshal(t, in); after != before {
		t.Error("Prune modified its input")
	}
	for _, k := range []string{"id", "invoice_no", "created_at", "customer_no", "state", "record_state", "total_net", "contact_person", "project", "type"} {
		if _, ok := res.Body[k]; ok {
			t.Errorf("response-only field %q kept", k)
		}
	}
	for _, k := range []string{"name", "description", "document_date", "supply_date", "gross", "billing", "customer", "payment_term", "custom_template", "line_items"} {
		if _, ok := res.Body[k]; !ok {
			t.Errorf("request field %q removed", k)
		}
	}
	// the top-level references of the answer go into the customer (see
	// TestPruneMovesResponseReferencesIntoRequest)
	if got := marshal(t, res.Body["customer"]); got != `{"contact_person":{"id":1205},"id":3214,"project":{"id":4576}}` {
		t.Errorf("customer = %s", got)
	}
	if got := marshal(t, res.Body["payment_term"]); got != `{"id":1130}` {
		t.Errorf("payment_term = %s, want {\"id\":1130}", got)
	}
	for _, it := range res.Body["line_items"].([]any) {
		item := it.(map[string]any)
		for _, k := range []string{"total_net", "total_gross", "total_vat", "document_charge", "position_in_group"} {
			if _, ok := item[k]; ok {
				t.Errorf("line item field %q kept", k)
			}
		}
	}
	for _, want := range []string{"id", "invoice_no", "customer_no", "customer.name", "customer.type", "payment_term.template_id", "line_items[].total_net", "line_items[].position_in_group"} {
		if !slices.Contains(res.Removed, want) {
			t.Errorf("Removed %q lacks %q", res.Removed, want)
		}
	}
	if !slices.IsSorted(res.Removed) {
		t.Errorf("Removed not sorted: %q", res.Removed)
	}
	if n := len(slices.Compact(slices.Clone(res.Removed))); n != len(res.Removed) {
		t.Errorf("Removed has duplicates: %q", res.Removed)
	}
	// The request schema wins: billing.contact_person is a request field,
	// although the response has a top-level contact_person too.
	if b := res.Body["billing"].(map[string]any); b["contact_person"] != "John Doe" {
		t.Errorf("billing.contact_person = %v", b["contact_person"])
	}
}

// The answer of an invoice or estimate has its contact person and project
// on top level; the request takes them only below customer. A describe |
// update round trip that dropped them made the live API clear both
// (measured 2026-11-04, run 2), so Prune moves a top-level reference of
// the answer into the one request object that has a reference of that
// name, when that object is in the body and does not name it yet. Only
// the id is taken over.
func TestPruneMovesResponseReferencesIntoRequest(t *testing.T) {
	for _, op := range []string{"UpdateInvoice", "CreateInvoice", "UpdateEstimate", "CreateEstimate"} {
		in := map[string]any{
			"name":           "A",
			"customer":       map[string]any{"id": json.Number("7"), "name": "Kunde", "type": "company"},
			"contact_person": map[string]any{"id": json.Number("4"), "first_name": "Anna", "type": "person"},
			"project":        map[string]any{"id": json.Number("10"), "name": "P", "type": "project"},
		}
		res := prune(t, op, in)
		if got := marshal(t, res.Body["customer"]); got != `{"contact_person":{"id":4},"id":7,"project":{"id":10}}` {
			t.Errorf("%s: customer = %s", op, got)
		}
		for _, k := range []string{"contact_person", "project"} {
			if _, ok := res.Body[k]; ok || slices.Contains(res.Removed, k) {
				t.Errorf("%s: %s kept on top level or named as removed: %v %q", op, k, res.Body, res.Removed)
			}
		}
		if want := []string{"contact_person → customer.contact_person", "project → customer.project"}; !slices.Equal(res.Moved, want) {
			t.Errorf("%s: Moved = %q, want %q", op, res.Moved, want)
		}
	}

	// not moved: no customer in the body, a reference the customer already
	// names, a null, an object without id
	for _, tc := range []struct {
		name string
		in   map[string]any
		want string
	}{
		{"ohne customer", map[string]any{"contact_person": map[string]any{"id": json.Number("4")}}, `{}`},
		{"customer nennt sie schon", map[string]any{"customer": map[string]any{"id": json.Number("7"), "contact_person": map[string]any{"id": json.Number("5")}},
			"contact_person": map[string]any{"id": json.Number("4")}}, `{"customer":{"contact_person":{"id":5},"id":7}}`},
		{"null", map[string]any{"customer": map[string]any{"id": json.Number("7")}, "project": nil}, `{"customer":{"id":7}}`},
		{"ohne id", map[string]any{"customer": map[string]any{"id": json.Number("7")}, "project": map[string]any{"name": "P"}}, `{"customer":{"id":7}}`},
	} {
		res := prune(t, "UpdateInvoice", tc.in)
		if got := marshal(t, res.Body); got != tc.want || len(res.Moved) != 0 {
			t.Errorf("%s: body %s, Moved %q; want %s and nothing moved", tc.name, got, res.Moved, tc.want)
		}
	}
}

// Moving references is spec-driven; this pins where it applies: only the
// contact person and project of invoices and estimates (into customer). A
// new match after a spec update shows up here.
func TestPruneMovableReferences(t *testing.T) {
	var got []string
	for _, o := range Operations() {
		if o.Method != "POST" && o.Method != "PUT" {
			continue
		}
		req := RequestSchema(o.ID)
		resp := ResourceResponseSchema(o.ID)
		if req == nil || resp == nil {
			continue
		}
		for _, m := range movableReferences(req, flatten([]*openapi3.Schema{resp})) {
			got = append(got, o.ID+": "+m)
		}
	}
	slices.Sort(got)
	want := []string{
		"CreateEstimate: contact_person → customer.contact_person", "CreateEstimate: project → customer.project",
		"CreateInvoice: contact_person → customer.contact_person", "CreateInvoice: project → customer.project",
		"UpdateEstimate: contact_person → customer.contact_person", "UpdateEstimate: project → customer.project",
		"UpdateInvoice: contact_person → customer.contact_person", "UpdateInvoice: project → customer.project",
	}
	if !slices.Equal(got, want) {
		t.Errorf("movable references:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestR31_DescribeRoundTripKeepsItems(t *testing.T) {
	req := RequestSchema("UpdateInvoice")
	res := prune(t, "UpdateInvoice", getExample(t, "GetInvoice", ""))
	coerced, err := CoerceValue(req, res.Body, "")
	if err != nil {
		t.Fatalf("CoerceValue: %v", err)
	}
	if err := Validate(coerced, req); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	items := coerced.(map[string]any)["line_items"].([]any)
	if len(items) != 2 {
		t.Fatalf("got %d line items, want 2", len(items))
	}
	first := items[0].(map[string]any)
	for _, k := range []string{"name", "description", "quantity", "price", "unit", "type_of", "discount_percentage", "discount_calculated", "price_gross"} {
		if _, ok := first[k]; !ok {
			t.Errorf("line item field %q removed (request schema must win over readOnly)", k)
		}
	}
	if first["vat_rate"] != json.Number("0.19") {
		t.Errorf("vat_rate = %#v, want json.Number 0.19", first["vat_rate"])
	}
	if v := items[1].(map[string]any)["vat_rate"]; v != json.Number("0.07") {
		t.Errorf("second vat_rate = %#v, want json.Number 0.07", v)
	}
	if first["name"] != "Büroartikel" || first["price"] != json.Number("100") {
		t.Errorf("line item values changed: %v", first)
	}
}

// Every GET example of a resource with an update operation survives the
// describe → update round trip: Prune, CoerceValue and Validate accept it.
func TestPruneRoundTripAllUpdateExamples(t *testing.T) {
	n := 0
	for _, o := range Operations() {
		if o.Method != "PUT" || RequestSchema(o.ID) == nil {
			continue
		}
		get := strings.Replace(o.ID, "Update", "Get", 1)
		if _, ok := OperationByID(get); !ok {
			t.Errorf("%s: no %s", o.ID, get)
			continue
		}
		// GET answers (describe) and the PUT's own 200 answers (output of a
		// previous update) both go back in.
		for _, src := range []string{get, o.ID} {
			s, _ := OperationByID(src)
			mt := s.Op.Responses.Status(200).Value.Content["application/json"]
			if mt == nil {
				t.Errorf("%s: no JSON 200 response", src)
				continue
			}
			for name := range mt.Examples {
				n++
				req := RequestSchema(o.ID)
				res, err := Prune(getExample(t, src, name), req, ResourceResponseSchema(o.ID))
				if err != nil {
					t.Errorf("%s / %s: Prune: %v", src, name, err)
					continue
				}
				c, err := CoerceValue(req, res.Body, "")
				if err != nil {
					t.Errorf("%s / %s: CoerceValue: %v", src, name, err)
					continue
				}
				if err := Validate(c, req); err != nil {
					t.Errorf("%s / %s: Validate: %v", src, name, err)
				}
			}
		}
	}
	if n < 60 {
		t.Errorf("only %d examples checked", n)
	}
}

func TestR13_NoteSuggestsNotes(t *testing.T) {
	body := map[string]any{"name": "ACME", "contact_type": "customer", "note": "x"}
	_, err := Prune(body, RequestSchema("CreateCompany"), ResourceResponseSchema("CreateCompany"))
	wantValidation(t, err, `unbekanntes Feld "note" – meintest du "notes"?`)
	if exit := err.(interface{ Exit() int }).Exit(); exit != 5 {
		t.Errorf("exit = %d, want 5", exit)
	}
	ce := validationError(t, err)
	if want := "stammt das Feld aus einer describe-Ausgabe: mit jq 'del(.note)' entfernen oder die CLI aktualisieren"; ce.Hint != want {
		t.Errorf("hint = %q, want %q", ce.Hint, want)
	}
	// Validate without Prune reports the same (additionalProperties: false).
	wantValidation(t, Validate(body, RequestSchema("CreateCompany")), `unbekanntes Feld "note" – meintest du "notes"?`)
}

func TestPruneCollectsAllUnknownFields(t *testing.T) {
	body := map[string]any{
		"name": "R", "nmae": "x", "zzzzzzzz": 1,
		"customer":   map[string]any{"id": json.Number("1"), "proejct": "y"},
		"line_items": []any{map[string]any{"name": "a", "quantty": 1}, map[string]any{"name": "b", "quantty": 2}},
	}
	_, err := Prune(body, RequestSchema("CreateInvoice"), ResourceResponseSchema("CreateInvoice"))
	wantValidation(t, err,
		`unbekanntes Feld "nmae" – meintest du "name"?`,
		`unbekanntes Feld "customer.proejct" – meintest du "project"?`,
		`unbekanntes Feld "line_items[].quantty" – meintest du "quantity"?`,
		`jq 'del(.customer.proejct, .line_items[].quantty, .nmae, .zzzzzzzz)'`,
	)
	if ce := validationError(t, err); strings.Contains(ce.Hint, "meintest") {
		t.Errorf("hint repeats the suggestions: %q", ce.Hint)
	}
	msg := err.Error()
	if !strings.Contains(msg, "unbekanntes Feld \"zzzzzzzz\"\n") && !strings.HasSuffix(msg, `unbekanntes Feld "zzzzzzzz"`) {
		t.Errorf("unknown field without suggestion missing or with suggestion clause: %q", msg)
	}
	if c := strings.Count(msg, "quantty"); c != 1 {
		t.Errorf("line item unknown reported %d times, want once: %q", c, msg)
	}
	if lines := strings.Count(msg, "\n") + 1; lines != 4 {
		t.Errorf("got %d lines, want 4: %q", lines, msg)
	}
}

func TestPruneNullForNonNullableRequestField(t *testing.T) {
	body := map[string]any{
		"name":            "R",
		"custom_template": nil,
		"description":     nil,
		"id":              json.Number("1"),
		"line_items":      []any{map[string]any{"name": "a", "proposition": nil}},
	}
	res := prune(t, "UpdateInvoice", body)
	if _, ok := res.Body["custom_template"]; ok {
		t.Error("custom_template: null kept although the request does not allow null")
	}
	if v, ok := res.Body["description"]; !ok || v != nil {
		t.Error("description: null removed although the request allows null")
	}
	if _, ok := res.Body["line_items"].([]any)[0].(map[string]any)["proposition"]; ok {
		t.Error("line_items[].proposition: null kept")
	}
	// Dropped nulls are reported apart from response-only fields: the
	// builder says the field stays unchanged.
	if !slices.Equal(res.DroppedNulls, []string{"custom_template", "line_items[].proposition"}) {
		t.Errorf("DroppedNulls = %q", res.DroppedNulls)
	}
	if !slices.Equal(res.Removed, []string{"id"}) {
		t.Errorf("Removed = %q", res.Removed)
	}
}

// A null in a required field is not dropped (that would turn it into a
// missing field); Validate reports it.
func TestPruneKeepsNullInRequiredField(t *testing.T) {
	req := RequestSchema("CreateInvoice")
	body := map[string]any{"name": "R", "payment_term": nil, "line_items": []any{map[string]any{"name": "a", "vat_rate": json.Number("0.19")}}}
	res := prune(t, "CreateInvoice", body)
	if v, ok := res.Body["payment_term"]; !ok || v != nil {
		t.Fatalf("payment_term = %#v, %v; want kept null", v, ok)
	}
	if len(res.DroppedNulls) != 0 {
		t.Errorf("DroppedNulls = %q", res.DroppedNulls)
	}
	wantValidation(t, Validate(res.Body, req), "payment_term: darf nicht null sein")
}

func TestPruneKeepsFreeFormMaps(t *testing.T) {
	body := map[string]any{"name": "ACME", "contact_type": "customer", "custom_attributes": map[string]any{"12": "x", "13": json.Number("4")}}
	res := prune(t, "UpdateCompany", body)
	if got := marshal(t, res.Body["custom_attributes"]); got != `{"12":"x","13":4}` {
		t.Errorf("custom_attributes = %s", got)
	}
	if len(res.Removed) != 0 {
		t.Errorf("Removed = %q", res.Removed)
	}
}

func TestPruneKeepsPrecision(t *testing.T) {
	body := map[string]any{"name": "A", "article_no": "1", "time_unit": "hour", "proposition_type": "service", "vat_rate": "0.19", "price": json.Number("19.99"), "id": json.Number("9007199254740993")}
	res := prune(t, "UpdateProposition", body)
	if res.Body["price"] != json.Number("19.99") {
		t.Errorf("price = %#v", res.Body["price"])
	}
	if !slices.Equal(res.Removed, []string{"id"}) {
		t.Errorf("Removed = %q", res.Removed)
	}
}

func TestPruneNilRequestSchema(t *testing.T) {
	_, err := Prune(map[string]any{"a": 1}, nil, nil)
	if err == nil {
		t.Fatal("want error for nil request schema")
	}
}

func TestResourceResponseSchema(t *testing.T) {
	get := ResponseSchema("GetInvoice")
	for _, op := range []string{"CreateInvoice", "UpdateInvoice", "GetInvoice"} {
		if ResourceResponseSchema(op) != get {
			t.Errorf("%s: not the GetInvoice response schema", op)
		}
	}
	if ResourceResponseSchema("CreatePerson") != ResponseSchema("GetPerson") {
		t.Error("CreatePerson: not the GetPerson response schema")
	}
	if ResourceResponseSchema("UpdatePerson") != ResponseSchema("GetPerson") {
		t.Error("UpdatePerson: not the GetPerson response schema")
	}
	for _, op := range []string{"DeliverInvoice", "Nope"} {
		if ResourceResponseSchema(op) != nil {
			t.Errorf("%s: want nil", op)
		}
	}
	// Every create/update operation with a body finds its resource.
	for _, o := range Operations() {
		if RequestSchema(o.ID) == nil || !strings.HasPrefix(o.ID, "Create") && !strings.HasPrefix(o.ID, "Update") {
			continue
		}
		if ResourceResponseSchema(o.ID) == nil {
			t.Errorf("%s: no resource response schema", o.ID)
		}
	}
}

// A field under a different name (address for recipient, R06 at the CLI
// level) is not close in spelling. When it is the only unknown field of
// its object and exactly one required field of that object is missing, the
// missing field is the suggestion.
func TestPruneSuggestsTheOnlyMissingRequiredField(t *testing.T) {
	body := map[string]any{"send_via": "email", "email": map[string]any{"address": "a@example.org", "subject": "S", "body": "B"}}
	_, err := Prune(body, RequestSchema("DeliverInvoice"), nil)
	wantValidation(t, err, `unbekanntes Feld "email.address" – meintest du "recipient"?`)

	// two missing required fields: no guess
	body = map[string]any{"send_via": "email", "email": map[string]any{"address": "a@example.org", "body": "B"}}
	_, err = Prune(body, RequestSchema("DeliverInvoice"), nil)
	if msg := validationError(t, err).Message; msg != `unbekanntes Feld "email.address"` {
		t.Errorf("message = %q", msg)
	}

	// two unknown fields: no guess
	body = map[string]any{"send_via": "email", "email": map[string]any{"address": "a@example.org", "foo": "x", "subject": "S", "body": "B"}}
	_, err = Prune(body, RequestSchema("DeliverInvoice"), nil)
	if msg := validationError(t, err).Message; strings.Contains(msg, "meintest") {
		t.Errorf("message = %q", msg)
	}
}

// The rule counts only the unknown fields and missing required fields of
// the object itself: an unknown field in a nested object or line item never
// gets the missing required field of its parent.
func TestPruneMissingRequiredSuggestionStaysInItsObject(t *testing.T) {
	// email lacks only recipient; the missing send_via of the parent does
	// not count
	body := map[string]any{"email": map[string]any{"address": "a@example.org", "subject": "S", "body": "B"}}
	_, err := Prune(body, RequestSchema("DeliverInvoice"), nil)
	wantValidation(t, err, `unbekanntes Feld "email.address" – meintest du "recipient"?`)

	for _, tc := range []struct {
		name, op string
		body     map[string]any
		want     string
	}{
		// email lacks recipient and subject; the parent lacks only send_via
		{"nested", "DeliverInvoice", map[string]any{"email": map[string]any{"address": "a@example.org", "body": "B"}},
			`unbekanntes Feld "email.address"`},
		// customer has no required field; the parent lacks only name
		{"reference", "CreateProject", map[string]any{"customer": map[string]any{"kundennummer": json.Number("5")}},
			`unbekanntes Feld "customer.kundennummer"`},
		// the parent lacks only payment_term
		{"line item", "CreateInvoice", map[string]any{"name": "R", "line_items": []any{map[string]any{"name": "A", "zzzzzzzz": 1}}},
			`unbekanntes Feld "line_items[].zzzzzzzz"`},
	} {
		_, err := Prune(tc.body, RequestSchema(tc.op), ResourceResponseSchema(tc.op))
		if msg := validationError(t, err).Message; msg != tc.want {
			t.Errorf("%s: message = %q, want %q", tc.name, msg, tc.want)
		}
	}
}
