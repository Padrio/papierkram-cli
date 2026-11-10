package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/buildinfo"
	"github.com/Padrio/papierkram-cli/internal/mcpserver"
)

// mcp serve (spec v0.2 §3): the commands as MCP tools over stdio. A tool
// call becomes the command line it describes and runs in-process through
// RunContext, with its own App (mcpChild) and -o json --no-input: every
// check, the read-only mode, the confirmations, the credits and the exit
// codes are those of the command line.

// MCPOptions configure the MCP server: Context is the context of every
// call (--context at the start; empty: the CLI chooses as usual).
type MCPOptions struct {
	Context string
}

const mcpServeLong = `Startet einen MCP-Server (Model Context Protocol) über stdin/stdout, damit ein KI-Client wie Claude Desktop oder Claude Code die Befehle als Tools aufrufen kann.

Je Lese-Befehl (Klasse read_only) gibt es ein Tool, z. B. invoice_list oder invoice_describe; Name, Beschreibung (mit Klasse und Credits) und Eingabe-Schema stammen aus papierkram commands --json. Lokale Befehle (auth, context, local, completion, time start|stop|status|discard) und papierkram api haben keine Tools.

Ein Tool-Aufruf führt genau den Befehl aus, den er beschreibt, mit -o json und --no-input: dieselben Prüfungen, derselbe Schreibschutz, dieselben Credits und Exit-Codes wie auf der Kommandozeile. Das Ergebnis ist die JSON-Ausgabe des Befehls; Hinweise und Warnungen von stderr folgen als zweiter Inhalt. Bei einem Fehler ist das Ergebnis ein Tool-Fehler mit der JSON-Fehlerzeile (code, message, hint, exit_code). Aufrufe laufen nacheinander.

--context wählt den Kontext für alle Aufrufe; Tools haben keinen Kontext-Parameter. --debug schreibt die Anfragen auf stderr, nie in ein Tool-Ergebnis. stdout trägt nur das Protokoll (JSON-RPC). Der Server endet, wenn stdin geschlossen wird oder ein Signal (Strg+C) kommt, mit Exit 0.

Sendet selbst keine Anfrage; jeder Tool-Aufruf kostet die Credits seines Befehls.`

func newMCPCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP-Server für KI-Clients",
		Long:  "Stellt die Befehle als Tools für KI-Clients bereit (Model Context Protocol).",
		Args:  noArgs,
		RunE:  func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "MCP-Server über stdio starten (Lese-Tools)",
		Long:  mcpServeLong,
		Example: "  papierkram mcp serve\n" +
			"  papierkram mcp serve --context firma",
		// sends nothing itself and writes nothing: --dry-run is rejected
		Annotations: map[string]string{annotLocal: "true"},
		Args:        noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return app.serveMCP(c.Context(), MCPOptions{Context: app.Flags.Context})
		},
	})
	return cmd
}

// NewMCPServer builds the MCP server of app: the read tools of the command
// tree, every call run by mcpRunner in context o.Context, with --debug
// when app was started with it. The configuration is resolved once
// without the keychain, so a broken config.toml or an unknown context
// stops the start (its error, no server).
func NewMCPServer(app *App, o MCPOptions) (*mcp.Server, error) {
	if _, err := app.resolveWith(o.Context, nil); err != nil {
		return nil, err
	}
	// the tree of a child: NewRootCmd binds the global flags to the App
	// it gets and would reset those of app
	specs, err := mcpToolSpecs(NewRootCmd(app.mcpChild(io.Discard, io.Discard)))
	if err != nil {
		return nil, err
	}
	return mcpserver.New(specs, app.mcpRunner(), mcpserver.Options{
		Mode:    mcpserver.ModeRead,
		Context: o.Context,
		Debug:   app.Flags.Debug,
		Version: buildinfo.Version,
	}), nil
}

// serveMCP runs the MCP server over app.In and app.Out until the client
// closes stdin or ctx ends (signal); both end with exit 0.
func (a *App) serveMCP(ctx context.Context, o MCPOptions) error {
	s, err := NewMCPServer(a, o)
	if err != nil {
		return err
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
