package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// itemAliases are the short forms of line item keys in --item (spec 6.2).
var itemAliases = map[string]string{"qty": "quantity", "vat": "vat_rate"}

// itemFormatHint explains the --item grammar.
const itemFormatHint = `Format: --item key=value,key=value; Werte mit Komma oder "=" in Anführungszeichen, z. B. --item 'name="Beratung, vor Ort",qty=2,price=100,vat=19'; in Anführungszeichen stehen \" für ein Anführungszeichen und \\ für einen Backslash`

// itemPair is one key=value pair of an --item value; col is the 1-based
// rune column of the key.
type itemPair struct {
	key, value string
	col        int
}

// ParseItem parses one --item value into a line item (spec 6.2). Grammar:
// pairs key=value separated by ","; key and value split at the first "=";
// whitespace around keys and unquoted values is trimmed; a value in "…"
// keeps its whitespace and may contain "," and "=", with \" and \\ as the
// only escapes. The short forms qty and vat stand for quantity and vat_rate.
//
// Values stay strings; the builder types them with spec.CoerceValue. When
// item (the line item schema) is not nil, every key must be one of its
// properties. All errors are usage errors (exit 2) naming the 1-based
// character column.
func ParseItem(s string, item *openapi3.Schema) (map[string]any, error) {
	pairs, err := splitItem(s)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(pairs))
	given := make(map[string]string, len(pairs)) // canonical key -> key as typed
	for _, p := range pairs {
		key := p.key
		if full, ok := itemAliases[key]; ok {
			key = full
		}
		if prev, dup := given[key]; dup {
			msg := fmt.Sprintf("Schlüssel %q mehrfach angegeben (Spalte %d)", key, p.col)
			if prev != p.key {
				msg = fmt.Sprintf("Schlüssel %q mehrfach angegeben (als %q und %q, Spalte %d)", key, prev, p.key, p.col)
			}
			return nil, itemErr(s, msg)
		}
		if item != nil && item.Properties[key] == nil {
			return nil, unknownItemKey(s, p, item)
		}
		given[key] = p.key
		out[key] = p.value
	}
	return out, nil
}

// splitItem splits an --item value into its pairs (syntax only).
func splitItem(s string) ([]itemPair, error) {
	if strings.TrimSpace(s) == "" {
		return nil, clierr.New(clierr.CodeUsage, "--item ist leer", itemFormatHint)
	}
	rs := []rune(s)
	n := len(rs)
	var pairs []itemPair
	for i := 0; ; {
		// key: up to "=" (or a "," / the end, which is an error)
		start := i
		for i < n && rs[i] != '=' && rs[i] != ',' {
			i++
		}
		key := strings.TrimSpace(string(rs[start:i]))
		switch {
		case key == "" && i < n && rs[i] == '=':
			return nil, itemErr(s, fmt.Sprintf("leerer Schlüssel vor \"=\" (Spalte %d)", i+1))
		case key == "":
			return nil, itemErr(s, fmt.Sprintf("leerer Eintrag (Spalte %d)", i+1))
		case i == n || rs[i] == ',':
			return nil, itemErr(s, fmt.Sprintf("Schlüssel %q ohne \"=\" (Spalte %d)", key, i+1))
		}
		col := start + 1 + leadingSpace(rs[start:i])
		i++ // "="

		// value: quoted or up to the next ","
		j := i
		for j < n && unicode.IsSpace(rs[j]) {
			j++
		}
		var value string
		if j < n && rs[j] == '"' {
			v, end, err := quotedValue(s, rs, j)
			if err != nil {
				return nil, err
			}
			value, i = v, end
			for i < n && unicode.IsSpace(rs[i]) {
				i++
			}
			if i < n && rs[i] != ',' {
				return nil, itemErr(s, fmt.Sprintf("unerwartetes Zeichen %q nach dem schließenden Anführungszeichen (Spalte %d), erwartet \",\" oder Ende", string(rs[i]), i+1))
			}
		} else {
			for i < n && rs[i] != ',' {
				i++
			}
			value = strings.TrimSpace(string(rs[j:i]))
		}
		pairs = append(pairs, itemPair{key: key, value: value, col: col})
		if i == n {
			return pairs, nil
		}
		i++ // ","
	}
}

// quotedValue reads the quoted value whose opening quote is at rs[q]; it
// returns the unescaped value and the index after the closing quote.
func quotedValue(s string, rs []rune, q int) (string, int, error) {
	var b strings.Builder
	for i := q + 1; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '"':
			return b.String(), i + 1, nil
		case r == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\'):
			b.WriteRune(rs[i+1])
			i++
		default:
			b.WriteRune(r)
		}
	}
	return "", 0, itemErr(s, fmt.Sprintf("nicht geschlossenes Anführungszeichen (Spalte %d)", q+1))
}

