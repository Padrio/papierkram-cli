package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
)

// The root PersistentPreRunE (flag checks, central config write guard) must
// run even when a subcommand defines its own persistent hooks.
func init() { cobra.EnableTraverseRunHooks = true }

// usageErr is a usage error (exit 2) without its own hint: executeContext
// adds the usage line of the command that failed (usageHint).
func usageErr(msg string) error { return clierr.New(clierr.CodeUsage, msg, "") }

// usageHint is the hint of a flag or argument error of c.
func usageHint(c *cobra.Command) string { return "Verwendung: " + useLine(c) }

// unknownCommand is the usage error of an unknown subcommand name of
// group, with the help of group as hint.
func unknownCommand(group *cobra.Command, name string) error {
	return clierr.New(clierr.CodeUsage, fmt.Sprintf("unbekannter Befehl %q für %q", name, group.CommandPath()),
		"Befehle anzeigen: "+group.CommandPath()+" --help")
}

// noArgs rejects positional arguments with a German usage error.
func noArgs(c *cobra.Command, args []string) error {
	if len(args) > 0 {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("unerwartetes Argument %q für %q", args[0], c.CommandPath()), usageHint(c))
	}
	return nil
}

// exactArgs requires exactly n positional arguments with a German usage error.
func exactArgs(n int) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("zu wenige Argumente für %q (erwartet: %d, erhalten: %d)", c.CommandPath(), n, len(args)), usageHint(c))
		case len(args) > n:
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("unerwartetes Argument %q für %q", args[n], c.CommandPath()), usageHint(c))
		}
		return nil
	}
}

// minArgs requires at least n positional arguments with a German usage
// error.
func minArgs(n int) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if len(args) < n {
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("zu wenige Argumente für %q (erwartet: mindestens %d, erhalten: %d)", c.CommandPath(), n, len(args)), usageHint(c))
		}
		return nil
	}
}

var (
	invalidValueRe = regexp.MustCompile(`^invalid argument (".*?") for "(-[^"]+)" flag: (.*)$`)
	strconvPrefix  = regexp.MustCompile(`strconv\.\w+: parsing "[^"]*": `)
	// time.ParseDuration: `time: unknown unit "x" in duration "1x"` and
	// `time: missing unit in duration "30"`.
	unknownUnitRe = regexp.MustCompile(`time: unknown unit (".*?") in duration (".*")`)
	missingUnitRe = regexp.MustCompile(`time: missing unit in duration (".*")`)
	englishCauses = strings.NewReplacer(
		"time: invalid duration", "ungültige Dauer",
		"invalid syntax", "ungültiges Format",
		"value out of range", "Wert außerhalb des Bereichs",
	)
)

// expectedValue names what a flag of pflag type typ takes, for a value
// strconv rejects ("invalid syntax"); "" for other types.
func expectedValue(typ string) string {
	switch {
	case typ == "bool":
		return "Wahrheitswert erwartet (true oder false)"
	case strings.HasPrefix(typ, "int") || strings.HasPrefix(typ, "uint"):
		return "ganze Zahl erwartet"
	case strings.HasPrefix(typ, "float"):
		return "Zahl erwartet (z. B. 12.5)"
	}
	return ""
}

// germanCause translates the cause part of a pflag "invalid argument"
// error of a flag of pflag type typ.
func germanCause(s, typ string) string {
	if strings.HasSuffix(s, ": invalid syntax") && expectedValue(typ) != "" {
		return expectedValue(typ)
	}
	s = strconvPrefix.ReplaceAllString(s, "")
	s = unknownUnitRe.ReplaceAllString(s, "ungültige Dauer $2: unbekannte Einheit $1 (erlaubt: ms, s, m, h)")
	s = missingUnitRe.ReplaceAllString(s, "ungültige Dauer $1: Einheit fehlt (z. B. 30s)")
	return englishCauses.Replace(s)
}

// flagTypeOf is the pflag type of the flag named in a pflag error
// ("--page" or "-p, --page"), "" when c has no such flag.
func flagTypeOf(c *cobra.Command, named string) string {
	if c == nil {
		return ""
	}
	if f := c.Flags().Lookup(named[strings.LastIndex(named, "--")+2:]); f != nil {
		return f.Value.Type()
	}
	return ""
}

