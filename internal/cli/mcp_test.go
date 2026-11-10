package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Padrio/papierkram-cli/internal/mcpserver"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// mcpApp is a test app against a fresh mock with the token
// test-token-mcp; its Err is the stderr of the server process.
func mcpApp(t *testing.T) (*App, *mockapi.Server, *bytes.Buffer) {
	t.Helper()
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-mcp"})
	return app, srv, app.Err.(*bytes.Buffer)
}

// mcpSession serves NewMCPServer(app, o) over the in-memory transport.
func mcpSession(t *testing.T, app *App, o MCPOptions) *mcp.ClientSession {
	t.Helper()
	s, err := NewMCPServer(app, o)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Wait()
	})
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func contentText(t *testing.T, res *mcp.CallToolResult, i int) string {
	t.Helper()
	if len(res.Content) <= i {
		t.Fatalf("Inhalt %d fehlt: %+v", i, res.Content)
	}
	return res.Content[i].(*mcp.TextContent).Text
}

// errorOf decodes the JSON error line of a tool error.
func errorOf(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if !res.IsError {
		t.Fatalf("kein Tool-Fehler: %+v", res.Content)
	}
	var doc struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(contentText(t, res, 0)), &doc); err != nil {
		t.Fatalf("Fehlerzeile %q: %v", contentText(t, res, 0), err)
	}
	return doc.Error
}

// Cross path: the tool call sends the same request and returns the same
// stdout as the command line.
func TestMCPReadToolCall(t *testing.T) {
	app, srv, _ := mcpApp(t)
	cli, _, _ := newTestApp(t)
	cli.Env = app.Env
	if code := Run(cli, []string{"invoice", "list", "--limit", "1", "-o", "json", "--no-input"}); code != 0 {
		t.Fatalf("CLI Exit %d: %s", code, cli.Err)
	}
	cliReqs := srv.Requests()
	cs := mcpSession(t, app, MCPOptions{})
	res := callTool(t, cs, "invoice_list", map[string]any{"limit": 1})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := srv.Requests()[len(cliReqs):]
	if len(cliReqs) != 1 || len(reqs) != 1 {
		t.Fatalf("Anfragen CLI %d, MCP %d", len(cliReqs), len(reqs))
	}
	c, m := cliReqs[0], reqs[0]
	if m.Method != http.MethodGet || m.Path != "/income/invoices" || m.Query.Encode() != "page_size=1" ||
		c.Method != m.Method || c.Path != m.Path || c.Query.Encode() != m.Query.Encode() || !bytes.Equal(c.Body, m.Body) {
		t.Errorf("CLI %s %s?%s, MCP %s %s?%s", c.Method, c.Path, c.Query.Encode(), m.Method, m.Path, m.Query.Encode())
	}
	if got := contentText(t, res, 0); got != cli.Out.(*bytes.Buffer).String() {
		t.Errorf("Tool-Ergebnis weicht von stdout ab\nMCP: %s\nCLI: %s", got, cli.Out)
	}
	if len(res.Content) != 1 {
		t.Errorf("Hinweise ohne Anlass: %+v", res.Content)
	}
}

func TestMCPErrorIsStructured(t *testing.T) {
	app, srv, _ := mcpApp(t)
	srv.On(http.MethodGet, "/income/invoices/999", http.StatusNotFound,
		[]byte(`{"type":"error","error":"not_found","message":"nicht gefunden"}`))
	cs := mcpSession(t, app, MCPOptions{})
	e := errorOf(t, callTool(t, cs, "invoice_describe", map[string]any{"id": 999}))
	if e["code"] != "not_found" || e["exit_code"] != float64(4) || e["http_status"] != float64(404) {
		t.Errorf("Fehler %v", e)
	}
	// an argument error is the usage error of the CLI (exit 2), no request
	n := len(srv.Requests())
	e = errorOf(t, callTool(t, cs, "invoice_describe", map[string]any{"id": "abc"}))
	if e["code"] != "usage" || e["exit_code"] != float64(2) || len(srv.Requests()) != n {
		t.Errorf("Fehler %v, %d Anfragen", e, len(srv.Requests())-n)
	}
	// a flag error of the CLI is a tool error as well
	e = errorOf(t, callTool(t, cs, "invoice_list", map[string]any{"limit": 500}))
	if e["code"] != "usage" || e["exit_code"] != float64(2) || len(srv.Requests()) != n {
		t.Errorf("Fehler %v, %d Anfragen", e, len(srv.Requests())-n)
	}
}

