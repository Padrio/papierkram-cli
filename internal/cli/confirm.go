package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// Summary is what a confirmation shows (spec 8.2): the German resource
// noun, the ID, and when known the name and the gross amount; for the send
// class also the way of sending, recipient and subject (see sendSummary);
// Details are further lines a handler adds from its body (the payment of
// voucher pay, see payDetails).
type Summary struct {
	Resource           string
	ID                 int64
	Name, Amount       string
	Via                string
	Recipient, Subject string
	Details            []Detail
}

// Detail is one labelled line of a confirmation summary.
type Detail struct {
	Label, Value string
}

// actionVerbs are the German infinitives of the confirmed actions.
var actionVerbs = map[string]string{
	"delete":          "löschen",
	"document delete": "löschen",
	"cancel":          "stornieren",
	"reverse":         "mit Gegenbuchung stornieren",
	"pay":             "als bezahlt buchen",
	"deliver":         "versenden",
}

func actionVerb(action string) string {
	if v, ok := actionVerbs[action]; ok {
		return v
	}
	return fmt.Sprintf("„%s“ ausführen", action)
}

// confirmed reports whether the class of e needs a confirmation (spec 8.1:
// destructive and send; archive is reversible and never asks, R25).
func confirmed(e registry.Entry) bool {
	return e.Class == registry.ClassDestructive || e.Class == registry.ClassSend
}

// mustAsk decides about the confirmation of e: false without one (other
// classes, or --yes), exit 8 when no prompt is possible (no terminal on
// stdin, or --no-input), true when the user has to be asked.
func (a *App) mustAsk(e registry.Entry, s Summary) (bool, error) {
	if !confirmed(e) || a.Flags.Yes {
		return false, nil
	}
	if !a.StdinTTY || a.Flags.NoInput {
		return false, clierr.New(clierr.CodeConfirmation,
			fmt.Sprintf("Bestätigung erforderlich: %s %d %s", s.Resource, s.ID, actionVerb(e.Action)),
			"--yes verwenden (ohne Terminal und mit --no-input wird nicht nachgefragt)")
	}
	return true, nil
}

// confirm asks for the confirmation of a destructive or send action
// (spec 8.2) on stderr: delete wants the ID typed in, other actions
// ja/yes/j/y (any case). Declining, an empty answer or a wrong ID is exit 8,
// an interrupt exit 10. Without a terminal or with --no-input it is exit 8
// with the hint on --yes; --yes and the other classes pass without asking.
func (a *App) confirm(ctx context.Context, e registry.Entry, s Summary) error {
	ask, err := a.mustAsk(e, s)
	if err != nil || !ask {
		return err
	}
	// texts from --input or the API: no \r, escape or line break may
	// overwrite or hide what is being confirmed
	for _, f := range []*string{&s.Name, &s.Amount, &s.Via, &s.Recipient, &s.Subject} {
		*f = printable(*f)
	}
	head := fmt.Sprintf("%s %d", s.Resource, s.ID)
	if s.Name != "" {
		head += " „" + s.Name + "“"
	}
	if s.Amount != "" {
		head += " (" + s.Amount + ")"
	}
	if s.Via != "" {
		_, _ = fmt.Fprintf(a.Err, "Versand:   %s\n", s.Via)
	}
	if s.Recipient != "" {
		_, _ = fmt.Fprintf(a.Err, "Empfänger: %s\n", s.Recipient)
	}
	if s.Subject != "" {
		_, _ = fmt.Fprintf(a.Err, "Betreff:   %s\n", s.Subject)
	}
	for _, d := range s.Details {
		_, _ = fmt.Fprintf(a.Err, "%-10s %s\n", printable(d.Label)+":", printable(d.Value))
	}
	id := strconv.FormatInt(s.ID, 10)
	if isDelete(e.Action) {
		line, err := a.prompt(ctx, head+" löschen? Zur Bestätigung ID eingeben: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(line) != id {
			return clierr.New(clierr.CodeConfirmation, "Abgebrochen durch Nutzer: eingegebene ID stimmt nicht überein",
				"Nichts gesendet. Zum Löschen genau die ID "+id+" eingeben oder --yes verwenden")
		}
		return nil
	}
	line, err := a.prompt(ctx, head+" wirklich "+actionVerb(e.Action)+"? [ja/nein]: ")
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "ja", "j", "yes", "y":
		return nil
	}
	return clierr.New(clierr.CodeConfirmation, "Abgebrochen durch Nutzer", "Nichts gesendet. Ohne Rückfrage mit --yes")
}

