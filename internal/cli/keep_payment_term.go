package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// invoice update keeps the payment term (ruling E2E-A): live run 3
// (2026-11-04) showed that a PUT of an invoice without payment_term
// switches it to the account's default payment term and changes due_date
// (contact person and project stay; a PUT with the current payment_term.id
// keeps it). Without --payment-term and without payment_term in --input,
// invoice update therefore reads the invoice first and sends its current
// payment_term.id along. The read mirrors that of --from-estimate
// (fromEstimate.load): the describe entry of the registry, the same
// client call with the path values, the same error handling (the read's
// code, "nothing changed") and the same --dry-run report (read, credits
// with the read). Deviations: it runs after the body is built and
// validated, because only the final body says whether payment_term is
// given (and an invalid body then costs no read); and only when needed.
//
// The same read keeps contact person and project (open point 29, task
// 8): a PUT whose customer lacks them clears both, also for the same
// customer (measured 2026-11-04). With --customer and without the flags
// of the missing references, the references of the invoice are taken
// over like those of an --input round trip (spec.Prune moves them below
// customer) and dropped for another customer like those of an estimate
// (dropForeignRefs, source "der Rechnung"). The answer is pruned without
// pruneHints: it is no input of the user.

const keepPaymentTermLong = `

Ohne --payment-term und ohne payment_term in --input liest das CLI die Rechnung vorher (%s) und sendet ihre aktuelle Zahlungsbedingung (payment_term.id) mit: Ein PUT ohne payment_term stellt die Rechnung sonst auf die Standard-Zahlungsbedingung des Accounts um und ändert das Fälligkeitsdatum (gemessen 2026-11-04). --dry-run liest ebenfalls und zeigt das Lesen. Scheitert das Lesen, wird nichts geändert. Mit --payment-term entfällt das Lesen. Mit --customer ohne --contact-person/--customer-project liest das CLI die Rechnung ebenfalls: beim gleichen Kunden werden Kontaktperson und Projekt mitgesendet, bei einem anderen entfallen sie (Hinweis auf stderr).`

// keepPaymentTerm is the read of invoice update: the describe entry of
// the invoice and the operation of the update (for its response schema,
// like the --input path).
type keepPaymentTerm struct {
	read   registry.Entry
	update string
}

// addKeepPaymentTerm adds the read to invoice update. commands --json
// counts it in the credits of the command (annotExtraCredits, like the
// readback of deliver): a run without payment term costs both requests.
func (a *App) addKeepPaymentTerm(e registry.Entry, cmd *cobra.Command) {
	read, ok := registry.Lookup(e.Resource, "describe")
	if !ok || read.Class != registry.ClassReadOnly {
		// fail closed: the read must be a plain GET without guards
		panic(fmt.Sprintf("registry: %s update ohne lesenden Eintrag %s describe", e.Resource, e.Resource))
	}
	cmd.Long += fmt.Sprintf(keepPaymentTermLong, moreCredits(read.Credits))
	// the flag of payment_term saves the read (spec-driven: the flag of
	// the field, not a fixed name)
	if f := cmd.Flags().Lookup(fieldFlag(newWriteSpec(e), "payment_term")); f != nil {
		f.Usage += fmt.Sprintf("; spart das Lesen der Rechnung (%s), außer mit --customer ohne --contact-person/--customer-project", creditsText(read.Credits))
	}
	cmd.Annotations[annotExtraCredits] = strconv.Itoa(read.Credits)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return a.runWriteThen(c, e, args, writeSteps{keep: &keepPaymentTerm{read: read, update: e.OperationID}})
	}
}

