package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
	"github.com/Padrio/papierkram-cli/internal/timer"
)

// The local timer (spec 9.6): time start saves start, task, user and
// comment per context, time status shows it, time stop books it through
// the time log path and deletes it only after the entry was created,
// time discard drops it. Only time stop sends a request.

// at runs args with the app clock at now, on fresh output buffers.
func at(app *App, now time.Time, args ...string) int {
	app.Now = func() time.Time { return now }
	app.Out.(*bytes.Buffer).Reset()
	app.Err.(*bytes.Buffer).Reset()
	return Run(app, args)
}

// timerFile is the content of the state file of ctx, nil without one.
func timerFile(t *testing.T, app *App, ctx string) []byte {
	t.Helper()
	data, err := os.ReadFile(timer.Path(app.CacheDir, ctx))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// writeTimerFile replaces the state file of ctx with content.
func writeTimerFile(t *testing.T, app *App, ctx, content string) {
	t.Helper()
	p := timer.Path(app.CacheDir, ctx)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func wantStderr(t *testing.T, app *App, want ...string) {
	t.Helper()
	_, errs := bufs(app)
	for _, w := range want {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %s", w, errs)
		}
	}
}

// nine is 09:00:12 on the day of morning (10:47:35).
func nine(t *testing.T) time.Time { return berlinAt(t, 2026, 11, 4, 9, 0, 12) }

func TestTimerStartStatusStopFlow(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "-m", "Abstimmung"), 0)
	wantRequests(t, srv, 0)
	state := timerFile(t, app, "")
	if state == nil {
		t.Fatal("no state file")
	}
	if fi, _ := os.Stat(timer.Path(app.CacheDir, "")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	if out, _ := bufs(app); !strings.Contains(out, "Timer gestartet") || !strings.Contains(out, "09:00") {
		t.Errorf("stdout = %s", out)
	}

	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
	wantRequests(t, srv, 0)
	out, _ := bufs(app)
	want := `{"account":"` + strings.TrimPrefix(strings.SplitN(srv.URL(), "/api", 2)[0], "http://") + `",` +
		`"comment":"Abstimmung","context":"","duration":"1:47","duration_minutes":107,"running":true,` +
		`"started_at":"2026-11-04T09:00:12+01:00","task":545,"user":1}`
	if got := canonJSON(t, []byte(out)); got != want {
		t.Errorf("status = %s\nwant     %s", got, want)
	}

	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":545},"user":{"id":1},"comments":"Abstimmung"}`)
	if timerFile(t, app, "") != nil {
		t.Error("state file left after stop")
	}
	if out, _ := bufs(app); !strings.Contains(out, `"id":`) {
		t.Errorf("stop output is no time entry: %s", out)
	}
	wantStderr(t, app, "Timer gestoppt", "09:00–10:47")

	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"running":false}` {
		t.Errorf("status after stop = %s", out)
	}
	wantRequests(t, srv, 1)
	assertNoToken(t, app)
}

// time status in a table: duration, start, task, comment; without a
// timer a German line. State content is made printable in every format.
func TestTimerStatusOutput(t *testing.T) {
	app, _ := logApp(t, nine(t))
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	if out, _ := bufs(app); !strings.Contains(out, "Kein Timer läuft") {
		t.Errorf("stdout = %s", out)
	}
	writeTimerFile(t, app, "", `{"context":"","started_at":"2026-11-04T09:00:12+01:00","task":545,"user":1,"comment":"a\u001b[2Jb\u202ec"}`)
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	out, _ := bufs(app)
	for _, w := range []string{"1:47", "2026-11-04 09:00", "545", "a [2Jb c"} {
		if !strings.Contains(out, w) {
			t.Errorf("table ohne %q: %s", w, out)
		}
	}
	for _, format := range []string{"table", "json", "yaml"} {
		wantExit(t, app, at(app, morning(t), "time", "status", "-o", format), 0)
		if out, _ := bufs(app); strings.ContainsAny(out, "\x1b\u202e") || strings.Contains(out, `\u001b`) || strings.Contains(out, `\u202e`) {
			t.Errorf("%s: unprintable comment: %q", format, out)
		}
	}
	// the context output default applies, like local show
	app, srv := logApp(t, nine(t))
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\noutput = \"json\"\n")
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"running":false}` {
		t.Errorf("context output json: %s", out)
	}
	wantRequests(t, srv, 0)
}

