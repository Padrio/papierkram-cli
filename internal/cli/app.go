// Package cli contains the cobra command tree of the papierkram CLI.
package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"
)

// Flags holds the values of the global persistent flags.
type Flags struct {
	Context string
	Output  string
	Fields  string
	JQ      string
	Debug   bool
	Timeout time.Duration
	NoInput bool
	Yes     bool
	DryRun  bool
	NoColor bool
}

// App bundles all injectable dependencies of the CLI.
type App struct {
	In         io.Reader
	Out, Err   io.Writer
	Env        func(string) string
	Now        func() time.Time
	Location   *time.Location
	StdinTTY   bool
	StdoutTTY  bool
	ConfigDir  string
	CacheDir   string
	HTTPClient *http.Client

	// Flags receives the parsed global flag values.
	Flags Flags
}

// NewApp creates an App wired to the operating system.
func NewApp() *App {
	app := &App{
		In:         os.Stdin,
		Out:        os.Stdout,
		Err:        os.Stderr,
		Env:        os.Getenv,
		Now:        time.Now,
		Location:   time.Local,
		StdinTTY:   term.IsTerminal(int(os.Stdin.Fd())),
		StdoutTTY:  term.IsTerminal(int(os.Stdout.Fd())),
		HTTPClient: http.DefaultClient,
	}
	if dir, err := os.UserConfigDir(); err == nil {
		app.ConfigDir = filepath.Join(dir, "papierkram")
	}
	if dir, err := os.UserCacheDir(); err == nil {
		app.CacheDir = filepath.Join(dir, "papierkram")
	}
	return app
}
