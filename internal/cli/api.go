package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// papierkram api <METHOD> <path> (spec 7.2): the escape hatch like gh api.
// The body is sent as given (not pruned); the protection rules of the
// registry still apply: method and path are matched against the spec
// (spec.MatchPath), the class of the matched registry entry decides about
// the read-only lock, the confirmation and --dry-run; an unknown path is
// read_only for GET and destructive otherwise. deliver keeps its email
// rule (spec 9.1, R33).

const apiShort = "Beliebige Anfrage an die Papierkram-API senden (wie gh api)"

const apiLong = apiShort + `.

<pfad> ist relativ zur API-Basis /api/v1, beginnt mit / und darf eine Query enthalten (/income/invoices?page=2). Vollständige URLs, //, \, die Segmente . und .. (auch %-kodiert), kodierte Schrägstriche, # und Steuerzeichen sind nicht erlaubt; Segmente bestehen nur aus Buchstaben, Ziffern, _ und -. Ein Pfad, der nur ohne Groß- und Kleinschreibung oder mit einer ID, die nicht nur aus Ziffern besteht, zu einer Operation der Spec passt (12-x, Deliver), ist nicht eindeutig und wird abgelehnt. METHOD ist GET, POST, PUT, PATCH oder DELETE (Groß- und Kleinschreibung egal).

Felder: -f key=value setzt immer Text. -F key=value erkennt true, false, null und JSON-Zahlen (exakt, 19.99 bleibt 19.99); @datei setzt den Inhalt der Datei als Text, @- die Standardeingabe; alles andere ist Text. Punkte im Schlüssel bilden verschachtelte Objekte (-F email.recipient=…); jeder Schlüssel nur einmal. Bei GET und bei DELETE ohne --input werden die Felder Query-Parameter, sonst bilden sie den JSON-Body. --input datei.json (oder - für die Standardeingabe) liefert den Body; -f/-F überschreiben darin einzelne, auch verschachtelte Felder.

Der Body wird ohne Bereinigung gesendet, wie angegeben. --validate prüft ihn vorher gegen das Request-Schema der Spec (Exit 5); für Pfade außerhalb der Spec geht das nicht.

Schutzregeln: Methode und Pfad werden in der Registry nachgeschlagen; deren Klasse entscheidet über Schreibschutz (Exit 9), Bestätigung bzw. --yes (Exit 8 ohne Terminal) und --dry-run. Pfade außerhalb der Spec gelten bei GET als lesend, sonst als destruktiv. Für …/deliver gelten die Regeln von deliver: keine Query (Exit 2), send_via (email oder pdf) ist Pflicht, bei email auch email.recipient, email.subject und email.body, und der Body wird immer gegen das Request-Schema geprüft (Exit 5). --dry-run zeigt Methode, URL, Body und Credits und gilt nur für ändernde Anfragen.

Kosten: wie der passende Befehl (z. B. 50 Credits für das Anlegen einer Rechnung); außerhalb der Spec nach derselben Regel (Anlegen 10 bzw. 50, sonst 1 Credit).

Antwort: JSON wie bei den anderen Befehlen (-o, --fields, --jq), ein leerer Body als {"ok": true}. Andere Antworten (PDF, Bilder) gehen unverändert nach stdout, aber nur, wenn stdout kein Terminal ist. --paginate lädt bei Listen der Spec alle Seiten (1 Anfrage je Seite, ohne page_size 100 je Seite) und führt sie zusammen wie --all; ab 10 Seiten Warnung, --max-pages begrenzt.`

var apiExamples = []string{
	"papierkram api GET /info",
	"papierkram api GET '/income/invoices?page=2' -f page_size=50 -o json",
	"papierkram api GET /contact/companies --paginate --jq '.entries[].name'",
	"papierkram api POST /income/invoices/12/deliver -F send_via=email -f email.recipient=kunde@example.org -f email.subject='Ihre Rechnung' -F email.body=@mail.txt --yes",
	"papierkram api PUT /projects/5 --input projekt.json --validate --dry-run",
	"papierkram api DELETE /tracker/time_entries/7 --yes",
}

