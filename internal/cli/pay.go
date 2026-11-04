package cli

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// voucher pay <id> (spec 9.5, 8.1: class destructive, a payment cannot be
// taken back through the API).

// payFlags map the flags of pay to the fields of its request body
// (spec 9.5). Type, format, enum and the conversion of each value come
// from the field of the request schema (spec.Fields, spec.CoerceFlag).
var payFlags = []struct{ flag, field, help string }{
	{"amount", "value", "gezahlter Betrag mit Dezimalpunkt, z. B. 165.80"},
	{"transaction", "banking_transaction", "ID des Bankumsatzes, mit dem der Beleg bezahlt wurde (papierkram transaction list)"},
	{"date", "payment_date", "Zahlungsdatum"},
	{"reason", "difference_reason", "Grund, wenn der Betrag vom offenen Betrag abweicht"},
}

const payLong = `

Mindestens --amount oder --transaction ist nötig: --amount bucht eine Zahlung ohne Bankumsatz, --transaction verknüpft einen vorhandenen Bankumsatz. Weicht die Zahlung vom offenen Betrag ab, verlangt Papierkram --reason. --input liefert den Body (value, banking_transaction, payment_date, difference_reason); Flags überschreiben einzelne Felder daraus.

Eine Zahlung lässt sich per API nicht zurücknehmen: Im Terminal fragt der Befehl nach, ohne Terminal ist --yes nötig.`

// payFields are the fields of the pay request schema by flag of payFlags,
// with the flag name set, so errors of spec.CoerceFlag name --amount and
// not --value. A field missing in the spec is an error: the command is not
// available then.
func payFields(e registry.Entry) (map[string]spec.Field, error) {
	fields, err := spec.Fields(spec.RequestSchema(e.OperationID), resourceNames())
	if err != nil {
		return nil, err
	}
	out := map[string]spec.Field{}
	for _, pf := range payFlags {
		for _, f := range fields {
			if len(f.Path) == 1 && f.Path[0] == pf.field {
				f.Flag = pf.flag
				out[pf.flag] = f
			}
		}
		if _, ok := out[pf.flag]; !ok {
			return nil, fmt.Errorf("im Request-Schema von %s fehlt das Feld %s", e.OperationID, pf.field)
		}
	}
	return out, nil
}

// newPayCmd builds the pay command of a voucher.
func (a *App) newPayCmd(e registry.Entry) *cobra.Command {
	cmd := &cobra.Command{
		Use:         e.Action + " <id>",
		Short:       e.Short,
		Long:        entryLong(e) + payLong,
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class)},
		Args:        idArgs,
		RunE:        func(c *cobra.Command, args []string) error { return a.runPay(c, e, args) },
	}
	fields, _ := payFields(e) // runPay reports the error
	fs := cmd.Flags()
	for _, pf := range payFlags {
		f, ok := fields[pf.flag]
		if !ok {
			fs.String(pf.flag, "", pf.help)
			continue
		}
		fs.Var(&rawValue{typ: flagType(f)}, pf.flag, pf.help+spec.HelpSuffix(f))
	}
	fs.String("input", "", "Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder")
	return cmd
}

// runPay records a payment of a voucher (spec 9.5). Order (plan, as in
// runWriteThen): the ID, the fields of the request schema (exit 1 when the
// spec lacks one, like newWriteSpec), the flags (exit 2: without --input,
// --amount or --transaction is required), configuration and read-only
// gate (mutationClient, exit 9, before --input is read or a value
// checked), body (payBody, exit 5),
// --dry-run and confirmation (beforeSend, exit 8 without terminal and
// --yes), then the request. Output is the API object (the voucher); an
// empty answer prints {"id":N,"ok":true}.
func (a *App) runPay(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := pathValues(c, e, args)
	if err != nil {
		return err
	}
	fields, err := payFields(e)
	if err != nil {
		return errUnavailable(err)
	}
	if err := checkPayFlags(c); err != nil {
		return err
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	body, err := a.payBody(c, e, fields)
	if err != nil {
		return err
	}
	if proceed, err := a.beforeSend(c, e, client, params, body, sendExtras{details: payDetails(body)}); !proceed || err != nil {
		return err
	}
	resp, err := client.Call(c.Context(), e.OperationID, params, nil, body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(resp.Body)) > 0 {
		return a.render(c, resp.Body, e.Columns)
	}
	id := lastID(params)
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if ownText(o) {
		_, err := fmt.Fprintf(a.Out, "%s %d: Zahlung erfasst.\n", e.Resource, id)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, okJSON{ID: id, OK: true}, output.Columns{}))
}

