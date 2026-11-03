package spec

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// FieldKind says how a request body field is set from the command line.
type FieldKind string

// The field kinds of spec 6.1/6.2.
const (
	KindScalar     FieldKind = "scalar"      // top-level scalar: --x-y
	KindRef        FieldKind = "ref"         // top-level {id}: --customer 12
	KindNestedRef  FieldKind = "nested_ref"  // a.b{id}: --contact-person, --customer-project
	KindGroup      FieldKind = "group"       // scalar inside an object: --billing-company
	KindRefList    FieldKind = "ref_list"    // array of {id}: --team-member (repeatable)
	KindAttrs      FieldKind = "attrs"       // custom_attributes: --attr id=wert
	KindItems      FieldKind = "items"       // line_items: --item / --items
	KindObjectList FieldKind = "object_list" // any other array: only via --input, no flag
)

// itemsField is the array of line items; it gets --item/--items (spec 6.2)
// instead of being an input-only object list.
const itemsField = "line_items"

// Field is one settable part of a request body, derived from the request
// schema. Path is the JSON path of the value in the body. For KindRef,
// KindNestedRef and KindRefList the value at Path is {"id": n}; Type and
// Format are then those of the id. Flag is empty for KindObjectList. Schema
// is the property schema at Path (enum hints, key pattern).
type Field struct {
	Flag                             string
	Path                             []string
	Kind                             FieldKind
	Type, Format                     string
	Enum                             []string
	Required, BoolString, Repeatable bool
	Help                             string
	Schema                           *openapi3.Schema
}

// IsBool reports whether the field is a boolean flag: type boolean, or a
// string marked x-cli-bool-string (sent as "true"/"false").
func (f Field) IsBool() bool { return f.Type == "boolean" || f.BoolString }

// name is the property name of the field (the last path element).
func (f Field) name() string { return f.Path[len(f.Path)-1] }

// reservedFlags are the global flags and the flags the builder adds to
// create/update commands; no schema field may take them.
var reservedFlags = []string{
	"attr", "context", "debug", "dry-run", "fields", "help", "input", "item", "items",
	"jq", "no-color", "no-input", "output", "replace-items", "timeout", "yes",
}

// ReservedFlags returns the flag names no schema-derived field may use.
// --item belongs to the KindItems field and --attr to the KindAttrs field.
func ReservedFlags() []string { return slices.Clone(reservedFlags) }

// ownFlag is the reserved flag a field kind itself stands for.
func ownFlag(k FieldKind) string {
	switch k {
	case KindItems:
		return "item"
	case KindAttrs:
		return "attr"
	}
	return ""
}

// bigEnum is the enum size above which help texts refer to completion
// instead of listing the values (the 94 expense categories).
const bigEnum = 20

// Fields derives the settable fields of a request body from its schema
// (spec 5.1, 6.1): flag name = field name in kebab-case; objects with an id
// become ID flags; a nested reference a.b{id} becomes --b, or --a-b when b is
// one of resourceNames; scalars inside objects become --<object>-<field>;
// arrays of {id} become a repeatable singular flag; custom_attributes become
// --attr, line_items --item; all other arrays (people,
// entertainment_persons) are input-only.
//
// The result is sorted by flag name. When two fields (or a field and a
// reserved flag) would share a flag name, a nested field falls back to its
// full path (--<parent>-<field>); if that still collides, Fields returns an
// error. A nil schema yields no fields.
func Fields(req *openapi3.Schema, resourceNames []string) ([]Field, error) {
	if req == nil {
		return nil, nil
	}
	w := &walker{resources: map[string]bool{}}
	for _, n := range resourceNames {
		w.resources[n] = true
	}
	w.object(req, nil, true, "")
	if err := w.resolve(); err != nil {
		return nil, err
	}
	fs := w.fields
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Flag != fs[j].Flag {
			return fs[i].Flag < fs[j].Flag
		}
		return strings.Join(fs[i].Path, ".") < strings.Join(fs[j].Path, ".")
	})
	return fs, nil
}

type walker struct {
	resources map[string]bool
	fields    []Field
	long      []string // fallback flag per field (full path)
}

// object walks the properties of s; skip is a property to leave out (the id
// of a reference, which the reference field itself covers).
func (w *walker) object(s *openapi3.Schema, path []string, required bool, skip string) {
	for name, ref := range s.Properties {
		if name == skip || ref == nil || ref.Value == nil {
			continue
		}
		p := append(slices.Clone(path), name)
		w.property(ref.Value, p, required && slices.Contains(s.Required, name))
	}
}