// leadingSpace counts the whitespace runes at the start of rs.
func leadingSpace(rs []rune) int {
	n := 0
	for n < len(rs) && unicode.IsSpace(rs[n]) {
		n++
	}
	return n
}

// itemErr is a syntax error in the --item value s.
func itemErr(s, msg string) error {
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("--item %s: %s", showItem(s), msg), itemFormatHint)
}

// showItem renders an --item value for an error message so that the column
// counts (runes) match what is shown: unescaped between ‹ and ›, with each
// C0 control character replaced by one visible symbol (U+2400 block) and
// every other character printable hides (other controls, Unicode Cf such as
// bidi controls and zero width characters) by �, never by an escape
// sequence or a removal that would shift the columns.
func showItem(s string) string {
	return "‹" + strings.Map(func(r rune) rune {
		switch {
		case r < 0x20:
			return 0x2400 + r
		case r == 0x7f:
			return '␡'
		case invisible(r):
			return '�'
		}
		return r
	}, s) + "›"
}

// unknownItemKey reports a key that is no property of the line item schema,
// with a suggestion and the valid keys (sorted, then the usable short forms).
func unknownItemKey(s string, p itemPair, item *openapi3.Schema) error {
	keys := make([]string, 0, len(item.Properties))
	for k := range item.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var aliases, shortForms []string
	for _, a := range []string{"qty", "vat"} {
		if item.Properties[itemAliases[a]] != nil {
			aliases = append(aliases, a)
			shortForms = append(shortForms, a+" → "+itemAliases[a])
		}
	}
	msg := fmt.Sprintf("unbekannter Schlüssel %q (Spalte %d)", p.key, p.col)
	if sug := spec.Suggest(p.key, append(keys, aliases...), 1); len(sug) > 0 {
		msg += fmt.Sprintf(" – meintest du %q?", sug[0])
	}
	hint := "gültige Schlüssel: " + strings.Join(keys, ", ")
	if len(shortForms) > 0 {
		hint += "; Kurzformen: " + strings.Join(shortForms, ", ")
	}
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("--item %s: %s", showItem(s), msg), hint)
}

// itemsHint shows the expected shape of an --items file.
const itemsHint = `--items erwartet ein JSON-Array von Positionen, z. B. [{"name": "Beratung", "quantity": 2, "price": 100, "vat_rate": "19%"}]`

// LoadItems reads the line items of --items: a JSON file, or stdin for "-"
// (spec 6.2). The input is decoded like --input (spec.DecodeJSON: UTF-8 BOM
// removed, CRLF accepted, numbers kept as json.Number) and must be an array
// of objects. Keys and values are checked later by Prune and Validate, like
// --input. A missing or unreadable file is a usage error (exit 2); invalid
// content is a validation error (exit 5), as for --input.
func LoadItems(path string, stdin io.Reader) ([]any, error) {
	v, err := readJSON("--items", path, stdin)
	if err != nil {
		return nil, err
	}
	items, ok := v.([]any)
	if !ok {
		return nil, clierr.New(clierr.CodeValidation, "--items: Eingabe muss ein JSON-Array von Positionen sein", itemsHint)
	}
	for i, it := range items {
		if _, ok := it.(map[string]any); !ok {
			return nil, clierr.New(clierr.CodeValidation, fmt.Sprintf("--items: Position %d ist kein JSON-Objekt", i+1), itemsHint)
		}
	}
	return items, nil
}

