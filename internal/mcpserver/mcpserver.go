// Package mcpserver serves commands of the papierkram CLI as MCP tools
// (spec v0.2 §3). It knows nothing of the CLI itself: the caller describes
// the tools (ToolSpec, built from commands --json) and runs a call through
// a Runner, which executes the translated argument list in-process like
// the command line. So every check of the CLI (flags, read-only mode,
// confirmations, credits, exit codes) applies to a tool call unchanged;
// this package only translates arguments and results.
package mcpserver

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Mode decides which classes of commands become tools.
type Mode int

const (
	// ModeRead serves read_only commands only (the default).
	ModeRead Mode = iota
	// ModeWrite adds create, update and archive.
	ModeWrite
	// ModeDestructive adds destructive and send.
	ModeDestructive
)

// Allows reports whether a command of class becomes a tool in mode m.
func (m Mode) Allows(class string) bool {
	switch class {
	case "read_only":
		return m == ModeRead || m == ModeWrite || m == ModeDestructive
	case "create", "update", "archive":
		return m == ModeWrite || m == ModeDestructive
	case "destructive", "send":
		return m == ModeDestructive
	}
	return false
}

// Kind is the JSON type of a parameter and how it becomes a CLI argument.
type Kind string

const (
	KindString   Kind = "string"    // text
	KindInteger  Kind = "integer"   // whole number (also IDs without name resolution)
	KindNumber   Kind = "number"    // number as written
	KindBool     Kind = "boolean"   // true or false
	KindStrings  Kind = "strings"   // list of texts, a repeatable flag once per element
	KindRef      Kind = "ref"       // whole number (ID) or name
	KindJSONFile Kind = "json_file" // JSON value, written to a temporary file; the flag gets its path
	KindPath     Kind = "path"      // file path; "-" (stdin/stdout) is not allowed
)

// Param is one parameter of a tool: a flag (Pos -1, Flag its name) or the
// positional argument with index Pos. Enum, Default, Minimum and Maximum
// describe the value in the input schema; the CLI checks them. JSONType is
// the JSON type of a KindJSONFile value, "object" or "array".
type Param struct {
	Name, Flag       string
	Pos              int
	Kind             Kind
	Description      string
	Enum             []string
	Default          any
	Minimum, Maximum *float64
	Required         bool
	JSONType         string
}

// ToolSpec is one tool: the command words of the CLI, its parameters and
// arguments every call gets (Fixed, after the flags). The description
// names class and credits.
type ToolSpec struct {
	Name, Description, Class string
	Command                  []string
	Params                   []Param
	Fixed                    []string
}

// Result is the outcome of one CLI run.
type Result struct {
	Exit           int
	Stdout, Stderr []byte
}

// Runner executes the CLI with args; ctx is cancelled when the client
// cancels the call or the server stops.
type Runner func(ctx context.Context, args []string) Result

// Options configure the server: the mode, the context every call runs in
// (--context, empty: the CLI chooses), the --timeout of every call (empty:
// none), --debug for every call, the version the server reports and the
// absolute directory every file path of a call must lie in (localPath;
// empty: no tool with a file path) with FilesRoot, that directory with its
// links resolved at the start (final review A-M4: every call requires
// FilesDir to resolve to it; empty: resolved per call), and the log of the
// server process (its stderr; nil: none), which gets the stack of a call
// that panicked.
type Options struct {
	Mode      Mode
	Context   string
	Timeout   string
	Debug     bool
	Version   string
	FilesDir  string
	FilesRoot string
	Log       io.Writer
}

// DryRun is the parameter every tool of a class but read_only gets: the
// --dry-run of the CLI (spec v0.2 §3). No flag of a command may take its
// name.
const DryRun = "dry_run"

// dryRunParam is the parameter DryRun.
var dryRunParam = Param{
	Name: DryRun, Flag: "dry-run", Pos: -1, Kind: KindBool,
	Description: "nur prüfen und zeigen, was gesendet würde (--dry-run): sendet nichts; kostet nur vorbereitende Leseanfragen",
}

// params are the parameters of t as the client sees them: its own and,
// for a tool that changes something (every class a mode can serve but
// read_only), dry_run.
func (t ToolSpec) params() []Param {
	if t.Class == "read_only" || !ModeDestructive.Allows(t.Class) {
		return t.Params
	}
	return append(slices.Clip(t.Params), dryRunParam)
}

// confirms reports whether a call of t runs with --yes: a destructive or
// send tool in a mode that serves it (--allow-destructive is the consent
// of the human, spec v0.2 §3). Elsewhere the CLI asks and ends with exit
// 8, it never confirms on its own.
func (t ToolSpec) confirms(m Mode) bool {
	return (t.Class == "destructive" || t.Class == "send") && m.Allows(t.Class)
}

// Tools are the specs whose class mode m allows, in their order.
func Tools(specs []ToolSpec, m Mode) []ToolSpec {
	var out []ToolSpec
	for _, s := range specs {
		if m.Allows(s.Class) {
			out = append(out, s)
		}
	}
	return out
}

// InputSchema is the JSON schema of the arguments of t: an object of its
// parameters, no other properties, the required ones sorted.
func InputSchema(t ToolSpec) map[string]any {
	props := map[string]any{}
	var required []string
	for _, p := range t.params() {
		props[p.Name] = p.schema()
		if p.Required {
			required = append(required, p.Name)
		}
	}
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		slices.Sort(required)
		s["required"] = required
	}
	return s
}

