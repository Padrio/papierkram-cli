package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/timer"
)

// The local timer (spec 9.6): time start saves start, task, user and
// comment of the effective context in a state file (internal/timer),
// time status shows it, time stop books the period through the path of
// time log (sendTimeEntry) and deletes the state only after the entry was
// created, time discard drops it. There is no timer on the server; only
// time stop sends a request.

const timerNote = "Der Timer ist lokal (eine Datei je Kontext im Cache-Verzeichnis, PAPIERKRAM_CACHE_DIR); Papierkram kennt ihn nicht."

func (a *App) newTimeStartCmd() *cobra.Command {
	e, ok := registry.Lookup("time", "create")
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Timer starten (lokal, Buchung mit time stop)",
		Long: "Startet den Timer für den wirksamen Kontext: Startzeit (jetzt), Aufgabe, Benutzer und Kommentar werden gespeichert; " +
			"time stop bucht die Zeit bis dahin als Zeiteintrag.\n\n" +
			"Aufgabe und Benutzer werden jetzt bestimmt wie bei time log: Flag, sonst PAPIERKRAM_TASK bzw. PAPIERKRAM_USER, " +
			"sonst task bzw. user aus der Projektdatei .papierkram.toml (anlegen: papierkram local init), beim Benutzer zuletzt user des Kontexts. " +
			"--task nimmt auch einen Alias aus [tasks] der Projektdatei.\n\n" +
			"Läuft schon ein Timer, bricht der Befehl ab (Exit 7). Unter Schreibschutz ist er gesperrt (Exit 9), weil time stop nicht buchen könnte. " +
			"Sendet keine Anfrage. " + timerNote,
		Example: "  papierkram time start --task 545 -m \"Abstimmung mit Kunde\"\n" +
			"  papierkram time start --task tickets",
		Annotations: map[string]string{annotClass: string(registry.ClassCreate), annotLocal: "true"},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !ok || e.Class != registry.ClassCreate {
				return errUnavailable(fmt.Errorf("time create (CreateTimeEntry) fehlt in der Registry"))
			}
			return a.runTimeStart(c, e)
		},
	}
	fs := cmd.Flags()
	fs.String("task", "", "Aufgabe: ID oder Alias aus [tasks] der Projektdatei; ohne Angabe "+presetNote(presetTask))
	fs.Var(&rawValue{typ: "id"}, "user", "Benutzer-ID; ohne Angabe "+presetNote(presetUser)+" oder user des Kontexts")
	fs.StringP("comments", "m", "", "Kommentar (time stop -m ersetzt ihn)")
	return cmd
}

func (a *App) newTimeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Laufenden Timer anzeigen",
		Long: "Zeigt den Timer des wirksamen Kontexts: Start, bisherige Dauer (h:mm nach Uhrzeit, wie time stop sie bucht, auch an Tagen der Zeitumstellung), " +
			"Aufgabe, Benutzer und Kommentar. Ohne Timer: „Kein Timer läuft“, mit -o json {\"running\":false} (Exit 0). " +
			"Sendet keine Anfrage. " + timerNote,
		Example:     "  papierkram time status\n  papierkram time status -o json",
		Annotations: readsOnly(),
		Args:        noArgs,
		RunE:        func(c *cobra.Command, _ []string) error { return a.runTimeStatus(c) },
	}
}

func (a *App) newTimeStopCmd() *cobra.Command {
	e, ok := registry.Lookup("time", "create")
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Timer stoppen und als Zeiteintrag buchen",
		Long: "Bucht die Zeit des Timers wie time log: Datum und Uhrzeit des Starts bis jetzt (in ganzen Minuten), " +
			"Aufgabe, Benutzer und Kommentar des Timers; -m ersetzt den Kommentar. " +
			"Einträge über Mitternacht und unter einer Minute werden abgelehnt (Exit 2).\n\n" +
			"Im Terminal zeigt der Befehl vorher eine Zusammenfassung und fragt nach (Enter = ja), wie time log; --dry-run zeigt die Anfrage. " +
			"Der Timer wird erst gelöscht, wenn der Zeiteintrag angelegt ist; bei jedem Fehler, Abbruch oder Probelauf läuft er weiter " +
			"(dann von Hand mit papierkram time log buchen oder mit papierkram time discard verwerfen)." +
			"\n\nKosten: " + creditsText(e.Credits) + " je Zeiteintrag.",
		Example:     "  papierkram time stop\n  papierkram time stop -m \"Fehler behoben\" --yes",
		Annotations: map[string]string{annotClass: string(registry.ClassCreate)},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !ok || e.Class != registry.ClassCreate {
				return errUnavailable(fmt.Errorf("time create (CreateTimeEntry) fehlt in der Registry"))
			}
			return a.runTimeStop(c, e)
		},
	}
	cmd.Flags().StringP("comments", "m", "", "Kommentar statt des Kommentars aus time start")
	return cmd
}

