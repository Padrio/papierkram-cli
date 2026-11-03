package spec

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// PruneResult is a body cleaned up against the request schema. Removed lists
// the dotted paths of the response-only fields that were dropped;
// DroppedNulls the request fields whose null was dropped (they stay
// unchanged on update). Both are sorted and without duplicates; array
// elements appear as [] (line_items[].total_net).
type PruneResult struct {
	Body         map[string]any
	Removed      []string
	DroppedNulls []string
}

// Prune matches an --input body recursively against the request schema
// (spec 6.3); objects, arrays of objects (line_items) and nested references
// (customer.contact_person) are followed. For every key:
//   - in the request schema: kept, the request schema wins over readOnly
//     markers in the response (R31). A null where the request does not allow
//     null (e.g. line_items[].proposition from a GET answer) is dropped and
//     listed in DroppedNulls when the response has the field, since the
//     request cannot express "none" otherwise; required fields keep their
//     null so that Validate reports it;
//   - not in the request schema but in resp: removed and listed in Removed
//     (id, customer.name, line_items[].total_net, and top-level response
//     references such as contact_person or project, which are never
//     translated into request references);
//   - in neither: an unknown field. All unknown fields are collected into one
//     validation error (exit 5), with a suggestion from the request keys at
//     that level: unbekanntes Feld "note" – meintest du "notes"?
//
// resp is the response schema of the resource; allOf/oneOf/anyOf are merged.
// The caller chooses it; ResourceResponseSchema maps a create or update
// operation to the schema of its GET. Objects without properties in the
// request (custom_attributes) are free-form and kept as they are; their keys
// are checked by Validate (see isFreeFormMap). Values whose type does not match the schema are
// kept for Validate. The input is not modified; kept values are copied.
func Prune(body map[string]any, req, resp *openapi3.Schema) (PruneResult, error) {
	if req == nil {
		return PruneResult{}, clierr.New(clierr.CodeError, "Bereinigung ohne Request-Schema nicht möglich", "")
	}
	p := &pruner{removed: map[string]bool{}, nulls: map[string]bool{}, unknownSeen: map[string]bool{}}
	out := p.object(body, req, flatten([]*openapi3.Schema{resp}), "")
	if len(p.unknown) > 0 {
		return PruneResult{}, unknownFieldsError(p.unknown)
	}
	return PruneResult{
		Body:         out,
		Removed:      slices.Sorted(maps.Keys(p.removed)),
		DroppedNulls: slices.Sorted(maps.Keys(p.nulls)),
	}, nil
}

type pruner struct {
	removed     map[string]bool
	nulls       map[string]bool
	unknown     []unknownField
	unknownSeen map[string]bool
}

// unknownField is a key that neither schema knows; suggestion may be empty.
type unknownField struct {
	path, suggestion string
}

func (p *pruner) object(in map[string]any, req *openapi3.Schema, resp []*openapi3.Schema, path string) map[string]any {
	if isFreeFormMap(req) {
		return cloneValue(in).(map[string]any)
	}
	out := make(map[string]any, len(in))
	for _, k := range slices.Sorted(maps.Keys(in)) {
		v := in[k]
		cp := dotPath(path, k)
		rs := children(resp, k)
		ps := req.Properties[k]
		switch {
		case ps != nil && ps.Value != nil:
			if v == nil && !allowsNull(ps.Value) && len(rs) > 0 && !slices.Contains(req.Required, k) {
				p.nulls[cp] = true
				continue
			}
			out[k] = p.value(v, ps.Value, rs, cp)
		case len(rs) > 0:
			p.removed[cp] = true
		default:
			if !p.unknownSeen[cp] {
				p.unknownSeen[cp] = true
				p.unknown = append(p.unknown, unknownField{cp, suggestKey(k, req)})
			}
		}
	}
	return out
}

func (p *pruner) value(v any, req *openapi3.Schema, resp []*openapi3.Schema, path string) any {
	switch x := v.(type) {
	case map[string]any:
		if isObject(req) {
			return p.object(x, req, resp, path)
		}
	case []any:
		if it := itemSchema(req); it != nil {
			ri := items(resp)
			out := make([]any, len(x))
			for i, e := range x {
				out[i] = p.value(e, it, ri, path+"[]")
			}
			return out
		}
	}
	return cloneValue(v)
}

// allowsNull reports whether the schema accepts null (OpenAPI 3.0 nullable,
// also in an anyOf/oneOf branch).
func allowsNull(s *openapi3.Schema) bool {
	if s.Nullable {
		return true
	}
	for _, b := range slices.Concat(s.AnyOf, s.OneOf) {
		if b.Value != nil && b.Value.Nullable {
			return true
		}
	}
	return false
}

