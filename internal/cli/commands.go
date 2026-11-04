package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// commands --json (spec 7.2, 11): the machine-readable list of every
// command, the contract for agents (docs/schemas/commands.json). It is
// built by walking the cobra tree, so no registered command can be
// missing, and joins each command to its registry entry.

// commandsVersion is the version of the contract; it changes only with
// incompatible changes.
const commandsVersion = 1

// The classes commands --json reports besides the registry classes
// (spec 8.1). classLocal: the command acts on local state only (config,
// keychain, project file, timer, the CLI's own output) and costs nothing
// unless it names an operation (auth login verifies the token with GET
// /info). classPerRequest: the class is decided per request (papierkram
// api, see apiTarget).
const (
	classLocal      = "local"
	classPerRequest = "per_request"
)

// annotOperation names the operation a command outside the registry sends
// (time log and time stop: CreateTimeEntry; auth login and auth status:
// GetInfo). commands --json and schema report it with its registry entry.
const annotOperation = "pk:operation"

// annotPerRequest marks papierkram api: its pk:class is a fail-closed
// default, the effective class depends on method and path.
const annotPerRequest = "pk:per-request"

// Flag annotations describe a flag for commands --json; they are set
// where the flag is defined, from the same values its help and checks
// use.
const (
	flagRequired    = "pk:required"     // "true": the help says Pflicht
	flagEnum        = "pk:enum"         // the allowed values
	flagLocalFilter = "pk:local-filter" // "true": filter the CLI applies itself (spec 9.7)
	flagDefaultFrom = "pk:default-from" // sources of the value without the flag (spec 9.8)
	flagEnv         = "pk:env"          // environment variable that sets the value
	flagValueType   = "pk:type"         // the contract type when the pflag type says less (date filters are text flags)
	flagAlternative = "pk:alternatives" // other flags that satisfy the same requirement (--items for --item)
)

// annotate sets a flag annotation; a flag that does not exist is a
// programming error.
func annotate(fs *pflag.FlagSet, name, key string, values ...string) {
	if err := fs.SetAnnotation(name, key, values); err != nil {
		panic(err)
	}
}

// markRequired marks a flag the help calls Pflicht.
func markRequired(fs *pflag.FlagSet, name string) { annotate(fs, name, flagRequired, "true") }

// addAlternatives records flags (with dashes) that satisfy the
// requirement of the required flag name as well; the list stays sorted
// and unique.
func addAlternatives(fs *pflag.FlagSet, name string, alts ...string) {
	all := append(slices.Clone(fs.Lookup(name).Annotations[flagAlternative]), alts...)
	slices.Sort(all)
	annotate(fs, name, flagAlternative, slices.Compact(all)...)
}

// markEnum records the allowed values of a flag (nothing without values).
func markEnum(fs *pflag.FlagSet, name string, values []string) {
	if len(values) > 0 {
		annotate(fs, name, flagEnum, values...)
	}
}

// markPreset records where the value of a preset flag comes from when it
// is not given (spec 9.8, resolvePreset): flag, PAPIERKRAM_<KEY>, the
// project file and, for user once the configuration is resolved
// (withContext), the user of the context.
func markPreset(fs *pflag.FlagSet, name, key string, withContext bool) {
	from := []string{config.SourceFlag, config.SourceEnv, config.SourceProjectFile}
	if withContext && key == presetUser {
		from = append(from, config.SourceContext)
	}
	annotate(fs, name, flagDefaultFrom, from...)
	annotate(fs, name, flagEnv, presetEnv(key))
}

// commandsDoc is the output of commands --json.
type commandsDoc struct {
	Version     int           `json:"version"`
	Commands    []commandInfo `json:"commands"`
	GlobalFlags []flagInfo    `json:"global_flags"`
}

// commandInfo is one command. Usage is the German use line with the
// positional arguments (useLine). Credits is nil for class per_request;
// OperationID, Method and Path are empty without a fixed operation.
type commandInfo struct {
	Command     string     `json:"command"`
	Usage       string     `json:"usage"`
	Aliases     []string   `json:"aliases"`
	Short       string     `json:"short"`
	Class       string     `json:"class"`
	Credits     *int       `json:"credits"`
	Blocked     bool       `json:"blocked_by_read_only"`
	OperationID string     `json:"operation_id,omitempty"`
	Method      string     `json:"method,omitempty"`
	Path        string     `json:"path,omitempty"`
	Flags       []flagInfo `json:"flags"`
	Examples    []string   `json:"examples"`
}

// flagInfo is one flag; Shorthand is omitted when the flag has none.
type flagInfo struct {
	Name         string   `json:"name"`
	Shorthand    string   `json:"shorthand,omitempty"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	Repeatable   bool     `json:"repeatable"`
	Enum         []string `json:"enum"`
	Description  string   `json:"description"`
	LocalFilter  bool     `json:"local_filter,omitempty"`
	DefaultFrom  []string `json:"default_from,omitempty"`
	Env          string   `json:"env,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
}

