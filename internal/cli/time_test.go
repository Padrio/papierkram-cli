package cli

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// time entry commands (spec 7.1, 9.8, 15 no. 81–87). time log has its
// own tests (time_log_test.go); the timer comes in a later task.

// TestTimeGolden renders list and describe from the spec examples of the
// mock in every output format. The working directory has no project file,
// so no preset applies.
func TestTimeGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"time-list", []string{"time", "list"}, "/tracker/time_entries", url.Values{"page_size": {"25"}}},
		{"time-describe", []string{"time", "describe", "272"}, "/tracker/time_entries/272", url.Values{}},
	}
	for _, tc := range cases {
		for _, format := range []string{"table", "wide", "json", "yaml"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				app, srv := apiApp(t)
				wantExit(t, app, Run(app, append(tc.args, "-o", format)), 0)
				r := wantRequests(t, srv, 1)[0]
				if r.Method != "GET" || r.Path != tc.path || r.Query.Encode() != tc.query.Encode() {
					t.Errorf("request = %s %s?%s", r.Method, r.Path, r.Query.Encode())
				}
				out, errs := bufs(app)
				if errs != "" {
					t.Errorf("stderr = %q", errs)
				}
				golden.Assert(t, tc.name+"."+format, []byte(out))
				assertNoToken(t, app)
			})
		}
	}
}

