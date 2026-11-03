package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Format hints of Validate, collected into the error hint.
const (
	dateFormatHint = "Datum im Format YYYY-MM-DD angeben"
	vatFormatHint  = "Steuersatz als Dezimalbruch angeben, z. B. 0.19"
	bigEnumHint    = "der Wert muss exakt einem Eintrag entsprechen; alle Werte zeigt die Completion (Tab)"
	keyPatternHint = "Schlüssel ist die ID der Attributdefinition; gültige IDs zeigt `papierkram custom-attribute list`"
)

// problem is one validation finding: path (for ordering), message line and
// an optional hint.
type problem struct {
	path, line, hint string
}

// Validate checks a finished body against the request schema before it is
// sent (spec 6.4): kin-openapi VisitJSON with all errors collected (types,
// enums, required fields, additionalProperties, date formats), plus the rules
// OpenAPI 3.0 cannot express:
//   - x-cli-key-pattern on free-form objects (custom_attributes keys);
//   - line_items that the schema requires (create of invoice, estimate,
//     voucher) must hold at least one position; the API rejects an empty or
//     null list (spec 6.2, R17). On update the list is optional and may be
//     empty (R18).
//
// All findings come back as one clierr validation error (exit 5): one German
// line per problem with the field path (line_items[0].category), sorted by
// path; the hint aggregates the format tips. Enum errors suggest values
// (x-cli-enum-hints first, then the closest values); enums with more than 20
// values (the expense categories) always name the three closest values and
// never list them all. json.Number values validate as numbers; the body is
// not modified.
func Validate(body any, req *openapi3.Schema) error {
	if req == nil {
		return clierr.New(clierr.CodeError, "Validierung ohne Request-Schema nicht möglich", "")
	}
	err := req.VisitJSON(body,
		openapi3.MultiErrors(),
		openapi3.VisitAsRequest(),
		openapi3.WithStringFormatValidator("date", openapi3.NewCallbackValidator(func(s string) error {
			_, err := time.Parse(dateLayout, s)
			return err
		})),
	)
	var ps []problem
	if err != nil {
		ps = append(ps, convertErrors(body, err)...)
	}
	kp, err := checkKeyPatterns(req, body, nil, body)
	if err != nil {
		return err
	}
	ps = append(ps, kp...)
	ps = append(ps, checkLineItems(req, body)...)
	if len(ps) == 0 {
		return nil
	}
	return problemsError(ps)
}

func problemsError(ps []problem) *clierr.Error {
	slices.SortStableFunc(ps, func(a, b problem) int { return strings.Compare(a.path, b.path) })
	var lines, hints []string
	for _, p := range ps {
		if !slices.Contains(lines, p.line) {
			lines = append(lines, p.line)
		}
		if p.hint != "" && !slices.Contains(hints, p.hint) {
			hints = append(hints, p.hint)
		}
	}
	return clierr.New(clierr.CodeValidation, strings.Join(lines, "\n"), strings.Join(hints, "; "))
}

// convertErrors turns kin-openapi errors into German problems.
func convertErrors(body any, err error) []problem {
	if me, ok := err.(openapi3.MultiError); ok {
		var out []problem
		for _, e := range me {
			out = append(out, convertErrors(body, e)...)
		}
		return out
	}
	var se *openapi3.SchemaError
	if !errors.As(err, &se) {
		return []problem{{line: "ungültiger Body"}}
	}
	return []problem{convertSchemaError(body, se)}
}

func convertSchemaError(body any, se *openapi3.SchemaError) problem {
	segs := se.JSONPointer()
	path := displayPath(body, segs)
	where := or(path, "Body")
	key := ""
	if len(segs) > 0 {
		key = segs[len(segs)-1]
	}
	s := se.Schema
	switch se.SchemaField {
	case "required":
		return problem{path, fmt.Sprintf("Pflichtfeld %s fehlt", strconv.Quote(path)), ""}
	case "properties":
		return unknownProblem(path, se.Value, s)
	case "enum":
		return enumProblem(path, se.Value, s)
	case "nullable":
		return problem{path, where + ": darf nicht null sein", ""}
	}
	if key == vatField {
		return problem{path, fmt.Sprintf("%s: ungültiger Steuersatz %s", where, showJSON(se.Value)), vatFormatHint}
	}
	switch se.SchemaField {
	case "type":
		return problem{path, fmt.Sprintf("%s: %s erwartet", where, typeName(s.Type.Slice())), ""}
	case "anyOf", "oneOf":
		if types := branchTypes(s); len(types) > 0 {
			return problem{path, fmt.Sprintf("%s: %s erwartet", where, typeName(types)), ""}
		}
	case "format":
		if s.Format == "date" {
			return problem{path, fmt.Sprintf("%s: ungültiges Datum %s", where, showJSON(se.Value)), dateFormatHint}
		}
		return problem{path, fmt.Sprintf("%s: ungültiges Format %s", where, showJSON(se.Value)), ""}
	}
	return problem{path, fmt.Sprintf("%s: ungültiger Wert %s", where, showJSON(se.Value)), ""}
}

