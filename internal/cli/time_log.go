package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// time log (spec 9.6): creates a time entry from a date, a period (start
// and end, or a duration) and task and user, which may come from presets
// (spec 9.8). It is no registry entry (the operation CreateTimeEntry
// belongs to time create); it uses that entry for class, credits, request
// schema and guards.

const timeLogLong = `Erfasst Zeit als Zeiteintrag: Datum, Zeitraum, Aufgabe und Benutzer.

Zeitraum: --from und --to (H:MM oder HH:MM) oder --duration (1h30m, 90m, 1:30), mit --from ab dieser Uhrzeit, ohne --from bis jetzt (nur für heute). Ein Zeiteintrag hat nur ein Datum: Einträge über Mitternacht werden abgelehnt; sie werden als zwei Einträge erfasst. Eine Dauer wird in Uhrzeit gerechnet, wie Papierkram sie bucht (Uhrzeiten ohne Zeitzone): --from 01:30 --duration 2h ergibt 01:30–03:30, auch an den Tagen der Zeitumstellung; --from und --to gelten wie angegeben.

Aufgabe und Benutzer: Flag, sonst PAPIERKRAM_TASK bzw. PAPIERKRAM_USER, sonst task bzw. user aus der Projektdatei .papierkram.toml (anlegen: papierkram local init), beim Benutzer zuletzt user des Kontexts. --task nimmt auch einen Alias aus [tasks] der Projektdatei.

Im Terminal zeigt der Befehl vorher eine Zusammenfassung mit Firma, Projekt, Aufgabe und Benutzer und fragt nach (Enter = ja); jeder Name kostet beim ersten Mal 1 Credit und wird 60 Sekunden zwischengespeichert. --yes, --no-input und Aufrufe ohne Terminal senden ohne Rückfrage.`

var timeLogExamples = []string{
	`papierkram time log --task 545 --from 9:00 --to 10:30 -m "Abstimmung mit Kunde"`,
	"papierkram time log --task tickets --duration 1h30m",
	"papierkram time log --date gestern --from 13:00 --duration 45m --unbillable --yes",
}

// timeLogFields map the flags of time log to the fields of the request
// schema of CreateTimeEntry; their values go through the same conversion
// (spec.CoerceFlag) as the field flags of time create.
var timeLogFields = []struct{ flag, field string }{
	{"date", "entry_date"},
	{"from", "started_at_time"},
	{"to", "ended_at_time"},
	{"task", "task"},
	{"user", "user"},
	{"comments", "comments"},
	{"unbillable", "unbillable"},
}

// newTimeLogCmd builds time log for the time group.
func (a *App) newTimeLogCmd() *cobra.Command {
	e, ok := registry.Lookup("time", "create")
	cmd := &cobra.Command{
		Use:         "log",
		Short:       "Zeit erfassen (Zeiteintrag mit Zeitraum, Aufgabe und Benutzer)",
		Long:        timeLogLong + "\n\nKosten: " + creditsText(e.Credits) + " je Zeiteintrag.",
		Example:     "  " + strings.Join(timeLogExamples, "\n  "),
		Annotations: map[string]string{annotClass: string(registry.ClassCreate)},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !ok || e.Class != registry.ClassCreate {
				return errUnavailable(fmt.Errorf("time create (CreateTimeEntry) fehlt in der Registry"))
			}
			return a.runTimeLog(c, e)
		},
	}
	fs := cmd.Flags()
	fs.String("task", "", "Aufgabe: ID oder Alias aus [tasks] der Projektdatei; ohne Angabe "+presetNote(presetTask))
	fs.Var(&rawValue{typ: "id"}, "user", "Benutzer-ID; ohne Angabe "+presetNote(presetUser)+" oder user des Kontexts")
	fs.Var(&rawValue{typ: "date"}, "date", "Datum: YYYY-MM-DD, today/heute, yesterday/gestern (Standard: heute)")
	fs.String("from", "", "Beginn als H:MM oder HH:MM")
	fs.String("to", "", "Ende als H:MM oder HH:MM (nicht mit --duration)")
	fs.String("duration", "", "Dauer: 1h30m, 90m oder 1:30; ohne --from endet der Eintrag jetzt")
	fs.StringP("comments", "m", "", "Kommentar")
	// a switch like the bool field flags of time create (also ja/nein)
	fs.VarPF(&rawValue{typ: "bool"}, "unbillable", "", "nicht abrechenbar").NoOptDefVal = "true"
	return cmd
}

// timeLogInput is what the flags of time log give: the period, or the
// error of --date, which is reported after the read-only gate like the
// date of time create (exit 5).
type timeLogInput struct {
	period  period
	dateErr error
}

