package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// time log (spec 9.6): a custom command of the time group that creates a
// time entry from a date, start/end or duration, task and user.

// logApp is a mock app in Europe/Berlin whose clock shows now; the
// working directory has no project file.
func logApp(t *testing.T, now time.Time) (*App, *mockapi.Server) {
	t.Helper()
	app, srv := berlinApp(t)
	app.Now = func() time.Time { return now }
	return app, srv
}

// berlinAt is a wall-clock time in Europe/Berlin.
func berlinAt(t *testing.T, y int, m time.Month, d, h, mi, s int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return time.Date(y, m, d, h, mi, s, 0, loc)
}

// morning is a working day at 10:47:35 in Berlin.
func morning(t *testing.T) time.Time { return berlinAt(t, 2026, 11, 4, 10, 47, 35) }

const timeEntries = "/tracker/time_entries"

func TestR16_TimeEntryBodyHHMM(t *testing.T) {
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", "2026-11-03",
		"--from", "9:05", "--to", "10:30", "-o", "json"}), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-03","started_at_time":"09:05","ended_at_time":"10:30","task":{"id":545},"user":{"id":1}}`)
	body := string(srv.Requests()[0].Body)
	if strings.Contains(body, "T09") || strings.Contains(body, "+01:00") {
		t.Errorf("ISO timestamp in body: %s", body)
	}
	assertNoToken(t, app)
}

func TestR15_TimeLogWithoutTaskUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"both", nil, []string{"Aufgabe und Benutzer", "--task <id> --user <id>"}},
		{"task", []string{"--user", "1"}, []string{"Aufgabe", "PAPIERKRAM_TASK", "--task <id>"}},
		{"user", []string{"--task", "545"}, []string{"Benutzer", "PAPIERKRAM_USER", "--user <id>"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			args := append([]string{"time", "log", "--from", "09:00", "--to", "10:00"}, tc.args...)
			wantExit(t, app, Run(app, args), 2)
			wantRequests(t, srv, 0)
			_, errs := bufs(app)
			for _, w := range append(tc.want, "papierkram local init") {
				if !strings.Contains(errs, w) {
					t.Errorf("stderr ohne %q: %s", w, errs)
				}
			}
		})
	}
}

// --duration without --from ends now (minute precision) on today's date.
func TestTimeLogDurationEndsNow(t *testing.T) {
	for _, d := range []string{"1h30m", "90m", "1:30"} {
		t.Run(d, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--duration", d, "-o", "json"}), 0)
			wantBody(t, srv, "POST", timeEntries,
				`{"entry_date":"2026-11-04","started_at_time":"09:17","ended_at_time":"10:47","task":{"id":545},"user":{"id":1}}`)
		})
	}
	// with --from it starts there
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", "gestern",
		"--from", "13:00", "--duration", "45m", "-m", "Abstimmung", "--unbillable", "-o", "json"}), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-03","started_at_time":"13:00","ended_at_time":"13:45","task":{"id":545},"user":{"id":1},"comments":"Abstimmung","unbillable":true}`)
}

// The body knows one date: an entry over midnight, or one that ends at or
// before its start, is a usage error before anything is sent. The
// message fits the case (review M6): an end before the start names
// midnight only as one possibility, an end equal to the start is a
// duration of 0.
func TestTimeLogMidnightRejected(t *testing.T) {
	for _, tc := range []struct {
		name      string
		now       time.Time
		args      []string
		want, not string
	}{
		{"from+duration", morning(t), []string{"--from", "23:30", "--duration", "1h"}, "endet am Folgetag", ""},
		{"to before from", morning(t), []string{"--from", "22:00", "--to", "01:00"}, "Ende 01:00 liegt vor dem Beginn 22:00", ""},
		{"to equals from", morning(t), []string{"--from", "10:00", "--to", "10:00"}, "Dauer 0", "Mitternacht"},
		{"duration before midnight", berlinAt(t, 2026, 11, 4, 0, 30, 0), []string{"--duration", "2h"}, "beginnt am Vortag", ""},
		{"exactly midnight", morning(t), []string{"--from", "23:00", "--duration", "1h"}, "endet am Folgetag", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.now)
			wantExit(t, app, Run(app, append([]string{"time", "log", "--task", "545", "--user", "1"}, tc.args...)), 2)
			wantRequests(t, srv, 0)
			_, errs := bufs(app)
			if !strings.Contains(errs, tc.want) || (tc.not == "" && !strings.Contains(errs, "Mitternacht")) {
				t.Errorf("stderr ohne %q: %s", tc.want, errs)
			}
			if tc.not != "" && strings.Contains(errs, tc.not) {
				t.Errorf("stderr mit %q: %s", tc.not, errs)
			}
		})
	}
	// ending at 23:59 is fine
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--from", "23:00", "--duration", "59m"}), 0)
	if r := wantRequests(t, srv, 1)[0]; !strings.Contains(string(r.Body), `"ended_at_time":"23:59"`) {
		t.Errorf("body = %s", r.Body)
	}
}

