package output

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Column describes one table column: Header is the column title (empty:
// the upper-cased Path), Path the dotted JSON path of the value (as in
// --fields) and Fmt an optional formatter for the value. Fallback is an
// optional second path whose value is shown when the value at Path is null
// or missing (company: customer_no, else supplier_no). Fmt is not called
// for null or missing values; those are always empty cells.
type Column struct {
	Header, Path, Fallback string
	Fmt                    func(any) string
}

// Columns are the default columns of a resource: -o table shows Table,
// -o wide shows Table followed by Wide.
type Columns struct {
	Table, Wide []Column
}

// header is the single-line column title: Header or, if empty, the
// upper-cased path (e.g. BILLING.COMPANY for --fields billing.company).
func (c Column) header() string {
	if c.Header != "" {
		return oneLine(c.Header)
	}
	return oneLine(strings.ToUpper(c.Path))
}

// FmtDurationHM formats a duration in seconds as h:mm (whole minutes,
// truncated). The API sends TimeEntry.duration in seconds: the spec
// examples of /tracker/time_entries have 13:44–16:44 = 10800 and
// 10:20–12:00 = 6000. Values that are no integer are shown as given.
func FmtDurationHM(v any) string {
	var s string
	switch v := v.(type) {
	case json.Number:
		s = string(v)
	case string:
		s = v
	case int64:
		s = strconv.FormatInt(v, 10)
	case int:
		s = strconv.Itoa(v)
	default:
		return plain(v)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	m := n / 60
	if m == 0 {
		sign = "" // -59 s is 0:00, not -0:00
	}
	return fmt.Sprintf("%s%d:%02d", sign, m/60, m%60)
}

// FmtMoney shows an amount exactly as the API sent it (1190.00 stays
// 1190.00): no float conversion, no rounding, no thousands separators.
func FmtMoney(v any) string {
	return plain(v)
}

// FmtDate shows a date (YYYY-MM-DD) unchanged and a date-time as
// YYYY-MM-DD HH:MM in the offset the API sent (Papierkram sends
// Europe/Berlin offsets; converting to the local time zone would make the
// same data look different per machine). Other values are shown as given.
func FmtDate(v any) string {
	s, ok := v.(string)
	if !ok {
		return plain(v)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02 15:04")
	}
	return s
}

// FmtIDNameList shows a list of referenced objects (e.g. the invoice
// templates of GET /info) as "249 Invoice type 1, 250 Invoice type 2":
// id and name per element, comma-separated, untruncated. Elements without
// id or name show what they have; other values are shown as plain cells.
func FmtIDNameList(v any) string {
	list, ok := v.([]any)
	if !ok {
		return plain(v)
	}
	parts := make([]string, 0, len(list))
	for _, el := range list {
		o, ok := el.(object)
		if !ok {
			parts = append(parts, plain(el))
			continue
		}
		var words []string
		for _, k := range []string{"id", "name"} {
			if x, ok := o.get(k); ok && x != nil {
				words = append(words, plain(x))
			}
		}
		parts = append(parts, strings.Join(words, " "))
	}
	return strings.Join(parts, ", ")
}
