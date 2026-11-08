package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/redact"
)

// Task 12: time start --unbillable stores the checked value in the timer
// and time stop sends it, like time log --unbillable sends its flag.

// TestTimerStartUnbillableSent: --unbillable (true), --unbillable=false and
// --unbillable=nein are stored and sent as the body of time log with the
// same flag; without the flag nothing is stored or sent.
func TestTimerStartUnbillableSent(t *testing.T) {
	for _, tc := range []struct {
		flag  []string
		state string // what the state file holds, "" for no field
		body  string // unbillable in the body, "" for no field
	}{
		{[]string{"--unbillable"}, `"unbillable":true`, `,"unbillable":true`},
		{[]string{"--unbillable=false"}, `"unbillable":false`, `,"unbillable":false`},
		{[]string{"--unbillable=nein"}, `"unbillable":false`, `,"unbillable":false`},
		{nil, "", ""},
	} {
		name := strings.Join(tc.flag, " ")
		app, srv := logApp(t, nine(t))
		wantExit(t, app, at(app, nine(t), append([]string{"time", "start", "--task", "545", "--user", "1"}, tc.flag...)...), 0)
		file := string(timerFile(t, app, ""))
		if tc.state == "" && strings.Contains(file, "unbillable") || tc.state != "" && !strings.Contains(file, tc.state) {
			t.Errorf("%q: state file %s, want %q", name, file, tc.state)
		}
		wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 0)
		wantBody(t, srv, "POST", timeEntries,
			`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":545},"user":{"id":1}`+tc.body+`}`)

		// parity: time log with the same flag sends the same body
		logA, logSrv := logApp(t, morning(t))
		wantExit(t, logA, at(logA, morning(t), append([]string{"time", "log", "--task", "545", "--user", "1",
			"--from", "09:00", "--to", "10:47", "-o", "json"}, tc.flag...)...), 0)
		stop, log := wantRequests(t, srv, 1)[0], wantRequests(t, logSrv, 1)[0]
		if canonJSON(t, stop.Body) != canonJSON(t, log.Body) {
			t.Errorf("%q: time stop %s\ntime log  %s", name, stop.Body, log.Body)
		}
	}
}

// An invalid value is exit 5 at start (like time log): no timer, no
// request. Under read-only the gate comes first (exit 9). time stop has
// no --unbillable (exit 2): the start decides.
func TestTimerStartUnbillableInvalid(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "--unbillable=vielleicht"), 5)
	wantStderr(t, app, `"vielleicht"`, "--unbillable", "Timer nicht gestartet")
	if timerFile(t, app, "") != nil {
		t.Error("state saved for an invalid --unbillable")
	}
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-log", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "--unbillable=vielleicht"), 9)

	app, srv = logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	wantExit(t, app, at(app, morning(t), "time", "stop", "--unbillable"), 2)
	if timerFile(t, app, "") == nil {
		t.Error("timer gone after a rejected stop")
	}
	wantRequests(t, srv, 0)
}

// A state file of an older version has no unbillable: time stop sends
// none (the timer stays valid and is claimed as read).
func TestTimerStopOldStateWithoutUnbillable(t *testing.T) {
	app, srv := logApp(t, nine(t))
	writeTimerFile(t, app, "", `{"context":"","started_at":"2026-11-04T09:00:12+01:00","task":545,"user":1,"comment":"Abstimmung","base_url":"`+
		redact.URL(srv.URL())+`"}`+"\n")
	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
	if out, _ := bufs(app); !strings.Contains(out, `"unbillable": false`) {
		t.Errorf("status = %s", out)
	}
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 0)
	wantBody(t, srv, "POST", timeEntries,
		`{"entry_date":"2026-11-04","started_at_time":"09:00","ended_at_time":"10:47","task":{"id":545},"user":{"id":1},"comments":"Abstimmung"}`)
	if timerFile(t, app, "") != nil || claimFile(t, app, "") != nil {
		t.Error("timer files left after stop")
	}
}