// Review Focus 4, controller ruling: --duration is the booked duration in
// wall-clock arithmetic. Papierkram stores HH:MM without offset, so the
// booked duration is the wall-clock difference, also on the days Berlin
// leaves (2026-10-25, 03:00 → 02:00) and enters (2026-03-29,
// 02:00 → 03:00) daylight saving time. --from/--to stay as given.
func TestTimeLogDurationAcrossDSTChange(t *testing.T) {
	for _, tc := range []struct {
		name, date, from, dur, wantFrom, wantTo string
	}{
		{"autumn", "2026-10-25", "01:30", "2h", "01:30", "03:30"},
		{"spring", "2026-03-29", "01:30", "2h", "01:30", "03:30"},
		{"normal day", "2026-10-26", "01:30", "2h", "01:30", "03:30"},
		{"autumn repeated hour", "2026-10-25", "02:30", "30m", "02:30", "03:00"},
		{"spring skipped hour", "2026-03-29", "02:30", "1h", "02:30", "03:30"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", tc.date,
				"--from", tc.from, "--duration", tc.dur, "-o", "json"}), 0)
			wantBody(t, srv, "POST", timeEntries, `{"entry_date":"`+tc.date+`","started_at_time":"`+tc.wantFrom+
				`","ended_at_time":"`+tc.wantTo+`","task":{"id":545},"user":{"id":1}}`)
		})
	}
	// --duration without --from on a change day: the end is the wall clock
	// of now, the start that minus the duration in wall-clock minutes
	for _, tc := range []struct {
		name     string
		now      time.Time
		date     string
		from, to string
	}{
		{"autumn", time.Date(2026, 10, 25, 1, 10, 0, 0, time.UTC), "2026-10-25", "00:40", "02:10"}, // 02:10 CET
		{"spring", time.Date(2026, 3, 29, 1, 10, 0, 0, time.UTC), "2026-03-29", "01:40", "03:10"},  // 03:10 CEST
	} {
		t.Run("ends now "+tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.now)
			wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--duration", "1h30m", "-o", "json"}), 0)
			wantBody(t, srv, "POST", timeEntries, `{"entry_date":"`+tc.date+`","started_at_time":"`+tc.from+
				`","ended_at_time":"`+tc.to+`","task":{"id":545},"user":{"id":1}}`)
		})
	}
}

// Flag combinations and formats are usage errors before anything is
// resolved or sent.
func TestTimeLogFlagErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"neither", nil, "--from und --to oder --duration"},
		{"only from", []string{"--from", "09:00"}, "--to"},
		{"only to", []string{"--to", "09:00"}, "--from"},
		{"to with duration", []string{"--to", "10:00", "--duration", "1h"}, "--duration"},
		{"all three", []string{"--from", "09:00", "--to", "10:00", "--duration", "1h"}, "--duration"},
		{"from without colon", []string{"--from", "0900", "--to", "10:00"}, "--from"},
		{"hour 24", []string{"--from", "24:00", "--to", "10:00"}, "HH:MM"},
		{"one minute digit", []string{"--from", "9:5", "--to", "10:00"}, "HH:MM"},
		{"iso", []string{"--from", "2026-11-04T09:00:00+01:00", "--to", "10:00"}, "HH:MM"},
		{"zero duration", []string{"--duration", "0m"}, "--duration"},
		{"negative duration", []string{"--duration", "-1h"}, "--duration"},
		{"duration without unit", []string{"--duration", "90"}, "--duration"},
		{"duration seconds", []string{"--duration", "90s"}, "Minuten"},
		{"duration minutes over 59", []string{"--duration", "1:75"}, "--duration"},
		{"duration garbage", []string{"--duration", "lang"}, "--duration"},
		{"duration other day", []string{"--date", "gestern", "--duration", "1h"}, "--from"},
		{"bad task", []string{"--task", "-3", "--from", "09:00", "--to", "10:00"}, "--task"},
		{"argument", []string{"545", "--from", "09:00", "--to", "10:00"}, "unerwartetes Argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			wantExit(t, app, Run(app, append([]string{"time", "log", "--user", "1"}, tc.args...)), 2)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
				t.Errorf("stderr ohne %q: %s", tc.want, errs)
			}
		})
	}
}

