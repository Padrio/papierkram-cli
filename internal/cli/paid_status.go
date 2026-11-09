package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// invoice paid-status (spec v0.2 §5): the open items of the invoice list.
// It loads the list like invoice list --all (ListInvoices, 100 per page,
// fetchPages) but with at most --max-pages pages (default 5), keeps the
// invoices with outstanding_amount > 0 whose state is not in
// paidStatusSkipStates, and sums their amounts exactly (math/big). The
// filters are those of invoice list: --customer takes the company filter
// (ID or name), --from and --to the document date filters.

// paidStatusSkipStates are the states of invoices that are never open
// items, whatever outstanding_amount says.
var paidStatusSkipStates = []string{"draft", "canceled"}

// paidStatusMaxPages is the default of --max-pages.
const paidStatusMaxPages = 5

// paidStatusFilters map the filter flags of paid-status to the filters of
// invoice list (registry ListInvoices) whose parameter and kind they use.
var paidStatusFilters = []struct{ flag, filter, help string }{
	{"customer", "company", "Nur Rechnungen dieses Kunden (ID)"},
	{"from", "from", ""},
	{"to", "to", ""},
}

// paidStatusEntry is one open item. Amounts are the text the API sent
// (number or decimal string), as JSON numbers.
type paidStatusEntry struct {
	ID                int64       `json:"id"`
	InvoiceNo         *string     `json:"invoice_no"`
	Name              string      `json:"name"`
	Customer          string      `json:"customer"`
	State             string      `json:"state"`
	DocumentDate      *string     `json:"document_date"`
	DueDate           *string     `json:"due_date"`
	DaysOverdue       int         `json:"days_overdue"`
	TotalGross        json.Number `json:"total_gross"`
	OutstandingAmount json.Number `json:"outstanding_amount"`
}

// paidStatusReport is the output of paid-status (docs/schemas/paid-status.json).
// Complete is false when --max-pages left pages out: then the sum and the
// counts cover the loaded pages only.
type paidStatusReport struct {
	Entries          []paidStatusEntry `json:"entries"`
	TotalOutstanding json.Number       `json:"total_outstanding"`
	Count            int               `json:"count"`
	OverdueCount     int               `json:"overdue_count"`
	AsOf             string            `json:"as_of"`
	Complete         bool              `json:"complete"`
}

// paidStatusColumns are the table columns; amounts in German notation
// like the sum line.
var paidStatusColumns = output.Columns{
	Table: []output.Column{
		{Header: "NR.", Path: "invoice_no"},
		{Header: "KUNDE", Path: "customer"},
		{Header: "DATUM", Path: "document_date", Fmt: output.FmtDate},
		{Header: "FÄLLIG", Path: "due_date", Fmt: output.FmtDate},
		{Header: "ÜBERFÄLLIG", Path: "days_overdue", Fmt: fmtDaysOverdue},
		{Header: "BRUTTO", Path: "total_gross", Fmt: fmtAmount},
		{Header: "OFFEN", Path: "outstanding_amount", Fmt: fmtAmount},
	},
	Wide: []output.Column{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "STATUS", Path: "state"},
	},
}

// fmtDaysOverdue shows the days overdue, nothing for 0.
func fmtDaysOverdue(v any) string {
	if s := fmt.Sprint(v); s != "0" {
		return s
	}
	return ""
}

// fmtAmount shows an amount like germanAmount (119.0 → 119,00 €).
func fmtAmount(v any) string { return germanAmount(fmt.Sprint(v)) }

