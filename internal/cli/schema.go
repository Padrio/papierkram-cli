package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// schema <resource> [action] (spec 7.2): the request and response schema
// of the operation a command sends, resolved ($ref replaced, allOf
// merged) from the embedded patched spec, so x-cli-* extensions are
// kept. The command is found in the cobra tree like on the command line
// (aliases of resource and action work) and joined to its operation by
// metaOf, like commands --json.

const schemaLong = `Zeigt das Request- und das Response-Schema der API-Operation eines Befehls als {"request": …, "response": …} – aufgelöst ($ref ersetzt, allOf zusammengeführt) aus der eingebetteten, korrigierten Spec, mit den x-cli-*-Erweiterungen des CLI. null = die Spec beschreibt kein Schema (kein Body; die Antwort kann trotzdem JSON sein, z. B. bei info und upload); PDF-Downloads antworten mit {"type": "string", "format": "binary"}.

Ressource und Aktion dürfen Aliase sein (rechnung send). Ohne Aktion kommen alle Aktionen der Ressource, die eine Anfrage senden: {"resource": …, "actions": {"<aktion>": {"request": …, "response": …}}}. time log und time stop senden die Operation von time create.

Ausgabe als JSON (Standard) oder mit -o yaml; --fields und --jq wirken wie bei anderen Befehlen. Sendet keine Anfrage.`

func newSchemaCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "schema <ressource> [<aktion>]",
		Short: "Request- und Response-Schema einer Aktion als JSON anzeigen",
		Long:  schemaLong,
		Example: "  papierkram schema invoice create\n" +
			"  papierkram schema rechnung send -o yaml\n" +
			"  papierkram schema voucher --jq '.actions | keys'",
		Annotations: readsOnly(),
		Args:        minArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			v, err := schemaOf(c.Root(), args)
			if err != nil {
				return err
			}
			// a schema has no table form: JSON unless YAML is asked for
			format := output.FormatJSON
			if app.Flags.Output == string(output.FormatYAML) {
				format = output.FormatYAML
			}
			return output.RenderValue(app.outputOptions(c.Context(), string(format)), v, output.Columns{})
		},
	}
}

// schemaPair is the request and response schema of one operation.
type schemaPair struct {
	Request  any `json:"request"`
	Response any `json:"response"`
}

// resourceSchemas are the schemas of all actions of a resource.
type resourceSchemas struct {
	Resource string                `json:"resource"`
	Actions  map[string]schemaPair `json:"actions"`
}

