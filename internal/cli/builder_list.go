package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

const (
	// defaultLimit is the page size of list without --limit (spec 7.1);
	// allPageSize the page size of --all without --limit: the API maximum,
	// so --all needs as few pages (credits) as possible.
	defaultLimit = 25
	allPageSize  = 100
	// minLimit and maxLimit bound --limit, firstPage --page and
	// minMaxPages --max-pages (list and api --paginate); the checks and
	// commands --json (minimum, maximum) both use them.
	minLimit    = 1
	maxLimit    = 100
	firstPage   = 1
	minMaxPages = 1
	// warnPages is the number of pages from which --all warns and, in a
	// terminal, asks before loading more (spec 8.5).
	warnPages = 10
)

// localKind is the filter kind the CLI applies itself: the API ignores the
// parameter (company list --type, spec 9.7), so the list runner loads all
// pages and keeps the entries whose property Param has the given value of
// its enum.
const localKind = "local_enum"

// localFlags returns the flags of e's local filters, with dashes.
func localFlags(e registry.Entry) []string {
	var flags []string
	for _, flt := range e.Filters {
		if flt.Kind == localKind {
			flags = append(flags, "--"+flt.Flag)
		}
	}
	return flags
}

// loadAllFlags are the flags that make list load all pages: --all and the
// local filters.
func loadAllFlags(e registry.Entry) string {
	return strings.Join(append([]string{"--all"}, localFlags(e)...), " oder ")
}

// addListFlags adds the common list flags (spec 7.1) and the filter flags
// of e: "id" filters are integer flags, all others text flags that
// runList parses. The filter of e's preset names the preset sources.
func addListFlags(cmd *cobra.Command, e registry.Entry) {
	f := cmd.Flags()
	page := fmt.Sprintf("nur diese Seite laden (ab %d)", firstPage)
	limit := fmt.Sprintf("Einträge pro Seite, %d–%d (--all ohne --limit: %d)", minLimit, maxLimit, allPageSize)
	if local := strings.Join(localFlags(e), "/"); local != "" {
		// with a local filter all pages are loaded; both apply to the result
		page += "; mit " + local + ": diese Seite des gefilterten Ergebnisses ausgeben"
		limit += "; mit " + local + ": Seitengröße des gefilterten Ergebnisses"
	}
	f.Int("page", 0, page)
	f.Int("limit", defaultLimit, limit)
	f.Bool("all", false, "alle Seiten laden und zusammenführen (1 Anfrage je Seite)")
	f.Int("max-pages", 0, "mit "+loadAllFlags(e)+" höchstens so viele Seiten laden")
	markBounds(f, "page", bound(firstPage), nil)
	markBounds(f, "limit", bound(minLimit), bound(maxLimit))
	markBounds(f, "max-pages", bound(minMaxPages), nil)
	f.String("sort", "", "nach diesem Feld sortieren (setzt order_by und order_direction)")
	f.Bool("desc", false, "absteigend sortieren (nur mit --sort)")
	for _, flt := range e.Filters {
		help := flt.Help
		// the values filterValue accepts (enum and local filters)
		var values []string
		if flt.Kind == "enum" || flt.Kind == localKind {
			_, values = filterEnum(e, flt)
		}
		if flt.Kind == "enum" && len(values) > 0 {
			help += " (erlaubt: " + strings.Join(values, ", ") + ")"
		}
		if flt.Flag == e.Preset {
			help += "; ohne Angabe " + presetNote(e.Preset)
		}
		if flt.Kind == "id" {
			f.Int64(flt.Flag, 0, help)
		} else {
			f.String(flt.Flag, "", help)
		}
		markEnum(f, flt.Flag, values)
		markSchemaBounds(f, flt.Flag, filterSchema(e, flt))
		if flt.Kind == "date" {
			annotate(f, flt.Flag, flagValueType, "date") // parsed like a date field
		}
		if flt.Kind == localKind {
			annotate(f, flt.Flag, flagLocalFilter, "true")
		}
		if flt.Flag == e.Preset {
			// presetQuery runs before the configuration is resolved
			markPreset(f, flt.Flag, e.Preset, false)
		}
	}
}