func (a *App) newTimeDiscardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discard",
		Short: "Timer verwerfen, ohne zu buchen",
		Long: "Löscht den Timer des wirksamen Kontexts, ohne einen Zeiteintrag anzulegen: die erfasste Zeit geht verloren. " +
			"Im Terminal fragt der Befehl nach (Enter = nein); ohne Terminal oder mit --no-input ist --yes nötig (sonst Exit 8). " +
			"Verwirft auch eine beschädigte Timer-Datei. Auch unter Schreibschutz erlaubt, da nur lokal. " + timerNote,
		Example:     "  papierkram time discard\n  papierkram time discard --yes",
		Annotations: map[string]string{annotLocal: "true"},
		Args:        noArgs,
		RunE:        func(c *cobra.Command, _ []string) error { return a.runTimeDiscard(c) },
	}
}

// runTimeStart saves a new timer. Order like time log: flags (exit 2),
// configuration and read-only gate (mutationClient, exit 9: stop could not
// book), task and user (exit 2 when missing, R15), a running or unusable
// timer (exit 7, exit 1), then the state file. No request is sent; the
// client is built so that a missing token shows now and not at stop.
func (a *App) runTimeStart(c *cobra.Command, e registry.Entry) error {
	if err := checkTaskUserFlags(c); err != nil {
		return err
	}
	if n := len(flagString(c, "comments")); n > timer.MaxComment {
		return usageErr(fmt.Sprintf("Kommentar zu lang (%d Bytes, höchstens %d KiB)", n, timer.MaxComment>>10))
	}
	_, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	task, user, err := a.timeLogPresets(c, r)
	if err != nil {
		return err
	}
	// ID 0 passes the preset rules (R24) and is rejected by the server
	// at once on time log; a timer would only fail at stop, hours later
	// (review I1)
	for _, p := range []struct {
		key string
		p   preset
	}{{presetTask, task}, {presetUser, user}} {
		if p.p.ID <= 0 {
			return clierr.New(clierr.CodeUsage,
				fmt.Sprintf("%s %d aus %s ist ungültig: der Timer braucht eine ID größer als 0", resourceNoun(p.key), p.p.ID, printable(p.p.Origin)),
				"Timer nicht gestartet. Gültige ID angeben, z. B. "+useLine(c)+" --"+p.key+" <id>")
		}
	}
	path, cur, err := a.loadTimer(r)
	if err != nil {
		return err
	}
	if cur != nil {
		return a.errTimerRunning(*cur)
	}
	st := timer.State{
		Context:   r.Context,
		StartedAt: a.Now().In(a.loc()).Truncate(time.Second),
		Task:      task.ID,
		User:      user.ID,
		Comment:   flagString(c, "comments"),
		BaseURL:   r.BaseURL,
	}
	if err := timer.Save(path, st); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// started by another invocation since the check
			if cur, lerr := timer.Load(path); lerr == nil && cur != nil {
				return a.errTimerRunning(*cur)
			}
			return clierr.New(clierr.CodeConflict, "Es läuft bereits ein Timer", timer.DiscardHint)
		}
		return clierr.New(clierr.CodeError, "Timer nicht gespeichert: "+printable(err.Error()),
			"Cache-Verzeichnis prüfen (PAPIERKRAM_CACHE_DIR)")
	}
	a.flushPresetHints()
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if !ownText(o) {
		return afterRequest(output.RenderValue(o, a.timerStatus(st), output.Columns{}))
	}
	_, err = fmt.Fprintf(a.Out, "Timer gestartet um %s (%s): %s.\n",
		hhmmOf(st.StartedAt.In(a.loc())), contextLabel(r.Context), timerSubject(st))
	return afterRequest(err)
}