func (a *App) newPaidStatusCmd() *cobra.Command {
	e, ok := registry.Lookup("invoice", "list")
	cmd := &cobra.Command{
		Use:   "paid-status",
		Short: "Offene Posten: unbezahlte Rechnungen mit Summe",
		Long: "Zeigt die offenen Posten: Rechnungen mit outstanding_amount > 0, deren Status nicht " +
			strings.Join(paidStatusSkipStates, " oder ") + " ist (archivierte zählen mit), " +
			"sortiert nach Fälligkeit (ohne Fälligkeit zuletzt), mit den Tagen seit der Fälligkeit " +
			"(überfällig: Fälligkeit vor heute, Berliner Tag) und der Summe der offenen Beträge (exakt, auf den Cent gerundet). " +
			"--overdue zeigt nur die überfälligen; Summe und Anzahl gelten dann für sie.\n\n" +
			"Lädt die Rechnungsliste wie invoice list --all (100 je Seite), aber höchstens --max-pages Seiten. " +
			"Lässt --max-pages Seiten aus, warnt der Befehl auf stderr, und JSON hat complete false: " +
			"Summe und Anzahl gelten dann nur für die geladenen Seiten.\n\n" +
			"Fehlt outstanding_amount in der Liste, bricht der Befehl ab (Exit 1), statt eine falsche Summe zu zeigen; " +
			"--details liest dann diese Rechnungen einzeln nach.\n\n" +
			"Kosten: 1 Credit je Seite, mit --details 1 Credit je nachgelesener Rechnung; " +
			"ein Name bei --customer kostet 1 Credit je Seite der Firmenliste.",
		Example: "  papierkram invoice paid-status\n" +
			"  papierkram invoice paid-status --overdue -o json\n" +
			"  papierkram invoice paid-status --customer 3214 --from 2026-01-01 --to 2026-12-31",
		Annotations: map[string]string{annotClass: string(registry.ClassReadOnly), annotOperation: "ListInvoices"},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !ok || e.OperationID != "ListInvoices" {
				return errUnavailable(fmt.Errorf("invoice list (ListInvoices) fehlt in der Registry"))
			}
			return a.runPaidStatus(c, e)
		},
	}
	fs := cmd.Flags()
	for _, pf := range paidStatusFilters {
		flt, found := listFilter(e, pf.filter)
		if !found {
			continue // no flag without its filter (paidStatusQuery relies on it)
		}
		help := pf.help
		if help == "" {
			help = flt.Help
		}
		if flt.Kind == "id" {
			fs.Var(&rawValue{typ: "id"}, pf.flag, help)
			markResolves(fs, pf.flag)
		} else {
			fs.String(pf.flag, "", help)
		}
		if flt.Kind == "date" {
			annotate(fs, pf.flag, flagValueType, "date")
		}
	}
	fs.Bool("overdue", false, "nur überfällige Rechnungen (Fälligkeit vor heute, Berliner Tag)")
	fs.Int("max-pages", paidStatusMaxPages, fmt.Sprintf("höchstens so viele Seiten à %d Rechnungen laden (1 Credit je Seite)", allPageSize))
	markBounds(fs, "max-pages", bound(minMaxPages), nil)
	fs.Bool("details", false, "Rechnungen ohne outstanding_amount in der Liste einzeln nachlesen (1 Credit je Rechnung)")
	if read, ok := registry.Lookup("invoice", "describe"); ok {
		markCredits(fs, "details", read.Credits)
	}
	return cmd
}

// listFilter is the filter of e with the flag name.
func listFilter(e registry.Entry, name string) (registry.Filter, bool) {
	for _, flt := range e.Filters {
		if flt.Flag == name {
			return flt, true
		}
	}
	return registry.Filter{}, false
}

// paidStatusQuery builds the filter query of the flags given, each through
// filterValue with the filter of invoice list under the paid-status flag
// name (a name of --customer is 0 until resolveNames replaced it).
func (a *App) paidStatusQuery(c *cobra.Command, e registry.Entry) (url.Values, error) {
	q := url.Values{}
	for _, pf := range paidStatusFilters {
		if !c.Flags().Changed(pf.flag) {
			continue
		}
		flt, _ := listFilter(e, pf.filter) // the flag exists only with its filter
		flt.Flag = pf.flag
		v, err := a.filterValue(c, e, flt)
		if err != nil {
			return nil, err
		}
		q.Set(flt.Param, v)
	}
	q.Set("page_size", strconv.Itoa(allPageSize))
	return q, nil
}

// runPaidStatus: flags (exit 2, filter values exit 5) → client → names →
// pages → report → --details → output. Order like runList.
func (a *App) runPaidStatus(c *cobra.Command, e registry.Entry) error {
	if err := checkNameFlags(c); err != nil {
		return err
	}
	f := c.Flags()
	maxPages, _ := f.GetInt("max-pages")
	if maxPages < minMaxPages {
		return listUsage(c, fmt.Sprintf("ungültiger Wert %d für --max-pages: mindestens %d", maxPages, minMaxPages))
	}
	query, err := a.paidStatusQuery(c, e)
	if err != nil {
		return err
	}
	client, r, err := a.Client(c)
	if err != nil {
		return err
	}
	res, err := a.resolveNames(c, client, r)
	if err != nil {
		return err
	}
	if len(res) > 0 {
		if query, err = a.paidStatusQuery(c, e); err != nil {
			return err
		}
	}
	lr := &listRequest{query: query, pageSize: allPageSize, all: true, maxPages: maxPages, via: "invoice paid-status"}
	merged, err := a.fetchAll(c, e, client, nil, lr)
	if err != nil {
		return err
	}
	today := a.Now().In(a.dayZone())
	overdue, _ := f.GetBool("overdue")
	report, missing, err := buildPaidStatus(merged.Entries, today, overdue)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		if details, _ := f.GetBool("details"); !details {
			return clierr.New(clierr.CodeError,
				fmt.Sprintf("Rechnungsliste ohne outstanding_amount bei %d Rechnung(en): offene Posten nicht bestimmbar", len(missing)),
				"mit --details je Rechnung nachlesen (1 Credit je Rechnung)")
		}
		entries, err := a.paidStatusDetails(c, client, merged.Entries, missing)
		if err != nil {
			return err
		}
		if report, missing, err = buildPaidStatus(entries, today, overdue); err != nil {
			return err
		}
		if len(missing) > 0 {
			return clierr.New(clierr.CodeError,
				fmt.Sprintf("Rechnung %s ohne outstanding_amount, auch einzeln gelesen: offene Posten nicht bestimmbar", joinIDs(missing)), "")
		}
	}
	report.Complete = !merged.HasMore
	return a.renderPaidStatus(c, report)
}