// filterEnum returns the schema and the values (without null) of an enum
// filter: the query parameter of "enum" (time list --state:
// billing_state), the list entry property of "local_enum". The flag help
// and the check in filterValue both use it, so they name the same values.
func filterEnum(e registry.Entry, flt registry.Filter) (*openapi3.Schema, []string) {
	s := filterSchema(e, flt)
	if s == nil {
		return nil, nil
	}
	var values []string
	for _, v := range s.Enum {
		if v != nil { // nullable enums list null
			values = append(values, fmt.Sprint(v))
		}
	}
	return s, values
}

// filterSchema is the spec schema of a filter: the list entry property of
// "local_enum", the query parameter of every other kind; nil when the spec
// has none.
func filterSchema(e registry.Entry, flt registry.Filter) *openapi3.Schema {
	if flt.Kind == localKind {
		return spec.ListEntryProperty(e.OperationID, flt.Param)
	}
	if p := spec.QueryParams(e.OperationID)[flt.Param]; p != nil && p.Schema != nil {
		return p.Schema.Value
	}
	return nil
}

// listRequest is a validated list invocation.
type listRequest struct {
	query    url.Values // filters, sorting and page_size; page is set per request
	page     int        // --page, 0 when not given
	pageSize int        // --limit: the API page size, with local filters the page size of the filtered result
	all      bool
	maxPages int // 0: no cap
	// local are the values of the local filters by entry property;
	// hasLocal is set when one is given: then all pages are loaded. via
	// names the flag that loads all pages in warnings and prompts (--all
	// or the local filter), it is display text only.
	local    map[string]string
	hasLocal bool
	via      string
}

// parseListFlags validates every list flag and builds the query, before
// any request (plan: flags first). Usage errors exit 2; filter values that
// fail the spec (enum, date) are validation errors (exit 5, R30).
func (a *App) parseListFlags(c *cobra.Command, e registry.Entry) (*listRequest, error) {
	f := c.Flags()
	lr := &listRequest{query: url.Values{}, local: map[string]string{}, via: "--all"}
	for _, flt := range e.Filters {
		if flt.Kind == localKind && f.Changed(flt.Flag) {
			lr.hasLocal, lr.via = true, "--"+flt.Flag
		}
	}
	lr.page, _ = f.GetInt("page")
	lr.pageSize, _ = f.GetInt("limit")
	lr.all, _ = f.GetBool("all")
	lr.maxPages, _ = f.GetInt("max-pages")
	sort, _ := f.GetString("sort")
	desc, _ := f.GetBool("desc")
	switch {
	case lr.pageSize < minLimit || lr.pageSize > maxLimit:
		return nil, listUsage(c, fmt.Sprintf("ungültiger Wert %d für --limit: erlaubt ist %d–%d", lr.pageSize, minLimit, maxLimit))
	case f.Changed("page") && lr.page < firstPage:
		return nil, listUsage(c, fmt.Sprintf("ungültiger Wert %d für --page: Seiten beginnen bei %d", lr.page, firstPage))
	case f.Changed("page") && lr.all:
		return nil, listUsage(c, "--page und --all schließen sich aus")
	case f.Changed("max-pages") && !lr.all && !lr.hasLocal:
		return nil, listUsage(c, "--max-pages gilt nur zusammen mit "+loadAllFlags(e))
	case f.Changed("max-pages") && lr.maxPages < minMaxPages:
		return nil, listUsage(c, fmt.Sprintf("ungültiger Wert %d für --max-pages: mindestens %d", lr.maxPages, minMaxPages))
	case f.Changed("sort") && strings.TrimSpace(sort) == "":
		return nil, listUsage(c, "--sort braucht einen Feldnamen")
	case desc && sort == "":
		return nil, listUsage(c, "--desc gilt nur zusammen mit --sort")
	}
	if lr.all && !f.Changed("limit") {
		lr.pageSize = allPageSize
	}
	// R20: order_by only ever together with order_direction.
	if sort != "" {
		dir := "asc"
		if desc {
			dir = "desc"
		}
		lr.query.Set("order_by", sort)
		lr.query.Set("order_direction", dir)
	}
	var preset *registry.Filter // the unset filter of e's preset
	for _, flt := range e.Filters {
		if !f.Changed(flt.Flag) {
			if flt.Flag == e.Preset {
				preset = &flt
			}
			continue
		}
		v, err := a.filterValue(c, e, flt)
		if err != nil {
			return nil, err
		}
		if flt.Kind == localKind {
			lr.local[flt.Param] = v // never sent: the API ignores it
			continue
		}
		lr.query.Set(flt.Param, v)
	}
	// spec 9.8: the preset filter (task list --project), only once every
	// flag is valid: resolving it reads environment, project file and
	// configuration, whose errors must not hide a flag error (exit 2)
	if preset != nil {
		if err := a.presetQuery(e, *preset, lr); err != nil {
			return nil, err
		}
	}
	apiSize := lr.pageSize
	if lr.hasLocal {
		apiSize = allPageSize // as few pages (credits) as possible
	}
	lr.query.Set("page_size", strconv.Itoa(apiSize))
	return lr, nil
}

