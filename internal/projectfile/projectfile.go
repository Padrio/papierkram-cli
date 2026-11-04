// Package projectfile finds and parses the per-project .papierkram.toml.
package projectfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
)

// FileName is the name of the project file.
const FileName = ".papierkram.toml"

var knownKeys = map[string]bool{
	"context": true, "company": true, "project": true,
	"task": true, "user": true, "tasks": true,
}

// File is the parsed content of a .papierkram.toml.
type File struct {
	Path                         string
	Context                      string
	Company, Project, Task, User int64
	Tasks                        map[string]int64
	// Unknown lists unknown top-level keys, sorted.
	Unknown []string
}

// Find looks for the project file in cwd and, if cwd lies inside a Git
// repository, in every parent up to and including the Git root. Outside Git
// only cwd is checked. It never looks above the Git root.
func Find(cwd string) (string, bool) {
	dir := filepath.Clean(cwd)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	dirs := []string{dir}
	if root, ok := gitRoot(dir); ok {
		dirs = dirs[:0]
		for d := dir; ; d = filepath.Dir(d) {
			dirs = append(dirs, d)
			if d == root || filepath.Dir(d) == d {
				break
			}
		}
	}
	for _, d := range dirs {
		p := filepath.Join(d, FileName)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}

// gitRoot returns the first ancestor (or dir itself) containing .git as a
// directory or file (worktrees and submodules use a file).
func gitRoot(dir string) (string, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}

// Load reads and validates the project file at path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s lesen: %w", output.Printable(path), err)
	}
	m, err := decode(path, data)
	if err != nil {
		return nil, err
	}
	// messages show the path without control characters (a directory of
	// a cloned repository may be named with terminal escapes)
	shown := output.Printable(path)
	f := &File{Path: path}
	if v, ok := m["context"]; ok {
		ctx, isString := v.(string)
		if !isString {
			return nil, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("Projektdatei %s: %q muss eine Zeichenkette sein", shown, "context"),
				`Beispiel: context = "firma-gmbh"`)
		}
		// the rules of context names: the file may come from a cloned
		// repository, its context is printed in messages
		if err := config.ValidateName(ctx); err != nil {
			ce := clierr.From(err)
			return nil, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("Projektdatei %s: ungültiger Wert für \"context\": %s", shown, ce.Message), ce.Hint)
		}
		f.Context = ctx
	}
	for _, id := range []struct {
		key string
		dst *int64
	}{{"company", &f.Company}, {"project", &f.Project}, {"task", &f.Task}, {"user", &f.User}} {
		key, dst := id.key, id.dst
		v, ok := m[key]
		if !ok {
			continue
		}
		id, isInt := v.(int64)
		if !isInt || id <= 0 {
			return nil, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("Projektdatei %s: %q muss eine positive ganze Zahl sein", shown, key),
				fmt.Sprintf("Beispiel: %s = 34", key))
		}
		*dst = id
	}
	if v, ok := m["tasks"]; ok {
		tbl, isTable := v.(map[string]any)
		if !isTable {
			return nil, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("Projektdatei %s: %q muss eine Tabelle sein", shown, "tasks"),
				"Beispiel: [tasks] tickets = 57")
		}
		f.Tasks = make(map[string]int64, len(tbl))
		aliases := make([]string, 0, len(tbl))
		for alias := range tbl {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases) // deterministic error for the first bad alias
		for _, alias := range aliases {
			val := tbl[alias]
			// A numeric alias would be shadowed by the ID parsing in ResolveTask.
			if _, err := strconv.ParseInt(alias, 10, 64); err == nil {
				return nil, clierr.New(clierr.CodeUsage,
					fmt.Sprintf("Projektdatei %s: Aufgaben-Alias %q darf keine Zahl sein (wird als Aufgaben-ID gelesen)", shown, alias),
					"Alias mit Buchstaben benennen, z. B. [tasks] tickets = 57")
			}
			id, isInt := val.(int64)
			if !isInt || id <= 0 {
				return nil, clierr.New(clierr.CodeUsage,
					fmt.Sprintf("Projektdatei %s: Aufgaben-Alias %q muss eine positive ganze Zahl sein", shown, alias),
					"Beispiel: [tasks] "+alias+" = 123")
			}
			f.Tasks[alias] = id
		}
	}
	for k := range m {
		if !knownKeys[k] {
			f.Unknown = append(f.Unknown, k)
		}
	}
	sort.Strings(f.Unknown)
	return f, nil
}

// decode parses the TOML of the project file at path; a syntax error is a
// usage error naming line and column.
func decode(path string, data []byte) (map[string]any, error) {
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		hint := "TOML-Syntax prüfen, z. B. project = 34"
		var de *toml.DecodeError
		if errors.As(err, &de) {
			row, col := de.Position()
			hint = fmt.Sprintf("TOML-Syntax in Zeile %d, Spalte %d prüfen, z. B. project = 34", row, col)
		}
		e := clierr.New(clierr.CodeUsage, fmt.Sprintf("Projektdatei %s ist ungültig", output.Printable(path)), hint)
		e.Cause = err
		return nil, e
	}
	return m, nil
}

// ResolveTask turns a --task value (ID or alias from [tasks]) into a task ID.
func (f *File) ResolveTask(v string) (int64, error) {
	if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
		return id, nil
	}
	if f != nil {
		if id, ok := f.Tasks[v]; ok {
			return id, nil
		}
	}
	msg := fmt.Sprintf("unbekannter Aufgaben-Alias %q", v)
	if f == nil || len(f.Tasks) == 0 {
		return 0, clierr.New(clierr.CodeUsage, msg,
			"Aufgaben-ID verwenden oder Aliase in der Projektdatei unter [tasks] anlegen")
	}
	names := make([]string, 0, len(f.Tasks))
	for k := range f.Tasks {
		names = append(names, k)
	}
	sort.Strings(names)
	return 0, clierr.New(clierr.CodeUsage, msg, "Verfügbare Aliase: "+output.Printable(strings.Join(names, ", ")))
}
