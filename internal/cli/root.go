package cli

import (
	"context"
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
	"github.com/Padrio/papierkram-cli/internal/output"
)

// The root PersistentPreRunE (flag checks, central config write guard) must
// run even when a subcommand defines its own persistent hooks.
func init() { cobra.EnableTraverseRunHooks = true }

func usageErr(msg string) error { return clierr.New(clierr.CodeUsage, msg, "") }

func unknownCommand(root *cobra.Command, name string) error {
	return usageErr(fmt.Sprintf("unbekannter Befehl %q für %q", name, root.CommandPath()))
}

// noArgs rejects positional arguments with a German usage error.
func noArgs(c *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageErr(fmt.Sprintf("unerwartetes Argument %q für %q", args[0], c.CommandPath()))
	}
	return nil
}

// exactArgs requires exactly n positional arguments with a German usage error.
func exactArgs(n int) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("zu wenige Argumente für %q (erwartet: %d, erhalten: %d)", c.CommandPath(), n, len(args)), "Verwendung: "+useLine(c))
		case len(args) > n:
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("unerwartetes Argument %q für %q", args[n], c.CommandPath()), "Verwendung: "+useLine(c))
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
		"invalid syntax", "ungültige Syntax",
		"value out of range", "Wert außerhalb des Bereichs",
	)
)

// germanCause translates the cause part of a pflag "invalid argument" error.
func germanCause(s string) string {
	s = strconvPrefix.ReplaceAllString(s, "")
	s = unknownUnitRe.ReplaceAllString(s, "ungültige Dauer $2: unbekannte Einheit $1 (erlaubt: ms, s, m, h)")
	s = missingUnitRe.ReplaceAllString(s, "ungültige Dauer $1: Einheit fehlt (z. B. 30s)")
	return englishCauses.Replace(s)
}

// germanFlagError translates pflag error messages into German usage errors.
func germanFlagError(_ *cobra.Command, err error) error {
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
		msg = fmt.Sprintf("ungültiger Wert %s für %s: %s", m[1], m[2], germanCause(m[3]))
	}
	return usageErr(msg)
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
			if err := app.checkGlobalFlags(c); err != nil {
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
	f.DurationVar(&fl.Timeout, "timeout", 30*time.Second, "Zeitlimit je Anfrage, pro Versuch; GET wird bis zu zweimal wiederholt")
	f.BoolVar(&fl.NoInput, "no-input", false, "keine interaktiven Rückfragen")
	f.BoolVarP(&fl.Yes, "yes", "y", false, "Bestätigungen automatisch bejahen")
	f.BoolVar(&fl.DryRun, "dry-run", false, "Mutation nur anzeigen, nicht senden")
	f.BoolVar(&fl.NoColor, "no-color", false, "Farben abschalten")

	root.AddCommand(newVersionCmd(app))
	root.AddCommand(newContextCmd(app))
	root.AddCommand(newAuthCmd(app))
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
	return executeContext(ctx, app, root, args)
}

// executeContext runs root with ctx as the command context (cmd.Context()).
func executeContext(ctx context.Context, app *App, root *cobra.Command, args []string) int {
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	ce := clierr.From(err)
	if ctx.Err() != nil && ce.Code == clierr.CodeNetwork {
		// The request did not fail, it was interrupted (Ctrl+C).
		ce = &clierr.Error{Code: ce.Code, Message: "Abgebrochen: Anfrage durch Unterbrechung beendet", Cause: err}
	}
	// The format of the error output is the resolved one (context output
	// value) when the configuration was resolved, otherwise the flag.
	output.WriteError(app.outputOptions(context.Background(), app.outputFormat(app.resolved)), ce)
	return ce.Exit()
}

// Main is the process entry point and returns the exit code.
func Main() int {
	return Run(NewApp(), os.Args[1:])
}