// apiMethods are the methods papierkram api sends.
var apiMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// newAPICmd builds papierkram api. Its static class annotation is
// destructive, the class of an unknown mutation; the effective class is
// that of the request (apiTarget), so --dry-run passes the root check and
// runAPI rejects it for reads.
func newAPICmd(app *App) *cobra.Command {
	lines := make([]string, len(apiExamples))
	for i, ex := range apiExamples {
		lines[i] = "  " + ex
	}
	cmd := &cobra.Command{
		Use:         "api <METHOD> <pfad>",
		Short:       apiShort,
		Long:        apiLong,
		Example:     strings.Join(lines, "\n"),
		Annotations: map[string]string{annotClass: string(registry.ClassDestructive), annotPerRequest: "true"},
		Args:        apiArgs,
		RunE:        func(c *cobra.Command, args []string) error { return app.runAPI(c, args) },
		// METHOD completes, the path does not (no file names)
		ValidArgsFunction: apiArgCompletion,
	}
	fs := cmd.Flags()
	fs.StringArrayP("raw-field", "f", nil, "Feld als Text: key=value (Punkte verschachteln; bei GET und DELETE ohne --input Query-Parameter)")
	fs.StringArrayP("field", "F", nil, "Feld mit Typ: true, false, null, Zahl, @datei, @- (Standardeingabe), sonst Text (bei GET und DELETE ohne --input Query-Parameter)")
	fs.String("input", "", "Body als JSON-Datei oder - für die Standardeingabe; -f/-F überschreiben einzelne Felder")
	fs.Bool("paginate", false, "bei GET alle Seiten einer Liste laden und zusammenführen (1 Anfrage je Seite)")
	fs.Int("max-pages", 0, "mit --paginate höchstens so viele Seiten laden")
	markBounds(fs, "max-pages", bound(minMaxPages), nil) // as list --max-pages
	fs.Bool("validate", false, "Body vor dem Senden gegen das Request-Schema der Spec prüfen")
	return cmd
}

// apiArgs checks METHOD and path before anything else (exit 2).
func apiArgs(c *cobra.Command, args []string) error {
	if err := exactArgs(2)(c, args); err != nil {
		return err
	}
	if _, err := apiMethod(c, args[0]); err != nil {
		return err
	}
	_, err := parseAPIPath(c, args[1])
	return err
}

// apiMethod is the upper-case method or a usage error.
func apiMethod(c *cobra.Command, raw string) (string, error) {
	m := strings.ToUpper(raw)
	if !slices.Contains(apiMethods, m) {
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige Methode %q: erlaubt sind %s", raw, strings.Join(apiMethods, ", ")),
			"Verwendung: "+useLine(c))
	}
	return m, nil
}

// apiTarget is the operation of a request: the registry entry (class,
// credits, columns) and the IDs of the path, and whether the spec knows
// the path (op). A path outside the spec, or an operation without a
// registry entry, gets an entry of its own: read_only for GET, else
// destructive, credits by registry.ExpectedCredits.
type apiTarget struct {
	entry registry.Entry
	ids   []int64
	op    spec.Operation
	known bool
}

func apiTargetOf(method string, p apiPath) apiTarget {
	op, ids, ok := spec.MatchPath(method, p.match)
	if ok {
		if e, found := registry.ByOperationID(op.ID); found {
			return apiTarget{entry: e, ids: ids, op: op, known: true}
		}
	}
	class := registry.ClassDestructive
	if method == "GET" {
		class = registry.ClassReadOnly
	}
	e := registry.Entry{Resource: "api", Action: "api", Method: method, Path: p.send, Class: class, Credits: registry.ExpectedCredits(method, p.send)}
	if ok {
		e.OperationID = op.ID
	}
	return apiTarget{entry: e, ids: ids, op: op, known: ok}
}

// unknown reports a target without registry entry: its confirmation
// names method and path (Summary.Label).
func (t apiTarget) unknown() bool { return t.entry.Resource == "api" }

// isList reports a list operation of the spec with the page envelope that
// --paginate follows (page and page_size, entries).
func (t apiTarget) isList() bool {
	if !t.known || t.op.Method != "GET" {
		return false
	}
	q := spec.QueryParams(t.op.ID)
	s := spec.ResponseSchema(t.op.ID)
	return q["page"] != nil && q["page_size"] != nil && s != nil && s.Properties["entries"] != nil
}

// apiCall is a checked invocation of papierkram api.
type apiCall struct {
	method   string
	path     apiPath
	target   apiTarget
	fields   []apiField
	input    string // --input path, "" without
	hasBody  bool   // fields and --input go into a JSON body
	paginate bool
	maxPages int
	validate bool
	// ambiguous is the canonical form of a path that matches a spec
	// operation only loosely (spec.MatchPathLoose), "" otherwise
	ambiguous string
}

