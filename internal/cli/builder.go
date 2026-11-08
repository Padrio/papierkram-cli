package cli

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// BuildResourceCommands adds one command group per registry resource to
// root (spec 7.1), with the German aliases, and below it one command per
// registry entry. A resource whose entry has no action (info) becomes that
// command itself. Entries the generic builder does not make (see generic)
// get the command of their handler (customCmds) or, without one yet, none.
// Resources without any entry get no command yet.
func BuildResourceCommands(app *App, root *cobra.Command) {
	buildResourceCommands(app, root, registry.Resources(), registry.Entries())
}

// buildResourceCommands is BuildResourceCommands for the given resources
// and entries.
func buildResourceCommands(app *App, root *cobra.Command, resources []registry.Resource, entries []registry.Entry) {
	for _, r := range resources {
		var own []registry.Entry
		for _, e := range entries {
			if e.Resource == r.Name {
				own = append(own, e)
			}
		}
		if len(own) == 0 {
			continue
		}
		if len(own) == 1 && own[0].Action == "" {
			if cmd := app.entryCmd(own[0]); cmd != nil {
				cmd.Aliases = r.Aliases
				root.AddCommand(cmd)
			}
			continue
		}
		group := &cobra.Command{
			Use:     r.Name,
			Aliases: r.Aliases,
			Short:   r.Short,
			Args: func(c *cobra.Command, args []string) error {
				if len(args) > 0 {
					return unknownCommand(c, args[0])
				}
				return nil
			},
			RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
		}
		for _, e := range own {
			if cmd := app.entryCmd(e); cmd != nil {
				group.AddCommand(cmd)
			}
		}
		for _, build := range groupCmds[r.Name] {
			group.AddCommand(build(app))
		}
		root.AddCommand(group)
	}
}

// customCmds build the commands of Custom entries by action (their own
// handlers, Phase 8). Each handler runs the protection layer itself:
// mutationClient, then the body, then beforeMutation, and its command
// carries the pk:class annotation.
var customCmds = map[string]func(*App, registry.Entry) *cobra.Command{
	"deliver":         (*App).newDeliverCmd,
	"pdf":             (*App).newPDFCmd,
	"upload":          (*App).newUploadCmd,
	"document delete": (*App).newDocumentCmd,
	"pay":             (*App).newPayCmd,
}

// groupCmds are custom commands of a resource group without a registry
// entry of their own (time log and the timer, spec 9.6: the operation
// belongs to time create, whose entry they use for class, credits and
// guards).
var groupCmds = map[string][]func(*App) *cobra.Command{
	"time": {(*App).newTimeLogCmd, (*App).newTimeStartCmd, (*App).newTimeStatusCmd, (*App).newTimeStopCmd, (*App).newTimeDiscardCmd},
}

// entryExtras add a step of their own to a generic command, by operation
// (voucher create --file, spec 9.3; invoice create --from-estimate, spec
// 9.4; the read of invoice update that keeps the payment term, ruling
// E2E-A). Without their flags (the read: with a payment term given) the
// command runs exactly as the builder made it.
var entryExtras = map[string]func(*App, registry.Entry, *cobra.Command){
	"CreateVoucher": (*App).addCreateFile,
	"CreateInvoice": (*App).addFromEstimate,
	"UpdateInvoice": (*App).addKeepPaymentTerm,
}

// entryCmd builds the command of e: the generic one (with its extras), the
// one of its Custom handler, or nil for a Custom entry whose handler does
// not exist yet.
func (a *App) entryCmd(e registry.Entry) *cobra.Command {
	if generic(e) {
		cmd := a.newEntryCmd(e)
		if extend := entryExtras[e.OperationID]; extend != nil {
			extend(a, e, cmd)
		}
		return cmd
	}
	if build := customCmds[e.Action]; e.Custom && build != nil {
		return build(a, e)
	}
	return nil
}

// simpleActions are the actions handled by runSimpleAction, with the German
// past participle for the text output.
var simpleActions = map[string]string{
	"delete":    "gelöscht",
	"archive":   "archiviert",
	"unarchive": "aus dem Archiv geholt",
	"cancel":    "storniert",
	"reverse":   "mit Gegenbuchung storniert",
}

