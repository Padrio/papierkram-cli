package cli

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

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
	args, err := a.presetArgs(c, e, args)
	if err != nil {
		return err
	}
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	client, _, err := a.Client(c)
	if err != nil {
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
// configuration and read-only gate (mutationClient, exit 9), the preset
// hint, --dry-run and confirmation (beforeMutation), then the request.
// delete prints {"id":N,"deleted":true}; another action prints
// the API object, or {"id":N,"ok":true} when the body is empty (R03: an
// empty 2xx body is never an error). Without --fields, table and wide show
// a German sentence instead.
func (a *App) runSimpleAction(c *cobra.Command, e registry.Entry, args []string) error {
	params, err := a.pathValues(c, e, args)
	if err != nil {
		return err
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	if proceed, err := a.beforeMutation(c, e, client, params, nil); !proceed || err != nil {
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
