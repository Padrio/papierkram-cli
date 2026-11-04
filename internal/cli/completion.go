package cli

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/namecache"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// Shell completion (spec 7.2, 7.3): the completion command prints cobra's
// script for a shell; the script calls papierkram __complete, which
// completes commands, flags, allowed values (pk:enum, the values help and
// commands --json name), context names and IDs (completion_ids.go).
// Completion is silent: it never asks, never writes to stderr (cobra's own
// directive line aside) and every failure means no candidates.

// completionShells are the shells completion writes a script for.
var completionShells = []string{"bash", "zsh", "fish", "powershell"}

// completionLong is the help of completion; the credits are those of a
// list request (registry, spec 4.3).
func completionLong() string {
	return fmt.Sprintf(`Gibt das Skript für die Shell-Vervollständigung aus (bash, zsh, fish oder powershell).

Vervollständigt werden Befehle, Flags, erlaubte Werte (z. B. -o, --state, die %d Ausgabekategorien bei --item category=), Kontextnamen aus der config.toml und IDs. Kategorien mit Komma (z. B. "Gas, Strom, Wasser") stehen in Anführungszeichen: dafür mit category=" beginnen.

IDs (z. B. papierkram invoice describe <Tab>, --company <Tab>) lädt die Vervollständigung per Listenabfrage aus der API: erste Seite, bis zu %d Einträge, angezeigt als ID mit Name. Jede Abfrage kostet %d Credit. Das Ergebnis wird je Kontext und Ressource %d Sekunden im Cache-Verzeichnis gehalten; so lange zeigt die Vervollständigung die IDs auch ohne neue Anfrage. auth logout, context delete und context rename löschen den Cache des Kontexts. Ohne Token wird nichts geladen; bei Fehlern oder nach %d Sekunden gibt es keine IDs, aber nie eine Meldung. Das Token wird erst gelesen, wenn eine Anfrage nötig ist; bei gesperrtem Schlüsselbund (Linux) kann dabei ein Entsperrdialog erscheinen, completion_lookup = false vermeidet jeden Zugriff. Abschalten je Kontext in der config.toml:

  [contexts.<name>]
  completion_lookup = false

Dann sendet die Vervollständigung keine Anfrage und liest keinen Cache.

Jedes Skript vervollständigt auch die Kurzform pk, die Homebrew, Scoop, deb und rpm installieren; deren Pakete richten die Vervollständigung selbst ein (Scoop ausgenommen).

Einrichten (danach eine neue Shell starten):

bash (braucht das Paket bash-completion):
  aktuelle Shell:  source <(papierkram completion bash)
  dauerhaft Linux: papierkram completion bash > /etc/bash_completion.d/papierkram
  dauerhaft macOS: papierkram completion bash > $(brew --prefix)/etc/bash_completion.d/papierkram

zsh (falls noch nicht aktiv, vorher einmal: echo "autoload -U compinit; compinit" >> ~/.zshrc):
  papierkram completion zsh > "${fpath[1]}/_papierkram"

fish (lädt die Datei nach dem Befehlsnamen, daher für pk ein zweiter Name):
  papierkram completion fish > ~/.config/fish/completions/papierkram.fish
  ln -s papierkram.fish ~/.config/fish/completions/pk.fish

PowerShell:
  aktuelle Sitzung: papierkram completion powershell | Out-String | Invoke-Expression
  dauerhaft: dieselbe Zeile in das Profil ($PROFILE) eintragen

Gibt nur das Skript aus und sendet keine Anfrage.`,
		len(spec.VoucherCategories()), completionPageSize, completionCredits(), int(namecache.TTL.Seconds()), int(completionTimeout.Seconds()))
}

// completionCredits is the cost of one ID lookup: the most expensive list
// request the completion sends (registry.ExpectedCredits, spec 4.3; all
// lists cost the same).
func completionCredits() int {
	n := 0
	for _, e := range registry.Entries() {
		if e.Action == "list" {
			n = max(n, registry.ExpectedCredits(e.Method, e.Path))
		}
	}
	return n
}

func newCompletionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "completion <shell>",
		Short: "Skript für die Shell-Vervollständigung ausgeben",
		Long:  completionLong(),
		Example: "  papierkram completion zsh > \"${fpath[1]}/_papierkram\"\n" +
			"  papierkram completion fish > ~/.config/fish/completions/papierkram.fish\n" +
			"  papierkram completion bash > /etc/bash_completion.d/papierkram",
		Annotations: readsOnly(),
		ValidArgs:   completionShells,
		Args: func(c *cobra.Command, args []string) error {
			if err := exactArgs(1)(c, args); err != nil {
				return err
			}
			if !slices.Contains(completionShells, args[0]) {
				return clierr.New(clierr.CodeUsage, fmt.Sprintf("unbekannte Shell %q für %q", args[0], c.CommandPath()),
					"Erlaubt sind "+strings.Join(completionShells, ", ")+". Verwendung: "+useLine(c))
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			root := c.Root()
			var buf bytes.Buffer
			var err error
			switch args[0] {
			case "bash":
				err = root.GenBashCompletionV2(&buf, true)
			case "zsh":
				err = root.GenZshCompletion(&buf)
			case "fish":
				err = root.GenFishCompletion(&buf, true)
			default:
				err = root.GenPowerShellCompletionWithDesc(&buf)
			}
			if err != nil {
				return err
			}
			script, err := withShortForm(args[0], buf.String())
			if err != nil {
				return err
			}
			_, err = io.WriteString(app.Out, script)
			return err
		},
	}
}

// isCompletionRequest reports whether args call cobra's hidden __complete
// command (what the scripts run on <Tab>).
func isCompletionRequest(args []string) bool {
	return len(args) > 0 && (args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd)
}

// isCompletionCmd reports whether c is cobra's __complete command.
func isCompletionCmd(c *cobra.Command) bool {
	return c.Name() == cobra.ShellCompRequestCmd
}

// registerCompletions installs the completion functions on the tree of
// root. It runs only for a completion request: cobra keeps flag
// completions in a process-wide map that would hold every command tree
// built (tests build thousands).
func (a *App) registerCompletions(root *cobra.Command) {
	walk(root, func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if fn := a.flagCompletion(c, f); fn != nil {
				_ = c.RegisterFlagCompletionFunc(f.Name, fn)
			}
		})
		if c.ValidArgsFunction == nil {
			c.ValidArgsFunction = a.argsCompletion(c)
		}
	})
}

