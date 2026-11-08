package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Europe/Berlin must load on hosts without zone data (Windows)
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// invoice|estimate deliver <id> --via email|pdf (spec 9.1, 10.4).

// The values of send_via the handler treats specially; the allowed values
// come from the request schema (deliverModes).
const (
	viaEmail = "email"
	viaPDF   = "pdf"
)

// mailFields map the mail flags to the fields of the email object
// (spec 9.1: --to, --subject, --body → email:{recipient,subject,body});
// --body-file is the other source of body.
var mailFields = []mailField{
	{"to", "recipient"},
	{"subject", "subject"},
	{"body", "body"},
}

// mailField is a mail flag of deliver and its field in the email object.
type mailField struct{ flag, field string }

// utf8BOM starts text files written by Windows editors.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// bodyFilePending stands for the text of --body-file in the flag checks,
// before the file is read (after the read-only gate).
const bodyFilePending = "<--body-file>"

const deliverLong = `

--via email versendet eine E-Mail mit Empfänger (--to), Betreff (--subject) und Text (--body oder --body-file, - für die Standardeingabe); Betreff und Text dürfen Platzhalter wie {{rechnung.rechnungsnummer}} oder {{angebot.angebotsnummer}} enthalten. --via pdf finalisiert den Beleg und vergibt die Nummer, ohne E-Mail. Ohne Terminal ist --yes nötig.

Danach liest das CLI den Beleg nach (%s; --dry-run zeigt die Summe) und zeigt id, Nummer, state, sent_on, sent_via und sent_to. Scheitert nur das Nachlesen, endet der Befehl mit Exit 0 und einer Warnung – dann NICHT erneut ausführen. Bricht die Verbindung nach dem Senden ab, entscheidet das Nachlesen: sent_on von heute → Exit 0 mit Warnung, sonst Exit 12 (vor Wiederholung per describe prüfen). Bei --via pdf setzt die API weder sent_on noch sent_via (beide bleiben null, state wird z. B. sent); ein abgebrochener PDF-Versand endet deshalb immer mit Exit 12.`

// newDeliverCmd builds the deliver command of an invoice or estimate entry.
func (a *App) newDeliverCmd(e registry.Entry) *cobra.Command {
	cmd := &cobra.Command{
		Use:         e.Action + " <id>",
		Aliases:     []string{"send"},
		Short:       e.Short,
		Long:        entryLong(e) + fmt.Sprintf(deliverLong, moreCredits(readbackCredits(e))),
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class), annotExtraCredits: strconv.Itoa(readbackCredits(e))},
		Args:        idArgs,
		RunE:        func(c *cobra.Command, args []string) error { return a.runDeliver(c, e, args) },
	}
	fs := cmd.Flags()
	fs.String("via", "", "Versandart: "+strings.Join(deliverModes(e), " oder ")+" (Pflicht; pdf finalisiert ohne E-Mail)")
	markRequired(fs, "via")
	markEnum(fs, "via", deliverModes(e))
	fs.String("to", "", "Empfängeradresse der E-Mail (Pflicht bei --via email)")
	fs.String("subject", "", "Betreff der E-Mail, Platzhalter erlaubt (Pflicht bei --via email)")
	fs.String("body", "", "Text der E-Mail, Platzhalter erlaubt (Pflicht bei --via email, sonst --body-file)")
	fs.String("body-file", "", "Text der E-Mail aus Datei oder - für die Standardeingabe")
	fs.String(deliverBodyFlag, "", "Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder")
	// with the body flag, checkDeliverFlags leaves send_via to the body
	addAlternatives(fs, "via", "--"+deliverBodyFlag)
	return cmd
}

// deliverBodyFlag gives the whole deliver body; send_via may come from it,
// so checkDeliverFlags checks the body only after the merge and --via
// names it as its alternative.
const deliverBodyFlag = "input"