func listUsage(c *cobra.Command, msg string) error {
	return clierr.New(clierr.CodeUsage, msg, "Verwendung: "+useLine(c))
}

// datetimeHint lists the accepted forms of the date-time filters.
const datetimeHint = "erlaubt: YYYY-MM-DD, today/heute, yesterday/gestern oder RFC 3339 (z. B. 2026-10-25T08:00:00+02:00)"

// filterValue parses the value of a filter flag by its kind: "id" an
// integer ≥ 0; "date" a date (YYYY-MM-DD, today/heute, yesterday/gestern);
// "datetime_from"/"datetime_to" a date-time in RFC 3339, where a date
// means the start or the end of that day in the local time zone; "enum"
// one of the values of the spec parameter, "local_enum" one of the values
// of the list entry property, both with suggestions.
func (a *App) filterValue(c *cobra.Command, e registry.Entry, flt registry.Filter) (string, error) {
	if flt.Kind == "id" {
		v, _ := c.Flags().GetInt64(flt.Flag)
		if v < 0 {
			return "", listUsage(c, fmt.Sprintf("ungültiger Wert %d für --%s: ID muss ≥ 0 sein", v, flt.Flag))
		}
		return strconv.FormatInt(v, 10), nil
	}
	raw, _ := c.Flags().GetString(flt.Flag)
	cc := a.dayContext(e.Resource)
	field := spec.Field{Flag: flt.Flag, Path: []string{flt.Param}, Kind: spec.KindScalar, Type: "string"}
	switch flt.Kind {
	case "date":
		field.Format = "date"
		v, err := spec.CoerceFlag(cc, field, raw)
		if err != nil {
			return "", err
		}
		return v.(string), nil
	case "datetime_from", "datetime_to":
		// only time list has these: the host's day (dayContext), like the
		// host-zone bounds of datetimeValue and the day time log and
		// time stop book on
		return a.datetimeValue(cc, field, flt.Kind == "datetime_to", raw)
	case "enum", localKind:
		s, values := filterEnum(e, flt)
		if len(values) == 0 {
			return "", clierr.New(clierr.CodeError, fmt.Sprintf("--%s: Parameter %s von %s hat in der Spec keine Werteliste", flt.Flag, flt.Param, e.OperationID), "")
		}
		field.Enum, field.Schema = values, s
		v, err := spec.CoerceFlag(cc, field, raw)
		if err != nil {
			return "", err
		}
		return v.(string), nil
	}
	return "", clierr.New(clierr.CodeError, fmt.Sprintf("--%s: unbekannte Filterart %q", flt.Flag, flt.Kind), "")
}