// schemaOf resolves args (resource, then the words of the action) in the
// command tree of root and returns the schemas: of the action, or of all
// actions of the resource that send an operation. Unknown names, local
// commands and api are usage errors (exit 2).
func schemaOf(root *cobra.Command, args []string) (any, error) {
	if args[0] == "api" {
		return nil, clierr.New(clierr.CodeUsage, "papierkram api hat kein festes Schema: Methode und Pfad bestimmen die Operation",
			"Schema der Aktion abfragen, die der Pfad meint, z. B. papierkram schema invoice create")
	}
	res, ok := findResource(args[0])
	if !ok {
		hint := "Ressourcen: " + strings.Join(resourceNames(), ", ")
		// a meta command that sends an operation (auth status: GetInfo)
		// points to the resource command of that operation
		if c, rest, err := root.Find(args); err == nil && c != root && len(rest) == 0 && metaOf(c).op {
			e := metaOf(c).entry
			name := strings.TrimSpace(e.Resource + " " + e.Action)
			hint = fmt.Sprintf("%s sendet %s wie %s: papierkram schema %s", strings.Join(commandWords(c), " "), e.OperationID, name, name)
		}
		return nil, unknownName(fmt.Sprintf("unbekannte Ressource %q", args[0]), args[0], resourceCandidates(), hint)
	}
	group, _, err := root.Find([]string{res.Name})
	if err != nil || group == root {
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("Ressource %q hat keine Befehle", res.Name), "")
	}
	actions := schemaActions(group)
	names := actionNames(group, actions)
	hint := "Aktionen mit Schema: " + strings.Join(names, ", ")
	if !group.HasSubCommands() { // a resource that is a command itself (info)
		if len(args) > 1 {
			return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("unbekannte Aktion %q für %s", strings.Join(args[1:], " "), res.Name),
				"Verwendung: papierkram schema "+res.Name)
		}
		return operationSchemas(metaOf(group).entry.OperationID)
	}
	if len(args) == 1 {
		out := resourceSchemas{Resource: res.Name, Actions: map[string]schemaPair{}}
		for i, c := range actions {
			pair, err := operationSchemas(metaOf(c).entry.OperationID)
			if err != nil {
				return nil, err
			}
			out.Actions[names[i]] = pair
		}
		return out, nil
	}
	action := strings.Join(args[1:], " ")
	c, rest, err := group.Find(args[1:])
	if err == nil && c != group && len(rest) == 0 && c.HasSubCommands() {
		// a group below the resource (voucher document)
		var sub []string
		for _, n := range names {
			if strings.HasPrefix(n, action+" ") {
				sub = append(sub, n)
			}
		}
		if len(sub) > 0 {
			return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s %s ist eine Gruppe: Aktion angeben, z. B. %s", res.Name, action, strings.Join(sub, ", ")), hint)
		}
	}
	if err != nil || c == group || len(rest) > 0 {
		return nil, unknownName(fmt.Sprintf("unbekannte Aktion %q für %s", action, res.Name), action, names, hint)
	}
	if !metaOf(c).op {
		return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s sendet keine Anfrage und hat kein Schema", strings.Join(commandWords(c), " ")), hint)
	}
	return operationSchemas(metaOf(c).entry.OperationID)
}

// unknownName is a usage error for a name that is not known, with the
// closest candidate (spec.Suggest) in the message.
func unknownName(msg, in string, candidates []string, hint string) error {
	if s := spec.Suggest(in, candidates, 1); len(s) > 0 {
		msg += fmt.Sprintf(" – meintest du %q?", s[0])
	}
	return clierr.New(clierr.CodeUsage, msg, hint)
}

// findResource finds a resource by name or alias.
func findResource(name string) (registry.Resource, bool) {
	for _, r := range registry.Resources() {
		if r.Name == name || slices.Contains(r.Aliases, name) {
			return r, true
		}
	}
	return registry.Resource{}, false
}

// resourceCandidates are all resource names and aliases.
func resourceCandidates() []string {
	var out []string
	for _, r := range registry.Resources() {
		out = append(out, r.Name)
		out = append(out, r.Aliases...)
	}
	return out
}

// schemaActions are the commands below group that send an operation
// (metaOf), in the order of commandsList.
func schemaActions(group *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range commandsList(group) {
		if metaOf(c).op {
			out = append(out, c)
		}
	}
	return out
}

// actionNames are the action names of cmds relative to group
// ("document delete").
func actionNames(group *cobra.Command, cmds []*cobra.Command) []string {
	depth := len(commandWords(group))
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = strings.Join(commandWords(c)[depth:], " ")
	}
	return names
}

// operationSchemas resolves the request schema (spec.RequestSchema) and
// the response schema of an operation: its JSON answer
// (spec.ResponseSchema) or its file answer (spec.FileSchema).
func operationSchemas(opID string) (schemaPair, error) {
	req, err := resolveSchema(spec.RequestSchema(opID))
	if err != nil {
		return schemaPair{}, err
	}
	resp := spec.ResponseSchema(opID)
	if resp == nil {
		resp = spec.FileSchema(opID)
	}
	res, err := resolveSchema(resp)
	if err != nil {
		return schemaPair{}, err
	}
	return schemaPair{Request: req, Response: res}, nil
}

