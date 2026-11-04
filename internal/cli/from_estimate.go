package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// invoice create --from-estimate <id> (spec 9.4): read the estimate, build
// the invoice body from it, let every flag override it, then create the
// invoice like invoice create does.

// estimateField maps a field of the estimate (GetEstimate,
// EstimateWithDetailedEmbeds) to a field of the invoice request
// (CreateInvoice), both as dotted paths. A source that is an object is
// copied field by field: only the fields the target object has in the
// request schema. Nulls are sent only where the request allows them (see
// copyFields).
type estimateField struct{ from, to string }

// fromEstimateFields are the invoice fields taken from the estimate (spec
// 9.4, checked against both schemas by TestFromEstimateMappingMatchesSchemas).
// The estimate has its references on top level (contact_person, project);
// the request nests them below customer, so they are only sent with the
// customer ID. The positions are mapped by fromEstimateItemFields.
var fromEstimateFields = []estimateField{
	{"name", "name"},
	{"description", "description"},
	{"gross", "gross"},
	{"billing", "billing"},
	{"customer.id", "customer.id"},
	{"contact_person.id", "customer.contact_person.id"},
	{"project.id", "customer.project.id"},
}

// fromEstimateItemFields are the fields of a position taken from the
// estimate (spec 9.4). vat_rate comes as "19%" and is normalized to 0.19
// by the coercion of the request body (spec.NormalizeVAT).
var fromEstimateItemFields = []estimateField{
	{"name", "name"},
	{"description", "description"},
	{"quantity", "quantity"},
	{"unit", "unit"},
	{"price", "price"},
	{"price_gross", "price_gross"},
	{"discount_percentage", "discount_percentage"},
	{"discount_calculated", "discount_calculated"},
	{"discount_calculated_gross", "discount_calculated_gross"},
	{"proposition.id", "proposition.id"},
	{"type_of", "type_of"},
	{"vat_rate", "vat_rate"},
}

// invoiceOnlyFields are the invoice request fields the estimate has no
// value for: payment_term (required, --payment-term), the dates, flagged,
// and custom_template (the estimate's template is one for estimates;
// invoices have their own, GET /info settings.custom_templates.invoices).
var invoiceOnlyFields = []string{"payment_term", "document_date", "supply_date", "flagged", "custom_template"}

// invoiceOnlyItemFields are the position fields the estimate has no value
// for (its position_in_group is no group start).
var invoiceOnlyItemFields = []string{"start_new_item_group"}

// estimateOnlyFields are the writable-looking estimate fields that have no
// place in the invoice request; the computed and read-only fields (id,
// estimate_no, state, totals, …) are not listed.
var estimateOnlyFields = []string{"document_date", "custom_template", "document_charges",
	"line_items.document_charge", "line_items.document_charge_gross", "line_items.position_in_group"}

const fromEstimateLong = `

--from-estimate <id> liest das Angebot (1 weiterer Credit) und übernimmt daraus name, description, gross, billing, customer (mit contact_person und project des Angebots) und alle Positionen (name, description, quantity, unit, price, price_gross, discount_percentage, discount_calculated, discount_calculated_gross, proposition, type_of, vat_rate; "19%" wird zu 0.19). --payment-term ist Pflicht, weil Angebote keine Zahlungsbedingung haben.

Alle anderen Flags überschreiben die Werte des Angebots; --item/--items ersetzen alle Positionen. Nennt --customer einen anderen Kunden als das Angebot, werden contact_person und project des Angebots nicht übernommen (Hinweis auf stderr); sie lassen sich per --contact-person bzw. --customer-project setzen. --input ist zusammen mit --from-estimate nicht möglich.

Nicht übernommen werden: document_date (Rechnungsdatum per --document-date), custom_template (Angebotsvorlagen gelten nicht für Rechnungen; --custom-template), document_charges und je Position document_charge, document_charge_gross und position_in_group. supply_date, flagged und start_new_item_group hat das Angebot nicht. Leere Werte (null) des Angebots werden übernommen, wo die Rechnung sie erlaubt, sonst weggelassen und auf stderr genannt.

--dry-run liest das Angebot ebenfalls (1 Credit) und zeigt den Body, ohne die Rechnung anzulegen. Scheitert das Lesen, wird nichts angelegt.`

// addFromEstimate adds --from-estimate to invoice create (spec 9.4).
// Without it the command runs runWrite unchanged.
func (a *App) addFromEstimate(e registry.Entry, cmd *cobra.Command) {
	read, ok := registry.Lookup("estimate", "describe")
	if !ok || read.Class != registry.ClassReadOnly {
		// fail closed: the read must be a plain GET without guards
		panic(fmt.Sprintf("registry: %s create --from-estimate ohne lesenden Eintrag estimate describe", e.Resource))
	}
	cmd.Long += fromEstimateLong
	cmd.Flags().Var(&rawValue{typ: "id"}, "from-estimate", "Angebot, aus dem die Rechnung entsteht (liest es: 1 weiterer Credit)")
	write := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if !c.Flags().Changed("from-estimate") {
			return write(c, args)
		}
		return a.runWriteThen(c, e, args, writeSteps{from: &fromEstimate{read: read, raw: rawText(c, "from-estimate")}})
	}
}