// newEntryCmd builds the command of one registry entry: list, describe (and
// info, Action ""), a simple action, or create/update. Path parameters other
// than the positional ID are required flags (see pathParamsOf).
func (a *App) newEntryCmd(e registry.Entry) *cobra.Command {
	params := pathParamsOf(e)
	cmd := &cobra.Command{
		Use:         e.Action,
		Short:       e.Short,
		Long:        entryLong(e),
		Example:     entryExample(e),
		Annotations: map[string]string{annotClass: string(e.Class)},
	}
	if e.Action == "" {
		cmd.Use = e.Resource
	}
	kind, err := entryKind(e)
	if err != nil {
		// fail closed: a command that could send a mutation past the
		// guards is never built (registry defect, caught by every test)
		panic(err)
	}
	positional := len(params) > 0 && params[len(params)-1].flag == ""
	switch {
	case positional && kind == kindDescribe && presetsID(e):
		// spec 9.8: without ID the preset (project file) decides
		cmd.Use += " [<id>]"
		cmd.Args = optionalIDArgs
	case positional:
		cmd.Use += " <id>"
		cmd.Args = idArgs
	default:
		cmd.Args = noArgs
	}
	for _, p := range params {
		switch {
		case p.flag != "" && p.flag == e.Preset:
			// open point 27: required, but PAPIERKRAM_<KEY> or the project
			// file may give it (pathValues, before the configuration is
			// resolved: no context default)
			cmd.Flags().Int64(p.flag, 0, fmt.Sprintf("ID des übergeordneten Objekts (%s); ohne Angabe %s (Pflicht)", p.name, presetNote(e.Preset)))
			markRequired(cmd.Flags(), p.flag)
			markPreset(cmd.Flags(), p.flag, e.Preset, false)
		case p.flag != "":
			cmd.Flags().Int64(p.flag, 0, fmt.Sprintf("ID des übergeordneten Objekts (%s, Pflicht)", p.name))
			markRequired(cmd.Flags(), p.flag)
		}
	}
	switch kind {
	case kindList:
		addListFlags(cmd, e)
		cmd.RunE = func(c *cobra.Command, _ []string) error { return a.runList(c, e) }
	case kindAction:
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runSimpleAction(c, e, args) }
	case kindWrite:
		addWriteFlags(cmd, e)
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runWrite(c, e, args) }
	default:
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runDescribe(c, e, args) }
	}
	return cmd
}

// The runners of the generic builder.
const (
	kindList     = "list"
	kindDescribe = "describe"
	kindAction   = "action"
	kindWrite    = "write"
)

// entryKind picks the runner of a generic entry and fails closed: list and
// describe (every other action, also info and pdf-like reads) send without
// guards, so they are only allowed for class read_only; simple actions and
// create/update run the guards and must not be read_only (the gate would
// let them pass). Any other combination, e.g. a destructive entry with an
// unknown action, is an error: such commands need a Custom handler.
func entryKind(e registry.Entry) (string, error) {
	_, simple := simpleActions[e.Action]
	readOnly := e.Class == registry.ClassReadOnly
	kind := kindDescribe
	switch {
	case e.Action == "list":
		kind = kindList
	case simple:
		kind = kindAction
	case isWrite(e):
		kind = kindWrite
	}
	mutating := kind == kindAction || kind == kindWrite
	if mutating == readOnly {
		return "", fmt.Errorf("registry: %s %s (%s) hat Klasse %q, der generische Builder führt sie als %s aus – Klasse korrigieren oder Custom-Handler verwenden",
			e.Resource, e.Action, e.OperationID, e.Class, kind)
	}
	return kind, nil
}

// entryLong is the German long help of an entry command: the short help
// plus the credit cost and, with a preset, where the value comes from.
func entryLong(e registry.Entry) string {
	cost := creditsText(e.Credits)
	long := fmt.Sprintf("%s.\n\nKosten: %s je Anfrage", e.Short, cost)
	if e.Action == "list" {
		long += " (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach)"
	}
	long += "."
	if e.Preset != "" {
		given := "--" + e.Preset
		if presetsID(e) {
			given = "die ID"
		}
		long += fmt.Sprintf("\n\nOhne %s gilt %s (anlegen: papierkram local init).", given, presetNote(e.Preset))
	}
	for _, flag := range localFlags(e) {
		long += fmt.Sprintf("\n\nDie API ignoriert diesen Filter: Mit %[1]s lädt das CLI alle Seiten wie --all "+
			"(je Seite %[2]s, Warnung ab 10 Seiten, im Terminal mit Rückfrage; --max-pages begrenzt) und filtert lokal. "+
			"--page und --limit gelten für das gefilterte Ergebnis: page, page_size, total_entries, total_pages und has_more "+
			"beziehen sich auf die gefilterten Einträge; mit --all kommen alle gefilterten Einträge. "+
			"Mit --max-pages zählen total_entries und total_pages nur die Treffer der geladenen Seiten; "+
			"has_more ist dann true, weil weitere Seiten Treffer enthalten können.", flag, cost)
	}
	return long
}

