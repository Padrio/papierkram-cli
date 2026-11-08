package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/redact"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/timer"
)

// The local timer (spec 9.6): time start saves start, task, user, comment
// and unbillable of the effective context in a state file (internal/timer),
// time status shows it, time stop books the period through the path of
// time log (sendTimeEntry) and deletes the state only after the entry was
// created, time discard drops it. There is no timer on the server; only
// time stop sends a request. Before it sends, time stop claims the state
// file (timer.Claim, task 11): of two stops of one timer only one books
// it.

const timerNote = "Der Timer ist lokal (eine Datei je Kontext im Cache-Verzeichnis, PAPIERKRAM_CACHE_DIR); Papierkram kennt ihn nicht."

func (a *App) newTimeStartCmd() *cobra.Command {
	e, ok := registry.Lookup("time", "create")
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Timer starten (lokal, Buchung mit time stop)",
		Long: "Startet den Timer für den wirksamen Kontext: Startzeit (jetzt), Aufgabe, Benutzer, Kommentar und --unbillable werden gespeichert; " +
			"time stop bucht die Zeit bis dahin als Zeiteintrag.\n\n" +
			"Aufgabe und Benutzer werden jetzt bestimmt wie bei time log: Flag, sonst PAPIERKRAM_TASK bzw. PAPIERKRAM_USER, " +
			"sonst task bzw. user aus der Projektdatei .papierkram.toml (anlegen: papierkram local init), beim Benutzer zuletzt user des Kontexts. " +
			"--task nimmt auch einen Alias aus [tasks] der Projektdatei.\n\n" +
			"--unbillable (auch --unbillable=false, ja/nein) legt wie bei time log fest, ob der Eintrag nicht abrechenbar ist; der Wert wird jetzt geprüft (Exit 5) " +
			"und von time stop gesendet, ohne Angabe sendet time stop keinen. time stop kennt kein --unbillable.\n\n" +
			"Läuft schon ein Timer oder läuft gerade ein time stop bzw. wurde einer nicht abgeschlossen, bricht der Befehl ab (Exit 7). Unter Schreibschutz ist er gesperrt (Exit 9), weil time stop nicht buchen könnte. " +
			"Sendet keine Anfrage. " + timerNote,
		Example: "  papierkram time start --task 545 -m \"Abstimmung mit Kunde\"\n" +
			"  papierkram time start --task tickets --unbillable",
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
	addTaskUserFlags(fs)
	fs.StringP("comments", "m", "", "Kommentar (time stop -m ersetzt ihn)")
	addUnbillableFlag(fs)
	return cmd
}

func (a *App) newTimeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Laufenden Timer anzeigen",
		Long: "Zeigt den Timer des wirksamen Kontexts: Start, bisherige Dauer (h:mm nach Uhrzeit, wie time stop sie bucht, auch an Tagen der Zeitumstellung), " +
			"Aufgabe, Benutzer, Kommentar, ob der Eintrag nicht abrechenbar ist (JSON unbillable, -o wide Spalte NICHT ABRECHENBAR; " +
			"false bzw. leer auch ohne --unbillable beim Start) und das Konto (Host), für das er gestartet wurde (-o wide, JSON account; " +
			"gilt jetzt ein anderes, warnt der Befehl, denn time stop bucht dann nicht). Läuft gerade ein time stop oder wurde einer nicht abgeschlossen, zeigt der Befehl dessen Timer mit einer Warnung. Ohne Timer: „Kein Timer läuft“, mit -o json {\"running\":false} (Exit 0). " +
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
			"Außerdem sendet er den beim Start mit --unbillable gespeicherten Wert als unbillable (ohne --unbillable beim Start keinen); " +
			"time stop selbst hat kein --unbillable. " +
			"Stoppt der Timer in der ersten Minute des Folgetags (00:00:00 bis 00:00:59), endet der Eintrag um 24:00 des Starttags; " +
			"andere Einträge über Mitternacht und Einträge unter einer Minute werden abgelehnt (Exit 2). " +
			"Start und Ende gelten in der Zeitzone des Rechners beim Stoppen: Wechselt die Zeitzone zwischen time start und time stop " +
			"(z. B. auf Reisen), werden die Uhrzeiten des Stopp-Orts gebucht (Start 09:00 in Berlin, Stopp 11:00 in London ergibt 08:00–11:00); " +
			"die Dauer bleibt richtig.\n\n" +
			"Im Terminal zeigt der Befehl vorher eine Zusammenfassung und fragt nach (Enter = ja), wie time log; --dry-run zeigt die Anfrage. " +
			"Der Timer wird erst gelöscht, wenn der Zeiteintrag angelegt ist; bei jedem Fehler, Abbruch oder Probelauf läuft er weiter " +
			"(dann von Hand mit papierkram time log buchen oder mit papierkram time discard verwerfen).\n\n" +
			"Direkt vor dem Senden beansprucht der Befehl den Timer atomar (er legt <datei>.stopping exklusiv an und entfernt danach die Timer-Datei): " +
			"Ein zweiter time stop desselben Timers endet mit Exit 7, ohne etwas zu senden. Scheitert die Anfrage, wird die Timer-Datei wiederhergestellt. " +
			"Bleibt sie nach einem Absturz liegen, brechen time start und time stop mit Exit 7 ab, bis sie geprüft und mit papierkram time discard verworfen ist." +
			"\n\nKosten: " + creditsText(e.Credits) + " je Zeiteintrag.",
		Example:     "  papierkram time stop\n  papierkram time stop -m \"Fehler behoben\" --yes",
		Annotations: map[string]string{annotClass: string(registry.ClassCreate), annotOperation: e.OperationID},
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
			"Verwirft auch eine beschädigte Timer-Datei und die Datei eines nicht abgeschlossenen time stop (<datei>.stopping). Auch unter Schreibschutz erlaubt, da nur lokal. " + timerNote,
		Example:     "  papierkram time discard\n  papierkram time discard --yes",
		Annotations: map[string]string{annotLocal: "true"},
		Args:        noArgs,
		RunE:        func(c *cobra.Command, _ []string) error { return a.runTimeDiscard(c) },
	}
}

