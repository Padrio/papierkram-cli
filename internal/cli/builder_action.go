package cli

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// deletedJSON and okJSON are the JSON contract of delete and of actions
// whose API response is empty (spec 10.3).
type deletedJSON struct {
	ID      int64 `json:"id"`
	Deleted bool  `json:"deleted"`
}

type okJSON struct {
	ID int64 `json:"id"`
	OK bool  `json:"ok"`
}

// runDescribe sends a GET for one object (describe, info) and renders it.
// Without ID a describe entry with a preset takes it from there (spec 9.8).
func (a *App) runDescribe(c *cobra.Command, e registry.Entry, args []string) error {
	if err := checkNameFlags(c); err != nil {
		return err
	}
	args, err := a.presetArgs(c, e, args)
	if err != nil {
		return err
	}
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	client, r, err := a.Client(c)
	if err != nil {
		return err
	}
	if _, err := a.resolvePathNames(c, e, client, r, params); err != nil {
		return err
	}
	resp, err := client.Call(c.Context(), e.OperationID, params, nil, nil)
	if err != nil {
		return err
	}
	return a.render(c, resp.Body, e.Columns)
}

// runSimpleAction sends delete, archive, unarchive, cancel or reverse for
// one object. Order: arguments and a preset path flag (exit 2),
// configuration and read-only gate (mutationClient, exit 9), a
// confirmation that cannot be asked (exit 8), names of the path flags
// (resolveNames), the preset hint, --dry-run and confirmation
// (beforeSend), then the request.
// delete prints {"id":N,"deleted":true}; another action prints
// the API object, or {"id":N,"ok":true} when the body is empty (R03: an
// empty 2xx body is never an error). Without --fields, table and wide show
// a German sentence instead.
func (a *App) runSimpleAction(c *cobra.Command, e registry.Entry, args []string) error {
	if err := checkNameFlags(c); err != nil {
		return err
	}
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	// ruling T3-D: a confirmation that cannot be asked (no terminal,
	// --no-input) fails before the list requests of the resolution; a
	// prompt needs the resolved ID and comes after it (beforeSend)
	if !a.Flags.DryRun {
		if _, err := a.mustAsk(e, Summary{Resource: resourceNoun(e.Resource), ID: lastID(params)}); err != nil {
			a.flushPresetHints() // review T9-M1: the preset hint names the company first
			return err
		}
	}
	res, err := a.resolvePathNames(c, e, client, r, params)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	if proceed, err := a.beforeSend(c, e, client, params, nil, sendExtras{resolved: res}); !proceed || err != nil {
		return err
	}
	resp, err := client.Call(c.Context(), e.OperationID, params, nil, nil)
	if err != nil {
		return err
	}
	id := lastID(params)
	var result any
	switch {
	case e.Action == "delete":
		result = deletedJSON{ID: id, Deleted: true}
	case len(bytes.TrimSpace(resp.Body)) == 0:
		result = okJSON{ID: id, OK: true}
	default:
		return a.render(c, resp.Body, e.Columns)
	}
	return a.emptyOK(c, r, result, fmt.Sprintf("%s %d %s.", e.Resource, id, simpleActions[e.Action]))
}

// resolvePathNames resolves the names of the flags (resolveNames) and sets
// the path values of the path flags given anew (patchPathParams).
func (a *App) resolvePathNames(c *cobra.Command, e registry.Entry, client *api.Client, r *config.Resolved, params []int64) ([]Resolution, error) {
	res, err := a.resolveNames(c, client, r)
	if err != nil {
		return nil, err
	}
	return res, patchPathParams(c, e, params)
}
