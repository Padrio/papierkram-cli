package cli

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// builderFlags are the flags create/update commands get besides the field
// flags of the request schema; spec.Fields reserves all of them. --item and
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
	case f.Kind == spec.KindRef || f.Kind == spec.KindNestedRef || f.Kind == spec.KindRefList:
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
		if f.Flag == e.Preset {
			usage += "; ohne Angabe " + presetNote(e.Preset)
		}
		if f.Required {
			usage += " (Pflicht)"
		}
		if f.Repeatable {
			// a text list; its elements are values of the field type
			// (--team-member: IDs), checked by spec.CoerceFlag
			fs.StringArray(f.Flag, nil, usage)
			annotate(fs, f.Flag, flagValueType, flagType(f))
		} else if fl := fs.VarPF(&rawValue{typ: flagType(f)}, f.Flag, "", usage); f.IsBool() {
			fl.NoOptDefVal = "true"
		}
		if flagType(f) == "id" {
			markResolves(fs, f.Flag)
		}
		if f.Required {
			// --input (merged before Validate) and, for the line items,
			// --items give the value as well
			markRequired(fs, f.Flag)
			addAlternatives(fs, f.Flag, "--input")
			if f.Kind == spec.KindItems {
				addAlternatives(fs, f.Flag, "--items")
			}
		}
		markEnum(fs, f.Flag, f.Enum)
		markSchemaBounds(fs, f.Flag, f.Schema)
		if f.Flag == e.Preset {
			// presetBody runs after the configuration is resolved
			markPreset(fs, f.Flag, e.Preset, true)
		}
	}
	if ws.items != nil {
		fs.String("items", "", "Positionen als JSON-Array aus Datei oder - für die Standardeingabe")
		if e.Action == "update" {
			fs.Bool("replace-items", false, "alle Positionen ersetzen (nötig für --item/--items; ohne Positionen werden alle gelöscht)")
		}
	}
}

// runWrite sends create or update (spec 6, 6.3a). Order (plan): arguments,
// flag combinations, name flags and a preset path flag (exit 2),
// configuration and read-only gate (mutationClient, exit 9, before
// anything is read), names (resolveNames, spec v0.2 §4), body
// (buildBodyFrom: exit 2/5), --dry-run and confirmation (beforeSend),
// then the request. Output is the API object; an empty
// response prints {"id":N,"ok":true} on update and {"ok":true} with a
// warning on create.
func (a *App) runWrite(c *cobra.Command, e registry.Entry, args []string) error {
	return a.runWriteThen(c, e, args, writeSteps{})
}

// writeSteps are the optional steps a generic create adds around its
// request; without any (the zero value) runWriteThen is runWrite.
//   - up uploads a file after the create (voucher create --file, see
//     createUpload). The file is checked with the flags (exit 2, before
//     anything is resolved or sent); --dry-run shows the create, the upload
//     as a second request and the credits of both; upload and create share
//     their class (checkCreateFile), so the confirmation of create is that
//     of the run. After the create the file goes to the new object
//     (uploadCreated).
//   - from reads the estimate the body is built from (invoice create
//     --from-estimate, see fromEstimate). Its flags are checked with the
//     others (exit 2); the read comes after the read-only gate and after
//     the field flags and line items are checked, and replaces --input as
//     the base the flags override (buildBodyFrom). --dry-run reads too and
//     names the read; its credits are those of the create.
//   - keep reads the invoice when the body of invoice update has no
//     payment_term and sends its current one (keepPaymentTerm, ruling
//     E2E-A); the same read, once, keeps contact person and project when
//     --customer names the invoice's customer without their flags (open
//     point 29) and drops them for another one. It runs after the body is
//     built and validated, then counts its credits and shows the read in
//     --dry-run like from.
type writeSteps struct {
	up   *createUpload
	from *fromEstimate
	keep *keepPaymentTerm
}

