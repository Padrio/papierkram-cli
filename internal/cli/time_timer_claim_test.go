package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
	"github.com/Padrio/papierkram-cli/internal/timer"
)

// Task 11 (C2): time stop claims the timer (renames its state file to
// <file>.stopping) after the question and --dry-run and before the
// request. A second stop finds the claim or no file and ends with exit 7
// without a request; a failure after the claim puts the file back.

const (
	claimedMsg      = "Timer wird bereits von einem anderen papierkram time stop gebucht"
	wantClaimedHint = "Nichts gesendet. Mit papierkram time status prüfen, ob er noch läuft"
	// the remaining claim of the timer of nine(t), task 545, user 1
	unfinishedMsg  = "Ein time stop läuft gerade oder wurde nicht abgeschlossen (Timer seit 2026-11-04 09:00, Aufgabe 545, Benutzer 1)"
	unfinishedHint = "Erst mit papierkram time list --since 2026-11-04 prüfen, ob der Eintrag angelegt wurde. Läuft kein anderer time stop mehr: " +
		"wenn ja, papierkram time discard; sonst papierkram time discard und von Hand buchen mit " +
		"papierkram time log --task 545 --user 1 --date 2026-11-04 --from 09:00 --to HH:MM"
)

// claimFile is the content of the claim of the timer of ctx, nil
// without one.
func claimFile(t *testing.T, app *App, ctx string) []byte {
	t.Helper()
	data, err := os.ReadFile(timer.StoppingPath(timer.Path(app.CacheDir, ctx)))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// leaveClaim leaves the claim of a time stop that did not finish: the
// state file at path is now its claim.
func leaveClaim(t *testing.T, path string) {
	t.Helper()
	if err := os.Rename(path, timer.StoppingPath(path)); err != nil {
		t.Fatal(err)
	}
}

// twin is a second invocation on the same machine: another app with the
// cache directory, account and token of app.
func twin(t *testing.T, app *App) *App {
	t.Helper()
	b, _ := logApp(t, nine(t))
	b.CacheDir, b.Env = app.CacheDir, app.Env
	return b
}

// jsonError is the structured error of the last stderr line (-o json).
func jsonError(t *testing.T, app *App) (msg, hint string) {
	t.Helper()
	_, errs := bufs(app)
	lines := strings.Split(strings.TrimSpace(errs), "\n")
	var e struct {
		Error struct {
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatalf("no JSON error on stderr: %v\n%s", err, errs)
	}
	return e.Error.Message, e.Error.Hint
}

// barrierTimeout ends a barrier of the mock that a broken claim would
// never release; the test then fails instead of hanging.
const barrierTimeout = 10 * time.Second

func posts(srv *mockapi.Server) int {
	n := 0
	for _, r := range srv.Requests() {
		if r.Method == "POST" {
			n++
		}
	}
	return n
}

func entryCreated(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"type":"time_entry","id":77}`))
}

// A stop while another one waits for its answer finds the claim: exit 7
// without a request, like time start; time status shows the claim with
// a warning. The first stop books once and leaves no file.
func TestTimerStopWhileAnotherSends(t *testing.T) {
	a, srv := logApp(t, nine(t))
	wantExit(t, a, at(a, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	arrived, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	srv.OnFunc("POST", timeEntries, func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(arrived) })
		select { // the barrier: answer only after the second stop
		case <-release:
		case <-time.After(barrierTimeout): // a second POST must not hang the test
		}
		entryCreated(w)
	})
	done := make(chan int, 1)
	go func() { done <- at(a, morning(t), "time", "stop", "-o", "json") }()
	select {
	case <-arrived:
	case code := <-done:
		out, errs := bufs(a)
		t.Fatalf("first stop ended before its request: exit %d\n%s%s", code, out, errs)
	}
	if timerFile(t, a, "") != nil || claimFile(t, a, "") == nil {
		t.Error("the timer is not claimed while the first stop sends")
	}

	b := twin(t, a)
	for _, args := range [][]string{{"time", "stop", "-o", "json"}, {"time", "start", "--task", "545", "--user", "1", "-o", "json"}} {
		wantExit(t, b, at(b, morning(t), args...), 7)
		if msg, hint := jsonError(t, b); msg != unfinishedMsg || hint != unfinishedHint {
			t.Errorf("%v: message %q\nhint %q", args, msg, hint)
		}
	}
	wantExit(t, b, at(b, morning(t), "time", "status", "-o", "json"), 0)
	if out, _ := bufs(b); !strings.Contains(out, `"running": true`) || !strings.Contains(out, `"task": 545`) {
		t.Errorf("status of the claim = %s", out)
	}
	wantStderr(t, b, "Warnung: "+unfinishedMsg+". "+unfinishedHint)

	close(release)
	if code := <-done; code != 0 {
		out, errs := bufs(a)
		t.Fatalf("first stop: exit %d\n%s%s", code, out, errs)
	}
	if n := posts(srv); n != 1 {
		t.Errorf("%d POST, want 1", n)
	}
	if timerFile(t, a, "") != nil || claimFile(t, a, "") != nil {
		t.Error("a file is left after the stop")
	}
}

// Two stops that both read the timer and asked: the barrier in the mock
// holds both at the name of the task until both are there, so both
// confirm with the timer read. Exactly one claims it and books; the other
// ends with exit 7 before its request.
func TestTimerStopParallelClaim(t *testing.T) {
	a, srv := logApp(t, nine(t))
	wantExit(t, a, at(a, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	var mu sync.Mutex
	arrivals := 0
	both := make(chan struct{})
	srv.OnFunc("GET", "/tracker/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals++
		if arrivals == 2 {
			close(both)
		}
		mu.Unlock()
		select {
		case <-both:
		case <-r.Context().Done():
			return
		case <-time.After(barrierTimeout):
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"task","id":545,"name":"Tickets"}`))
	})
	apps := []*App{a, twin(t, a)}
	codes := make([]int, len(apps))
	var wg sync.WaitGroup
	for i, app := range apps {
		app.StdinTTY, app.In = true, strings.NewReader("\n")
		wg.Go(func() { codes[i] = at(app, morning(t), "time", "stop", "-o", "json") })
	}
	wg.Wait()
	loser := slices.Index(codes, 7)
	if !slices.Contains(codes, 0) || loser < 0 {
		for _, app := range apps {
			out, errs := bufs(app)
			t.Logf("stdout: %s\nstderr: %s", out, errs)
		}
		t.Fatalf("exit codes %v, want 0 and 7", codes)
	}
	if msg, hint := jsonError(t, apps[loser]); msg != claimedMsg || hint != wantClaimedHint {
		t.Errorf("message %q\nhint %q", msg, hint)
	}
	if n := posts(srv); n != 1 {
		t.Errorf("%d POST, want 1", n)
	}
	if timerFile(t, a, "") != nil || claimFile(t, a, "") != nil {
		t.Error("a file is left after the stops")
	}
}

