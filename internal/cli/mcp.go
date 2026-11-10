package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/buildinfo"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/mcpserver"
)

// mcp serve (spec v0.2 §3): the commands as MCP tools over stdio. A tool
// call becomes the command line it describes and runs in-process through
// RunContext, with its own App (mcpChild) and -o json --no-input: every
// check, the read-only mode, the confirmations, the credits and the exit
// codes are those of the command line.

// MCPOptions configure the MCP server. Context is --context of the start;
// the context it resolves to (or the one the CLI chooses without it) is
// fixed for every call. FilesDir is the directory every file path of a
// tool lies in (empty: no tool with files, ruling T5-E). Timeout is
// the --timeout of every call (empty: the CLI default or the context's).
type MCPOptions struct {
	Context  string
	FilesDir string
	Timeout  string
}

// mcpStartFlags are global flags that mean nothing for the server: every
// call runs with -o json and --no-input, and never with --yes, --jq or
// --fields. Given at the start they are a usage error, like --dry-run.
var mcpStartFlags = []string{"yes", "jq", "fields", "output"}

const mcpServeLong = `Startet einen MCP-Server (Model Context Protocol) über stdin/stdout, damit ein KI-Client wie Claude Desktop oder Claude Code die Befehle als Tools aufrufen kann.

Je Lese-Befehl (Klasse read_only) gibt es ein Tool, z. B. invoice_list oder invoice_describe; Name, Beschreibung (mit Klasse und Credits) und Eingabe-Schema stammen aus papierkram commands --json. Lokale Befehle (auth, context, local, completion, time start|stop|status|discard) und papierkram api haben keine Tools.

Ein Tool-Aufruf führt genau den Befehl aus, den er beschreibt, mit -o json und --no-input: dieselben Prüfungen, derselbe Schreibschutz, dieselben Credits und Exit-Codes wie auf der Kommandozeile. Das Ergebnis ist die JSON-Ausgabe des Befehls; Hinweise und Warnungen von stderr folgen als zweiter Inhalt. Bei einem Fehler ist das Ergebnis ein Tool-Fehler mit der JSON-Fehlerzeile (code, message, hint, exit_code). Aufrufe laufen nacheinander.

Tools mit Dateien (pdf) gibt es nur mit --files-dir: ein vorhandener Ordner, nicht die Wurzel des Dateisystems und nicht das Home-Verzeichnis selbst (sonst Exit 2), am besten ein eigener Unterordner. Ohne --files-dir fehlen diese Tools, mit einem Hinweis auf stderr; ein Arbeitsverzeichnis gilt nie als Dateiordner (Claude Desktop startet Server z. B. in /). Dateipfade der Tools sind relativ zu diesem Ordner: kein absoluter Pfad, kein „..“, kein „-“, kein symbolischer Verweis ins Leere oder aus dem Ordner hinaus, kein fehlendes Verzeichnis (sonst Exit 2 ohne Anfrage). Das CLI erhält den absoluten Pfad im Ordner. Die pdf-Tools laufen mit --no-clobber: Sie ersetzen keine vorhandene Datei.

In der Konfiguration eines Clients (JSON, z. B. {"command":"papierkram","args":["mcp","serve","--files-dir","/Users/name/Belege"]}) wird ~ nicht erweitert: den Ordner absolut angeben.

Der Kontext wird beim Start festgelegt (--context oder wie sonst: PAPIERKRAM_CONTEXT, Projektdatei, aktiver Kontext) und gilt für alle Aufrufe, auch wenn sich der aktive Kontext später ändert; Tools haben keinen Kontext-Parameter. --timeout gilt für jeden Aufruf. --yes, --jq, --fields, -o und --dry-run gelten nicht für den Server (Exit 2). --debug schreibt die Anfragen auf stderr, nie in ein Tool-Ergebnis. stdout trägt nur das Protokoll (JSON-RPC).

Der Server endet mit Exit 0, wenn stdin geschlossen wird oder ein Signal (Strg+C) kommt. Ein Signal wartet auf den laufenden Aufruf und bricht ihn nicht ab: Er läuft zu Ende, seine Antwort erreicht den Client aber nicht mehr (Verbindung geschlossen; eine Änderung dann wie nach Exit 12 per describe/list prüfen). Ein zweites Strg+C beendet den Prozess sofort. Bricht der Client einen Aufruf ab, endet der Befehl wie nach Strg+C (Exit 10, nach gesendeter Änderung 12), der Client erhält dafür keine Antwort.

Sendet selbst keine Anfrage; jeder Tool-Aufruf kostet die Credits seines Befehls.`

func newMCPCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP-Server für KI-Clients",
		Long:  "Stellt die Befehle als Tools für KI-Clients bereit (Model Context Protocol).",
		Args:  noArgs,
		RunE:  func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "MCP-Server über stdio starten (Lese-Tools)",
		Long:  mcpServeLong,
		Example: "  papierkram mcp serve\n" +
			"  papierkram mcp serve --context firma\n" +
			"  papierkram mcp serve --files-dir /Users/name/Belege",
		// sends nothing itself and writes nothing: --dry-run is rejected
		Annotations: map[string]string{annotLocal: "true"},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			for _, name := range mcpStartFlags {
				if f := c.Flags().Lookup(name); f.Changed {
					spelled := "--" + name
					if f.Shorthand != "" {
						spelled = "-" + f.Shorthand + ", " + spelled
					}
					return clierr.New(clierr.CodeUsage, spelled+" gilt nicht für mcp serve",
						"Tool-Aufrufe laufen immer mit -o json und --no-input, nie mit --yes, --jq oder --fields; ohne "+spelled+" starten")
				}
			}
			o := MCPOptions{Context: app.Flags.Context, FilesDir: flagString(c, "files-dir")}
			if c.Flags().Changed("timeout") {
				o.Timeout = app.Flags.Timeout.String()
			}
			return app.serveMCP(c.Context(), o)
		},
	}
	serve.Flags().String("files-dir", "", "Ordner, in dem alle Dateipfade der Tools liegen (absolut; ohne ihn keine Tools mit Dateien)")
	cmd.AddCommand(serve)
	return cmd
}

