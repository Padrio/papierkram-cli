package cli

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// builderFlags are the flags create/update commands get besides the field
// flags of the request schema; all are in spec.ReservedFlags. --item and
// --attr are the flags of the line item and custom attribute fields.
var builderFlags = []string{"input", "item", "items", "replace-items", "attr"}

// replaceHint explains why line items need --replace-items (spec 6.3a).
const replaceHint = "Positionen haben im Request keine ID: jedes Senden von line_items ersetzt alle Positionen. " +
	"Positionen ergänzen: describe <id> -o json | jq … | update <id> --input - --replace-items"

// isWrite reports a create or update entry, built from the request schema.
func isWrite(e registry.Entry) bool { return e.Action == "create" || e.Action == "update" }

// generic reports whether the generic builder makes the command of e: not
// Custom, and for create/update a JSON request body. Multipart uploads
// (UploadVoucherDocument) have their own handler.
func generic(e registry.Entry) bool {
	if e.Custom {
		return false
	}
	return !isWrite(e) || jsonBody(e.OperationID)
}

// jsonBody reports an operation with an application/json request body.
func jsonBody(opID string) bool {
	op, ok := spec.OperationByID(opID)
	if !ok || op.Op.RequestBody == nil || op.Op.RequestBody.Value == nil {
		return false
	}
	return op.Op.RequestBody.Value.Content.Get("application/json") != nil
}

// resourceNames are the registry resource names; spec.Fields uses them to
// name nested references (--customer-project).
func resourceNames() []string {
	var names []string
	for _, r := range registry.Resources() {
		names = append(names, r.Name)
	}
	return names
}

// writeSpec is the request schema of a create/update entry with its fields.
type writeSpec struct {
	req    *openapi3.Schema
	fields []spec.Field
	items  *spec.Field // the line items field, nil without one
	err    error       // the fields cannot be derived (flag collision)
}

func newWriteSpec(e registry.Entry) writeSpec {
	ws := writeSpec{req: spec.RequestSchema(e.OperationID)}
	ws.fields, ws.err = spec.Fields(ws.req, resourceNames())
	for i := range ws.fields {
		if ws.fields[i].Kind == spec.KindItems {
			ws.items = &ws.fields[i]
		}
	}
	return ws
}

// itemSchema is the schema of one line item.
func (ws writeSpec) itemSchema() *openapi3.Schema {
	if ws.items == nil || ws.items.Schema.Items == nil {
		return nil
	}
	return ws.items.Schema.Items.Value
}

// rawValue is the value of a scalar field flag: the text as given, converted
// by spec.CoerceFlag when the command runs, so invalid values are German
// validation errors naming the flag (exit 5). typ is the placeholder type in
// the help (see germanTypeNames); "bool" makes a switch (--flagged,
// --flagged=false, also ja/nein).
type rawValue struct {
	s, typ string
}

func (v *rawValue) String() string {
	if v.typ == "bool" && v.s == "" {
		return "false"
	}
	return v.s
}

func (v *rawValue) Set(s string) error { v.s = s; return nil }
func (v *rawValue) Type() string       { return v.typ }

// flagType is the rawValue type of a scalar field.
func flagType(f spec.Field) string {
	switch {
	case f.IsBool():
		return "bool"
	case f.Kind == spec.KindRef || f.Kind == spec.KindNestedRef:
		return "id"
	case f.Format == "date":
		return "date"
	case f.Type == "integer" || f.Type == "number":
		return f.Type
	}
	return "string"
}

// addWriteFlags registers the flags of a create/update command (spec 6.1):
// one flag per field of the request schema, --input, and with line items
// --items (plus --replace-items on update). Required fields are not
// enforced by flags: Validate reports them after --input is merged.
func addWriteFlags(cmd *cobra.Command, e registry.Entry) {
	ws := newWriteSpec(e)
	if ws.err != nil {
		return // runWrite reports it
	}
	fs := cmd.Flags()
	fs.String("input", "", "Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder")
	for _, f := range ws.fields {
		if f.Flag == "" {
			continue
		}
		usage := f.Help
		if f.Required {
			usage += " (Pflicht)"
		}
		if f.Repeatable {
			fs.StringArray(f.Flag, nil, usage)
			continue
		}
		fl := fs.VarPF(&rawValue{typ: flagType(f)}, f.Flag, "", usage)
		if f.IsBool() {
			fl.NoOptDefVal = "true"
		}
	}
	if ws.items != nil {
		fs.String("items", "", "Positionen als JSON-Array aus Datei oder - für die Standardeingabe")
		if e.Action == "update" {
			fs.Bool("replace-items", false, "alle Positionen ersetzen (nötig für --item/--items; ohne Positionen werden alle gelöscht)")
		}
	}
}

