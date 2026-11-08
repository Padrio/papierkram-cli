package output

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// Golden files: go test ./internal/output -run TestTable -update

// renderCols runs Render with columns and returns stdout and stderr.
func renderCols(t *testing.T, o Options, raw string, cols Columns) (string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	o.Out, o.Err = &out, &errb
	if err := Render(o, []byte(raw), cols); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String(), errb.String()
}

const tableList = `{"type":"list","page":1,"page_size":3,"total_pages":1,"total_entries":3,"has_more":false,"entries":[
{"id":1,"name":"Müller GmbH","city":"Köln","total_gross":1190.00,"active":true,"billing":{"company":"Größe AG","street":"Hauptstraße 1"},"note":null},
{"id":9007199254740993,"name":"Straße & Co","city":"Düsseldorf","total_gross":19.99,"active":false,"billing":null,"note":"Zeile 1\nZeile 2\tmit Tab"},
{"id":3,"name":"Größe","total_gross":"0.00"}]}`

var tableCols = Columns{
	Table: []Column{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "ORT", Path: "city"},
		{Header: "BRUTTO", Path: "total_gross", Fmt: FmtMoney},
		{Header: "AKTIV", Path: "active"},
	},
	Wide: []Column{
		{Header: "FIRMA", Path: "billing.company"},
		{Path: "note"},
	},
}

func TestTableList(t *testing.T) {
	out, errb := renderCols(t, Options{Format: FormatTable}, tableList, tableCols)
	golden.Assert(t, "table_list.txt", []byte(out))
	if errb != "" {
		t.Errorf("stderr: %q", errb)
	}
}

func TestTableWide(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatWide}, tableList, tableCols)
	golden.Assert(t, "table_wide.txt", []byte(out))
}

// --fields selects the columns in the given order; a known column (same
// path) keeps its header and formatter, other paths get the upper-cased
// path as header.
func TestTableFields(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatWide} {
		out, _ := renderCols(t, Options{Format: f, Fields: []string{"name", "billing.company", "billing.street", "total_gross", "id"}}, tableList, tableCols)
		golden.Assert(t, "table_fields.txt", []byte(out))
	}
}

func TestTableListWithoutColumns(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, tableList, Columns{})
	golden.Assert(t, "table_list_nocols.txt", []byte(out))
}

// Nested objects become compact JSON (key order kept), cut at 60 runes;
// null and missing values are empty cells, booleans ja/nein, line breaks
// and tabs become spaces.
func TestTableCells(t *testing.T) {
	raw := `{"type":"list","entries":[
{"id":1,"flag":true,"nested":{"b":1,"a":"Äpfel"},"long":{"text":"Ein sehr langer Text mit Umlauten: äöüß ÄÖÜ, der abgeschnitten wird"},"arr":[1,2],"n":null,"s":"a\r\nb\tc"},
{"id":2,"flag":false}]}`
	cols := Columns{Table: []Column{{Path: "id"}, {Path: "flag"}, {Path: "nested"}, {Path: "long"}, {Path: "arr"}, {Path: "n"}, {Path: "missing"}, {Path: "s"}}}
	out, _ := renderCols(t, Options{Format: FormatTable}, raw, cols)
	golden.Assert(t, "table_cells.txt", []byte(out))
	if strings.Contains(out, "null") {
		t.Errorf("null in Tabelle:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `{"text"`) {
			cell := line[strings.Index(line, `{"text"`):]
			cell = strings.TrimSpace(cell[:strings.Index(cell, "  ")])
			if n := utf8.RuneCountInString(cell); n != 60 || !strings.HasSuffix(cell, "…") {
				t.Errorf("gekürzte Zelle %q hat %d Runen", cell, n)
			}
		}
	}
}