// runWriteThen is runWrite with the steps s.
func (a *App) runWriteThen(c *cobra.Command, e registry.Entry, args []string, s writeSteps) error {
	isUpdate := e.Action == "update"
	ws := newWriteSpec(e)
	if ws.err != nil {
		return errUnavailable(ws.err)
	}
	if err := checkWriteFlags(c, ws, isUpdate); err != nil {
		return err
	}
	if s.from != nil {
		if err := s.from.check(c, ws); err != nil {
			return err
		}
	}
	if err := checkNameFlags(c); err != nil {
		return err
	}
	// after the flag checks, before the read-only gate (exit 2 first): a
	// preset path flag (person create --company) reads environment,
	// project file and configuration, whose errors must not hide a flag
	// error. The body is built later: with a project file of another
	// context and an invalid body, the warning and the missing --company
	// (exit 2) come first, the body error (exit 5) is not reached.
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	up := s.up
	run, extras := e, sendExtras{}
	if up != nil {
		if err := up.open(); err != nil {
			return err
		}
		defer up.close()
		run = up.run(e)
	}
	// create and upload have the same class (checkCreateFile): the gate of
	// e is that of the whole run
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	// names after the gate and before the body (spec v0.2 §4): the body,
	// the reads of --from-estimate and of invoice update see IDs only
	if extras.resolved, err = a.resolvePathNames(c, e, client, r, params); err != nil {
		return err
	}
	if up != nil {
		extras.upload = up.dryRunStep(client)
	}
	var load func() (map[string]any, error)
	if s.from != nil {
		// the real call reads the estimate again: like the upload of
		// create --file and the readback of deliver, its credits belong
		// to the run (the read of a dry run is shown as spent on its own)
		run.Credits += s.from.read.Credits
		load = func() (map[string]any, error) { return a.estimateBase(c, client, ws, s.from) }
		if extras.read, err = s.from.readInfo(client); err != nil {
			return err
		}
	}
	body, info, err := a.buildBodyFrom(c, e, isUpdate, load)
	if err != nil {
		if s.from != nil && s.from.loaded {
			return s.from.bodyErr(err)
		}
		return err
	}
	if s.keep != nil {
		read, err := s.keep.apply(c, a, client, params, ws, body, info.foreignInput)
		if err != nil {
			return err
		}
		if read != nil {
			run.Credits += read.Credits
			extras.read = read
		}
	}
	a.flushPresetHints()
	if proceed, err := a.beforeSend(c, run, client, params, body, extras); !proceed || err != nil {
		return err
	}
	resp, err := client.Call(c.Context(), e.OperationID, params, nil, body)
	if err != nil {
		if up != nil && errors.Is(err, api.ErrResponseTooLarge) {
			// created, but the ID is in the unread answer: the upload
			// cannot follow (exit 11 without ID)
			return a.uploadPartial(a.outputOptions(c.Context(), a.outputFormat(r)), e, up, nil, err)
		}
		return err
	}
	if up != nil {
		return a.uploadCreated(c, e, client, r, resp, up)
	}
	return a.writeResult(c, e, r, params, resp.Body)
}

