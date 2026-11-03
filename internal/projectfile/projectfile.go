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
)

// FileName is the name of the project file.
const FileName = ".papierkram.toml"

var knownKeys = map[string]bool{
	"context": true, "company": true, "project": true,
	"task": true, "user": true, "tasks": true,
}

// File is the parsed content of a .papierkram.toml.
type File struct {
	Path                         string           `toml:"-"`
	Context                      string           `toml:"context"`
	Company, Project, Task, User int64            `toml:"-"`
	Tasks                        map[string]int64 `toml:"-"`
	// Unknown lists unknown top-level keys, sorted.
	Unknown []string `toml:"-"`
}

type raw struct {
	Context string `toml:"context"`
	Company int64  `toml:"company"`
	Project int64  `toml:"project"`
	Task    int64  `toml:"task"`
	User    int64  `toml:"user"`
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
		return nil, fmt.Errorf("%s lesen: %w", path, err)
	}
	invalid := func(detail string) error {
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Projektdatei %s ist ungültig", path),
			detail)
	}
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, invalid(err.Error())
	}
	f := &File{Path: path}
	if v, ok := m["tasks"]; ok {
		tbl, isTable := v.(map[string]any)
		if !isTable {
			return nil, invalid(`"tasks" muss eine Tabelle sein: [tasks] alias = 123`)
		}
		f.Tasks = make(map[string]int64, len(tbl))
		for alias, val := range tbl {
			id, isInt := val.(int64)
			if !isInt || id <= 0 {
				return nil, clierr.New(clierr.CodeUsage,
					fmt.Sprintf("Projektdatei %s: Aufgaben-Alias %q muss eine positive ganze Zahl sein", path, alias),
					"Beispiel: [tasks] "+alias+" = 123")
			}
			f.Tasks[alias] = id
		}
	}
	var r raw
	if err := toml.Unmarshal(data, &r); err != nil {
		var de *toml.DecodeError
		detail := err.Error()
		if errors.As(err, &de) {
			detail = de.String()
		}
		return nil, invalid(detail)
	}
	f.Context, f.Company, f.Project, f.Task, f.User = r.Context, r.Company, r.Project, r.Task, r.User
	for k := range m {
		if !knownKeys[k] {
			f.Unknown = append(f.Unknown, k)
		}
	}
	sort.Strings(f.Unknown)
	return f, nil
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
	return 0, clierr.New(clierr.CodeUsage, msg, "Verfügbare Aliase: "+strings.Join(names, ", "))
}