// flagCompletion is the completion of the values of f on c, nil for
// cobra's default (file names): allowed values, context names, the enum
// keys of a line item, IDs of the referenced resource.
func (a *App) flagCompletion(c *cobra.Command, f *pflag.Flag) cobra.CompletionFunc {
	if values := f.Annotations[flagEnum]; len(values) > 0 {
		return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return withPrefix(values, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
	}
	switch {
	case f.Name == "context" && f.Value.Type() == "string":
		return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return a.contextNames(toComplete), cobra.ShellCompDirectiveNoFileComp
		}
	case f.Name == "item":
		if m := metaOf(c); m.op {
			return itemCompletion(newWriteSpec(m.entry).itemSchema())
		}
		return nil
	}
	resource, isID := idFlagResource(f)
	switch {
	case isID && resource != "" && !newAccount(c):
		return func(c *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return a.completeIDs(c, resource, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
	case isID || takesNoFile(f):
		// an ID without a listable resource or account (--custom-template,
		// context create --user), a number, date or duration: nothing,
		// never file names
		return noCompletion
	}
	return nil
}

// noCompletion offers nothing and no file names.
func noCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// newAccount reports a command that sets up a context: the account its
// ID flags refer to is not known yet (context create --user).
func newAccount(c *cobra.Command) bool {
	return strings.Join(commandWords(c), " ") == "context create"
}

// takesNoFile reports a flag whose value is never a file name: every
// type but text (numbers, IDs, dates, durations). Switches take no value.
func takesNoFile(f *pflag.Flag) bool {
	typ := f.Value.Type()
	if t := f.Annotations[flagValueType]; len(t) == 1 {
		typ = t[0] // the contract type: date filters are text flags (review R2-M2)
	}
	return f.NoOptDefVal == "" && typ != "string" && typ != "stringArray"
}

// argsCompletion completes the positional arguments of a registry command:
// the first one is the ID of the command's resource, a file argument
// (voucher upload) gets file names, anything else nothing. nil for other
// commands (cobra's default).
func (a *App) argsCompletion(c *cobra.Command) cobra.CompletionFunc {
	m := metaOf(c)
	if !m.op || !strings.Contains(c.Use, "<") {
		return nil
	}
	resource := m.entry.Resource
	file := hasFileArg(m.entry)
	return func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch {
		case len(args) == 0:
			return a.completeIDs(c, resource, toComplete), cobra.ShellCompDirectiveNoFileComp
		case len(args) == 1 && file:
			return nil, cobra.ShellCompDirectiveDefault
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// hasFileArg reports whether the request of e carries a file (multipart
// upload): its second positional argument is a file name.
func hasFileArg(e registry.Entry) bool {
	fields, _ := spec.Fields(spec.RequestSchema(e.OperationID), nil)
	return slices.ContainsFunc(fields, func(f spec.Field) bool { return f.Kind == spec.KindBinary })
}

// refResources map reference flags named after their role to the resource
// they reference; every other ID flag is named after its resource
// (--company, --project, --task, --user, ...). The request schemas do not
// name the referenced resource (inline {id} objects), so the roles are
// listed here.
var refResources = map[string]string{
	"customer":            "company",
	"creditor":            "company",
	"customer-project":    "project",
	"default-proposition": "proposition",
	"team-member":         "user",
	"from-estimate":       "estimate",
}

// idFlagResource is the resource whose IDs f takes ("" when the
// reference has no list of its own); ok is false for flags that take no
// ID. An ID flag (create/update reference, list filter, preset) is named
// after a resource or a role in refResources. --task of time log and
// local init is a text flag (ID or alias) and completes the IDs as well.
func idFlagResource(f *pflag.Flag) (resource string, ok bool) {
	typ := f.Value.Type()
	if t := f.Annotations[flagValueType]; len(t) == 1 {
		typ = t[0]
	}
	if typ != "id" && typ != "int64" && (typ != "string" || f.Name != presetTask) {
		return "", false
	}
	name := f.Name
	if r, found := refResources[name]; found {
		name = r
	}
	if _, found := registry.Lookup(name, "list"); !found {
		return "", true
	}
	return name, true
}

// contextNames are the context names of config.toml starting with
// prefix; a name that is no valid context name (edited by hand) is left
// out. Errors give none.
func (a *App) contextNames(prefix string) []string {
	defer a.silence()()
	cfg, _, err := a.loadConfig()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range cfg.Names() {
		if config.ValidateName(n) == nil && strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

// contextArgCompletion completes the first argument of context
// use|delete|rename with the context names.
func (a *App) contextArgCompletion(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return a.contextNames(toComplete), cobra.ShellCompDirectiveNoFileComp
}

// silence sends the warnings of this invocation to nowhere until the
// returned function restores them: completion never writes to stderr.
func (a *App) silence() func() {
	w := a.Err
	a.Err = io.Discard
	return func() { a.Err = w }
}

// withPrefix are the values starting with prefix.
func withPrefix(values []string, prefix string) []string {
	var out []string
	for _, v := range values {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// itemCompletion completes --item for the line item schema item: within
// the pair being typed, the values of an enum key (voucher category= with
// the expense categories of the spec) and, for a started key, the enum
// keys themselves. Values that the --item grammar must quote (",", "=",
// a quote or a backslash) are quoted. Without a schema nothing is
// completed, never file names.
func itemCompletion(item *openapi3.Schema) cobra.CompletionFunc {
	enums := map[string][]string{}
	if item != nil {
		for key, p := range item.Properties {
			if p == nil || p.Value == nil {
				continue
			}
			var values []string
			for _, v := range p.Value.Enum {
				if s, ok := v.(string); ok && !slices.Contains(values, s) {
					values = append(values, s)
				}
			}
			if len(values) > 0 {
				enums[key] = values
			}
		}
	}
	keys := slices.Sorted(maps.Keys(enums))
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		start := lastItemPair(toComplete)
		pair := strings.TrimLeft(toComplete[start:], " ")
		prefix := toComplete[:len(toComplete)-len(pair)]
		key, _, typed := strings.Cut(pair, "=")
		var out []string
		if !typed {
			for _, k := range keys {
				if strings.HasPrefix(k, pair) {
					out = append(out, prefix+k+"=")
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
		}
		// the typed value matches with or without its opening quote; the
		// candidate is the form the grammar needs (review M4)
		_, typedValue, _ := strings.Cut(pair, "=")
		typedValue = strings.TrimPrefix(strings.TrimLeft(typedValue, " "), `"`)
		for _, v := range enums[strings.TrimSpace(key)] {
			if strings.HasPrefix(v, typedValue) {
				out = append(out, prefix+key+"="+itemQuote(v))
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

// lastItemPair is the byte offset where the last pair of an --item value
// starts: after the last "," outside quotes (splitItem's grammar).
func lastItemPair(s string) int {
	start, quoted, escaped := 0, false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case escaped:
			escaped = false
		case quoted && c == '\\':
			escaped = true
		case c == '"':
			quoted = !quoted
		case !quoted && c == ',':
			start = i + 1
		}
	}
	return start
}

// itemQuote writes v as an --item value: in quotes, with \" and \\, when
// it holds a character the grammar would split at or trim.
func itemQuote(v string) string {
	if !strings.ContainsAny(v, `,="\`) && strings.TrimSpace(v) == v {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

// schemaArgCompletion completes schema <ressource> [<aktion>]: the
// resource names, then the words of its actions (schemaActions, the
// actions schema accepts; "voucher document delete" word by word).
func schemaArgCompletion(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		var names []string
		for _, r := range registry.Resources() {
			names = append(names, r.Name)
		}
		return withPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	res, ok := findResource(args[0])
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	group, _, err := c.Root().Find([]string{res.Name})
	if err != nil || group == c.Root() {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	typed := args[1:]
	var out []string
	for _, name := range actionNames(group, schemaActions(group)) {
		words := strings.Fields(name)
		if len(words) > len(typed) && slices.Equal(words[:len(typed)], typed) &&
			strings.HasPrefix(words[len(typed)], toComplete) && !slices.Contains(out, words[len(typed)]) {
			out = append(out, words[len(typed)])
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// apiArgCompletion completes the method of papierkram api; the path gets
// nothing (no file names either).
func apiArgCompletion(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// in the case the user started typing (api accepts both), so the
		// shell's own prefix filter keeps them (review R2-M6)
		out := withPrefix(apiMethods, strings.ToUpper(toComplete))
		if toComplete != "" && toComplete == strings.ToLower(toComplete) {
			for i := range out {
				out[i] = strings.ToLower(out[i])
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// shortForm is the short command name the packages install next to
// papierkram (deb/rpm link, Homebrew binary target, Scoop shim; README
// "Kurzform pk").
const shortForm = "pk"

// withShortForm adds the short form pk to cobra's script for shell: the
// same registration as papierkram, nothing else is needed. bash, zsh and
// PowerShell run the program named on the command line ("pk __complete");
// fish completes pk through "complete -w papierkram" and runs
// "papierkram __complete" (both names are the same binary). For zsh the two
// header lines name both commands: compinit binds the #compdef names when
// the file is found in fpath, the compdef line when it is sourced, and an
// appended compdef line would only run inside the autoloaded function.
// Every change first checks the line of cobra's script it builds on; a
// cobra with other names is an error, never a script without pk.
func withShortForm(shell, script string) (string, error) {
	lines := strings.Split(script, "\n")
	has := func(line string) bool { return slices.Contains(lines, line) }
	var anchors []string
	var extra string
	switch shell {
	case "bash":
		anchors = []string{
			"    complete -o default -F __start_papierkram papierkram",
			"    complete -o default -o nospace -F __start_papierkram papierkram",
		}
		extra = "\n# short form " + shortForm + "\n" +
			"if [[ $(type -t compopt) = \"builtin\" ]]; then\n" +
			"    complete -o default -F __start_papierkram " + shortForm + "\n" +
			"else\n" +
			"    complete -o default -o nospace -F __start_papierkram " + shortForm + "\n" +
			"fi\n"
	case "zsh":
		head := []string{"#compdef papierkram", "compdef _papierkram papierkram"}
		if len(lines) < len(head) || !slices.Equal(lines[:len(head)], head) {
			return "", shortFormErr(shell, head[0])
		}
		lines[0] += " " + shortForm
		lines[1] += " " + shortForm
		return strings.Join(lines, "\n"), nil
	case "fish":
		anchors = []string{"complete -c papierkram -e"}
		extra = "\n# short form " + shortForm + "\ncomplete -c " + shortForm + " -w papierkram\n"
	case "powershell":
		reg := "Register-ArgumentCompleter -CommandName '%s' -ScriptBlock ${__papierkramCompleterBlock}"
		anchors = []string{fmt.Sprintf(reg, "papierkram")}
		extra = "\n# short form " + shortForm + "\n" + fmt.Sprintf(reg, shortForm) + "\n"
	default:
		return "", shortFormErr(shell, "")
	}
	for _, a := range anchors {
		if !has(a) {
			return "", shortFormErr(shell, a)
		}
	}
	return script + extra, nil
}

// shortFormErr is the (internal) error of a cobra script that lacks line.
func shortFormErr(shell, line string) error {
	return clierr.New(clierr.CodeError, fmt.Sprintf("Vervollständigung für %s: erwartete Zeile fehlt: %q", shell, line),
		"Interner Fehler: das Skript von cobra hat sich geändert; bitte melden")
}
