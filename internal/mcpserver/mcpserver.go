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
	"os"
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
// (--context, empty: the CLI chooses), --debug for every call and the
// version the server reports.
type Options struct {
	Mode    Mode
	Context string
	Debug   bool
	Version string
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
	for _, p := range t.Params {
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

// annotations are the hints of a tool of class.
func annotations(class string) *mcp.ToolAnnotations {
	destructive := class == "destructive" || class == "send"
	openWorld := true
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    class == "read_only",
		DestructiveHint: &destructive,
		IdempotentHint:  class == "read_only",
		OpenWorldHint:   &openWorld,
	}
}

// waitCancelled is called when a call is cancelled while it waits for the
// lock (a test seam).
var waitCancelled = func() {}

// New builds the MCP server with one tool per spec that o.Mode allows. A
// call is translated by Args and run by run; one lock serializes all
// calls, because the CLI keeps per-invocation state (one App per run, but
// shared files like the name cache and the timer). A call cancelled while
// it waits never runs.
func New(specs []ToolSpec, run Runner, o Options) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "papierkram", Version: o.Version}, nil)
	lock := make(chan struct{}, 1)
	for _, t := range Tools(specs, o.Mode) {
		tool := &mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: InputSchema(t),
			Annotations: annotations(t.Class),
		}
		s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			select {
			case lock <- struct{}{}:
			case <-ctx.Done():
				waitCancelled()
				return nil, ctx.Err()
			}
			defer func() { <-lock }()
			return call(ctx, t, req.Params.Arguments, run, o), nil
		})
	}
	return s
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