// germanFlagError translates pflag error messages into German usage errors
// with the usage line of c as hint.
func germanFlagError(c *cobra.Command, err error) error {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown flag: "):
		msg = "unbekanntes Flag: " + strings.TrimPrefix(msg, "unknown flag: ")
	case strings.HasPrefix(msg, "unknown shorthand flag: "):
		msg = "unbekanntes Kurz-Flag: " + strings.TrimPrefix(msg, "unknown shorthand flag: ")
	case strings.HasPrefix(msg, "flag needs an argument: "):
		msg = "Flag benötigt einen Wert: " + strings.TrimPrefix(msg, "flag needs an argument: ")
	case strings.HasPrefix(msg, "bad flag syntax: "):
		msg = "ungültige Flag-Syntax: " + strings.TrimPrefix(msg, "bad flag syntax: ")
	case invalidValueRe.MatchString(msg):
		m := invalidValueRe.FindStringSubmatch(msg)
		msg = fmt.Sprintf("ungültiger Wert %s für %s: %s", m[1], m[2], germanCause(m[3], flagTypeOf(c, m[2])))
	}
	return clierr.New(clierr.CodeUsage, msg, usageHint(c))
}

// outputFormats are the values accepted by -o/--output (spec 10.3).
var outputFormats = []string{"table", "wide", "json", "yaml"}

// checkGlobalFlags validates the global flags and records which of them were
// set explicitly (context values only apply to unset flags).
func (a *App) checkGlobalFlags(c *cobra.Command) error {
	fl := &a.Flags
	fl.OutputSet = c.Flags().Changed("output")
	fl.TimeoutSet = c.Flags().Changed("timeout")
	if fl.Timeout <= 0 {
		return usageErr(fmt.Sprintf("ungültiger Wert %q für --timeout: positive Dauer erwartet (z. B. 30s)", c.Flags().Lookup("timeout").Value.String()))
	}
	if !slices.Contains(outputFormats, fl.Output) {
		return usageErr(fmt.Sprintf("ungültiger Wert %q für -o, --output: erlaubt sind %s", fl.Output, strings.Join(outputFormats, ", ")))
	}
	// --jq is compiled here, before any request (plan: flags first). --fields
	// cannot fail: unknown paths are left out of the output.
	if fl.JQ != "" {
		return output.CheckJQ(fl.JQ)
	}
	return nil
}

// NewRootCmd builds the root command with all global flags.
func NewRootCmd(app *App) *cobra.Command {
	// a new invocation resolves configuration and project file anew and
	// reads its own stdin (App.In may be replaced between invocations)
	app.pfile = projectFileCache{}
	app.stdinBuf = nil
	app.presetHints = nil
	app.resolved = nil
	root := &cobra.Command{
		Use:           "papierkram",
		Short:         "Kommandozeile für die Papierkram-API",
		Long:          "papierkram steuert Papierkram (Buchhaltung) von der Kommandozeile aus.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommand(c, args[0])
			}
			return nil
		},
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			if isCompletionCmd(c) {
				// the completion request parses the flags of the completed
				// command itself and stays silent (no warnings)
				return nil
			}
			if err := app.checkGlobalFlags(c); err != nil {
				return err
			}
			if err := app.checkDryRun(c); err != nil {
				return err
			}
			app.warnReadOnlyEnv()
			if c.Annotations[annotWritesConfig] == "true" {
				return app.guardConfigWrite()
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(germanFlagError)
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	root.SetIn(app.In)

	f := root.PersistentFlags()
	fl := &app.Flags
	f.StringVar(&fl.Context, "context", "", "Kontext statt des aktiven verwenden")
	f.StringVarP(&fl.Output, "output", "o", "table", "Ausgabeformat: table|wide|json|yaml")
	f.StringVar(&fl.Fields, "fields", "", "Felder der Ausgabe, kommagetrennt")
	f.StringVar(&fl.JQ, "jq", "", "jq-Ausdruck auf die Ausgabe anwenden")
	f.BoolVar(&fl.Debug, "debug", false, "HTTP-Debug-Ausgabe auf stderr (ohne Token)")
	f.DurationVar(&fl.Timeout, "timeout", 30*time.Second, "Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt")
	f.BoolVar(&fl.NoInput, "no-input", false, "keine interaktiven Rückfragen")
	f.BoolVarP(&fl.Yes, "yes", "y", false, "Bestätigungen automatisch bejahen")
	f.BoolVar(&fl.DryRun, "dry-run", false, "Mutation nur anzeigen, nicht senden")
	f.BoolVar(&fl.NoColor, "no-color", false, "Farben abschalten")
	markEnum(f, "output", outputFormats)
	// the order of config.SelectContext: flag > PAPIERKRAM_CONTEXT >
	// project file > active_context
	annotate(f, "context", flagEnv, "PAPIERKRAM_CONTEXT")
	annotate(f, "context", flagDefaultFrom, config.SourceFlag, config.SourceEnv, config.SourceProjectFile, config.SourceConfig)

	root.AddCommand(newVersionCmd(app))
	root.AddCommand(newContextCmd(app))
	root.AddCommand(newAuthCmd(app))
	root.AddCommand(newLocalCmd(app))
	root.AddCommand(newAPICmd(app))
	root.AddCommand(newCommandsCmd(app))
	root.AddCommand(newSchemaCmd(app))
	root.AddCommand(newCompletionCmd(app))
	root.AddCommand(newGenDocsCmd(app))
	BuildResourceCommands(app, root)
	applyGerman(root)
	return root
}