// creditsText is "1 Credit" or "N Credits".
func creditsText(n int) string {
	if n == 1 {
		return "1 Credit"
	}
	return fmt.Sprintf("%d Credits", n)
}

// entryExample indents the registry examples like cobra expects.
func entryExample(e registry.Entry) string {
	lines := make([]string, len(e.Examples))
	for i, ex := range e.Examples {
		lines[i] = "  " + ex
	}
	return strings.Join(lines, "\n")
}

// pathParam is a parameter of an entry's path template. flag is the name of
// the flag that sets it, empty for the positional ID.
type pathParam struct {
	name, flag string
}

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)

// pathParamsOf returns the path parameters of e in template order. For
// every action but list and create (which work on a collection) the last
// one is the positional ID; all others come
// from required flags named after the parameter without "_id" in
// kebab-case ({company_id} → --company).
func pathParamsOf(e registry.Entry) []pathParam {
	m := pathParamRe.FindAllStringSubmatch(e.Path, -1)
	params := make([]pathParam, len(m))
	for i, sub := range m {
		params[i] = pathParam{name: sub[1], flag: strings.ReplaceAll(strings.TrimSuffix(sub[1], "_id"), "_", "-")}
	}
	if e.Action != "list" && e.Action != "create" && len(params) > 0 {
		params[len(params)-1].flag = ""
	}
	return params
}

// parseID parses a non-negative int64 ID; 0 is valid (R24).
func parseID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id >= 0
}

// idArgs requires exactly one argument that is an ID (integer ≥ 0).
func idArgs(c *cobra.Command, args []string) error {
	if err := exactArgs(1)(c, args); err != nil {
		return err
	}
	if _, ok := parseID(args[0]); !ok {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige ID %q: ganze Zahl ≥ 0 erwartet", args[0]), "Verwendung: "+useLine(c))
	}
	return nil
}

// optionalIDArgs allows no argument or one ID; without it the ID comes
// from the preset (see presetArgs).
func optionalIDArgs(c *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return idArgs(c, args)
}

// presetsID reports whether the preset of e is the positional ID of its
// describe (spec 9.8: project describe without ID); any other preset sets
// a flag (see pathValues, presetQuery, presetBody) and keeps the ID
// required.
func presetsID(e registry.Entry) bool {
	return e.Preset != "" && e.Preset == e.Resource && e.Action == "describe"
}

// pathValues collects the path parameter values of e in template order:
// required flags for all but the positional ID, which is args[0]. The
// flag of e's preset (person … --company, open point 27) comes, when not
// given, from the preset like the other presets: PAPIERKRAM_<KEY>, then
// the project file (presetFile), no context default; its hint is queued
// for flushPresetHints. Without a value it is a usage error (exit 2).
func (a *App) pathValues(c *cobra.Command, e registry.Entry, args []string) ([]int64, error) {
	params := pathParamsOf(e)
	values := make([]int64, len(params))
	for i, p := range params {
		if p.flag == "" {
			values[i], _ = parseID(args[0]) // checked by idArgs
			continue
		}
		if !c.Flags().Changed(p.flag) && p.flag == e.Preset {
			pr, err := a.preset(e.Preset)
			if err != nil {
				return nil, err
			}
			if pr.Source == "" {
				return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("Pflichtangabe --%s fehlt", p.flag),
					fmt.Sprintf("--%[1]s angeben oder voreinstellen: %[2]s oder %[1]s in %[3]s (papierkram local init --%[1]s <id>)",
						e.Preset, presetEnv(e.Preset), projectfile.FileName))
			}
			a.presetHint(e.Preset, pr)
			values[i] = pr.ID
			continue
		}
		if !c.Flags().Changed(p.flag) {
			return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("Pflichtangabe --%s fehlt", p.flag), "Verwendung: "+useLine(c)+" --"+p.flag+" <id>")
		}
		v, _ := c.Flags().GetInt64(p.flag)
		if v < 0 {
			return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültiger Wert %d für --%s: ID muss ≥ 0 sein", v, p.flag), "")
		}
		values[i] = v
	}
	return values, nil
}