func TestTimeAliases(t *testing.T) {
	for _, args := range [][]string{
		{"zeit", "list", "-o", "json"},
		{"time-entry", "describe", "272", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

// Every filter of spec 4.2 maps 1:1 to its query parameter; --state is
// billing_state.
func TestTimeListFilters(t *testing.T) {
	app, srv := berlinApp(t)
	wantExit(t, app, Run(app, []string{"time", "list", "--project", "4963", "--task", "545", "--invoice", "494",
		"--user", "1", "--state", "unbilled", "--since", "2026-10-01T08:00:00+02:00", "--until", "2026-10-31", "-o", "json"}), 0)
	want := url.Values{
		"project_id": {"4963"}, "task_id": {"545"}, "invoice_id": {"494"}, "user_id": {"1"},
		"billing_state": {"unbilled"}, "start_time_range_start": {"2026-10-01T08:00:00+02:00"},
		"start_time_range_end": {"2026-10-31T23:59:59+01:00"}, "page_size": {"25"},
	}
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/tracker/time_entries" || r.Query.Encode() != want.Encode() {
		t.Errorf("request = %s?%s", r.Path, r.Query.Encode())
	}
}

// --since/--until with a date are the start and the end of that day in the
// local time zone, with the offset valid at that time: Berlin leaves
// daylight saving time on 2026-10-25.
func TestTimeListSinceUntilDayAcrossDSTChange(t *testing.T) {
	app, srv := berlinApp(t)
	wantExit(t, app, Run(app, []string{"time", "list", "--since", "2026-10-25", "--until", "2026-10-25", "-o", "json"}), 0)
	q := wantRequests(t, srv, 1)[0].Query
	if got := q.Get("start_time_range_start"); got != "2026-10-25T00:00:00+02:00" {
		t.Errorf("start_time_range_start = %q", got)
	}
	if got := q.Get("start_time_range_end"); got != "2026-10-25T23:59:59+01:00" {
		t.Errorf("start_time_range_end = %q", got)
	}

	app, srv = berlinApp(t)
	wantExit(t, app, Run(app, []string{"time", "list", "--until", "25.10.2026"}), 5)
	wantRequests(t, srv, 0)
}

// Spec R30 at the CLI level (TestR30_UnknownBillingStateRejectedLocally
// covers the builder): an unknown --state never reaches the API, which
// would answer 400. The help lists the values of the spec parameter.
func TestTimeListRejectsUnknownState(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "list", "--state", "offen"}), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--state") || !strings.Contains(errs, "billed, unbilled, billable, unbillable, archived") {
		t.Errorf("stderr = %q", errs)
	}

	app, _ = apiApp(t)
	cmd, _, err := NewRootCmd(app).Find([]string{"time", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if u := cmd.Flags().Lookup("state").Usage; !strings.HasSuffix(u, "(erlaubt: billed, unbilled, billable, unbillable, archived)") {
		t.Errorf("--state usage = %q", u)
	}
}

// Spec R16 at the create/update level: the body has entry_date as
// YYYY-MM-DD and the times as HH:MM, never ISO timestamps. The mock
// validates the body against the patched spec (auto-fail).
func TestTimeCreateSendsDateAndHHMM(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "create", "--entry-date", "2026-11-04", "--started-at-time", "09:00",
		"--ended-at-time", "10:30", "--task", "545", "--user", "1", "--comments", "Abstimmung mit Müller", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/time_entries", `{"entry_date":"2026-11-04","started_at_time":"09:00",`+
		`"ended_at_time":"10:30","task":{"id":545},"user":{"id":1},"comments":"Abstimmung mit Müller"}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "time_entry"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}

	// heute is the local date; --unbillable is a switch
	app, srv = berlinApp(t)
	wantExit(t, app, Run(app, []string{"time", "create", "--entry-date", "heute", "--started-at-time", "23:00",
		"--ended-at-time", "23:59", "--task", "545", "--user", "1", "--unbillable", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/time_entries", `{"entry_date":"2026-10-26","started_at_time":"23:00",`+
		`"ended_at_time":"23:59","task":{"id":545},"user":{"id":1},"unbillable":true}`)
}

// --started-at-time 9:00 and other forms than HH:MM are validation errors
// (exit 5) before any request; the hint names HH:MM.
// The help of time create --billable-duration names the measured
// behaviour of the API (2026-11-04): it stores duration instead of the
// billable_duration sent with the create. time update keeps the plain
// help: not measured there.
func TestTimeCreateBillableDurationHelpNamesMeasurement(t *testing.T) {
	line := func(action string) string {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{"time", action, "--help"}), 0)
		wantRequests(t, srv, 0)
		out, _ := bufs(app)
		i := strings.Index(out, "--billable-duration ")
		if i < 0 {
			t.Fatalf("time %s --help ohne --billable-duration:\n%s", action, out)
		}
		l, _, _ := strings.Cut(out[i:], "\n")
		return l
	}
	if l := line("create"); !strings.Contains(l, "in Sekunden") || !strings.Contains(l, "Beim Anlegen übernimmt die API die Dauer statt dieses Werts") {
		t.Errorf("time create: %q", l)
	}
	if l := line("update"); !strings.HasSuffix(strings.TrimSpace(l), "Abrechenbare Dauer in Sekunden") {
		t.Errorf("time update: %q", l)
	}
}

func TestTimeCreateRejectsTimesOtherThanHHMM(t *testing.T) {
	base := []string{"time", "create", "--entry-date", "2026-11-04", "--task", "545", "--user", "1"}
	for _, tc := range []struct {
		times []string
		want  string
	}{
		{[]string{"--started-at-time", "9:00", "--ended-at-time", "10:30"}, `started_at_time: ungültiger Wert "9:00", erwartet HH:MM`},
		{[]string{"--started-at-time", "09:00", "--ended-at-time", "2026-11-04T10:30:00+01:00"}, `ended_at_time: ungültiger Wert "2026-11-04T10:30:00+01:00", erwartet HH:MM`},
		{[]string{"--started-at-time", "09:00", "--ended-at-time", "24:00"}, `ended_at_time: ungültiger Wert "24:00", erwartet HH:MM`},
	} {
		for _, extra := range [][]string{nil, {"--dry-run"}} {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, append(append(append([]string{}, base...), tc.times...), extra...)), 5)
			wantRequests(t, srv, 0)
			_, errs := bufs(app)
			if !strings.Contains(errs, tc.want) || !strings.Contains(errs, "Hinweis: Wert im Format HH:MM angeben") {
				t.Errorf("%v %v: stderr = %q", tc.times, extra, errs)
			}
		}
	}

	// required fields and the date format are checked before any request too
	for _, args := range [][]string{
		{"time", "create", "--entry-date", "2026-11-04", "--started-at-time", "09:00", "--ended-at-time", "10:30", "--task", "545"},
		{"time", "create", "--entry-date", "04.11.2026", "--started-at-time", "09:00", "--ended-at-time", "10:30", "--task", "545", "--user", "1"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 5)
		wantRequests(t, srv, 0)
	}

	// the flag help names the form
	app, _ := apiApp(t)
	cmd, _, err := NewRootCmd(app).Find([]string{"time", "create"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"started-at-time", "ended-at-time"} {
		if u := cmd.Flags().Lookup(flag).Usage; !strings.Contains(u, "(HH:MM)") {
			t.Errorf("--%s usage = %q", flag, u)
		}
	}
}

func TestTimeUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "update", "272", "--ended-at-time", "12:30", "--unbillable=false", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/tracker/time_entries/272", `{"ended_at_time":"12:30","unbillable":false}`)

	// describe | update --input -: the response timestamps (started_at,
	// ended_at) are response fields and removed, never sent (R16)
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "describe", "272", "-o", "json"}), 0)
	described := mustOut(app)
	wantRequests(t, srv, 1)
	app, srv = apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, Run(app, []string{"time", "update", "272", "--input", "-", "--started-at-time", "10:20", "-o", "json"}), 0)
	raw := string(wantRequests(t, srv, 1)[0].Body)
	for _, bad := range []string{"started_at\"", "ended_at\"", "T10:00:00"} {
		if strings.Contains(raw, bad) {
			t.Errorf("body enthält %s: %s", bad, raw)
		}
	}
	if !strings.Contains(raw, `"started_at_time":"10:20"`) {
		t.Errorf("body = %s", raw)
	}

	for _, tc := range []struct {
		args []string
		exit int
	}{
		{[]string{"time", "update", "272"}, 2},
		{[]string{"time", "update", "272", "--started-at-time", "9:00"}, 5},
		{[]string{"time", "update", "272", "--entry-date", "2026-11-31"}, 5},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), tc.exit)
		wantRequests(t, srv, 0)
	}
}

func TestTimeDeleteNeedsConfirmation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "delete", "272"}), 8)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"time", "delete", "272", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/tracker/time_entries/272" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 272,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

// archive is not destructive (spec 8.1): no confirmation, also without TTY.
func TestTimeArchiveWithoutConfirmation(t *testing.T) {
	for _, action := range []string{"archive", "unarchive"} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{"time", action, "272", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/tracker/time_entries/272/"+action {
			t.Errorf("%s: request = %s %s", action, r.Method, r.Path)
		}
		if out := mustOut(app); !strings.Contains(out, `"type": "time_entry"`) {
			t.Errorf("%s: stdout = %s", action, out)
		}
	}
}

// timeMutations are the mutating time commands with valid arguments, and
// what their dry run shows.
var timeMutations = []struct {
	args          []string
	method, path  string
	credits, body string
}{
	{[]string{"time", "create", "--entry-date", "2026-11-04", "--started-at-time", "09:00", "--ended-at-time", "10:30",
		"--task", "545", "--user", "1"}, "POST", "/tracker/time_entries", `"credits": 10`, `"started_at_time": "09:00"`},
	{[]string{"time", "update", "272", "--comments", "Review"}, "PUT", "/tracker/time_entries/272", `"credits": 1`, `"comments": "Review"`},
	{[]string{"time", "delete", "272"}, "DELETE", "/tracker/time_entries/272", `"credits": 1`, `"body": null`},
	{[]string{"time", "archive", "272"}, "POST", "/tracker/time_entries/272/archive", `"credits": 1`, `"body": null`},
	{[]string{"time", "unarchive", "272"}, "POST", "/tracker/time_entries/272/unarchive", `"credits": 1`, `"body": null`},
}

// The registered mutations run the protection layer (Task 24).
func TestTimeMutationsDryRun(t *testing.T) {
	for _, tc := range timeMutations {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(tc.args, "--dry-run", "-o", "json")), 0)
		wantRequests(t, srv, 0)
		out := mustOut(app)
		for _, want := range []string{`"dry_run": true`, `"method": "` + tc.method + `"`, tc.path + `"`, tc.credits, tc.body} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: stdout ohne %q:\n%s", tc.args, want, out)
			}
		}
	}
}