// commandMeta is what a command sends: its registry entry (op false
// without one), the reported class and the credits (nil: per request).
type commandMeta struct {
	entry   registry.Entry
	op      bool
	class   string
	credits *int
}

// metaOf joins c to the registry: a registry command by its path
// (<resource> <action>, info without action), another command by its
// annotOperation. With an operation the class and credits are those of
// the entry, except for commands that write the configuration (auth
// login), which are local. papierkram api is per_request; everything else
// sends nothing: local, 0 credits. commands --json and schema share it.
func metaOf(c *cobra.Command) commandMeta {
	if c.Annotations[annotPerRequest] == "true" {
		return commandMeta{class: classPerRequest}
	}
	words := commandWords(c)
	e, ok := registry.Entry{}, false
	if len(words) > 0 {
		e, ok = registry.Lookup(words[0], strings.Join(words[1:], " "))
	}
	if !ok {
		if id := c.Annotations[annotOperation]; id != "" {
			e, ok = registry.ByOperationID(id)
		}
	}
	if !ok {
		zero := 0
		return commandMeta{class: classLocal, credits: &zero}
	}
	m := commandMeta{entry: e, op: true, class: string(e.Class), credits: &e.Credits}
	if c.Annotations[annotWritesConfig] == "true" {
		m.class = classLocal
	}
	return m
}

// readOnlyBlocked reports whether c can exit 9 under PAPIERKRAM_READ_ONLY
// (spec 8.3): it writes the configuration (guardConfigWrite) or carries a
// class other than read_only, which the root and guardMutation block
// (mutations, time start, api with a mutating request).
func readOnlyBlocked(c *cobra.Command) bool {
	class := c.Annotations[annotClass]
	return c.Annotations[annotWritesConfig] == "true" || (class != "" && class != string(registry.ClassReadOnly))
}

// commandWords is the command path of c without the root.
func commandWords(c *cobra.Command) []string {
	var words []string
	for ; c.HasParent(); c = c.Parent() {
		words = append([]string{c.Name()}, words...)
	}
	return words
}

// commandsList returns the visible commands below root that run
// something (groups only print help), sorted by command path.
func commandsList(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	walk(root, func(c *cobra.Command) {
		if c != root && !c.Hidden && !c.HasSubCommands() && hiddenParents(c) == 0 {
			out = append(out, c)
		}
	})
	slices.SortFunc(out, func(a, b *cobra.Command) int {
		return strings.Compare(strings.Join(commandWords(a), " "), strings.Join(commandWords(b), " "))
	})
	return out
}

// hiddenParents counts the hidden commands above c.
func hiddenParents(c *cobra.Command) int {
	n := 0
	for p := c.Parent(); p != nil; p = p.Parent() {
		if p.Hidden {
			n++
		}
	}
	return n
}

// buildCommandsDoc builds commands --json for the tree of root. cobra adds
// its help command when it executes; it is added here too, so the result
// does not depend on that.
func buildCommandsDoc(root *cobra.Command) (commandsDoc, error) {
	root.InitDefaultHelpCmd()
	doc := commandsDoc{Version: commandsVersion, Commands: []commandInfo{}}
	for _, c := range commandsList(root) {
		info, err := commandInfoOf(c)
		if err != nil {
			return commandsDoc{}, err
		}
		doc.Commands = append(doc.Commands, info)
	}
	var err error
	doc.GlobalFlags, err = flagInfos(root.PersistentFlags())
	return doc, err
}

func commandInfoOf(c *cobra.Command) (commandInfo, error) {
	m := metaOf(c)
	info := commandInfo{
		Command:  strings.Join(commandWords(c), " "),
		Usage:    useLine(c),
		Aliases:  commandAliases(c),
		Short:    c.Short,
		Class:    m.class,
		Credits:  m.credits,
		Blocked:  readOnlyBlocked(c),
		Examples: commandExamples(c),
	}
	if m.op {
		info.OperationID, info.Method, info.Path = m.entry.OperationID, m.entry.Method, m.entry.Path
	}
	flags, err := flagInfos(c.LocalNonPersistentFlags())
	if err != nil {
		return commandInfo{}, fmt.Errorf("%s: %w", info.Command, err)
	}
	info.Flags = flags
	return info, nil
}