// A failed stop keeps the timer: nothing booked means nothing lost. The
// hint names the manual booking and time discard.
func TestTimerStopFailureKeepsState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		start  time.Time
		stop   time.Time
		args   []string
		tty    string // stdin in a terminal, "" without one
		setup  func(*mockapi.Server)
		exit   int
		posts  int
		stderr []string
	}{
		{name: "server error", start: nine(t), stop: morning(t), exit: 1, posts: 1,
			setup: func(s *mockapi.Server) {
				s.On("POST", timeEntries, http.StatusInternalServerError, []byte(`{"type":"error","message":"kaputt"}`))
			},
			stderr: []string{"Timer läuft weiter", "papierkram time list", "papierkram time log --task 545 --user 1 --date 2026-11-04 --from 09:00 --to 10:47", "papierkram time discard"}},
		{name: "validation", start: nine(t), stop: morning(t), exit: 5, posts: 1,
			setup: func(s *mockapi.Server) {
				s.On("POST", timeEntries, http.StatusUnprocessableEntity, []byte(`{"type":"error","message":"Aufgabe abgeschlossen"}`))
			},
			stderr: []string{"Timer läuft weiter", "papierkram time log", "papierkram time discard"}},
		{name: "outcome unknown", start: nine(t), stop: morning(t), args: []string{"--timeout", "50ms"}, exit: 12, posts: 1,
			setup: func(s *mockapi.Server) {
				s.OnFunc("POST", timeEntries, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
			},
			stderr: []string{"Ergebnis unbekannt", "Timer läuft weiter", "papierkram time list", "papierkram time discard"}},
		{name: "declined", start: nine(t), stop: morning(t), tty: "n\n", exit: 8,
			stderr: []string{"Abgebrochen", "Timer läuft weiter"}},
		{name: "end of input", start: nine(t), stop: morning(t), tty: "", exit: 8},
		{name: "dry run", start: nine(t), stop: morning(t), args: []string{"--dry-run"}, exit: 0,
			stderr: []string{"Timer läuft weiter"}},
		{name: "midnight", start: berlinAt(t, 2026, 11, 3, 22, 10, 0), stop: berlinAt(t, 2026, 11, 4, 1, 15, 0), exit: 2,
			stderr: []string{"Mitternacht", "papierkram time log --task 545 --user 1 --date 2026-11-03 --from 22:10 --to 24:00",
				"papierkram time log --task 545 --user 1 --date 2026-11-04 --from 00:00 --to 01:15", "papierkram time discard"}},
		{name: "days later", start: berlinAt(t, 2026, 11, 1, 22, 10, 0), stop: morning(t), exit: 2,
			stderr: []string{"Mitternacht", "papierkram time log", "papierkram time discard"}},
		{name: "under a minute", start: nine(t), stop: nine(t).Add(47 * time.Second), exit: 2,
			stderr: []string{"zu kurz", "Timer läuft weiter", "papierkram time discard"}},
		{name: "start in the future", start: morning(t), stop: nine(t), exit: 2,
			stderr: []string{"Zukunft", "papierkram time discard"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.start)
			if tc.setup != nil {
				tc.setup(srv)
			}
			wantExit(t, app, at(app, tc.start, "time", "start", "--task", "545", "--user", "1"), 0)
			before := timerFile(t, app, "")
			if tc.name == "declined" || tc.name == "end of input" {
				app.StdinTTY = true
				app.In = strings.NewReader(tc.tty)
			}
			wantExit(t, app, at(app, tc.stop, append([]string{"time", "stop"}, tc.args...)...), tc.exit)
			posts := 0
			for _, r := range srv.Requests() {
				if r.Method == "POST" {
					posts++
				}
			}
			if posts != tc.posts {
				t.Errorf("%d POST, want %d", posts, tc.posts)
			}
			if after := timerFile(t, app, ""); !bytes.Equal(after, before) {
				t.Errorf("state changed: %s → %s", before, after)
			}
			if claimFile(t, app, "") != nil {
				t.Error("claim left")
			}
			wantStderr(t, app, tc.stderr...)
		})
	}
}