// deliverModes are the values of send_via in the request schema.
func deliverModes(e registry.Entry) []string {
	var out []string
	if req := spec.RequestSchema(e.OperationID); req != nil {
		if p := req.Properties["send_via"]; p != nil && p.Value != nil {
			for _, v := range p.Value.Enum {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// runDeliver sends deliver and reads the document back (spec 9.1). Order
// (plan): flags (exit 2), configuration and read-only gate (mutationClient,
// exit 9, before --input or --body-file is read), body (exit 2/5),
// --dry-run and confirmation (beforeMutation), then the request.
//
// After a successful deliver (live: 2xx with empty body, R01) the document
// is read back and id, number, state, sent_on, sent_via and sent_to are
// printed. A failed readback is exit 0 with a warning (R02). When the
// connection breaks after the request was sent (outcome_unknown), the
// readback decides: sent_on today → exit 0 with a warning, else exit 12 (R37,
// see deliverUnknown).
func (a *App) runDeliver(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	if err := checkDeliverFlags(c, e); err != nil {
		return err
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	body, err := a.deliverBody(c, e)
	if err != nil {
		return err
	}
	// the run costs the deliver and the readback; --dry-run shows both
	run := e
	run.Credits += readbackCredits(e)
	if proceed, err := a.beforeMutation(c, run, client, params, body); !proceed || err != nil {
		return err
	}
	_, sendErr := client.Call(c.Context(), e.OperationID, params, nil, body)
	if sendErr != nil && clierr.From(sendErr).Code != clierr.CodeOutcomeUnknown {
		return deliverFailed(e, lastID(params), sendErr)
	}
	doc, readErr := a.readBack(c, e, client, params)
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	id := lastID(params)
	cols := deliverColumns(e)
	if sendErr != nil {
		via, _ := body["send_via"].(string)
		return a.deliverUnknown(o, e, id, via, doc, sendErr, readErr)
	}
	if readErr != nil {
		warning := "Versand erfolgreich, Nachlesen fehlgeschlagen: " + clierr.From(readErr).Message + " – NICHT erneut ausführen"
		output.Warn(a.Err, warning)
		if ownText(o) {
			_, err := fmt.Fprintf(a.Out, "%s %d versendet.\n", e.Resource, id)
			return afterRequest(err)
		}
		return afterRequest(output.RenderValue(o, deliveredJSON{ID: id, Delivered: true, Warnings: []string{warning}}, output.Columns{}))
	}
	return afterRequest(output.Render(o, doc.result(cols, nil), cols))
}

// readbackCredits is the cost of reading the document back: that of its
// describe entry.
func readbackCredits(e registry.Entry) int {
	d, _ := registry.Lookup(e.Resource, "describe")
	return d.Credits
}

// moreCredits is "1 weiterer Credit" or "N weitere Credits".
func moreCredits(n int) string {
	if n == 1 {
		return "1 weiterer Credit"
	}
	return fmt.Sprintf("%d weitere Credits", n)
}

// deliverFailed is the error of a deliver POST that got an error answer.
// A 5xx or an unexpected status does not prove that no mail went out, so
// the hint names the describe command to check before repeating; client
// errors (422: broken placeholder, 404, …) keep theirs, nothing was sent.
func deliverFailed(e registry.Entry, id int64, err error) error {
	ce := clierr.From(err)
	if ce.Code != clierr.CodeServer && ce.Code != clierr.CodeError {
		return err
	}
	out := *ce
	out.Hint = fmt.Sprintf("Papierkram-Fehler – der Versand kann trotzdem erfolgt sein: vor Wiederholung per describe prüfen: papierkram %s describe %d", e.Resource, id)
	return &out
}

// deliveredJSON is the result when deliver succeeded but the readback
// failed (spec 9.1).
type deliveredJSON struct {
	ID        int64    `json:"id"`
	Delivered bool     `json:"delivered"`
	Warnings  []string `json:"warnings"`
}

// deliverUnknown decides a deliver whose answer was lost after sending
// (spec 10.4). Only a sent_on of today (a Papierkram day, see dayZone)
// counts as this delivery: exit 0 with a warning. An earlier date only
// shows a previous delivery (a re-delivery of an already sent document);
// that, no sent_on or no readback is exit 12 with the describe command to
// check, and the hint names the earlier date. via pdf leaves sent_on null
// (measured 2026-11-04), so it always ends with exit 12; the message says
// so and names the state, which alone proves nothing (the document may
// have been sent before).
func (a *App) deliverUnknown(o output.Options, e registry.Entry, id int64, via string, doc readback, sendErr, readErr error) error {
	ce := clierr.From(sendErr)
	on := doc.sentOn()
	day := a.sentDay(on)
	if readErr == nil && day != "" && day == a.today() {
		warning := fmt.Sprintf("keine Antwort auf den Versand (%s), laut Nachlesen heute (%s) versendet: sehr wahrscheinlich versendet – NICHT erneut ausführen", ce.Message, on)
		output.Warn(a.Err, warning)
		cols := deliverColumns(e)
		return afterRequest(output.Render(o, doc.result(cols, []string{warning}), cols))
	}
	hint := fmt.Sprintf("Ergebnis unbekannt – vor Wiederholung per describe prüfen: papierkram %s describe %d", e.Resource, id)
	why := "Nachlesen: nicht als versendet markiert (sent_on leer)"
	if via == "pdf" {
		why = fmt.Sprintf("Nachlesen: sent_on leer – bei --via pdf setzt die API sent_on nicht, der Versand lässt sich so nicht bestätigen (state %q)", doc.text("state"))
	}
	switch {
	case readErr != nil:
		why = "Nachlesen fehlgeschlagen: " + clierr.From(readErr).Message
	case on != "":
		why = "Nachlesen: sent_on " + on + " ist nicht heute"
		if day != "" {
			on = day
		}
		hint += " (zuletzt versendet am " + on + ", dieser Versand ist nicht bestätigt)"
	}
	return &clierr.Error{
		Code:    clierr.CodeOutcomeUnknown,
		Message: ce.Message + "; " + why,
		Hint:    hint,
		Cause:   sendErr,
	}
}

// sentOnLayouts are the forms of sent_on: a date (spec: format date) or a
// date-time with offset as the API writes them elsewhere (supply_date
// "2026-08-28 13:41:23 +0200", RFC 3339).
var sentOnLayouts = []string{"2006-01-02 15:04:05 -0700", time.RFC3339}

// sentDay is the calendar day (YYYY-MM-DD) of sent_on in dayZone; a
// date-time is converted first. "" when empty or not readable.
func (a *App) sentDay(on string) string {
	if _, err := time.Parse(time.DateOnly, on); err == nil {
		return on
	}
	for _, layout := range sentOnLayouts {
		if t, err := time.Parse(layout, on); err == nil {
			return t.In(a.dayZone()).Format(time.DateOnly)
		}
	}
	return ""
}

// today is the current day (YYYY-MM-DD) in dayZone.
func (a *App) today() string { return a.Now().In(a.dayZone()).Format(time.DateOnly) }

// berlin is the time zone of Papierkram. The embedded time/tzdata makes it
// load everywhere; nil stays as a defensive fallback.
var berlin = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return nil
	}
	return loc
})

// dayZone is the zone of the calendar days of Papierkram (Europe/Berlin),
// not of the host, which may be a UTC container or lie hours behind: a
// date-only sent_on is such a day (both sides are compared there), and so
// are today/heute and yesterday/gestern of every resource but time
// (dayContext). Without zone data for Berlin it falls back
// to App.Location.
func (a *App) dayZone() *time.Location {
	if loc := berlin(); loc != nil {
		return loc
	}
	if a.Location == nil {
		return time.Local
	}
	return a.Location
}

// hostDayResource is the resource whose relative dates are days of the
// host clock (dayContext).
const hostDayResource = "time"

// dayContext is the clock of the relative dates (today/heute,
// yesterday/gestern) of the entry's resource. One rule, per resource and
// not per command: Zeiterfassung (time …): Tag der Rechneruhr; Belege,
// Zahlungen, Datumsfilter: Berliner Tag.
//
// Time entries (create, update, log, stop, list --since/--until) take the
// host's day: their date goes with wall-clock times of the host clock
// (--duration without --from ends now; time stop books the timer's start
// in the host zone; --since/--until bounds are host-zone times), so a
// Berlin date with host times would book 23:30 on the following day, and
// the check after time stop (time list --since heute) would miss the
// entry. Every other resource takes the day of Papierkram (dayZone).
// Every caller of spec.CoerceFlag goes through it.
func (a *App) dayContext(resource string) spec.CoerceContext {
	if resource == hostDayResource {
		return spec.CoerceContext{Now: a.Now, Loc: a.loc()}
	}
	return spec.CoerceContext{Now: a.Now, Loc: a.dayZone()}
}

// deliverColumns are the output fields of deliver (spec 9.1) and their
// table columns; the headers match those of describe.
func deliverColumns(e registry.Entry) output.Columns {
	return output.Columns{Table: []output.Column{
		{Header: "ID", Path: "id"},
		{Header: "NR.", Path: e.Resource + "_no"},
		{Header: "STATUS", Path: "state"},
		{Header: "VERSENDET", Path: "sent_on", Fmt: output.FmtDate},
		{Header: "VERSANDART", Path: "sent_via"},
		{Header: "EMPFÄNGER", Path: "sent_to"},
	}}
}

// readback is the document read after deliver, values as the API sent
// them (exact numbers).
type readback map[string]json.RawMessage

// readBack loads the delivered document with its GET (1 credit).
func (a *App) readBack(c *cobra.Command, e registry.Entry, client *api.Client, params []int64) (readback, error) {
	op, ok := objectGet(e)
	if !ok {
		return nil, clierr.New(clierr.CodeError, "kein Lesebefehl für "+e.Resource, "")
	}
	resp, err := client.Call(c.Context(), op.ID, params, nil, nil)
	if err != nil {
		return nil, err
	}
	var doc readback
	if err := json.Unmarshal(resp.Body, &doc); err != nil || doc == nil {
		return nil, clierr.New(clierr.CodeError, "Antwort ist kein JSON-Objekt", "")
	}
	return doc, nil
}

// sentOn is the sent_on date of the readback, "" when null or missing.
func (d readback) sentOn() string { return d.text("sent_on") }

// text is the string field key of the readback, "" when null, missing or
// no string.
func (d readback) text(key string) string {
	var s *string
	if json.Unmarshal(d[key], &s) != nil || s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// result is the output object: the column fields in column order (null
// when missing), then warnings when there are any.
func (d readback) result(cols output.Columns, warnings []string) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, col := range cols.Table {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(col.Path)
		b.Write(k)
		b.WriteByte(':')
		if v := d[col.Path]; len(v) > 0 {
			b.Write(v)
		} else {
			b.WriteString("null")
		}
	}
	if len(warnings) > 0 {
		b.WriteString(`,"warnings":`)
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(warnings) // []string always encodes
	}
	b.WriteByte('}')
	return b.Bytes()
}

// checkDeliverFlags runs every check that needs only the flags, before
// the configuration is resolved and anything is read or sent (exit 2).
// Without --input the flags are the whole body and get the checks of the
// body (checkDeliverBody); with --input the body checks run after the
// merge, except that --via pdf never takes mail flags.
func checkDeliverFlags(c *cobra.Command, e registry.Entry) error {
	fs := c.Flags()
	for _, flag := range []string{deliverBodyFlag, "body-file"} {
		if fs.Changed(flag) && flagString(c, flag) == "" {
			return errMissingPath("--" + flag)
		}
	}
	via, modes := flagString(c, "via"), deliverModes(e)
	input := fs.Changed(deliverBodyFlag)
	switch {
	case fs.Changed("via") && !slices.Contains(modes, via):
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültiger Wert %q für --via: erlaubt sind %s", via, strings.Join(modes, ", ")),
			"Verwendung: "+useLine(c)+" --via "+strings.Join(modes, "|"))
	case fs.Changed("body") && fs.Changed("body-file"):
		return clierr.New(clierr.CodeUsage, "--body und --body-file nicht zusammen verwenden",
			"Text der E-Mail entweder direkt per --body oder aus einer Datei per --body-file angeben")
	case flagString(c, deliverBodyFlag) == "-" && flagString(c, "body-file") == "-":
		return clierr.New(clierr.CodeUsage, "--input - und --body-file - lesen beide die Standardeingabe",
			"nur eines davon mit - angeben, das andere als Datei")
	case !input || via == viaPDF:
		body := map[string]any{}
		applyDeliverFlags(c, body, bodyFilePending)
		return checkDeliverBody(c, body)
	}
	return nil
}

// applyDeliverFlags sets the given flags into body: --via as send_via,
// the mail flags into email (merged with an email object from --input),
// and bodyFile, the text of --body-file, as email.body.
func applyDeliverFlags(c *cobra.Command, body map[string]any, bodyFile string) {
	if c.Flags().Changed("via") {
		body["send_via"] = flagString(c, "via")
	}
	for _, m := range mailFields {
		if c.Flags().Changed(m.flag) {
			setPath(body, []string{"email", m.field}, flagString(c, m.flag))
		}
	}
	if c.Flags().Changed("body-file") {
		setPath(body, []string{"email", "body"}, bodyFile)
	}
}

// checkDeliverBody checks the way of sending (exit 2, spec 9.1): send_via
// must be given, pdf takes no mail, and email needs recipient, subject and
// body, so the API never sends its default mail to the default address
// (spec 4.5). Other values and types are left to the schema validation.
func checkDeliverBody(c *cobra.Command, body map[string]any) error {
	mail, hasMail := body["email"]
	switch via, _ := body["send_via"].(string); via {
	case "":
		return clierr.New(clierr.CodeUsage, "Pflichtangabe --via fehlt: email oder pdf",
			"--via email versendet eine E-Mail (dazu --to, --subject und --body oder --body-file), --via pdf finalisiert ohne E-Mail. Verwendung: "+useLine(c))
	case viaPDF:
		if hasMail {
			return clierr.New(clierr.CodeUsage, "--via pdf versendet keine E-Mail: --to, --subject, --body und --body-file nicht angeben",
				"für eine E-Mail --via email verwenden; ein email-Objekt aus --input entfernen")
		}
	case viaEmail:
		var missing []string
		for _, f := range missingMail(mail) {
			name := "--" + f.flag
			if f.flag == "body" {
				name += " (oder --body-file)"
			}
			missing = append(missing, name)
		}
		if len(missing) > 0 {
			return clierr.New(clierr.CodeUsage, "--via email verlangt "+strings.Join(missing, ", "),
				"Empfänger, Betreff und Text angeben; ohne sie würde die API still ihre Standardmail an die Standardadresse senden")
		}
	}
	return nil
}

// missingMail is the completeness rule of a mail (spec 9.1): the fields
// of mailFields (recipient, subject, body) that the email object mail
// lacks or holds only blanks in. A mail of another type than an object is
// left to the schema validation (nothing missing). deliver (as flags) and
// papierkram api (as JSON fields) share it.
func missingMail(mail any) []mailField {
	m, isMap := mail.(map[string]any)
	if mail != nil && !isMap {
		return nil
	}
	var missing []mailField
	for _, f := range mailFields {
		v := m[f.field]
		if s, isStr := v.(string); v == nil || isStr && strings.TrimSpace(s) == "" {
			missing = append(missing, f)
		}
	}
	return missing
}

// deliverBody builds the request body like buildBodyFrom does for create:
// --input (pruned against the request schema; deliver has no response
// schema, so every field outside the request is unknown, exit 5), then the
// flags, which win, with the text of --body-file; coerced like create,
// checked by checkDeliverBody (exit 2) and validated against the request
// schema (exit 5).
func (a *App) deliverBody(c *cobra.Command, e registry.Entry) (map[string]any, error) {
	req := spec.RequestSchema(e.OperationID)
	body := map[string]any{}
	if c.Flags().Changed("input") {
		in, err := a.readInput(flagString(c, "input"))
		if err != nil {
			return nil, err
		}
		pr, err := spec.Prune(in, req, nil)
		if err != nil {
			return nil, prefixErr("--input: ", err)
		}
		a.pruneHints(pr)
		body = pr.Body
		// "email": null (a copied GET answer) is no mail; the request
		// does not allow null, and --via pdf must not see a mail
		if v, ok := body["email"]; ok && v == nil {
			delete(body, "email")
		}
	}
	var text string
	if c.Flags().Changed("body-file") {
		var err error
		if text, err = a.readText("--body-file", flagString(c, "body-file")); err != nil {
			return nil, err
		}
	}
	applyDeliverFlags(c, body, text)
	// like create/update (buildBodyFrom): coerce, then check and validate
	v, err := spec.CoerceValue(req, body, "")
	if err != nil {
		return nil, err
	}
	body, _ = v.(map[string]any)
	if err := checkDeliverBody(c, body); err != nil {
		return nil, err
	}
	if err := spec.Validate(body, req); err != nil {
		return nil, err
	}
	return body, nil
}

// readText reads the text of flag (--body-file) from a file or, for "-",
// the shared stdin reader. A UTF-8 BOM is removed; line breaks (CRLF or
// LF) stay as they are. Text that is not UTF-8 (Windows-1252, UTF-16) is a
// validation error (exit 5), a missing file a usage error; an empty text is
// a missing body like an empty --body (checkDeliverBody, exit 2).
func (a *App) readText(flag, path string) (string, error) {
	var r io.Reader = a.stdin()
	if path != "-" {
		f, err := openJSONFile(flag, path)
		if err != nil {
			return "", err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", clierr.New(clierr.CodeError, flag+": Eingabe nicht lesbar: "+err.Error(), "")
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", clierr.New(clierr.CodeValidation, flag+": Text ist nicht UTF-8-kodiert", "Datei als UTF-8 speichern")
	}
	return string(data), nil
}

// sendSummary is what the confirmation of deliver shows besides the
// document (spec 8.2): the way of sending, recipient and subject.
func sendSummary(body map[string]any) (via, recipient, subject string) {
	switch body["send_via"] {
	case viaEmail:
		m, _ := body["email"].(map[string]any)
		recipient, _ = m["recipient"].(string)
		subject, _ = m["subject"].(string)
		return "E-Mail", recipient, subject
	case viaPDF:
		return "PDF (finalisieren und Nummer vergeben, keine E-Mail)", "", ""
	}
	return "", "", ""
}
