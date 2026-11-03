package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// CoerceContext is the clock for relative dates (today/heute,
// yesterday/gestern): the CLI passes App.Now and App.Location. A nil Now
// means time.Now, a nil Loc time.Local.
type CoerceContext struct {
	Now func() time.Time
	Loc *time.Location
}

func (cc CoerceContext) today() time.Time {
	now, loc := time.Now, time.Local
	if cc.Now != nil {
		now = cc.Now
	}
	if cc.Loc != nil {
		loc = cc.Loc
	}
	return now().In(loc)
}

const dateLayout = "2006-01-02"

var (
	// jsonNumberRe is the JSON number grammar; json.Number values must match
	// it or encoding/json fails (ParseFloat alone accepts NaN, Inf, hex).
	jsonNumberRe = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
	jsonIntRe    = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
)

// defaultKeyPattern is the custom attribute key pattern when the schema has
// no x-cli-key-pattern.
const defaultKeyPattern = `^[0-9]+$`

// CoerceFlag converts the raw value of a field's flag into the JSON value of
// the request body (spec 5.1, 6.1):
//   - string: as is; format date accepts YYYY-MM-DD, today/heute and
//     yesterday/gestern (resolved with cc); vat_rate is normalized and sent
//     as "0.19"; enums are checked, with x-cli-enum-hints and suggestions in
//     the error;
//   - integer: int64; number: json.Number (text kept);
//   - boolean: bool from true/false/ja/nein/1/0; x-cli-bool-string sends
//     "true"/"false";
//   - KindRef, KindNestedRef, KindRefList: {"id": int64} (one value per flag
//     occurrence for ref lists);
//   - KindAttrs: {"<id>": "<wert>"} from <id>=<wert>.
//
// KindItems and KindObjectList have no single flag value (line items have
// their own parser) and yield an internal error. Invalid values are clierr
// validation errors that name the flag.
func CoerceFlag(cc CoerceContext, f Field, raw string) (any, error) {
	switch f.Kind {
	case KindRef, KindNestedRef, KindRefList:
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 0 {
			return nil, flagError(f, raw, "ID muss eine ganze Zahl ≥ 0 sein", "")
		}
		return map[string]any{"id": id}, nil
	case KindAttrs:
		return coerceAttr(f, raw)
	case KindScalar, KindGroup:
		return coerceScalar(cc, f, raw)
	default:
		return nil, clierr.New(clierr.CodeError, fmt.Sprintf("--%s: Feldart %s hat keinen einzelnen Flag-Wert", f.Flag, f.Kind), "")
	}
}

func coerceScalar(cc CoerceContext, f Field, raw string) (any, error) {
	if f.name() == vatField {
		v, err := NormalizeVAT(raw)
		if err != nil {
			return nil, flagError(f, raw, "kein gültiger Steuersatz", vatHint)
		}
		if f.Type == "string" {
			return FormatVAT(v), nil
		}
		return json.Number(FormatVAT(v)), nil
	}
	if f.IsBool() {
		b, ok := parseBool(raw)
		if !ok {
			return nil, flagError(f, raw, "Wahrheitswert erwartet", "erlaubt: true, false, ja, nein, 1, 0")
		}
		if f.BoolString {
			return strconv.FormatBool(b), nil
		}
		return b, nil
	}
	switch f.Type {
	case "integer":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, flagError(f, raw, "ganze Zahl erwartet", "")
		}
		return n, nil
	case "number":
		if !jsonNumberRe.MatchString(raw) {
			hint := ""
			if strings.Contains(raw, ",") {
				hint = "Dezimaltrennzeichen ist der Punkt (z. B. 12.5)"
			}
			return nil, flagError(f, raw, "Zahl erwartet (z. B. 12.5)", hint)
		}
		return json.Number(raw), nil
	}
	if f.Format == "date" {
		return coerceDate(cc, f, raw)
	}
	if len(f.Enum) > 0 {
		if err := checkEnum(f, raw); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "ja", "1":
		return true, true
	case "false", "nein", "0":
		return false, true
	}
	return false, false
}

func coerceDate(cc CoerceContext, f Field, raw string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "today", "heute":
		return cc.today().Format(dateLayout), nil
	case "yesterday", "gestern":
		return cc.today().AddDate(0, 0, -1).Format(dateLayout), nil
	}
	if _, err := time.Parse(dateLayout, raw); err != nil {
		return nil, flagError(f, raw, "Datum erwartet", "erlaubt: YYYY-MM-DD, today/heute, yesterday/gestern")
	}
	return raw, nil
}

func checkEnum(f Field, raw string) error {
	for _, e := range f.Enum {
		if raw == e {
			return nil
		}
	}
	msg := fmt.Sprintf("ungültiger Wert %q für --%s", raw, f.Flag)
	var sugg []string
	if h, ok := EnumHint(f.Schema, raw); ok {
		sugg = []string{h}
	} else {
		sugg = Suggest(raw, f.Enum, 3)
	}
	if len(sugg) > 0 {
		msg += " – meintest du " + quoteList(sugg, "oder") + "?"
	}
	hint := "erlaubt: " + strings.Join(f.Enum, ", ")
	if len(f.Enum) > bigEnum {
		hint = "alle Werte zeigt die Completion (Tab)"
	}
	return clierr.New(clierr.CodeValidation, msg, hint)
}