// isDelete reports a delete action: delete, or the delete of a sub-object
// (document delete). They want the ID typed in (spec 8.2).
func isDelete(action string) bool {
	return action == "delete" || strings.HasSuffix(action, " delete")
}

// printable replaces control characters (line breaks, \r, tabs, the ESC
// of ANSI sequences) and invisible format characters (bidi overrides,
// zero-width spaces, which could disguise a recipient) with a space.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if invisible(r) {
			return ' '
		}
		return r
	}, s)
}

// invisible reports a control character or an invisible format character
// (Unicode Cf).
func invisible(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// lastID is the ID of the object a command acts on: the last path
// parameter (0 without one).
func lastID(params []int64) int64 {
	if len(params) == 0 {
		return 0
	}
	return params[len(params)-1]
}

// resourceNoun is the German singular of a registry resource.
func resourceNoun(name string) string {
	for _, r := range registry.Resources() {
		if r.Name == name && r.Noun != "" {
			return r.Noun
		}
	}
	return name
}

// objectGet finds the GET operation of the object e acts on: the spec
// operation with the path of e up to its last parameter
// (/income/invoices/{id}/cancel → /income/invoices/{id}).
func objectGet(e registry.Entry) (spec.Operation, bool) {
	i := strings.LastIndex(e.Path, "}")
	if i < 0 {
		return spec.Operation{}, false
	}
	path := e.Path[:i+1]
	for _, op := range spec.Operations() {
		if op.Method == "GET" && op.Path == path {
			return op, true
		}
	}
	return spec.Operation{}, false
}

// fetchSummary builds the summary of the object e acts on; it loads the
// object (1 credit) for name and amount. It is only called when a prompt
// follows (terminal, no --yes). A failed load does not abort: the summary
// is shown without name and amount, with a warning. Only an interrupt
// ends it (exit 10).
func (a *App) fetchSummary(ctx context.Context, client *api.Client, e registry.Entry, params []int64) (Summary, error) {
	s := Summary{Resource: resourceNoun(e.Resource), ID: lastID(params)}
	op, ok := objectGet(e)
	if !ok {
		return s, nil
	}
	resp, err := client.Call(ctx, op.ID, params, nil, nil)
	if err != nil {
		if ctx.Err() != nil { // interrupted: no prompt follows
			return s, err
		}
		output.Warn(a.Err, "Zusammenfassung nicht geladen ("+clierr.From(err).Message+"); Rückfrage ohne Name und Betrag")
		return s, nil
	}
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	dec.UseNumber()
	if dec.Decode(&obj) != nil {
		return s, nil
	}
	s.Name = objectName(obj)
	for _, k := range []string{"total_gross", "amount"} {
		if v, ok := obj[k]; ok && v != nil {
			s.Amount = germanAmount(fmt.Sprint(v))
			break
		}
	}
	return s, nil
}

// objectName is the name of an API object: name, or first and last name.
func objectName(obj map[string]any) string {
	if n, ok := obj["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	first, _ := obj["first_name"].(string)
	last, _ := obj["last_name"].(string)
	return strings.TrimSpace(first + " " + last)
}

var decimalRe = regexp.MustCompile(`^(-?)(\d+)(?:\.(\d+))?$`)

// germanAmount formats a decimal as sent by the API in German notation
// with euro sign, without float conversion: 1190.00 → 1.190,00 €. Other
// values are shown unchanged.
func germanAmount(s string) string {
	m := decimalRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	digits := m[2]
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	frac := m[3]
	for len(frac) < 2 {
		frac += "0"
	}
	return m[1] + b.String() + "," + frac + " €"
}