// --date goes through the date conversion of the create flags: an invalid
// date is a validation error (exit 5) like --entry-date, after the
// read-only gate.
func TestTimeLogInvalidDate(t *testing.T) {
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", "04.11.2026",
		"--from", "09:00", "--to", "10:00"}), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--date") || !strings.Contains(errs, "YYYY-MM-DD") {
		t.Errorf("stderr = %s", errs)
	}

	app, srv = logApp(t, morning(t))
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--date", "04.11.2026",
		"--from", "09:00", "--to", "10:00"}), 9)
	wantRequests(t, srv, 0)
}

// Carry N2 (Task 29): the task from the flag, the environment and the
// project file gives the same body.
func TestTimeLogTaskSourcesGiveSameBody(t *testing.T) {
	want := `{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:00","task":{"id":57},"user":{"id":1}}`
	args := []string{"time", "log", "--user", "1", "--from", "09:00", "--to", "10:00", "-o", "json"}
	for _, tc := range []struct {
		name, file, hint string
		env              map[string]string
		extra            []string
	}{
		{name: "flag", extra: []string{"--task", "57"}},
		{name: "env", env: map[string]string{"PAPIERKRAM_TASK": "57"}, hint: "Aufgabe 57 aus PAPIERKRAM_TASK"},
		{name: "project file", file: "task = 57\n", hint: "Aufgabe 57 aus Projektdatei"},
		{name: "alias", file: "[tasks]\ntickets = 57\n", extra: []string{"--task", "tickets"}},
		{name: "env alias", file: "[tasks]\ntickets = 57\n", env: map[string]string{"PAPIERKRAM_TASK": "tickets"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			env := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log"}
			for k, v := range tc.env {
				env[k] = v
			}
			app.Env = envMap(env)
			if tc.file != "" {
				writeProjectFile(t, app, tc.file)
			}
			wantExit(t, app, Run(app, append(args, tc.extra...)), 0)
			wantBody(t, srv, "POST", timeEntries, want)
			if _, errs := bufs(app); tc.hint != "" && !strings.Contains(errs, tc.hint) {
				t.Errorf("stderr ohne %q: %s", tc.hint, errs)
			}
		})
	}
}

