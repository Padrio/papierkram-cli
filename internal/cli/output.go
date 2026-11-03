package cli

import (
	"context"

	"github.com/Padrio/papierkram-cli/internal/output"
)

// outputOptions builds the rendering options for the given format from the
// global flags (--fields, --jq, --no-color) and the environment (NO_COLOR);
// ctx (the command context) cancels a running --jq expression.
func (a *App) outputOptions(ctx context.Context, format string) output.Options {
	return output.Options{
		Context: ctx,
		Format:  output.Format(format),
		Fields:  output.ParseFields(a.Flags.Fields),
		JQ:      a.Flags.JQ,
		Color:   output.UseColor(a.StdoutTTY, a.Env, a.Flags.NoColor),
		Out:     a.Out,
		Err:     a.Err,
	}
}

// ownText reports whether a command prints its own human-readable text
// (version, context active, auth status/login): -o table or wide without
// --fields. Structured formats and --fields go through output.Render, so
// --fields works in every format (table/wide: one FELD/WERT row per field).
func ownText(o output.Options) bool {
	return !o.Structured() && len(o.Fields) == 0
}
