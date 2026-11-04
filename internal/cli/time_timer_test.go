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
	want := `{"comment":"Abstimmung","context":"","duration":"1:47","duration_minutes":107,"running":true,` +
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
			stderr: []string{"Timer läuft weiter", "papierkram time log --task 545 --user 1 --date 2026-11-04 --from 09:00 --to 10:47", "papierkram time discard"}},
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
			stderr: []string{"Mitternacht", "papierkram time log --task 545 --user 1 --date 2026-11-03 --from 22:10 --to 23:59",
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

// When the timer file cannot be deleted after the entry was created, the
// entry is shown and the error says not to stop again (exit 11).
func TestTimerStopDeleteFailureIsPartial(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions")
	}
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	dir := filepath.Dir(timer.Path(app.CacheDir, ""))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 11)
	wantRequests(t, srv, 1)
	if out, _ := bufs(app); !strings.Contains(out, `"id":`) {
		t.Errorf("created entry not shown: %s", out)
	}
	wantStderr(t, app, "Zeiteintrag angelegt", "NICHT erneut", "papierkram time discard")
}