// fromEstimate is the estimate an invoice is created from: the read entry
// (estimate describe) and the ID as given.
type fromEstimate struct {
	read registry.Entry
	raw  string
	id   int64
}

// paymentTermFlag is the flag of the required payment_term.
const paymentTermFlag = "payment-term"

// check runs the usage checks of --from-estimate (exit 2), before anything
// is resolved or read: an ID like describe takes it (integer ≥ 0, 0 is
// valid, R24), no --input (the body comes from the estimate) and
// --payment-term (estimates have none).
func (f *fromEstimate) check(c *cobra.Command) error {
	id, ok := parseID(f.raw)
	if !ok {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige ID %q für --from-estimate: ganze Zahl ≥ 0 erwartet", f.raw), "Verwendung: "+useLine(c))
	}
	f.id = id
	if c.Flags().Changed("input") {
		return clierr.New(clierr.CodeUsage, "--from-estimate und --input nicht zusammen verwenden",
			"der Body kommt aus dem Angebot; einzelne Felder per Flag überschreiben")
	}
	if !c.Flags().Changed(paymentTermFlag) {
		return clierr.New(clierr.CodeUsage, "Pflichtangabe --payment-term fehlt: Angebote haben keine Zahlungsbedingung",
			"Zahlungsbedingungen: papierkram payment-term list; Verwendung: "+useLine(c)+" --from-estimate <id> --payment-term <id>")
	}
	return nil
}

// params are the path values of the read.
func (f *fromEstimate) params() []int64 { return []int64{f.id} }

// load reads the estimate (like estimate describe: same operation and
// path values) and maps it to the invoice body (estimateBody). The answer
// is decoded with exact numbers: the generated model has float32 prices
// and quantities, which would turn 19.99 into 19.989999771118164. A failed
// read keeps its code (404: exit 4) and says that nothing was created.
func (f *fromEstimate) load(c *cobra.Command, client *api.Client, req *openapi3.Schema) (map[string]any, []string, error) {
	resp, err := client.Call(c.Context(), f.read.OperationID, f.params(), nil, nil)
	if err != nil {
		ce := *clierr.From(err)
		ce.Message = fmt.Sprintf("Angebot %d nicht gelesen, keine Rechnung angelegt: %s", f.id, ce.Message)
		ce.Cause = err
		return nil, nil, &ce
	}
	var est map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if dec.Decode(&est) != nil || est == nil {
		return nil, nil, clierr.New(clierr.CodeError, fmt.Sprintf("Angebot %d: Antwort ist kein JSON-Objekt, keine Rechnung angelegt", f.id), "")
	}
	body, nulls := estimateBody(est, req)
	return body, nulls, nil
}

// estimateBase is the base body of the invoice: the estimate read by
// load, without the references of its customer when --customer names
// another one (dropForeignRefs). The nulls left out are named with the
// hint of the --input path (pruneHints).
func (a *App) estimateBase(c *cobra.Command, client *api.Client, ws writeSpec, f *fromEstimate) (map[string]any, error) {
	base, nulls, err := f.load(c, client, ws.req)
	if err != nil {
		return nil, err
	}
	a.pruneHints(spec.PruneResult{DroppedNulls: nulls})
	a.dropForeignRefs(c, ws, base)
	return base, nil
}

// customerRefs are the references below customer that belong to the
// estimate's customer.
var customerRefs = []string{"contact_person", "project"}

// dropForeignRefs removes the estimate's contact person and project from
// base when --customer names another customer than the estimate's: they
// belong to the estimate's customer. Their own flags still set them; for
// the others a hint names what was dropped and the flags that set it. The
// same customer keeps both.
func (a *App) dropForeignRefs(c *cobra.Command, ws writeSpec, base map[string]any) {
	flag := fieldFlag(ws, "customer")
	cust, ok := base["customer"].(map[string]any)
	if flag == "" || !c.Flags().Changed(flag) || !ok {
		return
	}
	given := strings.TrimSpace(rawText(c, flag))
	newID, okNew := parseID(given)
	oldID, okOld := parseID(fmt.Sprint(cust["id"]))
	if okNew && okOld && newID == oldID {
		return
	}
	var dropped, flags []string
	for _, k := range customerRefs {
		if _, has := cust[k]; !has {
			continue
		}
		delete(cust, k)
		f := fieldFlag(ws, "customer", k)
		if f != "" && c.Flags().Changed(f) {
			continue // set by its flag
		}
		dropped = append(dropped, k)
		flags = append(flags, "--"+f)
	}
	if len(dropped) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: Kunde %s statt %v aus dem Angebot: %s nicht übernommen (setzen per %s)\n",
			given, cust["id"], strings.Join(dropped, " und "), strings.Join(flags, " bzw. "))
	}
}