// time stop takes the comment of the timer; -m replaces it.
func TestTimerStopCommentOverride(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "-m", "alt"), 0)
	wantExit(t, app, at(app, morning(t), "time", "stop", "-m", "neu", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":545},"user":{"id":1},"comments":"neu"}`)
}

// Parity: time stop sends the body of time log with the same period,
// task, user and comment, without and with comment.
func TestTimerStopBodyEqualsTimeLog(t *testing.T) {
	for _, comment := range [][]string{nil, {"-m", "Abstimmung mit Kunde"}} {
		stopApp, stopSrv := logApp(t, nine(t))
		wantExit(t, stopApp, at(stopApp, nine(t), append([]string{"time", "start", "--task", "545", "--user", "1"}, comment...)...), 0)
		wantExit(t, stopApp, at(stopApp, morning(t), "time", "stop", "-o", "json"), 0)
		logApp_, logSrv := logApp(t, morning(t))
		wantExit(t, logApp_, at(logApp_, morning(t), append([]string{"time", "log", "--task", "545", "--user", "1",
			"--from", "09:00", "--to", "10:47", "-o", "json"}, comment...)...), 0)
		stop, log := wantRequests(t, stopSrv, 1)[0], wantRequests(t, logSrv, 1)[0]
		if stop.Method != log.Method || stop.Path != log.Path || canonJSON(t, stop.Body) != canonJSON(t, log.Body) {
			t.Errorf("time stop %s %s %s\ntime log  %s %s %s", stop.Method, stop.Path, stop.Body, log.Method, log.Path, log.Body)
		}
		// the dry run shows the same request too
		dryApp, drySrv := logApp(t, nine(t))
		wantExit(t, dryApp, at(dryApp, nine(t), append([]string{"time", "start", "--task", "545", "--user", "1"}, comment...)...), 0)
		wantExit(t, dryApp, at(dryApp, morning(t), "time", "stop", "--dry-run", "-o", "json"), 0)
		wantRequests(t, drySrv, 0)
		var dry struct {
			Body json.RawMessage `json:"body"`
		}
		out, _ := bufs(dryApp)
		if err := json.Unmarshal([]byte(out), &dry); err != nil || canonJSON(t, dry.Body) != canonJSON(t, log.Body) {
			t.Errorf("dry run %s, want body %s (%v)", out, log.Body, err)
		}
	}
}

// A running timer is not replaced: exit 7, the state stays.
func TestTimerDoubleStartIsConflict(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	wantExit(t, app, at(app, morning(t), "time", "start", "--task", "999", "--user", "1"), 7)
	if after := timerFile(t, app, ""); !bytes.Equal(after, before) {
		t.Errorf("state changed: %s → %s", before, after)
	}
	wantStderr(t, app, "läuft bereits", "papierkram time stop", "papierkram time discard")
	wantRequests(t, srv, 0)
}

// time start resolves task and user like time log: missing values are
// exit 2 with the hint on local init, nothing is saved; presets from the
// environment or the project file are saved with their hint.
func TestTimerStartPresets(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start"), 2)
	wantStderr(t, app, "Aufgabe und Benutzer", "papierkram local init")
	if timerFile(t, app, "") != nil {
		t.Error("state saved without task and user")
	}
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "-3"), 2)
	wantStderr(t, app, "--task")

	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_USER": "2"})
	writeProjectFile(t, app, "[tasks]\ntickets = 57\n")
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "tickets"), 0)
	wantStderr(t, app, "Benutzer 2 aus PAPIERKRAM_USER")
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":57},"user":{"id":2}}`)
}

// The read-only modes block start and stop (exit 9, start fails early
// because stop would); discard is local and allowed.
func TestTimerReadOnly(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")

	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, at(app, morning(t), "time", "stop"), 9)
	if !bytes.Equal(timerFile(t, app, ""), before) {
		t.Error("state changed by a blocked stop")
	}
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes"), 0)
	if timerFile(t, app, "") != nil {
		t.Error("discard under read-only kept the state")
	}
	wantExit(t, app, at(app, morning(t), "time", "start", "--task", "545", "--user", "1"), 9)
	if timerFile(t, app, "") != nil {
		t.Error("start under read-only saved a state")
	}
	wantRequests(t, srv, 0)

	// a read-only context
	app, srv = logApp(t, nine(t))
	writeCfg(t, app, "active_context = \"ro\"\n[contexts.ro]\nsubdomain = \"firma\"\nread_only = true\n")
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log"})
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 9)
	if timerFile(t, app, "ro") != nil {
		t.Error("start in a read-only context saved a state")
	}
	wantRequests(t, srv, 0)
}