// period is a time entry: its date and the start and end as wall-clock
// minutes after 00:00 of that date (one date, end after start; outside
// 0–1439 means another day).
//
// Controller ruling (Task 36): all arithmetic is wall-clock arithmetic.
// Papierkram stores entry_date plus HH:MM without offset, so the booked
// duration is the wall-clock difference; --duration 2h from 01:30 books
// 01:30–03:30 on every day, the days of the daylight saving time change
// included, and --from/--to are taken as given.
type period struct {
	date       string
	start, end int
}

// runTimeLog creates the time entry. Order (plan, as in runWriteThen):
// flags (exit 2: combinations, formats, period over midnight), the fields
// of the request schema (exit 1 when the spec lacks one), configuration
// and read-only gate (mutationClient, exit 9), task and user (exit 2 when
// missing, R15), body (exit 5: --date and the validation against the
// request schema), --dry-run (beforeSend), the summary with names and the
// question in a terminal (exit 8 when declined), then the request.
func (a *App) runTimeLog(c *cobra.Command, e registry.Entry) error {
	in, err := a.checkTimeLog(c, e)
	if err != nil {
		return err
	}
	fields, err := timeLogFieldsOf(e)
	if err != nil {
		return errUnavailable(err)
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	task, user, err := a.timeLogPresets(c, r)
	if err != nil {
		return err
	}
	if in.dateErr != nil {
		return in.dateErr
	}
	body, err := a.timeLogBody(c, e, fields, in.period, task.ID, user.ID)
	if err != nil {
		return err
	}
	a.flushPresetHints()
	if proceed, err := a.beforeSend(c, e, client, nil, body, sendExtras{}); !proceed || err != nil {
		return err
	}
	if a.StdinTTY && !a.Flags.NoInput && !a.Flags.Yes {
		if err := a.confirmTimeLog(c.Context(), client, r, in.period, task.ID, user.ID, body); err != nil {
			return err
		}
	}
	resp, err := client.Call(c.Context(), e.OperationID, nil, nil, body)
	if err != nil {
		return err
	}
	return a.writeResult(c, e, r, nil, resp.Body)
}

// checkTimeLog checks the flags before anything is resolved or sent
// (exit 2) and computes the period. An invalid --date is kept as dateErr
// (exit 5 after the gate); the period is then not checked.
func (a *App) checkTimeLog(c *cobra.Command, e registry.Entry) (timeLogInput, error) {
	fs := c.Flags()
	for _, key := range []string{presetTask, presetUser} {
		if !fs.Changed(key) {
			continue
		}
		v := rawText(c, key)
		if _, ok := parseID(v); ok || (key == presetTask && v != "" && !strings.HasPrefix(v, "-")) {
			continue
		}
		want := "ID (ganze Zahl ≥ 0) erwartet"
		if key == presetTask {
			want = "ID (ganze Zahl ≥ 0) oder Alias aus [tasks] erwartet"
		}
		return timeLogInput{}, usageErr(fmt.Sprintf("ungültiger Wert %q für --%s: %s", v, key, want))
	}
	from, to, dur := fs.Changed("from"), fs.Changed("to"), fs.Changed("duration")
	usage := "Verwendung: " + useLine(c) + " --from HH:MM --to HH:MM | --duration 1h30m [--from HH:MM]"
	switch {
	case dur && to:
		return timeLogInput{}, clierr.New(clierr.CodeUsage, "--to und --duration nicht zusammen verwenden",
			"Zeitraum entweder mit --from und --to oder mit --duration (optional ab --from) angeben")
	case !dur && !from && !to:
		return timeLogInput{}, clierr.New(clierr.CodeUsage, "Zeitraum fehlt: --from und --to oder --duration angeben", usage)
	case !dur && !to:
		return timeLogInput{}, clierr.New(clierr.CodeUsage, "--to fehlt: zu --from gehört --to oder --duration", usage)
	case !dur && !from:
		return timeLogInput{}, clierr.New(clierr.CodeUsage, "--from fehlt: zu --to gehört --from", usage)
	}
	var startClock, endClock clock
	var d time.Duration
	var err error
	if from {
		if startClock, err = parseClock("--from", flagString(c, "from")); err != nil {
			return timeLogInput{}, err
		}
	}
	if to {
		if endClock, err = parseClock("--to", flagString(c, "to")); err != nil {
			return timeLogInput{}, err
		}
	}
	if dur {
		if d, err = parseLogDuration(flagString(c, "duration")); err != nil {
			return timeLogInput{}, err
		}
	}

	f, ok := fieldByName(e, "entry_date")
	if !ok {
		return timeLogInput{}, nil // timeLogFieldsOf reports it
	}
	f.Flag = "date"
	rawDate := "today"
	if fs.Changed("date") {
		rawDate = rawText(c, "date")
	}
	v, err := spec.CoerceFlag(spec.CoerceContext{Now: a.Now, Loc: a.Location}, f, rawDate)
	if err != nil {
		return timeLogInput{dateErr: err}, nil
	}
	date, _ := v.(string)

	// wall-clock arithmetic, see period
	p := period{date: date}
	minutes := int(d / time.Minute)
	switch {
	case dur && !from:
		// ends at the wall clock of now (minute precision), on today's date
		now := a.Now().In(a.loc())
		if today := now.Format(time.DateOnly); date != today {
			return timeLogInput{}, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("--duration ohne --from endet jetzt (heute, %s), nicht am %s", today, date),
				"für einen anderen Tag zusätzlich --from angeben")
		}
		p.end = now.Hour()*60 + now.Minute()
		p.start = p.end - minutes
	default:
		p.start = startClock.minutes()
		if dur {
			p.end = p.start + minutes
		} else {
			p.end = endClock.minutes()
		}
	}
	return timeLogInput{period: p}, checkPeriod(p)
}

