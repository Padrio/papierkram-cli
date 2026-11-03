package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// filteredList is the envelope of a list with a local filter (spec 9.7):
// the API envelope fields, recomputed for the filtered entries. page and
// page_size are --page and --limit, total_entries the number of matching
// entries, total_pages the pages of page_size they fill, has_more whether
// a later page or, with --max-pages, a page not loaded may hold more. With
// --all, entries are all matching entries (page 1).
type filteredList struct {
	Type         json.RawMessage   `json:"type"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	TotalPages   int               `json:"total_pages"`
	TotalEntries int               `json:"total_entries"`
	HasMore      bool              `json:"has_more"`
	Entries      []json.RawMessage `json:"entries"`
}

// runLocalFilter loads every page like --all (warning from warnPages on,
// prompt in a terminal, --max-pages), keeps the entries that match all
// local filters and applies --page/--limit to the result.
func (a *App) runLocalFilter(c *cobra.Command, e registry.Entry, client *api.Client, params []int64, lr *listRequest) error {
	merged, err := a.fetchAll(c, e, client, params, lr)
	if err != nil {
		return err
	}
	matched := []json.RawMessage{}
	for _, raw := range merged.Entries {
		ok, err := matchesLocal(raw, lr.local)
		if err != nil {
			return unexpectedList(err)
		}
		if ok {
			matched = append(matched, raw)
		}
	}
	n, size := len(matched), lr.pageSize
	out := &filteredList{
		Type: merged.Type, Page: max(lr.page, 1), PageSize: size,
		TotalPages: (n + size - 1) / size, TotalEntries: n,
		HasMore: merged.HasMore, Entries: matched,
	}
	if !lr.all {
		start := min((out.Page-1)*size, n)
		out.Entries = matched[start:min(start+size, n)]
		out.HasMore = out.HasMore || out.Page < out.TotalPages
	}
	return afterRequest(output.RenderValue(a.outputOptions(c.Context(), a.outputFormat(a.resolved)), out, e.Columns))
}

// matchesLocal reports whether the entry has every property of want with
// that exact string value (null or another type never matches).
func matchesLocal(raw json.RawMessage, want map[string]string) (bool, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return false, err
	}
	for k, v := range want {
		var got any
		if len(entry[k]) > 0 {
			if err := json.Unmarshal(entry[k], &got); err != nil {
				return false, err
			}
		}
		if got != v {
			return false, nil
		}
	}
	return true, nil
}