// errTimerRunning is the conflict of time start with a running timer.
func (a *App) errTimerRunning(st timer.State) error {
	v := a.timerStatus(st)
	return clierr.New(clierr.CodeConflict,
		fmt.Sprintf("Timer läuft bereits seit %s (%s h): %s", output.FmtDate(v.StartedAt), v.Duration, timerSubject(st)),
		"Erst buchen mit papierkram time stop oder verwerfen mit papierkram time discard")
}

// timerStatusJSON is a running timer in the output of time status and
// time start. Texts from the state file are printable.
type timerStatusJSON struct {
	Running         bool   `json:"running"`
	Context         string `json:"context"`
	StartedAt       string `json:"started_at"`
	Duration        string `json:"duration"`
	DurationMinutes int    `json:"duration_minutes"`
	Task            int64  `json:"task"`
	User            int64  `json:"user"`
	Comment         string `json:"comment"`
}

// timerStoppedJSON is the output of time status without a timer.
type timerStoppedJSON struct {
	Running bool `json:"running"`
}

var timerColumns = output.Columns{
	Table: []output.Column{
		{Header: "GESTARTET", Path: "started_at", Fmt: output.FmtDate},
		{Header: "DAUER", Path: "duration"},
		{Header: "AUFGABE", Path: "task"},
		{Header: "BENUTZER", Path: "user"},
		{Header: "KOMMENTAR", Path: "comment"},
	},
	Wide: []output.Column{{Header: "KONTEXT", Path: "context"}},
}

// timerStatus is st as output, its duration at the app clock.
func (a *App) timerStatus(st timer.State) timerStatusJSON {
	start := st.StartedAt.In(a.loc())
	m := timerMinutes(start, a.Now().In(a.loc()))
	return timerStatusJSON{
		Running:         true,
		Context:         printable(st.Context),
		StartedAt:       start.Format(time.RFC3339),
		Duration:        fmt.Sprintf("%d:%02d", m/60, m%60),
		DurationMinutes: m,
		Task:            st.Task,
		User:            st.User,
		Comment:         printable(st.Comment),
	}
}

// timerMinutes is the wall-clock difference from start to now (both in
// the app's time zone) in whole minutes: calendar days times 24 hours
// plus the difference of the clock times. On the date of the start it is
// what time stop books (wall-clock arithmetic, see period), also on the
// days of the time change (review M2); 0 when the clock is before the
// start.
func timerMinutes(start, now time.Time) int {
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	days := int(day(now).Sub(day(start)) / (24 * time.Hour))
	m := days*24*60 + now.Hour()*60 + now.Minute() - start.Hour()*60 - start.Minute()
	if m < 0 {
		return 0
	}
	return m
}