// midnightHint explains why a period over midnight is rejected.
const midnightHint = "Ein Zeiteintrag hat nur ein Datum: über Mitternacht als zwei Einträge erfassen (bis 23:59 und ab 00:00 am Folgetag mit --date)"

// checkPeriod rejects a period whose wall-clock times do not lie within
// its date or whose end does not follow its start (spec 9.6, exit 2).
func checkPeriod(p period) error {
	const day = 24 * 60
	switch {
	case p.start < 0:
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Zeiteintrag beginnt am Vortag um %s: Einträge über Mitternacht sind nicht möglich", hhmm(p.start+day)),
			midnightHint)
	case p.end >= day:
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Zeiteintrag endet am Folgetag um %s: Einträge über Mitternacht sind nicht möglich", hhmm(p.end%day)),
			midnightHint)
	case p.end == p.start:
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Dauer 0: Ende %s ist gleich dem Beginn", hhmm(p.end)),
			"--to nach --from angeben oder --duration verwenden")
	case p.end < p.start:
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Ende %s liegt vor dem Beginn %s", hhmm(p.end), hhmm(p.start)),
			"Uhrzeiten vertauscht? Einträge über Mitternacht sind nicht möglich. "+midnightHint)
	}
	return nil
}

// loc is the local time zone of the app (nil: time.Local).
func (a *App) loc() *time.Location {
	if a.Location != nil {
		return a.Location
	}
	return time.Local
}

// clock is a wall-clock time of day.
type clock struct{ h, m int }

// minutes is c as minutes after 00:00.
func (c clock) minutes() int { return c.h*60 + c.m }

var clockRe = regexp.MustCompile(`^([0-9]{1,2}):([0-9]{2})$`)

// parseClock parses H:MM or HH:MM (00:00–23:59).
func parseClock(flag, raw string) (clock, error) {
	m := clockRe.FindStringSubmatch(raw)
	if m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		if h < 24 && mi < 60 {
			return clock{h, mi}, nil
		}
	}
	return clock{}, usageErr(fmt.Sprintf("ungültiger Wert %q für %s: Uhrzeit als HH:MM oder H:MM erwartet (00:00 bis 23:59, z. B. 09:00)", raw, flag))
}

var durationClockRe = regexp.MustCompile(`^([0-9]+):([0-5][0-9])$`)

// parseLogDuration parses a positive duration in whole minutes: a Go
// duration with h and m (1h30m, 90m) or H:MM (1:30).
func parseLogDuration(raw string) (time.Duration, error) {
	var d time.Duration
	if m := durationClockRe.FindStringSubmatch(raw); m != nil {
		h, err := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		if err != nil || h > 24*365 {
			return 0, usageErr(fmt.Sprintf("ungültiger Wert %q für --duration: Dauer zu groß", raw))
		}
		d = time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute
	} else {
		var err error
		if d, err = time.ParseDuration(raw); err != nil {
			return 0, usageErr(fmt.Sprintf("ungültiger Wert %q für --duration: Dauer wie 1h30m, 90m oder 1:30 erwartet", raw))
		}
	}
	switch {
	case d <= 0:
		return 0, usageErr(fmt.Sprintf("ungültiger Wert %q für --duration: Dauer muss größer als 0 sein", raw))
	case d%time.Minute != 0:
		return 0, usageErr(fmt.Sprintf("ungültiger Wert %q für --duration: Dauer in ganzen Minuten angeben (Zeiteinträge haben Uhrzeiten HH:MM)", raw))
	}
	return d, nil
}