// datetimeValue accepts RFC 3339 as given; a date (any form of "date")
// becomes 00:00:00 or, for the end of a range, 23:59:59 of that day in
// a.Location, with the offset valid at that time (Review Focus 4). A
// keyword names the day of cc (dayContext of the resource; for time, the
// host's day), so the range is that of the same explicit date in the host
// zone.
func (a *App) datetimeValue(cc spec.CoerceContext, field spec.Field, end bool, raw string) (string, error) {
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw, nil
	}
	field.Format = "date"
	v, err := spec.CoerceFlag(cc, field, raw)
	if err != nil {
		ce := clierr.From(err)
		ce.Hint = datetimeHint
		return "", ce
	}
	loc := a.Location
	if loc == nil {
		loc = time.Local
	}
	d, err := time.ParseInLocation("2006-01-02", v.(string), loc)
	if err != nil { // CoerceFlag only returns valid dates
		return "", err
	}
	if end {
		d = time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, loc)
	}
	return d.Format(time.RFC3339), nil
}

// runList lists one page, or with --all every page merged into one
// envelope (spec 10.3).
func (a *App) runList(c *cobra.Command, e registry.Entry) error {
	lr, err := a.parseListFlags(c, e)
	if err != nil {
		return err
	}
	// after the flags, like the preset filter: resolving a preset path
	// flag (person list --company) reads environment, project file and
	// configuration, whose errors must not hide a flag error (exit 2)
	params, err := a.pathValues(c, e, nil)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	client, _, err := a.Client(c)
	if err != nil {
		return err
	}
	if lr.hasLocal {
		return a.runLocalFilter(c, e, client, params, lr)
	}
	if !lr.all {
		if lr.page > 0 {
			lr.query.Set("page", strconv.Itoa(lr.page))
		}
		resp, err := client.Call(c.Context(), e.OperationID, params, lr.query, nil)
		if err != nil {
			return pageTooLarge(err, hintListTooLarge)
		}
		return a.render(c, resp.Body, e.Columns)
	}
	env, err := a.fetchAll(c, e, client, params, lr)
	if err != nil {
		return err
	}
	return afterRequest(output.RenderValue(a.outputOptions(c.Context(), a.outputFormat(a.resolved)), env, e.Columns))
}

// listPage is the part of a list envelope --all needs. Entries stay raw
// bytes, so numbers and texts are merged exactly as the API sent them.
type listPage struct {
	Type         json.RawMessage   `json:"type"`
	PageSize     json.RawMessage   `json:"page_size"`
	TotalPages   json.RawMessage   `json:"total_pages"`
	TotalEntries json.RawMessage   `json:"total_entries"`
	Entries      []json.RawMessage `json:"entries"`
}

// mergedList is the envelope of --all: the envelope of page 1 unchanged
// except page (1), has_more and the entries of all loaded pages
// (spec 10.3).
type mergedList struct {
	Type         json.RawMessage   `json:"type"`
	Page         int               `json:"page"`
	PageSize     json.RawMessage   `json:"page_size"`
	TotalPages   json.RawMessage   `json:"total_pages"`
	TotalEntries json.RawMessage   `json:"total_entries"`
	HasMore      bool              `json:"has_more"`
	Entries      []json.RawMessage `json:"entries"`
}

// pageFunc sends one list request with query (page included).
type pageFunc func(query url.Values) (*api.Response, error)

// fetchAll is fetchPages for the list operation of e.
func (a *App) fetchAll(c *cobra.Command, e registry.Entry, client *api.Client, params []int64, lr *listRequest) (*mergedList, error) {
	return a.fetchPages(c, e.Credits, lr, func(q url.Values) (*api.Response, error) {
		resp, err := client.Call(c.Context(), e.OperationID, params, q, nil)
		return resp, pageTooLarge(err, hintListTooLarge)
	})
}

// Hints for a list page longer than api.MaxResponseSize (ruling T15-A):
// list and api each name their own way to ask for smaller pages.
const (
	hintListTooLarge    = "mit --limit kleinere Seiten anfordern"
	hintAPIListTooLarge = "mit kleinerem page_size kleinere Seiten anfordern (z. B. -f page_size=25)"
)

// pageTooLarge replaces the neutral hint of a list answer longer than
// api.MaxResponseSize with hint; every other error stays as it is.
func pageTooLarge(err error, hint string) error {
	if !errors.Is(err, api.ErrResponseTooLarge) {
		return err
	}
	ce := *clierr.From(err)
	ce.Hint = hint
	return &ce
}