// task and user: flag > environment > project file > context (user only).
func TestTimeLogPresetOrder(t *testing.T) {
	const cfg = "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\nuser = 7\n"
	run := func(t *testing.T, env map[string]string, file string, extra ...string) string {
		t.Helper()
		app, srv := logApp(t, morning(t))
		all := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()}
		for k, v := range env {
			all[k] = v
		}
		app.Env = envMap(all)
		writeCfg(t, app, cfg)
		if err := app.Keyring.Set("a", "test-token-a"); err != nil {
			t.Fatal(err)
		}
		if file != "" {
			writeProjectFile(t, app, file)
		}
		wantExit(t, app, Run(app, append([]string{"time", "log", "--from", "09:00", "--to", "10:00", "-o", "json"}, extra...)), 0)
		r := wantRequests(t, srv, 1)[0]
		b := decodeBody(t, r.Body)
		return canonJSON(t, mustJSON(t, map[string]any{"task": b["task"], "user": b["user"]}))
	}
	file := "task = 30\nuser = 3\n"
	env := map[string]string{"PAPIERKRAM_TASK": "20", "PAPIERKRAM_USER": "2"}
	for _, tc := range []struct {
		name  string
		env   map[string]string
		file  string
		extra []string
		want  string
	}{
		{"flag", env, file, []string{"--task", "10", "--user", "1"}, `{"task":{"id":10},"user":{"id":1}}`},
		{"env", env, file, nil, `{"task":{"id":20},"user":{"id":2}}`},
		{"project file", nil, file, nil, `{"task":{"id":30},"user":{"id":3}}`},
		{"context user", nil, "task = 30\n", nil, `{"task":{"id":30},"user":{"id":7}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, tc.env, tc.file, tc.extra...); got != tc.want {
				t.Errorf("task/user = %s, want %s", got, tc.want)
			}
		})
	}
}

// Protection layer (Task 24): --dry-run sends nothing, the hard read-only
// mode blocks before anything else is resolved, class create.
func TestTimeLogProtection(t *testing.T) {
	args := []string{"time", "log", "--task", "545", "--user", "1", "--from", "09:00", "--to", "10:30"}
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, append(args, "--dry-run", "-o", "json")), 0)
	wantRequests(t, srv, 0)
	out, _ := bufs(app)
	if got := canonJSON(t, []byte(out)); !strings.Contains(got, `"dry_run":true`) || !strings.Contains(got, `"credits":10`) ||
		!strings.Contains(got, `"method":"POST"`) || !strings.Contains(got, `"ended_at_time":"10:30"`) {
		t.Errorf("dry run = %s", got)
	}

	// read-only before the missing task is noticed
	app, srv = logApp(t, morning(t))
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"time", "log", "--from", "09:00", "--to", "10:30"}), 9)
	wantRequests(t, srv, 0)

	app, _ = logApp(t, morning(t))
	cmd, _, err := NewRootCmd(app).Find([]string{"time", "log"})
	if err != nil || cmd.Name() != "log" || cmd.Annotations[annotClass] != "create" {
		t.Errorf("time log: %v, annotations %v", err, cmd.Annotations)
	}
}

// Without a terminal the entry is sent without asking (class create);
// with one, a summary with the resolved names is shown and Enter
// confirms. Every name costs one request, cached for 60 seconds.
func TestTimeLogConfirmationInTerminal(t *testing.T) {
	args := []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:30", "-m", "Abstimmung", "-o", "json"}
	paths := func(srv *mockapi.Server) []string {
		var out []string
		for _, r := range srv.Requests() {
			out = append(out, r.Method+" "+r.Path)
		}
		return out
	}
	lookups := "GET /tracker/tasks/492,GET /contact/companies/3352,GET /users/1,POST /tracker/time_entries"

	app, srv := logApp(t, morning(t))
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, args), 0)
	if got := strings.Join(paths(srv), ","); got != lookups {
		t.Errorf("requests = %s", got)
	}
	_, errs := bufs(app)
	for _, w := range []string{"Some important task (492)", "Important project (4835)", "ACME Corp. (3352)", "Will E Coyote (1)",
		"2026-11-04", "09:00–10:30 (1:30 h)", "Abstimmung", "Zeiteintrag anlegen? [J/n]"} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %s", w, errs)
		}
	}

	// the names are cached per account (context and base URL, so the same
	// mock): a second run within 60 s only sends
	again := func(at time.Time, answer string) string {
		t.Helper()
		before := len(srv.Requests())
		app.Now = func() time.Time { return at }
		app.In = strings.NewReader(answer)
		app.Err.(*bytes.Buffer).Reset()
		wantExit(t, app, Run(app, args), 0)
		return strings.Join(paths(srv)[before:], ",")
	}
	if got := again(morning(t).Add(59*time.Second), "ja\n"); got != "POST /tracker/time_entries" {
		t.Errorf("cached run: requests = %s", got)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Some important task (492)") || !strings.Contains(errs, "ACME Corp. (3352)") {
		t.Errorf("cached run: stderr = %s", errs)
	}
	// after the TTL they are loaded again
	if got := again(morning(t).Add(61*time.Second), "\n"); got != lookups {
		t.Errorf("expired cache: requests = %s", got)
	}
	// another account (other base URL) does not share the names
	_, other := logApp(t, morning(t))
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": other.URL(), "PAPIERKRAM_TOKEN": "test-token-log"})
	app.Now = func() time.Time { return morning(t).Add(62 * time.Second) }
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, args), 0)
	if got := strings.Join(paths(other), ","); got != lookups {
		t.Errorf("other account: requests = %s", got)
	}

	// declining sends nothing
	app, srv = logApp(t, morning(t))
	app.StdinTTY = true
	app.In = strings.NewReader("n\n")
	wantExit(t, app, Run(app, args), 8)
	if got := strings.Join(paths(srv), ","); strings.Contains(got, "POST") {
		t.Errorf("declined: requests = %s", got)
	}

	// --yes, --no-input and no terminal: no lookup, no question
	for _, tc := range []struct {
		name  string
		tty   bool
		extra []string
	}{{"yes", true, []string{"--yes"}}, {"no-input", true, []string{"--no-input"}}, {"no terminal", false, nil}} {
		app, srv = logApp(t, morning(t))
		app.StdinTTY = tc.tty
		wantExit(t, app, Run(app, append(args, tc.extra...)), 0)
		if got := strings.Join(paths(srv), ","); got != "POST /tracker/time_entries" {
			t.Errorf("%s: requests = %s", tc.name, got)
		}
		if _, errs := bufs(app); strings.Contains(errs, "[J/n]") {
			t.Errorf("%s: asked: %s", tc.name, errs)
		}
	}
}

// A failed name lookup shows the ID without name and the entry is still
// sent after the confirmation.
func TestTimeLogLookupFailureShowsID(t *testing.T) {
	app, srv := logApp(t, morning(t))
	srv.On("GET", "/tracker/tasks/{id}", http.StatusNotFound, []byte(`{"error":"not found"}`))
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:30", "-o", "json"}), 0)
	reqs := srv.Requests()
	if last := reqs[len(reqs)-1]; last.Method != "POST" {
		t.Errorf("last request = %s %s", last.Method, last.Path)
	}
	_, errs := bufs(app)
	for _, w := range []string{"Aufgabe:", "492", "Will E Coyote (1)", "Warnung:"} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %s", w, errs)
		}
	}
	if strings.Contains(errs, "Projekt:") || strings.Contains(errs, "Firma:") {
		t.Errorf("project/company without task: %s", errs)
	}
}

// The output is the created time entry like time create.
func TestTimeLogOutputIsCreatedEntry(t *testing.T) {
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "545", "--user", "1", "--from", "09:00", "--to", "10:30", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	out, _ := bufs(app)
	if !strings.Contains(out, `"id":`) {
		t.Errorf("stdout = %s", out)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The cache directory follows the config directory: App.CacheDir is the
// plain user cache directory, PAPIERKRAM_CACHE_DIR overrides it, and the
// cache appends papierkram/.
func TestCacheDirMirrorsConfigDir(t *testing.T) {
	if filepath.Base(NewApp().CacheDir) == "papierkram" {
		t.Errorf("NewApp().CacheDir = %q, want the plain user cache directory", NewApp().CacheDir)
	}
	app, _, _ := newTestApp(t)
	app.CacheDir = "/base"
	if got := app.cacheDir(); got != "/base" {
		t.Errorf("cacheDir() = %q", got)
	}
	app.Env = envMap(map[string]string{"PAPIERKRAM_CACHE_DIR": "/env"})
	if got := app.cacheDir(); got != "/env" {
		t.Errorf("cacheDir() with PAPIERKRAM_CACHE_DIR = %q", got)
	}

	// a TTY run stores the names below <cache dir>/papierkram/
	app, _ = logApp(t, morning(t))
	dir := t.TempDir()
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": app.Env("PAPIERKRAM_BASE_URL"), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_CACHE_DIR": dir})
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00"}), 0)
	if files, _ := filepath.Glob(filepath.Join(dir, "papierkram", "names", "*.json")); len(files) == 0 {
		t.Error("no cache files below PAPIERKRAM_CACHE_DIR/papierkram")
	}
}

// A corrupt or unwritable cache never fails time log.
func TestTimeLogCacheFailureIsHarmless(t *testing.T) {
	app, srv := logApp(t, morning(t))
	if err := os.WriteFile(filepath.Join(app.CacheDir, "papierkram"), []byte("kein Verzeichnis"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00"}), 0)
	if n := len(srv.Requests()); n != 4 {
		t.Errorf("%d requests, want 3 lookups and the POST", n)
	}
}

// --unbillable is a switch like the bool field flags of time create:
// --unbillable=nein sends false, an invalid value is exit 5.
func TestTimeLogUnbillableLikeCreate(t *testing.T) {
	base := []string{"time", "log", "--task", "545", "--user", "1", "--from", "09:00", "--to", "10:00", "-o", "json"}
	app, srv := logApp(t, morning(t))
	wantExit(t, app, Run(app, append(base, "--unbillable=nein")), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:00","task":{"id":545},"user":{"id":1},"unbillable":false}`)
	app, srv = logApp(t, morning(t))
	wantExit(t, app, Run(app, append(base, "--unbillable=vielleicht")), 5)
	wantRequests(t, srv, 0)
}