// swapReader runs swap once before the first read, like another
// invocation while a stop asks.
type swapReader struct {
	once sync.Once
	swap func()
	r    io.Reader
}

func (s *swapReader) Read(p []byte) (int, error) {
	s.once.Do(s.swap)
	return s.r.Read(p)
}

// While a stop asks in the terminal, another one books the timer and a
// new one is started: the claim is not the timer that was read, so the
// stop sends nothing (exit 7) and puts the new timer back unchanged.
func TestTimerStopClaimOfAnotherTimer(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	path := timer.Path(app.CacheDir, "")
	var newer []byte
	app.StdinTTY = true
	app.In = &swapReader{r: strings.NewReader("\n"), swap: func() {
		st, err := timer.Load(path)
		if err != nil || st == nil {
			t.Errorf("Load = %+v, %v", st, err)
			return
		}
		st.StartedAt, st.Task = st.StartedAt.Add(time.Hour), 999
		if err := timer.Delete(path); err != nil {
			t.Error(err)
		}
		if err := timer.Save(path, *st); err != nil {
			t.Error(err)
		}
		newer, _ = os.ReadFile(path)
	}}
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 7)
	if msg, hint := jsonError(t, app); !strings.Contains(msg, "seit dem Lesen ersetzt") || hint != wantClaimedHint {
		t.Errorf("message %q\nhint %q", msg, hint)
	}
	if n := posts(srv); n != 0 {
		t.Errorf("%d POST, want 0", n)
	}
	if got := timerFile(t, app, ""); newer == nil || !bytes.Equal(got, newer) {
		t.Errorf("new timer not put back: %s, want %s", got, newer)
	}
	if claimFile(t, app, "") != nil {
		t.Error("claim left")
	}
}