// flatten returns the schemas and, recursively, their allOf/oneOf/anyOf
// branches (the union of everything a response may contain), without nil
// entries and duplicates.
func flatten(list []*openapi3.Schema) []*openapi3.Schema {
	var out []*openapi3.Schema
	var add func(s *openapi3.Schema)
	add = func(s *openapi3.Schema) {
		if s == nil || slices.Contains(out, s) {
			return
		}
		out = append(out, s)
		for _, r := range slices.Concat(s.AllOf, s.OneOf, s.AnyOf) {
			add(r.Value)
		}
	}
	for _, s := range list {
		add(s)
	}
	return out
}

// children returns the flattened property schemas named k of the response
// schemas.
func children(resp []*openapi3.Schema, k string) []*openapi3.Schema {
	var out []*openapi3.Schema
	for _, s := range resp {
		if ps := s.Properties[k]; ps != nil {
			out = append(out, ps.Value)
		}
	}
	return flatten(out)
}

// items returns the flattened element schemas of the response array schemas.
// A list envelope ({"type": "list", "entries": [...]}) counts as an array of
// its entries.
func items(resp []*openapi3.Schema) []*openapi3.Schema {
	var out []*openapi3.Schema
	for _, s := range resp {
		if it := itemSchema(s); it != nil {
			out = append(out, it)
		} else if e := s.Properties["entries"]; e != nil && e.Value != nil {
			if it := itemSchema(e.Value); it != nil {
				out = append(out, it)
			}
		}
	}
	return flatten(out)
}

func dotPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// suggestKey returns the request key at this level closest to k, or "".
func suggestKey(k string, req *openapi3.Schema) string {
	if s := Suggest(k, slices.Sorted(maps.Keys(req.Properties)), 1); len(s) > 0 {
		return s[0]
	}
	return ""
}

// unknownLine renders one unknown field for an error message.
func unknownLine(u unknownField) string {
	line := fmt.Sprintf("unbekanntes Feld %s", strconv.Quote(u.path))
	if u.suggestion != "" {
		line += " – meintest du " + strconv.Quote(u.suggestion) + "?"
	}
	return line
}

// unknownHint tells where unknown fields usually come from and how to drop
// them with jq. The suggestions are already on the message lines.
func unknownHint(us []unknownField) string {
	paths := make([]string, len(us))
	for i, u := range us {
		paths[i] = jqPath(u.path)
	}
	lead := "stammt das Feld"
	if len(us) > 1 {
		lead = "stammen die Felder"
	}
	return fmt.Sprintf("%s aus einer describe-Ausgabe: mit jq 'del(%s)' entfernen oder die CLI aktualisieren", lead, strings.Join(paths, ", "))
}

var jqIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// jqPath turns a dotted field path (line_items[].name, line_items[1].name)
// into a jq path (.line_items[].name); keys that are no jq identifiers are
// quoted (."12").
func jqPath(path string) string {
	var b strings.Builder
	for _, seg := range strings.Split(path, ".") {
		key, idx, _ := strings.Cut(seg, "[")
		if jqIdentRe.MatchString(key) {
			b.WriteString("." + key)
		} else {
			b.WriteString("." + strconv.Quote(key))
		}
		if idx != "" {
			b.WriteString("[" + idx)
		}
	}
	return b.String()
}

func unknownFieldsError(us []unknownField) *clierr.Error {
	lines := make([]string, len(us))
	for i, u := range us {
		lines[i] = unknownLine(u)
	}
	return clierr.New(clierr.CodeValidation, strings.Join(lines, "\n"), unknownHint(us))
}

// lastKey is the last element of a dotted path without array markers.
func lastKey(path string) string {
	k := path[strings.LastIndex(path, ".")+1:]
	if i := strings.IndexByte(k, '['); i >= 0 {
		k = k[:i]
	}
	return k
}

// cloneValue deep-copies JSON values (maps and slices); scalars are shared.
func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = cloneValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneValue(e)
		}
		return out
	}
	return v
}

// ResourceResponseSchema returns the response schema of the GET operation of
// the resource an operation works on: for a POST on a collection (create)
// the GET on <path>/{id}, for a PUT, PATCH or GET on an item the GET on the
// same path. Callers pass it to Prune, so describe output can be fed back to
// create and update. It is nil when there is no such GET.
func ResourceResponseSchema(opID string) *openapi3.Schema {
	o, ok := OperationByID(opID)
	if !ok {
		return nil
	}
	path := o.Path
	if o.Method == "POST" {
		path += "/{id}"
	}
	item := Doc().Paths.Value(path)
	if item == nil || item.Get == nil {
		return nil
	}
	return ResponseSchema(item.Get.OperationID)
}