func TestTableObject(t *testing.T) {
	raw := `{"id":9007199254740993,"name":"Müller GmbH","total_gross":1190.00,"billing":{"company":"Größe AG"},"active":true,"note":null}`
	out, errb := renderCols(t, Options{Format: FormatTable}, raw, tableCols)
	golden.Assert(t, "table_object.txt", []byte(out))
	if errb != "" {
		t.Errorf("stderr: %q", errb)
	}
	out, _ = renderCols(t, Options{Format: FormatWide}, raw, tableCols)
	golden.Assert(t, "table_object_wide.txt", []byte(out))
}

// Without columns an object lists its top-level scalar keys in API order.
func TestTableObjectWithoutColumns(t *testing.T) {
	raw := `{"id":1,"name":"Straße","billing":{"company":"X"},"tags":["a"],"vat_rate":0.19,"archived":false,"note":null}`
	out, _ := renderCols(t, Options{Format: FormatTable}, raw, Columns{})
	golden.Assert(t, "table_object_nocols.txt", []byte(out))
}

func TestTableObjectFields(t *testing.T) {
	raw := `{"id":1,"name":"Straße","billing":{"company":"Größe AG"}}`
	out, _ := renderCols(t, Options{Format: FormatTable, Fields: []string{"billing.company", "id"}}, raw, Columns{})
	if want := "FELD             WERT\nBILLING.COMPANY  Größe AG\nID               1\n"; out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

func TestTableEmptyList(t *testing.T) {
	out, errb := renderCols(t, Options{Format: FormatTable}, `{"type":"list","page":1,"total_entries":0,"has_more":false,"entries":[]}`, tableCols)
	golden.Assert(t, "table_empty.txt", []byte(out))
	if errb != "Keine Einträge.\n" {
		t.Errorf("stderr: %q", errb)
	}
	// Without columns there is no header to print.
	out, errb = renderCols(t, Options{Format: FormatTable}, `[]`, Columns{})
	if out != "" || errb != "Keine Einträge.\n" {
		t.Errorf("out %q, stderr %q", out, errb)
	}
}

// A top-level array (e.g. context list) is rendered like a list envelope.
func TestTableTopLevelArray(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, `[{"name":"a","read_only":false},{"name":"b","read_only":true}]`, Columns{})
	if want := "NAME  READ_ONLY\na     nein\nb     ja\n"; out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// Columns are aligned by runes: umlauts and ß count as one character.
func TestTableUmlautAlignment(t *testing.T) {
	raw := `[{"name":"Müller","x":"1"},{"name":"Größe","x":"2"},{"name":"Straße","x":"3"},{"name":"Ab","x":"4"},{"name":"ÄÖÜäöüß","x":"5"}]`
	out, _ := renderCols(t, Options{Format: FormatTable}, raw, Columns{Table: []Column{{Path: "name"}, {Path: "x"}}})
	golden.Assert(t, "table_umlauts.txt", []byte(out))
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	want := runeIndex(lines[0], "X")
	for _, l := range lines[1:] {
		if got := runeIndex(l, string(l[len(l)-1])); got != want {
			t.Errorf("Spalte 2 in %q bei Rune %d, Kopf bei %d", l, got, want)
		}
	}
}

func runeIndex(s, sub string) int {
	return utf8.RuneCountInString(s[:strings.LastIndex(s, sub)])
}

func TestTableNoTrailingSpaces(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatWide}, tableList, tableCols)
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimRight(l, " ") != l {
			t.Errorf("Leerzeichen am Zeilenende: %q", l)
		}
	}
}