// checkAPIFlags runs every check that needs only arguments and flags,
// before the configuration is resolved and anything is read or sent
// (exit 2).
func (a *App) checkAPIFlags(c *cobra.Command, args []string) (*apiCall, error) {
	method, err := apiMethod(c, args[0])
	if err != nil {
		return nil, err
	}
	p, err := parseAPIPath(c, args[1])
	if err != nil {
		return nil, err
	}
	fields, err := parseAPIFields(c)
	if err != nil {
		return nil, err
	}
	fs := c.Flags()
	call := &apiCall{method: method, path: p, target: apiTargetOf(method, p), fields: fields, input: flagString(c, "input"),
		paginate: flagBool(c, "paginate"), validate: flagBool(c, "validate")}
	call.maxPages, _ = fs.GetInt("max-pages")
	if !call.target.known {
		// a form the server may route to a spec operation (12-x, Deliver)
		if _, canonical, ok := spec.MatchPathLoose(method, p.match); ok {
			call.ambiguous = canonical
		}
	}
	call.hasBody = method != "GET" && (fs.Changed("input") || method != "DELETE" && len(fields) > 0)
	readers := 0
	if call.input == "-" {
		readers++
	}
	for _, f := range fields {
		if f.stdin() {
			readers++
		}
	}
	usage := func(msg, hint string) (*apiCall, error) {
		if hint == "" {
			hint = "Verwendung: " + useLine(c)
		}
		return nil, clierr.New(clierr.CodeUsage, msg, hint)
	}
	switch {
	case fs.Changed("input") && call.input == "":
		return nil, errMissingPath("--input")
	case readers > 1:
		return usage("die Standardeingabe kann nur einmal gelesen werden (--input - und @-)", "höchstens eine Angabe mit -, die anderen als Datei")
	case method == "GET" && fs.Changed("input"):
		return usage("GET sendet keinen Body: --input nicht möglich", "Felder mit -f/-F werden bei GET (und DELETE ohne --input) Query-Parameter")
	case method == "GET" && a.Flags.DryRun:
		return usage("--dry-run gilt nur für ändernde Befehle", "GET liest nur; ohne --dry-run ausführen. Verwendung: "+useLine(c))
	case call.paginate && method != "GET":
		return usage("--paginate gilt nur für GET", "")
	case fs.Changed("max-pages") && !call.paginate:
		return usage("--max-pages gilt nur zusammen mit --paginate", "")
	case fs.Changed("max-pages") && call.maxPages < minMaxPages:
		return usage(fmt.Sprintf("ungültiger Wert %d für --max-pages: mindestens %d", call.maxPages, minMaxPages), "")
	case call.paginate && !call.target.isList():
		return usage("--paginate braucht eine Liste der API-Spec (Seiten mit page/page_size und entries)",
			"ohne --paginate eine Seite laden oder einen Listenpfad wie /income/invoices angeben")
	case !call.target.known && call.ambiguous != "" && call.ambiguous == p.match:
		// the loose form is the input itself: only an ID beyond int64
		// keeps MatchPath from matching
		return usage("ID zu groß: IDs reichen höchstens bis 9223372036854775807", "ID prüfen")
	case !call.target.known && call.ambiguous != "":
		return usage("Pfad nicht eindeutig: gemeint ist "+call.ambiguous,
			"den Pfad genau so angeben: Kleinschreibung wie in der API, IDs nur aus Ziffern")
	case call.target.entry.Class == registry.ClassSend && (len(p.query) > 0 || !call.hasBody && len(fields) > 0):
		return usage(fmt.Sprintf("%s %s nimmt keine Query-Parameter: send_via und email gehören in den Body", method, p.send),
			"Query entfernen und die Felder mit -f/-F oder --input als Body senden")
	case call.validate && !call.target.known:
		return usage(fmt.Sprintf("--validate: %s %s ist nicht in der API-Spec, es gibt kein Request-Schema", method, p.send),
			"ohne --validate senden oder den Pfad prüfen")
	}
	if !call.hasBody {
		for _, f := range fields {
			if call.paginate && f.name() == "page" {
				return usage("--paginate setzt page selbst: page nicht angeben", "")
			}
			if p.query.Has(f.name()) {
				return usage(fmt.Sprintf("Feld %s steht schon in der Query des Pfads", f.flag()), "jeden Parameter nur einmal angeben")
			}
		}
	}
	if call.paginate && p.query.Has("page") {
		return usage("--paginate setzt page selbst: page nicht angeben", "")
	}
	if err := a.checkAPIFile(call); err != nil {
		return nil, err
	}
	return call, nil
}

