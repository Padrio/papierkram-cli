package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// render runs Render with fresh buffers and returns stdout, stderr and the
// error.
func render(t *testing.T, o Options, raw string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	o.Out, o.Err = &out, &errb
	err := Render(o, []byte(raw), Columns{})
	return out.String(), errb.String(), err
}

func mustRender(t *testing.T, o Options, raw string) string {
	t.Helper()
	out, _, err := render(t, o, raw)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

const precise = `{"id":9007199254740993,"price":19.99,"total_gross":1190.00,"name":"Müller & Söhne <GmbH>"}`

// Review Focus 2: decimals and IDs beyond 2^53 stay byte-exact in JSON.
func TestJSONKeepsNumbersExact(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON}, precise)
	want := "{\n  \"id\": 9007199254740993,\n  \"price\": 19.99,\n  \"total_gross\": 1190.00,\n  \"name\": \"Müller & Söhne <GmbH>\"\n}\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestJSONFieldsKeepNumbersExact(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"total_gross", "id", "name"}}, precise)
	want := "{\n  \"total_gross\": 1190.00,\n  \"id\": 9007199254740993,\n  \"name\": \"Müller & Söhne <GmbH>\"\n}\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

const list = `{"type":"list","page":1,"page_size":2,"total_pages":3,"total_entries":5,"has_more":true,"entries":[
{"id":1,"name":"A","billing":{"company":"Müller GmbH","city":"Köln"},"total_gross":1190.00},
{"id":9007199254740993,"name":"B","billing":null},
{"id":3,"name":"C"}]}`

func TestFieldsOnListEnvelope(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"id", "billing.company"}}, list)
	want := `{
  "type": "list",
  "page": 1,
  "page_size": 2,
  "total_pages": 3,
  "total_entries": 5,
  "has_more": true,
  "entries": [
    {
      "id": 1,
      "billing": {
        "company": "Müller GmbH"
      }
    },
    {
      "id": 9007199254740993
    },
    {
      "id": 3
    }
  ]
}
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFieldsNestedObject(t *testing.T) {
	raw := `{"id":7,"billing":{"company":"X","city":"Y","zip":"1"},"state":"draft","note":null}`
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"billing.city", "id", "billing.company", "missing", "note", "state.x"}}, raw)
	// Order follows --fields; parents are created on first use; missing
	// paths are omitted; null values are kept.
	want := "{\n  \"billing\": {\n    \"city\": \"Y\",\n    \"company\": \"X\"\n  },\n  \"id\": 7,\n  \"note\": null\n}\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFieldsWholeAndPartialParentDoNotMutateInput(t *testing.T) {
	raw := `{"billing":{"company":"X","city":"Y"}}`
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"billing", "billing.company"}}, raw)
	want := "{\n  \"billing\": {\n    \"company\": \"X\",\n    \"city\": \"Y\"\n  }\n}\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFieldsOnTopLevelArray(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"id"}}, `[{"id":1,"x":2},{"id":2}]`)
	want := "[\n  {\n    \"id\": 1\n  },\n  {\n    \"id\": 2\n  }\n]\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestEntriesWithoutListTypeAreAnObjectField(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"id"}}, `{"id":1,"entries":[{"id":2}]}`)
	if got != "{\n  \"id\": 1\n}\n" {
		t.Errorf("got\n%s", got)
	}
}

func TestParseFields(t *testing.T) {
	got := ParseFields(" id, billing.company ,,")
	if len(got) != 2 || got[0] != "id" || got[1] != "billing.company" {
		t.Errorf("ParseFields = %q", got)
	}
	if ParseFields("") != nil {
		t.Error("empty must be nil")
	}
}

func TestYAMLKeepsKeyOrderAndNumbers(t *testing.T) {
	raw := `{"z":1,"id":9007199254740993,"price":19.99,"total_gross":1190.00,"a":{"y":true,"b":null},"m":[1,"x"],"empty":{},"none":[],"zip":"0815","text":"Zeile 1\nZeile 2"}`
	got := mustRender(t, Options{Format: FormatYAML}, raw)
	want := `z: 1
id: 9007199254740993
price: 19.99
total_gross: 1190.00
a:
  "y": true
  b: null