// writeResult prints the answer of a sent create or update (e): the API
// object, or for an empty answer {"id":N,"ok":true} on update and
// {"ok":true} with a warning on create. time log shares it with create.
func (a *App) writeResult(c *cobra.Command, e registry.Entry, r *config.Resolved, params []int64, body []byte) error {
	isUpdate := e.Action == "update"
	if len(bytes.TrimSpace(body)) > 0 {
		return a.render(c, body, e.Columns)
	}
	if isUpdate {
		id := lastID(params)
		return a.emptyOK(c, r, okJSON{ID: id, OK: true}, fmt.Sprintf("%s %d aktualisiert.", e.Resource, id))
	}
	output.Warn(a.Err, "API lieferte kein Objekt; angelegtes Objekt per list prüfen")
	return a.emptyOK(c, r, struct {
		OK bool `json:"ok"`
	}{true}, e.Resource+" angelegt.")
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
// read or sent (exit 2): flag combinations, an empty, missing or
// directory --input/--items path, and the --item values (syntax, keys,
// object fields; see parseItems).
func checkWriteFlags(c *cobra.Command, ws writeSpec, isUpdate bool) error {
	item, items := c.Flags().Changed("item"), c.Flags().Changed("items")
	for _, flag := range []string{"input", "items"} {
		if !c.Flags().Changed(flag) {
			continue
		}
		path := flagString(c, flag)
		if path == "" {
			return errMissingPath("--" + flag)
		}
		// a missing file is a flag error before the gate and the name
		// resolution (its list requests cost credits); read later
		if err := checkJSONPath("--"+flag, path); err != nil {
			return err
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

// buildBodyFrom builds the request body of create/update (spec 6.1–6.4, 6.3a):
// --input (pruned against the request schema, hints on stderr) → the
// changed field flags at their path (references as {id}, nested objects
// created as needed, maps merged) → --attr into custom_attributes → line
// items (--items pruned like --input, or --item) → CoerceValue → the
// preset field of create when still unset (spec 9.8) → Validate.
// It returns the body and what it learned on the way (bodyInfo): the
// response-only fields removed from --input and --items, and whether
// --customer names another customer than --input.
//
// --attr values are sent as strings: converting by the attribute's
// data_type would need the definition, which costs a request (spec 6.1:
// "sonst als String"). On update only what is given is sent, and positions
// only with --replace-items (spec 6.3a): without it line_items from --input
// are left out with a hint, so describe | jq '.name="Neu"' | update
// --input - renames without replacing the positions; with it they are sent
// as given, and --replace-items without positions sends line_items: []
// (R18). On create line_items from --input are always sent.
//
// load, when not nil, returns a base body used instead of --input
// (invoice create --from-estimate, spec 9.4). It runs after the field
// flags and line items are checked (their errors cost no read), then the
// flags and positions override the base like they override --input, and
// the base goes through the same coercion and validation.
func (a *App) buildBodyFrom(c *cobra.Command, e registry.Entry, isUpdate bool, load func() (map[string]any, error)) (map[string]any, bodyInfo, error) {
	ws := newWriteSpec(e)
	if ws.err != nil {
		return nil, bodyInfo{}, errUnavailable(ws.err)
	}
	resp := spec.ResourceResponseSchema(e.OperationID)
	replace := flagBool(c, "replace-items")
	body := map[string]any{}
	var removed, moved []string
	var foreign bool
	if load == nil && c.Flags().Changed("input") {
		in, err := a.readInput(flagString(c, "input"))
		if err != nil {
			return nil, bodyInfo{}, err
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
			return nil, bodyInfo{}, prefixErr("--input: ", err)
		}
		// null positions with --replace-items: an error whether Prune
		// dropped the null or kept it (nullable on update), never a
		// silent []; positions per --item/--items win like every flag
		// over --input
		itemFlags := ws.items != nil && (c.Flags().Changed("items") || c.Flags().Changed(ws.items.Flag))
		if v, ok := pr.Body[key]; key != "" && replace && !itemFlags && ((ok && v == nil) || slices.Contains(pr.DroppedNulls, key)) {
			return nil, bodyInfo{}, clierr.New(clierr.CodeValidation, key+": null ist nicht erlaubt – für leere Positionsliste [] angeben oder Positionen per --item/--items setzen", "")
		}
		a.pruneHints(pr)
		body, removed, moved = pr.Body, pr.Removed, pr.Moved
		// the moved references belong to the customer of the input, like
		// those of an estimate to its customer (--from-estimate)
		foreign = a.dropForeignRefs(c, ws, body, "--input", movedCustomerRefs(moved), nil)
	}

	sets, err := a.flagSets(c, e.Resource, ws)
	if err != nil {
		return nil, bodyInfo{}, err
	}
	var items []any
	if ws.items != nil {
		var gone []string
		if items, gone, err = a.lineItems(c, ws, resp); err != nil {
			return nil, bodyInfo{}, err
		}
		removed = append(removed, gone...)
	}
	if load != nil {
		if body, err = load(); err != nil {
			return nil, bodyInfo{}, err
		}
	}
	for _, fs := range sets {
		setPath(body, fs.path, fs.value)
	}
	a.movedHint(c, ws, body, moved)
	if ws.items != nil {
		switch {
		case items != nil:
			setPath(body, ws.items.Path, items)
		case replace && !hasKey(body, itemsKey(ws)):
			setPath(body, ws.items.Path, []any{})
		}
	}
	if isUpdate && len(body) == 0 {
		return nil, bodyInfo{}, clierr.New(clierr.CodeUsage, "keine Änderungen angegeben",
			"mindestens ein Feld-Flag, --input oder --replace-items angeben; Verwendung: "+useLine(c))
	}

	v, err := spec.CoerceValue(ws.req, body, "")
	if err != nil {
		return nil, bodyInfo{}, err
	}
	body, _ = v.(map[string]any)
	// spec 9.8: the preset field (task create --project) after --input and
	// the flags, which win, and after their checks, right before the
	// validation that needs it
	presetMissing, err := a.presetBody(c, e, ws, body)
	if err != nil {
		return nil, bodyInfo{}, err
	}
	if err := spec.Validate(body, ws.req); err != nil {
		if presetMissing {
			err = withPresetHint(err, e.Preset)
		}
		return nil, bodyInfo{}, err
	}
	return body, bodyInfo{removed: removed, foreignInput: foreign}, nil
}

// bodyInfo is what buildBodyFrom reports besides the body: removed are the
// response-only fields removed from --input and --items; foreignInput says
// that --input brought a customer and --customer names another one
// (dropForeignRefs), so the references of --input belong to the old
// customer.
type bodyInfo struct {
	removed      []string
	foreignInput bool
}

// fieldSet is the value of a field flag at its body path.
type fieldSet struct {
	path  []string
	value any
}

// flagSets converts the changed field flags of ws, in field order, to the
// values buildBodyFrom sets: references as {id}, repeatable flags as
// lists, each --attr as its own set (setPath merges them into
// custom_attributes). Invalid values are German validation errors naming
// the flag (exit 5). Relative dates follow the rule of resource
// (dayContext).
func (a *App) flagSets(c *cobra.Command, resource string, ws writeSpec) ([]fieldSet, error) {
	cc := a.dayContext(resource)
	var sets []fieldSet
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
					return nil, err
				}
				sets = append(sets, fieldSet{f.Path, v})
			}
		case spec.KindRefList, spec.KindScalarList:
			raws, _ := c.Flags().GetStringArray(f.Flag)
			list := make([]any, 0, len(raws))
			for _, raw := range raws {
				v, err := spec.CoerceFlag(cc, f, raw)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			}
			sets = append(sets, fieldSet{f.Path, list})
		default:
			v, err := spec.CoerceFlag(cc, f, rawText(c, f.Flag))
			if err != nil {
				return nil, err
			}
			sets = append(sets, fieldSet{f.Path, v})
		}
	}
	return sets, nil
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
	v, err := readJSON("--input", path, a.stdin())
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, clierr.New(clierr.CodeValidation, "--input: Eingabe muss ein JSON-Objekt sein", `z. B. {"name": "Beispiel"}`)
	}
	return m, nil
}

