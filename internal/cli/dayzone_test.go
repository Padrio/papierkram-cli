package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// lateUTCApp is an app on a UTC host (an agent's container) at 23:30 UTC
// on 2026-11-04: in Berlin, the day of Papierkram, it is already
// 2026-11-05.
func lateUTCApp(t *testing.T) (*App, *mockapi.Server) {
	t.Helper()
	app, srv := apiApp(t)
	app.Location = time.UTC
	app.Now = func() time.Time { return time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC) }
	return app, srv
}

// today/heute and yesterday/gestern are days of Papierkram (Europe/Berlin)
// for document and payment dates and list filters, like the sent_on check
// of deliver (dayZone), not of the host zone (M7).
func TestRelativeDatesUseBerlinDay(t *testing.T) {
	t.Run("create/update", func(t *testing.T) {
		app, srv := lateUTCApp(t)
		wantExit(t, app, runWrite(t, app, "project", "update", "5", "--start-date", "heute", "--end-date", "gestern", "-o", "json"), 0)
		wantBody(t, srv, "PUT", "/projects/5", `{"start_date":"2026-11-05","end_date":"2026-11-04"}`)
	})
	t.Run("pay", func(t *testing.T) {
		app, srv := lateUTCApp(t)
		wantExit(t, app, Run(app, []string{"voucher", "pay", "295", "--amount", "10", "--date", "today", "--yes", "-o", "json"}), 0)
		wantBody(t, srv, "POST", payPath, `{"value":10,"payment_date":"2026-11-05"}`)
	})
	t.Run("date filter", func(t *testing.T) {
		list := specEntry(t, "ListInvoices", "invoice", "list", registry.ClassReadOnly,
			registry.Filter{Flag: "from", Param: "document_date_range_start", Kind: "date", Help: "ab"},
			registry.Filter{Flag: "to", Param: "document_date_range_end", Kind: "date", Help: "bis"},
		)
		app, srv := lateUTCApp(t)
		wantExit(t, app, runEntries(t, app, nil, []string{"invoice", "list", "--from", "gestern", "--to", "heute"}, list), 0)
		q := wantRequests(t, srv, 1)[0].Query
		if q.Get("document_date_range_start") != "2026-11-04" || q.Get("document_date_range_end") != "2026-11-05" {
			t.Errorf("query = %v", q)
		}
	})
}

// The datetime filters of time list (--since/--until) are the second
// exception (N3): their keywords name the host's day, like their host-zone
// bounds and like time log and time stop book. The check AGENTS.md
// prescribes after time stop (time list --since heute) must find the entry
// just booked, also on a UTC host after 23:00 UTC.
func TestTimeListSinceTodayFindsTimeLogDay(t *testing.T) {
	app, srv := lateUTCApp(t)
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--duration", "30m", "-o", "json"}), 0)
	var booked struct {
		EntryDate string `json:"entry_date"`
	}
	if err := json.Unmarshal(wantRequests(t, srv, 1)[0].Body, &booked); err != nil {
		t.Fatal(err)
	}

	app, srv = lateUTCApp(t)
	wantExit(t, app, runEntries(t, app, nil, []string{"time", "list", "--since", "heute", "--until", "heute"}, timeListEntry(t)), 0)
	q := wantRequests(t, srv, 1)[0].Query
	since, until := q.Get("start_time_range_start"), q.Get("start_time_range_end")
	if booked.EntryDate != "2026-11-04" || since != booked.EntryDate+"T00:00:00Z" || until != booked.EntryDate+"T23:59:59Z" {
		t.Errorf("time log entry_date %s, time list --since %s --until %s", booked.EntryDate, since, until)
	}
}

// time log is the exception: its date goes with wall-clock times of the
// host clock (--duration without --from ends now, time stop books the
// timer's start), so its keywords stay on the host's day.
func TestTimeLogDateStaysOnHostDay(t *testing.T) {
	app, srv := lateUTCApp(t)
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--duration", "30m", "-o", "json"}), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"23:00","ended_at_time":"23:30","task":{"id":545},"user":{"id":1}}`)

	app, srv = lateUTCApp(t)
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", "heute", "--from", "9:00", "--to", "10:00", "-o", "json"}), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:00","task":{"id":545},"user":{"id":1}}`)
}
