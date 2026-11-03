package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// itemSchema is the line item schema of a request body.
func itemSchema(t *testing.T, opID string) *openapi3.Schema {
	t.Helper()
	req := spec.RequestSchema(opID)
	if req == nil || req.Properties["line_items"] == nil {
		t.Fatalf("%s: no line_items in request schema", opID)
	}
	return req.Properties["line_items"].Value.Items.Value
}

// cliErr asserts err is a clierr with the given code and returns it.
func cliErr(t *testing.T, err error, code clierr.Code) *clierr.Error {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("want %s error, got %#v", code, err)
	}
	return ce
}

func TestParseItemBriefExample(t *testing.T) {
	got, err := ParseItem(`name="Beratung, vor Ort",qty=2,price=100,vat=19`, itemSchema(t, "CreateInvoice"))
	if err != nil {
		t.Fatalf("ParseItem: %v", err)
	}
	want := map[string]any{"name": "Beratung, vor Ort", "quantity": "2", "price": "100", "vat_rate": "19"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestParseItemGrammar(t *testing.T) {
	tests := []struct {
		in   string
		want map[string]any
	}{
		// escapes inside quotes
		{`name="Er sagte \"Hallo\""`, map[string]any{"name": `Er sagte "Hallo"`}},
		{`name="C:\\temp\\x"`, map[string]any{"name": `C:\temp\x`}},
		{`name="a\\"`, map[string]any{"name": `a\`}},
		{`name="a\",b",qty=1`, map[string]any{"name": `a",b`, "quantity": "1"}},
		{`name="a\\",qty=1`, map[string]any{"name": `a\`, "quantity": "1"}},
		// other backslashes stay as they are, also outside quotes
		{`name="a\nb"`, map[string]any{"name": `a\nb`}},
		{`name=C:\temp`, map[string]any{"name": `C:\temp`}},
		// quoted values keep whitespace and may hold , and =
		{`name="  a=b, c  "`, map[string]any{"name": "  a=b, c  "}},
		{`name=""`, map[string]any{"name": ""}},
		// empty values
		{`unit=`, map[string]any{"unit": ""}},
		{`unit=,name=x`, map[string]any{"unit": "", "name": "x"}},
		// whitespace around keys, "=" and unquoted values is trimmed
		{` name = Beratung vor Ort , qty = 2 `, map[string]any{"name": "Beratung vor Ort", "quantity": "2"}},
		{`name = "x" , unit=h`, map[string]any{"name": "x", "unit": "h"}},
		// split at the first "="
		{`description=a=b`, map[string]any{"description": "a=b"}},
		{`description==`, map[string]any{"description": "="}},
		// unicode
		{`name=Größe,description="Müller 😀, ß"`, map[string]any{"name": "Größe", "description": "Müller 😀, ß"}},
		// a quote inside an unquoted value is literal
		{`name=5" Zoll`, map[string]any{"name": `5" Zoll`}},
		// aliases
		{`vat=7%,qty=1.5`, map[string]any{"vat_rate": "7%", "quantity": "1.5"}},
		{`vat_rate=0.19,quantity=1`, map[string]any{"vat_rate": "0.19", "quantity": "1"}},
	}
	item := itemSchema(t, "CreateInvoice")
	for _, tt := range tests {
		got, err := ParseItem(tt.in, item)
		if err != nil {
			t.Errorf("ParseItem(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseItem(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestParseItemManyPairs(t *testing.T) {
	item := itemSchema(t, "CreateInvoice")
	var keys []string
	for k := range item.Properties {
		// proposition is an object ({id}); its value is a reference, not a
		// plain string, and is typed by the builder (Task 23). Every other
		// property is a scalar and gets a distinct string value here.
		if k != "proposition" {
			keys = append(keys, k)
		}
	}
	var parts []string
	want := map[string]any{}
	for i, k := range keys {
		v := strings.Repeat("x", i)
		parts = append(parts, k+"="+v)
		want[k] = v
	}
	got, err := ParseItem(strings.Join(parts, ","), item)
	if err != nil {
		t.Fatalf("ParseItem: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// end marks a column one past the last character of the shown input.
const end = "<end>"

// checkColumn asserts that msg shows the input between ‹ and › and that its
// "Spalte n" points at the character at (or at end for the position after
// the last character) in the shown text.
func checkColumn(t *testing.T, msg, at string) {
	t.Helper()
	open, cl := strings.Index(msg, "‹"), strings.Index(msg, "›")
	if open < 0 || cl < open {
		t.Errorf("message %q does not show the input in ‹…›", msg)
		return
	}
	shown := []rune(msg[open+len("‹") : cl])
	var col int
	if i := strings.LastIndex(msg, "Spalte "); i < 0 {
		t.Errorf("message %q has no column", msg)
		return
	} else if _, err := fmt.Sscanf(msg[i:], "Spalte %d", &col); err != nil {
		t.Errorf("message %q: column: %v", msg, err)
		return
	}
	got := end
	if col >= 1 && col <= len(shown) {
		got = string(shown[col-1])
	} else if col != len(shown)+1 {
		got = fmt.Sprintf("column %d outside %q", col, string(shown))
	}
	if got != at {
		t.Errorf("message %q: Spalte %d points at %q in %q, want %q", msg, col, got, string(shown), at)
	}
}

func TestParseItemErrors(t *testing.T) {
	tests := []struct {
		in   string
		want []string // substrings of the message
		at   string   // character at the reported column; "" = no column
	}{
		{``, []string{"--item", "leer"}, ""},
		{`   `, []string{"--item", "leer"}, ""},
		{`name="Beratung, vor Ort`, []string{`--item ‹name="Beratung, vor Ort›`, "nicht geschlossenes Anführungszeichen", "Spalte 6"}, `"`},
		{`name="a\"`, []string{`--item ‹name="a\"›`, "nicht geschlossenes Anführungszeichen", "Spalte 6"}, `"`},
		{`name="Größe`, []string{"nicht geschlossenes Anführungszeichen", "Spalte 6"}, `"`},
		{`qty=1,größe="x`, []string{"nicht geschlossenes Anführungszeichen", "Spalte 13"}, `"`},
		{`name="a"b`, []string{`unerwartetes Zeichen "b" nach dem schließenden Anführungszeichen`, "Spalte 9"}, "b"},
		{`name="a" "b"`, []string{`--item ‹name="a" "b"›`, `unerwartetes Zeichen "\""`, "Spalte 10"}, `"`},
		{`name`, []string{`Schlüssel "name" ohne "="`, "Spalte 5"}, end},
		{`name=x,qty`, []string{`Schlüssel "qty" ohne "="`, "Spalte 11"}, end},
		{`name=x,qty,price=1`, []string{`Schlüssel "qty" ohne "="`, "Spalte 11"}, ","},
		{`=x`, []string{`leerer Schlüssel`, "Spalte 1"}, "="},
		{`name=x, =y`, []string{`leerer Schlüssel`, "Spalte 9"}, "="},
		{`name=x,`, []string{`leerer Eintrag`, "Spalte 8"}, end},
		{`name=x,,qty=1`, []string{`leerer Eintrag`, "Spalte 8"}, ","},
		{`,name=x`, []string{`leerer Eintrag`, "Spalte 1"}, ","},
		{`name=a,name=b`, []string{`Schlüssel "name" mehrfach angegeben`, "Spalte 8"}, "n"},
		{`qty=1,quantity=2`, []string{`Schlüssel "quantity" mehrfach angegeben`, `"qty"`, "Spalte 7"}, "q"},
		{`vat=19,vat_rate=0.19`, []string{`Schlüssel "vat_rate" mehrfach angegeben`, `"vat"`, "Spalte 8"}, "v"},
		// control characters are shown as one symbol each, so columns stay right
		{"\x01=x,name=\"a", []string{"--item ‹␁=x,name=\"a›", "Spalte 10"}, `"`},
		{"name=\"a\tb\"c", []string{"--item ‹name=\"a␉b\"c›", "Spalte 11"}, "c"},
	}
	item := itemSchema(t, "CreateInvoice")
	for _, tt := range tests {
		_, err := ParseItem(tt.in, item)
		if err == nil {
			t.Errorf("ParseItem(%q): no error", tt.in)
			continue
		}
		ce := cliErr(t, err, clierr.CodeUsage)
		if ce.Exit() != 2 {
			t.Errorf("ParseItem(%q): exit %d, want 2", tt.in, ce.Exit())
		}
		for _, w := range tt.want {
			if !strings.Contains(ce.Message, w) {
				t.Errorf("ParseItem(%q): message %q lacks %q", tt.in, ce.Message, w)
			}
		}
		if tt.at != "" {
			checkColumn(t, ce.Message, tt.at)
		}
		if !strings.Contains(ce.Hint, "--item") {
			t.Errorf("ParseItem(%q): hint %q lacks the format", tt.in, ce.Hint)
		}
	}
}

func TestParseItemUnknownKey(t *testing.T) {
	_, err := ParseItem(`nme=Beratung,qty=1`, itemSchema(t, "CreateInvoice"))
	ce := cliErr(t, err, clierr.CodeUsage)
	for _, w := range []string{`--item`, `unbekannter Schlüssel "nme"`, "Spalte 1", `meintest du "name"?`} {
		if !strings.Contains(ce.Message, w) {
			t.Errorf("message %q lacks %q", ce.Message, w)
		}
	}
	checkColumn(t, ce.Message, "n")
	wantHint := "gültige Schlüssel: description, discount_calculated, discount_calculated_gross, discount_percentage, name, price, price_gross, proposition, quantity, start_new_item_group, type_of, unit, vat_rate; Kurzformen: qty → quantity, vat → vat_rate"
	if ce.Hint != wantHint {
		t.Errorf("hint = %q, want %q", ce.Hint, wantHint)
	}

	// Vouchers have no quantity: the alias is unknown there and only vat is
	// offered as a short form.
	_, err = ParseItem(`name=Papier,qty=1`, itemSchema(t, "CreateVoucher"))
	ce = cliErr(t, err, clierr.CodeUsage)
	if !strings.Contains(ce.Message, `unbekannter Schlüssel "qty"`) || !strings.Contains(ce.Message, "Spalte 13") {
		t.Errorf("voucher qty: %q", ce.Message)
	}
	checkColumn(t, ce.Message, "q")
	if want := "gültige Schlüssel: amount, billing, category, depreciation, name, vat_rate; Kurzformen: vat → vat_rate"; ce.Hint != want {
		t.Errorf("voucher hint = %q, want %q", ce.Hint, want)
	}

	// Nonsense yields no suggestion; keys are case-sensitive.
	_, err = ParseItem(`xyzzy=1`, itemSchema(t, "CreateInvoice"))
	if ce = cliErr(t, err, clierr.CodeUsage); strings.Contains(ce.Message, "meintest") {
		t.Errorf("nonsense key: %q", ce.Message)
	}
	_, err = ParseItem(`Name=x`, itemSchema(t, "CreateInvoice"))
	if ce = cliErr(t, err, clierr.CodeUsage); !strings.Contains(ce.Message, `meintest du "name"?`) {
		t.Errorf("Name: %q", ce.Message)
	}

	// Voucher keys and categories pass unchecked (Validate checks values).
	got, err := ParseItem(`name=Papier,amount=11.90,vat=19%,category=Bürobedarff`, itemSchema(t, "UpdateVoucher"))
	if err != nil {
		t.Fatalf("voucher: %v", err)
	}
	if got["category"] != "Bürobedarff" || got["vat_rate"] != "19%" {
		t.Errorf("voucher: %#v", got)
	}
}

func TestParseItemNilSchemaSkipsKeyCheck(t *testing.T) {
	got, err := ParseItem(`foo=1,qty=2`, nil)
	if err != nil {
		t.Fatalf("ParseItem: %v", err)
	}
	if want := (map[string]any{"foo": "1", "quantity": "2"}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if _, err := ParseItem(`foo=1,foo=2`, nil); err == nil {
		t.Error("duplicate without schema: no error")
	}
}

func TestLoadItemsStdin(t *testing.T) {
	in := strings.NewReader(`[{"name": "Beratung", "quantity": 1.25, "price": 19.99, "vat_rate": "19%"}, {"name": "Größe 😀"}]`)
	got, err := LoadItems("-", in)
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}
	want := []any{
		map[string]any{"name": "Beratung", "quantity": json.Number("1.25"), "price": json.Number("19.99"), "vat_rate": "19%"},
		map[string]any{"name": "Größe 😀"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	// An empty array is a valid (replacing) item list.
	got, err = LoadItems("-", strings.NewReader("[]"))
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty array: %#v, %v", got, err)
	}
}

func TestLoadItemsFileWithBOMAndCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	data := "\xEF\xBB\xBF[\r\n  {\r\n    \"name\": \"Beratung\",\r\n    \"description\": \"Zeile 1\\r\\nZeile 2\",\r\n    \"price\": 1190.00\r\n  }\r\n]\r\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadItems(path, strings.NewReader("not used"))
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}
	want := []any{map[string]any{"name": "Beratung", "description": "Zeile 1\r\nZeile 2", "price": json.Number("1190.00")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestLoadItemsErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		path, stdin string
		code        clierr.Code
		want        []string
	}{
		{write("obj.json", `{"name": "x"}`), "", clierr.CodeValidation, []string{"--items", "muss ein JSON-Array von Positionen sein"}},
		{"-", `"x"`, clierr.CodeValidation, []string{"--items", "muss ein JSON-Array von Positionen sein"}},
		{"-", `[{"name": "a"}, 3]`, clierr.CodeValidation, []string{"--items", "Position 2 ist kein JSON-Objekt"}},
		{"-", `[{"name": "a",}]`, clierr.CodeValidation, []string{"--items", "ungültiges JSON in Zeile 1, Spalte 15"}},
		{"-", "", clierr.CodeValidation, []string{"--items", "leer"}},
		{write("cp1252.json", "[{\"name\": \"M\xfcller\"}]"), "", clierr.CodeValidation, []string{"--items", "nicht UTF-8-kodiert"}},
		{filepath.Join(dir, "fehlt.json"), "", clierr.CodeUsage, []string{"--items", "fehlt.json", "nicht gefunden"}},
		{dir, "", clierr.CodeUsage, []string{"--items", "ist ein Verzeichnis"}},
		{"", "", clierr.CodeUsage, []string{"--items", "Dateiname fehlt"}},
	}
	for _, tt := range tests {
		_, err := LoadItems(tt.path, strings.NewReader(tt.stdin))
		if err == nil {
			t.Errorf("LoadItems(%q, %q): no error", tt.path, tt.stdin)
			continue
		}
		ce := cliErr(t, err, tt.code)
		for _, w := range tt.want {
			if !strings.Contains(ce.Message, w) {
				t.Errorf("LoadItems(%q, %q): message %q lacks %q", tt.path, tt.stdin, ce.Message, w)
			}
		}
		if ce.Hint == "" {
			t.Errorf("LoadItems(%q, %q): no hint", tt.path, tt.stdin)
		}
	}
	// A nil stdin reads as empty input.
	_, err := LoadItems("-", nil)
	if ce := cliErr(t, err, clierr.CodeValidation); !strings.Contains(ce.Message, "--items") || !strings.Contains(ce.Message, "leer") {
		t.Errorf("nil stdin: message %q lacks --items/leer", ce.Message)
	}
}
