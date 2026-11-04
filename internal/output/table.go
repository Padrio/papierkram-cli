package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// maxNestedRunes caps a nested object or array shown as compact JSON in a
// table cell.
const maxNestedRunes = 60

// renderTable writes -o table and -o wide (spec 10.3):
//   - a list envelope ({"type":"list",…,"entries":[…]}) or a top-level
//     array: one row per entry below a header line; an empty list prints
//     only the header and "Keine Einträge." on stderr;
//   - an object: a two-column FELD/WERT table, one row per column;
//   - any other value: one line (null: nothing).
//
// Columns are cols.Table (wide: cols.Table + cols.Wide) or, with --fields,
// one column per field in the given order. Without columns a list shows
// the top-level keys of its entries and an object its top-level keys, in
// API order, leaving out keys with nested objects or arrays.
//
// Columns are aligned by text/tabwriter, which counts runes: umlauts and ß
// are one character. Characters that take two terminal cells (CJK, emoji)
// or combining marks are not accounted for.
func renderTable(o Options, raw []byte, cols Columns) error {
	v, err := decode(raw)
	if err != nil {
		return err
	}
	columns := tableColumns(o, cols)
	var rows [][]string
	entries, isList := tableEntries(v)
	switch obj, isObj := v.(object); {
	case isList:
		if len(columns) == 0 {
			columns = scalarColumns(entries)
		}
		if len(columns) == 0 && len(entries) > 0 {
			columns = []Column{{Header: "WERT"}} // entries are no objects
		}
		if len(columns) > 0 {
			rows = append(rows, headers(columns))
		}
		for _, e := range entries {
			row := make([]string, len(columns))
			for i, c := range columns {
				row[i] = cell(e, c)
			}
			rows = append(rows, row)
		}
	case isObj:
		if len(columns) == 0 {
			columns = scalarColumns([]any{obj})
		}
		rows = append(rows, []string{"FELD", "WERT"})
		for _, c := range columns {
			rows = append(rows, []string{c.header(), cell(obj, c)})
		}
	case v != nil:
		rows = append(rows, []string{cell(v, Column{})})
	}
	if err := writeRows(o.Out, rows); err != nil {
		return err
	}
	if isList && len(entries) == 0 {
		_, _ = io.WriteString(o.Err, "Keine Einträge.\n")
	}
	return nil
}

// tableEntries returns the entries of a list envelope or a top-level array.
func tableEntries(v any) ([]any, bool) {
	switch v := v.(type) {
	case object:
		return listEntries(v)
	case []any:
		return v, true
	}
	return nil, false
}

// tableColumns picks the columns: --fields (a known column with the same
// path keeps its header and formatter, other paths get the upper-cased path
// as header), else Table, for wide followed by Wide.
func tableColumns(o Options, cols Columns) []Column {
	known := slices.Concat(cols.Table, cols.Wide)
	if len(o.Fields) > 0 {
		out := make([]Column, len(o.Fields))
		for i, f := range o.Fields {
			out[i] = Column{Path: f}
			// a fallback column shows more than the path asked for
			if j := slices.IndexFunc(known, func(c Column) bool { return c.Path == f && c.Fallback == "" }); j >= 0 {
				out[i] = known[j]
			}
		}
		return out
	}
	if o.Format == FormatWide {
		return known
	}
	return cols.Table
}

// scalarColumns returns one column per top-level key of the objects in
// entries (order of first appearance); keys that hold an object or array in
// any entry are left out.
func scalarColumns(entries []any) []Column {
	var keys []string
	seen, nested := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		o, ok := e.(object)
		if !ok {
			continue
		}
		for _, m := range o {
			switch m.val.(type) {
			case object, []any:
				nested[m.key] = true
			}
			if !seen[m.key] {
				seen[m.key] = true
				keys = append(keys, m.key)
			}
		}
	}
	var cols []Column
	for _, k := range keys {
		if !nested[k] {
			cols = append(cols, Column{Path: k})
		}
	}
	return cols
}

func headers(cols []Column) []string {
	h := make([]string, len(cols))
	for i, c := range cols {
		h[i] = c.header()
	}
	return h
}

// cell formats the value at c.Path of v (the whole value for an empty
// path), or at c.Fallback when that is null or missing. null and missing
// values are empty; Fmt is only called for other values.
func cell(v any, c Column) string {
	if c.Path != "" {
		x, ok := lookup(v, strings.Split(c.Path, "."))
		if (!ok || x == nil) && c.Fallback != "" {
			x, ok = lookup(v, strings.Split(c.Fallback, "."))
		}
		if !ok {
			return ""
		}
		v = x
	}
	if v == nil {
		return ""
	}
	if c.Fmt != nil {
		return oneLine(c.Fmt(v))
	}
	return oneLine(plain(v))
}

// plain is the default cell text: booleans ja/nein, numbers with their
// API literal, strings unchanged, nested objects and arrays as compact JSON
// (key order kept) cut to maxNestedRunes.
func plain(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case bool:
		if v {
			return "ja"
		}
		return "nein"
	case json.Number:
		return string(v)
	case string:
		return v
	case object, []any:
		b, err := marshalJSON(v)
		if err != nil {
			return ""
		}
		return Truncate(string(b), maxNestedRunes)
	}
	return fmt.Sprint(v)
}

// Truncate cuts s to n runes, the last one being "…" (nested values in
// tables, names in the shell completion).
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// oneLine replaces line breaks, tabs, other control characters and
// invisible format characters with a space; they would break (or
// disguise) the table layout.
func oneLine(s string) string {
	// one implementation with Printable: control and invisible format
	// characters (bidi overrides) become spaces
	return Printable(strings.ReplaceAll(s, "\r\n", "\n"))
}

// writeRows aligns rows with two spaces between columns. Trailing spaces
// of every line are removed: the padding of empty last cells, but also
// trailing spaces inside the last cell's own text. A row whose cells are
// all empty would thereby become a blank line, so its first cell shows "-".
func writeRows(w io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	for _, r := range rows {
		if len(r) > 0 && !slices.ContainsFunc(r, func(c string) bool { return c != "" }) {
			r = slices.Clone(r)
			r[0] = "-"
		}
		_, _ = fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	var out bytes.Buffer
	for line := range strings.Lines(b.String()) {
		out.WriteString(strings.TrimRight(line, " \n"))
		out.WriteByte('\n')
	}
	_, err := w.Write(out.Bytes())
	return err
}