// runWrite sends create or update (spec 6, 6.3a). Order: arguments and flag
// combinations (exit 2), configuration, body (buildBody: exit 2/5), the
// mutation hook, then the request. Output is the API object; an empty
// response prints {"id":N,"ok":true} on update and {"ok":true} with a
// warning on create.
func (a *App) runWrite(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := pathValues(c, e, args)
	if err != nil {
		return err
	}
	isUpdate := e.Action == "update"
	ws := newWriteSpec(e)
	if ws.err != nil {
		return errUnavailable(ws.err)
	}
	if err := checkWriteFlags(c, ws, isUpdate); err != nil {
		return err
	}
	client, r, err := a.Client(c)
	if err != nil {
		return err
	}
	body, _, err := a.buildBody(c, e, isUpdate)
	if err != nil {
		return err
	}
	if err := a.beforeMutation(c, e, r, params, body); err != nil {
		return err
	}
	resp, err := client.Call(c.Context(), e.OperationID, params, nil, body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(resp.Body)) > 0 {
		return a.render(c, resp.Body, e.Columns)
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	var result any = struct {
		OK bool `json:"ok"`
	}{true}
	text := e.Resource + " angelegt.\n"
	if isUpdate {
		id := params[len(params)-1]
		result = okJSON{ID: id, OK: true}
		text = fmt.Sprintf("%s %d aktualisiert.\n", e.Resource, id)
	} else {
		output.Warn(a.Err, "API lieferte kein Objekt; angelegtes Objekt per list prüfen")
	}
	if ownText(o) {
		_, err := io.WriteString(a.Out, text)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, result, output.Columns{}))
}

// flagString is the value of a string flag, "" when the command has none.
func flagString(c *cobra.Command, name string) string {
	v, _ := c.Flags().GetString(name)
	return v
}

func flagBool(c *cobra.Command, name string) bool {
	v, _ := c.Flags().GetBool(name)
	return v
}

// checkWriteFlags runs every usage check of create/update that needs only
// the flags, before the configuration is resolved and before anything is
// read or sent (exit 2): flag combinations, an empty --input/--items path,
// and the --item values (syntax, keys, object fields; see parseItems).
func checkWriteFlags(c *cobra.Command, ws writeSpec, isUpdate bool) error {
	item, items := c.Flags().Changed("item"), c.Flags().Changed("items")
	for _, flag := range []string{"input", "items"} {
		if c.Flags().Changed(flag) && flagString(c, flag) == "" {
			return errMissingPath("--" + flag)
		}
	}
	switch {
	case item && items:
		return clierr.New(clierr.CodeUsage, "--item und --items nicht zusammen verwenden",
			"Positionen entweder einzeln per --item oder als JSON-Array per --items angeben")
	case flagString(c, "input") == "-" && flagString(c, "items") == "-":
		return clierr.New(clierr.CodeUsage, "--input - und --items - lesen beide die Standardeingabe",
			"nur eines davon mit - angeben, das andere als Datei")
	case isUpdate && (item || items) && !flagBool(c, "replace-items"):
		flag := "--item"
		if items {
			flag = "--items"
		}
		return clierr.New(clierr.CodeUsage, flag+" ersetzt alle Positionen; dafür zusätzlich --replace-items angeben", replaceHint)
	case item:
		raws, _ := c.Flags().GetStringArray(ws.items.Flag)
		_, err := parseItems(raws, ws.itemSchema())
		return err
	}
	return nil
}

// errUnavailable reports a command whose fields cannot be derived.
func errUnavailable(err error) error {
	return clierr.New(clierr.CodeError, "Befehl nicht verfügbar: "+err.Error(), "")
}

