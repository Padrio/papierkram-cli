package cli

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// emptyOK prints the result of a sent request that has no API object to
// show: an empty 2xx answer ({"id":N,"ok":true}, create's {"ok":true}),
// delete ({"id":N,"deleted":true}) or a deliver whose readback failed.
// In the effective output format of r (outputFormat) result is rendered;
// table and wide without --fields (ownText) print text as one line
// instead. A warning belongs on stderr before the call. Every error goes
// through afterRequest: the request was sent.
func (a *App) emptyOK(c *cobra.Command, r *config.Resolved, result any, text string) error {
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if ownText(o) {
		_, err := fmt.Fprintln(a.Out, text)
		return afterRequest(err)
	}
	return afterRequest(output.RenderValue(o, result, output.Columns{}))
}

// prunedInput reads --input (readInput) and prunes it against the request
// schema req with the response schema resp (spec.Prune; nil makes every
// field outside the request unknown, exit 5). Errors of Prune get the
// prefix "--input: "; the hints of Prune go to stderr (pruneHints).
// given reports whether --input was set; without it nothing is read.
// What follows (flags, coercion, checks, validation) is up to each
// caller. buildBodyFrom keeps its own --input step: it drops line_items
// before Prune and checks null positions before the hints are printed.
func (a *App) prunedInput(c *cobra.Command, req, resp *openapi3.Schema) (pr spec.PruneResult, given bool, err error) {
	if !c.Flags().Changed("input") {
		return spec.PruneResult{}, false, nil
	}
	in, err := a.readInput(flagString(c, "input"))
	if err != nil {
		return spec.PruneResult{}, true, err
	}
	pr, err = spec.Prune(in, req, resp)
	if err != nil {
		return spec.PruneResult{}, true, prefixErr("--input: ", err)
	}
	a.pruneHints(pr)
	return pr, true, nil
}
