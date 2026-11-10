package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// stdioServer is mcp serve running over pipes as In/Out, with a client
// connected over the same pipes; wire records everything on Out.
type stdioServer struct {
	cs   *mcp.ClientSession
	wire *syncBuffer
	exit chan int
}

// serveStdio starts RunContext(ctx, app, args) with pipes as In/Out and
// connects a client.
func serveStdio(t *testing.T, ctx context.Context, app *App, args ...string) *stdioServer {
	t.Helper()
	clientToServer, serverIn := io.Pipe()
	serverOut, serverToClient := io.Pipe()
	s := &stdioServer{wire: &syncBuffer{}, exit: make(chan int, 1)}
	app.In = clientToServer
	app.Out = io.MultiWriter(serverToClient, s.wire)
	go func() {
		code := RunContext(ctx, app, args)
		_ = serverToClient.Close()
		s.exit <- code
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).
		Connect(context.Background(), &mcp.IOTransport{Reader: serverOut, Writer: serverIn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.cs = cs
	return s
}

// close closes the client and returns the exit code of the server.
func (s *stdioServer) close(t *testing.T) int {
	t.Helper()
	if err := s.cs.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-s.exit:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("mcp serve endet nicht nach EOF")
	}
	return -1
}

// checkWire requires every line on Out to be a JSON-RPC 2.0 message and
// returns their number.
func (s *stdioServer) checkWire(t *testing.T) int {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(s.wire.String()))
	sc.Buffer(nil, 1<<24)
	n := 0
	for sc.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil || msg["jsonrpc"] != "2.0" {
			t.Errorf("keine JSON-RPC-Nachricht auf stdout: %q", sc.Text())
		}
		n++
	}
	return n
}