// payMissing is the error when neither a payment amount nor a bank
// transaction is given (spec 9.5).
const payMissing = "Zahlung fehlt: --amount oder --transaction angeben"

// checkPayFlags runs the checks that need only the flags, before the
// configuration is resolved and anything is read or sent (exit 2). With
// --input the body may hold value or banking_transaction, so the check
// runs on the merged body (payBody, exit 5).
func checkPayFlags(c *cobra.Command) error {
	fs := c.Flags()
	if fs.Changed("input") && flagString(c, "input") == "" {
		return errMissingPath("--input")
	}
	if !fs.Changed("input") && !fs.Changed("amount") && !fs.Changed("transaction") {
		return clierr.New(clierr.CodeUsage, payMissing,
			"--amount bucht einen Betrag, --transaction verknüpft einen Bankumsatz. Verwendung: "+useLine(c)+" --amount <betrag> | --transaction <id>")
	}
	return nil
}

// payBody builds the request body like buildBody does for create:
// --input pruned against the request schema with the resource response
// schema (spec.ResourceResponseSchema, nil for this action, so every field
// outside the request is unknown, exit 5), then the changed flags at their
// field (converted by spec.CoerceFlag: exact decimals, relative dates,
// enum with suggestions, {id}), which win; coerced, checked for a payment
// (exit 5) and validated against the request schema (exit 5). fields
// are those of payFields.
func (a *App) payBody(c *cobra.Command, e registry.Entry, fields map[string]spec.Field) (map[string]any, error) {
	req := spec.RequestSchema(e.OperationID)
	body := map[string]any{}
	if c.Flags().Changed("input") {
		in, err := a.readInput(flagString(c, "input"))
		if err != nil {
			return nil, err
		}
		pr, err := spec.Prune(in, req, spec.ResourceResponseSchema(e.OperationID))
		if err != nil {
			return nil, prefixErr("--input: ", err)
		}
		a.pruneHints(pr)
		body = pr.Body
	}
	cc := spec.CoerceContext{Now: a.Now, Loc: a.Location}
	for _, pf := range payFlags {
		if !c.Flags().Changed(pf.flag) {
			continue
		}
		f := fields[pf.flag]
		v, err := spec.CoerceFlag(cc, f, rawText(c, pf.flag))
		if err != nil {
			return nil, err
		}
		setPath(body, f.Path, v)
	}
	v, err := spec.CoerceValue(req, body, "")
	if err != nil {
		return nil, err
	}
	body, _ = v.(map[string]any)
	if !hasPayment(body) {
		return nil, clierr.New(clierr.CodeValidation, "--input: "+payMissing,
			"in --input value oder banking_transaction.id setzen oder --amount bzw. --transaction angeben")
	}
	if err := spec.Validate(body, req); err != nil {
		return nil, err
	}
	return body, nil
}

// hasPayment reports a body with an amount (value) or a bank transaction
// with an ID; null counts as missing.
func hasPayment(body map[string]any) bool {
	if body["value"] != nil {
		return true
	}
	bt, _ := body["banking_transaction"].(map[string]any)
	return bt["id"] != nil
}

// payDetails are the lines of the confirmation besides the voucher
// (spec 8.2): the amount paid, the bank transaction, date and reason, as
// far as they are in the body.
func payDetails(body map[string]any) []Detail {
	var out []Detail
	if v := body["value"]; v != nil {
		out = append(out, Detail{"Zahlung", germanAmount(fmt.Sprint(v))})
	}
	if bt, _ := body["banking_transaction"].(map[string]any); bt["id"] != nil {
		out = append(out, Detail{resourceNoun("transaction"), fmt.Sprint(bt["id"])})
	}
	if v, ok := body["payment_date"].(string); ok {
		out = append(out, Detail{"Datum", v})
	}
	if v, ok := body["difference_reason"].(string); ok {
		out = append(out, Detail{"Grund", v})
	}
	return out
}
