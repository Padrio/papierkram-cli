// Package cli contains the cobra command tree of the papierkram CLI.
package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/auth"
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

	// OutputSet and TimeoutSet report whether -o/--output and --timeout were
	// given explicitly; only then do they override the context values.
	OutputSet, TimeoutSet bool
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
	// Keyring stores the API tokens per context (tests use auth.NewMemory()).
	Keyring auth.Store
	// Getwd returns the working directory, the start of the project file
	// search.
	Getwd func() (string, error)
	// ReadPassword reads a line from the terminal without echo (auth login).
	ReadPassword func() ([]byte, error)

	// Flags receives the parsed global flag values.
	Flags Flags

	// quota is the last quota state reported by the API in this invocation;
	// quotaWarned records that the low-quota warning was printed.
	quota       api.Quota
	quotaWarned bool
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
		Keyring:    auth.Keyring(),
		Getwd:      os.Getwd,
		ReadPassword: func() ([]byte, error) {
			return term.ReadPassword(int(os.Stdin.Fd()))
		},
	}
	if dir, err := os.UserConfigDir(); err == nil {
		// config.Path appends "papierkram/config.toml" itself.
		app.ConfigDir = dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		app.CacheDir = filepath.Join(dir, "papierkram")
	}
	return app
}