// runTimeStart saves a new timer. Order like time log: flags (exit 2),
// the fields of the request schema (exit 1 when the spec lacks one),
// configuration and read-only gate (mutationClient, exit 9: stop could not
// book), task and user (exit 2 when missing, R15), --unbillable (exit 5,
// task 12), a running or unusable timer (exit 7, exit 1), then the state
// file. No request is sent; the client is built so that a missing token
// shows now and not at stop.
func (a *App) runTimeStart(c *cobra.Command, e registry.Entry) error {
	if err := checkTaskUserFlags(c); err != nil {
		return err
	}
	if n := len(flagString(c, "comments")); n > timer.MaxComment {
		return usageErr(fmt.Sprintf("Kommentar zu lang (%d Bytes, höchstens %d KiB)", n, timer.MaxComment>>10))
	}
	fields, err := timeLogFieldsOf(e)
	if err != nil {
		return errUnavailable(err)
	}
	_, r, err := a.mutationClient(e)
	if err != nil {
		return err
	}
	if _, err := url.Parse(r.BaseURL); err != nil {
		// no request can be built with it: time stop could never book
		// the timer (review N3); the URL is not shown (credentials)
		return clierr.New(clierr.CodeUsage, "Basis-URL ist ungültig: "+redact.URL(r.BaseURL)+", Timer nicht gestartet",
			"PAPIERKRAM_BASE_URL bzw. die Subdomain des Kontexts prüfen")
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
	unbillable, err := a.timerUnbillable(c, e, fields)
	if err != nil {
		return err
	}
	path, cur, err := a.loadTimer(r)
	if err != nil {
		return err
	}
	if cur != nil {
		return a.errTimerRunning(*cur)
	}
	st := timer.State{
		Context:    r.Context,
		StartedAt:  a.Now().In(a.loc()).Truncate(time.Second),
		Task:       task.ID,
		User:       user.ID,
		Comment:    flagString(c, "comments"),
		BaseURL:    redact.URL(r.BaseURL), // never userinfo in the file (N1)
		Unbillable: unbillable,
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

// timerUnbillable is --unbillable of time start, checked now like time
// log checks it when it builds its body (spec.CoerceFlag with the field
// unbillable of the request schema, exit 5): a wrong value fails at start,
// not hours later at stop. nil without the flag: time stop sends none.
func (a *App) timerUnbillable(c *cobra.Command, e registry.Entry, fields map[string]spec.Field) (*bool, error) {
	if !c.Flags().Changed("unbillable") {
		return nil, nil
	}
	v, err := spec.CoerceFlag(a.dayContext(e.Resource), fields["unbillable"], rawText(c, "unbillable"))
	if err != nil {
		ce := *clierr.From(err)
		ce.Hint = strings.TrimSuffix(ce.Hint, ".")
		if ce.Hint != "" {
			ce.Hint += ". "
		}
		ce.Hint += "Timer nicht gestartet"
		return nil, &ce
	}
	b, ok := v.(bool)
	if !ok {
		return nil, errUnavailable(fmt.Errorf("unbillable im Request-Schema von %s ist kein Wahrheitswert", e.OperationID))
	}
	return &b, nil
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
	// Account is the host of the base URL the timer was started for, ""
	// for a state of the first format (review M8).
	Account string `json:"account"`
	// Unbillable is what time stop sends as unbillable; false also when
	// the timer has none (then nothing is sent, task 12).
	Unbillable bool `json:"unbillable"`
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
	Wide: []output.Column{
		{Header: "KONTEXT", Path: "context"},
		{Header: "KONTO", Path: "account"},
		{Header: "NICHT ABRECHENBAR", Path: "unbillable", Fmt: yesOrEmpty},
	},
}

// yesOrEmpty is a bool cell that shows only true (ja).
func yesOrEmpty(v any) string {
	if b, _ := v.(bool); b {
		return "ja"
	}
	return ""
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
		Account:         accountHost(st.BaseURL),
		Unbillable:      st.Unbillable != nil && *st.Unbillable,
	}
}

// accountHost is the printable host of a base URL, "" without one.
func accountHost(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	return printable(redact.Host(baseURL))
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
	_, st, stopping, err := a.readTimers(r)
	if err != nil {
		return err
	}
	if stopping {
		ce := a.errStopUnfinished(*st)
		output.Warn(a.Err, ce.Message+". "+ce.Hint)
	}
	if st != nil && !sameAccount(st.BaseURL, r.BaseURL) {
		// time stop would refuse (controller ruling C1): say so now
		ce := clierr.From(errTimerAccount(st.BaseURL, r.BaseURL))
		output.Warn(a.Err, ce.Message+". "+ce.Hint)
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
	if !sameAccount(st.BaseURL, r.BaseURL) {
		return errTimerAccount(st.BaseURL, r.BaseURL)
	}
	// eff is the timer with the comment of this stop (-m replaces it)
	eff := *st
	if c.Flags().Changed("comments") {
		eff.Comment = flagString(c, "comments")
	}
	p, err := a.timerPeriod(eff)
	if err != nil {
		return err
	}
	te := timeEntry{period: p, task: eff.Task, user: eff.User}
	if c.Flags().Changed("comments") || eff.Comment != "" {
		te.comments = &eff.Comment
	}
	if eff.Unbillable != nil {
		// the checked value of time start as text, like the flag of
		// time log; without one nothing is sent (task 12)
		v := strconv.FormatBool(*eff.Unbillable)
		te.unbillable = &v
	}
	// the claim (task 11, C2): after the question and --dry-run, right
	// before the request, so a dry run or a declined question never takes
	// the timer and of two stops only one sends
	var claimed bool
	var claimErr error
	claim := func() error {
		claimErr = a.claimTimer(path, *st, eff, p)
		claimed = claimErr == nil
		return claimErr
	}
	resp, err := a.sendTimeEntry(c, e, client, r, fields, te, claim)
	if err != nil {
		kept := err // an error of the claim is complete
		if claimErr == nil {
			kept = a.keepTimer(err, eff, p)
		}
		if claimed {
			if rerr := timer.Release(path); rerr != nil {
				return errNotReleased(path, rerr, kept)
			}
		}
		return kept
	}
	if resp == nil {
		_, _ = fmt.Fprintln(a.Err, "Hinweis: Probelauf – nichts gebucht, der Timer läuft weiter.")
		return nil
	}
	claimPath := timer.StoppingPath(path)
	delErr := timer.Delete(claimPath)
	outErr := a.writeResult(c, e, r, nil, resp.Body)
	if delErr != nil {
		ce := clierr.New(clierr.CodePartial,
			fmt.Sprintf("Zeiteintrag angelegt, aber Timer-Datei nicht gelöscht: %s", printable(delErr.Error())),
			fmt.Sprintf("NICHT erneut papierkram time stop ausführen (würde doppelt buchen); den Timer mit papierkram time discard verwerfen "+
				"oder die Datei %s von Hand löschen", printable(claimPath)))
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

// claimTimer claims the timer st at path for this stop (timer.Claim,
// ruling T11-A); on an error nothing is claimed and nothing was sent.
// Another stop's claim, or no file, is exit 7; a state file that is no
// longer st (in the meantime another stop booked it and a new timer was
// started, e.g. while this one asked) is exit 7 too. Any other error ends
// the stop before the request (exit 1), the timer stays.
func (a *App) claimTimer(path string, st, eff timer.State, p period) error {
	switch err := timer.Claim(path, st); {
	case err == nil:
		return nil
	case errors.Is(err, timer.ErrClaimed):
		return clierr.New(clierr.CodeConflict, "Timer wird bereits von einem anderen papierkram time stop gebucht", claimedHint)
	case errors.Is(err, timer.ErrChanged):
		return clierr.New(clierr.CodeConflict,
			"Timer wurde seit dem Lesen ersetzt (von einem anderen papierkram time stop gebucht und neu gestartet?)", claimedHint)
	default:
		ce := clierr.New(clierr.CodeError, "Timer-Datei nicht beansprucht: "+printable(clierr.From(err).Message), "Nichts gesendet")
		ce.Cause = err
		return a.keepTimer(ce, eff, p)
	}
}

// claimedHint is the hint of a stop that found the timer claimed or
// replaced: it sent nothing.
const claimedHint = "Nichts gesendet. Mit papierkram time status prüfen, ob er noch läuft"

// errNotReleased is the failure rerr to put the claim of path back after
// the stop failed with cause. When only the claim could not be removed
// (timer.ErrClaimLeft), it is cause with a hint to delete the claim.
// Otherwise it is exit 1: the claim stays, so time start and
// time stop refuse until it is renamed back or discarded. The hint keeps
// what went wrong before, also an unknown outcome.
func errNotReleased(path string, rerr, cause error) error {
	if errors.Is(rerr, timer.ErrClaimLeft) {
		// the timer runs again, only the claim is left over: the error of
		// the stop stays (its code tells what happened to the entry)
		ce := *clierr.From(cause)
		ce.Hint = strings.TrimSuffix(ce.Hint, ".") + ". Außerdem die Datei " + printable(timer.StoppingPath(path)) +
			" von Hand löschen (die Timer-Datei ist wiederhergestellt; sonst brechen time start und time stop mit Exit 7 ab)"
		return &ce
	}
	prev := clierr.From(cause)
	before := prev.Message
	if prev.Hint != "" {
		before += ". " + prev.Hint
	}
	ce := clierr.New(clierr.CodeError, "Timer-Datei nicht zurückgestellt: "+printable(rerr.Error()),
		fmt.Sprintf("Die Datei %s in %s umbenennen (dann läuft der Timer weiter) oder papierkram time discard. Zuvor gescheitert: %s",
			printable(timer.StoppingPath(path)), printable(path), before))
	ce.Cause = rerr
	return ce
}

// errStopUnfinished is the claim of a time stop that is still sending or
// did not finish (killed between the claim and the end); the two cannot
// be told apart, and the entry may exist. time start
// and time stop refuse (exit 7), time status warns with its text.
func (a *App) errStopUnfinished(st timer.State) *clierr.Error {
	v := a.timerStatus(st)
	start := st.StartedAt.In(a.loc())
	date := start.Format(time.DateOnly)
	return clierr.New(clierr.CodeConflict,
		fmt.Sprintf("Ein time stop läuft gerade oder wurde nicht abgeschlossen (Timer seit %s, %s)", output.FmtDate(v.StartedAt), timerSubject(st)),
		fmt.Sprintf("Erst mit papierkram time list --since %s prüfen, ob der Eintrag angelegt wurde. Läuft kein anderer time stop mehr: "+
			"wenn ja, papierkram time discard; sonst papierkram time discard und von Hand buchen mit %s", date, a.timeLogCommand(st, date, hhmmOf(start), "HH:MM")))
}

// sameAccount reports whether the base URL a timer was started with is
// the effective one, both without userinfo (other credentials for the
// same URL are the same account; the state holds none, N1). A state
// without base URL (first format) never matches. time start refuses an
// unparseable base URL, so a stored placeholder (redact.Invalid) comes
// only from an edited file; it parses as a relative path and never
// matches a base URL, so stop refuses it (exit 2).
func sameAccount(started, now string) bool {
	return started != "" && redact.URL(started) == redact.URL(now)
}

// errTimerAccount refuses to book a timer into another account than the
// one it was started for (controller ruling): its task and user IDs
// belong there. A state without base URL (first format) is a mismatch
// too. Exit 2 before any request; the timer stays.
func errTimerAccount(started, now string) error {
	from := "einem unbekannten Account (Timer-Datei ohne Angabe, ältere Version)"
	if started != "" {
		from = printable(redact.Host(started))
	}
	return clierr.New(clierr.CodeUsage,
		fmt.Sprintf("Timer wurde für %s gestartet, wirksam ist %s", from, printable(redact.Host(now))),
		"Der Timer läuft weiter. Zum Account des Starts zurückwechseln (Subdomain bzw. PAPIERKRAM_BASE_URL) und erneut papierkram time stop, "+
			"oder den Timer mit papierkram time discard verwerfen")
}

// timerPeriod is the period of st that time stop books: the date and the
// wall clock (minutes) of its start to the wall clock of now, like
// time log --date <start> --from <start> --to <now>. A stop in the first
// minute of the next day (00:00:00–00:00:59) ends at 24:00 of the start
// date, like time log --to 24:00 (open point 22). Any other period over
// midnight, under one minute or before the start (clock changed) is a
// usage error (exit 2) whose hint says how to go on; the timer stays.
func (a *App) timerPeriod(st timer.State) (period, error) {
	loc := a.loc()
	start, now := st.StartedAt.In(loc), a.Now().In(loc)
	p := period{date: start.Format(time.DateOnly), start: start.Hour()*60 + start.Minute(), end: now.Hour()*60 + now.Minute()}
	today := now.Format(time.DateOnly)
	if p.end == 0 && today == start.AddDate(0, 0, 1).Format(time.DateOnly) {
		p.end, today = day, p.date
	}
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
// hand: two when it ends on the next day, otherwise one per day. The
// first ends at 24:00 of the start date; a stop at 00:00 of the next day
// is booked by timerPeriod, so both halves have at least one minute.
func (a *App) midnightTimerHint(st timer.State, start, now time.Time) string {
	const keep = "Der Timer läuft weiter. "
	first := a.timeLogCommand(st, start.Format(time.DateOnly), hhmmOf(start), hhmm(day))
	if start.AddDate(0, 0, 1).Format(time.DateOnly) != now.Format(time.DateOnly) {
		return keep + "Je Tag einen Eintrag von Hand buchen, z. B. " + first + ", danach papierkram time discard"
	}
	return keep + "Von Hand buchen: " + first + " und " +
		a.timeLogCommand(st, now.Format(time.DateOnly), "00:00", hhmmOf(now)) + ", danach papierkram time discard"
}

// timeLogCommand is the time log command that books st from from to to on
// date by hand. The context of --context is quoted for the shell (a name
// may hold ; or $, review M7); a comment is named as a placeholder, since
// its text would need quoting for every shell. The unbillable of the
// timer is passed on as time stop would send it (task 12).
func (a *App) timeLogCommand(st timer.State, date, from, to string) string {
	cmd := fmt.Sprintf("papierkram time log --task %d --user %d --date %s --from %s --to %s", st.Task, st.User, date, from, to)
	switch {
	case st.Unbillable == nil:
	case *st.Unbillable:
		cmd += " --unbillable"
	default:
		cmd += " --unbillable=false"
	}
	if st.Comment != "" {
		cmd += ` -m "<Kommentar>"`
	}
	if a.Flags.Context != "" {
		cmd += " --context " + shellWord(a.Flags.Context)
	}
	return cmd
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// shellWord is s as one word of a POSIX shell: as is when it has only
// letters, digits, - and _, otherwise in single quotes (printable).
func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(printable(s), "'", `'\''`) + "'"
}

// keepTimer adds to an error of time stop after the timer was read that
// the timer stays and how to go on: after an unknown outcome or an answer
// too long to read (api.ErrResponseTooLarge, the entry exists) check first,
// after a declined question nothing else, after any other error book by
// hand or stop again.
func (a *App) keepTimer(err error, st timer.State, p period) error {
	ce := *clierr.From(err)
	var keep string
	switch {
	case ce.Code == clierr.CodeOutcomeUnknown, errors.Is(err, api.ErrResponseTooLarge):
		// a 2xx answer too long to read: the entry was created
		keep = "Der Timer läuft weiter: erst mit papierkram time list prüfen, ob der Eintrag angelegt wurde; " +
			"wenn ja, den Timer mit papierkram time discard verwerfen, sonst erneut papierkram time stop"
	case ce.Code == clierr.CodeServer:
		// a 5xx after the POST may still have created the entry
		keep = "Der Timer läuft weiter: erst mit papierkram time list prüfen, ob der Eintrag angelegt wurde; " +
			"wenn ja, den Timer mit papierkram time discard verwerfen, sonst erneut papierkram time stop oder von Hand buchen mit " +
			a.timeLogCommand(st, p.date, hhmm(p.start), hhmm(p.end)) + ", danach papierkram time discard"
	case ce.Code == clierr.CodeConfirmation:
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
	path, err := a.timerPath(r)
	if err != nil {
		return err
	}
	// the claim of an unfinished stop first, then the state file; both
	// are discarded, also when they cannot be used
	var what []string
	for _, stopping := range []bool{true, false} {
		st, err := a.timerState(r, path, stopping)
		switch {
		case err != nil:
			what = append(what, clierr.From(err).Message)
		case st != nil && stopping:
			what = append(what, a.errStopUnfinished(*st).Message)
		case st != nil:
			v := a.timerStatus(*st)
			what = append(what, fmt.Sprintf("Timer läuft seit %s (%s h): %s", output.FmtDate(v.StartedAt), v.Duration, timerSubject(*st)))
		}
	}
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	if len(what) == 0 {
		if !ownText(o) {
			return output.RenderValue(o, discardJSON{}, output.Columns{})
		}
		_, err := fmt.Fprintf(a.Out, "Kein Timer läuft (%s), nichts zu verwerfen.\n", contextLabel(r.Context))
		return err
	}
	if err := a.confirmDiscard(c.Context(), strings.Join(what, "; ")); err != nil {
		return err
	}
	for _, p := range []string{timer.StoppingPath(path), path} {
		if err := timer.Delete(p); err != nil {
			return clierr.New(clierr.CodeError, "Timer nicht verworfen: "+printable(err.Error()),
				"Die Timer-Datei "+printable(p)+" von Hand löschen")
		}
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
		return interruptedHint(err, nothingChanged+". Der Timer läuft weiter")
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
// edited or copied), is an error (exit 1) that names time discard. The
// claim of an unfinished time stop comes first: it is exit 7
// (errStopUnfinished), since its entry may exist.
func (a *App) loadTimer(r *config.Resolved) (string, *timer.State, error) {
	path, st, stopping, err := a.readTimers(r)
	if err == nil && stopping {
		return path, nil, a.errStopUnfinished(*st)
	}
	return path, st, err
}

// readTimers reads the claim of an unfinished time stop of the context of
// r and, without one, its state file; stopping reports that st is the
// claim. Errors as loadTimer.
func (a *App) readTimers(r *config.Resolved) (path string, st *timer.State, stopping bool, err error) {
	path, err = a.timerPath(r)
	if err != nil {
		return "", nil, false, err
	}
	if st, err = a.timerState(r, path, true); err != nil || st != nil {
		return path, st, st != nil, err
	}
	st, err = a.timerState(r, path, false)
	return path, st, false, err
}

// timerState reads the state file at path, or its claim with stopping,
// for the context of r: nil without one; one of another context is an
// error like an unusable one (exit 1, naming the file and time discard).
func (a *App) timerState(r *config.Resolved, path string, stopping bool) (*timer.State, error) {
	load, shown := timer.Load, path
	if stopping {
		load, shown = timer.LoadStopping, timer.StoppingPath(path)
	}
	st, err := load(path)
	if err != nil || st == nil {
		return nil, err
	}
	if st.Context != r.Context {
		return nil, clierr.New(clierr.CodeError,
			fmt.Sprintf("Timer-Datei %s ist ungültig: sie gehört zu %s, wirksam ist %s",
				printable(shown), contextLabel(st.Context), contextLabel(r.Context)),
			timer.DiscardHint)
	}
	return st, nil
}

// contextLabel names a context in texts; "" is direct mode.
func contextLabel(name string) string {
	if name == "" {
		return "ohne Kontext"
	}
	return fmt.Sprintf("Kontext %q", printable(name))
}

// timerSubject names task, user, comment and an unbillable of st.
func timerSubject(st timer.State) string {
	s := fmt.Sprintf("Aufgabe %d, Benutzer %d", st.Task, st.User)
	if st.Comment != "" {
		s += ", Kommentar „" + printable(st.Comment) + "“"
	}
	if st.Unbillable != nil && *st.Unbillable {
		s += ", nicht abrechenbar"
	}
	return s
}

// hhmmOf is the wall clock of t as HH:MM.
func hhmmOf(t time.Time) string { return hhmm(t.Hour()*60 + t.Minute()) }
