package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

// local init|show (spec 9.8): the project file .papierkram.toml of a
// directory, which presets context, company, project, task and user.

func newLocalCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Projektdatei .papierkram.toml anlegen und anzeigen",
		Long: "Die Projektdatei " + projectfile.FileName + " legt für ein Verzeichnis Kontext, Firma, Projekt, Aufgabe und Benutzer fest " +
			"(gesucht im aktuellen Verzeichnis, in einem Git-Repository aufwärts bis zur Git-Wurzel). " +
			"Vorrang: Flag > Umgebung (PAPIERKRAM_PROJECT, PAPIERKRAM_TASK, …) > Projektdatei > Kontext. " +
			"Die Werte gelten nur, wenn der wirksame Kontext der Kontext der Datei ist.",
		Args: noArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newLocalInitCmd(app), newLocalShowCmd(app))
	return cmd
}

// localKeys are the ID keys of local init in the order of its prompts,
// with the German prompt.
var localKeys = []struct{ key, prompt string }{
	{presetCompany, "Firma-ID"},
	{presetProject, "Projekt-ID"},
	{presetTask, "Aufgabe (ID oder Alias aus [tasks])"},
	{presetUser, "Benutzer-ID"},
}

// localInit are the values given to local init: the IDs by key, a task
// alias to resolve with the file, and the context.
type localInit struct {
	ids       map[string]int64
	taskAlias string
	context   *string
}

func (l localInit) empty() bool { return len(l.ids) == 0 && l.taskAlias == "" && l.context == nil }