// Review focus 1: mcp serve over In/Out (stdio): everything on stdout is
// JSON-RPC, closing stdin ends the server with exit 0.
func TestMCPServeStdioOnlyProtocol(t *testing.T) {
	app, _, procErr := mcpApp(t)
	s := serveStdio(t, context.Background(), app, "mcp", "serve", "--debug")
	tools, err := s.cs.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatalf("tools/list: %v, %v", tools, err)
	}
	for _, tool := range tools.Tools {
		a := tool.Annotations
		// a pdf tool writes a local file (ruling T5-C)
		ro := !strings.HasSuffix(tool.Name, "_pdf")
		if a.ReadOnlyHint != ro || *a.DestructiveHint {
			t.Errorf("%s: %+v", tool.Name, a)
		}
	}
	res := callTool(t, s.cs, "invoice_list", map[string]any{"limit": 1})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	if code := s.close(t); code != 0 {
		t.Errorf("Exit %d, stderr %s", code, procErr)
	}
	if n := s.checkWire(t); n < 3 {
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

// A broken stdout with stdin still open: the session ends with the write
// error, not with EOF (CI fix: the race of TestMCPServeWriteFails made
// deterministic). The client is gone all the same: exit 0, no error.
func TestMCPServeWriteFailsStdinOpen(t *testing.T) {
	app, _, errb := mcpApp(t)
	r, w := io.Pipe()
	defer func() { _ = w.Close() }()
	app.In = r
	app.Out = failWriter{}
	exit := make(chan int, 1)
	go func() { exit <- Run(app, []string{"mcp", "serve"}) }()
	if _, err := io.WriteString(w, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`+"\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-exit:
		if code != 0 || strings.Contains(errb.String(), "Fehler") {
			t.Errorf("Exit %d, stderr %s", code, errb)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mcp serve endet nicht nach kaputtem stdout")
	}
}

// A stdout that breaks means the client is gone: the server ends like at
// EOF, with exit 0 and no error on stderr (only the start hint).
func TestMCPServeWriteFails(t *testing.T) {
	app, _, errb := mcpApp(t)
	app.In = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n")
	app.Out = failWriter{}
	if code := Run(app, []string{"mcp", "serve"}); code != 0 || strings.Contains(errb.String(), "Fehler") {
		t.Errorf("Exit %d, stderr %s", code, errb)
	}
}

// syncEnv is an environment a test may change while the server runs.
type syncEnv struct {
	mu sync.Mutex
	m  map[string]string
}

func (e *syncEnv) get(k string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.m[k]
}

func (e *syncEnv) set(k, v string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.m[k] = v
}

// mcpEnvApp is mcpApp with an environment the test can change later.
func mcpEnvApp(t *testing.T, extra map[string]string) (*App, *mockapi.Server, *syncEnv) {
	t.Helper()
	app, srv, _ := mcpApp(t)
	env := &syncEnv{m: map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-mcp"}}
	for k, v := range extra {
		env.m[k] = v
	}
	app.Env = env.get
	return app, srv, env
}

// toolNames lists the tools of a session, sorted.
func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

// wantToolsOfMode lists the tools commands --json implies for mode m
// (every command a tool may have whose class m allows), sorted.
func wantToolsOfMode(t *testing.T, m mcpserver.Mode) []string {
	t.Helper()
	var want []string
	for _, c := range decodeCommands(t, runCommandsJSON(t, "--json")).Commands {
		if m.Allows(c.Class) && !wantNoTool(c) {
			want = append(want, strings.NewReplacer(" ", "_", "-", "_").Replace(c.Command))
		}
	}
	slices.Sort(want)
	return want
}

func TestMCPModeFlags(t *testing.T) {
	app, _, _ := mcpApp(t)
	for _, tc := range []struct {
		o    MCPOptions
		want mcpserver.Mode
	}{
		{MCPOptions{}, mcpserver.ModeRead},
		{MCPOptions{AllowWrites: true}, mcpserver.ModeWrite},
		{MCPOptions{AllowDestructive: true}, mcpserver.ModeDestructive},
		{MCPOptions{AllowWrites: true, AllowDestructive: true}, mcpserver.ModeDestructive},
	} {
		m, err := app.mcpMode(tc.o)
		if err != nil || m != tc.want {
			t.Errorf("%+v: Mode %d, %v; erwartet %d", tc.o, m, err, tc.want)
		}
	}
	if s := app.Err.(*bytes.Buffer).String(); s != "" {
		t.Errorf("stderr %q", s)
	}
	// a broken configuration stops the start
	writeCfg(t, app, "kaputt = [")
	if _, err := app.mcpMode(MCPOptions{AllowWrites: true}); err == nil {
		t.Error("kaputte Konfiguration ohne Fehler")
	}
}

// The options of the start decide the tools: exactly the classes of
// Mode.Allows; voucher_pay and the deliver tools only with
// --allow-destructive, time_log from --allow-writes on.
func TestMCPToolListPerMode(t *testing.T) {
	for _, tc := range []struct {
		flags []string
		mode  mcpserver.Mode
	}{
		{nil, mcpserver.ModeRead},
		{[]string{"--allow-writes"}, mcpserver.ModeWrite},
		{[]string{"--allow-destructive"}, mcpserver.ModeDestructive},
		{[]string{"--allow-writes", "--allow-destructive"}, mcpserver.ModeDestructive},
	} {
		app, _, procErr := mcpApp(t)
		args := append([]string{"mcp", "serve", "--files-dir", t.TempDir()}, tc.flags...)
		s := serveStdio(t, context.Background(), app, args...)
		got := toolNames(t, s.cs)
		if code := s.close(t); code != 0 {
			t.Fatalf("%v: Exit %d, %s", tc.flags, code, procErr)
		}
		if want := wantToolsOfMode(t, tc.mode); !slices.Equal(got, want) {
			t.Errorf("%v: Tools\n got %v\nwant %v", tc.flags, got, want)
		}
		for name, from := range map[string]mcpserver.Mode{
			"invoice_list": mcpserver.ModeRead, "time_log": mcpserver.ModeWrite, "company_create": mcpserver.ModeWrite,
			"company_archive": mcpserver.ModeWrite, "voucher_upload": mcpserver.ModeWrite, "voucher_pay": mcpserver.ModeDestructive,
			"invoice_deliver": mcpserver.ModeDestructive, "estimate_deliver": mcpserver.ModeDestructive, "invoice_delete": mcpserver.ModeDestructive,
		} {
			if slices.Contains(got, name) != (tc.mode >= from) {
				t.Errorf("%v: %s vorhanden = %v", tc.flags, name, slices.Contains(got, name))
			}
		}
		if procErr.Len() != 0 {
			t.Errorf("%v: stderr %q", tc.flags, procErr)
		}
	}
}

// startReadOnly starts mcp serve --allow-destructive in app and returns
// its tools and the stderr of the start.
func startReadOnly(t *testing.T, app *App) ([]string, string) {
	t.Helper()
	s := serveStdio(t, context.Background(), app, "mcp", "serve", "--allow-destructive", "--allow-writes", "--files-dir", t.TempDir())
	got := toolNames(t, s.cs)
	if code := s.close(t); code != 0 {
		t.Fatalf("Exit %d", code)
	}
	return got, app.Err.(*bytes.Buffer).String()
}

func TestMCPReadOnlyEnvForcesRead(t *testing.T) {
	app, _, _ := mcpEnvApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	got, errs := startReadOnly(t, app)
	if want := wantToolsOfMode(t, mcpserver.ModeRead); !slices.Equal(got, want) {
		t.Errorf("Tools\n got %v\nwant %v", got, want)
	}
	if want := "Hinweis: Schreibschutz aktiv (PAPIERKRAM_READ_ONLY): --allow-writes/--allow-destructive wirkungslos, nur lesende Tools\n"; errs != want {
		t.Errorf("stderr %q\nerwartet %q", errs, want)
	}
	// without the options the read mode needs no hint
	app, _, _ = mcpEnvApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	if m, err := app.mcpMode(MCPOptions{}); err != nil || m != mcpserver.ModeRead || app.Err.(*bytes.Buffer).Len() != 0 {
		t.Errorf("Mode %d, %v, stderr %q", m, err, app.Err)
	}
}

func TestMCPReadOnlyContextForcesRead(t *testing.T) {
	app, _, env := mcpEnvApp(t, nil)
	env.set("PAPIERKRAM_TOKEN", "")
	writeCfg(t, app, "active_context = \"agent\"\n\n[contexts.agent]\nsubdomain = \"firma\"\nread_only = true\n")
	if err := app.Keyring.Set("agent", "test-token-agent"); err != nil {
		t.Fatal(err)
	}
	got, errs := startReadOnly(t, app)
	if want := wantToolsOfMode(t, mcpserver.ModeRead); !slices.Equal(got, want) {
		t.Errorf("Tools\n got %v\nwant %v", got, want)
	}
	if want := "Hinweis: Schreibschutz aktiv (Kontext „agent“ ist schreibgeschützt): --allow-writes/--allow-destructive wirkungslos, nur lesende Tools\n"; errs != want {
		t.Errorf("stderr %q\nerwartet %q", errs, want)
	}
}

// The mode only decides which tools exist; every call still passes the
// read-only gate of the CLI: PAPIERKRAM_READ_ONLY set after the start
// blocks a write tool with exit 9 and no request.
func TestMCPGateStillHolds(t *testing.T) {
	app, srv, env := mcpEnvApp(t, nil)
	cs := mcpSession(t, app, MCPOptions{AllowWrites: true})
	env.set("PAPIERKRAM_READ_ONLY", "1")
	e := errorOf(t, callTool(t, cs, "company_create", map[string]any{"name": "X", "contact_type": "customer"}))
	if e["code"] != "read_only" || e["exit_code"] != float64(9) {
		t.Errorf("Fehler %v", e)
	}
	wantRequests(t, srv, 0)
}

func TestMCPWriteCall(t *testing.T) {
	app, srv, _ := mcpApp(t)
	created := `{"id":2843,"name":"X","contact_type":"customer"}`
	srv.On(http.MethodPost, "/contact/companies", http.StatusCreated, []byte(created))
	cs := mcpSession(t, app, MCPOptions{AllowWrites: true})
	res := callTool(t, cs, "company_create", map[string]any{"name": "X", "contact_type": "customer"})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Method != http.MethodPost || reqs[0].Path != "/contact/companies" || !jsonEqual(reqs[0].Body, []byte(`{"name":"X","contact_type":"customer"}`)) {
		t.Errorf("Anfrage %s %s %s", reqs[0].Method, reqs[0].Path, reqs[0].Body)
	}
	if got := contentText(t, res, 0); !jsonEqual([]byte(got), []byte(created)) {
		t.Errorf("Ergebnis %s", got)
	}
}

func TestMCPWriteDryRun(t *testing.T) {
	app, srv, _ := mcpApp(t)
	cs := mcpSession(t, app, MCPOptions{AllowWrites: true})
	res := callTool(t, cs, "company_create", map[string]any{"name": "X", "contact_type": "customer", "dry_run": true})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	wantRequests(t, srv, 0)
	wantContract(t, "dry-run", []byte(contentText(t, res, 0)))
}

func TestMCPDestructiveCall(t *testing.T) {
	app, srv, _ := mcpApp(t)
	cs := mcpSession(t, app, MCPOptions{AllowDestructive: true})
	res := callTool(t, cs, "invoice_delete", map[string]any{"id": 459})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Method != http.MethodDelete || reqs[0].Path != "/income/invoices/459" {
		t.Errorf("Anfrage %s %s", reqs[0].Method, reqs[0].Path)
	}
	if got := contentText(t, res, 0); !jsonEqual([]byte(got), []byte(`{"id":459,"deleted":true}`)) {
		t.Errorf("Ergebnis %s", got)
	}
	// with --allow-writes only, the tool does not exist: a protocol error
	app, srv, _ = mcpApp(t)
	cs = mcpSession(t, app, MCPOptions{AllowWrites: true})
	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_delete", Arguments: map[string]any{"id": 459}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("Fehler %v, erwartet unbekanntes Tool", err)
	}
	wantRequests(t, srv, 0)
}

// deliver over MCP sends only as PDF: the tool has id and dry_run, its
// body is that of invoice deliver 459 --via pdf --yes, and the mail
// parameters do not exist.
func TestMCPDeliverPdfOnly(t *testing.T) {
	specs := testToolSpecs(t)
	for _, name := range []string{"invoice_deliver", "estimate_deliver"} {
		props := toolSchema(t, specByName(t, specs, name))["properties"].(map[string]any)
		if got := slices.Sorted(maps.Keys(props)); !slices.Equal(got, []string{"dry_run", "id"}) {
			t.Errorf("%s: Parameter %v", name, got)
		}
	}
	// every send tool is a deliver tool: fixed --via=pdf, no flag
	// parameter (no --input with send_via, no mail flag)
	n := 0
	for _, s := range specs {
		if s.Class != "send" {
			continue
		}
		n++
		if !slices.Equal(s.Fixed, []string{"--via=pdf"}) || slices.ContainsFunc(s.Params, func(p mcpserver.Param) bool { return p.Pos < 0 }) ||
			!strings.Contains(s.Description, "nur als PDF") {
			t.Errorf("%s: %+v", s.Name, s)
		}
	}
	if n != 2 {
		t.Errorf("%d send-Tools, erwartet 2", n)
	}
	cli, cliSrv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-mcp"})
	wantExit(t, cli, Run(cli, []string{"invoice", "deliver", "459", "--via", "pdf", "--yes", "-o", "json", "--no-input"}), 0)
	app, srv, _ := mcpApp(t)
	cs := mcpSession(t, app, MCPOptions{AllowDestructive: true})
	res := callTool(t, cs, "invoice_deliver", map[string]any{"id": 459})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	want, got := cliSrv.Requests(), srv.Requests()
	if len(got) != 2 || len(want) != 2 || got[0].Method != http.MethodPost || got[0].Path != "/income/invoices/459/deliver" ||
		!jsonEqual(got[0].Body, []byte(`{"send_via":"pdf"}`)) || !jsonEqual(got[0].Body, want[0].Body) {
		t.Errorf("Anfragen MCP %+v, CLI %+v", got, want)
	}
	n = len(srv.Requests())
	e := errorOf(t, callTool(t, cs, "invoice_deliver", map[string]any{"id": 459, "via": "email"}))
	if e["code"] != "usage" || e["exit_code"] != float64(2) || len(srv.Requests()) != n {
		t.Errorf("Fehler %v, %d Anfragen", e, len(srv.Requests())-n)
	}
}

// IDs may be names in a write tool as on the command line (task 3): one
// path, the company list, then the POST with the ID.
func TestMCPNameResolution(t *testing.T) {
	app, srv, _ := mcpApp(t)
	mustermann(t, srv)
	cs := mcpSession(t, app, MCPOptions{AllowWrites: true})
	res := callTool(t, cs, "invoice_create", map[string]any{
		"name": "X", "customer": "Mustermann GmbH", "payment_term": 1130, "document_date": "heute",
		"item": []string{"name=Beratung,qty=2,unit=Stunde,price=100,vat=19"},
	})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := wantPaths(t, srv, "GET /contact/companies", "POST /income/invoices")
	var body struct {
		Customer struct {
			ID int64 `json:"id"`
		} `json:"customer"`
	}
	if err := json.Unmarshal(reqs[1].Body, &body); err != nil || body.Customer.ID != 2 {
		t.Errorf("Body %s (%v)", reqs[1].Body, err)
	}
	if len(res.Content) < 2 || !strings.Contains(contentText(t, res, 1), "Hinweis: Kunde „Mustermann GmbH“ → 2 (1 Credit)") {
		t.Errorf("Hinweis fehlt: %+v", res.Content)
	}
}

// Carry of task 5: the read paths of the write tools (voucher upload
// <datei>, voucher create --file) follow the files directory policy like
// the pdf target: a path outside is an argument error without a request,
// a file inside is sent.
func TestMCPWritePathsInFilesDir(t *testing.T) {
	app, srv, _ := mcpApp(t)
	dir, outside := t.TempDir(), t.TempDir()
	secret := filepath.Join(outside, "geheim.pdf")
	for _, p := range []string{secret, filepath.Join(dir, "beleg.pdf")} {
		if err := os.WriteFile(p, []byte(binaryPDF), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bad := []string{secret, "../" + filepath.Base(outside) + "/geheim.pdf", "-", "/dev/stdin", ""}
	if os.Symlink(secret, filepath.Join(dir, "verweis.pdf")) == nil {
		bad = append(bad, "verweis.pdf")
	}
	cs := mcpSession(t, app, MCPOptions{AllowWrites: true, FilesDir: dir})
	for _, p := range bad {
		for name, args := range map[string]map[string]any{
			"voucher_upload": {"voucher_id": 5, "file": p},
			"voucher_create": {"name": "X", "provenance": "domestic", "item": []string{"name=A,amount=1,vat=19"}, "file": p},
		} {
			if e := errorOf(t, callTool(t, cs, name, args)); e["code"] != "usage" || e["exit_code"] != float64(2) {
				t.Errorf("%s %q: Fehler %v", name, p, e)
			}
		}
	}
	wantRequests(t, srv, 0)
	res := callTool(t, cs, "voucher_upload", map[string]any{"voucher_id": 5, "file": "beleg.pdf"})
	if res.IsError {
		t.Fatalf("Tool-Fehler %+v", res.Content)
	}
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Method != http.MethodPost || !strings.HasPrefix(reqs[0].Path, "/expense/vouchers/5/") || !bytes.Contains(reqs[0].Body, []byte(binaryPDF)) {
		t.Errorf("Anfrage %s %s", reqs[0].Method, reqs[0].Path)
	}
	// without --files-dir the tools with files do not exist
	app, _, _ = mcpApp(t)
	cs = mcpSession(t, app, MCPOptions{AllowDestructive: true})
	got := toolNames(t, cs)
	for _, name := range []string{"voucher_upload", "voucher_create", "invoice_pdf"} {
		if slices.Contains(got, name) {
			t.Errorf("%s ohne --files-dir", name)
		}
	}
	if !slices.Contains(got, "invoice_deliver") || !slices.Contains(got, "voucher_pay") {
		t.Errorf("Tools ohne Dateien fehlen: %v", got)
	}
}

// jsonEqual reports whether a and b are the same JSON value.
func jsonEqual(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}
