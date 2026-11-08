//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// minEarliest is 01:15 in minutes after midnight: the earliest start of
// TestE2E_TimeFlow (its log begins 60 minutes before, plus a margin).
const minEarliest = 75

// Spec 9.6, 9.8, AGENTS "time stop": time tracking live (time log, time
// list --since heute, archive, unarchive) and the local timer (start,
// status, stop, discard). Hard: every step; the timer commands other than
// stop send nothing (0 credits), stop without a timer is exit 2, and the
// stopped entry spans now-10 to now-8 minutes of today with duration 120
// and is unbillable (time start --unbillable, task 12).
//
// All times derive from the current minute in Berlin (s.loc) and lie in
// the past of the same day (ruling FR-C): time log from now-60 to now-30,
// the timer from now-10 to now-8, the discarded timer at now-5. Before
// 01:15 Berlin (minEarliest) the log would start before midnight, so the
// test fails before its first request. All runs share one directory (the
// state file of the timer lies below <dir>/cache/papierkram/) and a fixed
// clock (execAt): heute of time log and time list is the date of the
// timer steps, so the flow never waits a minute and never crosses
// midnight (review of task 5).
func TestE2E_TimeFlow(t *testing.T) {
	s := live(t)

	now := time.Now().In(s.loc)
	if now.Hour()*60+now.Minute() < minEarliest {
		s.fatalf(t, "TimeFlow erst nach 01:15 Berlin starten (jetzt %s): der Zeiteintrag von jetzt−60 min läge am Vortag", now.Format("15:04"))
	}
	// at is the minute ago minutes before now (wall clock of s.loc,
	// seconds 0)
	at := func(ago int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()-ago, 0, 0, s.loc)
	}
	clock := func(ago int) func() time.Time {
		tm := at(ago)
		return func() time.Time { return tm }
	}
	hhmm := func(ago int) string { return at(ago).Format("15:04") }

	task, user := s.task(t), s.user(t)
	dir := t.TempDir()
	// local runs a timer command that sends nothing: hard 0 requests
	local := func(clk func() time.Time, args ...string) result {
		t.Helper()
		r := s.runIn(t, dir, clk, "", args...)
		if len(r.exchanges) != 0 {
			s.errorf(t, "papierkram %s: %d Anfrage(n) gesendet, erwartet keine", strings.Join(args, " "), len(r.exchanges))
		}
		return r
	}
	// stopped checks that time status shows no timer
	stopped := func(clk func() time.Time, after string) {
		t.Helper()
		r := local(clk, "time", "status", "-o", "json")
		if r.code != 0 {
			s.errorf(t, "time status nach %s: Exit %d\n%s", after, r.code, r.stderr)
		} else if m := tryDecode(r.stdout); m == nil || m["running"] != false {
			s.errorf(t, "time status nach %s: %s, erwartet running false", after, strings.TrimSpace(r.stdout))
		}
	}

	// 1. time log from now-60 to now-30, run at now-30 (10), hard exit 0
	logComment := s.prefix + "zeit-log"
	s.need(t, 10)
	r := s.runIn(t, dir, clock(30), "", "time", "log", "--task", ids(task), "--user", ids(user), "--date", "heute",
		"--from", hhmm(60), "--to", hhmm(30), "-m", logComment, "-o", "json")
	logged := idOf(tryDecode(r.stdout))
	if logged != 0 {
		// registered before the hard check, like the entry of time stop
		s.track(object{resource: "time", id: logged, name: logComment})
		s.logf(t, "angelegt: time %d (%s)", logged, logComment)
	}
	if r.code != 0 || logged == 0 {
		s.fatalf(t, "time log: Exit %d, Eintrag %d\n%s", r.code, logged, r.stderr)
	}

	// 2. time list --since heute at now-30 (1), hard: the entry of 1 is
	// listed
	s.need(t, 1)
	r = s.runIn(t, dir, clock(30), "", "time", "list", "--since", "heute", "--until", "heute", "--limit", "100", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "time list --since heute --until heute: Exit %d\n%s", r.code, r.stderr)
	}
	found := false
	for _, e := range entries(s.decode(t, r.stdout)) {
		found = found || idOf(e) == logged
	}
	if !found {
		s.errorf(t, "time list --since heute --until heute: Eintrag %d fehlt", logged)
	}

	// 3. archive, unarchive (2), hard exit 0
	s.need(t, 2)
	if a := s.run(t, "", "time", "archive", ids(logged), "-o", "json"); a.code != 0 {
		s.errorf(t, "time archive: Exit %d\n%s", a.code, a.stderr)
	} else if u := s.run(t, "", "time", "unarchive", ids(logged), "-o", "json"); u.code != 0 {
		s.errorf(t, "time unarchive: Exit %d\n%s", u.code, u.stderr)
	}

	// 4. time start --unbillable at now-10 and time status (0), hard
	timerComment := s.prefix + "zeit-timer"
	r = local(clock(10), "time", "start", "--task", ids(task), "--user", ids(user), "-m", timerComment, "--unbillable", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "time start: Exit %d\n%s", r.code, r.stderr)
	}
	r = local(clock(10), "time", "status", "-o", "json")
	if m := tryDecode(r.stdout); r.code != 0 || m == nil || m["running"] != true || str(m["task"]) != ids(task) || m["unbillable"] != true {
		s.errorf(t, "time status nach time start: Exit %d, %s, erwartet running true, task %d und unbillable true\n%s",
			r.code, strings.TrimSpace(r.stdout), task, r.stderr)
	}

	// 5. time stop at now-8 (10), hard exit 0; then no timer, and a second
	// stop is exit 2 without a request
	s.need(t, 10)
	r = s.runIn(t, dir, clock(8), "", "time", "stop", "-o", "json")
	stop := idOf(tryDecode(r.stdout))
	if stop != 0 {
		// registered before a failure too: exit 11 shows the entry
		s.track(object{resource: "time", id: stop, name: timerComment})
		s.logf(t, "angelegt: time %d (%s)", stop, timerComment)
	}
	if r.code != 0 || stop == 0 {
		s.fatalf(t, "time stop: Exit %d, Eintrag %d\n%s", r.code, stop, r.stderr)
	}
	stopped(clock(8), "time stop")
	r = local(clock(8), "time", "stop", "-o", "json")
	if r.code != 2 || !strings.Contains(r.stderr, "Kein Timer läuft") {
		s.errorf(t, "zweites time stop: Exit %d, erwartet 2 („Kein Timer läuft“)\n%s", r.code, r.stderr)
	}

	// 6. time describe (1), hard: now-10 to now-8 of today, duration 120,
	// unbillable (from time start --unbillable)
	s.need(t, 1)
	m := s.ok(t, "", "time", "describe", ids(stop))
	for _, f := range []struct {
		key  string
		want time.Time
	}{{"started_at", at(10)}, {"ended_at", at(8)}} {
		if got, err := time.Parse(time.RFC3339, str(m[f.key])); err != nil || !got.Equal(f.want) {
			s.errorf(t, "time describe %d: %s %v, erwartet %s", stop, f.key, m[f.key], f.want.Format(time.RFC3339))
		}
	}
	if str(m["duration"]) != "120" {
		s.errorf(t, "time describe %d: duration %v, erwartet 120", stop, m["duration"])
	}
	if m["unbillable"] != true {
		s.errorf(t, "time describe %d: unbillable %v, erwartet true (time start --unbillable)", stop, m["unbillable"])
	}

	// 7. time start at now-5 and time discard (0), hard
	r = local(clock(5), "time", "start", "--task", ids(task), "--user", ids(user), "-m", timerComment, "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "time start (zum Verwerfen): Exit %d\n%s", r.code, r.stderr)
	}
	r = local(clock(5), "time", "discard", "--yes", "-o", "json")
	if d := tryDecode(r.stdout); r.code != 0 || len(d) != 1 || d["discarded"] != true {
		s.errorf(t, "time discard: Exit %d, %s, erwartet {\"discarded\":true}\n%s", r.code, strings.TrimSpace(r.stdout), r.stderr)
	}
	stopped(clock(5), "time discard")
}
