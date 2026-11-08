package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func voucher(items ...any) map[string]any {
	return map[string]any{"name": "Beleg", "provenance": "domestic", "line_items": items}
}

func validationError(t *testing.T, err error) *clierr.Error {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeValidation {
		t.Fatalf("want validation error, got %#v", err)
	}
	if ce.Exit() != 5 {
		t.Fatalf("exit = %d, want 5", ce.Exit())
	}
	return ce
}

func TestR12_ProvenanceNonEuSuggestsForeign(t *testing.T) {
	body := voucher(map[string]any{"vat_rate": json.Number("0.19")})
	body["provenance"] = "non_eu"
	ce := validationError(t, Validate(body, RequestSchema("CreateVoucher")))
	if want := `ungültiger Wert "non_eu" für provenance – meintest du "foreign"? Erlaubt: domestic, eu, foreign`; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
	// Without an enum hint the closest value is suggested.
	body["provenance"] = "domestc"
	ce = validationError(t, Validate(body, RequestSchema("CreateVoucher")))
	if want := `ungültiger Wert "domestc" für provenance – meintest du "domestic"? Erlaubt: domestic, eu, foreign`; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
	body["provenance"] = "xyz"
	ce = validationError(t, Validate(body, RequestSchema("CreateVoucher")))
	if want := `ungültiger Wert "xyz" für provenance. Erlaubt: domestic, eu, foreign`; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
}

func TestR17_LineItemsRequired(t *testing.T) {
	bodies := map[string]map[string]any{
		"CreateInvoice":  {"name": "R", "payment_term": map[string]any{"id": json.Number("1")}, "document_date": "2026-11-04"},
		"CreateEstimate": {"name": "A", "document_date": "2026-11-04"},
		"CreateVoucher":  {"name": "B", "provenance": "domestic"},
	}
	for op, body := range bodies {
		req := RequestSchema(op)
		ce := validationError(t, Validate(body, req))
		if ce.Message != `Pflichtfeld "line_items" fehlt` {
			t.Errorf("%s without line_items: message = %q", op, ce.Message)
		}
		for _, empty := range []any{[]any{}, nil} {
			with := map[string]any{"line_items": empty}
			for k, v := range body {
				with[k] = v
			}
			ce := validationError(t, Validate(with, req))
			if ce.Message != "line_items: mindestens eine Position erforderlich" {
				t.Errorf("%s with line_items %v: message = %q", op, empty, ce.Message)
			}
		}
	}
	// Update replaces the items; an empty list is allowed there (spec 6.3a, R18).
	for _, op := range []string{"UpdateInvoice", "UpdateEstimate", "UpdateVoucher"} {
		if err := Validate(map[string]any{"line_items": []any{}}, RequestSchema(op)); err != nil {
			t.Errorf("%s with empty line_items: %v", op, err)
		}
	}
}

func TestR28_CategoryTypoThreeSuggestions(t *testing.T) {
	req := RequestSchema("CreateVoucher")
	ce := validationError(t, Validate(voucher(map[string]any{"vat_rate": json.Number("0.19"), "category": "Bürobedraf"}), req))
	want := `ungültiger Wert "Bürobedraf" für line_items[0].category – meintest du "Bürobedarf", "Büroartikel" oder "Betriebsbedarf"?`
	if ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
	if strings.Contains(ce.Message+ce.Hint, "Grundstücke") || strings.Contains(ce.Message, "Erlaubt") {
		t.Errorf("category error lists all values: %q / %q", ce.Message, ce.Hint)
	}
	// Even far-off input gets exactly three suggestions.
	ce = validationError(t, Validate(voucher(map[string]any{"vat_rate": json.Number("0.19"), "category": "qqqqqqqqqqqqqqqqqqqq"}), req))
	if !strings.Contains(ce.Message, "– meintest du ") || strings.Count(ce.Message, `"`) != 8 || !strings.Contains(ce.Message, " oder ") {
		t.Errorf("want three suggestions, got %q", ce.Message)
	}
	// A valid category passes.
	if err := Validate(voucher(map[string]any{"vat_rate": json.Number("0.19"), "category": "Bürobedarf"}), req); err != nil {
		t.Errorf("valid category: %v", err)
	}
}