// discard drops the timer after a confirmation: in a terminal it asks
// (no by default, end of input is no), without one --yes is needed (exit
// 8). Without a timer it says so (exit 0); a corrupt state is discarded
// too, while the other commands name discard (exit 1).
func TestTimerDiscard(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, morning(t), "time", "discard"), 0)
	if out, _ := bufs(app); !strings.Contains(out, "Kein Timer läuft") {
		t.Errorf("stdout = %s", out)
	}
	wantExit(t, app, at(app, morning(t), "time", "discard", "-o", "json"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"discarded":false}` {
		t.Errorf("json = %s", out)
	}

	start := func() {
		t.Helper()
		wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	}
	start()
	for _, tc := range []struct {
		name  string
		tty   bool
		input string
		args  []string
	}{
		{"no terminal", false, "", nil},
		{"no-input", true, "ja\n", []string{"--no-input"}},
		{"declined", true, "n\n", nil},
		{"enter", true, "\n", nil},
		{"end of input", true, "", nil},
	} {
		app.StdinTTY, app.In = tc.tty, strings.NewReader(tc.input)
		wantExit(t, app, at(app, morning(t), append([]string{"time", "discard"}, tc.args...)...), 8)
		if timerFile(t, app, "") == nil {
			t.Fatalf("%s: state removed", tc.name)
		}
	}
	app.StdinTTY, app.In = true, strings.NewReader("ja\n")
	wantExit(t, app, at(app, morning(t), "time", "discard"), 0)
	wantStderr(t, app, "1:47", "[j/N]")
	if timerFile(t, app, "") != nil {
		t.Error("state kept after ja")
	}
	start()
	app.StdinTTY = false
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes", "-o", "json"), 0)
	if out, _ := bufs(app); !strings.Contains(out, `"discarded": true`) {
		t.Errorf("json = %s", out)
	}
	if timerFile(t, app, "") != nil {
		t.Error("state kept after --yes")
	}

	// a corrupt state
	writeTimerFile(t, app, "", "kein JSON")
	for _, args := range [][]string{{"time", "status"}, {"time", "start", "--task", "1", "--user", "1"}, {"time", "stop"}} {
		wantExit(t, app, at(app, morning(t), args...), 1)
		wantStderr(t, app, "ungültig", "papierkram time discard")
		if string(timerFile(t, app, "")) != "kein JSON" {
			t.Fatalf("%v changed the corrupt state", args)
		}
	}
	wantExit(t, app, at(app, morning(t), "time", "discard"), 8)
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes"), 0)
	if timerFile(t, app, "") != nil {
		t.Error("corrupt state kept")
	}
	// a directory in its place cannot be removed when it has content: exit 1
	writeTimerFile(t, app, "", "")
	p := timer.Path(app.CacheDir, "")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes"), 1)
	wantRequests(t, srv, 0)
}

// Every context has its own timer; direct mode (no context) one of its
// own, whose file name no context can have.
func TestTimerContextIsolation(t *testing.T) {
	app, srv := logApp(t, nine(t))
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\n[contexts.b]\nsubdomain = \"firma-b\"\n")
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	for _, name := range []string{"a", "b"} {
		if err := app.Keyring.Set(name, "test-token-"+name); err != nil {
			t.Fatal(err)
		}
	}
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "1", "--user", "1"), 0)
	wantExit(t, app, at(app, nine(t), "time", "status", "--context", "b", "-o", "json"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"running":false}` {
		t.Errorf("context b sees the timer of a: %s", out)
	}
	wantExit(t, app, at(app, nine(t).Add(time.Hour), "time", "start", "--context", "b", "--task", "2", "--user", "2"), 0)
	if timerFile(t, app, "a") == nil || timerFile(t, app, "b") == nil {
		t.Fatal("state files missing")
	}
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":1},"user":{"id":1}}`)
	if timerFile(t, app, "a") != nil || timerFile(t, app, "b") == nil {
		t.Error("stop of a touched the timer of b")
	}
	wantExit(t, app, at(app, morning(t), "time", "status", "--context", "b", "-o", "json"), 0)
	if out, _ := bufs(app); !strings.Contains(out, `"task": 2`) || !strings.Contains(out, `"context": "b"`) {
		t.Errorf("status of b = %s", out)
	}

	// direct mode
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-direct"})
	writeCfg(t, app, "")
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "3", "--user", "3"), 0)
	if filepath.Base(timer.Path(app.CacheDir, "")) != "timer-.direct.json" || timerFile(t, app, "") == nil {
		t.Error("direct mode state missing")
	}

	// a state that names another context is not used (exit 1)
	writeTimerFile(t, app, "a", `{"context":"b","started_at":"2026-11-04T09:00:12+01:00","task":1,"user":1}`)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\n")
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	wantExit(t, app, at(app, morning(t), "time", "stop"), 1)
	wantStderr(t, app, "Kontext", "papierkram time discard")
}