// runTimeStatus shows the timer of the effective context. It resolves the
// configuration like local show (no token needed) and uses its output
// format.
func (a *App) runTimeStatus(c *cobra.Command) error {
	r, err := a.resolveWithoutToken()
	if err != nil {
		return err
	}
	_, st, err := a.loadTimer(r)
	if err != nil {
		return err
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	switch {
	case st == nil && !ownText(o):
		return output.RenderValue(o, timerStoppedJSON{}, output.Columns{})
	case st == nil:
		_, err := fmt.Fprintf(a.Out, "Kein Timer läuft (%s).\n", contextLabel(r.Context))
		return err
	case !ownText(o):
		return output.RenderValue(o, a.timerStatus(*st), output.Columns{})
	}
	return output.RenderValue(o, []timerStatusJSON{a.timerStatus(*st)}, timerColumns)
}

// runTimeStop books the timer like time log and deletes it after the
// entry was created. Order like time log: flags (exit 2), the fields of
// the request schema, configuration and read-only gate (mutationClient,
// exit 9), then the timer, which stands for the arguments of time log and
// is read after the gate like its presets (exit 1 when unusable, exit 2
// without one or for a period that cannot be booked), then the shared
// path: body (exit 5), --dry-run, summary and question, request. Every
// failure keeps the timer and names how to go on.
func (a *App) runTimeStop(c *cobra.Command, e registry.Entry) error {
	fields, err := timeLogFieldsOf(e)
	if err != nil {
		return errUnavailable(err)
	}
	client, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	path, st, err := a.loadTimer(r)
	if err != nil {
		return err
	}
	if st == nil {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Kein Timer läuft (%s)", contextLabel(r.Context)),
			"Timer starten: papierkram time start --task <id>; ohne Timer buchen: papierkram time log")
	}
	if st.BaseURL != r.BaseURL {
		return errTimerAccount(st.BaseURL, r.BaseURL)
	}
	p, err := a.timerPeriod(*st)
	if err != nil {
		return err
	}
	te := timeEntry{period: p, task: st.Task, user: st.User}
	comment := st.Comment
	if c.Flags().Changed("comments") {
		comment = flagString(c, "comments")
		te.comments = &comment
	} else if comment != "" {
		te.comments = &comment
	}
	// not guarded: two concurrent stops of one timer may both book it
	resp, err := a.sendTimeEntry(c, e, client, r, fields, te)
	if err != nil {
		return a.keepTimer(err, *st, p)
	}
	if resp == nil {
		_, _ = fmt.Fprintln(a.Err, "Hinweis: Probelauf – nichts gebucht, der Timer läuft weiter.")
		return nil
	}
	delErr := timer.Delete(path)
	outErr := a.writeResult(c, e, r, nil, resp.Body)
	if delErr != nil {
		ce := clierr.New(clierr.CodePartial,
			fmt.Sprintf("Zeiteintrag angelegt, aber Timer-Datei nicht gelöscht: %s", printable(delErr.Error())),
			"NICHT erneut papierkram time stop ausführen (würde doppelt buchen); den Timer mit papierkram time discard verwerfen")
		ce.Cause = delErr
		return ce
	}
	if outErr != nil {
		return outErr
	}
	d := p.end - p.start
	_, _ = fmt.Fprintf(a.Err, "Timer gestoppt: %s %s–%s (%d:%02d h) gebucht.\n", p.date, hhmm(p.start), hhmm(p.end), d/60, d%60)
	return nil
}

// errTimerAccount refuses to book a timer into another account than the
// one it was started for (controller ruling): its task and user IDs
// belong there. A state without base URL (first format) is a mismatch
// too. Exit 2 before any request; the timer stays.
func errTimerAccount(started, now string) error {
	from := "einem unbekannten Account (Timer-Datei ohne Angabe, ältere Version)"
	if started != "" {
		from = printable(urlHost(started))
	}
	return clierr.New(clierr.CodeUsage,
		fmt.Sprintf("Timer wurde für %s gestartet, wirksam ist %s", from, printable(urlHost(now))),
		"Der Timer läuft weiter. Zum Account des Starts zurückwechseln (Subdomain bzw. PAPIERKRAM_BASE_URL) und erneut papierkram time stop, "+
			"oder den Timer mit papierkram time discard verwerfen")
}

// urlHost is the host (with port) of a base URL, the URL itself when it
// has none.
func urlHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// timerPeriod is the period of st that time stop books: the date and the
// wall clock (minutes) of its start to the wall clock of now, like
// time log --date <start> --from <start> --to <now>. A period over
// midnight, under one minute or before the start (clock changed) is a
// usage error (exit 2) whose hint says how to go on; the timer stays.
func (a *App) timerPeriod(st timer.State) (period, error) {
	loc := a.loc()
	start, now := st.StartedAt.In(loc), a.Now().In(loc)
	p := period{date: start.Format(time.DateOnly), start: start.Hour()*60 + start.Minute(), end: now.Hour()*60 + now.Minute()}
	today := now.Format(time.DateOnly)
	since := p.date + " " + hhmm(p.start)
	switch {
	case now.Before(start):
		return p, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Startzeit des Timers (%s) liegt in der Zukunft (jetzt %s %s)", since, today, hhmm(p.end)),
			"Systemuhr prüfen. Der Timer läuft weiter; verwerfen mit papierkram time discard")
	case today != p.date:
		return p, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Timer läuft seit %s über Mitternacht (jetzt %s %s): Einträge über Mitternacht sind nicht möglich", since, today, hhmm(p.end)),
			a.midnightTimerHint(st, start, now))
	case p.end == p.start && now.Sub(start) >= time.Minute:
		// the same clock time an hour later: the clock went back
		return p, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Timer läuft seit %s, die Uhr wurde seither umgestellt: nach Uhrzeit Dauer 0, kein Zeiteintrag möglich", since),
			"Der Timer läuft weiter: Zeit von Hand mit "+a.timeLogCommand(st, p.date, "HH:MM", "HH:MM")+" buchen, danach papierkram time discard")
	case p.end == p.start:
		return p, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Timer läuft erst seit %s, weniger als eine Minute: zu kurz für einen Zeiteintrag", hhmm(p.start)),
			"Der Timer läuft weiter: später erneut papierkram time stop oder verwerfen mit papierkram time discard")
	}
	if err := checkPeriod(p); err != nil {
		// the wall clock went back (end of daylight saving time, clock set)
		ce := *clierr.From(err)
		ce.Hint = "Uhr umgestellt? Der Timer läuft weiter: Zeit von Hand mit " + a.timeLogCommand(st, p.date, "HH:MM", "HH:MM") +
			" buchen, danach papierkram time discard"
		return p, &ce
	}
	return p, nil
}