// checkAPIFile refuses a known file answer (PDF) when stdout is a
// terminal, before the request (like … pdf without -O).
func (a *App) checkAPIFile(call *apiCall) error {
	t := call.target
	if !a.StdoutTTY || !t.known || spec.FileMediaType(t.op.ID) == "" {
		return nil
	}
	return clierr.New(clierr.CodeUsage, "kein Ziel für die Datei: stdout ist ein Terminal", fileHint(t))
}

// fileHint is the hint for a file answer in a terminal.
func fileHint(t apiTarget) string {
	hint := "Ausgabe umleiten (… > datei)"
	if t.entry.Action == "pdf" && len(t.ids) > 0 {
		hint += fmt.Sprintf(" oder papierkram %s pdf %d -O datei.pdf verwenden", t.entry.Resource, lastID(t.ids))
	} else {
		hint += " oder für PDFs papierkram <ressource> pdf <id> -O datei.pdf verwenden"
	}
	return hint
}

// runAPI sends the request. Order (plan): arguments and flags (exit 2),
// configuration and read-only gate (mutationClient, exit 9, before
// --input, @file or @- is read), body and its checks (exit 2/5), dry run
// and confirmation (beforeSend), then the request.
func (a *App) runAPI(c *cobra.Command, args []string) error {
	call, err := a.checkAPIFlags(c, args)
	if err != nil {
		return err
	}
	e := call.target.entry
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	query := url.Values{}
	for k, v := range call.path.query {
		query[k] = slices.Clone(v)
	}
	var body map[string]any
	if call.hasBody {
		if body, err = a.apiBody(call); err != nil {
			return err
		}
	} else {
		for _, f := range call.fields {
			v, err := a.fieldValue(f)
			if err != nil {
				return err
			}
			query.Add(f.name(), queryText(v))
		}
	}
	if err := checkAPIBody(call, body); err != nil {
		return err
	}
	raw := &rawRequest{method: call.method, path: call.path.send, query: query}
	if call.method != "GET" {
		x := sendExtras{raw: raw, details: apiDetails(raw, body)}
		if call.target.unknown() {
			x.summary = &Summary{Label: call.method + " " + call.path.send}
		}
		if proceed, err := a.beforeSend(c, e, client, call.target.ids, body, x); !proceed || err != nil {
			return err
		}
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if call.paginate {
		// like list --all without --limit: as few pages as possible
		if !query.Has("page_size") {
			query.Set("page_size", strconv.Itoa(allPageSize))
		}
		lr := &listRequest{query: query, all: true, maxPages: call.maxPages, via: "--paginate"}
		merged, err := a.fetchPages(c, e.Credits, lr, func(q url.Values) (*api.Response, error) {
			resp, err := client.CallRaw(c.Context(), "GET", raw.path, q, "", nil)
			return resp, pageTooLarge(err, hintAPIListTooLarge)
		})
		if err != nil {
			return err
		}
		return afterRequest(output.RenderValue(o, merged, e.Columns))
	}
	var rd io.Reader
	contentType := ""
	if body != nil {
		data, err := encodeBody(body)
		if err != nil {
			return err
		}
		// a bytes.Reader lets --debug show the body (GetBody)
		rd, contentType = bytes.NewReader(data), "application/json"
	}
	resp, err := client.CallRaw(c.Context(), call.method, raw.path, query, contentType, rd)
	if err != nil {
		if e.Class == registry.ClassSend {
			return deliverFailed(e, lastID(call.target.ids), err) // like deliver
		}
		if call.target.isList() {
			return pageTooLarge(err, hintAPIListTooLarge)
		}
		return err
	}
	return a.renderAPI(o, call, resp)
}

// confirmBodyRunes caps the body shown in a confirmation.
const confirmBodyRunes = 500

// apiDetails are the lines the confirmation of api adds (destructive and
// send, e.g. voucher pay, which shows the payment like the pay command):
// method with path and query (relative, so without host or userinfo) and
// the body as compact JSON, cut to confirmBodyRunes characters. confirm
// makes both printable.
func apiDetails(raw *rawRequest, body map[string]any) []Detail {
	req := raw.method + " " + raw.path
	if len(raw.query) > 0 {
		req += "?" + raw.query.Encode()
	}
	details := []Detail{{Label: "Anfrage", Value: req}}
	if body != nil {
		data, err := encodeBody(body)
		if err != nil {
			return details
		}
		text := string(data)
		if r := []rune(text); len(r) > confirmBodyRunes {
			text = string(r[:confirmBodyRunes]) + "…"
		}
		details = append(details, Detail{Label: "Body", Value: text})
	}
	return details
}

// checkAPIBody applies the rules of the body after it is built: for the
// send class the rules of deliver (checkSendBody), else --validate against
// the request schema (exit 5).
func checkAPIBody(call *apiCall, body map[string]any) error {
	if call.target.entry.Class == registry.ClassSend {
		return checkSendBody(call, body)
	}
	if !call.validate {
		return nil
	}
	req := spec.RequestSchema(call.target.op.ID)
	switch {
	case req == nil && body != nil:
		return clierr.New(clierr.CodeValidation, fmt.Sprintf("%s %s erwartet keinen Body", call.method, call.path.send),
			"ohne --input und Felder senden (bei DELETE werden Felder ohne --input Query-Parameter)")
	case req == nil:
		return nil
	case body == nil:
		return spec.Validate(map[string]any{}, req)
	}
	return spec.Validate(body, req)
}

// checkSendBody applies the rules of deliver (spec 9.1, R33) to the body
// of a send-class request, fail-closed and always, with or without
// --validate (exit 5): send_via must be set (not missing, not null: the
// API would pick a mode itself), send_via "email" needs recipient,
// subject and body (missingMail, shared with deliver), and the body is
// validated against the request schema exactly like deliverBody does
// (spec.Validate(body, spec.RequestSchema(opID))), which catches an email
// that is no object, fields that are no text and unknown fields. No body
// counts as {}. The query is rejected before (checkAPIFlags).
func checkSendBody(call *apiCall, body map[string]any) error {
	if body == nil {
		body = map[string]any{}
	}
	if body["send_via"] == nil {
		return clierr.New(clierr.CodeValidation, "send_via fehlt oder ist null: email oder pdf angeben",
			`-F send_via=pdf finalisiert ohne E-Mail; -F send_via=email verlangt email.recipient, email.subject und email.body`)
	}
	if body["send_via"] == viaEmail {
		if missing := missingMail(body["email"]); len(missing) > 0 {
			names := make([]string, len(missing))
			for i, f := range missing {
				names[i] = "email." + f.field
			}
			return clierr.New(clierr.CodeValidation, `send_via "email" verlangt `+strings.Join(names, ", "),
				"Empfänger, Betreff und Text angeben; ohne sie würde die API still ihre Standardmail an die Standardadresse senden")
		}
	}
	return spec.Validate(body, spec.RequestSchema(call.target.entry.OperationID))
}

// renderAPI writes the answer: an empty body as {"ok":true}, JSON through
// the output package with the columns of the matched command, any other
// answer unchanged, but only when stdout is no terminal. In a terminal that
// is exit 1 with the hint to redirect (afterRequest): the request was sent,
// so it is never exit 2, also for GET. A known file answer is refused
// before the request (checkAPIFile).
func (a *App) renderAPI(o output.Options, call *apiCall, resp *api.Response) error {
	if len(bytes.TrimSpace(resp.Body)) == 0 {
		return afterRequest(output.RenderValue(o, struct {
			OK bool `json:"ok"`
		}{true}, output.Columns{}))
	}
	if isJSONAnswer(resp) {
		return afterRequest(output.Render(o, resp.Body, call.target.entry.Columns))
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "ohne Content-Type"
	}
	desc := fmt.Sprintf("Antwort ist kein JSON (%s, %d Bytes)", ct, len(resp.Body))
	switch {
	case a.StdoutTTY:
		return afterRequest(clierr.New(clierr.CodeUsage, desc+": nicht im Terminal ausgegeben", fileHint(call.target)))
	case o.JQ != "" || len(o.Fields) > 0:
		return afterRequest(clierr.New(clierr.CodeUsage, desc+": --jq und --fields gelten nur für JSON", "ohne --jq und --fields ausführen"))
	}
	_, err := a.Out.Write(resp.Body)
	return afterRequest(err)
}

// isJSONAnswer reports a JSON answer: Content-Type application/json or
// *+json, or no Content-Type and a body that is valid JSON.
func isJSONAnswer(resp *api.Response) bool {
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return json.Valid(resp.Body)
	}
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
}