// A call cancelled during the request ends like Ctrl+C on the command
// line: exit 10, nothing changed.
func TestMCPCancelMapsExit(t *testing.T) {
	app, srv, _ := mcpApp(t)
	arrived := make(chan struct{})
	srv.OnFunc(http.MethodGet, "/income/invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-arrived
		cancel()
	}()
	r := app.mcpRunner()(ctx, []string{"invoice", "describe", "--no-input", "-o", "json", "--", "5"})
	res := mcpserver.ToolResult(r)
	e := errorOf(t, res)
	if r.Exit != 10 || e["code"] != "network" || e["exit_code"] != float64(10) {
		t.Errorf("Exit %d, Fehler %v", r.Exit, e)
	}
	if len(srv.Requests()) != 1 {
		t.Errorf("%d Anfragen, erwartet 1 (keine Wiederholung nach Abbruch)", len(srv.Requests()))
	}
}

// --debug at the start writes the request log to the stderr of the
// process, never into the tool result; the token is in neither.
func TestMCPDebugGoesToStderr(t *testing.T) {
	app, _, procErr := mcpApp(t)
	app.Flags.Debug = true
	cs := mcpSession(t, app, MCPOptions{})
	res := callTool(t, cs, "invoice_describe", map[string]any{"id": 5})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	var all strings.Builder
	for i := range res.Content {
		all.WriteString(contentText(t, res, i))
	}
	if !strings.Contains(procErr.String(), "> GET ") || !strings.Contains(procErr.String(), "Bearer ***") {
		t.Errorf("Debug-Zeilen fehlen auf stderr: %q", procErr)
	}
	if strings.Contains(all.String(), "> GET") || len(res.Content) != 1 {
		t.Errorf("Debug-Zeilen im Tool-Ergebnis: %q", all.String())
	}
	if strings.Contains(all.String(), "test-token-mcp") || strings.Contains(procErr.String(), "test-token-mcp") {
		t.Error("Token in Ausgabe")
	}
	if app.Out.(*bytes.Buffer).Len() != 0 {
		t.Errorf("stdout des Prozesses beschrieben: %q", app.Out)
	}
}

// Hints and warnings on stderr reach the agent as a second content.
func TestMCPHintsAsSecondContent(t *testing.T) {
	app, srv, _ := mcpApp(t)
	srv.On(http.MethodGet, "/income/invoices/5", http.StatusOK, []byte(`{"id":5}`), "X-Remaining-Quota", "42")
	cs := mcpSession(t, app, MCPOptions{})
	res := callTool(t, cs, "invoice_describe", map[string]any{"id": 5})
	if res.IsError || len(res.Content) != 2 || !strings.HasPrefix(contentText(t, res, 1), "Hinweise (stderr):\n") ||
		!strings.Contains(contentText(t, res, 1), "42 Credits") {
		t.Errorf("Ergebnis %+v", res.Content)
	}
}

// --context at the start applies to every call: the token of that
// context is used, and an unknown context stops the start.
func TestMCPContext(t *testing.T) {
	app, srv, _ := mcpApp(t)
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("b", "test-token-b"); err != nil {
		t.Fatal(err)
	}
	env := app.Env
	app.Env = func(k string) string {
		if k == "PAPIERKRAM_TOKEN" {
			return ""
		}
		return env(k)
	}
	cs := mcpSession(t, app, MCPOptions{Context: "b"})
	if res := callTool(t, cs, "info", nil); res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "Bearer test-token-b" {
		t.Errorf("Anfragen %+v", reqs)
	}
	if _, err := NewMCPServer(app, MCPOptions{Context: "fehlt"}); err == nil {
		t.Error("unbekannter Kontext ohne Fehler")
	}
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	app.Out, app.Err = out, errb
	if code := Run(app, []string{"mcp", "serve", "--context", "fehlt", "-o", "json"}); code == 0 || out.Len() != 0 {
		t.Errorf("Exit %d, stdout %q", code, out)
	}
	if !strings.Contains(errb.String(), `"exit_code"`) {
		t.Errorf("stderr %q", errb)
	}
}

func TestMCPChild(t *testing.T) {
	app, _, procErr := mcpApp(t)
	app.Flags.Debug = true
	app.Flags.Context = "x"
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	c := app.mcpChild(out, errb)
	if c.Out != out || c.Err != errb || c.debugOut() != procErr || c.StdinTTY || c.StdoutTTY ||
		c.ConfigDir != app.ConfigDir || c.CacheDir != app.CacheDir || c.Keyring != app.Keyring || c.HTTPClient != app.HTTPClient ||
		c.Location != app.Location || c.Flags.Debug || c.Flags.Context != "" {
		t.Errorf("Kind %+v", c)
	}
	if b, err := io.ReadAll(c.In); err != nil || len(b) != 0 {
		t.Errorf("stdin %q, %v", b, err)
	}
	if _, err := c.ReadPassword(); err == nil {
		t.Error("ReadPassword ohne Fehler")
	}
	if c.Env("PAPIERKRAM_TOKEN") != "test-token-mcp" || !c.Now().Equal(app.Now()) {
		t.Error("Env oder Now nicht übernommen")
	}
	// a child of a child keeps the stderr of the process for --debug
	if c.mcpChild(io.Discard, io.Discard).debugOut() != procErr {
		t.Error("DebugOut nicht weitergegeben")
	}
	// without DebugOut, debugOut is Err
	plain, _, errOnly := newTestApp(t)
	if plain.debugOut() != errOnly {
		t.Error("debugOut ohne DebugOut ist nicht Err")
	}
}

