package cli

import (
	"github.com/Padrio/papierkram-cli/internal/output"
)

// outputOptions builds the rendering options for the given format from the
// global flags (--fields, --jq, --no-color) and the environment (NO_COLOR).
func (a *App) outputOptions(format string) output.Options {
	return output.Options{
		Format: output.Format(format),
		Fields: output.ParseFields(a.Flags.Fields),
		JQ:     a.Flags.JQ,
		Color:  output.UseColor(a.StdoutTTY, a.Env, a.Flags.NoColor),
		Out:    a.Out,
		Err:    a.Err,
	}
}