// unknownProblem reports the keys of an object that its schema does not
// allow (additionalProperties: false). kin-openapi reports one error per key
// without naming it in a structured way, so all unknown keys are derived from
// the value; duplicate lines are merged later.
func unknownProblem(path string, v any, s *openapi3.Schema) problem {
	m, _ := v.(map[string]any)
	var us []unknownField
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if s.Properties[k] == nil {
			us = append(us, unknownField{dotPath(path, k), suggestKey(k, s)})
		}
	}
	if len(us) == 0 {
		return problem{path, fmt.Sprintf("%s: unbekanntes Feld", or(path, "Body")), ""}
	}
	lines := make([]string, len(us))
	for i, u := range us {
		lines[i] = unknownLine(u)
	}
	return problem{path, strings.Join(lines, "\n"), unknownHint(us)}
}

func enumProblem(path string, v any, s *openapi3.Schema) problem {
	var allowed []string
	for _, e := range s.Enum {
		if e != nil {
			allowed = append(allowed, fmt.Sprint(e))
		}
	}
	msg := fmt.Sprintf("ungültiger Wert %s für %s", showJSON(v), path)
	sugg := enumSuggestions(s, fmt.Sprint(v), allowed)
	if len(allowed) > bigEnum {
		return problem{path, msg + " – meintest du " + quoteList(sugg, "oder") + "?", bigEnumHint}
	}
	if len(sugg) > 0 {
		msg += " – meintest du " + quoteList(sugg, "oder") + "?"
	} else {
		msg += "."
	}
	return problem{path, msg + " Erlaubt: " + strings.Join(allowed, ", "), ""}
}

// typeName names JSON types in German for "… erwartet": "Zahl",
// "Text, ganze Zahl, Zahl oder Wahrheitswert". A lone boolean is
// "Wahrheitswert (true/false)".
func typeName(types []string) string {
	if len(types) == 1 && types[0] == "boolean" {
		return "Wahrheitswert (true/false)"
	}
	names := make([]string, 0, len(types))
	for _, t := range types {
		switch t {
		case "string":
			t = "Text"
		case "number":
			t = "Zahl"
		case "integer":
			t = "ganze Zahl"
		case "boolean":
			t = "Wahrheitswert"
		case "object":
			t = "Objekt"
		case "array":
			t = "Liste"
		}
		if !slices.Contains(names, t) {
			names = append(names, t)
		}
	}
	switch len(names) {
	case 0:
		return "anderer Typ"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " oder " + names[len(names)-1]
}

// branchTypes collects the types of the anyOf/oneOf branches in order.
func branchTypes(s *openapi3.Schema) []string {
	var out []string
	for _, b := range slices.Concat(s.AnyOf, s.OneOf) {
		if b.Value != nil && b.Value.Type != nil {
			out = append(out, b.Value.Type.Slice()...)
		}
	}
	return out
}

// displayPath renders a JSON pointer as a dotted path with array indices,
// e.g. line_items[0].category. Whether a segment is an index is read from the
// body, so numeric object keys (custom_attributes) stay keys.
func displayPath(body any, segs []string) string {
	var b strings.Builder
	cur := body
	for _, s := range segs {
		switch x := cur.(type) {
		case []any:
			b.WriteString("[" + s + "]")
			if i, err := strconv.Atoi(s); err == nil && i >= 0 && i < len(x) {
				cur = x[i]
			} else {
				cur = nil
			}
			continue
		case map[string]any:
			cur = x[s]
		default:
			cur = nil
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s)
	}
	return b.String()
}

// or returns s, or def when s is empty.
func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// showJSON renders a body value for an error message: strings quoted,
// objects and arrays as JSON.
func showJSON(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return showValue(v)
}

// checkKeyPatterns enforces x-cli-key-pattern (default ^[0-9]+$, as for
// --attr) on the keys of free-form objects (custom_attributes), which
// OpenAPI 3.0 cannot express.
func checkKeyPatterns(s *openapi3.Schema, v any, segs []string, body any) ([]problem, error) {
	if s == nil {
		return nil, nil
	}
	var out []problem
	switch x := v.(type) {
	case map[string]any:
		if _, ok := s.Extensions["x-cli-key-pattern"]; ok || isFreeFormMap(s) {
			pattern := keyPattern(s)
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, clierr.New(clierr.CodeError, fmt.Sprintf("ungültiges Schlüsselmuster %q in der Spec", pattern), "")
			}
			path := displayPath(body, segs)
			for _, k := range slices.Sorted(maps.Keys(x)) {
				if !re.MatchString(k) {
					out = append(out, problem{path, fmt.Sprintf("%s: Attribut-ID %s passt nicht zum Muster %s", path, strconv.Quote(k), pattern), keyPatternHint})
				}
			}
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			if ps := s.Properties[k]; ps != nil {
				p, err := checkKeyPatterns(ps.Value, x[k], append(slices.Clone(segs), k), body)
				if err != nil {
					return nil, err
				}
				out = append(out, p...)
			}
		}
	case []any:
		it := itemSchema(s)
		for i, e := range x {
			p, err := checkKeyPatterns(it, e, append(slices.Clone(segs), strconv.Itoa(i)), body)
			if err != nil {
				return nil, err
			}
			out = append(out, p...)
		}
	}
	return out, nil
}