// midnightTimerHint names the entries that book a timer over midnight by
// hand: two when it ends on the next day, otherwise one per day.
func (a *App) midnightTimerHint(st timer.State, start, now time.Time) string {
	const keep = "Der Timer läuft weiter. "
	from, end := hhmmOf(start), hhmmOf(now)
	if start.AddDate(0, 0, 1).Format(time.DateOnly) != now.Format(time.DateOnly) {
		return keep + "Je Tag einen Eintrag von Hand buchen, z. B. " +
			a.timeLogCommand(st, start.Format(time.DateOnly), from, "23:59") + ", danach papierkram time discard"
	}
	var cmds []string
	if from != "23:59" {
		cmds = append(cmds, a.timeLogCommand(st, start.Format(time.DateOnly), from, "23:59"))
	}
	if end != "00:00" {
		cmds = append(cmds, a.timeLogCommand(st, now.Format(time.DateOnly), "00:00", end))
	}
	return keep + "Von Hand buchen: " + strings.Join(cmds, " und ") + ", danach papierkram time discard"
}

// timeLogCommand is the time log command that books st from from to to on
// date by hand (the comment is left out: shell quoting).
func (a *App) timeLogCommand(st timer.State, date, from, to string) string {
	cmd := fmt.Sprintf("papierkram time log --task %d --user %d --date %s --from %s --to %s", st.Task, st.User, date, from, to)
	if a.Flags.Context != "" {
		cmd += " --context " + printable(a.Flags.Context)
	}
	return cmd
}

// keepTimer adds to an error of time stop after the timer was read that
// the timer stays and how to go on: after an unknown outcome check first
// (the entry may exist), after a declined question nothing else, after
// any other error book by hand or stop again.
func (a *App) keepTimer(err error, st timer.State, p period) error {
	ce := *clierr.From(err)
	var keep string
	switch ce.Code {
	case clierr.CodeOutcomeUnknown:
		keep = "Der Timer läuft weiter: erst mit papierkram time list prüfen, ob der Eintrag angelegt wurde; " +
			"wenn ja, den Timer mit papierkram time discard verwerfen, sonst erneut papierkram time stop"
	case clierr.CodeConfirmation:
		keep = "Der Timer läuft weiter"
	default:
		keep = "Der Timer läuft weiter: erneut papierkram time stop oder von Hand buchen mit " +
			a.timeLogCommand(st, p.date, hhmm(p.start), hhmm(p.end)) + ", danach papierkram time discard"
	}
	if ce.Hint != "" {
		keep = strings.TrimSuffix(ce.Hint, ".") + ". " + keep
	}
	ce.Hint = keep
	return &ce
}

