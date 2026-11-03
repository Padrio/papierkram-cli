package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"

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
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("--item %q: %s", s, msg), itemFormatHint)
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
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("--item %q: %s", s, msg), hint)
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
	var r io.Reader
	switch path {
	case "":
		return nil, clierr.New(clierr.CodeUsage, "--items: Dateiname fehlt", "Datei angeben oder - für die Standardeingabe")
	case "-":
		r = stdin
		if r == nil {
			r = strings.NewReader("")
		}
	default:
		f, err := openItemsFile(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	v, err := spec.DecodeJSON(r)
	if err != nil {
		e := *clierr.From(err)
		e.Message = "--items: " + e.Message
		return nil, &e
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

// openItemsFile opens the --items file with German errors.
func openItemsFile(path string) (*os.File, error) {
	hint := "Pfad prüfen oder - für die Standardeingabe angeben"
	f, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("--items: Datei %q nicht gefunden", path), hint)
	case errors.Is(err, fs.ErrPermission):
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("--items: keine Leseberechtigung für %q", path), hint)
	case err != nil:
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("--items: Datei %q nicht lesbar", path), hint)
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		_ = f.Close()
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("--items: %q ist ein Verzeichnis", path), hint)
	}
	return f, nil
}
