package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// Presets (spec 9.8) are values a command falls back to when they are not
// given, e.g. the project of task list. Order: flag > environment
// (PAPIERKRAM_<KEY>) > project file > context default (user only) > none.

// The preset keys: the ID keys of the project file.
const (
	presetCompany = "company"
	presetProject = "project"
	presetTask    = "task"
	presetUser    = "user"
)

// preset is the effective value of a preset key. Source is "flag", "env",
// "projectfile" or "context", "" when nothing sets the value; Origin names
// where it comes from (flag, environment variable, project file path or
// context name) for hints and local show.
type preset struct {
	ID             int64
	Source, Origin string
}

// presetSources are the inputs of resolvePreset, highest priority first.
// File is called only when the project file decides (or resolves a task
// alias); it returns nil without a project file.
type presetSources struct {
	Flag        *string // nil: flag not given
	Env         func(string) string
	File        func() (*projectfile.File, error)
	Context     int64 // context default, 0: none
	ContextName string
}

// presetEnv is the environment variable of a preset key.
func presetEnv(key string) string { return "PAPIERKRAM_" + strings.ToUpper(key) }

// resolvePreset applies the order of spec 9.8 to key. A flag or
// environment value is an ID (≥ 0, R24); for task also an alias of the
// project file's [tasks]. Invalid values are usage errors (exit 2).
func resolvePreset(key string, src presetSources) (preset, error) {
	if src.Flag != nil {
		id, err := presetValue(key, *src.Flag, "--"+key, src.File)
		return preset{ID: id, Source: config.SourceFlag, Origin: "--" + key}, err
	}
	name := presetEnv(key)
	if src.Env != nil {
		if v := src.Env(name); v != "" {
			id, err := presetValue(key, v, name, src.File)
			return preset{ID: id, Source: config.SourceEnv, Origin: name}, err
		}
	}
	if src.File != nil {
		f, err := src.File()
		if err != nil {
			return preset{}, err
		}
		if id := fileValue(f, key); id != 0 {
			return preset{ID: id, Source: config.SourceProjectFile, Origin: f.Path}, nil
		}
	}
	if src.Context != 0 {
		return preset{ID: src.Context, Source: config.SourceContext, Origin: src.ContextName}, nil
	}
	return preset{}, nil
}

// presetValue parses a flag or environment value of key; from names it in
// errors.
func presetValue(key, raw, from string, file func() (*projectfile.File, error)) (int64, error) {
	if id, ok := parseID(raw); ok {
		return id, nil
	}
	if key == presetTask && !strings.HasPrefix(raw, "-") {
		var f *projectfile.File
		if file != nil {
			var err error
			if f, err = file(); err != nil {
				return 0, err
			}
		}
		return f.ResolveTask(raw)
	}
	return 0, clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültiger Wert %q für %s: ID (ganze Zahl ≥ 0) erwartet", raw, from), "")
}

// fileValue is the value of key in f, 0 when f is nil or has none.
func fileValue(f *projectfile.File, key string) int64 {
	if f == nil {
		return 0
	}
	switch key {
	case presetCompany:
		return f.Company
	case presetProject:
		return f.Project
	case presetTask:
		return f.Task
	case presetUser:
		return f.User
	}
	return 0
}

// preset resolves key for this invocation without a flag: environment,
// project file and, for user, the context default once the configuration
// is resolved.
func (a *App) preset(key string) (preset, error) {
	src := presetSources{Env: a.Env, File: a.projectFile}
	if key == presetUser && a.resolved != nil {
		src.Context, src.ContextName = a.resolved.User, a.resolved.Context
	}
	return resolvePreset(key, src)
}

// presetNote names the sources of a preset after flag or ID for help texts.
func presetNote(key string) string {
	return fmt.Sprintf("%s oder %s aus der Projektdatei %s", presetEnv(key), key, projectfile.FileName)
}

// presetHint names a preset a command uses on stderr.
func (a *App) presetHint(key string, p preset) {
	from := p.Origin
	switch p.Source {
	case config.SourceProjectFile:
		from = "Projektdatei " + p.Origin
	case config.SourceContext:
		from = fmt.Sprintf("Kontext %q", p.Origin)
	}
	_, _ = fmt.Fprintf(a.Err, "Hinweis: %s %d aus %s\n", resourceNoun(key), p.ID, from)
}

// presetArgs returns args with the positional ID of a describe entry with
// a preset filled in when it is missing (spec 9.8: project describe
// without ID). Without a preset value it is a usage error (exit 2).
func (a *App) presetArgs(c *cobra.Command, e registry.Entry, args []string) ([]string, error) {
	if len(args) > 0 || e.Preset == "" {
		return args, nil
	}
	p, err := a.preset(e.Preset)
	if err != nil {
		return nil, err
	}
	if p.Source == "" {
		where := projectfile.FileName
		if f, _ := a.projectFile(); f != nil {
			where = f.Path
		}
		return nil, clierr.New(clierr.CodeUsage,
			fmt.Sprintf("keine ID angegeben und keine Voreinstellung gefunden (%s oder %s in %s)", presetEnv(e.Preset), e.Preset, where),
			fmt.Sprintf("ID angeben (Verwendung: %s) oder voreinstellen: papierkram local init --%s <id>", useLine(c), e.Preset))
	}
	a.presetHint(e.Preset, p)
	return []string{strconv.FormatInt(p.ID, 10)}, nil
}

// presetQuery sets the list filter of e's preset when its flag is not
// given and a preset value exists (spec 9.8: task list).
func (a *App) presetQuery(e registry.Entry, flt registry.Filter, lr *listRequest) error {
	if flt.Flag != e.Preset {
		return nil
	}
	p, err := a.preset(e.Preset)
	if err != nil || p.Source == "" {
		return err
	}
	lr.query.Set(flt.Param, strconv.FormatInt(p.ID, 10))
	a.presetHint(e.Preset, p)
	return nil
}

// presetBody sets the field of e's preset in a create body when neither
// its flag nor --input gives it and a preset value exists (spec 9.8: task
// create).
func (a *App) presetBody(c *cobra.Command, e registry.Entry, ws writeSpec, body map[string]any) error {
	if e.Preset == "" || e.Action != "create" || c.Flags().Changed(e.Preset) {
		return nil
	}
	for _, f := range ws.fields {
		if f.Flag != e.Preset || hasPath(body, f.Path) {
			continue
		}
		p, err := a.preset(e.Preset)
		if err != nil || p.Source == "" {
			return err
		}
		v, err := spec.CoerceFlag(spec.CoerceContext{Now: a.Now, Loc: a.Location}, f, strconv.FormatInt(p.ID, 10))
		if err != nil {
			return err
		}
		setPath(body, f.Path, v)
		a.presetHint(e.Preset, p)
	}
	return nil
}

// hasPath reports whether body has a value at path.
func hasPath(body map[string]any, path []string) bool {
	m := body
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			return false
		}
		m = next
	}
	return hasKey(m, path[len(path)-1])
}