func TestValidateCollectsAllErrors(t *testing.T) {
	body := map[string]any{
		"provenance":    "non_eu",
		"document_date": "13.06.2020",
		"due_date":      "2026-02-30",
		"flagged":       "vielleicht",
		"line_items": []any{
			map[string]any{"vat_rate": true, "amount": "abc"},
			map[string]any{"name": "x"},
		},
	}
	ce := validationError(t, Validate(body, RequestSchema("CreateVoucher")))
	for _, want := range []string{
		`Pflichtfeld "name" fehlt`,
		`ungültiger Wert "non_eu" für provenance – meintest du "foreign"?`,
		`document_date: ungültiges Datum "13.06.2020"`,
		`due_date: ungültiges Datum "2026-02-30"`,
		`flagged: Wahrheitswert (true/false) erwartet`,
		`line_items[0].vat_rate: ungültiger Steuersatz true`,
		`line_items[0].amount: Zahl erwartet`,
		`Pflichtfeld "line_items[1].vat_rate" fehlt`,
	} {
		if !strings.Contains(ce.Message, want) {
			t.Errorf("message lacks %q:\n%s", want, ce.Message)
		}
	}
	if n := strings.Count(ce.Message, "\n") + 1; n != 8 {
		t.Errorf("got %d lines, want 8:\n%s", n, ce.Message)
	}
	for _, want := range []string{"YYYY-MM-DD", "0.19"} {
		if !strings.Contains(ce.Hint, want) {
			t.Errorf("hint %q lacks %q", ce.Hint, want)
		}
	}
	if strings.Contains(ce.Message, "property") || strings.Contains(ce.Message, "value must") {
		t.Errorf("English validator text leaked: %s", ce.Message)
	}
}

func TestValidateTypesAndNull(t *testing.T) {
	body := map[string]any{
		"name":         json.Number("5"),
		"payment_term": map[string]any{"id": json.Number("1.5")},
		"customer":     "12",
		"line_items":   []any{map[string]any{"quantity": "1", "proposition": nil, "vat_rate": json.Number("0.19")}},
	}
	ce := validationError(t, Validate(body, RequestSchema("CreateInvoice")))
	for _, want := range []string{
		`name: Text erwartet`,
		`payment_term.id: ganze Zahl erwartet`,
		`customer: Objekt erwartet`,
		`line_items[0].quantity: Zahl erwartet`,
		`line_items[0].proposition: darf nicht null sein`,
	} {
		if !strings.Contains(ce.Message, want) {
			t.Errorf("message lacks %q:\n%s", want, ce.Message)
		}
	}
	if err := Validate([]any{}, RequestSchema("CreateInvoice")); err == nil || !strings.Contains(err.Error(), "Body: Objekt erwartet") {
		t.Errorf("array body: %v", err)
	}
}

func TestValidateAnyOfNamesTypes(t *testing.T) {
	body := map[string]any{"name": "A", "contact_type": "customer", "custom_attributes": map[string]any{"12": map[string]any{"x": "y"}}}
	ce := validationError(t, Validate(body, RequestSchema("CreateCompany")))
	if want := "custom_attributes.12: Text, ganze Zahl, Zahl oder Wahrheitswert erwartet"; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
}