// syncBuffer is a bytes.Buffer safe for the writer goroutine of the
// transport and the reading test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Review focus 1: mcp serve over In/Out (stdio): everything on stdout is
// JSON-RPC, closing stdin ends the server with exit 0.
func TestMCPServeStdioOnlyProtocol(t *testing.T) {
	app, _, procErr := mcpApp(t)
	clientToServer, serverIn := io.Pipe()
	serverOut, serverToClient := io.Pipe()
	var wire syncBuffer
	app.In = clientToServer
	app.Out = io.MultiWriter(serverToClient, &wire)
	exit := make(chan int, 1)
	go func() {
		code := Run(app, []string{"mcp", "serve", "--debug"})
		_ = serverToClient.Close()
		exit <- code
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).
		Connect(context.Background(), &mcp.IOTransport{Reader: serverOut, Writer: serverIn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatalf("tools/list: %v, %v", tools, err)
	}
	for _, tool := range tools.Tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s ist kein Lese-Tool", tool.Name)
		}
	}
	res := callTool(t, cs, "invoice_list", map[string]any{"limit": 1})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("Exit %d, stderr %s", code, procErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mcp serve endet nicht nach EOF")
	}
	sc := bufio.NewScanner(strings.NewReader(wire.String()))
	sc.Buffer(nil, 1<<24)
	n := 0
	for sc.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil || msg["jsonrpc"] != "2.0" {
			t.Errorf("keine JSON-RPC-Nachricht auf stdout: %q", sc.Text())
		}
		n++
	}
	if n < 3 {
		t.Errorf("%d Nachrichten auf stdout, erwartet mindestens 3 (initialize, tools/list, tools/call)", n)
	}
	// --debug of the serve process: the request log is on stderr
	if !strings.Contains(procErr.String(), "> GET ") {
		t.Errorf("Debug-Zeilen fehlen auf stderr: %q", procErr)
	}
}

// A signal (the cancelled command context) ends mcp serve with exit 0.
func TestMCPServeEndsOnCancel(t *testing.T) {
	app, _, _ := mcpApp(t)
	r, w := io.Pipe()
	defer func() { _ = w.Close() }()
	app.In = r
	ctx, cancel := context.WithCancel(context.Background())
	exit := make(chan int, 1)
	go func() { exit <- RunContext(ctx, app, []string{"mcp", "serve"}) }()
	time.AfterFunc(50*time.Millisecond, cancel)
	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("Exit %d: %s", code, app.Err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mcp serve endet nicht nach Abbruch")
	}
	if app.Out.(*bytes.Buffer).Len() != 0 {
		t.Errorf("stdout %q", app.Out)
	}
}

func TestCommandsJSONMCPServe(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	c := d.find(t, "mcp serve")
	if c.Class != "local" || c.Credits == nil || *c.Credits != 0 || c.Blocked == nil || *c.Blocked || c.OperationID != "" {
		t.Errorf("mcp serve: %+v", c)
	}
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"mcp", "serve", "--dry-run"}); code != 2 || !strings.Contains(errb.String(), "--dry-run") {
		t.Errorf("--dry-run: Exit %d, %s", code, errb)
	}
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"mcp", "serve", "extra"}); code != 2 || out.Len() != 0 {
		t.Errorf("Argument: Exit %d", code)
	}
	app, out, _ = newTestApp(t)
	if code := Run(app, []string{"mcp"}); code != 0 || !strings.Contains(out.String(), "serve") {
		t.Errorf("mcp: Exit %d, %s", code, out)
	}
}

// A stdout that breaks means the client is gone: the server ends like at
// EOF, with exit 0 and nothing on stderr.
func TestMCPServeWriteFails(t *testing.T) {
	app, _, errb := mcpApp(t)
	app.In = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n")
	app.Out = failWriter{}
	if code := Run(app, []string{"mcp", "serve", "-o", "json"}); code != 0 || errb.Len() != 0 {
		t.Errorf("Exit %d, stderr %s", code, errb)
	}
}