// fetchPages loads page 1, decides from total_pages how many pages to load
// (capped by --max-pages), warns from warnPages on (credits per page) and
// asks in a terminal, then loads the remaining pages with fetch. list
// --all and papierkram api --paginate share it.
func (a *App) fetchPages(c *cobra.Command, credits int, lr *listRequest, fetch pageFunc) (*mergedList, error) {
	first, err := fetchPage(fetch, lr.query, 1)
	if err != nil {
		return nil, err
	}
	// total_pages decides how many pages exist; null or missing would make
	// --all claim a complete list.
	var total int64
	if err := json.Unmarshal(first.TotalPages, &total); err != nil || total < 0 || string(first.TotalPages) == "null" {
		return nil, unexpectedList(fmt.Errorf("total_pages ist %s", orMissing(first.TotalPages)))
	}
	pages := max(total, 1)
	capped := lr.maxPages > 0 && int64(lr.maxPages) < pages
	if capped {
		pages = int64(lr.maxPages)
	}
	if pages >= warnPages {
		output.Warn(a.Err, fmt.Sprintf("%s lädt %d Seiten (%d Credits); mit --max-pages begrenzen", lr.via, pages, pages*int64(credits)))
		if err := a.confirmAll(c.Context(), lr.via); err != nil {
			return nil, err
		}
	}
	merged := &mergedList{
		Type: first.Type, Page: 1, PageSize: first.PageSize,
		TotalPages: first.TotalPages, TotalEntries: first.TotalEntries,
		Entries: append([]json.RawMessage{}, first.Entries...),
	}
	for p := int64(2); p <= pages; p++ {
		page, err := fetchPage(fetch, lr.query, p)
		if err != nil {
			return nil, err
		}
		if len(page.Entries) == 0 {
			break
		}
		merged.Entries = append(merged.Entries, page.Entries...)
	}
	if capped {
		merged.HasMore = true
		more := "weitere Einträge vorhanden"
		if lr.hasLocal {
			// the pages not loaded may or may not hold matches
			more = "Ergebnis unvollständig, möglicherweise weitere Treffer"
		}
		output.Warn(a.Err, fmt.Sprintf("--max-pages %d: %d von %d Seiten geladen; %s", lr.maxPages, pages, total, more))
	}
	return merged, nil
}

// fetchPage loads one page of the list.
func fetchPage(fetch pageFunc, query url.Values, page int64) (*listPage, error) {
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("page", strconv.FormatInt(page, 10))
	resp, err := fetch(q)
	if err != nil {
		return nil, err
	}
	var lp listPage
	dec := json.NewDecoder(bytes.NewReader(resp.Body))
	if err := dec.Decode(&lp); err != nil {
		return nil, unexpectedList(err)
	}
	return &lp, nil
}

// orMissing shows a raw JSON value, or "nicht vorhanden" when it is absent.
func orMissing(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "nicht vorhanden"
	}
	return string(raw)
}

func unexpectedList(err error) error {
	e := clierr.New(clierr.CodeError, "unerwartete Listenantwort der Papierkram-API: "+err.Error(), "")
	e.Cause = err
	return e
}

// confirmAll asks "Fortfahren? [j/N]" when stdin is a terminal and neither
// --yes nor --no-input is given; without a terminal --all proceeds (the
// warning is on stderr). Declining is exit 8, an interrupt exit 10. via is
// the flag that loads all pages (--all or a local filter).
func (a *App) confirmAll(ctx context.Context, via string) error {
	if !a.StdinTTY || a.Flags.Yes || a.Flags.NoInput {
		return nil
	}
	line, err := a.prompt(ctx, "Fortfahren? [j/N] ")
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "j", "ja", "y", "yes":
		return nil
	}
	return clierr.New(clierr.CodeConfirmation, fmt.Sprintf("Abgebrochen: %s nicht fortgesetzt (nur Seite 1 geladen)", via),
		"Mit --max-pages begrenzen oder mit --yes ohne Rückfrage laden")
}
