package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/mcpserver"
)

// The tools of mcp serve (spec v0.2 §3) are built from commands --json,
// the same description agents read: one tool per command with its flags
// and positional arguments as parameters. A call runs the command line
// the tool describes (RunContext), so a tool can never do more than the
// command.

// mcpExcludedFirst are the first command words without tools: local
// commands, the meta commands and papierkram api (spec v0.2 §3).
var mcpExcludedFirst = []string{"auth", "context", "local", "completion", "api", "commands", "schema", "version", "help", "gen-docs", "mcp"}

// mcpExcludedCommands are further commands without tools: the timer works
// on a local file of the machine (time stop sends, but only for a timer
// started there).
var mcpExcludedCommands = []string{"time start", "time stop", "time status", "time discard"}

// mcpPositional is the parameter of a positional argument token.
type mcpPositional struct {
	name, desc string
	kind       mcpserver.Kind
}

// mcpPositionals map the tokens of the use lines (without [ ]) to
// parameters; an unknown token stops mcpToolSpecs (fail closed).
var mcpPositionals = map[string]mcpPositional{
	"<id>":       {"id", "ID", mcpserver.KindInteger},
	"<beleg-id>": {"voucher_id", "ID des Belegs", mcpserver.KindInteger},
	"<datei-id>": {"document_id", "ID der Datei am Beleg", mcpserver.KindInteger},
	"<datei>":    {"file", "Pfad der Datei, relativ zum Dateiordner des Servers (mcp serve --files-dir)", mcpserver.KindPath},
}

// mcpReserved is the parameter of the dry run switch of the write tools
// (task 6); no flag may take its name.
const mcpReserved = "dry_run"

// mcpPathFlags take a file path; "-" (stdin/stdout, the transport of the
// server) is not allowed there.
var mcpPathFlags = []string{"output-file", "file", "body-file"}

// mcpFixedFlags are flags that are no parameters but set on every call:
// a pdf tool never replaces a file (ruling T5-B).
var mcpFixedFlags = []string{"no-clobber"}

// mcpToolSpecs are the tool specs of every command of root that may have
// a tool, sorted by name; Tools picks those of a mode.
func mcpToolSpecs(root *cobra.Command) ([]mcpserver.ToolSpec, error) {
	doc, err := buildCommandsDoc(root)
	if err != nil {
		return nil, err
	}
	return mcpToolSpecsOf(doc.Commands)
}

// mcpToolSpecsOf are the tool specs of the commands cmds of commands
// --json, sorted by name.
func mcpToolSpecsOf(cmds []commandInfo) ([]mcpserver.ToolSpec, error) {
	var specs []mcpserver.ToolSpec
	names := map[string]bool{}
	for _, c := range cmds {
		if mcpExcluded(c) {
			continue
		}
		s, err := mcpToolSpec(c)
		if err != nil {
			return nil, err
		}
		if names[s.Name] {
			return nil, fmt.Errorf("mcp: Tool-Name %s doppelt", s.Name)
		}
		names[s.Name] = true
		specs = append(specs, s)
	}
	slices.SortFunc(specs, func(a, b mcpserver.ToolSpec) int { return strings.Compare(a.Name, b.Name) })
	return specs, nil
}

// mcpExcluded reports whether c gets no tool.
func mcpExcluded(c commandInfo) bool {
	return c.Class == classLocal || c.Class == classPerRequest ||
		slices.Contains(mcpExcludedFirst, strings.Fields(c.Command)[0]) || slices.Contains(mcpExcludedCommands, c.Command)
}

// mcpName is the parameter or tool name of a flag or command words.
func mcpName(s string) string { return strings.NewReplacer(" ", "_", "-", "_").Replace(s) }

// mcpToolSpec is the tool of command c.
func mcpToolSpec(c commandInfo) (mcpserver.ToolSpec, error) {
	s := mcpserver.ToolSpec{
		Name:    mcpName(c.Command),
		Class:   c.Class,
		Command: strings.Fields(c.Command),
	}
	pos, err := mcpPositionalParams(c)
	if err != nil {
		return s, err
	}
	s.Params = pos
	resolves, perPage := false, false
	for _, f := range c.Flags {
		if slices.Contains(mcpFixedFlags, f.Name) {
			s.Fixed = append(s.Fixed, "--"+f.Name)
			continue
		}
		p, err := mcpFlagParam(f)
		if err != nil {
			return s, fmt.Errorf("mcp: %s --%s: %w", c.Command, f.Name, err)
		}
		if p.Name == mcpReserved || slices.ContainsFunc(s.Params, func(q mcpserver.Param) bool { return q.Name == p.Name }) {
			return s, fmt.Errorf("mcp: %s --%s: Parametername %s doppelt oder reserviert", c.Command, f.Name, p.Name)
		}
		resolves = resolves || f.Resolves != ""
		perPage = perPage || f.Name == "max-pages"
		s.Params = append(s.Params, p)
	}
	credits := 0
	if c.Credits != nil {
		credits = *c.Credits
	}
	unit := "je Aufruf"
	if perPage {
		unit = "je Seite"
	}
	s.Description = fmt.Sprintf("%s. Klasse %s, %s %s", strings.TrimSuffix(c.Short, "."), c.Class, creditsText(credits), unit)
	if resolves {
		s.Description += "; ID-Felder nehmen auch Namen (1 Credit je geladener Seite)"
	}
	return s, nil
}