// --dry-run belongs to time stop: start and discard only change local
// state, status only reads (exit 2 before anything happens).
func TestTimerDryRunOnlyOnStop(t *testing.T) {
	for _, args := range [][]string{
		{"time", "start", "--task", "545", "--user", "1", "--dry-run"},
		{"time", "discard", "--yes", "--dry-run"},
		{"time", "status", "--dry-run"},
	} {
		app, srv := logApp(t, nine(t))
		writeTimerFile(t, app, "", `{"context":"","started_at":"2026-11-04T08:00:00+01:00","task":1,"user":1}`)
		wantExit(t, app, at(app, morning(t), args...), 2)
		if string(timerFile(t, app, "")) != `{"context":"","started_at":"2026-11-04T08:00:00+01:00","task":1,"user":1}` {
			t.Errorf("%v changed the state", args)
		}
		wantRequests(t, srv, 0)
	}
}

func TestTimerStopWithoutTimer(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, morning(t), "time", "stop"), 2)
	wantStderr(t, app, "Kein Timer läuft", "papierkram time start")
	wantRequests(t, srv, 0)
}

// The classes of the timer commands: start and stop create (read-only
// gate), status reads.
func TestTimerCommandAnnotations(t *testing.T) {
	app, _ := logApp(t, nine(t))
	root := NewRootCmd(app)
	for name, class := range map[string]string{"start": "create", "stop": "create", "status": "read_only"} {
		cmd, _, err := root.Find([]string{"time", name})
		if err != nil || cmd.Name() != name || cmd.Annotations[annotClass] != class {
			t.Errorf("time %s: %v, annotations %v", name, err, cmd.Annotations)
		}
	}
}

// Controller ruling (concern 1): the timer remembers the account (base
// URL) it was started for; time stop against another account, or with a
// state of the old format without it, is exit 2 before any request, the
// state stays and the hint names both hosts and time discard.
func TestTimerStopOtherAccountRefused(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	if !strings.Contains(string(before), `"base_url":"`+srv.URL()+`"`) {
		t.Errorf("state without base URL: %s", before)
	}
	other := mockapi.New(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": other.URL(), "PAPIERKRAM_TOKEN": "test-token-log"})
	wantExit(t, app, at(app, morning(t), "time", "stop", "--yes"), 2)
	wantRequests(t, srv, 0)
	wantRequests(t, other, 0)
	if !bytes.Equal(timerFile(t, app, ""), before) {
		t.Error("state changed")
	}
	host := func(u string) string { return strings.TrimPrefix(strings.SplitN(u, "/api", 2)[0], "http://") }
	wantStderr(t, app, host(srv.URL()), host(other.URL()), "papierkram time discard")

	// the old format without base_url is a mismatch too (fail-safe)
	app, srv = logApp(t, nine(t))
	old := `{"context":"","started_at":"2026-11-04T09:00:12+01:00","task":545,"user":1}`
	writeTimerFile(t, app, "", old)
	wantExit(t, app, at(app, morning(t), "time", "stop", "--yes"), 2)
	wantRequests(t, srv, 0)
	if string(timerFile(t, app, "")) != old {
		t.Error("old state changed")
	}
	wantStderr(t, app, host(srv.URL()), "papierkram time discard")
}

// Review I1: time start never saves a timer the other commands reject:
// task or user 0 (flag or environment) and an overlong comment are exit 2
// before anything is saved or sent.
func TestTimerStartRejectsUnusableState(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"task 0", nil, []string{"--task", "0", "--user", "1"}, "Aufgabe 0 aus --task"},
		{"user 0", nil, []string{"--task", "545", "--user", "0"}, "Benutzer 0 aus --user"},
		{"env task 0", map[string]string{"PAPIERKRAM_TASK": "0"}, []string{"--user", "1"}, "Aufgabe 0 aus PAPIERKRAM_TASK"},
		{"long comment", nil, []string{"--task", "545", "--user", "1", "-m", strings.Repeat("x", 70000)}, "Kommentar zu lang"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, nine(t))
			env := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log"}
			for k, v := range tc.env {
				env[k] = v
			}
			app.Env = envMap(env)
			wantExit(t, app, at(app, nine(t), append([]string{"time", "start"}, tc.args...)...), 2)
			wantStderr(t, app, tc.want)
			if timerFile(t, app, "") != nil {
				t.Error("state saved")
			}
			wantRequests(t, srv, 0)
		})
	}
	// the longest comment is fine and the timer can be read again
	app, _ := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "-m", strings.Repeat("<", timer.MaxComment)), 0)
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
}