// time start and time status show unbillable: JSON as a bool (false
// without the flag), the table with -o wide as NICHT ABRECHENBAR (ja or
// empty), the text of time start names it.
func TestTimerUnbillableOutput(t *testing.T) {
	unbillableOf := func(t *testing.T, app *App) any {
		t.Helper()
		out, _ := bufs(app)
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return m["unbillable"]
	}
	app, _ := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "--unbillable", "-o", "json"), 0)
	if v := unbillableOf(t, app); v != true {
		t.Errorf("start unbillable = %v", v)
	}
	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
	if v := unbillableOf(t, app); v != true {
		t.Errorf("status unbillable = %v", v)
	}
	wantExit(t, app, at(app, morning(t), "time", "status", "-o", "wide"), 0)
	out, _ := bufs(app)
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 || !strings.Contains(lines[0], "NICHT ABRECHENBAR") ||
		!strings.Contains(lines[1], "ja") {
		t.Errorf("wide = %s", out)
	}
	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	if out, _ := bufs(app); strings.Contains(out, "NICHT ABRECHENBAR") {
		t.Errorf("table without -o wide shows unbillable: %s", out)
	}
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes"), 0)
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1", "--unbillable"), 0)
	if out, _ := bufs(app); !strings.Contains(out, "nicht abrechenbar") {
		t.Errorf("start text = %s", out)
	}

	for _, flag := range [][]string{nil, {"--unbillable=false"}} {
		app, _ = logApp(t, nine(t))
		wantExit(t, app, at(app, nine(t), append([]string{"time", "start", "--task", "545", "--user", "1", "-o", "json"}, flag...)...), 0)
		if v := unbillableOf(t, app); v != false {
			t.Errorf("%v: start unbillable = %v", flag, v)
		}
		if out, _ := bufs(app); strings.Contains(out, "nicht abrechenbar") {
			t.Errorf("%v: start names unbillable: %s", flag, out)
		}
		wantExit(t, app, at(app, morning(t), "time", "status", "-o", "wide"), 0)
		out, _ := bufs(app)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 {
			t.Fatalf("wide = %s", out)
		}
		header, row := lines[0], lines[1]
		col := strings.Index(header, "NICHT ABRECHENBAR")
		if col < 0 || len(row) > col && strings.TrimSpace(row[col:min(len(row), col+len("NICHT ABRECHENBAR"))]) != "" {
			t.Errorf("%v: wide column not empty:\n%s", flag, out)
		}
	}
}

// The commands for booking by hand that the hints of time stop name
// book what the timer would: with --unbillable or --unbillable=false
// when it was given at start, without otherwise.
func TestTimerHintCommandsNameUnbillable(t *testing.T) {
	for _, tc := range []struct {
		flag []string
		want string
	}{
		{[]string{"--unbillable"}, "--to 10:47 --unbillable"},
		{[]string{"--unbillable=nein"}, "--to 10:47 --unbillable=false"},
		{nil, ""},
	} {
		app, srv := logApp(t, nine(t))
		srv.On("POST", timeEntries, http.StatusInternalServerError, []byte(`{"type":"error","message":"kaputt"}`))
		wantExit(t, app, at(app, nine(t), append([]string{"time", "start", "--task", "545", "--user", "1"}, tc.flag...)...), 0)
		wantExit(t, app, at(app, morning(t), "time", "stop"), 1)
		_, errs := bufs(app)
		if tc.want != "" && !strings.Contains(errs, tc.want) || tc.want == "" && strings.Contains(errs, "--unbillable") {
			t.Errorf("%v: stderr = %s", tc.flag, errs)
		}
	}
}

// time log and time start share the definition of --unbillable.
func TestTimeUnbillableFlagShared(t *testing.T) {
	app, _ := logApp(t, nine(t))
	root := NewRootCmd(app)
	var defs []string
	for _, name := range []string{"log", "start"} {
		cmd, _, err := root.Find([]string{"time", name})
		if err != nil {
			t.Fatal(err)
		}
		f := cmd.Flags().Lookup("unbillable")
		if f == nil {
			t.Fatalf("time %s without --unbillable", name)
		}
		defs = append(defs, f.Value.Type()+"|"+f.NoOptDefVal+"|"+f.Usage+"|"+f.Shorthand)
	}
	if defs[0] != defs[1] || defs[0] != "bool|true|nicht abrechenbar|" {
		t.Errorf("--unbillable: time log %q, time start %q", defs[0], defs[1])
	}
}