// readJSON reads the JSON value of flag (--input, --items): a file, or
// stdin for "-" (a nil stdin reads as empty). It decodes with
// spec.DecodeJSON (BOM removed, CRLF, json.Number). A missing path or an
// unreadable file is a usage error (exit 2), invalid content a validation
// error (exit 5); messages start with the flag.
func readJSON(flag, path string, stdin io.Reader) (any, error) {
	r := stdin
	switch path {
	case "":
		return nil, errMissingPath(flag)
	case "-":
		if r == nil {
			r = strings.NewReader("")
		}
	default:
		f, err := openJSONFile(flag, path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	v, err := spec.DecodeJSON(r)
	if err != nil {
		return nil, prefixErr(flag+": ", err)
	}
	return v, nil
}

// errMissingPath is the usage error of --input/--items without a path.
func errMissingPath(flag string) error {
	return clierr.New(clierr.CodeUsage, flag+": Dateiname fehlt", "Datei angeben oder - für die Standardeingabe")
}

// prefixErr puts prefix (the flag) before the message of err.
func prefixErr(prefix string, err error) error {
	e := *clierr.From(err)
	e.Message = prefix + e.Message
	return &e
}

// openJSONFile opens the JSON file of flag (--input, --items) with German
// errors; all are usage errors (exit 2).
func openJSONFile(flag, path string) (*os.File, error) {
	hint := "Pfad prüfen oder - für die Standardeingabe angeben"
	f, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s: Datei %q nicht gefunden", flag, path), hint)
	case errors.Is(err, fs.ErrPermission):
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s: keine Leseberechtigung für %q", flag, path), hint)
	case err != nil:
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s: Datei %q nicht lesbar", flag, path), hint)
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		_ = f.Close()
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s: %q ist ein Verzeichnis", flag, path), hint)
	}
	return f, nil
}

// lineItems returns the positions of --items (pruned like --input) or
// --item, nil when neither is given, and the removed response-only fields.
func (a *App) lineItems(c *cobra.Command, ws writeSpec, resp *openapi3.Schema) ([]any, []string, error) {
	key := itemsKey(ws)
	if c.Flags().Changed("items") {
		raw, err := LoadItems(flagString(c, "items"), a.stdin())
		if err != nil {
			return nil, nil, err
		}
		pr, err := spec.Prune(map[string]any{key: raw}, ws.req, resp)
		if err != nil {
			return nil, nil, prefixErr("--items: ", err)
		}
		a.pruneHints(pr)
		items, _ := pr.Body[key].([]any)
		if items == nil {
			items = []any{}
		}
		return items, pr.Removed, nil
	}
	if !c.Flags().Changed(ws.items.Flag) {
		return nil, nil, nil
	}
	raws, _ := c.Flags().GetStringArray(ws.items.Flag)
	schema := ws.itemSchema()
	parsed, err := parseItems(raws, schema)
	if err != nil {
		return nil, nil, err
	}
	items := make([]any, len(parsed))
	for i, it := range parsed {
		if err := itemRefs(raws[i], it, schema); err != nil {
			return nil, nil, err
		}
		items[i] = it
	}
	return items, nil, nil
}

// parseItems parses --item values with ParseItem and rejects object fields
// that key=value cannot express (checkItemShape). It needs only the flags,
// so checkWriteFlags runs it before anything is read or sent; all its
// errors are usage errors (exit 2).
func parseItems(raws []string, schema *openapi3.Schema) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(raws))
	for _, s := range raws {
		it, err := ParseItem(s, schema)
		if err != nil {
			return nil, err
		}
		if err := checkItemShape(s, it, schema); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

// itemField is the schema of the line item property key, nil if unknown.
func itemField(schema *openapi3.Schema, key string) *openapi3.Schema {
	if schema == nil {
		return nil
	}
	if ref := schema.Properties[key]; ref != nil {
		return ref.Value
	}
	return nil
}

// checkItemShape rejects --item fields that are objects with more than an
// id, or arrays: they cannot be written as key=value and need --items.
func checkItemShape(s string, it map[string]any, schema *openapi3.Schema) error {
	keys := make([]string, 0, len(it))
	for k := range it {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ps := itemField(schema, k)
		if ps != nil && !spec.IsIDOnly(ps) && (spec.IsObject(ps) || ps.Type.Is("array")) {
			return clierr.New(clierr.CodeUsage,
				fmt.Sprintf("--item %s: Feld %q ist ein Objekt und lässt sich nicht als key=value angeben", showItem(s), k),
				"Positionen mit diesem Feld als JSON per --items angeben")
		}
	}
	return nil
}

// itemRefs turns the reference fields of an --item position, objects with
// only an id, from an integer into {"id": n} (proposition=2); an invalid ID
// is a validation error (exit 5). The keys are checked in sorted order, so
// of several invalid IDs always the same one is reported.
func itemRefs(s string, it map[string]any, schema *openapi3.Schema) error {
	for _, k := range slices.Sorted(maps.Keys(it)) {
		v := it[k]
		ps := itemField(schema, k)
		if ps == nil || !spec.IsIDOnly(ps) {
			continue
		}
		raw, _ := v.(string)
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 0 {
			return clierr.New(clierr.CodeValidation,
				fmt.Sprintf("--item %s: ungültiger Wert %q für %s: ID muss eine ganze Zahl ≥ 0 sein", showItem(s), raw, k), "")
		}
		it[k] = map[string]any{"id": id}
	}
	return nil
}
