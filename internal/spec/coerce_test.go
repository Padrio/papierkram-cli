package spec

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// testCC is late in the evening UTC, already the next day in Berlin.
func testCC(t *testing.T) CoerceContext {
	return CoerceContext{
		Now: func() time.Time { return time.Date(2026, 11, 3, 23, 30, 0, 0, time.UTC) },
		Loc: berlin(t),
	}
}

func coerceFlag(t *testing.T, opID, flag, raw string) (any, error) {
	t.Helper()
	return CoerceFlag(testCC(t), fieldByFlag(t, mustFields(t, opID), flag), raw)
}

func wantValidation(t *testing.T, err error, contains ...string) {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeValidation {
		t.Fatalf("want validation error, got %#v", err)
	}
	full := ce.Message + " | " + ce.Hint
	for _, c := range contains {
		if !strings.Contains(full, c) {
			t.Errorf("error %q does not contain %q", full, c)
		}
	}
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestR07_BudgetAsString(t *testing.T) {
	for _, op := range []string{"CreateProject", "UpdateProject"} {
		for _, flag := range []string{"budget-money", "budget-time"} {
			v, err := coerceFlag(t, op, flag, "5000")
			if err != nil || v != "5000" {
				t.Errorf("%s --%s 5000: got %#v, %v", op, flag, v, err)
			}
		}
		body := map[string]any{"budget_money": json.Number("5000"), "budget_time": float64(12.5), "name": "P"}
		got, err := CoerceValue(RequestSchema(op), body, "")
		if err != nil {
			t.Fatal(err)
		}
		if s := marshal(t, got); s != `{"budget_money":"5000","budget_time":"12.5","name":"P"}` {
			t.Errorf("%s body: %s", op, s)
		}
	}
}

func TestR08_TaskFieldsAsString(t *testing.T) {
	for _, op := range []string{"CreateTask", "UpdateTask"} {
		for _, tc := range []struct{ flag, raw, want string }{
			{"complete", "true", "true"},
			{"complete", "nein", "false"},
			{"flagged", "ja", "true"},
			{"flagged", "0", "false"},
			{"relative-costs", "50", "50"},
		} {
			v, err := coerceFlag(t, op, tc.flag, tc.raw)
			if err != nil || v != tc.want {
				t.Errorf("%s --%s %s: got %#v, %v; want %q", op, tc.flag, tc.raw, v, err, tc.want)
			}
		}
		for _, tc := range []struct {
			complete, flagged any
		}{
			{true, false},
			{"ja", "nein"},
			{"TRUE", "false"},
			{json.Number("1"), json.Number("0")},
			{float64(1), 0},
			{"1", "0"},
		} {
			body := map[string]any{"complete": tc.complete, "flagged": tc.flagged, "relative_costs": json.Number("50")}
			got, err := CoerceValue(RequestSchema(op), body, "")
			if err != nil {
				t.Fatal(err)
			}
			if s := marshal(t, got); s != `{"complete":"true","flagged":"false","relative_costs":"50"}` {
				t.Errorf("%s body %v: %s", op, tc, s)
			}
		}
		for _, bad := range []any{"vielleicht", json.Number("2"), float64(0.5), map[string]any{}} {
			_, err := CoerceValue(RequestSchema(op), map[string]any{"flagged": bad}, "")
			wantValidation(t, err, "flagged", "erlaubt: true, false, ja, nein, 1, 0")
		}
	}
}

func TestR09_PersonDefaultFlaggedAsString(t *testing.T) {
	for _, op := range []string{"CreatePerson", "UpdatePerson"} {
		for _, flag := range []string{"default", "flagged"} {
			v, err := coerceFlag(t, op, flag, "true")
			if err != nil || v != "true" {
				t.Errorf("%s --%s: got %#v, %v", op, flag, v, err)
			}
		}
		got, err := CoerceValue(RequestSchema(op), map[string]any{"default": "ja", "flagged": "nein"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if s := marshal(t, got); s != `{"default":"true","flagged":"false"}` {
			t.Errorf("%s body: %s", op, s)
		}
	}
	// people of company create carry the same string booleans.
	body := map[string]any{"name": "ACME", "people": []any{map[string]any{"default": true, "flagged": "nein", "first_name": "A"}}}
	got, err := CoerceValue(RequestSchema("CreateCompany"), body, "")
	if err != nil {
		t.Fatal(err)
	}
	if s := marshal(t, got); s != `{"name":"ACME","people":[{"default":"true","first_name":"A","flagged":"false"}]}` {
		t.Errorf("company body: %s", s)
	}
	body = map[string]any{"people": []any{map[string]any{"default": "x"}}}
	_, err = CoerceValue(RequestSchema("CreateCompany"), body, "")
	wantValidation(t, err, `people[0].default`, `"x"`)
}

func TestR10_PropositionPriceVatAsString(t *testing.T) {
	for _, op := range []string{"CreateProposition", "UpdateProposition"} {
		for _, body := range []map[string]any{
			{"price": json.Number("12.5"), "vat_rate": json.Number("19")},
			{"price": float64(12.5), "vat_rate": float64(19)},
			{"price": "12.5", "vat_rate": "19%"},
		} {
			got, err := CoerceValue(RequestSchema(op), body, "")
			if err != nil {
				t.Fatal(err)
			}
			if s := marshal(t, got); s != `{"price":"12.5","vat_rate":"0.19"}` {
				t.Errorf("%s %v: %s", op, body, s)
			}
		}
		if v, err := coerceFlag(t, op, "price", "12.5"); err != nil || v != "12.5" {
			t.Errorf("%s --price: %#v, %v", op, v, err)
		}
		for _, raw := range []string{"19", "19%", "0,19", "0.19"} {
			if v, err := coerceFlag(t, op, "vat-rate", raw); err != nil || v != "0.19" {
				t.Errorf("%s --vat-rate %s: %#v, %v", op, raw, v, err)
			}
		}
		_, err := coerceFlag(t, op, "vat-rate", "abc")
		wantValidation(t, err, "--vat-rate", `"abc"`)
	}
}

func TestR11_VatNormalization(t *testing.T) {
	tests := []struct {
		in   any
		want float64
	}{
		{19, 0.19},
		{"19%", 0.19},
		{"19", 0.19},
		{"0,19", 0.19},
		{"0.19", 0.19},
		{float64(19), 0.19},
		{json.Number("19"), 0.19},
		{int64(19), 0.19},
		{" 19 % ", 0.19},
		{7, 0.07},
		{"7%", 0.07},
		{0, 0},
		{"0", 0},
		{"0%", 0},
		{"5,5", 0.055},
		{"100", 1},
		{100, 1},
		{"100%", 1},
		{"1%", 0.01},
		{"0.12345", 0.1235},
	}
	for _, tc := range tests {
		got, err := NormalizeVAT(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeVAT(%#v) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []any{-1, "-19", 101, "101%", "abc", "", "NaN", "Inf", "1e3", true, nil, []any{}} {
		_, err := NormalizeVAT(bad)
		wantValidation(t, err, "Steuersatz")
	}
	// A bare 1 could mean 1 % or 100 %.
	for _, amb := range []any{1, "1", "1.0", " 1 ", json.Number("1"), float64(1), int64(1)} {
		_, err := NormalizeVAT(amb)
		wantValidation(t, err, "vat_rate 1 ist mehrdeutig (1 % oder 100 %?)", "0.01 bzw. 1% oder 100% angeben")
	}
	_, err := coerceFlag(t, "CreateProposition", "vat-rate", "1")
	wantValidation(t, err, "--vat-rate", "mehrdeutig")

	// Line items: every accepted form ends up as the number 0.19.
	for _, in := range []any{"19%", json.Number("19"), float64(19), "0,19", "19"} {
		body := map[string]any{"line_items": []any{map[string]any{"name": "X", "vat_rate": in}}}
		got, err := CoerceValue(RequestSchema("CreateInvoice"), body, "")
		if err != nil {
			t.Fatal(err)
		}
		if s := marshal(t, got); s != `{"line_items":[{"name":"X","vat_rate":0.19}]}` {
			t.Errorf("vat_rate %#v: %s", in, s)
		}
	}
	body := map[string]any{"line_items": []any{map[string]any{"vat_rate": "x"}}}
	_, err = CoerceValue(RequestSchema("CreateVoucher"), body, "")
	wantValidation(t, err, "line_items[0].vat_rate", `"x"`)
}

func TestFormatVAT(t *testing.T) {
	for in, want := range map[float64]string{0.19: "0.19", 0.07: "0.07", 0: "0", 0.055: "0.055", 1: "1"} {
		if got := FormatVAT(in); got != want {
			t.Errorf("FormatVAT(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestCoerceFlagTypes(t *testing.T) {
	tests := []struct {
		op, flag, raw string
		want          any
	}{
		{"CreateTimeEntry", "billable-duration", "90", int64(90)},
		{"PayVoucher", "value", "12.50", json.Number("12.50")},
		{"PayVoucher", "value", "-3", json.Number("-3")},
		{"CreateInvoice", "flagged", "ja", true},
		{"CreateInvoice", "flagged", "False", false},
		{"CreateInvoice", "flagged", "1", true},
		{"CreateInvoice", "document-date", "2026-11-03", "2026-11-03"},
		{"CreateInvoice", "document-date", "heute", "2026-11-04"},
		{"CreateInvoice", "document-date", "Today", "2026-11-04"},
		{"CreateInvoice", "document-date", "gestern", "2026-11-03"},
		{"CreateInvoice", "document-date", "yesterday", "2026-11-03"},
		{"CreateInvoice", "customer", "12", map[string]any{"id": int64(12)}},
		{"CreateInvoice", "customer", "0", map[string]any{"id": int64(0)}},
		{"CreateInvoice", "customer", "9007199254740993", map[string]any{"id": int64(9007199254740993)}},
		{"CreateInvoice", "contact-person", "7", map[string]any{"id": int64(7)}},
		{"CreateProject", "team-member", "1", map[string]any{"id": int64(1)}},
		{"CreateProject", "attr", "12=Wert mit = Zeichen", map[string]any{"12": "Wert mit = Zeichen"}},
		{"CreateProject", "color", "blue", "blue"},
		{"CreateVoucher", "provenance", "eu", "eu"},
		{"CreateInvoice", "name", " Beratung ", " Beratung "},
		{"CreateVoucher", "entertainment-person", "Carl Customer", "Carl Customer"},
	}
	for _, tc := range tests {
		got, err := coerceFlag(t, tc.op, tc.flag, tc.raw)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s --%s %q: got %#v, %v; want %#v", tc.op, tc.flag, tc.raw, got, err, tc.want)
		}
	}
}

func TestCoerceFlagErrors(t *testing.T) {
	tests := []struct {
		op, flag, raw string
		contains      []string
	}{
		{"CreateTimeEntry", "billable-duration", "9x", []string{"--billable-duration", `"9x"`, "ganze Zahl"}},
		{"CreateTimeEntry", "billable-duration", "1.5", []string{"ganze Zahl"}},
		{"PayVoucher", "value", "12,5", []string{"--value", "Punkt"}},
		{"PayVoucher", "value", "NaN", []string{"--value", "Zahl"}},
		{"PayVoucher", "value", "0x10", []string{"--value"}},
		{"CreateInvoice", "flagged", "vielleicht", []string{"--flagged", "true", "nein"}},
		{"CreateTask", "complete", "maybe", []string{"--complete"}},
		{"CreateInvoice", "document-date", "03.11.2026", []string{"--document-date", "YYYY-MM-DD", "heute"}},
		{"CreateInvoice", "document-date", "2026-02-30", []string{"--document-date"}},
		{"CreateInvoice", "customer", "-1", []string{"--customer", "ID"}},
		{"CreateInvoice", "customer", "abc", []string{"--customer", "ID"}},
		{"CreateProject", "team-member", "x", []string{"--team-member"}},
		{"CreateProject", "attr", "x=1", []string{"--attr", `"x"`, "passt nicht zum Muster ^[0-9]+$"}},
		{"CreateProject", "attr", "12", []string{"--attr", "<id>=<wert>"}},
		{"CreateVoucher", "provenance", "non_eu", []string{"--provenance", `"non_eu"`, `meintest du "foreign"?`, "erlaubt: domestic, eu, foreign"}},
		{"CreateVoucher", "provenance", "domestc", []string{`meintest du "domestic"?`}},
		{"CreateProject", "color", "zzzzzz", []string{"--color", "erlaubt: blue"}},
	}
	for _, tc := range tests {
		t.Run(tc.flag+" "+tc.raw, func(t *testing.T) {
			_, err := coerceFlag(t, tc.op, tc.flag, tc.raw)
			wantValidation(t, err, tc.contains...)
		})
	}
	// A wrong enum value without a close match gets no suggestion.
	_, err := coerceFlag(t, "CreateProject", "color", "zzzzzz")
	if err != nil && strings.Contains(err.Error(), "meintest du") {
		t.Errorf("unexpected suggestion: %v", err)
	}
}

func TestCoerceFlagZeroContext(t *testing.T) {
	f := fieldByFlag(t, mustFields(t, "CreateInvoice"), "document-date")
	before := time.Now().Format("2006-01-02")
	v, err := CoerceFlag(CoerceContext{}, f, "heute")
	after := time.Now().Format("2006-01-02")
	if err != nil || (v != before && v != after) {
		t.Errorf("got %#v, %v; want %s or %s", v, err, before, after)
	}
}

func TestCoerceFlagListKindsNotSupported(t *testing.T) {
	f := fieldByFlag(t, mustFields(t, "CreateInvoice"), "item")
	if _, err := CoerceFlag(testCC(t), f, "name=X"); err == nil {
		t.Error("items must be parsed by the item parser, not CoerceFlag")
	}
}

func TestCoerceValueRecursesAndKeepsNumbers(t *testing.T) {
	in := map[string]any{
		"name":     "R",
		"unknown":  float64(1),
		"customer": map[string]any{"id": "9007199254740993", "project": map[string]any{"id": json.Number("3")}},
		"line_items": []any{
			map[string]any{"quantity": "2", "price": json.Number("19.99"), "vat_rate": "19%", "name": json.Number("5"), "proposition": map[string]any{"id": "4"}},
			map[string]any{"quantity": float64(1.5), "price": "abc"},
		},
		"flagged": "true",
		"gross":   nil,
	}
	snapshot := marshal(t, in)
	got, err := CoerceValue(RequestSchema("CreateInvoice"), in, "")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"customer":{"id":9007199254740993,"project":{"id":3}},"flagged":true,"gross":null,` +
		`"line_items":[{"name":"5","price":19.99,"proposition":{"id":4},"quantity":2,"vat_rate":0.19},` +
		`{"price":"abc","quantity":1.5}],"name":"R","unknown":1}`
	if s := marshal(t, got); s != want {
		t.Errorf("got  %s\nwant %s", s, want)
	}
	if marshal(t, in) != snapshot {
		t.Error("CoerceValue modified its input")
	}
	m := got.(map[string]any)
	if id := m["customer"].(map[string]any)["id"]; id != json.Number("9007199254740993") {
		t.Errorf("big id %#v", id)
	}
	if p := m["line_items"].([]any)[0].(map[string]any)["price"]; p != json.Number("19.99") {
		t.Errorf("price %#v", p)
	}
}

func TestCoerceValueNilSchema(t *testing.T) {
	v := map[string]any{"a": 1}
	got, err := CoerceValue(nil, v, "")
	if err != nil || !reflect.DeepEqual(got, v) {
		t.Errorf("got %#v, %v", got, err)
	}
}

func TestEnumHint(t *testing.T) {
	s := RequestSchema("CreateVoucher").Properties["provenance"].Value
	if v, ok := EnumHint(s, "non_eu"); !ok || v != "foreign" {
		t.Errorf("got %q, %v", v, ok)
	}
	if v, ok := EnumHint(s, "NON_EU"); !ok || v != "foreign" {
		t.Errorf("case-insensitive: got %q, %v", v, ok)
	}
	if _, ok := EnumHint(s, "eu"); ok {
		t.Error("eu has no hint")
	}
	if _, ok := EnumHint(nil, "non_eu"); ok {
		t.Error("nil schema")
	}
}
