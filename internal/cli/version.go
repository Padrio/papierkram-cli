package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/buildinfo"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Version, Commit und Build-Datum anzeigen",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if app.Flags.Output == "json" {
				enc := json.NewEncoder(app.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{
					"version":    buildinfo.Version,
					"commit":     buildinfo.Commit,
					"date":       buildinfo.Date,
					"go_version": runtime.Version(),
				})
			}
			_, err := fmt.Fprintf(app.Out, "papierkram %s (Commit %s, Build %s, %s)\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date, runtime.Version())
			return err
		},
	}
}
