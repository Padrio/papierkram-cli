package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// BuildResourceCommands adds one command group per registry resource to
// root (spec 7.1), with the German aliases, and below it one command per
// registry entry. A resource whose entry has no action (info) becomes that
// command itself. Entries the generic builder does not make (see generic)
// are skipped: their handlers add themselves to the group. Resources without
// any entry get no command yet.
func BuildResourceCommands(app *App, root *cobra.Command) {
	entries := registry.Entries()
	for _, r := range registry.Resources() {
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
			if generic(own[0]) {
				cmd := app.newEntryCmd(own[0])
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
			if generic(e) {
				group.AddCommand(app.newEntryCmd(e))
			}
		}
		root.AddCommand(group)
	}
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
		Use:     e.Action,
		Short:   e.Short,
		Long:    entryLong(e),
		Example: entryExample(e),
	}
	if e.Action == "" {
		cmd.Use = e.Resource
	}
	positional := len(params) > 0 && params[len(params)-1].flag == ""
	if positional {
		cmd.Use += " <id>"
		cmd.Args = idArgs
	} else {
		cmd.Args = noArgs
	}
	for _, p := range params {
		if p.flag != "" {
			cmd.Flags().Int64(p.flag, 0, fmt.Sprintf("ID des übergeordneten Objekts (%s, Pflicht)", p.name))
		}
	}
	_, simple := simpleActions[e.Action]
	switch {
	case e.Action == "list":
		addListFlags(cmd, e)
		cmd.RunE = func(c *cobra.Command, _ []string) error { return a.runList(c, e) }
	case simple:
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runSimpleAction(c, e, args) }
	case isWrite(e):
		addWriteFlags(cmd, e)
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runWrite(c, e, args) }
	default:
		cmd.RunE = func(c *cobra.Command, args []string) error { return a.runDescribe(c, e, args) }
	}
	return cmd
}

// entryLong is the German long help of an entry command: the short help
// plus the credit cost.
func entryLong(e registry.Entry) string {
	unit := "Credits"
	if e.Credits == 1 {
		unit = "Credit"
	}
	long := fmt.Sprintf("%s.\n\nKosten: %d %s je Anfrage", e.Short, e.Credits, unit)
	if e.Action == "list" {
		long += " (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach)"
	}
	return long + "."
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

// pathValues collects the path parameter values of e in template order:
// required flags for all but the positional ID, which is args[0].
func pathValues(c *cobra.Command, e registry.Entry, args []string) ([]int64, error) {
	params := pathParamsOf(e)
	values := make([]int64, len(params))
	for i, p := range params {
		if p.flag == "" {
			values[i], _ = parseID(args[0]) // checked by idArgs
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

// afterRequest maps an error that happens after a request was sent (output,
// rendering) to exit 1: a usage error (exit 2) would tell agents that
// nothing happened.
func afterRequest(err error) error {
	if err == nil {
		return nil
	}
	ce := clierr.From(err)
	if ce.Code != clierr.CodeUsage {
		return ce
	}
	out := clierr.New(clierr.CodeError, "Befehl ausgeführt, Ausgabe fehlgeschlagen: "+ce.Message, ce.Hint)
	out.Cause = err
	return out
}

// beforeMutation is the single hook every mutating builder path calls
// after the arguments are checked and the configuration is resolved, right
// before the request is sent; body is the finished request body (nil for
// actions without one). The protection rules of spec 8 (read-only lock,
// --dry-run, confirmation; Task 24) go here. It does nothing yet.
func (a *App) beforeMutation(_ *cobra.Command, _ registry.Entry, _ *config.Resolved, _ []int64, _ map[string]any) error {
	return nil
}