// componentPrefix starts every $ref of the spec.
const componentPrefix = "#/components/schemas/"

// resolveSchema returns s as plain JSON values with every $ref replaced
// by its component and every allOf merged into its schema; nil for nil.
func resolveSchema(s *openapi3.Schema) (any, error) {
	if s == nil {
		return nil, nil
	}
	v, err := schemaJSON(s)
	if err != nil {
		return nil, err
	}
	return resolveValue(v, nil)
}

// schemaJSON is the JSON form of s; nested references stay $ref.
func schemaJSON(s *openapi3.Schema) (any, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // numbers (minimum, examples) stay as written
	var v any
	err = dec.Decode(&v)
	return v, err
}

// resolveValue resolves one schema value. seen are the components being
// resolved above it; a cycle is an error.
func resolveValue(v any, seen []string) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return v, nil // a boolean schema
	}
	if ref, ok := m["$ref"].(string); ok {
		return resolveRef(ref, seen)
	}
	out := make(map[string]any, len(m))
	for k, x := range m {
		var err error
		switch k {
		case "allOf":
			continue
		case "properties":
			props, _ := x.(map[string]any)
			res := make(map[string]any, len(props))
			for name, p := range props {
				if res[name], err = resolveValue(p, seen); err != nil {
					return nil, err
				}
			}
			out[k] = res
		case "items", "not", "additionalProperties":
			if out[k], err = resolveValue(x, seen); err != nil {
				return nil, err
			}
		case "oneOf", "anyOf":
			list, _ := x.([]any)
			res := make([]any, len(list))
			for i, b := range list {
				if res[i], err = resolveValue(b, seen); err != nil {
					return nil, err
				}
			}
			out[k] = res
		default:
			out[k] = x
		}
	}
	all, _ := m["allOf"].([]any)
	for _, b := range all {
		rb, err := resolveValue(b, seen)
		if err != nil {
			return nil, err
		}
		if bm, ok := rb.(map[string]any); ok {
			mergeSchema(out, bm)
		}
	}
	return out, nil
}

// resolveRef resolves a $ref to a component schema.
func resolveRef(ref string, seen []string) (any, error) {
	name, ok := strings.CutPrefix(ref, componentPrefix)
	if !ok {
		return nil, fmt.Errorf("nicht unterstützte Schema-Referenz %s", strconv.Quote(ref))
	}
	if slices.Contains(seen, name) {
		return nil, fmt.Errorf("zyklische Schema-Referenz %s", strconv.Quote(ref))
	}
	sr := spec.Doc().Components.Schemas[name]
	if sr == nil || sr.Value == nil {
		return nil, fmt.Errorf("unbekannte Schema-Referenz %s", strconv.Quote(ref))
	}
	v, err := schemaJSON(sr.Value)
	if err != nil {
		return nil, err
	}
	return resolveValue(v, append(slices.Clone(seen), name))
}

// mergeSchema merges an allOf branch src into dst: properties are united
// (a property in both is merged the same way), required lists united in
// order; for every other keyword the value already in dst wins (the
// schema's own keywords, then earlier branches).
func mergeSchema(dst, src map[string]any) {
	for k, v := range src {
		switch k {
		case "properties":
			props, _ := dst[k].(map[string]any)
			if props == nil {
				props = map[string]any{}
				dst[k] = props
			}
			for name, p := range v.(map[string]any) {
				old, oldOK := props[name].(map[string]any)
				pm, pOK := p.(map[string]any)
				if oldOK && pOK {
					mergeSchema(old, pm)
					continue
				}
				if _, exists := props[name]; !exists {
					props[name] = p
				}
			}
		case "required":
			req, _ := dst[k].([]any)
			for _, r := range v.([]any) {
				if !slices.Contains(req, r) {
					req = append(req, r)
				}
			}
			dst[k] = req
		default:
			if _, exists := dst[k]; !exists {
				dst[k] = v
			}
		}
	}
}
