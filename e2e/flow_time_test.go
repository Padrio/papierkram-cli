//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// Spec 9.6, 9.8, AGENTS "time stop": time tracking live (time log, time
// list --since heute, archive, unarchive) and the local timer (start,
// status, stop, discard). Hard: every step; the timer commands other than
// stop send nothing (0 credits), stop without a timer is exit 2, and the
// stopped entry spans 11:00 to 11:02 of today with duration 120 and is
// unbillable (time start --unbillable, task 12).
//
// All runs share one directory (the state file of the timer lies below
// <dir>/cache/papierkram/) and a clock set to today in s.loc (execAt):
// heute of time log and time list is the date of the timer steps, so the
// flow never waits a minute and never crosses midnight (review of task 5).
func TestE2E_TimeFlow(t *testing.T) {
	s := live(t)

	task, user := s.task(t), s.user(t)
	dir := t.TempDir()
	today := time.Now().In(s.loc)
	at := func(h, m int) time.Time {
		return time.Date(today.Year(), today.Month(), today.Day(), h, m, 0, 0, s.loc)
	}
	clock := func(h, m int) func() time.Time {
		tm := at(h, m)
		return func() time.Time { return tm }
	}
	// local runs a timer command that sends nothing: hard 0 requests
	local := func(now func() time.Time, args ...string) result {
		t.Helper()
		r := s.runIn(t, dir, now, "", args...)
		if len(r.exchanges) != 0 {
			s.errorf(t, "papierkram %s: %d Anfrage(n) gesendet, erwartet keine", strings.Join(args, " "), len(r.exchanges))
		}
		return r
	}
	// stopped checks that time status shows no timer
	stopped := func(now func() time.Time, after string) {
		t.Helper()
		r := local(now, "time", "status", "-o", "json")
		if r.code != 0 {
			s.errorf(t, "time status nach %s: Exit %d\n%s", after, r.code, r.stderr)
		} else if m := tryDecode(r.stdout); m == nil || m["running"] != false {
			s.errorf(t, "time status nach %s: %s, erwartet running false", after, strings.TrimSpace(r.stdout))
		}
	}

	// 1. time log at 08:30 (10), hard exit 0
	logComment := s.prefix + "zeit-log"
	s.need(t, 10)
	r := s.runIn(t, dir, clock(8, 30), "", "time", "log", "--task", ids(task), "--user", ids(user), "--date", "heute",
		"--from", "08:00", "--to", "08:30", "-m", logComment, "-o", "json")
	logged := idOf(tryDecode(r.stdout))
	if logged != 0 {
		// registered before the hard check, like the entry of time stop
		s.track(object{resource: "time", id: logged, name: logComment})
		s.logf(t, "angelegt: time %d (%s)", logged, logComment)
	}
	if r.code != 0 || logged == 0 {
		s.fatalf(t, "time log: Exit %d, Eintrag %d\n%s", r.code, logged, r.stderr)
	}

	// 2. time list --since heute at 08:30 (1), hard: the entry of 1 is listed
	s.need(t, 1)
	r = s.runIn(t, dir, clock(8, 30), "", "time", "list", "--since", "heute", "--until", "heute", "--limit", "100", "-o", "json")
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

	// 4. time start --unbillable at 11:00 and time status (0), hard
	timerComment := s.prefix + "zeit-timer"
	r = local(clock(11, 0), "time", "start", "--task", ids(task), "--user", ids(user), "-m", timerComment, "--unbillable", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "time start: Exit %d\n%s", r.code, r.stderr)
	}
	r = local(clock(11, 0), "time", "status", "-o", "json")
	if m := tryDecode(r.stdout); r.code != 0 || m == nil || m["running"] != true || str(m["task"]) != ids(task) || m["unbillable"] != true {
		s.errorf(t, "time status nach time start: Exit %d, %s, erwartet running true, task %d und unbillable true\n%s",
			r.code, strings.TrimSpace(r.stdout), task, r.stderr)
	}

	// 5. time stop at 11:02 (10), hard exit 0; then no timer, and a second
	// stop is exit 2 without a request
	s.need(t, 10)
	r = s.runIn(t, dir, clock(11, 2), "", "time", "stop", "-o", "json")
	stop := idOf(tryDecode(r.stdout))
	if stop != 0 {
		// registered before a failure too: exit 11 shows the entry
		s.track(object{resource: "time", id: stop, name: timerComment})
		s.logf(t, "angelegt: time %d (%s)", stop, timerComment)
	}
	if r.code != 0 || stop == 0 {
		s.fatalf(t, "time stop: Exit %d, Eintrag %d\n%s", r.code, stop, r.stderr)
	}
	stopped(clock(11, 2), "time stop")
	r = local(clock(11, 2), "time", "stop", "-o", "json")
	if r.code != 2 || !strings.Contains(r.stderr, "Kein Timer läuft") {
		s.errorf(t, "zweites time stop: Exit %d, erwartet 2 („Kein Timer läuft“)\n%s", r.code, r.stderr)
	}

	// 6. time describe (1), hard: 11:00 to 11:02 of today, duration 120,
	// unbillable (from time start --unbillable)
	s.need(t, 1)
	m := s.ok(t, "", "time", "describe", ids(stop))
	for _, f := range []struct {
		key  string
		want time.Time
	}{{"started_at", at(11, 0)}, {"ended_at", at(11, 2)}} {
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

	// 7. time start at 12:00 and time discard (0), hard
	r = local(clock(12, 0), "time", "start", "--task", ids(task), "--user", ids(user), "-m", timerComment, "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "time start (zum Verwerfen): Exit %d\n%s", r.code, r.stderr)
	}
	r = local(clock(12, 0), "time", "discard", "--yes", "-o", "json")
	if d := tryDecode(r.stdout); r.code != 0 || len(d) != 1 || d["discarded"] != true {
		s.errorf(t, "time discard: Exit %d, %s, erwartet {\"discarded\":true}\n%s", r.code, strings.TrimSpace(r.stdout), r.stderr)
	}
	stopped(clock(12, 0), "time discard")
}