// paidStatusDetails reads the invoices of missing one by one (invoice
// describe) and returns entries with their outstanding_amount.
func (a *App) paidStatusDetails(c *cobra.Command, client *api.Client, entries []json.RawMessage, missing []int64) ([]json.RawMessage, error) {
	read, _ := registry.Lookup("invoice", "describe") // GetInvoice, a fixed registry entry
	n := len(missing)
	what := "1 Rechnung"
	if n != 1 {
		what = fmt.Sprintf("%d Rechnungen", n)
	}
	_, _ = fmt.Fprintf(a.Err, "Hinweis: --details liest %s einzeln (%s)\n", what, creditsText(n*read.Credits))
	amounts := map[int64]json.RawMessage{}
	for _, id := range missing {
		resp, err := client.Call(c.Context(), read.OperationID, []int64{id}, nil, nil)
		if err != nil {
			return nil, err
		}
		var inv map[string]json.RawMessage
		if err := json.Unmarshal(resp.Body, &inv); err != nil {
			return nil, unexpectedInvoice(id, err)
		}
		amounts[id] = inv["outstanding_amount"]
	}
	out := make([]json.RawMessage, len(entries))
	for i, raw := range entries {
		out[i] = raw
		// buildPaidStatus decoded every entry before, so an entry that is
		// no object with an integer id was none of missing
		var entry map[string]json.RawMessage
		var id int64
		if json.Unmarshal(raw, &entry) != nil || json.Unmarshal(entry["id"], &id) != nil {
			continue
		}
		amount, ok := amounts[id]
		if !ok || len(amount) == 0 {
			continue
		}
		entry["outstanding_amount"] = amount
		if patched, err := json.Marshal(entry); err == nil { // raw JSON values only
			out[i] = patched
		}
	}
	return out, nil
}

func unexpectedInvoice(id int64, err error) error {
	e := clierr.New(clierr.CodeError, fmt.Sprintf("unerwartete Antwort der Papierkram-API zu Rechnung %d: %v", id, err), "")
	e.Cause = err
	return e
}

// joinIDs lists IDs separated by commas.
func joinIDs(ids []int64) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(s, ", ")
}

// paidStatusRow is the part of an invoice list entry paid-status reads.
type paidStatusRow struct {
	ID                int64           `json:"id"`
	InvoiceNo         *string         `json:"invoice_no"`
	Name              *string         `json:"name"`
	State             *string         `json:"state"`
	DocumentDate      *string         `json:"document_date"`
	DueDate           *string         `json:"due_date"`
	TotalGross        json.RawMessage `json:"total_gross"`
	OutstandingAmount json.RawMessage `json:"outstanding_amount"`
	Billing           *struct {
		Company *string `json:"company"`
	} `json:"billing"`
}