// fieldFlag is the flag of the field at path, "" without one.
func fieldFlag(ws writeSpec, path ...string) string {
	for _, f := range ws.fields {
		if slices.Equal(f.Path, path) {
			return f.Flag
		}
	}
	return ""
}

// readInfo is the read as --dry-run shows it: it has been sent.
func (f *fromEstimate) readInfo(client *api.Client) (*ReadInfo, error) {
	method, url, err := client.Preview(f.read.OperationID, f.params(), nil)
	if err != nil {
		return nil, err
	}
	return &ReadInfo{Method: method, URL: url, Credits: f.read.Credits}, nil
}

// estimateBody maps the estimate est to the invoice body by
// fromEstimateFields and fromEstimateItemFields. Positions that are no
// object are passed on as they are, for the validation to report. nulls
// are the paths of the nulls left out (see copyFields), sorted, positions
// as line_items[] like Prune names them.
func estimateBody(est map[string]any, req *openapi3.Schema) (body map[string]any, nulls []string) {
	fields := fromEstimateFields
	if id, ok := lookupPath(est, "customer.id"); !ok || id == nil {
		// contact person and project are references below the customer
		fields = slices.DeleteFunc(slices.Clone(fields), func(f estimateField) bool { return strings.HasPrefix(f.to, "customer.") })
	}
	body = map[string]any{}
	dropped := map[string]bool{}
	copyFields(body, est, fields, req, "", dropped)
	items, ok := est["line_items"].([]any)
	if !ok {
		return body, slices.Sorted(maps.Keys(dropped))
	}
	itemReq := itemsSchema(req)
	list := make([]any, len(items))
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			list[i] = it
			continue
		}
		out := map[string]any{}
		copyFields(out, m, fromEstimateItemFields, itemReq, "line_items[].", dropped)
		list[i] = out
	}
	body["line_items"] = list
	return body, slices.Sorted(maps.Keys(dropped))
}

// itemsSchema is the schema of one line item of req, nil without one.
func itemsSchema(req *openapi3.Schema) *openapi3.Schema {
	p := req.Properties["line_items"]
	if p == nil || p.Value == nil || p.Value.Items == nil {
		return nil
	}
	return p.Value.Items.Value
}

// copyFields copies the fields of src to dst by fields; an object goes
// field by field into the fields of its target in req. A missing value is
// not sent; a null is sent where the request allows null (an empty
// billing field stays empty, like --input after Prune) and left out
// otherwise (the request cannot express it, e.g. a null vat_rate), its
// target path (after prefix) added to dropped. A missing reference
// (proposition: null) has no value to lose and is not added.
func copyFields(dst, src map[string]any, fields []estimateField, req *openapi3.Schema, prefix string, dropped map[string]bool) {
	for _, f := range fields {
		v, ok := lookupPath(src, f.from)
		if !ok {
			continue
		}
		to := strings.Split(f.to, ".")
		ts := targetSchema(req, to)
		switch obj, isObj := v.(map[string]any); {
		case v == nil:
			if ts == nil || !ts.Nullable {
				dropped[prefix+f.to] = true
				continue
			}
		case isObj:
			if obj = objectFields(obj, ts, prefix+f.to+".", dropped); len(obj) == 0 {
				continue
			}
			v = obj
		}
		setPath(dst, to, v)
	}
}

// objectFields are the fields of obj that s has, nulls only where s
// allows them; the others are added to dropped (after prefix).
func objectFields(obj map[string]any, s *openapi3.Schema, prefix string, dropped map[string]bool) map[string]any {
	out := map[string]any{}
	if s == nil {
		return out
	}
	for k, v := range obj {
		p := s.Properties[k]
		if p == nil || p.Value == nil {
			continue
		}
		if v == nil && !p.Value.Nullable {
			dropped[prefix+k] = true
			continue
		}
		out[k] = v
	}
	return out
}

// targetSchema is the schema at path below req, nil when missing.
func targetSchema(req *openapi3.Schema, path []string) *openapi3.Schema {
	s := req
	for _, k := range path {
		if s == nil {
			return nil
		}
		p := s.Properties[k]
		if p == nil {
			return nil
		}
		s = p.Value
	}
	return s
}

// lookupPath is the value at the dotted path in m; ok is false when a
// segment is missing or not an object.
func lookupPath(m map[string]any, path string) (any, bool) {
	var v any = m
	for _, k := range strings.Split(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = obj[k]; !ok {
			return nil, false
		}
	}
	return v, true
}