// NewMCPServer builds the MCP server of app: the read tools of the command
// tree, every call run by mcpRunner in context o.Context, with --debug
// when app was started with it. The configuration is resolved once
// without the keychain, so a broken config.toml or an unknown context
// stops the start (its error, no server).
func NewMCPServer(app *App, o MCPOptions) (*mcp.Server, error) {
	r, err := app.resolveWith(o.Context, nil)
	if err != nil {
		return nil, err
	}
	dir, err := app.mcpFilesDir(o.FilesDir)
	if err != nil {
		return nil, err
	}
	// the tree of a child: NewRootCmd binds the global flags to the App
	// it gets and would reset those of app
	specs, err := mcpToolSpecs(NewRootCmd(app.mcpChild(io.Discard, io.Discard)))
	if err != nil {
		return nil, err
	}
	return mcpserver.New(specs, app.mcpRunner(), mcpserver.Options{
		Mode:     mcpserver.ModeRead,
		Context:  r.Context,
		Timeout:  o.Timeout,
		Debug:    app.Flags.Debug,
		Version:  buildinfo.Version,
		FilesDir: dir,
	}), nil
}

// mcpFilesDir is the files directory of the server: dir made absolute
// (relative to the working directory), an existing directory, neither the
// root of a file system nor the home directory (rulings T5-A, T5-E); ""
// without dir: no tool with files.
func (a *App) mcpFilesDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		wd, err := a.Getwd()
		if err != nil {
			return "", clierr.New(clierr.CodeError, "Arbeitsverzeichnis nicht lesbar: "+err.Error(), "--files-dir mit absolutem Pfad angeben")
		}
		dir = filepath.Join(wd, dir)
	}
	dir = filepath.Clean(dir)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("--files-dir %s ist kein vorhandenes Verzeichnis", dir),
			"Verzeichnis anlegen oder ein anderes angeben")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		real = dir
	}
	switch home := a.homeDir(); {
	case filepath.Dir(dir) == dir || filepath.Dir(real) == real:
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("--files-dir %s ist die Wurzel des Dateisystems", dir),
			"Tools dürften dann jede Datei des Rechners lesen und anlegen; einen eigenen Unterordner angeben, z. B. "+filepath.Join(dir, "Belege"))
	case home != "" && samePath(dir, home):
		return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("--files-dir %s ist das Home-Verzeichnis", dir),
			"Tools dürften dann jede Datei des Nutzers lesen und anlegen; einen eigenen Unterordner angeben, z. B. "+filepath.Join(dir, "Belege"))
	}
	return dir, nil
}

// homeDir is the home directory from the environment of a, like
// os.UserHomeDir ("" when unset).
func (a *App) homeDir() string {
	if runtime.GOOS == "windows" {
		return a.Env("USERPROFILE")
	}
	return a.Env("HOME")
}

// samePath reports whether a and b name the same directory: lexically or
// with their symbolic links resolved.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// serveMCP runs the MCP server over app.In and app.Out until the client
// closes stdin or ctx ends (signal); both end with exit 0.
func (a *App) serveMCP(ctx context.Context, o MCPOptions) error {
	s, err := NewMCPServer(a, o)
	if err != nil {
		return err
	}
	if o.FilesDir == "" {
		_, _ = io.WriteString(a.Err, "Hinweis: ohne --files-dir keine Tools mit Dateien (pdf, Upload); --files-dir <absoluter Ordner> setzen\n")
	}
	t := &mcp.IOTransport{Reader: io.NopCloser(a.In), Writer: nopWriteCloser{a.Out}}
	// Run returns nil when the client closes stdin (or stdout breaks: the
	// client is gone) and the error of ctx after a signal
	if err := s.Run(ctx, t); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// nopWriteCloser keeps the stdout of the process open when the transport
// closes.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// mcpRunner runs one tool call: the arguments through RunContext in a
// child App whose stdout and stderr become the result.
func (a *App) mcpRunner() mcpserver.Runner {
	return func(ctx context.Context, args []string) mcpserver.Result {
		var out, errb bytes.Buffer
		exit := RunContext(ctx, a.mcpChild(&out, &errb), args)
		return mcpserver.Result{Exit: exit, Stdout: out.Bytes(), Stderr: errb.Bytes()}
	}
}

// mcpChild is the App of one tool call: the environment, clock, paths,
// HTTP client and keychain of a, stdout and stderr into out and errw, no
// stdin and no terminal (no prompt can wait), and the --debug log to the
// stderr of the process. Its flags are fresh: the call sets them.
func (a *App) mcpChild(out, errw io.Writer) *App {
	return &App{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        errw,
		Env:        a.Env,
		Now:        a.Now,
		Location:   a.Location,
		ConfigDir:  a.ConfigDir,
		CacheDir:   a.CacheDir,
		HTTPClient: a.HTTPClient,
		Keyring:    a.Keyring,
		Getwd:      a.Getwd,
		ReadPassword: func() ([]byte, error) {
			return nil, errors.New("keine Eingabe im MCP-Server")
		},
		Sleep:      a.Sleep,
		DebugOut:   a.debugOut(),
		saveConfig: a.saveConfig,
	}
}
