package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// usageError marks errors caused by wrong flags or arguments (exit code 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func unknownCommand(root *cobra.Command, name string) error {
	return &usageError{fmt.Sprintf("unbekannter Befehl %q für %q", name, root.CommandPath())}
}

// germanFlagError translates pflag error messages.
func germanFlagError(_ *cobra.Command, err error) error {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown flag: "):
		msg = "unbekanntes Flag: " + strings.TrimPrefix(msg, "unknown flag: ")
	case strings.HasPrefix(msg, "unknown shorthand flag: "):
		msg = "unbekanntes Kurz-Flag: " + strings.TrimPrefix(msg, "unknown shorthand flag: ")
	case strings.HasPrefix(msg, "flag needs an argument: "):
		msg = "Flag benötigt einen Wert: " + strings.TrimPrefix(msg, "flag needs an argument: ")
	case strings.HasPrefix(msg, "invalid argument "):
		msg = "ungültiger Wert " + strings.TrimPrefix(msg, "invalid argument ")
	}
	return &usageError{msg}
}

// NewRootCmd builds the root command with all global flags.
func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "papierkram",
		Short:         "Kommandozeile für die Papierkram-API",
		Long:          "papierkram steuert Papierkram (Buchhaltung) von der Kommandozeile aus.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommand(c, args[0])
			}
			return c.Help()
		},
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
	applyGerman(root)
	return root
}

// Run executes the CLI with the given arguments and returns the exit code.
func Run(app *App, args []string) int {
	root := NewRootCmd(app)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "unknown command ") {
		msg = "unbekannter Befehl" + strings.Replace(strings.TrimPrefix(msg, "unknown command"), "\" for \"", "\" für \"", 1)
		err = &usageError{msg}
	}
	_, _ = fmt.Fprintf(app.Err, "Fehler: %s\n", msg)
	var ue *usageError
	if errors.As(err, &ue) {
		return 2
	}
	return 1
}

// Main is the process entry point and returns the exit code.
func Main() int {
	return Run(NewApp(), os.Args[1:])
}