// buildPaidStatus builds the report of the list entries as of the day of
// today (its location is the day zone), with overdueOnly only the
// overdue items. The second value lists the IDs of entries that may be
// open but have no outstanding_amount (missing or null): the report
// leaves them out, the caller must not show it as complete. Complete is
// left to the caller.
func buildPaidStatus(entries []json.RawMessage, today time.Time, overdueOnly bool) (paidStatusReport, []int64, error) {
	day := calendarDay(today)
	type item struct {
		entry  paidStatusEntry
		due    time.Time
		hasDue bool
	}
	var items []item
	var missing []int64
	for _, raw := range entries {
		var row paidStatusRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return paidStatusReport{}, nil, unexpectedList(err)
		}
		state := deref(row.State)
		if slices.Contains(paidStatusSkipStates, state) {
			continue
		}
		if len(row.OutstandingAmount) == 0 || string(row.OutstandingAmount) == "null" {
			missing = append(missing, row.ID)
			continue
		}
		open, err := amountOf(row.ID, "outstanding_amount", row.OutstandingAmount)
		if err != nil {
			return paidStatusReport{}, nil, err
		}
		if r, _ := new(big.Rat).SetString(string(open)); r.Sign() <= 0 {
			continue
		}
		gross, err := amountOf(row.ID, "total_gross", row.TotalGross)
		if err != nil {
			return paidStatusReport{}, nil, err
		}
		it := item{entry: paidStatusEntry{
			ID: row.ID, InvoiceNo: row.InvoiceNo, Name: deref(row.Name), State: state,
			DocumentDate: row.DocumentDate, DueDate: row.DueDate, TotalGross: gross, OutstandingAmount: open,
		}}
		if row.Billing != nil {
			it.entry.Customer = deref(row.Billing.Company)
		}
		if row.DueDate != nil {
			if it.due, err = dueDay(row.ID, *row.DueDate, today.Location()); err != nil {
				return paidStatusReport{}, nil, err
			}
			it.hasDue = true
			if d := int(day.Sub(it.due).Hours() / 24); d > 0 {
				it.entry.DaysOverdue = d
			}
		}
		if overdueOnly && it.entry.DaysOverdue == 0 {
			continue
		}
		items = append(items, it)
	}
	slices.SortStableFunc(items, func(x, y item) int {
		switch {
		case x.hasDue != y.hasDue && x.hasDue:
			return -1
		case x.hasDue != y.hasDue:
			return 1
		}
		if c := x.due.Compare(y.due); c != 0 {
			return c
		}
		return cmp.Compare(x.entry.ID, y.entry.ID)
	})
	report := paidStatusReport{Entries: []paidStatusEntry{}, AsOf: today.Format(time.DateOnly)}
	var amounts []json.Number
	for _, it := range items {
		report.Entries = append(report.Entries, it.entry)
		amounts = append(amounts, it.entry.OutstandingAmount)
		if it.entry.DaysOverdue > 0 {
			report.OverdueCount++
		}
	}
	report.Count = len(report.Entries)
	total, err := sumDecimals(amounts)
	if err != nil {
		return paidStatusReport{}, nil, err
	}
	report.TotalOutstanding = total
	return report, missing, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// calendarDay is the date of t in its location, as midnight UTC: the
// difference of two such days is a whole number of 24 hours, also across
// a change of daylight saving time.
func calendarDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// dueDay is the calendar day of a due_date: a date, or a date-time taken
// as its day in loc (the day zone).
func dueDay(id int64, s string, loc *time.Location) (time.Time, error) {
	if d, err := time.Parse(time.DateOnly, s); err == nil {
		return d, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return calendarDay(t.In(loc)), nil
	}
	return time.Time{}, clierr.New(clierr.CodeError,
		fmt.Sprintf("Rechnung %d: ungültiges due_date „%s“ in der Rechnungsliste", id, printable(s)), "")
}

// amountOf reads an amount the API sent as number or decimal string; its
// text stays as sent. Anything else is exit 1 naming the invoice.
func amountOf(id int64, field string, raw json.RawMessage) (json.Number, error) {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal(raw, &s); err != nil {
			s = string(raw)
		}
	}
	if !isJSONNumber(s) {
		shown := s
		if len(raw) == 0 {
			shown = "fehlt"
		}
		return "", clierr.New(clierr.CodeError,
			fmt.Sprintf("Rechnung %d: ungültiger Betrag „%s“ in %s der Rechnungsliste", id, printable(shown), field), "")
	}
	return json.Number(s), nil
}

// sumDecimals adds decimal amounts exactly (math/big) and returns the sum
// with two decimals (rounded half away from zero).
func sumDecimals(vs []json.Number) (json.Number, error) {
	sum := new(big.Rat)
	for _, v := range vs {
		r, ok := new(big.Rat).SetString(string(v))
		if !ok || !isJSONNumber(string(v)) {
			return "", clierr.New(clierr.CodeError, fmt.Sprintf("ungültiger Betrag „%s“", printable(string(v))), "")
		}
		sum.Add(sum, r)
	}
	return json.Number(sum.FloatString(2)), nil
}

// renderPaidStatus prints the report: structured formats the report
// itself, table and wide the entries and a sum line.
func (a *App) renderPaidStatus(c *cobra.Command, report paidStatusReport) error {
	o := a.outputOptions(c.Context(), a.outputFormat(a.resolved))
	if o.Structured() {
		return afterRequest(output.RenderValue(o, report, paidStatusColumns))
	}
	if err := output.RenderValue(o, report.Entries, paidStatusColumns); err != nil {
		return afterRequest(err)
	}
	invoices := "1 Rechnung"
	if report.Count != 1 {
		invoices = fmt.Sprintf("%d Rechnungen", report.Count)
	}
	_, err := fmt.Fprintf(a.Out, "Summe offen: %s (%s, davon %d überfällig; Stand %s)\n",
		germanAmount(string(report.TotalOutstanding)), invoices, report.OverdueCount, report.AsOf)
	return afterRequest(err)
}
