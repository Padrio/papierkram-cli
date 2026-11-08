package spec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/itchyny/gojq"
)

// Body values in messages read as JSON, never as Go syntax (task 17,
// L179).
func TestShowValueJSON(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{map[string]any{}, `{}`},
		{[]any{}, `[]`},
		{map[string]any{"a": json.Number("1")}, `{"a":1}`},
		{[]any{"x", true, nil}, `["x",true,null]`},
		{map[string]any{"html": "<b>&"}, `{"html":"<b>&"}`},
		{"text", `"text"`},
		{nil, "null"},
		{json.Number("2.5"), "2.5"},
		{true, "true"},
	} {
		if got := showValue(tc.v); got != tc.want {
			t.Errorf("showValue(%#v) = %s, want %s", tc.v, got, tc.want)
		}
	}
	s := openapi3.NewStringSchema()
	s.Extensions = map[string]any{"x-cli-bool-string": true}
	_, err := CoerceValue(s, map[string]any{}, "flagged")
	if ce := validationError(t, err); ce.Message != "flagged: ungültiger Wahrheitswert {}" {
		t.Errorf("message = %q", ce.Message)
	}
}

// A JSON boolean from --input also takes the numbers 0 and 1, like the
// flag; other numbers stay for the validator (exit 5) (task 17, L179).
func TestCoerceBooleanFromNumbers(t *testing.T) {
	req := RequestSchema("CreateInvoice")
	for _, tc := range []struct {
		in   any
		want any
	}{
		{json.Number("1"), true},
		{json.Number("0"), false},
		{float64(1), true},
		{0, false},
		{true, true},
		{"ja", true},
	} {
		got, err := CoerceValue(req.Properties["flagged"].Value, tc.in, "flagged")
		if err != nil || got != tc.want {
			t.Errorf("CoerceValue(%#v) = %#v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []any{json.Number("2"), json.Number("1.0"), json.Number("-1")} {
		body := map[string]any{"name": "R", "customer": map[string]any{"id": json.Number("1")}, "flagged": in}
		v, err := CoerceValue(req, body, "")
		if err != nil {
			t.Fatalf("%v: %v", in, err)
		}
		if v.(map[string]any)["flagged"] != in {
			t.Errorf("%v: changed to %#v", in, v.(map[string]any)["flagged"])
		}
		_ = validationError(t, Validate(v.(map[string]any), req))
	}
}

// The --attr hint shows the example 12=Wert only when 12 fits the key
// pattern (task 17, L179).
func TestAttrHintFollowsPattern(t *testing.T) {
	f := fieldByFlag(t, mustFields(t, "CreateProject"), "attr")
	for _, raw := range []string{"x=1", "12"} {
		_, err := coerceAttr(f, raw)
		if ce := validationError(t, err); ce.Hint != "z. B. --attr 12=Wert" {
			t.Errorf("%s: hint = %q", raw, ce.Hint)
		}
	}
	custom := f
	custom.Schema = openapi3.NewObjectSchema()
	custom.Schema.Extensions = map[string]any{"x-cli-key-pattern": "^[a-z]+$"}
	for _, raw := range []string{"12=1", "abc"} {
		_, err := coerceAttr(custom, raw)
		if ce := validationError(t, err); ce.Hint != "Schlüssel muss zum Muster ^[a-z]+$ passen" {
			t.Errorf("%s: hint = %q", raw, ce.Hint)
		}
	}
	if v, err := coerceAttr(custom, "abc=1"); err != nil || v.(map[string]any)["abc"] != "1" {
		t.Errorf("abc=1: %#v, %v", v, err)
	}
}

// Keys that are no jq identifiers appear as .["…"] at the top level and
// as ["…"] below (ruling T17-A: jq 1.6 rejects .a.["k"]), JSON-quoted with
// ' as a JSON unicode escape for the single-quoted shell argument (task
// 17, L188). The hint's jq program, run by gojq on the body, removes
// exactly the unknown fields.
// Review T17-deferred: keys with a quote, a backslash or control
// characters (also U+2028) are JSON-escaped in the jq step, so the hint
// holds no raw control character and no ', and jq removes exactly them.
func TestUnknownFieldJQPathQuotingEscapes(t *testing.T) {
	body := map[string]any{
		"name": "R", `q"uote`: 1, `back\slash`: 1, "tab\tkey": 1, "nl\nkey": 1, "ctl\x01": 1, "ls\u2028": 1,
		"customer": map[string]any{"id": json.Number("1"), `"`: 1},
	}
	_, err := Prune(body, RequestSchema("CreateInvoice"), ResourceResponseSchema("CreateInvoice"))
	ce := validationError(t, err)
	want := `stammen die Felder aus einer describe-Ausgabe: mit jq 'del(.["back\\slash"], .["ctl\u0001"], .customer["\""], .["ls\u2028"], .["nl\nkey"], .["q\"uote"], .["tab\tkey"])' entfernen oder die CLI aktualisieren`
	if ce.Hint != want {
		t.Errorf("hint =\n%s\nwant\n%s", ce.Hint, want)
	}
	for _, r := range ce.Hint {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			t.Errorf("hint holds raw %U: %q", r, ce.Hint)
		}
	}
	i, j := strings.Index(ce.Hint, "jq '"), strings.LastIndex(ce.Hint, "'")
	if i < 0 || j <= i+4 || strings.Contains(ce.Hint[i+4:j], "'") {
		t.Fatalf("no single-quoted jq program in %q", ce.Hint)
	}
	prog := ce.Hint[i+4 : j]
	q, err := gojq.Parse(prog)
	if err != nil {
		t.Fatalf("gojq parses %q not: %v", prog, err)
	}
	got, ok := q.Run(body).Next()
	if !ok {
		t.Fatal("no result")
	}
	if err, isErr := got.(error); isErr {
		t.Fatalf("gojq %q: %v", prog, err)
	}
	clean := map[string]any{"name": "R", "customer": map[string]any{"id": json.Number("1")}}
	if !reflect.DeepEqual(got, clean) {
		t.Errorf("gojq %q: %#v, want %#v", prog, got, clean)
	}
}

func TestUnknownFieldJQPathQuoting(t *testing.T) {
	body := map[string]any{
		"name": "R", "12": 1, "a.b": 1, "x[1]": 1, "it's": 1, "with space": 1, "ä": 1, "ok_1": 1,
		"customer":   map[string]any{"id": json.Number("1"), "c.d": 1},
		"line_items": []any{map[string]any{"name": "a", "e f": 1}},
	}
	_, err := Prune(body, RequestSchema("CreateInvoice"), ResourceResponseSchema("CreateInvoice"))
	ce := validationError(t, err)
	want := `stammen die Felder aus einer describe-Ausgabe: mit jq 'del(.["12"], .["a.b"], .customer["c.d"], .["it\u0027s"], .line_items[]["e f"], .ok_1, .["with space"], .["x[1]"], .["ä"])' entfernen oder die CLI aktualisieren`
	if ce.Hint != want {
		t.Errorf("hint =\n%s\nwant\n%s", ce.Hint, want)
	}
	i, j := strings.Index(ce.Hint, "jq '"), strings.LastIndex(ce.Hint, "'")
	if i < 0 || j <= i+4 {
		t.Fatalf("no jq program in %q", ce.Hint)
	}
	prog := ce.Hint[i+4 : j]
	q, err := gojq.Parse(prog)
	if err != nil {
		t.Fatalf("gojq parses %q not: %v", prog, err)
	}
	got, ok := q.Run(body).Next()
	if !ok {
		t.Fatal("no result")
	}
	if err, isErr := got.(error); isErr {
		t.Fatalf("gojq %q: %v", prog, err)
	}
	clean := map[string]any{
		"name":       "R",
		"customer":   map[string]any{"id": json.Number("1")},
		"line_items": []any{map[string]any{"name": "a"}},
	}
	if !reflect.DeepEqual(got, clean) {
		t.Errorf("gojq %q: %#v, want %#v", prog, got, clean)
	}
	if _, err := Prune(got.(map[string]any), RequestSchema("CreateInvoice"), ResourceResponseSchema("CreateInvoice")); err != nil {
		t.Errorf("Prune after jq: %v", err)
	}
}
