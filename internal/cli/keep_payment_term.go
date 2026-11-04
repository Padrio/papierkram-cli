package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

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

const keepPaymentTermLong = `

Ohne --payment-term und ohne payment_term in --input liest das CLI die Rechnung vorher (%s) und sendet ihre aktuelle Zahlungsbedingung (payment_term.id) mit: Ein PUT ohne payment_term stellt die Rechnung sonst auf die Standard-Zahlungsbedingung des Accounts um und ändert das Fälligkeitsdatum (gemessen 2026-11-04). --dry-run liest ebenfalls und zeigt das Lesen. Scheitert das Lesen, wird nichts geändert. Mit --payment-term entfällt das Lesen.`

// keepPaymentTerm is the read of invoice update: the describe entry of
// the invoice.
type keepPaymentTerm struct {
	read registry.Entry
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
	cmd.Annotations[annotExtraCredits] = strconv.Itoa(read.Credits)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return a.runWriteThen(c, e, args, writeSteps{keep: &keepPaymentTerm{read: read}})
	}
}

// apply reads the invoice when body has no payment_term and sets its
// current payment_term.id; it returns the read for --dry-run (nil without
// a read). The body is validated again with the value of the invoice.
func (k *keepPaymentTerm) apply(c *cobra.Command, a *App, client *api.Client, params []int64, ws writeSpec, body map[string]any) (*ReadInfo, error) {
	if _, given := body["payment_term"]; given {
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
	if dec.Decode(&doc) != nil || doc == nil || !atEOF(dec) {
		return nil, clierr.New(clierr.CodeError, fmt.Sprintf("Rechnung %d: Antwort ist kein JSON-Objekt, nichts geändert", id), hint)
	}
	term, _ := doc["payment_term"].(map[string]any)
	if term == nil || term["id"] == nil {
		return nil, clierr.New(clierr.CodeError, fmt.Sprintf("Rechnung %d: Antwort ohne payment_term.id, nichts geändert", id), hint)
	}
	body["payment_term"] = map[string]any{"id": term["id"]}
	if err := spec.Validate(body, ws.req); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(a.Err, "Hinweis: payment_term %v der Rechnung mitgesendet (ohne sie stellt die API auf die Standard-Zahlungsbedingung um)\n", term["id"])
	return &ReadInfo{Method: method, URL: url, Credits: k.read.Credits}, nil
}