func newLocalInitCmd(app *App) *cobra.Command {
	var in localInit
	cmd := &cobra.Command{
		Use:         "init",
		Annotations: writesConfig(),
		Short:       "Projektdatei anlegen oder ändern",
		Long: "Schreibt Werte in die Projektdatei " + projectfile.FileName + ": in die gefundene Datei, sonst neu ins aktuelle Verzeichnis. " +
			"Nur die angegebenen Schlüssel ändern sich; andere Schlüssel und [tasks] bleiben, Kommentare und Formatierung gehen beim Schreiben verloren.\n\n" +
			"Im Terminal ohne Flags fragt der Befehl jeden Wert ab (Enter behält den bisherigen). " +
			"Ohne Terminal oder mit --no-input sind Flags nötig. --context nennt hier den Kontext, für den die Datei gilt. " +
			"Unter PAPIERKRAM_READ_ONLY ist der Befehl gesperrt.",
		Example: "  papierkram local init --project 4963 --task 545\n" +
			"  papierkram local init --context firma-gmbh --company 3352 --user 1\n" +
			"  papierkram local init --task tickets",
		// flags are checked with the arguments, before the read-only guard
		// (usage errors come first)
		Args: func(c *cobra.Command, args []string) error {
			if err := noArgs(c, args); err != nil {
				return err
			}
			var err error
			if in, err = localInitFlags(c); err != nil {
				return err
			}
			if in.empty() && (!app.StdinTTY || app.Flags.NoInput) {
				return clierr.New(clierr.CodeUsage, "keine Werte angegeben",
					"ohne Terminal Werte per Flag angeben, z. B. "+useLine(c)+" --project <id> --task <id>")
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error { return app.runLocalInit(c, in) },
	}
	fs := cmd.Flags()
	fs.Var(&rawValue{typ: "id"}, "company", "Firma-ID")
	fs.Var(&rawValue{typ: "id"}, "project", "Projekt-ID")
	fs.String("task", "", "Aufgaben-ID oder Alias aus [tasks] der Projektdatei")
	fs.Var(&rawValue{typ: "id"}, "user", "Benutzer-ID")
	// shadows the global --context: the context of the file, not the
	// context of this command
	fs.String("context", "", "Kontext, für den die Projektdatei gilt")
	return cmd
}

// localInitFlags parses the flags of local init: IDs > 0 (the project file
// takes no others), a task alias, a valid context name (exit 2).
func localInitFlags(c *cobra.Command) (localInit, error) {
	in := localInit{ids: map[string]int64{}}
	for _, k := range localKeys {
		if !c.Flags().Changed(k.key) {
			continue
		}
		raw := rawText(c, k.key)
		id, err := strconv.ParseInt(raw, 10, 64)
		switch {
		case err == nil && id > 0:
			in.ids[k.key] = id
		case k.key == presetTask && err != nil && raw != "" && !strings.HasPrefix(raw, "-"):
			in.taskAlias = raw
		default:
			return in, usageErr(fmt.Sprintf("ungültiger Wert %q für --%s: positive ganze Zahl erwartet", raw, k.key))
		}
	}
	if c.Flags().Changed("context") {
		name := flagString(c, "context")
		if err := config.ValidateName(name); err != nil {
			return in, err
		}
		in.context = &name
	}
	return in, nil
}

// runLocalInit writes the values into the project file found from the
// working directory, or a new one there. Without values (terminal, no
// flags) it asks for each.
func (a *App) runLocalInit(c *cobra.Command, in localInit) error {
	f, err := a.projectFile()
	if err != nil {
		return err
	}
	path := ""
	if f != nil {
		path = f.Path
	} else {
		cwd, err := a.Getwd()
		if err != nil {
			return clierr.New(clierr.CodeError, "Arbeitsverzeichnis nicht ermittelbar: "+err.Error(), "")
		}
		path = filepath.Join(cwd, projectfile.FileName)
	}
	if in.empty() {
		if in, err = a.askLocalInit(c.Context(), f, path); err != nil {
			return err
		}
	}
	if in.taskAlias != "" {
		id, err := f.ResolveTask(in.taskAlias)
		if err != nil {
			return err
		}
		in.ids[presetTask] = id
	}
	ch := projectfile.Changes{Context: in.context}
	for key, dst := range map[string]**int64{presetCompany: &ch.Company, presetProject: &ch.Project, presetTask: &ch.Task, presetUser: &ch.User} {
		if id, ok := in.ids[key]; ok {
			*dst = &id
		}
	}
	if ch.Context != nil {
		a.warnUnknownContext(*ch.Context)
	} else if f != nil && f.Context != "" {
		a.warnOtherContext(f.Context)
	}
	res, err := a.saveProjectFile(path, ch)
	if err != nil {
		return err
	}
	if res.HadComments {
		output.Warn(a.Err, "Kommentare und Formatierung der Projektdatei sind beim Schreiben verloren gegangen")
	}
	var b strings.Builder
	shown := printable(path)
	switch {
	case res.Created:
		fmt.Fprintf(&b, "Projektdatei %s angelegt:\n", shown)
	case res.Changed:
		fmt.Fprintf(&b, "Projektdatei %s aktualisiert:\n", shown)
	case in.empty():
		fmt.Fprintf(&b, "Keine Werte angegeben, Projektdatei %s nicht geschrieben.\n", shown)
	default:
		fmt.Fprintf(&b, "Projektdatei %s unverändert.\n", shown)
	}
	if res.Changed {
		if ch.Context != nil {
			fmt.Fprintf(&b, "  context = %q\n", *ch.Context)
		}
		for _, k := range localKeys {
			if id, ok := in.ids[k.key]; ok {
				fmt.Fprintf(&b, "  %s = %d\n", k.key, id)
			}
		}
	}
	_, err = fmt.Fprint(a.Out, b.String())
	return err
}

// saveProjectFile writes the project file. It repeats the hard read-only
// check of the central guard (which ran before the command) right at the
// write, so no path writes the file past it.
func (a *App) saveProjectFile(path string, ch projectfile.Changes) (projectfile.SaveResult, error) {
	if err := a.guardConfigWrite(); err != nil {
		return projectfile.SaveResult{}, err
	}
	res, err := projectfile.Save(path, ch)
	var ce *clierr.Error
	if err != nil && !errors.As(err, &ce) {
		e := clierr.New(clierr.CodeError, "Projektdatei nicht geschrieben: "+err.Error(), "Schreibrechte im Verzeichnis prüfen")
		e.Cause = err
		return res, e
	}
	return res, err
}

// warnUnknownContext warns when the context of the project file is not in
// the configuration: commands in this directory would not find it.
func (a *App) warnUnknownContext(name string) {
	cfg, _, err := a.loadConfig()
	if err != nil || cfg.Contexts[name] != nil {
		return
	}
	output.Warn(a.Err, fmt.Sprintf("Kontext %q ist in der Konfiguration nicht angelegt; anlegen: papierkram context create %s --subdomain <subdomain>", name, name))
}

// warnOtherContext warns when the file being written belongs to another
// than the effective context (PAPIERKRAM_CONTEXT, or none in direct
// mode): its values only apply under its own context (review M3). Best
// effort: a configuration that cannot be resolved gives no warning.
func (a *App) warnOtherContext(fileCtx string) {
	ctx, source, err := a.effectiveContext()
	if err != nil || ctx == fileCtx {
		return
	}
	active := "kein Kontext ist aktiv"
	if ctx != "" {
		active = fmt.Sprintf("aktiv ist Kontext %q (%s)", ctx, contextSourceName(source))
	}
	output.Warn(a.Err, fmt.Sprintf("Die Projektdatei gilt für Kontext %q, %s: die geschriebenen Werte gelten nur unter Kontext %q", fileCtx, active, fileCtx))
}

// askLocalInit asks for each value on stderr (terminal); Enter keeps the
// current value of the file. An invalid answer is asked again; the end of
// the input keeps the rest.
func (a *App) askLocalInit(ctx context.Context, f *projectfile.File, path string) (localInit, error) {
	in := localInit{ids: map[string]int64{}}
	state := "neu"
	if f != nil {
		state = "vorhanden"
	}
	_, _ = fmt.Fprintf(a.Err, "Projektdatei %s (%s); Enter behält den bisherigen Wert.\n", printable(path), state)
	for _, k := range localKeys {
		q := k.prompt
		if cur := fileValue(f, k.key); cur != 0 {
			q += fmt.Sprintf(" [%d]", cur)
		}
		for {
			answer, err := a.prompt(ctx, q+": ")
			if err != nil {
				return in, err
			}
			answer = strings.TrimSpace(answer)
			if answer == "" {
				break
			}
			if id, err := strconv.ParseInt(answer, 10, 64); err == nil && id > 0 {
				in.ids[k.key] = id
				break
			}
			if k.key == presetTask && f != nil {
				if id, err := f.ResolveTask(answer); err == nil {
					in.ids[k.key] = id
					break
				}
			}
			_, _ = fmt.Fprintf(a.Err, "Eingabe %q ungültig: positive ganze Zahl erwartet\n", printable(answer))
		}
	}
	q := "Kontext"
	if f != nil && f.Context != "" {
		q += " [" + printable(f.Context) + "]"
	}
	for {
		answer, err := a.prompt(ctx, q+": ")
		if err != nil {
			return in, err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			break
		}
		if err := config.ValidateName(answer); err != nil {
			_, _ = fmt.Fprintf(a.Err, "Eingabe %q ungültig: %s\n", printable(answer), clierr.From(err).Message)
			continue
		}
		in.context = &answer
		break
	}
	return in, nil
}

// localRow is one effective value of local show. Source is flag, env,
// projectfile, config (active_context), context (context default, user
// only) or default (not set); Origin names the flag, variable, file or
// context.
type localRow struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source string `json:"source"`
	Origin string `json:"origin"`
}

// localShowJSON is the output of local show: the project file (null
// without one), whether its values are ignored because another context is
// effective, the context it names, and the effective values.
type localShowJSON struct {
	ProjectFile        *string    `json:"project_file"`
	ProjectFileContext string     `json:"project_file_context"`
	ProjectFileIgnored bool       `json:"project_file_ignored"`
	Values             []localRow `json:"values"`
}

var localColumns = output.Columns{
	Table: []output.Column{
		{Header: "SCHLÜSSEL", Path: "key"},
		{Header: "WERT", Path: "value"},
		{Header: "HERKUNFT", Path: "source"},
		{Header: "QUELLE", Path: "origin"},
	},
}

func newLocalShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "show",
		Annotations: readsOnly(),
		Short:       "Wirksame Werte mit Herkunft anzeigen",
		Long: "Zeigt Kontext, Firma, Projekt, Aufgabe und Benutzer, wie sie in diesem Verzeichnis gelten, mit Herkunft: " +
			"flag, env, projectfile, config (active_context), context (Standard des Kontexts, nur user) oder default (nicht gesetzt). " +
			"Gilt die Projektdatei für einen anderen Kontext als den wirksamen, werden ihre Werte ignoriert; das steht in der Ausgabe. Sendet keine Anfrage.",
		Example: "  papierkram local show\n  papierkram local show --context firma-gmbh -o json",
		Args:    noArgs,
		RunE:    func(c *cobra.Command, _ []string) error { return app.runLocalShow(c) },
	}
}