// pruneHints prints which fields Prune removed and whose null it dropped;
// the references it moved are named after the flags (movedHint).
func (a *App) pruneHints(pr spec.PruneResult) {
	if len(pr.Removed) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: nur lesbare Felder entfernt: %s\n", strings.Join(pr.Removed, ", "))
	}
	if len(pr.DroppedNulls) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: null ignoriert (Feld bleibt unverändert): %s\n", strings.Join(pr.DroppedNulls, ", "))
	}
}

// movedCustomerRefs are the keys of the references Prune moved below
// customer ("contact_person → customer.contact_person" → contact_person).
func movedCustomerRefs(moved []string) []string {
	var out []string
	for _, m := range moved {
		_, to, _ := strings.Cut(m, " → ")
		if k, ok := strings.CutPrefix(to, "customer."); ok && !strings.Contains(k, ".") {
			out = append(out, k)
		}
	}
	return out
}

// movedHint names the references Prune moved into the request that are
// sent: not dropped for another customer (dropForeignRefs) and not set by
// their own flag. It runs after the flags are applied.
func (a *App) movedHint(c *cobra.Command, ws writeSpec, body map[string]any, moved []string) {
	var kept []string
	for _, m := range moved {
		_, to, _ := strings.Cut(m, " → ")
		if flag := fieldFlag(ws, strings.Split(to, ".")...); flag != "" && c.Flags().Changed(flag) {
			continue
		}
		if _, ok := lookupPath(body, to); ok {
			kept = append(kept, m)
		}
	}
	if len(kept) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: Referenzen übernommen: %s\n", strings.Join(kept, ", "))
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