// Review M2: on the days of the time change, time status shows the
// wall-clock difference time stop books (controller ruling: wall-clock
// arithmetic), and a period of zero wall-clock minutes after an hour is
// named as a clock change, not as "zu kurz".
func TestTimerStatusAgreesWithStopOnDSTDays(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, time.UTC) }
	for _, tc := range []struct {
		name        string
		start, stop time.Time
		duration    string
		body        string // "" when stop is refused
		refused     string
	}{
		// 01:30 CEST → 03:00 CET: 2:30 elapsed, 1:30 by the clock
		{"autumn", utc(2026, 10, 24, 23, 30), utc(2026, 10, 25, 2, 0), "1:30",
			`{"entry_date":"2026-10-25","started_at_time":"01:30","ended_at_time":"03:00","task":{"id":545},"user":{"id":1}}`, ""},
		// 01:30 CET → 03:30 CEST: 1:00 elapsed, 2:00 by the clock
		{"spring", utc(2026, 3, 29, 0, 30), utc(2026, 3, 29, 1, 30), "2:00",
			`{"entry_date":"2026-03-29","started_at_time":"01:30","ended_at_time":"03:30","task":{"id":545},"user":{"id":1}}`, ""},
		// 02:30 CEST → 02:30 CET: one hour elapsed, 0 by the clock
		{"autumn repeated hour", utc(2026, 10, 25, 0, 30), utc(2026, 10, 25, 1, 30), "0:00", "", "umgestellt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.start)
			wantExit(t, app, at(app, tc.start, "time", "start", "--task", "545", "--user", "1"), 0)
			wantExit(t, app, at(app, tc.stop, "time", "status", "-o", "json"), 0)
			var v struct {
				Duration string `json:"duration"`
			}
			if out, _ := bufs(app); json.Unmarshal([]byte(out), &v) != nil || v.Duration != tc.duration {
				t.Errorf("status duration = %q, want %q", v.Duration, tc.duration)
			}
			if tc.body != "" {
				wantExit(t, app, at(app, tc.stop, "time", "stop", "-o", "json"), 0)
				wantBody(t, srv, "POST", timeEntries, tc.body)
				return
			}
			wantExit(t, app, at(app, tc.stop, "time", "stop"), 2)
			wantStderr(t, app, tc.refused, "papierkram time log", "papierkram time discard")
			if _, errs := bufs(app); strings.Contains(errs, "zu kurz") {
				t.Errorf("clock change named as too short: %s", errs)
			}
			wantRequests(t, srv, 0)
		})
	}
}