// runTimeDiscard deletes the timer of the effective context after the
// confirmation. It is local, so the read-only modes do not block it, and
// it needs no token. A state file that cannot be read is discarded too.
func (a *App) runTimeDiscard(c *cobra.Command) error {
	r, err := a.resolveWithoutToken()
	if err != nil {
		return err
	}
	path, st, lerr := a.loadTimer(r)
	if path == "" {
		return lerr
	}
	if st == nil && lerr != nil {
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			lerr = nil // a broken cache directory holds no timer
		}
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if st == nil && lerr == nil {
		if !ownText(o) {
			return output.RenderValue(o, discardJSON{}, output.Columns{})
		}
		_, err := fmt.Fprintf(a.Out, "Kein Timer läuft (%s), nichts zu verwerfen.\n", contextLabel(r.Context))
		return err
	}
	var what string
	if st != nil {
		v := a.timerStatus(*st)
		what = fmt.Sprintf("Timer läuft seit %s (%s h): %s", output.FmtDate(v.StartedAt), v.Duration, timerSubject(*st))
	} else {
		what = clierr.From(lerr).Message
	}
	if err := a.confirmDiscard(c.Context(), what); err != nil {
		return err
	}
	if err := timer.Delete(path); err != nil {
		return clierr.New(clierr.CodeError, "Timer nicht verworfen: "+printable(err.Error()),
			"Die Timer-Datei von Hand löschen")
	}
	if !ownText(o) {
		return afterRequest(output.RenderValue(o, discardJSON{Discarded: true}, output.Columns{}))
	}
	_, err = fmt.Fprintln(a.Out, "Timer verworfen.")
	return afterRequest(err)
}

// discardJSON is the output of time discard.
type discardJSON struct {
	Discarded bool `json:"discarded"`
}

// confirmDiscard asks before the timer is discarded, since the tracked
// time is lost: --yes passes, without a terminal or with --no-input it is
// exit 8, in a terminal only j/ja/y/yes confirm (Enter and the end of the
// input are no).
func (a *App) confirmDiscard(ctx context.Context, what string) error {
	if a.Flags.Yes {
		return nil
	}
	if !a.StdinTTY || a.Flags.NoInput {
		return clierr.New(clierr.CodeConfirmation,
			"Bestätigung erforderlich: Timer verwerfen (die erfasste Zeit geht verloren)",
			"--yes verwenden (ohne Terminal und mit --no-input wird nicht nachgefragt)")
	}
	_, _ = fmt.Fprintln(a.Err, printable(what))
	answer, eof, err := a.ask(ctx, "Timer verwerfen? Die erfasste Zeit geht verloren. [j/N]: ")
	if err != nil {
		return err
	}
	if eof {
		_, _ = fmt.Fprintln(a.Err)
		return clierr.New(clierr.CodeConfirmation, "Abgebrochen: Eingabe beendet", "Der Timer läuft weiter")
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "j", "ja", "y", "yes":
		return nil
	}
	return clierr.New(clierr.CodeConfirmation, "Abgebrochen durch Nutzer", "Der Timer läuft weiter. Ohne Rückfrage mit --yes")
}

// timerPath is the state file of the context of r. Without a cache
// directory there is no place for a timer (exit 1).
func (a *App) timerPath(r *config.Resolved) (string, error) {
	dir := a.cacheDir()
	if dir == "" {
		return "", clierr.New(clierr.CodeError, "Kein Cache-Verzeichnis für den Timer gefunden",
			"PAPIERKRAM_CACHE_DIR auf ein Verzeichnis setzen")
	}
	return timer.Path(dir, r.Context), nil
}

// loadTimer reads the timer of the context of r: nil without one. A state
// that cannot be used, also one that names another context (the file was
// edited or copied), is an error (exit 1) that names time discard.
func (a *App) loadTimer(r *config.Resolved) (string, *timer.State, error) {
	path, err := a.timerPath(r)
	if err != nil {
		return "", nil, err
	}
	st, err := timer.Load(path)
	if err != nil || st == nil {
		return path, nil, err
	}
	if st.Context != r.Context {
		return path, nil, clierr.New(clierr.CodeError,
			fmt.Sprintf("Timer-Datei %s ist ungültig: sie gehört zu %s, wirksam ist %s",
				printable(path), contextLabel(st.Context), contextLabel(r.Context)),
			timer.DiscardHint)
	}
	return path, st, nil
}

// contextLabel names a context in texts; "" is direct mode.
func contextLabel(name string) string {
	if name == "" {
		return "ohne Kontext"
	}
	return fmt.Sprintf("Kontext %q", printable(name))
}

// timerSubject names task, user and comment of st.
func timerSubject(st timer.State) string {
	s := fmt.Sprintf("Aufgabe %d, Benutzer %d", st.Task, st.User)
	if st.Comment != "" {
		s += ", Kommentar „" + printable(st.Comment) + "“"
	}
	return s
}

// hhmmOf is the wall clock of t as HH:MM.
func hhmmOf(t time.Time) string { return hhmm(t.Hour()*60 + t.Minute()) }