// runLocalShow resolves the values like the commands that use them
// (resolvePreset, config resolution without keychain) and prints them.
func (a *App) runLocalShow(c *cobra.Command) error {
	f, err := a.projectFile()
	if err != nil {
		return err
	}
	r, err := a.resolveWithoutToken()
	if err != nil {
		return err
	}
	v := localShowJSON{}
	file := func() (*projectfile.File, error) { return f, nil }
	if f != nil {
		v.ProjectFile, v.ProjectFileContext = &f.Path, f.Context
		if f.Context != "" && f.Context != r.Context {
			// the presets of the file are ignored (see presetFile); the
			// output says so instead of a warning
			v.ProjectFileIgnored = true
			file = func() (*projectfile.File, error) { return nil, nil }
		}
	}
	ctxRow := localRow{Key: "context", Source: r.ContextSource, Origin: contextSourceName(r.ContextSource)}
	if r.Context == "" {
		ctxRow.Source, ctxRow.Origin = "default", ""
	} else {
		ctxRow.Value = r.Context
		if r.ContextSource == config.SourceProjectFile && f != nil {
			ctxRow.Origin = f.Path
		}
	}
	v.Values = append(v.Values, ctxRow)
	for _, key := range []string{presetCompany, presetProject, presetTask, presetUser} {
		src := presetSources{Env: a.Env, File: file}
		if key == presetUser {
			src.Context, src.ContextName = r.User, r.Context
		}
		p, err := resolvePreset(key, src)
		if err != nil {
			return err
		}
		row := localRow{Key: key, Source: p.Source, Origin: p.Origin}
		if p.Source == "" {
			row.Source = "default"
		} else {
			row.Value = p.ID
		}
		v.Values = append(v.Values, row)
	}
	// the format of the effective context like auth status (review M5)
	o := a.outputOptions(c.Context(), a.outputFormat(r))
	// path, context and origins come from an untrusted local file and
	// directory: printable in every format (YAML, --jq and --fields print
	// strings raw; review M1)
	v = printableLocalShow(v)
	if !ownText(o) {
		return output.RenderValue(o, v, output.Columns{})
	}
	if err := output.RenderValue(o, v.Values, localColumns); err != nil {
		return err
	}
	var b strings.Builder
	switch {
	case f == nil:
		b.WriteString("\nProjektdatei: keine gefunden\n")
	case v.ProjectFileIgnored:
		active := "kein Kontext aktiv"
		if r.Context != "" {
			active = fmt.Sprintf("aktiv ist Kontext %q (%s)", r.Context, contextSourceName(r.ContextSource))
		}
		fmt.Fprintf(&b, "\nProjektdatei: %s\nSie gilt für Kontext %q, %s: ihre Werte werden ignoriert.\n", printable(f.Path), f.Context, active)
	default:
		fmt.Fprintf(&b, "\nProjektdatei: %s\n", printable(f.Path))
	}
	_, err = fmt.Fprint(a.Out, b.String())
	return err
}

// printableLocalShow is v with every text made printable: the project
// file path and context and the value and origin of every row.
func printableLocalShow(v localShowJSON) localShowJSON {
	if v.ProjectFile != nil {
		p := printable(*v.ProjectFile)
		v.ProjectFile = &p
	}
	v.ProjectFileContext = printable(v.ProjectFileContext)
	rows := make([]localRow, len(v.Values))
	for i, row := range v.Values {
		row.Origin = printable(row.Origin)
		if s, ok := row.Value.(string); ok {
			row.Value = printable(s)
		}
		rows[i] = row
	}
	v.Values = rows
	return v
}
