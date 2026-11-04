package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

// projectFileCache is the project file of one invocation.
// warned records the warning of presetFile.
type projectFileCache struct {
	loaded, warned bool
	file           *projectfile.File
	err            error
}

// projectFile returns the project file of this invocation, nil without
// one. It is found and read at most once (context selection and presets
// share it); NewRootCmd starts a new invocation.
func (a *App) projectFile() (*projectfile.File, error) {
	if !a.pfile.loaded {
		a.pfile.file, a.pfile.err = a.loadProjectFile()
		a.pfile.loaded = true
	}
	return a.pfile.file, a.pfile.err
}

// loadProjectFile searches the project file from the working directory
// (spec 9.8) and reads it. Without a working directory there is none.
func (a *App) loadProjectFile() (*projectfile.File, error) {
	if a.Getwd == nil {
		return nil, nil
	}
	cwd, err := a.Getwd()
	if err != nil {
		return nil, nil
	}
	path, ok := projectfile.Find(cwd)
	if !ok {
		return nil, nil
	}
	f, err := projectfile.Load(path)
	if err != nil {
		var ce *clierr.Error
		if !errors.As(err, &ce) {
			ce = clierr.New(clierr.CodeError, err.Error(), "Projektdatei prüfen: "+printable(path))
			ce.Cause = err
		}
		return nil, ce
	}
	if len(f.Unknown) > 0 {
		// spec 9.8: a warning, no error; once per invocation, since the
		// file is loaded once
		// file content and path are shown printable: the file may come from
		// a cloned repository (terminal escapes in keys or directory names)
		keys := make([]string, len(f.Unknown))
		for i, k := range f.Unknown {
			keys[i] = strconv.Quote(k)
		}
		output.Warn(a.Err, fmt.Sprintf("Projektdatei %s: unbekannte Schlüssel werden ignoriert: %s (bekannt: context, company, project, task, user, [tasks])",
			printable(path), strings.Join(keys, ", ")))
	}
	return f, nil
}