// buildBody builds the request body of create/update (spec 6.1–6.4, 6.3a):
// --input (pruned against the request schema, hints on stderr) → the
// changed field flags at their path (references as {id}, nested objects
// created as needed, maps merged) → --attr into custom_attributes → line
// items (--items pruned like --input, or --item) → CoerceValue → Validate.
// It returns the body and the response-only fields removed from --input and
// --items.
//
// --attr values are sent as strings: converting by the attribute's
// data_type would need the definition, which costs a request (spec 6.1:
// "sonst als String"). On update only what is given is sent, and positions
// only with --replace-items (spec 6.3a): without it line_items from --input
// are left out with a hint, so describe | jq '.name="Neu"' | update
// --input - renames without replacing the positions; with it they are sent
// as given, and --replace-items without positions sends line_items: []
// (R18). On create line_items from --input are always sent.
func (a *App) buildBody(c *cobra.Command, e registry.Entry, isUpdate bool) (map[string]any, []string, error) {
	ws := newWriteSpec(e)
	if ws.err != nil {
		return nil, nil, errUnavailable(ws.err)
	}
	resp := spec.ResourceResponseSchema(e.OperationID)
	replace := flagBool(c, "replace-items")
	body := map[string]any{}
	var removed []string
	if c.Flags().Changed("input") {
		in, err := a.readInput(flagString(c, "input"))
		if err != nil {
			return nil, nil, err
		}
		key := itemsKey(ws)
		if _, ok := in[key]; ok && key != "" && isUpdate && !replace {
			// dropped before Prune: no hints or errors about positions
			// that are not sent
			delete(in, key)
			_, _ = fmt.Fprintf(a.Err, "Hinweis: %s aus --input nicht gesendet – Positionen bleiben unverändert; zum Ersetzen --replace-items angeben\n", key)
		}
		pr, err := spec.Prune(in, ws.req, resp)
		if err != nil {
			return nil, nil, prefixErr("--input: ", err)
		}
		// null positions with --replace-items: an error whether Prune
		// dropped the null or kept it (nullable on update), never a
		// silent []
		if v, ok := pr.Body[key]; key != "" && replace && ((ok && v == nil) || slices.Contains(pr.DroppedNulls, key)) {
			return nil, nil, clierr.New(clierr.CodeValidation, key+": null ist nicht erlaubt – für leere Positionsliste [] angeben", "")
		}
		a.pruneHints(pr)
		body, removed = pr.Body, pr.Removed
	}

	cc := spec.CoerceContext{Now: a.Now, Loc: a.Location}
	for _, f := range ws.fields {
		if f.Flag == "" || f.Kind == spec.KindItems || !c.Flags().Changed(f.Flag) {
			continue
		}
		switch f.Kind {
		case spec.KindAttrs:
			raws, _ := c.Flags().GetStringArray(f.Flag)
			for _, raw := range raws {
				v, err := spec.CoerceFlag(cc, f, raw)
				if err != nil {
					return nil, nil, err
				}
				setPath(body, f.Path, v) // merges into custom_attributes
			}
		case spec.KindRefList, spec.KindScalarList:
			raws, _ := c.Flags().GetStringArray(f.Flag)
			list := make([]any, 0, len(raws))
			for _, raw := range raws {
				v, err := spec.CoerceFlag(cc, f, raw)
				if err != nil {
					return nil, nil, err
				}
				list = append(list, v)
			}
			setPath(body, f.Path, list)
		default:
			v, err := spec.CoerceFlag(cc, f, rawText(c, f.Flag))
			if err != nil {
				return nil, nil, err
			}
			setPath(body, f.Path, v)
		}
	}

	if ws.items != nil {
		items, gone, err := a.lineItems(c, ws, resp)
		if err != nil {
			return nil, nil, err
		}
		removed = append(removed, gone...)
		switch {
		case items != nil:
			setPath(body, ws.items.Path, items)
		case replace && !hasKey(body, itemsKey(ws)):
			setPath(body, ws.items.Path, []any{})
		}
	}
	if isUpdate && len(body) == 0 {
		return nil, nil, clierr.New(clierr.CodeUsage, "keine Änderungen angegeben",
			"mindestens ein Feld-Flag, --input oder --replace-items angeben; Verwendung: "+useLine(c))
	}

	v, err := spec.CoerceValue(ws.req, body, "")
	if err != nil {
		return nil, nil, err
	}
	body, _ = v.(map[string]any)
	if err := spec.Validate(body, ws.req); err != nil {
		return nil, nil, err
	}
	return body, removed, nil
}

func hasKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

// itemsKey is the body key of the line items, "" without them.
func itemsKey(ws writeSpec) string {
	if ws.items == nil {
		return ""
	}
	return ws.items.Path[0]
}

// rawText is the text given to a scalar field flag, exactly as typed
// (--flagged= is "", not "false").
func rawText(c *cobra.Command, flag string) string {
	if v, ok := c.Flags().Lookup(flag).Value.(*rawValue); ok {
		return v.s
	}
	return c.Flags().Lookup(flag).Value.String()
}

// readInput reads the --input body: a JSON object from a file or stdin.
func (a *App) readInput(path string) (map[string]any, error) {
	v, err := readJSON("--input", path, a.In)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, clierr.New(clierr.CodeValidation, "--input: Eingabe muss ein JSON-Objekt sein", `z. B. {"name": "Beispiel"}`)
	}
	return m, nil
}

// pruneHints prints which fields Prune removed or whose null it dropped.
func (a *App) pruneHints(pr spec.PruneResult) {
	if len(pr.Removed) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: nur lesbare Felder entfernt: %s\n", strings.Join(pr.Removed, ", "))
	}
	if len(pr.DroppedNulls) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: null ignoriert (Feld bleibt unverändert): %s\n", strings.Join(pr.DroppedNulls, ", "))
	}
}

// setPath sets v at path in body, creating the objects on the way. A map
// value is merged into a map already there, so --customer keeps
// customer.contact_person and --attr adds to custom_attributes.
func setPath(body map[string]any, path []string, v any) {
	m := body
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[k] = next
		}
		m = next
	}
	last := path[len(path)-1]
	if nv, ok := v.(map[string]any); ok {
		if old, ok := m[last].(map[string]any); ok {
			for k, x := range nv {
				old[k] = x
			}
			return
		}
	}
	m[last] = v
}