// Time entries as in the spec examples (/tracker/time_entries): duration
// in seconds → h:mm, date-times in their API offset → YYYY-MM-DD HH:MM.
func TestTableTimeEntries(t *testing.T) {
	raw := `{"type":"list","entries":[
{"id":1,"started_at":"2026-08-28T13:44:00.000+02:00","ended_at":"2026-08-28T16:44:00.000+02:00","duration":10800,"comments":"Größe prüfen"},
{"id":2,"started_at":"2020-05-11T10:20:00.000+02:00","ended_at":"2020-05-11T12:00:00.000+02:00","duration":6000,"comments":null},
{"id":3,"started_at":null,"ended_at":null,"duration":null}]}`
	cols := Columns{Table: []Column{
		{Header: "ID", Path: "id"},
		{Header: "BEGINN", Path: "started_at", Fmt: FmtDate},
		{Header: "ENDE", Path: "ended_at", Fmt: FmtDate},
		{Header: "DAUER", Path: "duration", Fmt: FmtDurationHM},
		{Header: "KOMMENTAR", Path: "comments"},
	}}
	out, _ := renderCols(t, Options{Format: FormatTable}, raw, cols)
	golden.Assert(t, "table_time_entries.txt", []byte(out))
}

func TestTableScalar(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, `"Zeile 1\nZeile 2"`, Columns{})
	if out != "Zeile 1 Zeile 2\n" {
		t.Errorf("got %q", out)
	}
}

// A row whose cells are all empty would be trimmed to a blank line; it
// shows "-" in the first cell instead.
func TestTableAllEmptyRowStaysVisible(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, `[{"a":"x","b":"y"},{},{"a":null}]`, Columns{Table: []Column{{Path: "a"}, {Path: "b"}}})
	if want := "A  B\nx  y\n-\n-\n"; out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
}

// Headers are single-line like cells.
func TestTableHeaderOneLine(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, `[{"a":"x","b":"y"}]`, Columns{Table: []Column{{Header: "A\nZ", Path: "a"}, {Header: "B\tY", Path: "b"}}})
	if want := "A Z  B Y\nx    y\n"; out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
	out, _ = renderCols(t, Options{Format: FormatTable}, `{"a":"x"}`, Columns{Table: []Column{{Header: "A\nZ", Path: "a"}}})
	if want := "FELD  WERT\nA Z   x\n"; out != want {
		t.Errorf("object: got\n%q\nwant\n%q", out, want)
	}
}

// A column with Fallback shows the value at Fallback when the value at
// Path is null or missing (company: customer_no, else supplier_no, spec
// 10.3). --fields names plain paths: a fallback column is not reused.
func TestTableColumnFallback(t *testing.T) {
	raw := `{"type":"list","entries":[
{"id":1,"customer_no":"K-1","supplier_no":"L-9"},
{"id":2,"customer_no":null,"supplier_no":"L-2"},
{"id":3,"supplier_no":"L-3"},
{"id":4,"customer_no":null,"supplier_no":null}]}`
	cols := Columns{Table: []Column{{Header: "ID", Path: "id"}, {Header: "NR.", Path: "customer_no", Fallback: "supplier_no"}}}
	out, _ := renderCols(t, Options{Format: FormatTable}, raw, cols)
	want := "ID  NR.\n1   K-1\n2   L-2\n3   L-3\n4\n"
	if out != want {
		t.Errorf("table = %q, erwartet %q", out, want)
	}
	out, _ = renderCols(t, Options{Format: FormatTable, Fields: []string{"customer_no"}}, raw, cols)
	if want := "CUSTOMER_NO\nK-1\n-\n-\n-\n"; out != want { // all-empty rows show "-"
		t.Errorf("--fields = %q, erwartet %q", out, want)
	}
}

// A cell of white space only (a name " ", a comment "\n") is an empty
// cell: trimmed, a row of such cells would become a blank line (L158).
func TestTableWhitespaceCellIsEmpty(t *testing.T) {
	out, _ := renderCols(t, Options{Format: FormatTable}, `[{"a":"x","b":"y"},{"a":" ","b":"\n\t"},{"a":"z","b":"  "},{"a":"      ","b":"w"}]`, Columns{Table: []Column{{Path: "a"}, {Path: "b"}}})
	// the blank cell does not widen its column either
	if want := "A  B\nx  y\n-\nz\n   w\n"; out != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
}
