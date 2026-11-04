package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

// writeProjectFile writes a .papierkram.toml into a fresh directory and
// makes it the working directory of app.
func writeProjectFile(t *testing.T, app *App, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, projectfile.FileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	app.Getwd = func() (string, error) { return dir, nil }
	return path
}

// The project file is found and read at most once per invocation, by the
// context selection and the presets alike.
func TestProjectFileLoadedOnce(t *testing.T) {
	app, _, _ := newTestApp(t)
	path := writeProjectFile(t, app, "project = 34\n")
	f, err := app.projectFile()
	if err != nil || f == nil || f.Project != 34 || f.Path != path {
		t.Fatalf("projectFile() = %+v, %v", f, err)
	}
	if err := os.WriteFile(path, []byte("project = 35\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again, _ := app.projectFile(); again != f {
		t.Errorf("zweiter Aufruf liest neu: %+v", again)
	}
	// a new invocation (root command) reads it again
	NewRootCmd(app)
	if f, _ := app.projectFile(); f == nil || f.Project != 35 {
		t.Errorf("neuer Aufruf: %+v", f)
	}

	app, _, _ = newTestApp(t) // temp dir without project file
	if f, err := app.projectFile(); f != nil || err != nil {
		t.Errorf("ohne Projektdatei: %+v, %v", f, err)
	}
}