func TestTimeMutationsReadOnly(t *testing.T) {
	ro := map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"}
	for _, tc := range timeMutations {
		for _, extra := range [][]string{{"--yes"}, {"--dry-run"}} {
			app, srv := mockApp(t, ro)
			wantExit(t, app, Run(app, append(tc.args, extra...)), 9)
			wantRequests(t, srv, 0)
		}
	}
	app, srv := mockApp(t, ro)
	wantExit(t, app, Run(app, []string{"time", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
}

// Spec 9.8: time list filters by the preset project unless --project is
// given; time create has no preset (time log, a later task, has).
func TestTimeListUsesProjectFile(t *testing.T) {
	app, srv := apiApp(t)
	path := writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"time", "list", "-o", "json"}), 0)
	want := url.Values{"project_id": {"34"}, "page_size": {"25"}}
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != want.Encode() {
		t.Errorf("query = %s", r.Query.Encode())
	}
	if _, errs := bufs(app); errs != "Hinweis: Projekt 34 aus Projektdatei "+path+"\n" {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"time", "list", "--project", "5", "-o", "json"}), 0)
	want = url.Values{"project_id": {"5"}, "page_size": {"25"}}
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != want.Encode() {
		t.Errorf("--project: query = %s", r.Query.Encode())
	}
	if _, errs := bufs(app); errs != "" {
		t.Errorf("--project: stderr = %q", errs)
	}

	// an invalid preset is a usage error before any request
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "x"})
	wantExit(t, app, Run(app, []string{"time", "list"}), 2)
	wantRequests(t, srv, 0)

	// time create neither reads nor needs the project file
	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\ntask = 545\nuser = 1\n")
	wantExit(t, app, Run(app, []string{"time", "create", "--entry-date", "2026-11-04", "--started-at-time", "09:00",
		"--ended-at-time", "10:30"}), 5)
	wantRequests(t, srv, 0)
}