// The claim is taken before the request and released after any failure:
// during the request the state file is the claim; afterwards the timer
// is back unchanged and the hint is the one without a claim.
func TestTimerStopReleasesClaimOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		answer func(http.ResponseWriter, *http.Request)
		exit   int
		hint   string
	}{
		{name: "validation", exit: 5,
			answer: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"type":"error","message":"Aufgabe abgeschlossen"}`))
			},
			hint: "Der Timer läuft weiter: erneut papierkram time stop oder von Hand buchen mit " +
				"papierkram time log --task 545 --user 1 --date 2026-11-04 --from 09:00 --to 10:47, danach papierkram time discard"},
		{name: "outcome unknown", args: []string{"--timeout", "50ms"}, exit: 12,
			answer: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
			hint: "Ergebnis unbekannt – vor Wiederholung per describe/list prüfen. " +
				"Der Timer läuft weiter: erst mit papierkram time list prüfen, ob der Eintrag angelegt wurde; " +
				"wenn ja, den Timer mit papierkram time discard verwerfen, sonst erneut papierkram time stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, nine(t))
			wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
			before := timerFile(t, app, "")
			// the files during the request; the channel orders the
			// handler's reads before the checks (the client of the timeout
			// case does not wait for the handler)
			seen := make(chan [2][]byte, 1)
			srv.OnFunc("POST", timeEntries, func(w http.ResponseWriter, r *http.Request) {
				seen <- [2][]byte{timerFile(t, app, ""), claimFile(t, app, "")}
				tc.answer(w, r)
			})
			wantExit(t, app, at(app, morning(t), append([]string{"time", "stop", "-o", "json"}, tc.args...)...), tc.exit)
			files := <-seen
			if during, claim := files[0], files[1]; during != nil || !bytes.Equal(claim, before) {
				t.Errorf("during the request: state %s, claim %s", during, claim)
			}
			if after := timerFile(t, app, ""); !bytes.Equal(after, before) {
				t.Errorf("state not put back: %s → %s", before, after)
			}
			if claimFile(t, app, "") != nil {
				t.Error("claim left")
			}
			if _, hint := jsonError(t, app); !strings.HasSuffix(hint, tc.hint) {
				t.Errorf("hint %q\nwant suffix %q", hint, tc.hint)
			}
		})
	}
}

// A claim that cannot be put back (a new state file is in its place) is
// exit 1 naming the rename and time discard; neither file is touched.
func TestTimerStopReleaseFailure(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	path := timer.Path(app.CacheDir, "")
	srv.OnFunc("POST", timeEntries, func(w http.ResponseWriter, _ *http.Request) {
		writeTimerFile(t, app, "", `{"context":"","started_at":"2026-11-04T10:00:00+01:00","task":999,"user":1}`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"type":"error","message":"Aufgabe abgeschlossen"}`))
	})
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 1)
	msg, hint := jsonError(t, app)
	if !strings.HasPrefix(msg, "Timer-Datei nicht zurückgestellt: ") {
		t.Errorf("message %q", msg)
	}
	for _, w := range []string{"Die Datei " + printable(timer.StoppingPath(path)) + " in " + printable(path) + " umbenennen",
		"papierkram time discard", "Aufgabe abgeschlossen"} {
		if !strings.Contains(hint, w) {
			t.Errorf("hint without %q: %s", w, hint)
		}
	}
	if !bytes.Equal(claimFile(t, app, ""), before) || !strings.Contains(string(timerFile(t, app, "")), `"task":999`) {
		t.Errorf("files changed: claim %s, state %s", claimFile(t, app, ""), timerFile(t, app, ""))
	}
}

// A dry run and a declined question never claim the timer.
func TestTimerStopNoClaimWithoutRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		tty  bool
		exit int
	}{
		{"dry run", []string{"--dry-run"}, false, 0},
		{"dry run in a terminal", []string{"--dry-run"}, true, 0},
		{"declined", nil, true, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, nine(t))
			wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
			before := timerFile(t, app, "")
			// a claim during the question would show here
			var seen []byte
			app.StdinTTY = tc.tty
			app.In = &swapReader{r: strings.NewReader("n\n"), swap: func() { seen = claimFile(t, app, "") }}
			wantExit(t, app, at(app, morning(t), append([]string{"time", "stop"}, tc.args...)...), tc.exit)
			if seen != nil || claimFile(t, app, "") != nil || !bytes.Equal(timerFile(t, app, ""), before) {
				t.Errorf("claimed: during %s, after %s, state %s", seen, claimFile(t, app, ""), timerFile(t, app, ""))
			}
			if n := posts(srv); n != 0 {
				t.Errorf("%d POST", n)
			}
		})
	}
}