// Review M3: a time zone change between start and stop books the clock
// times of the stop location; the help says so.
func TestTimerStopInAnotherTimeZone(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, berlinAt(t, 2026, 11, 4, 9, 0, 0), "time", "start", "--task", "545", "--user", "1"), 0)
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	app.Location = london
	wantExit(t, app, at(app, time.Date(2026, 11, 4, 11, 0, 0, 0, london), "time", "stop", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"08:00","ended_at_time":"11:00","task":{"id":545},"user":{"id":1}}`)
	cmd, _, _ := NewRootCmd(app).Find([]string{"time", "stop"})
	if !strings.Contains(cmd.Long, "Zeitzone") || !strings.Contains(cmd.Long, "Stopp-Orts") {
		t.Errorf("help without the time zone rule: %s", cmd.Long)
	}
}

// Open point 22 (decided): a timer stopped in the first minute of the
// next day (00:00:00–00:00:59) books until 24:00 of its start date, like
// time log --to 24:00; a later stop is over midnight (exit 2, the timer
// stays). Review M1: the hint for a timer over midnight to the next day
// names both halves, the first one up to 24:00, so 23:59 → 00:30 books
// the minute before midnight as well.
func TestTimerStopAtMidnight(t *testing.T) {
	for _, tc := range []struct {
		name        string
		start, stop time.Time
		from        string
	}{
		{"23:00 to 00:00:30", berlinAt(t, 2026, 6, 10, 23, 0, 0), berlinAt(t, 2026, 6, 11, 0, 0, 30), "23:00"},
		{"23:59 to 00:00:20", berlinAt(t, 2026, 6, 10, 23, 59, 10), berlinAt(t, 2026, 6, 11, 0, 0, 20), "23:59"},
		{"22:10 to 00:00:00", berlinAt(t, 2026, 6, 10, 22, 10, 0), berlinAt(t, 2026, 6, 11, 0, 0, 0), "22:10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.start)
			wantExit(t, app, at(app, tc.start, "time", "start", "--task", "545", "--user", "1"), 0)
			wantExit(t, app, at(app, tc.stop, "time", "stop", "-o", "json"), 0)
			wantBody(t, srv, "POST", timeEntries,
				`{"entry_date":"2026-06-10","started_at_time":"`+tc.from+`","ended_at_time":"24:00","task":{"id":545},"user":{"id":1}}`)
			wantStderr(t, app, "Timer gestoppt: 2026-06-10 "+tc.from+"–24:00")
			if b := timerFile(t, app, ""); b != nil {
				t.Errorf("timer kept: %s", b)
			}
		})
	}
	for _, tc := range []struct {
		name        string
		start, stop time.Time
		want, not   []string
	}{
		{"23:00 to 00:01", berlinAt(t, 2026, 6, 10, 23, 0, 0), berlinAt(t, 2026, 6, 11, 0, 1, 0),
			[]string{"über Mitternacht", "papierkram time log --task 545 --user 1 --date 2026-06-10 --from 23:00 --to 24:00",
				"papierkram time log --task 545 --user 1 --date 2026-06-11 --from 00:00 --to 00:01", "papierkram time discard"}, nil},
		{"23:59 to 00:30", berlinAt(t, 2026, 11, 3, 23, 59, 0), berlinAt(t, 2026, 11, 4, 0, 30, 0),
			[]string{"papierkram time log --task 545 --user 1 --date 2026-11-03 --from 23:59 --to 24:00",
				"papierkram time log --task 545 --user 1 --date 2026-11-04 --from 00:00 --to 00:30", "papierkram time discard"}, nil},
		{"two days later at 00:00", berlinAt(t, 2026, 6, 10, 23, 0, 0), berlinAt(t, 2026, 6, 12, 0, 0, 30),
			[]string{"über Mitternacht", "Je Tag einen Eintrag", "--date 2026-06-10 --from 23:00 --to 24:00", "papierkram time discard"},
			[]string{"--date 2026-06-12"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, tc.start)
			wantExit(t, app, at(app, tc.start, "time", "start", "--task", "545", "--user", "1"), 0)
			before := timerFile(t, app, "")
			wantExit(t, app, at(app, tc.stop, "time", "stop"), 2)
			wantStderr(t, app, tc.want...)
			_, errs := bufs(app)
			for _, n := range tc.not {
				if strings.Contains(errs, n) {
					t.Errorf("stderr mit %q: %s", n, errs)
				}
			}
			wantRequests(t, srv, 0)
			if after := timerFile(t, app, ""); !bytes.Equal(after, before) {
				t.Errorf("state changed: %s → %s", before, after)
			}
		})
	}
}

// Review M7: the commands in the hints quote a context name the shell
// would read otherwise and name a stored comment as a placeholder, so it
// is not lost silently when booking by hand.
func TestTimerHintCommandsQuoteAndNameComment(t *testing.T) {
	app, srv := logApp(t, nine(t))
	writeCfg(t, app, "[contexts.'a;b']\nsubdomain = \"firma\"\n[contexts.plain]\nsubdomain = \"firma\"\n")
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	for _, name := range []string{"a;b", "plain"} {
		if err := app.Keyring.Set(name, "test-token-"+name); err != nil {
			t.Fatal(err)
		}
	}
	srv.On("POST", timeEntries, http.StatusInternalServerError, []byte(`{"type":"error","message":"kaputt"}`))
	wantExit(t, app, at(app, nine(t), "time", "start", "--context", "a;b", "--task", "545", "--user", "1", "-m", "Abstimmung"), 0)
	wantExit(t, app, at(app, morning(t), "time", "stop", "--context", "a;b"), 1)
	wantStderr(t, app, `--context 'a;b'`, `-m "<Kommentar>"`)

	wantExit(t, app, at(app, nine(t), "time", "start", "--context", "plain", "--task", "545", "--user", "1"), 0)
	wantExit(t, app, at(app, morning(t), "time", "stop", "--context", "plain"), 1)
	wantStderr(t, app, "--context plain")
	if _, errs := bufs(app); strings.Contains(errs, "<Kommentar>") || strings.Contains(errs, "'plain'") {
		t.Errorf("stderr = %s", errs)
	}
	// a comment given to stop is named as well
	wantExit(t, app, at(app, morning(t), "time", "stop", "--context", "plain", "-m", "neu"), 1)
	wantStderr(t, app, `-m "<Kommentar>"`)
}

// Review M4: a file where the cache directory belongs means no timer for
// status and discard; start cannot save and names the cache directory.
func TestTimerBrokenCacheDirectory(t *testing.T) {
	app, srv := logApp(t, nine(t))
	if err := os.WriteFile(filepath.Join(app.CacheDir, "papierkram"), []byte("kein Verzeichnis"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"running":false}` {
		t.Errorf("status = %s", out)
	}
	wantExit(t, app, at(app, morning(t), "time", "discard"), 0)
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 1)
	wantStderr(t, app, "Timer nicht gespeichert", "Cache-Verzeichnis")
	wantRequests(t, srv, 0)
}