func (w *walker) property(ps *openapi3.Schema, p []string, required bool) {
	top := len(p) == 1
	name := p[len(p)-1]
	full := kebab(strings.Join(p, "-"))
	switch {
	case ps.Type.Is("array"):
		it := itemSchema(ps)
		switch {
		case top && name == itemsField:
			w.add(Field{Flag: "item", Kind: KindItems, Type: "array", Repeatable: true}, ps, p, required, "item")
		case it != nil && isIDOnly(it):
			flag := singular(full)
			f := Field{Flag: flag, Kind: KindRefList, Repeatable: true}
			setType(&f, it.Properties["id"].Value)
			w.add(f, ps, p, required, flag)
		default:
			// people (objects) and entertainment_persons (strings) are set
			// only via --input (spec 6.1).
			w.add(Field{Kind: KindObjectList, Type: "array"}, ps, p, required, "")
		}
	case isAttrs(ps):
		w.add(Field{Flag: "attr", Kind: KindAttrs, Type: "object", Repeatable: true}, ps, p, required, "attr")
	case isObject(ps):
		skip := ""
		if id := ps.Properties["id"]; id != nil && id.Value != nil {
			skip = "id"
			f := Field{Flag: kebab(name), Kind: KindRef}
			if !top {
				f.Kind = KindNestedRef
				if w.resources[f.Flag] {
					f.Flag = full
				}
			}
			setType(&f, id.Value)
			w.add(f, ps, p, required, full)
		}
		w.object(ps, p, required, skip)
	default:
		f := Field{Flag: full, Kind: KindScalar}
		if !top {
			f.Kind = KindGroup
		}
		setScalar(&f, ps)
		w.add(f, ps, p, required, full)
	}
}

func (w *walker) add(f Field, ps *openapi3.Schema, p []string, required bool, long string) {
	f.Path, f.Schema, f.Required = p, ps, required
	f.Help = help(f, ps)
	w.fields = append(w.fields, f)
	w.long = append(w.long, long)
}

// resolve renames colliding nested fields to their full path and reports
// collisions that remain.
func (w *walker) resolve() error {
	clashes := func() map[string][]int {
		by := map[string][]int{}
		for i, f := range w.fields {
			if f.Flag != "" {
				by[f.Flag] = append(by[f.Flag], i)
			}
		}
		out := map[string][]int{}
		for flag, idx := range by {
			owned := len(idx) == 1 && ownFlag(w.fields[idx[0]].Kind) == flag
			reserved := slices.Contains(reservedFlags, flag) && !owned
			if len(idx) > 1 || reserved {
				out[flag] = idx
			}
		}
		return out
	}
	for _, idx := range clashes() {
		for _, i := range idx {
			if w.long[i] != "" {
				w.fields[i].Flag = w.long[i]
			}
		}
	}
	remaining := clashes()
	if len(remaining) == 0 {
		return nil
	}
	flags := make([]string, 0, len(remaining))
	for flag := range remaining {
		flags = append(flags, flag)
	}
	sort.Strings(flags)
	var msgs []string
	for _, flag := range flags {
		var paths []string
		for _, i := range remaining[flag] {
			paths = append(paths, strings.Join(w.fields[i].Path, "."))
		}
		msgs = append(msgs, fmt.Sprintf("--%s (%s)", flag, strings.Join(paths, ", ")))
	}
	return fmt.Errorf("Flag-Namen nicht eindeutig: %s", strings.Join(msgs, "; "))
}

func setType(f *Field, s *openapi3.Schema) {
	if ts := s.Type.Slice(); len(ts) > 0 {
		f.Type = ts[0]
	}
	f.Format = s.Format
}

func setScalar(f *Field, s *openapi3.Schema) {
	setType(f, s)
	f.BoolString, _ = s.Extensions["x-cli-bool-string"].(bool)
	for _, e := range s.Enum {
		if e != nil {
			f.Enum = append(f.Enum, fmt.Sprint(e))
		}
	}
}

func itemSchema(s *openapi3.Schema) *openapi3.Schema {
	if s.Items == nil {
		return nil
	}
	return s.Items.Value
}

func isObject(s *openapi3.Schema) bool {
	return s.Type.Is("object") || (s.Type.IsEmpty() && len(s.Properties) > 0)
}

func isIDOnly(s *openapi3.Schema) bool {
	return isObject(s) && len(s.Properties) == 1 && s.Properties["id"] != nil
}

// isAttrs reports a free-form map such as custom_attributes: an object
// without properties whose values are described by additionalProperties.
func isAttrs(s *openapi3.Schema) bool {
	return isObject(s) && len(s.Properties) == 0 && s.AdditionalProperties.Schema != nil
}

func kebab(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "-")) }

func singular(s string) string {
	if t, ok := strings.CutSuffix(s, "ies"); ok {
		return t + "y"
	}
	return strings.TrimSuffix(s, "s")
}

// help builds the German flag description: the schema description if there
// is one, else a generic text, plus date, VAT and enum hints.
func help(f Field, s *openapi3.Schema) string {
	path := strings.Join(f.Path, ".")
	h := s.Description
	switch {
	case h != "":
	case f.Kind == KindRef || f.Kind == KindNestedRef || f.Kind == KindRefList:
		h = "ID für " + path
	case f.Kind == KindAttrs:
		h = "eigenes Feld als <id>=<wert>"
	case f.Kind == KindItems:
		h = "Position als key=wert,key=wert"
	case f.Kind == KindObjectList:
		h = path + ": nur über --input setzbar"
	default:
		h = "Wert für " + path
	}
	switch {
	case f.Format == "date":
		h += " (YYYY-MM-DD, heute, gestern)"
	case f.Kind != KindObjectList && f.name() == vatField:
		h += " (19, 19%, 0,19 oder 0.19)"
	case f.IsBool():
		h += " (true/false)"
	}
	switch n := len(f.Enum); {
	case n > bigEnum:
		h += " (siehe Completion)"
	case n > 0:
		h += " (erlaubt: " + strings.Join(f.Enum, ", ") + ")"
	}
	if f.Repeatable {
		h += " (wiederholbar)"
	}
	return h
}