// render writes an API response in the effective output format. It runs
// after a request was sent, so its errors go through afterRequest.
func (a *App) render(c *cobra.Command, raw []byte, cols output.Columns) error {
	return afterRequest(output.Render(a.outputOptions(c.Context(), a.outputFormat(a.resolved)), raw, cols))
}

// afterRequest maps every error that happens after a request was sent
// (output, rendering, a 2xx answer that is no JSON, a failed write to
// stdout) to exit 1 with the prefix outputFailed: a usage error (exit 2)
// would tell agents that nothing happened, and without the prefix an
// agent could not tell that the command itself went through.
func afterRequest(err error) error {
	if err == nil {
		return nil
	}
	ce := clierr.From(err)
	if ce.Code == clierr.CodeError && strings.HasPrefix(ce.Message, outputFailed) {
		return ce
	}
	out := clierr.New(clierr.CodeError, outputFailed+ce.Message, ce.Hint)
	out.Cause = err
	return out
}

// outputFailed starts the message of an error after a sent request
// (AGENTS.md: then do not repeat).
const outputFailed = "Befehl ausgeführt, Ausgabe fehlgeschlagen: "

// beforeMutation is the single hook every mutating builder path calls
// after the body is built and validated, right before sending (plan order
// steps 4 and 5). It repeats the read-only gate (step 2, which ran in
// mutationClient before the body was read) so no path sends without it.
// With --dry-run it prints the request and returns false: nothing is sent,
// exit 0, no confirmation. Destructive and send classes then need the
// confirmation (exit 8); the summary GET runs only when a prompt follows,
// and for the send class the summary adds how and to whom (from body).
// body is the request body, nil for actions without one. proceed reports
// whether the caller sends the request.
func (a *App) beforeMutation(c *cobra.Command, e registry.Entry, client *api.Client, params []int64, body map[string]any) (proceed bool, err error) {
	return a.beforeSend(c, e, client, params, body, sendExtras{})
}

// sendExtras are what a handler adds to beforeMutation: the file of an
// upload and a read already sent to build the body (--from-estimate),
// both shown by --dry-run (spec 8.4), a summary of its own for the
// confirmation, used instead of the one loaded by fetchSummary (an object
// without a GET of its own, such as a document of a voucher), and details
// added to either summary (the payment of voucher pay). raw is the
// request of papierkram api, which --dry-run shows instead of the one
// built from the operation of e (path and query as given).
type sendExtras struct {
	upload  *UploadInfo
	read    *ReadInfo
	summary *Summary
	details []Detail
	raw     *rawRequest
}

// rawRequest is a request sent with api.Client.CallRaw.
type rawRequest struct {
	method, path string
	query        url.Values
}

// preview is the method and URL a dry run shows: those of the raw request
// of x, else of the operation of e.
func (x sendExtras) preview(client *api.Client, e registry.Entry, params []int64) (string, string, error) {
	if x.raw != nil {
		return client.PreviewRaw(x.raw.method, x.raw.path, x.raw.query)
	}
	return client.Preview(e.OperationID, params, nil)
}

// beforeSend is beforeMutation with the extras of a handler.
func (a *App) beforeSend(c *cobra.Command, e registry.Entry, client *api.Client, params []int64, body map[string]any, x sendExtras) (proceed bool, err error) {
	if err := a.guardMutation(e, a.resolved); err != nil {
		return false, err
	}
	if a.Flags.DryRun {
		method, url, err := x.preview(client, e, params)
		if err != nil {
			return false, err
		}
		var b any
		if body != nil {
			b = body
		}
		return false, a.dryRun(c.Context(), e, method, url, b, x)
	}
	if !confirmed(e) {
		return true, nil
	}
	s := Summary{Resource: resourceNoun(e.Resource), ID: lastID(params)}
	if x.summary != nil {
		s = *x.summary
	}
	ask, err := a.mustAsk(e, s)
	if err != nil {
		return false, err
	}
	if !ask {
		return true, nil
	}
	if x.summary == nil {
		if s, err = a.fetchSummary(c.Context(), client, e, params); err != nil {
			return false, err
		}
	}
	if e.Class == registry.ClassSend {
		s.Via, s.Recipient, s.Subject = sendSummary(body)
	}
	s.Details = x.details
	if err := a.confirm(c.Context(), e, s); err != nil {
		return false, err
	}
	return true, nil
}