// Review M8: time status names the account (host) of the timer and warns
// when it is not the effective one, since time stop would refuse.
func TestTimerStatusShowsAccount(t *testing.T) {
	app, srv := logApp(t, nine(t))
	host := func(u string) string { return strings.TrimPrefix(strings.SplitN(u, "/api", 2)[0], "http://") }
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "wide"), 0)
	if out, errs := bufs(app); !strings.Contains(out, "KONTO") || !strings.Contains(out, host(srv.URL())) || errs != "" {
		t.Errorf("stdout = %s\nstderr = %s", out, errs)
	}
	other := mockapi.New(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": other.URL(), "PAPIERKRAM_TOKEN": "test-token-log"})
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	wantStderr(t, app, host(srv.URL()), host(other.URL()), "papierkram time discard")
	wantRequests(t, srv, 0)
	wantRequests(t, other, 0)
}

// Review N3: time start refuses a base URL that cannot be parsed (exit
// 2, nothing saved): time stop could never book such a timer.
func TestTimerStartRefusesInvalidBaseURL(t *testing.T) {
	app, srv := logApp(t, nine(t))
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": "http://alice:s3cret@127.0.0.1:badport/api/v1", "PAPIERKRAM_TOKEN": "test-token-log"})
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 2)
	wantStderr(t, app, "Basis-URL", "ungültig", "PAPIERKRAM_BASE_URL")
	if out, errs := bufs(app); strings.Contains(out+errs, "alice") || strings.Contains(out+errs, "s3cret") {
		t.Errorf("credentials shown: %s%s", out, errs)
	}
	if timerFile(t, app, "") != nil {
		t.Error("state saved")
	}
	wantRequests(t, srv, 0)
}