// apply reads the invoice when body has no payment_term (needTerm) or
// --customer leaves out references of the invoice (needRefs), once for
// both, and sets its current payment_term.id and its references of the
// same customer; it returns the read for --dry-run (nil without a read).
// The body is validated again with the values of the invoice.
func (k *keepPaymentTerm) apply(c *cobra.Command, a *App, client *api.Client, params []int64, ws writeSpec, body map[string]any) (*ReadInfo, error) {
	_, given := body["payment_term"]
	needTerm, needRefs := !given, needCustomerRefs(c, ws, body)
	if !needTerm && !needRefs {
		return nil, nil
	}
	method, url, err := client.Preview(k.read.OperationID, params, nil)
	if err != nil {
		return nil, err
	}
	id := lastID(params)
	resp, err := client.Call(c.Context(), k.read.OperationID, params, nil, nil)
	if err != nil {
		ce := *clierr.From(err)
		ce.Message = fmt.Sprintf("Rechnung %d nicht gelesen, nichts geändert: %s", id, ce.Message)
		ce.Cause = err
		return nil, &ce
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	hint := "Zahlungsbedingung per --payment-term <id> angeben (papierkram payment-term list)"
	if !needTerm {
		hint = refsHint(ws)
	}
	if dec.Decode(&doc) != nil || doc == nil || !atEOF(dec) {
		return nil, clierr.New(clierr.CodeError, fmt.Sprintf("Rechnung %d: Antwort ist kein JSON-Objekt, nichts geändert", id), hint)
	}
	var term any
	if needTerm {
		t, _ := doc["payment_term"].(map[string]any)
		if t == nil || t["id"] == nil {
			return nil, clierr.New(clierr.CodeError, fmt.Sprintf("Rechnung %d: Antwort ohne payment_term.id, nichts geändert", id), hint)
		}
		term = t["id"]
		body["payment_term"] = map[string]any{"id": term}
	}
	var kept []string
	if needRefs {
		if kept, err = k.keepRefs(c, a, ws, body, doc, id); err != nil {
			return nil, err
		}
	}
	if err := spec.Validate(body, ws.req); err != nil {
		return nil, err
	}
	if needTerm {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: payment_term %v der Rechnung mitgesendet (ohne sie stellt die API auf die Standard-Zahlungsbedingung um)\n", term)
	}
	if len(kept) > 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: %s der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)\n", strings.Join(kept, " und "))
	}
	return &ReadInfo{Method: method, URL: url, Credits: k.read.Credits}, nil
}

// needCustomerRefs reports whether --customer is given and a reference
// below customer (customerRefs) is neither in body nor set by its flag:
// the PUT would clear it.
func needCustomerRefs(c *cobra.Command, ws writeSpec, body map[string]any) bool {
	if flag := fieldFlag(ws, "customer"); flag == "" || !c.Flags().Changed(flag) {
		return false
	}
	cust, _ := body["customer"].(map[string]any)
	for _, k := range customerRefs {
		if _, has := cust[k]; has {
			continue
		}
		if f := fieldFlag(ws, "customer", k); f != "" && c.Flags().Changed(f) {
			continue
		}
		return true
	}
	return false
}

// refLabels name the references below customer in the hint.
var refLabels = map[string]string{"contact_person": "Kontaktperson", "project": "Projekt"}

// keepRefs takes the references of the invoice doc below customer into
// body where body lacks them: doc is pruned like an --input body (the
// response references move below customer) and the references of another
// customer are dropped (dropForeignRefs). It returns the labels of the
// references taken over ("Kontaktperson 55").
func (k *keepPaymentTerm) keepRefs(c *cobra.Command, a *App, ws writeSpec, body, doc map[string]any, id int64) ([]string, error) {
	pr, err := spec.Prune(doc, ws.req, spec.ResourceResponseSchema(k.update))
	if err != nil {
		ce := *clierr.From(err)
		ce.Code = clierr.CodeError
		ce.Message = fmt.Sprintf("Rechnung %d: Antwort nicht verwendbar, nichts geändert: %s", id, ce.Message)
		ce.Hint = refsHint(ws)
		ce.Cause = err
		return nil, &ce
	}
	base := pr.Body
	a.dropForeignRefs(c, ws, base, "der Rechnung", customerRefs)
	from, _ := base["customer"].(map[string]any)
	cust, _ := body["customer"].(map[string]any)
	if from == nil || cust == nil {
		return nil, nil
	}
	var kept []string
	for _, key := range customerRefs {
		ref, ok := from[key].(map[string]any)
		if _, has := cust[key]; !ok || has {
			continue
		}
		cust[key] = ref
		kept = append(kept, fmt.Sprintf("%s %v", refLabels[key], ref["id"]))
	}
	return kept, nil
}

// refsHint says how to set the references by flag.
func refsHint(ws writeSpec) string {
	return fmt.Sprintf("Kontaktperson und Projekt per --%s/--%s angeben", fieldFlag(ws, "customer", "contact_person"), fieldFlag(ws, "customer", "project"))
}
