package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/buildinfo"
	"github.com/Padrio/papierkram-cli/internal/output"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Version, Commit und Build-Datum anzeigen",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if o := app.outputOptions(app.Flags.Output); o.Structured() {
				return output.RenderValue(o, map[string]string{
					"version":    buildinfo.Version,
					"commit":     buildinfo.Commit,
					"date":       buildinfo.Date,
					"go_version": runtime.Version(),
				}, output.Columns{})
			}
			_, err := fmt.Fprintf(app.Out, "papierkram %s (Commit %s, Build %s, %s)\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date, runtime.Version())
			return err
		},
	}
}