// mcpPositionalParams are the parameters of the positional arguments in
// the use line of c ("papierkram <command> <id> [Optionen]").
func mcpPositionalParams(c commandInfo) ([]mcpserver.Param, error) {
	rest, ok := strings.CutPrefix(c.Usage, "papierkram "+c.Command)
	if !ok {
		return nil, fmt.Errorf("mcp: %s: Verwendung %q ohne Befehl", c.Command, c.Usage)
	}
	toks := strings.Fields(rest)
	if len(toks) == 0 || toks[len(toks)-1] != "[Optionen]" {
		return nil, fmt.Errorf("mcp: %s: Verwendung %q endet nicht mit [Optionen]", c.Command, c.Usage)
	}
	var out []mcpserver.Param
	for i, tok := range toks[:len(toks)-1] {
		bare, optional := strings.CutPrefix(tok, "[")
		if optional {
			bare = strings.TrimSuffix(bare, "]")
		}
		m, ok := mcpPositionals[bare]
		if !ok {
			return nil, fmt.Errorf("mcp: %s: Positionsargument %s ohne Zuordnung", c.Command, tok)
		}
		p := mcpserver.Param{Name: m.name, Pos: i, Kind: m.kind, Description: m.desc, Required: !optional}
		if m.kind == mcpserver.KindInteger {
			p.Minimum = new(float64)
		}
		out = append(out, p)
	}
	return out, nil
}

// mcpFlagParam is the parameter of flag f: kind from its type, enum,
// typed default and bounds as in commands --json. A flag is required only
// when commands --json calls it required without alternatives and
// without a preset (the CLI checks the rest); --output-file is required
// because without it the PDF would go to stdout, which is no JSON.
func mcpFlagParam(f flagInfo) (mcpserver.Param, error) {
	p := mcpserver.Param{
		Name:        mcpName(f.Name),
		Flag:        f.Name,
		Pos:         -1,
		Description: f.Description,
		Enum:        f.Enum,
		Minimum:     f.Minimum,
		Maximum:     f.Maximum,
		Required:    f.Required && len(f.Alternatives) == 0 && len(f.DefaultFrom) == 0,
	}
	if len(p.Enum) == 0 {
		p.Enum = nil
	}
	switch {
	case f.Name == "input":
		p.Kind, p.JSONType = mcpserver.KindJSONFile, "object"
		p.Description = "Body als JSON-Objekt (das CLI erhält es als Datei für --input); Flags überschreiben einzelne Felder"
	case f.Name == "items":
		p.Kind, p.JSONType = mcpserver.KindJSONFile, "array"
		p.Description = "Positionen als JSON-Array (das CLI erhält es als Datei für --items)"
	case slices.Contains(mcpPathFlags, f.Name):
		p.Kind = mcpserver.KindPath
		p.Description = strings.NewReplacer(" oder - für stdout", "", " oder - für die Standardeingabe", "").Replace(p.Description) +
			"; relativer Pfad im Dateiordner des Servers (mcp serve --files-dir), ohne „..“"
		if f.Name == "output-file" {
			p.Description += "; eine vorhandene Datei wird nicht ersetzt"
			p.Required = true
		}
	case f.Repeatable:
		p.Kind = mcpserver.KindStrings
	default:
		kind, ok := map[string]mcpserver.Kind{
			"id": mcpserver.KindInteger, "integer": mcpserver.KindInteger, "number": mcpserver.KindNumber,
			"boolean": mcpserver.KindBool, "string": mcpserver.KindString, "date": mcpserver.KindString, "duration": mcpserver.KindString,
		}[f.Type]
		if !ok {
			return p, fmt.Errorf("unbekannter Typ %q ohne Tool-Parameter", f.Type)
		}
		p.Kind = kind
	}
	switch {
	case f.Type == "id" && p.Kind == mcpserver.KindInteger && f.Resolves != "":
		// a repeatable ID flag stays a list of texts (IDs or names)
		p.Kind = mcpserver.KindRef
	case f.Type == "id" && p.Kind == mcpserver.KindInteger && p.Minimum == nil:
		p.Minimum = new(float64)
	case f.Type == "date" && !strings.Contains(p.Description, "YYYY-MM-DD"):
		p.Description += " (Datum: YYYY-MM-DD, heute, gestern)"
	case f.Type == "duration":
		p.Description += " (Dauer, z. B. 30s, 5m oder 1h30m)"
	}
	if f.Credits != nil && !strings.Contains(p.Description, "Credit") {
		p.Description += fmt.Sprintf(" (zusätzlich %s)", creditsText(*f.Credits))
	}
	if f.Default != "" {
		d, err := mcpDefault(p.Kind, f.Default)
		if err != nil {
			return p, err
		}
		p.Default = d
	}
	return p, nil
}

// mcpDefault is the default of commands --json as a JSON value of kind.
func mcpDefault(kind mcpserver.Kind, v string) (any, error) {
	switch kind {
	case mcpserver.KindInteger:
		return strconv.ParseInt(v, 10, 64)
	case mcpserver.KindNumber:
		return strconv.ParseFloat(v, 64)
	case mcpserver.KindBool:
		return strconv.ParseBool(v)
	case mcpserver.KindString:
		return v, nil
	}
	return nil, fmt.Errorf("unbekannte Art %s für den Standardwert %q", kind, v)
}