// The three sources of the project preset produce the same request: the
// flag, PAPIERKRAM_PROJECT and the project file differ only in the hint.
func TestTimeListProjectSourcesSendSameQuery(t *testing.T) {
	type source struct {
		name string
		env  map[string]string
		file string
		args []string
	}
	var queries []string
	for _, s := range []source{
		{"flag", nil, "", []string{"--project", "34"}},
		{"env", map[string]string{"PAPIERKRAM_PROJECT": "34"}, "", nil},
		{"projectfile", nil, "project = 34\n", nil},
	} {
		env := map[string]string{"PAPIERKRAM_TOKEN": "test-token-p"}
		for k, v := range s.env {
			env[k] = v
		}
		app, srv := mockApp(t, env)
		if s.file != "" {
			writeProjectFile(t, app, s.file)
		}
		wantExit(t, app, Run(app, append([]string{"time", "list", "--state", "billable", "-o", "json"}, s.args...)), 0)
		r := wantRequests(t, srv, 1)[0]
		queries = append(queries, r.Method+" "+r.Path+"?"+r.Query.Encode())
	}
	want := "GET /tracker/time_entries?billing_state=billable&page_size=25&project_id=34"
	for i, q := range queries {
		if q != want {
			t.Errorf("source %d: %s, want %s", i, q, want)
		}
	}
}

// Help and check of an enum filter name the same values: every value the
// flag help offers is accepted (filterEnum is their one source).
func TestEnumFilterHelpValuesAreAccepted(t *testing.T) {
	n := 0
	for _, e := range registry.Entries() {
		for _, flt := range e.Filters {
			if flt.Kind != "enum" {
				continue
			}
			app, _ := apiApp(t)
			cmd, _, err := NewRootCmd(app).Find([]string{e.Resource, e.Action})
			if err != nil {
				t.Fatal(err)
			}
			usage := cmd.Flags().Lookup(flt.Flag).Usage
			_, list, ok := strings.Cut(usage, "(erlaubt: ")
			if !ok {
				t.Errorf("%s %s --%s: Hilfe ohne Werte: %q", e.Resource, e.Action, flt.Flag, usage)
				continue
			}
			list, _, _ = strings.Cut(list, ")")
			for _, v := range strings.Split(list, ", ") {
				n++
				app, srv := apiApp(t)
				wantExit(t, app, Run(app, []string{e.Resource, e.Action, "--" + flt.Flag, v, "-o", "json"}), 0)
				if q := wantRequests(t, srv, 1)[0].Query; q.Get(flt.Param) != v {
					t.Errorf("%s %s --%s %s: query = %v", e.Resource, e.Action, flt.Flag, v, q)
				}
			}
		}
	}
	if n == 0 {
		t.Error("keine Enum-Filter geprüft")
	}
}
