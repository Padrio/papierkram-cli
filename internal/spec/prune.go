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
// unchanged on update); Moved the response references taken over into the
// request ("contact_person → customer.contact_person"). All are sorted and
// without duplicates; array elements appear as [] (line_items[].total_net).
type PruneResult struct {
	Body         map[string]any
	Removed      []string
	DroppedNulls []string
	Moved        []string
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
//     (id, customer.name, line_items[].total_net). Exception: a reference
//     of the response (an object with id, e.g. the top-level
//     contact_person and project of an invoice) that exactly one request
//     object at the same level takes as a reference of the same name
//     (customer.contact_person) is moved there as {id} and listed in
//     Moved, when that object is in the body and does not name it yet
//     (movableReferences). The live API clears contact person and project
//     of an invoice whose PUT lacks them (measured 2026-11-04), so a
//     describe | update round trip has to send them;
//   - in neither: an unknown field. All unknown fields are collected into one
//     validation error (exit 5), with a suggestion from the request keys at
//     that level: unbekanntes Feld "note" – meintest du "notes"? Without a
//     close key, the only unknown field of an object suggests the only
//     required key it lacks (see onlyMissingRequired).
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
	p := &pruner{removed: map[string]bool{}, nulls: map[string]bool{}, moved: map[string]bool{}, unknownSeen: map[string]bool{}}
	out := p.object(body, req, flatten([]*openapi3.Schema{resp}), "")
	if len(p.unknown) > 0 {
		return PruneResult{}, unknownFieldsError(p.unknown)
	}
	return PruneResult{
		Body:         out,
		Removed:      slices.Sorted(maps.Keys(p.removed)),
		DroppedNulls: slices.Sorted(maps.Keys(p.nulls)),
		Moved:        slices.Sorted(maps.Keys(p.moved)),
	}, nil
}

type pruner struct {
	removed     map[string]bool
	nulls       map[string]bool
	moved       map[string]bool
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
	var own []int             // indexes in p.unknown of the unknown keys of this object
	var responseOnly []string // keys only the response has; moved or removed below
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
			responseOnly = append(responseOnly, k)
		default:
			if !p.unknownSeen[cp] {
				p.unknownSeen[cp] = true
				own = append(own, len(p.unknown))
				p.unknown = append(p.unknown, unknownField{cp, suggestKey(k, req)})
			}
		}
	}
	for _, k := range responseOnly {
		if !p.moveReference(out, in[k], k, req, path) {
			p.removed[dotPath(path, k)] = true
		}
	}
	// only this object's own unknown keys count: nested objects and line
	// items append theirs to p.unknown too, with their own missing fields
	if len(own) == 1 && p.unknown[own[0]].suggestion == "" {
		p.unknown[own[0]].suggestion = onlyMissingRequired(in, req)
	}
	return out
}

// moveReference puts the response reference v (named k, at path) into the
// request object of out that takes a reference of that name (see
// referenceTarget), as {id}. It reports whether it did: not when v is no
// object with an id, there is no single target, the target object is not
// in out, or it already names k.
func (p *pruner) moveReference(out map[string]any, v any, k string, req *openapi3.Schema, path string) bool {
	ref, ok := v.(map[string]any)
	if !ok || ref["id"] == nil {
		return false
	}
	target := referenceTarget(req, k)
	obj, ok := out[target].(map[string]any)
	if target == "" || !ok {
		return false
	}
	if _, named := obj[k]; named {
		return false
	}
	obj[k] = map[string]any{"id": cloneValue(ref["id"])}
	p.moved[dotPath(path, k)+" → "+dotPath(dotPath(path, target), k)] = true
	return true
}

// referenceTarget is the one property of the request object req whose
// object schema has a reference named k (an object with an id); "" when
// none or more than one has.
func referenceTarget(req *openapi3.Schema, k string) string {
	target := ""
	for _, name := range slices.Sorted(maps.Keys(req.Properties)) {
		ps := req.Properties[name]
		if ps == nil || ps.Value == nil || !isObject(ps.Value) {
			continue
		}
		if c := ps.Value.Properties[k]; c != nil && c.Value != nil && isObject(c.Value) && c.Value.Properties["id"] != nil {
			if target != "" {
				return ""
			}
			target = name
		}
	}
	return target
}

// movableReferences lists, for the top level of a request schema and the
// flattened response schemas of its resource, the response references
// Prune would move ("contact_person → customer.contact_person"): keys of
// the response that the request lacks at top level but takes below
// exactly one request object.
func movableReferences(req *openapi3.Schema, resp []*openapi3.Schema) []string {
	var out []string
	keys := map[string]bool{}
	for _, s := range resp {
		for k := range s.Properties {
			keys[k] = true
		}
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if req.Properties[k] != nil {
			continue
		}
		if t := referenceTarget(req, k); t != "" {
			out = append(out, k+" → "+t+"."+k)
		}
	}
	return out
}

// onlyMissingRequired returns the required key of req that in lacks when it
// is the only one, else "". An unknown field that is the only one of its
// object then most likely stands for it under another name (address for
// email.recipient, R06), even if the spelling is not close.
func onlyMissingRequired(in map[string]any, req *openapi3.Schema) string {
	var missing []string
	for _, k := range req.Required {
		if _, ok := in[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) != 1 {
		return ""
	}
	return missing[0]
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
