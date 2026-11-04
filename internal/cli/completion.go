package cli

import (
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

Vervollständigt werden Befehle, Flags, erlaubte Werte (z. B. -o, --state, die %d Ausgabekategorien bei --item category=), Kontextnamen aus der config.toml und IDs.

IDs (z. B. papierkram invoice describe <Tab>, --company <Tab>) lädt die Vervollständigung per Listenabfrage aus der API: erste Seite, %d Einträge, angezeigt als ID mit Name. Jede Abfrage kostet %d Credit. Das Ergebnis wird je Kontext und Ressource %d Sekunden im Cache-Verzeichnis gehalten. Ohne Token, bei Fehlern oder nach %d Sekunden gibt es keine IDs, aber nie eine Meldung. Abschalten je Kontext in der config.toml:

  [contexts.<name>]
  completion_lookup = false

Dann sendet die Vervollständigung keine Anfrage und liest keinen Cache.

Einrichten (danach eine neue Shell starten):

bash (braucht das Paket bash-completion):
  aktuelle Shell:  source <(papierkram completion bash)
  dauerhaft Linux: papierkram completion bash > /etc/bash_completion.d/papierkram
  dauerhaft macOS: papierkram completion bash > $(brew --prefix)/etc/bash_completion.d/papierkram

zsh (falls noch nicht aktiv, vorher einmal: echo "autoload -U compinit; compinit" >> ~/.zshrc):
  papierkram completion zsh > "${fpath[1]}/_papierkram"

fish:
  papierkram completion fish > ~/.config/fish/completions/papierkram.fish

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
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(app.Out, true)
			case "zsh":
				return root.GenZshCompletion(app.Out)
			case "fish":
				return root.GenFishCompletion(app.Out, true)
			default:
				return root.GenPowerShellCompletionWithDesc(app.Out)
			}
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
	if resource, ok := idFlagResource(f); ok {
		// an ID without a listable resource (--custom-template) completes
		// nothing, never file names
		return func(c *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return a.completeIDs(c, resource, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
	}
	return nil
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
		for _, v := range enums[strings.TrimSpace(key)] {
			if cand := prefix + key + "=" + itemQuote(v); strings.HasPrefix(cand, toComplete) {
				out = append(out, cand)
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