m:
- 1
- x
empty: {}
none: []
zip: "0815"
text: |-
  Zeile 1
  Zeile 2
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestYAMLListWithFields(t *testing.T) {
	got := mustRender(t, Options{Format: FormatYAML, Fields: []string{"id", "billing.company"}}, list)
	want := `type: list
page: 1
page_size: 2
total_pages: 3
total_entries: 5
has_more: true
entries:
- id: 1
  billing:
    company: Müller GmbH
- id: 9007199254740993
- id: 3
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Table rendering arrives with Task 17; until then table/wide fall back to
// indented JSON.
func TestTableFallsBackToJSON(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatWide} {
		if got := mustRender(t, Options{Format: f}, `{"id":1}`); got != "{\n  \"id\": 1\n}\n" {
			t.Errorf("%s: %q", f, got)
		}
	}
}

func TestEmptyBodyPrintsNothing(t *testing.T) {
	for _, o := range []Options{{Format: FormatJSON}, {Format: FormatYAML}, {Format: FormatJSON, JQ: "."}, {Format: FormatJSON, Fields: []string{"id"}}} {
		if got := mustRender(t, o, " \n"); got != "" {
			t.Errorf("%+v: %q", o, got)
		}
	}
}

func TestInvalidJSONIsAnError(t *testing.T) {
	for _, o := range []Options{{Format: FormatJSON}, {Format: FormatYAML}, {Format: FormatJSON, Fields: []string{"id"}}, {Format: FormatJSON, JQ: "."}} {
		out, _, err := render(t, o, `{"id":1`)
		if err == nil || out != "" {
			t.Errorf("%+v: err = %v, out = %q", o, err, out)
			continue
		}
		if ce := clierr.From(err); ce.Code != clierr.CodeError || !strings.Contains(ce.Message, "kein gültiges JSON") {
			t.Errorf("%+v: %+v", o, ce)
		}
	}
}

func TestUnknownFormatIsUsageError(t *testing.T) {
	_, _, err := render(t, Options{Format: "xml"}, `{}`)
	if ce := clierr.From(err); ce == nil || ce.Exit() != 2 {
		t.Errorf("err = %v", err)
	}
}

func TestRenderValue(t *testing.T) {
	var out bytes.Buffer
	v := struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}{9007199254740993, "a & b"}
	if err := RenderValue(Options{Format: FormatYAML, Out: &out}, v, Columns{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "id: 9007199254740993\nname: a & b\n" {
		t.Errorf("yaml = %q", got)
	}
	out.Reset()
	if err := RenderValue(Options{Format: FormatJSON, Fields: []string{"name"}, Out: &out}, v, Columns{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "{\n  \"name\": \"a & b\"\n}\n" {
		t.Errorf("json = %q", got)
	}
}

func TestStructured(t *testing.T) {
	cases := []struct {
		o    Options
		want bool
	}{
		{Options{Format: FormatTable}, false},
		{Options{Format: FormatWide}, false},
		{Options{Format: FormatJSON}, true},
		{Options{Format: FormatYAML}, true},
		{Options{Format: FormatTable, JQ: ".id"}, true},
	}
	for _, c := range cases {
		if got := c.o.Structured(); got != c.want {
			t.Errorf("%+v: %v", c.o, got)
		}
	}
}

func TestUseColor(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		name    string
		tty     bool
		env     map[string]string
		noColor bool
		want    bool
	}{
		{"tty", true, nil, false, true},
		{"no tty", false, nil, false, false},
		{"NO_COLOR", true, map[string]string{"NO_COLOR": "1"}, false, false},
		{"NO_COLOR any value", true, map[string]string{"NO_COLOR": "false"}, false, false},
		{"--no-color", true, nil, true, false},
	}
	for _, c := range cases {
		if got := UseColor(c.tty, env(c.env), c.noColor); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	if !UseColor(true, nil, false) {
		t.Error("nil env must not panic or disable color")
	}
}

func TestWarn(t *testing.T) {
	var b bytes.Buffer
	Warn(&b, "Restkontingent niedrig")
	if b.String() != "Warnung: Restkontingent niedrig\n" {
		t.Errorf("%q", b.String())
	}
}

// Strings that a YAML reader could take for numbers, booleans or null stay
// strings (zip codes, invoice numbers, times).
func TestYAMLQuotesAmbiguousStrings(t *testing.T) {
	values := []string{"0<8>&15", "0815", "01067", "1e5", "-1", ".5", "12:30", "1_000", "0x1F", "true", "no", "null", "~", "", "2026-11-03", "1. Mahnung"}
	var raw strings.Builder
	raw.WriteString("{")
	for i, v := range values {
		if i > 0 {
			raw.WriteString(",")
		}
		fmt.Fprintf(&raw, "%q:%q", fmt.Sprint("k", i), v)
	}
	raw.WriteString("}")
	got := mustRender(t, Options{Format: FormatYAML}, raw.String())
	var back map[string]any
	if err := yaml.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	for i, v := range values {
		if s, ok := back[fmt.Sprint("k", i)].(string); !ok || s != v {
			t.Errorf("%q kam als %#v zurück:\n%s", v, back[fmt.Sprint("k", i)], got)
		}
	}
	for _, want := range []string{`k1: "0815"`, `k0: "0<8>&15"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s fehlt (gequotet, ohne HTML-Escaping):\n%s", want, got)
		}
	}
}