// commandAliases are all other spellings of the command path of c (each
// word by its name or one of its aliases), sorted.
func commandAliases(c *cobra.Command) []string {
	paths := []string{""}
	var chain []*cobra.Command
	for p := c; p.HasParent(); p = p.Parent() {
		chain = append([]*cobra.Command{p}, chain...)
	}
	for _, p := range chain {
		var next []string
		for _, prefix := range paths {
			for _, w := range append([]string{p.Name()}, p.Aliases...) {
				next = append(next, strings.TrimSpace(prefix+" "+w))
			}
		}
		paths = next
	}
	out := []string{}
	self := strings.Join(commandWords(c), " ")
	for _, p := range paths {
		if p != self {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// commandExamples are the example lines of c without indentation (for
// registry commands its Examples, see entryExample).
func commandExamples(c *cobra.Command) []string {
	out := []string{}
	for line := range strings.SplitSeq(c.Example, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// flagTypes map pflag value types to the types of the contract. int64
// flags are the ID flags of list filters and path parameters.
var flagTypes = map[string]string{
	"string":      "string",
	"stringArray": "string",
	"bool":        "boolean",
	"int":         "integer",
	"integer":     "integer",
	"number":      "number",
	"int64":       "id",
	"id":          "id",
	"date":        "date",
	"duration":    "duration",
}

// flagInfos describes the visible flags of fs (without --help), sorted by
// name.
func flagInfos(fs *pflag.FlagSet) ([]flagInfo, error) {
	out := []flagInfo{}
	var err error
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" || err != nil {
			return
		}
		typ, ok := flagTypes[f.Value.Type()]
		if t := f.Annotations[flagValueType]; len(t) == 1 {
			typ, ok = t[0], true
		}
		if !ok {
			err = fmt.Errorf("unbekannter Typ %q des Flags --%s", f.Value.Type(), f.Name)
			return
		}
		enum := f.Annotations[flagEnum]
		if enum == nil {
			enum = []string{}
		}
		out = append(out, flagInfo{
			Name:         f.Name,
			Shorthand:    f.Shorthand,
			Type:         typ,
			Required:     f.Annotations[flagRequired] != nil,
			Repeatable:   f.Value.Type() == "stringArray",
			Enum:         slices.Clone(enum),
			Description:  f.Usage,
			LocalFilter:  f.Annotations[flagLocalFilter] != nil,
			DefaultFrom:  slices.Clone(f.Annotations[flagDefaultFrom]),
			Env:          strings.Join(f.Annotations[flagEnv], ""),
			Alternatives: slices.Clone(f.Annotations[flagAlternative]),
		})
	})
	return out, err
}

// commandsColumns are the columns of commands without --json.
var commandsColumns = output.Columns{Table: []output.Column{
	{Header: "COMMAND", Path: "command"},
	{Header: "KLASSE", Path: "class"},
	{Header: "CREDITS", Path: "credits"},
	{Header: "KURZ", Path: "short"},
}}

const commandsLong = `Listet alle Befehle mit Klasse (Spec 8.1), Credits je Anfrage und Kurzhilfe.

Mit --json (oder -o json|yaml, --jq) kommt die maschinenlesbare Liste für Agents: je Befehl Verwendungszeile mit Positionsargumenten, Aliase, Kurzhilfe, Klasse, Credits, Operation (operation_id, method, path), Flags mit Kurzform, Typ, Pflicht (mit gleichwertigen Flags unter alternatives), Wiederholbarkeit, erlaubten Werten und Hilfetext sowie Beispiele; die globalen Flags stehen einmal unter global_flags. Vertrag: docs/schemas/commands.json (Version 1).

Klassen außer denen der Spec: local wirkt nur lokal (Konfiguration, Schlüsselbund, Projektdatei, Timer) und kostet nichts, außer der Befehl nennt eine Operation (auth login prüft das Token per GET /info); per_request entscheidet sich je Anfrage (papierkram api), credits ist dann null. blocked_by_read_only sagt, ob der Befehl unter PAPIERKRAM_READ_ONLY mit Exit 9 abbricht.

Sendet keine Anfrage.`

func newCommandsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "Alle Befehle mit Klasse, Credits, Flags und Beispielen auflisten",
		Long:  commandsLong,
		Example: "  papierkram commands\n" +
			"  papierkram commands --json\n" +
			`  papierkram commands --json --jq '.commands[] | select(.class == "destructive") | .command'`,
		Annotations: readsOnly(),
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			asJSON, _ := c.Flags().GetBool("json")
			format := app.Flags.Output
			if asJSON {
				if app.Flags.OutputSet && format != string(output.FormatJSON) {
					return usageErr(fmt.Sprintf("--json und -o %s widersprechen sich", format))
				}
				format = string(output.FormatJSON)
			}
			doc, err := buildCommandsDoc(c.Root())
			if err != nil {
				return err
			}
			o := app.outputOptions(c.Context(), format)
			if o.Structured() {
				return output.RenderValue(o, doc, output.Columns{})
			}
			return output.RenderValue(o, doc.Commands, commandsColumns)
		},
	}
	cmd.Flags().Bool("json", false, "maschinenlesbare Liste als JSON (wie -o json)")
	return cmd
}