// checkLineItems rejects an empty or null line_items list where the schema
// requires the list (create of invoice, estimate, voucher; spec 6.2, R17):
// the schema has no minItems, but the API needs at least one position.
func checkLineItems(req *openapi3.Schema, body any) []problem {
	m, ok := body.(map[string]any)
	if !ok || !slices.Contains(req.Required, itemsField) {
		return nil
	}
	v, present := m[itemsField]
	if !present {
		return nil // reported as missing required field
	}
	if l, isList := v.([]any); v == nil || isList && len(l) == 0 {
		return []problem{{itemsField, itemsField + ": mindestens eine Position erforderlich", ""}}
	}
	return nil
}

var (
	utf8BOM = []byte{0xEF, 0xBB, 0xBF}
	utf16LE = []byte{0xFF, 0xFE}
	utf16BE = []byte{0xFE, 0xFF}
)

// invalidUTF8 returns the offset of the first byte that is not valid UTF-8.
func invalidUTF8(data []byte) int {
	for i := 0; i < len(data); {
		r, n := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && n <= 1 {
			return i
		}
		i += n
	}
	return len(data)
}

const jsonHint = "die Eingabe muss gültiges JSON (UTF-8) sein"

// DecodeJSON reads one JSON value from --input, --items or a body file:
// a UTF-8 BOM is removed, CRLF line ends are fine, UTF-16 (BOM FF FE / FE FF
// or NUL bytes) and other non-UTF-8 input (Windows-1252 umlauts) are
// rejected before decoding, since encoding/json would silently turn them
// into U+FFFD, numbers stay json.Number
// (exact decimals and large IDs), and anything after the value is an error.
// Syntax errors are validation errors (exit 5) that name line and column.
func DecodeJSON(r io.Reader) (any, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, clierr.New(clierr.CodeError, "Eingabe nicht lesbar: "+err.Error(), "")
	}
	if bytes.HasPrefix(data, utf16LE) || bytes.HasPrefix(data, utf16BE) || bytes.IndexByte(data, 0) >= 0 {
		return nil, clierr.New(clierr.CodeValidation, "Eingabe ist UTF-16-kodiert – als UTF-8 speichern", jsonHint)
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if !utf8.Valid(data) {
		at := invalidUTF8(data)
		return nil, clierr.New(clierr.CodeValidation, fmt.Sprintf("Eingabe ist nicht UTF-8-kodiert (z. B. Windows-1252, %s) – als UTF-8 speichern", position(data, at)), jsonHint)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, clierr.New(clierr.CodeValidation, "Eingabe ist leer", jsonHint)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, jsonError(data, err)
	}
	rest := data[dec.InputOffset():]
	if trimmed := bytes.TrimLeft(rest, " \t\r\n"); len(trimmed) > 0 {
		at := len(data) - len(trimmed)
		return nil, clierr.New(clierr.CodeValidation, fmt.Sprintf("ungültiges JSON: weitere Zeichen nach dem JSON-Wert (%s)", position(data, at)), jsonHint)
	}
	return v, nil
}

func jsonError(data []byte, err error) *clierr.Error {
	var se *json.SyntaxError
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF):
		return clierr.New(clierr.CodeValidation, "ungültiges JSON: Eingabe ist unvollständig", jsonHint)
	case errors.As(err, &se):
		at := min(max(int(se.Offset)-1, 0), len(data)-1)
		r, _ := utf8.DecodeRune(data[at:])
		return clierr.New(clierr.CodeValidation, fmt.Sprintf("ungültiges JSON in %s: unerwartetes Zeichen %s", position(data, at), strconv.Quote(string(r))), jsonHint)
	}
	return clierr.New(clierr.CodeValidation, "ungültiges JSON", jsonHint)
}

// position renders the byte offset at as "Zeile n, Spalte m" (1-based,
// columns in characters).
func position(data []byte, at int) string {
	line := 1 + bytes.Count(data[:at], []byte("\n"))
	start := bytes.LastIndexByte(data[:at], '\n') + 1
	return fmt.Sprintf("Zeile %d, Spalte %d", line, utf8.RuneCount(data[start:at])+1)
}
