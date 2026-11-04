package cli

import (
	"errors"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

// projectFileCache is the project file of one invocation.
type projectFileCache struct {
	loaded bool
	file   *projectfile.File
	err    error
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
			ce = clierr.New(clierr.CodeError, err.Error(), "Projektdatei prüfen: "+path)
			ce.Cause = err
		}
		return nil, ce
	}
	return f, nil
}