// Review M2: end of input (Ctrl-D) at a confirmation is "no" (exit 8) on
// every command that asks, also where Enter means yes (time log); nothing
// is changed.
func TestConfirmationEOFIsNo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		setup func(*mockapi.Server)
		// mutating is the method that must not be sent
		mutating string
	}{
		{"time log", []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00"}, nil, "POST"},
		{"delete", []string{"invoice", "delete", "459"}, nil, "DELETE"},
		{"pay", []string{"voucher", "pay", "1022", "--amount", "10"}, nil, "POST"},
		{"deliver", []string{"invoice", "deliver", "459", "--via", "pdf"}, nil, "POST"},
		{"list --all", []string{"user", "list", "--all", "-o", "json"}, func(s *mockapi.Server) { pagedUsers(s, 10) }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, morning(t))
			if tc.setup != nil {
				tc.setup(srv)
			}
			app.StdinTTY = true
			app.In = strings.NewReader("") // Ctrl-D at once
			wantExit(t, app, Run(app, tc.args), 8)
			for _, r := range srv.Requests() {
				if tc.mutating != "" && r.Method == tc.mutating {
					t.Errorf("sent %s %s", r.Method, r.Path)
				}
			}
			if tc.mutating == "" && len(srv.Requests()) != 1 {
				t.Errorf("%d pages loaded, want 1", len(srv.Requests()))
			}
		})
	}
	// a typed answer without line end still counts
	app, srv := logApp(t, morning(t))
	app.StdinTTY = true
	app.In = strings.NewReader("ja")
	wantExit(t, app, Run(app, []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00"}), 0)
	if reqs := srv.Requests(); reqs[len(reqs)-1].Method != "POST" {
		t.Errorf("not sent: %+v", reqs)
	}
}

// Review M7: (a) a project embedded in the task without company_id is not
// cached as known, so the project is loaded for the company; (b) an
// answer that is no JSON object is a warning like a failed request.
func TestTimeLogNameLookupRobust(t *testing.T) {
	args := []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00", "-o", "json"}
	app, srv := logApp(t, morning(t))
	srv.On("GET", "/tracker/tasks/{id}", http.StatusOK, []byte(`{"id":492,"name":"Aufgabe X","project":{"id":4835,"name":"Projekt Y"}}`))
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, args), 0)
	var got []string
	for _, r := range srv.Requests() {
		got = append(got, r.Method+" "+r.Path)
	}
	if len(got) != 5 || got[1] != "GET /projects/4835" || !strings.HasPrefix(got[2], "GET /contact/companies/") || got[4] != "POST /tracker/time_entries" {
		t.Errorf("requests = %v", got)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Firma:") || !strings.Contains(errs, "Aufgabe X (492)") {
		t.Errorf("stderr = %s", errs)
	}

	app, srv = logApp(t, morning(t))
	srv.On("GET", "/users/{id}", http.StatusOK, []byte(`kein JSON`))
	app.StdinTTY = true
	app.In = strings.NewReader("\n")
	wantExit(t, app, Run(app, args), 0)
	if _, errs := bufs(app); !strings.Contains(errs, "Warnung: Name von Benutzer 1 nicht geladen") {
		t.Errorf("stderr = %s", errs)
	}
}