// For huge enums the x-cli-enum-hints entry comes first, the closest values
// fill up to three.
func TestValidateHugeEnumHintFirst(t *testing.T) {
	var enum []any
	for i := range 25 {
		enum = append(enum, fmt.Sprintf("wert%02d", i))
	}
	prop := &openapi3.Schema{Type: &openapi3.Types{"string"}, Enum: enum, Extensions: map[string]any{"x-cli-enum-hints": map[string]any{"alt": "wert24"}}}
	req := &openapi3.Schema{Type: &openapi3.Types{"object"}, Properties: openapi3.Schemas{"k": {Value: prop}}}
	ce := validationError(t, Validate(map[string]any{"k": "alt"}, req))
	if !strings.HasPrefix(ce.Message, `ungültiger Wert "alt" für k – meintest du "wert24", "wert00" oder "wert01"?`) {
		t.Errorf("message = %q", ce.Message)
	}
}

func TestValidateUnknownFieldWithoutSuggestion(t *testing.T) {
	ce := validationError(t, Validate(map[string]any{"name": "A", "contact_type": "customer", "zzzzzzzz": 1}, RequestSchema("CreateCompany")))
	if ce.Message != `unbekanntes Feld "zzzzzzzz"` {
		t.Errorf("message = %q", ce.Message)
	}
}

func TestValidateCustomAttributeKeyPattern(t *testing.T) {
	req := RequestSchema("CreateCompany")
	body := map[string]any{"name": "A", "contact_type": "customer", "custom_attributes": map[string]any{"12": "x", "abc": "y"}}
	ce := validationError(t, Validate(body, req))
	if want := `custom_attributes: Attribut-ID "abc" passt nicht zum Muster ^[0-9]+$`; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
	if !strings.Contains(ce.Hint, "papierkram custom-attribute list") {
		t.Errorf("hint = %q", ce.Hint)
	}
	body["custom_attributes"] = map[string]any{"12": "x", "13": json.Number("4"), "14": true, "15": nil}
	if err := Validate(body, req); err != nil {
		t.Errorf("valid custom attributes: %v", err)
	}
}

func TestValidateKeepsNumbers(t *testing.T) {
	body := map[string]any{"name": "A", "document_date": "2026-11-04", "line_items": []any{map[string]any{"name": "x", "quantity": json.Number("1"), "price": json.Number("19.99"), "vat_rate": json.Number("0.19"), "proposition": map[string]any{"id": json.Number("9007199254740993")}}}}
	before := marshal(t, body)
	if err := Validate(body, RequestSchema("CreateEstimate")); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if after := marshal(t, body); after != before {
		t.Errorf("Validate changed the body:\n%s\n%s", before, after)
	}
	if !strings.Contains(before, `"price":19.99`) || !strings.Contains(before, `"id":9007199254740993`) {
		t.Errorf("precision lost: %s", before)
	}
}

func TestValidateNilSchema(t *testing.T) {
	if err := Validate(map[string]any{"a": 1}, nil); err == nil {
		t.Error("want error for nil schema")
	}
}

func TestDecodeJSONBOMAndCRLF(t *testing.T) {
	in := "\xEF\xBB\xBF{\r\n  \"price\": 19.99,\r\n  \"id\": 9007199254740993,\r\n  \"name\": \"Zeile 1\\r\\nZeile 2\"\r\n}\r\n"
	v, err := DecodeJSON(strings.NewReader(in))
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	m := v.(map[string]any)
	if m["price"] != json.Number("19.99") {
		t.Errorf("price = %#v, want json.Number 19.99", m["price"])
	}
	if m["id"] != json.Number("9007199254740993") {
		t.Errorf("id = %#v", m["id"])
	}
	if m["name"] != "Zeile 1\r\nZeile 2" {
		t.Errorf("name = %q", m["name"])
	}
	// Arrays (--items) decode as well.
	v, err = DecodeJSON(strings.NewReader("\xEF\xBB\xBF[{\"quantity\": 1.25}]"))
	if err != nil || v.([]any)[0].(map[string]any)["quantity"] != json.Number("1.25") {
		t.Errorf("array: %#v, %v", v, err)
	}
}