// hhmm formats wall-clock minutes after 00:00 (0–1439) as HH:MM.
func hhmm(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// timeLogPresets resolves task and user after the read-only gate: flag >
// PAPIERKRAM_TASK/PAPIERKRAM_USER > project file (of the effective
// context) > the user of the context (spec 9.6, 9.8). A missing value is
// a usage error that points to papierkram local init (R15). Values that
// do not come from a flag are named in a hint.
func (a *App) timeLogPresets(c *cobra.Command, r *config.Resolved) (task, user preset, err error) {
	src := func(key string) presetSources {
		s := presetSources{Env: a.Env, File: a.presetFile}
		if c.Flags().Changed(key) {
			v := rawText(c, key)
			s.Flag = &v
		}
		return s
	}
	if task, err = resolvePreset(presetTask, src(presetTask)); err != nil {
		return
	}
	us := src(presetUser)
	us.Context, us.ContextName = r.User, r.Context
	if user, err = resolvePreset(presetUser, us); err != nil {
		return
	}
	var missing, flags, init, where []string
	if task.Source == "" {
		missing, flags, init = append(missing, "Aufgabe"), append(flags, "--task"), append(init, "--task <id>")
		where = append(where, "--task, "+presetEnv(presetTask)+" oder task in "+projectfile.FileName)
	}
	if user.Source == "" {
		missing, flags, init = append(missing, "Benutzer"), append(flags, "--user"), append(init, "--user <id>")
		where = append(where, "--user, "+presetEnv(presetUser)+", user in "+projectfile.FileName+" oder user des Kontexts")
	}
	if len(missing) > 0 {
		verb := "fehlt"
		if len(missing) > 1 {
			verb = "fehlen"
		}
		return task, user, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("%s %s (%s)", strings.Join(missing, " und "), verb, strings.Join(where, "; ")),
			strings.Join(flags, " und ")+" angeben oder für dieses Verzeichnis voreinstellen: papierkram local init "+strings.Join(init, " "))
	}
	for _, p := range []struct {
		key string
		p   preset
	}{{presetTask, task}, {presetUser, user}} {
		if p.p.Source != config.SourceFlag {
			a.presetHint(p.key, p.p)
		}
	}
	return task, user, nil
}

// fieldByName is the top-level field name of the request schema of e.
func fieldByName(e registry.Entry, name string) (spec.Field, bool) {
	ws := newWriteSpec(e)
	if ws.err != nil {
		return spec.Field{}, false
	}
	for _, f := range ws.fields {
		if len(f.Path) == 1 && f.Path[0] == name {
			return f, true
		}
	}
	return spec.Field{}, false
}

// timeLogFieldsOf are the fields of timeLogFields by flag, with the flag
// name set so conversion errors name --date and not --entry-date. A field
// missing in the spec is an error: the command is not available then.
func timeLogFieldsOf(e registry.Entry) (map[string]spec.Field, error) {
	ws := newWriteSpec(e)
	if ws.err != nil {
		return nil, ws.err
	}
	out := map[string]spec.Field{}
	for _, tf := range timeLogFields {
		f, ok := fieldByName(e, tf.field)
		if !ok {
			return nil, fmt.Errorf("im Request-Schema von %s fehlt das Feld %s", e.OperationID, tf.field)
		}
		f.Flag = tf.flag
		out[tf.flag] = f
	}
	return out, nil
}

// timeLogBody builds the request body like time create builds it from its
// field flags (flagSets and buildBodyFrom): every value as text through
// spec.CoerceFlag with the field of the request schema and the app clock,
// set at the field path, then spec.CoerceValue and spec.Validate against
// the request schema (exit 5). Comments and unbillable are only sent when
// given, like unset field flags.
func (a *App) timeLogBody(c *cobra.Command, e registry.Entry, fields map[string]spec.Field, p period, task, user int64) (map[string]any, error) {
	raw := map[string]string{
		"date": p.date,
		"from": hhmm(p.start),
		"to":   hhmm(p.end),
		"task": strconv.FormatInt(task, 10),
		"user": strconv.FormatInt(user, 10),
	}
	if c.Flags().Changed("comments") {
		raw["comments"] = flagString(c, "comments")
	}
	if c.Flags().Changed("unbillable") {
		raw["unbillable"] = rawText(c, "unbillable")
	}
	cc := spec.CoerceContext{Now: a.Now, Loc: a.Location}
	body := map[string]any{}
	for _, tf := range timeLogFields {
		text, ok := raw[tf.flag]
		if !ok {
			continue
		}
		f := fields[tf.flag]
		v, err := spec.CoerceFlag(cc, f, text)
		if err != nil {
			return nil, err
		}
		setPath(body, f.Path, v)
	}
	req := spec.RequestSchema(e.OperationID)
	v, err := spec.CoerceValue(req, body, "")
	if err != nil {
		return nil, err
	}
	body, _ = v.(map[string]any)
	if err := spec.Validate(body, req); err != nil {
		return nil, err
	}
	return body, nil
}
