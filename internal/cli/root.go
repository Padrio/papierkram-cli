package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

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
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("zu wenige Argumente für %q (erwartet: %d, erhalten: %d)", c.CommandPath(), n, len(args)), "Verwendung: "+c.UseLine())
		case len(args) > n:
			return clierr.New(clierr.CodeUsage, fmt.Sprintf("unerwartetes Argument %q für %q", args[n], c.CommandPath()), "Verwendung: "+c.UseLine())
		}
		return nil
	}
}

var (
	invalidValueRe = regexp.MustCompile(`^invalid argument (".*?") for "(-[^"]+)" flag: (.*)$`)
	strconvPrefix  = regexp.MustCompile(`strconv\.\w+: parsing "[^"]*": `)
	englishCauses  = strings.NewReplacer(
		"time: invalid duration", "ungültige Dauer",
		"invalid syntax", "ungültige Syntax",
		"value out of range", "Wert außerhalb des Bereichs",
	)
)

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
	case invalidValueRe.MatchString(msg):
		m := invalidValueRe.FindStringSubmatch(msg)
		msg = fmt.Sprintf("ungültiger Wert %s für %s: %s", m[1], m[2], englishCauses.Replace(strconvPrefix.ReplaceAllString(m[3], "")))
	}
	return usageErr(msg)
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
		PersistentPreRunE: func(*cobra.Command, []string) error {
			app.warnReadOnlyEnv()
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
	f.DurationVar(&fl.Timeout, "timeout", 30*time.Second, "Zeitlimit je Anfrage")
	f.BoolVar(&fl.NoInput, "no-input", false, "keine interaktiven Rückfragen")
	f.BoolVarP(&fl.Yes, "yes", "y", false, "Bestätigungen automatisch bejahen")
	f.BoolVar(&fl.DryRun, "dry-run", false, "Mutation nur anzeigen, nicht senden")
	f.BoolVar(&fl.NoColor, "no-color", false, "Farben abschalten")

	root.AddCommand(newVersionCmd(app))
	root.AddCommand(newContextCmd(app))
	applyGerman(root)
	return root
}

// Run executes the CLI with the given arguments and returns the exit code.
func Run(app *App, args []string) int {
	return execute(app, NewRootCmd(app), args)
}

// execute runs root and maps the resulting error to message and exit code.
func execute(app *App, root *cobra.Command, args []string) int {
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	ce := clierr.From(err)
	_, _ = io.WriteString(app.Err, ce.Format())
	return ce.Exit()
}

// Main is the process entry point and returns the exit code.
func Main() int {
	return Run(NewApp(), os.Args[1:])
}