// The NUL check against UTF-16 looks at raw bytes only: an escaped \u0000
// inside a JSON string is valid UTF-8 input and decodes to a NUL rune, next
// to non-ASCII text.
func TestDecodeJSONEscapedNULAndUnicode(t *testing.T) {
	v, err := DecodeJSON(strings.NewReader(`{"a":"\u0000 Müller 😀"}`))
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	got, _ := v.(map[string]any)["a"].(string)
	if got != "\x00 Müller 😀" {
		t.Errorf("a = %q, want %q", got, "\x00 Müller 😀")
	}
	for _, want := range []string{"\x00", "ü", "😀"} {
		if !strings.Contains(got, want) {
			t.Errorf("a = %q lacks %q", got, want)
		}
	}
}

func TestDecodeJSONRejectsOtherEncodings(t *testing.T) {
	tests := map[string]string{
		// "Müller" in Windows-1252: ü is the single byte 0xFC.
		"{\"name\": \"M\xfcller\"}":           "nicht UTF-8-kodiert (z. B. Windows-1252, Zeile 1, Spalte 12) – als UTF-8 speichern",
		"\xff\xfe{\x00\"\x00a\x00\"\x00}\x00": "UTF-16-kodiert – als UTF-8 speichern",
		"\xfe\xff\x00{\x00\"\x00a\x00\"\x00}": "UTF-16-kodiert – als UTF-8 speichern",
		"{\x00\"\x00a\x00\"\x00}\x00":         "UTF-16-kodiert – als UTF-8 speichern",
	}
	for in, want := range tests {
		_, err := DecodeJSON(strings.NewReader(in))
		ce := validationError(t, err)
		if !strings.Contains(ce.Message, want) {
			t.Errorf("DecodeJSON(%q): message %q lacks %q", in, ce.Message, want)
		}
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	tests := map[string]string{
		"":                        "leer",
		"\xEF\xBB\xBF  \r\n":      "leer",
		"{\"a\": 1} x":            "nach dem JSON-Wert",
		"{\"a\": 1}\n{\"b\": 2}":  "nach dem JSON-Wert",
		"{\r\n  \"a\": ,\r\n}":    `Zeile 2, Spalte 8: unerwartetes Zeichen ","`,
		"{\n  \"a\": 1,\n  \"b\"": "unvollständig",
	}
	for in, want := range tests {
		_, err := DecodeJSON(strings.NewReader(in))
		ce := validationError(t, err)
		if !strings.Contains(ce.Message, want) {
			t.Errorf("DecodeJSON(%q): message %q lacks %q", in, ce.Message, want)
		}
	}
}

// A pattern violation names the expected form (x-cli-pattern-format) in the
// message and the hint; without the extension the pattern itself is shown.
func TestValidatePatternNamesFormat(t *testing.T) {
	body := map[string]any{
		"entry_date": "2026-11-04", "started_at_time": "9:00", "ended_at_time": "10:30",
		"task": map[string]any{"id": json.Number("545")}, "user": map[string]any{"id": json.Number("1")},
	}
	ce := validationError(t, Validate(body, RequestSchema("CreateTimeEntry")))
	if want := `started_at_time: ungültiger Wert "9:00", erwartet HH:MM`; ce.Message != want {
		t.Errorf("message = %q, want %q", ce.Message, want)
	}
	if want := "Wert im Format HH:MM angeben"; ce.Hint != want {
		t.Errorf("hint = %q, want %q", ce.Hint, want)
	}

	s := &openapi3.Schema{Type: &openapi3.Types{"object"}, Properties: openapi3.Schemas{
		"code": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Pattern: "^[A-Z]{2}$"}},
	}}
	ce = validationError(t, Validate(map[string]any{"code": "de"}, s))
	if want := `code: ungültiger Wert "de" (Muster ^[A-Z]{2}$)`; ce.Message != want || ce.Hint != "" {
		t.Errorf("message = %q, hint = %q, want %q", ce.Message, ce.Hint, want)
	}
}