// quoteList renders ["a","b","c"] as `"a", "b" oder "c"`.
func quoteList(items []string, conj string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = strconv.Quote(s)
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " " + conj + " " + q[len(q)-1]
}

func coerceAttr(f Field, raw string) (any, error) {
	key, val, ok := strings.Cut(raw, "=")
	if !ok {
		return nil, flagError(f, raw, "erwartet <id>=<wert>", "z. B. --"+f.Flag+" 12=Wert")
	}
	pattern := defaultKeyPattern
	if f.Schema != nil {
		if p, ok := f.Schema.Extensions["x-cli-key-pattern"].(string); ok {
			pattern = p
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, clierr.New(clierr.CodeError, fmt.Sprintf("--%s: ungültiges Schlüsselmuster %q in der Spec", f.Flag, pattern), "")
	}
	if !re.MatchString(key) {
		return nil, flagError(f, raw, fmt.Sprintf("Attribut-ID %q ist keine Zahl", key), "z. B. --"+f.Flag+" 12=Wert")
	}
	return map[string]any{key: val}, nil
}

func flagError(f Field, raw, reason, hint string) *clierr.Error {
	return clierr.New(clierr.CodeValidation, fmt.Sprintf("ungültiger Wert %q für --%s: %s", raw, f.Flag, reason), hint)
}

// CoerceValue converts a body value (from --input or --items) to the type of
// its schema, recursing into objects and arrays (spec 4.4, 6.3): numbers and
// booleans for string fields become strings ("5000", "12.5", "true"),
// numeric strings for integer/number fields become json.Number, "true"/"false"
// for boolean fields become bool, and every vat_rate is normalized (number
// 0.19 in line items, "0.19" where the schema says string). Numbers stay
// json.Number text, never round-trip through float64. path is the location
// of v for error messages (e.g. "line_items[0]"); its last element names the
// field. Values that do not fit are left as they are for the validator; only
// an invalid vat_rate is an error here. The input is not modified.
func CoerceValue(s *openapi3.Schema, v any, path string) (any, error) {
	key := path[strings.LastIndex(path, ".")+1:]
	if i := strings.IndexByte(key, '['); i >= 0 {
		key = key[:i]
	}
	return coerceValue(s, v, path, key)
}

func coerceValue(s *openapi3.Schema, v any, path, key string) (any, error) {
	if s == nil || v == nil {
		return v, nil
	}
	isArray := s.Type.Is("array")
	if key == vatField && !isArray && !isObject(s) {
		f, err := NormalizeVAT(v)
		if err != nil {
			var ce *clierr.Error
			if errors.As(err, &ce) {
				return nil, clierr.New(clierr.CodeValidation, path+": "+ce.Message, ce.Hint)
			}
			return nil, err
		}
		if s.Type.Is("string") {
			return FormatVAT(f), nil
		}
		return json.Number(FormatVAT(f)), nil
	}
	switch {
	case isArray:
		in, ok := v.([]any)
		if !ok {
			return v, nil
		}
		it := itemSchema(s)
		out := make([]any, len(in))
		for i, e := range in {
			c, err := coerceValue(it, e, fmt.Sprintf("%s[%d]", path, i), "")
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case isObject(s):
		in, ok := v.(map[string]any)
		if !ok {
			return v, nil
		}
		out := make(map[string]any, len(in))
		for k, e := range in {
			ps := s.Properties[k]
			if ps == nil {
				out[k] = e
				continue
			}
			p := k
			if path != "" {
				p = path + "." + k
			}
			c, err := coerceValue(ps.Value, e, p, k)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	case s.Type.Is("string"):
		switch x := v.(type) {
		case json.Number:
			return string(x), nil
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64), nil
		case int:
			return strconv.Itoa(x), nil
		case int64:
			return strconv.FormatInt(x, 10), nil
		case bool:
			return strconv.FormatBool(x), nil
		}
	case s.Type.Is("integer"):
		if x, ok := v.(string); ok && jsonIntRe.MatchString(x) {
			return json.Number(x), nil
		}
	case s.Type.Is("number"):
		if x, ok := v.(string); ok && jsonNumberRe.MatchString(x) {
			return json.Number(x), nil
		}
	case s.Type.Is("boolean"):
		if x, ok := v.(string); ok && (x == "true" || x == "false") {
			return x == "true", nil
		}
	}
	return v, nil
}

// EnumHint looks in up in the schema's x-cli-enum-hints (case-insensitive)
// and returns the API value it stands for, e.g. non_eu → foreign.
func EnumHint(s *openapi3.Schema, in string) (string, bool) {
	if s == nil {
		return "", false
	}
	hints, ok := s.Extensions["x-cli-enum-hints"].(map[string]any)
	if !ok {
		return "", false
	}
	for k, v := range hints {
		if strings.EqualFold(k, in) {
			if t, ok := v.(string); ok {
				return t, true
			}
		}
	}
	return "", false
}
