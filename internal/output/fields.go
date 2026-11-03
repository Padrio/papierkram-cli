package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// object is a JSON object that keeps the key order of the API. Decoded
// values are nil, bool, json.Number, string, []any and object.
type object []member

type member struct {
	key string
	val any
}

func (o object) get(k string) (any, bool) {
	for _, m := range o {
		if m.key == k {
			return m.val, true
		}
	}
	return nil, false
}

// set replaces the value of k or appends k.
func (o *object) set(k string, v any) {
	for i := range *o {
		if (*o)[i].key == k {
			(*o)[i].val = v
			return
		}
	}
	*o = append(*o, member{k, v})
}

// MarshalJSON writes the members in order; numbers stay json.Number
// literals.
func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := marshalJSON(m.key)
		if err != nil {
			return nil, err
		}
		v, err := marshalJSON(m.val)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// decode parses one JSON value keeping key order and number literals.
func decode(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, invalidJSON(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalidJSON(errors.New("weitere Daten nach dem JSON-Wert"))
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		obj := object{}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			obj.set(k.(string), v)
		}
		_, err := dec.Token() // '}'
		return obj, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token() // ']'
		return arr, err
	}
	return tok, nil
}

// project reduces v to the dotted paths in fields. A list envelope
// ({"type":"list",…,"entries":[…]}) keeps its envelope keys and projects
// every entry; a top-level array projects every element.
func project(v any, fields []string) any {
	paths := make([][]string, len(fields))
	for i, f := range fields {
		paths[i] = strings.Split(f, ".")
	}
	switch v := v.(type) {
	case object:
		if entries, ok := listEntries(v); ok {
			env := slices.Clone(v)
			env.set("entries", projectAll(entries, paths))
			return env
		}
		return projectObject(v, paths)
	case []any:
		return projectAll(v, paths)
	}
	return v
}

func listEntries(o object) ([]any, bool) {
	if t, _ := o.get("type"); t != "list" {
		return nil, false
	}
	e, _ := o.get("entries")
	entries, ok := e.([]any)
	return entries, ok
}

func projectAll(arr []any, paths [][]string) []any {
	out := make([]any, len(arr))
	for i, e := range arr {
		if o, ok := e.(object); ok {
			out[i] = projectObject(o, paths)
		} else {
			out[i] = e
		}
	}
	return out
}

// projectObject builds a new object with the given paths in their order;
// missing paths are left out, null values are kept.
func projectObject(o object, paths [][]string) object {
	out := object{}
	for _, p := range paths {
		if v, ok := lookup(o, p); ok {
			setPath(&out, p, v)
		}
	}
	return out
}

func lookup(v any, path []string) (any, bool) {
	for _, k := range path {
		o, ok := v.(object)
		if !ok {
			return nil, false
		}
		if v, ok = o.get(k); !ok {
			return nil, false
		}
	}
	return v, true
}

// setPath sets path in o, creating parent objects. Parents taken over from
// the input (e.g. "billing" before "billing.company") are cloned first, so
// the input is never modified.
func setPath(o *object, path []string, v any) {
	if len(path) == 1 {
		o.set(path[0], v)
		return
	}
	child, _ := o.get(path[0])
	c, ok := child.(object)
	if ok {
		c = slices.Clone(c)
	} else {
		c = object{}
	}
	setPath(&c, path[1:], v)
	o.set(path[0], c)
}

// toYAML converts a decoded value for goccy/go-yaml: objects become
// MapSlices (key order) and numbers keep their literal.
func toYAML(v any) any {
	switch v := v.(type) {
	case object:
		ms := make(yaml.MapSlice, len(v))
		for i, m := range v {
			ms[i] = yaml.MapItem{Key: m.key, Value: toYAML(m.val)}
		}
		return ms
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = toYAML(e)
		}
		return out
	case json.Number:
		return yamlLiteral(v)
	case string:
		if looksNumeric(v) {
			b, err := json.Marshal(v) // a JSON string is a valid double-quoted YAML scalar
			if err == nil {
				return yamlLiteral(b)
			}
		}
	}
	return v
}

// yamlLiteral is written as is: JSON number literals stay unchanged
// (1190.00) and quoted strings stay strings.
type yamlLiteral string

func (l yamlLiteral) MarshalYAML() ([]byte, error) { return []byte(l), nil }

// looksNumeric reports single-line strings that start like a number.
// goccy/go-yaml leaves some of them unquoted (e.g. the zip code "0815"),
// and YAML readers would turn them into numbers; quoting them is always
// safe.
func looksNumeric(s string) bool {
	if strings.ContainsRune(s, '\n') {
		return false
	}
	s = strings.TrimLeft(s, "+-.")
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// toPlain converts a decoded value for gojq: objects become maps (gojq
// sorts keys on output); json.Number stays, gojq prints it unchanged.
func toPlain(v any) any {
	switch v := v.(type) {
	case object:
		m := make(map[string]any, len(v))
		for _, e := range v {
			m[e.key] = toPlain(e.val)
		}
		return m
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = toPlain(e)
		}
		return out
	}
	return v
}
