package mcpserver

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prefixes of the further contents of a tool result.
const (
	stderrPrefix = "Hinweise (stderr):\n"
	stdoutPrefix = "Ausgabe (stdout):\n"
)

// ToolResult maps the result of a CLI run to the tool result. Exit 0: the
// stdout JSON is the first content; hints and warnings on stderr follow as
// a second content (a warning like "NICHT erneut ausführen" must reach the
// agent). Exit != 0: IsError, the first content is the last non-empty line
// of stderr (the compact JSON error line of -o json), the rest of stderr
// follows, then stdout when the command wrote one (exit 11: the partial
// result with retry_command).
func ToolResult(r Result) *mcp.CallToolResult {
	if r.Exit == 0 {
		res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(r.Stdout)}}}
		if len(bytes.TrimSpace(r.Stderr)) > 0 {
			res.Content = append(res.Content, &mcp.TextContent{Text: stderrPrefix + string(r.Stderr)})
		}
		return res
	}
	rest, line := splitLastLine(string(r.Stderr))
	if line == "" {
		line = errorLine("error", "Befehl ohne Fehlermeldung beendet", "", r.Exit)
	}
	res := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: line}}}
	if strings.TrimSpace(rest) != "" {
		res.Content = append(res.Content, &mcp.TextContent{Text: stderrPrefix + rest})
	}
	if len(bytes.TrimSpace(r.Stdout)) > 0 {
		res.Content = append(res.Content, &mcp.TextContent{Text: stdoutPrefix + string(r.Stdout)})
	}
	return res
}

// splitLastLine splits s into the text before its last non-empty line
// (with its line breaks) and that line.
func splitLastLine(s string) (rest, line string) {
	s = strings.TrimRight(s, "\r\n\t ")
	i := strings.LastIndexByte(s, '\n')
	return s[:i+1], strings.TrimRight(s[i+1:], "\r")
}

// errorBody is the error object of the JSON error line of the CLI
// (docs/schemas/error.json) for errors the server finds itself.
type errorBody struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// errorLine is the compact JSON error line {"error":{…}} without HTML
// escaping, like the CLI writes it.
func errorLine(code, msg, hint string, exit int) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	// strings and an int always encode
	_ = enc.Encode(map[string]errorBody{"error": {Code: code, Message: msg, Hint: hint, ExitCode: exit}})
	return strings.TrimRight(b.String(), "\n")
}

// errorResult is the tool error of an error the server finds before the
// CLI runs: an ArgError is a usage error (exit 2), anything else exit 1.
func errorResult(err error) *mcp.CallToolResult {
	line := errorLine("error", err.Error(), "", 1)
	if ae, ok := err.(*ArgError); ok {
		line = errorLine("usage", ae.Msg, ae.Hint, 2)
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: line}}}
}
