// Package cli contains the cobra command tree of the papierkram CLI.
package cli

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/config"
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
	// Sleep waits between GET retries; nil means time.Sleep.
	Sleep func(time.Duration)
	// DebugOut receives the --debug request log; nil means Err. A tool
	// call of mcp serve captures Err into the tool result and logs to the
	// stderr of the process instead (mcpChild).
	DebugOut io.Writer

	// Flags receives the parsed global flag values.
	Flags Flags

	// saveConfig writes config.toml; nil means (*config.Config).Save. Tests
	// inject failures through it (portable, unlike file permissions).
	saveConfig func(*config.Config, string) error

	// quota is the last quota state reported by the API in this invocation;
	// quotaWarned records that the low-quota warning was printed.
	quota       api.Quota
	quotaWarned bool
	// insecureWarned records that the warning about a base URL without
	// https was printed (warnInsecure).
	insecureWarned bool

	// restoreTerminal restores the terminal state of the start; readCtx
	// calls it when an interrupt ends a hidden token read (nil: no
	// terminal).
	restoreTerminal func()

	// stdinBuf is the shared buffered reader over In (see stdin).
	stdinBuf *bufio.Reader

	// resolved is the configuration resolved in this invocation (nil until
	// then); execute uses its output format for the error output.
	resolved *config.Resolved

	// flagParseFailed is set when the flags could not be parsed: the
	// flags after the bad one are unset, so execute takes the error
	// format from the raw arguments (errorFormatFromArgs, review C-I2).
	flagParseFailed bool

	// pfile is the project file of this invocation (see projectFile).
	pfile projectFileCache

	// presetHints are the preset hints of this invocation, printed once
	// the command is valid (see flushPresetHints).
	presetHints []string
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
	if fd := int(os.Stdin.Fd()); app.StdinTTY {
		if st, err := term.GetState(fd); err == nil {
			app.restoreTerminal = func() { _ = term.Restore(fd, st) }
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		// config.Path appends "papierkram/config.toml" itself.
		app.ConfigDir = dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		// the cache packages append "papierkram/" (see cacheDir)
		app.CacheDir = dir
	}
	return app
}

// debugOut is the writer of the --debug request log.
func (a *App) debugOut() io.Writer {
	if a.DebugOut != nil {
		return a.DebugOut
	}
	return a.Err
}

// save writes cfg to path, through the saveConfig seam when set.
func (a *App) save(cfg *config.Config, path string) error {
	if a.saveConfig != nil {
		return a.saveConfig(cfg, path)
	}
	return cfg.Save(path)
}