// Run executes the CLI with the given arguments and returns the exit code.
func Run(app *App, args []string) int {
	return execute(app, NewRootCmd(app), args)
}

// interruptSignals cancel the command context: Ctrl+C, and SIGTERM where
// the platform delivers it.
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// execute runs root with a context that is cancelled on an interrupt, so a
// running request is aborted, and maps the resulting error to message and
// exit code.
func execute(app *App, root *cobra.Command, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals...)
	defer stop()
	// After the first signal the default behaviour returns: a second
	// Ctrl+C ends the process even if something still blocks.
	context.AfterFunc(ctx, stop)
	return executeContext(ctx, app, root, args)
}

// executeContext runs root with ctx as the command context (cmd.Context()).
func executeContext(ctx context.Context, app *App, root *cobra.Command, args []string) int {
	if isCompletionRequest(args) {
		app.registerCompletions(root)
	}
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	err = completeArgsErr(cmd, err)
	ce := clierr.From(err)
	if ce.Code == clierr.CodeUsage && ce.Hint == "" && cmd != nil {
		// every usage error names the way out (task 17): the usage line
		// of the command whose flags or arguments were wrong
		withHint := *ce
		withHint.Hint = usageHint(cmd)
		ce = &withHint
	}
	if ce.Code == clierr.CodeNetwork && ctx.Err() != nil && errors.Is(err, context.Canceled) {
		// The request did not fail, it was interrupted (Ctrl+C).
		ce = &clierr.Error{Code: ce.Code, Message: "Abgebrochen: Anfrage durch Unterbrechung beendet", Cause: err}
	}
	// The format of the error output is the resolved one (context output
	// value) when the configuration was resolved, otherwise the flag.
	output.WriteError(app.outputOptions(context.Background(), app.outputFormat(app.resolved)), ce)
	return ce.Exit()
}

// completeArgsErr is err of cmd, but for cobra's own __complete command
// (also called as __completeNoDesc) the German argument error of
// minArgs(1) instead of an untyped one. ExecuteC creates that command
// itself, with cobra's validator for at least one argument, right before
// it runs it, so no code of ours can set its Args before the check
// (review T19a-M1). Its flags are not parsed, its Run returns nothing and
// the root's PersistentPreRunE returns nil for it: an untyped error of it
// is that check, which fails only without arguments.
func completeArgsErr(cmd *cobra.Command, err error) error {
	var ce *clierr.Error
	if cmd == nil || !isCompletionCmd(cmd) || errors.As(err, &ce) {
		return err
	}
	if verr := minArgs(1)(cmd, nil); verr != nil {
		// its use line is cobra's English one ([command-line])
		ce := *clierr.From(verr)
		ce.Hint = "Interner Befehl der Shell-Vervollständigung; einrichten mit papierkram completion <shell>"
		return &ce
	}
	return err
}

// Main is the process entry point and returns the exit code.
func Main() int {
	return Run(NewApp(), os.Args[1:])
}