// A claim left behind (the stop was killed) blocks start and stop (exit
// 7, no request), status shows it with a warning, discard removes it and
// a state file next to it, naming both.
func TestTimerUnfinishedStop(t *testing.T) {
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	path := timer.Path(app.CacheDir, "")
	leaveClaim(t, path)
	claim := claimFile(t, app, "")
	for _, args := range [][]string{{"time", "stop", "-o", "json"}, {"time", "start", "--task", "1", "--user", "1", "-o", "json"}} {
		wantExit(t, app, at(app, morning(t), args...), 7)
		if msg, hint := jsonError(t, app); msg != unfinishedMsg || hint != unfinishedHint {
			t.Errorf("%v: message %q\nhint %q", args, msg, hint)
		}
	}
	wantExit(t, app, at(app, morning(t), "time", "stop"), 7)
	wantStderr(t, app, "Fehler: "+unfinishedMsg, "Hinweis: "+unfinishedHint)
	if timerFile(t, app, "") != nil || !bytes.Equal(claimFile(t, app, ""), claim) {
		t.Error("files changed by a blocked start or stop")
	}

	wantExit(t, app, at(app, morning(t), "time", "status"), 0)
	if out, _ := bufs(app); !strings.Contains(out, "1:47") || !strings.Contains(out, "545") {
		t.Errorf("status = %s", out)
	}
	wantStderr(t, app, "Warnung: "+unfinishedMsg+". "+unfinishedHint)

	// discard: confirmation as before, then both files are gone
	wantExit(t, app, at(app, morning(t), "time", "discard"), 8)
	writeTimerFile(t, app, "", `{"context":"","started_at":"2026-11-04T10:00:00+01:00","task":999,"user":1}`)
	app.StdinTTY, app.In = true, strings.NewReader("ja\n")
	wantExit(t, app, at(app, morning(t), "time", "discard"), 0)
	wantStderr(t, app, unfinishedMsg, "Timer läuft seit 2026-11-04 10:00 (0:47 h): Aufgabe 999")
	if timerFile(t, app, "") != nil || claimFile(t, app, "") != nil {
		t.Error("discard left a file")
	}

	// only a claim, discarded with --yes
	app.StdinTTY = false
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	leaveClaim(t, path)
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes", "-o", "json"), 0)
	if out, _ := bufs(app); canonJSON(t, []byte(out)) != `{"discarded":true}` {
		t.Errorf("json = %s", out)
	}
	if timerFile(t, app, "") != nil || claimFile(t, app, "") != nil {
		t.Error("discard --yes left a file")
	}
	wantRequests(t, srv, 0)
}

// A claim that cannot be used is an error (exit 1) that names the claim
// and time discard, like a corrupt state file; discard removes it.
func TestTimerUnusableClaim(t *testing.T) {
	app, srv := logApp(t, nine(t))
	p := timer.StoppingPath(timer.Path(app.CacheDir, ""))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("kein JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"time", "status"}, {"time", "start", "--task", "1", "--user", "1"}, {"time", "stop"}} {
		wantExit(t, app, at(app, morning(t), args...), 1)
		wantStderr(t, app, "timer-.direct.json.stopping", "ungültig", "papierkram time discard")
	}
	wantExit(t, app, at(app, morning(t), "time", "discard", "--yes"), 0)
	if claimFile(t, app, "") != nil {
		t.Error("unusable claim kept")
	}
	// a claim of another context is unusable too
	writeTimerFile(t, app, "", `{"context":"x","started_at":"2026-11-04T09:00:00+01:00","task":1,"user":1}`)
	leaveClaim(t, timer.Path(app.CacheDir, ""))
	wantExit(t, app, at(app, morning(t), "time", "stop"), 1)
	wantStderr(t, app, "timer-.direct.json.stopping", "gehört zu", "papierkram time discard")
	wantRequests(t, srv, 0)
}

// When the claim cannot be deleted after the entry was created, the entry
// is shown and the error says not to stop again (exit 11), naming the
// claim.
func TestTimerStopDeleteFailureIsPartial(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions")
	}
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	dir := filepath.Dir(timer.Path(app.CacheDir, ""))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	srv.OnFunc("POST", timeEntries, func(w http.ResponseWriter, _ *http.Request) {
		// after the claim: the claim cannot be deleted
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Error(err)
		}
		entryCreated(w)
	})
	wantExit(t, app, at(app, morning(t), "time", "stop", "-o", "json"), 11)
	wantRequests(t, srv, 1)
	if out, _ := bufs(app); !strings.Contains(out, `"id": 77`) {
		t.Errorf("created entry not shown: %s", out)
	}
	wantStderr(t, app, "Zeiteintrag angelegt", "NICHT erneut", "papierkram time discard", "timer-.direct.json.stopping", "von Hand löschen")
	// a later stop finds the claim: exit 7, no second entry
	_ = os.Chmod(dir, 0o700)
	wantExit(t, app, at(app, morning(t), "time", "stop"), 7)
	wantRequests(t, srv, 1)
}

// A claim the file system refuses (no write permission on the directory)
// sends nothing: exit 1, the timer stays.
func TestTimerStopClaimFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions")
	}
	app, srv := logApp(t, nine(t))
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	before := timerFile(t, app, "")
	dir := filepath.Dir(timer.Path(app.CacheDir, ""))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	wantExit(t, app, at(app, morning(t), "time", "stop"), 1)
	wantStderr(t, app, "Timer-Datei nicht beansprucht", "Nichts gesendet", "Der Timer läuft weiter", "papierkram time log --task 545")
	wantRequests(t, srv, 0)
	if !bytes.Equal(timerFile(t, app, ""), before) {
		t.Error("state changed")
	}
}