// schema is the JSON schema of the value of p.
func (p Param) schema() map[string]any {
	s := map[string]any{}
	switch p.Kind {
	case KindString, KindPath:
		s["type"] = "string"
	case KindInteger:
		s["type"] = "integer"
	case KindNumber:
		s["type"] = "number"
	case KindBool:
		s["type"] = "boolean"
	case KindStrings:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "string"}
	case KindRef:
		s["type"] = []string{"integer", "string"}
	case KindJSONFile:
		s["type"] = "object"
		if p.JSONType == "array" {
			s["type"] = "array"
		}
	}
	if p.Description != "" {
		s["description"] = p.Description
	}
	if len(p.Enum) > 0 {
		s["enum"] = p.Enum
	}
	if p.Default != nil {
		s["default"] = p.Default
	}
	if p.Minimum != nil {
		s["minimum"] = *p.Minimum
	}
	if p.Maximum != nil {
		s["maximum"] = *p.Maximum
	}
	return s
}

// annotations are the hints of tool t. Only read_only and create are
// additive; update and archive change or hide existing data and count as
// destructive like destructive and send (the MCP meaning of the hint). A
// read_only command with a file path (pdf -O) writes a local file: no
// read-only hint and, as the file is new (--no-clobber), not idempotent
// and not destructive (ruling T5-C).
func annotations(t ToolSpec) *mcp.ToolAnnotations {
	destructive := t.Class != "read_only" && t.Class != "create"
	readOnly := t.Class == "read_only" && !hasPath(t)
	openWorld := true
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  readOnly,
		OpenWorldHint:   &openWorld,
	}
}

// hasPath reports whether t has a file path parameter.
func hasPath(t ToolSpec) bool {
	return slices.ContainsFunc(t.Params, func(p Param) bool { return p.Kind == KindPath })
}

// served is t as a server with the files directory dir offers it (ruling
// T6-A): with dir unchanged; without dir its optional file path
// parameters are left out, and ok is false when a file path is required
// (pdf -O, voucher upload <datei>): no tool then.
func (t ToolSpec) served(dir string) (ToolSpec, bool) {
	if dir != "" || !hasPath(t) {
		return t, true
	}
	if slices.ContainsFunc(t.Params, func(p Param) bool { return p.Kind == KindPath && p.Required }) {
		return t, false
	}
	t.Params = slices.DeleteFunc(slices.Clone(t.Params), func(p Param) bool { return p.Kind == KindPath })
	return t, true
}

// waitCancelled is called when a call is cancelled while it waits for the
// lock (a test seam).
var waitCancelled = func() {}

// New builds the MCP server with one tool per spec that o.Mode allows;
// without o.FilesDir the tools with a required file path are left out
// and the optional file paths of the others (served). A
// call is translated by Args and run by run; one lock serializes all
// calls, because the CLI keeps per-invocation state (one App per run, but
// shared files like the name cache and the timer). A call cancelled while
// it waits never runs, and neither does one that waits when ctx, the
// context of the server, ends (a signal; final review A-M2): it is
// checked again after the lock, so a waiting call never sends after the
// stop, whatever the SDK does with it. The running call keeps its own
// context. A panic of a call is a tool error (recovered).
func New(ctx context.Context, specs []ToolSpec, run Runner, o Options) *mcp.Server {
	server := ctx
	s := mcp.NewServer(&mcp.Implementation{Name: "papierkram", Version: o.Version}, nil)
	lock := make(chan struct{}, 1)
	for _, t := range Tools(specs, o.Mode) {
		t, ok := t.served(o.FilesDir)
		if !ok {
			// no files directory, no tool that needs a file (rulings
			// T5-E, T6-A)
			continue
		}
		tool := &mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: InputSchema(t),
			Annotations: annotations(t),
		}
		s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			select {
			case lock <- struct{}{}:
			case <-ctx.Done():
				waitCancelled()
				return nil, ctx.Err()
			case <-server.Done():
				waitCancelled()
				return nil, server.Err()
			}
			defer func() { <-lock }()
			// the lock may be won after a stop or a cancellation: never run
			if err := server.Err(); err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return safeCall(ctx, t, req.Params.Arguments, run, o), nil
		})
	}
	return s
}

// safeCall is call with a panic turned into a tool error (final review
// A-M7): one broken call must not end the server and the session of the
// client. The outcome is unknown (a change may have been sent); the stack
// goes to o.Log.
func safeCall(ctx context.Context, t ToolSpec, raw []byte, run Runner, o Options) (res *mcp.CallToolResult) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		if o.Log != nil {
			_, _ = fmt.Fprintf(o.Log, "Fehler: Tool %s: interner Fehler: %v\n%s", t.Name, v, debug.Stack())
		}
		line := errorLine("error", fmt.Sprintf("interner Fehler im Tool %s: %v", t.Name, v),
			"Ergebnis unbekannt – per describe/list prüfen, ob eine Änderung angekommen ist", 1)
		res = &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: line}}}
	}()
	return call(ctx, t, raw, run, o)
}

// call runs one tool call: arguments to CLI arguments (JSON values in a
// temporary directory of this call, removed afterwards), run, result.
func call(ctx context.Context, t ToolSpec, raw []byte, run Runner, o Options) *mcp.CallToolResult {
	tmp := ""
	if slices.ContainsFunc(t.Params, func(p Param) bool { return p.Kind == KindJSONFile }) {
		dir, err := os.MkdirTemp("", "papierkram-mcp-")
		if err != nil {
			return errorResult(err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		tmp = dir
	}
	args, err := Args(t, raw, o, tmp)
	if err != nil {
		return errorResult(err)
	}
	return ToolResult(run(ctx, args))
}
