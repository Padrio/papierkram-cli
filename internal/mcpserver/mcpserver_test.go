package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModeAllows(t *testing.T) {
	classes := []string{"read_only", "create", "update", "archive", "destructive", "send", "local", "per_request", ""}
	for _, tc := range []struct {
		m    Mode
		want []string
	}{
		{ModeRead, []string{"read_only"}},
		{ModeWrite, []string{"read_only", "create", "update", "archive"}},
		{ModeDestructive, []string{"read_only", "create", "update", "archive", "destructive", "send"}},
		{Mode(99), nil},
	} {
		for _, c := range classes {
			if got := tc.m.Allows(c); got != slices.Contains(tc.want, c) {
				t.Errorf("Mode %d, Klasse %q: %v", tc.m, c, got)
			}
		}
	}
}

func TestTools(t *testing.T) {
	specs := []ToolSpec{
		{Name: "a", Class: "read_only"}, {Name: "b", Class: "create"}, {Name: "c", Class: "destructive"},
		{Name: "d", Class: "send"}, {Name: "e", Class: "archive"}, {Name: "f", Class: "local"},
	}
	names := func(m Mode) []string {
		var out []string
		for _, s := range Tools(specs, m) {
			out = append(out, s.Name)
		}
		return out
	}
	for m, want := range map[Mode][]string{
		ModeRead:        {"a"},
		ModeWrite:       {"a", "b", "e"},
		ModeDestructive: {"a", "b", "c", "d", "e"},
	} {
		if got := names(m); !slices.Equal(got, want) {
			t.Errorf("Mode %d: %v, erwartet %v", m, got, want)
		}
	}
}

func TestInputSchema(t *testing.T) {
	spec := ToolSpec{Name: "x", Params: []Param{
		{Name: "id", Pos: 0, Kind: KindInteger, Description: "ID", Minimum: f(0), Required: true},
		{Name: "sort", Flag: "sort", Pos: -1, Kind: KindString, Description: "Sortierung", Enum: []string{"a", "b"}, Default: "a"},
		{Name: "limit", Flag: "limit", Pos: -1, Kind: KindInteger, Description: "Anzahl", Default: 25, Minimum: f(1), Maximum: f(100)},
		{Name: "amount", Flag: "amount", Pos: -1, Kind: KindNumber, Description: "Betrag", Required: true},
		{Name: "all", Flag: "all", Pos: -1, Kind: KindBool, Description: "alle", Default: false},
		{Name: "attr", Flag: "attr", Pos: -1, Kind: KindStrings, Description: "Attribut"},
		{Name: "company", Flag: "company", Pos: -1, Kind: KindRef, Description: "Firma"},
		{Name: "input", Flag: "input", Pos: -1, Kind: KindJSONFile, JSONType: "object", Description: "Eingabe"},
		{Name: "items", Flag: "items", Pos: -1, Kind: KindJSONFile, JSONType: "array", Description: "Positionen"},
		{Name: "file", Pos: 1, Kind: KindPath, Description: "Datei", Required: true},
	}}
	got, err := json.MarshalIndent(InputSchema(spec), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "additionalProperties": false,
  "properties": {
    "all": {
      "default": false,
      "description": "alle",
      "type": "boolean"
    },
    "amount": {
      "description": "Betrag",
      "type": "number"
    },
    "attr": {
      "description": "Attribut",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "company": {
      "description": "Firma",
      "type": [
        "integer",
        "string"
      ]
    },
    "file": {
      "description": "Datei",
      "type": "string"
    },
    "id": {
      "description": "ID",
      "minimum": 0,
      "type": "integer"
    },
    "input": {
      "description": "Eingabe",
      "type": "object"
    },
    "items": {
      "description": "Positionen",
      "type": "array"
    },
    "limit": {
      "default": 25,
      "description": "Anzahl",
      "maximum": 100,
      "minimum": 1,
      "type": "integer"
    },
    "sort": {
      "default": "a",
      "description": "Sortierung",
      "enum": [
        "a",
        "b"
      ],
      "type": "string"
    }
  },
  "required": [
    "amount",
    "file",
    "id"
  ],
  "type": "object"
}`
	if string(got) != want {
		t.Errorf("Schema\n%s\nerwartet\n%s", got, want)
	}
	// without required parameters the key is left out
	s := InputSchema(ToolSpec{Name: "y"})
	if _, ok := s["required"]; ok || s["additionalProperties"] != false || s["type"] != "object" {
		t.Errorf("Schema ohne Parameter = %v", s)
	}
}

func text(t *testing.T, c mcp.Content) string {
	t.Helper()
	tc, ok := c.(*mcp.TextContent)
	if !ok {
		t.Fatalf("Inhalt %T, erwartet *mcp.TextContent", c)
	}
	return tc.Text
}

func TestToolResult(t *testing.T) {
	r := ToolResult(Result{Exit: 0, Stdout: []byte("{\"a\":1}\n")})
	if r.IsError || len(r.Content) != 1 || text(t, r.Content[0]) != "{\"a\":1}\n" {
		t.Errorf("Exit 0 ohne stderr: %+v", r)
	}
	r = ToolResult(Result{Exit: 0, Stdout: []byte("{}\n"), Stderr: []byte("Warnung: Restkontingent niedrig: 5 Credits\n")})
	if r.IsError || len(r.Content) != 2 || text(t, r.Content[0]) != "{}\n" ||
		text(t, r.Content[1]) != "Hinweise (stderr):\nWarnung: Restkontingent niedrig: 5 Credits\n" {
		t.Errorf("Exit 0 mit stderr: %+v", r)
	}
	line := `{"error":{"code":"not_found","message":"Rechnung 999 nicht gefunden","http_status":404,"exit_code":4}}`
	r = ToolResult(Result{Exit: 4, Stderr: []byte("Fehler: Rechnung 999 nicht gefunden\nHinweis: papierkram invoice list\n" + line + "\n\n")})
	if !r.IsError || len(r.Content) != 2 || text(t, r.Content[0]) != line ||
		text(t, r.Content[1]) != "Hinweise (stderr):\nFehler: Rechnung 999 nicht gefunden\nHinweis: papierkram invoice list\n" {
		t.Errorf("Exit 4: %+v", r)
	}
	// only the error line: one content
	r = ToolResult(Result{Exit: 2, Stderr: []byte(line + "\n")})
	if !r.IsError || len(r.Content) != 1 || text(t, r.Content[0]) != line {
		t.Errorf("nur Fehlerzeile: %+v", r)
	}
	// a partial result (exit 11) keeps its stdout: retry_command
	partial := `{"id":5,"completed":["create"],"failed":["upload"],"retry_command":"papierkram voucher upload 5 x.pdf"}` + "\n"
	r = ToolResult(Result{Exit: 11, Stdout: []byte(partial), Stderr: []byte(line + "\n")})
	if !r.IsError || len(r.Content) != 2 || text(t, r.Content[0]) != line || text(t, r.Content[1]) != "Ausgabe (stdout):\n"+partial {
		t.Errorf("Exit 11: %+v", r)
	}
	// no stderr at all (never from the CLI): a generic error line
	r = ToolResult(Result{Exit: 1})
	var doc struct {
		Error struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if !r.IsError || len(r.Content) != 1 || json.Unmarshal([]byte(text(t, r.Content[0])), &doc) != nil ||
		doc.Error.Code != "error" || doc.Error.ExitCode != 1 {
		t.Errorf("Exit 1 ohne stderr: %+v", r)
	}
}

// connect serves specs with run in mode m over the in-memory transport of
// the SDK and returns the client session.
func connect(t *testing.T, specs []ToolSpec, run Runner, o Options) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(ctx, specs, run, o).Connect(ctx, st, nil)
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

func TestNewListsToolsOfMode(t *testing.T) {
	pdf := ToolSpec{Name: "invoice_pdf", Class: "read_only", Command: []string{"invoice", "pdf"},
		Params: []Param{{Name: "id", Pos: 0, Kind: KindInteger, Required: true}, {Name: "output_file", Flag: "output-file", Pos: -1, Kind: KindPath, Required: true}}}
	specs := []ToolSpec{invoiceList, describe, upload, pdf,
		{Name: "invoice_delete", Class: "destructive", Command: []string{"invoice", "delete"}, Description: "Rechnung löschen"}}
	run := func(context.Context, []string) Result { return Result{} }
	cs := connect(t, specs, run, Options{Mode: ModeRead, Version: "1.2.3", FilesDir: t.TempDir()})
	if v := cs.InitializeResult().ServerInfo; v.Name != "papierkram" || v.Version != "1.2.3" {
		t.Errorf("ServerInfo = %+v", v)
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		a := tool.Annotations
		// a read tool that writes a file (pdf -O) is no read-only tool
		// (ruling T5-C)
		ro := tool.Name != "invoice_pdf"
		if a == nil || a.ReadOnlyHint != ro || a.IdempotentHint != ro || a.DestructiveHint == nil || *a.DestructiveHint ||
			a.OpenWorldHint == nil || !*a.OpenWorldHint {
			t.Errorf("%s: Annotationen %+v", tool.Name, a)
		}
	}
	slices.Sort(names)
	if want := []string{"invoice_describe", "invoice_list", "invoice_pdf"}; !slices.Equal(names, want) {
		t.Errorf("Tools = %v, erwartet %v", names, want)
	}

	cs = connect(t, specs, run, Options{Mode: ModeDestructive, FilesDir: t.TempDir()})
	res, err = cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		a := tool.Annotations
		switch tool.Name {
		case "invoice_delete":
			if a.ReadOnlyHint || a.IdempotentHint || !*a.DestructiveHint || tool.Description != "Rechnung löschen" {
				t.Errorf("%s: %+v", tool.Name, a)
			}
		case "voucher_upload":
			if a.ReadOnlyHint || a.IdempotentHint || *a.DestructiveHint {
				t.Errorf("%s: %+v", tool.Name, a)
			}
			b, _ := json.Marshal(tool.InputSchema)
			want, _ := json.Marshal(InputSchema(upload))
			if string(b) != string(want) {
				t.Errorf("%s: Schema %s, erwartet %s", tool.Name, b, want)
			}
		}
	}
	if len(res.Tools) != 5 {
		t.Errorf("%d Tools, erwartet 5", len(res.Tools))
	}
}

func TestNewCallRunsArgsAndMapsResult(t *testing.T) {
	var got []string
	run := func(_ context.Context, args []string) Result {
		got = args
		return Result{Exit: 0, Stdout: []byte(`{"id":7}` + "\n")}
	}
	cs := connect(t, []ToolSpec{describe}, run, Options{Context: "firma"})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 7}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || text(t, res.Content[0]) != `{"id":7}`+"\n" {
		t.Errorf("Ergebnis %+v", res)
	}
	if want := []string{"invoice", "describe", "--no-input", "-o", "json", "--context=firma", "--", "7"}; !slices.Equal(got, want) {
		t.Errorf("Runner-Argumente %q, erwartet %q", got, want)
	}
}

// An argument error is a tool error with the usage error line of the CLI;
// the runner is not called.
func TestNewArgErrorIsUsageError(t *testing.T) {
	called := false
	run := func(context.Context, []string) Result { called = true; return Result{} }
	cs := connect(t, []ToolSpec{describe}, run, Options{})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": "7", "via": "email"}})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("Runner trotz Argumentfehler aufgerufen")
	}
	var doc struct {
		Error map[string]any `json:"error"`
	}
	if !res.IsError || len(res.Content) != 1 {
		t.Fatalf("Ergebnis %+v", res)
	}
	line := text(t, res.Content[0])
	if err := json.Unmarshal([]byte(line), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Error["code"] != "usage" || doc.Error["exit_code"] != float64(2) || doc.Error["hint"] == "" ||
		!strings.Contains(doc.Error["message"].(string), "„") || len(doc.Error) != 4 {
		t.Errorf("Fehler %s", line)
	}
	if strings.Contains(line, `\u201`) || strings.Contains(line, `\u003`) {
		t.Errorf("Fehlerzeile maskiert Zeichen: %s", line)
	}
}

// The JSON file of a call is removed after the call.
func TestNewRemovesJSONFiles(t *testing.T) {
	var path string
	run := func(_ context.Context, args []string) Result {
		for _, a := range args {
			if p, ok := strings.CutPrefix(a, "--input="); ok {
				path = p
				if _, err := os.Stat(p); err != nil {
					t.Errorf("Datei fehlt während des Aufrufs: %v", err)
				}
			}
		}
		return Result{Stdout: []byte("{}\n")}
	}
	cs := connect(t, []ToolSpec{upload}, run, Options{Mode: ModeWrite, FilesDir: t.TempDir()})
	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "voucher_upload",
		Arguments: map[string]any{"voucher_id": 1, "file": "x.pdf", "input": map[string]any{"name": "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("kein --input")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("Verzeichnis %s nach dem Aufruf noch da: %v", filepath.Dir(path), err)
	}
}

// Review focus 5: parallel calls of one client run one at a time (the CLI
// keeps per-invocation state in its App; the runner is not re-entrant).
func TestMCPConcurrentCallsSerialized(t *testing.T) {
	var running, maxRunning, calls atomic.Int32
	run := func(context.Context, []string) Result {
		n := running.Add(1)
		for {
			m := maxRunning.Load()
			if n <= m || maxRunning.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		calls.Add(1)
		running.Add(-1)
		return Result{Stdout: []byte("{}\n")}
	}
	cs := connect(t, []ToolSpec{describe}, run, Options{})
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := range 10 {
		wg.Go(func() {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": i}})
			if err == nil && res.IsError {
				err = fmt.Errorf("Tool-Fehler %v", res.Content)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if calls.Load() != 10 || maxRunning.Load() != 1 {
		t.Errorf("%d Aufrufe, höchstens %d gleichzeitig; erwartet 10 und 1", calls.Load(), maxRunning.Load())
	}
}

// Review focus 5: a call the client cancels cancels the context of the
// runner (the CLI maps that to exit 10 or 12, TestMCPCancelMapsExit).
func TestMCPCancelledCall(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan error, 1)
	run := func(ctx context.Context, _ []string) Result {
		close(started)
		select {
		case <-ctx.Done():
			cancelled <- ctx.Err()
		case <-time.After(10 * time.Second):
			cancelled <- nil
		}
		return Result{Exit: 10}
	}
	cs := connect(t, []ToolSpec{describe}, run, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 1}})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Error("abgebrochener Aufruf ohne Fehler beim Client")
	}
	select {
	case err := <-cancelled:
		if err == nil {
			t.Error("Runner sah keinen abgebrochenen Kontext")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runner nicht beendet")
	}
	// the server stays usable: the next call runs (and is not blocked by
	// the cancelled one)
	next := make(chan struct{})
	cs2 := connect(t, []ToolSpec{describe}, func(context.Context, []string) Result {
		close(next)
		return Result{Stdout: []byte("{}\n")}
	}, Options{})
	if _, err := cs2.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 1}}); err != nil {
		t.Fatal(err)
	}
	<-next
}

// A call that waits for the lock and is cancelled meanwhile never runs.
func TestMCPCancelledWhileWaiting(t *testing.T) {
	release := make(chan struct{})
	first := make(chan struct{})
	var calls atomic.Int32
	run := func(context.Context, []string) Result {
		if calls.Add(1) == 1 {
			close(first)
			<-release
		}
		return Result{Stdout: []byte("{}\n")}
	}
	gaveUp := make(chan struct{})
	waitCancelled = func() { close(gaveUp) }
	cs := connect(t, []ToolSpec{describe}, run, Options{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	// registered after connect, so it runs first: a failing test frees
	// the blocked call before the session closes
	t.Cleanup(func() { free(); waitCancelled = func() {} })
	go func() {
		_, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 1}})
	}()
	<-first
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 2}}); err == nil {
		t.Error("wartender Aufruf ohne Fehler")
	}
	// the server saw the cancellation while the call waited for the lock
	select {
	case <-gaveUp:
	case <-time.After(5 * time.Second):
		t.Fatal("wartender Aufruf nicht abgebrochen")
	}
	free()
	// a third call proves the lock is free again; the cancelled one never ran
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 3}}); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("%d Läufe, erwartet 2 (der abgebrochene wartende Aufruf lief)", n)
	}
}

// final review A-M2: once the server stops (signal), a call waiting for
// the lock never runs, also when it gets the lock after the running call
// ended; the running call is not cancelled.
func TestMCPServerStopSkipsWaitingCalls(t *testing.T) {
	release := make(chan struct{})
	first := make(chan struct{})
	var calls atomic.Int32
	var firstCtxErr atomic.Value
	run := func(ctx context.Context, _ []string) Result {
		if calls.Add(1) == 1 {
			close(first)
			<-release
			firstCtxErr.Store(fmt.Sprint(ctx.Err()))
		}
		return Result{Stdout: []byte("{}\n")}
	}
	server, stop := context.WithCancel(context.Background())
	defer stop()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(server, []ToolSpec{describe}, run, Options{}).Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		free()
		_ = cs.Close()
		_ = ss.Wait()
	})
	done := make(chan error, 2)
	go func() {
		_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 1}})
		done <- err
	}()
	<-first
	go func() {
		_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 2}})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond) // the second call waits for the lock
	stop()
	time.Sleep(20 * time.Millisecond)
	free()
	errs := 0
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				errs++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Aufruf nicht beendet")
		}
	}
	if n := calls.Load(); n != 1 || errs != 1 {
		t.Errorf("%d Läufe, %d Fehler; erwartet 1 Lauf (der laufende) und 1 Fehler (der wartende)", n, errs)
	}
	if got := firstCtxErr.Load(); got != "<nil>" {
		t.Errorf("laufender Aufruf abgebrochen: %v", got)
	}
}

// final review A-M7: a panic in a call is a tool error (exit 1, outcome
// unknown) with the stack on the log, and the server keeps serving.
func TestMCPPanicIsToolError(t *testing.T) {
	var calls atomic.Int32
	run := func(context.Context, []string) Result {
		if calls.Add(1) == 1 {
			panic("kaputt")
		}
		return Result{Stdout: []byte("{}\n")}
	}
	var log bytes.Buffer
	cs := connect(t, []ToolSpec{describe}, run, Options{Log: &log})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 1}})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Error struct {
			Code, Message, Hint string
			ExitCode            int `json:"exit_code"`
		} `json:"error"`
	}
	if !res.IsError || json.Unmarshal([]byte(text(t, res.Content[0])), &doc) != nil || doc.Error.Code != "error" || doc.Error.ExitCode != 1 ||
		!strings.Contains(doc.Error.Message, "kaputt") || !strings.Contains(doc.Error.Hint, "Ergebnis unbekannt – per describe/list prüfen") {
		t.Errorf("Ergebnis %+v", res.Content)
	}
	if !strings.Contains(log.String(), "kaputt") || !strings.Contains(log.String(), "goroutine") {
		t.Errorf("Log ohne Stack: %q", log.String())
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "invoice_describe", Arguments: map[string]any{"id": 2}}); err != nil || res.IsError {
		t.Errorf("Server nach Panic: %v %+v", err, res)
	}
}

// Without a temporary directory a call with a JSON value ends as error
// (exit 1) without running.
func TestNewTempDirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TMPDIR gilt nur unter Unix")
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "fehlt"))
	called := false
	run := func(context.Context, []string) Result { called = true; return Result{} }
	cs := connect(t, []ToolSpec{upload}, run, Options{Mode: ModeWrite, FilesDir: t.TempDir()})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "voucher_upload",
		Arguments: map[string]any{"voucher_id": 1, "file": "x.pdf", "input": map[string]any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if called || !res.IsError || !strings.Contains(text(t, res.Content[0]), `"code":"error","message":`) ||
		!strings.Contains(text(t, res.Content[0]), `"exit_code":1`) {
		t.Errorf("Ergebnis %+v, Runner %v", res.Content, called)
	}
}

// Ruling T5-E: without a files directory no tool with a file path exists.
func TestNewWithoutFilesDir(t *testing.T) {
	pdf := ToolSpec{Name: "invoice_pdf", Class: "read_only", Command: []string{"invoice", "pdf"},
		Params: []Param{{Name: "output_file", Flag: "output-file", Pos: -1, Kind: KindPath, Required: true}}}
	run := func(context.Context, []string) Result { return Result{} }
	cs := connect(t, []ToolSpec{invoiceList, pdf, upload}, run, Options{Mode: ModeDestructive})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "invoice_list" {
		t.Errorf("Tools %+v", res.Tools)
	}
}

// Ruling T6-A: without a files directory only the optional file path
// parameters are left out; a tool goes missing only when a path is
// required (voucher upload <datei>, pdf -O). voucher create stays usable
// without file.
func TestNewOptionalPathWithoutFilesDir(t *testing.T) {
	create := ToolSpec{Name: "voucher_create", Class: "create", Command: []string{"voucher", "create"}, Params: []Param{
		{Name: "name", Flag: "name", Pos: -1, Kind: KindString},
		{Name: "file", Flag: "file", Pos: -1, Kind: KindPath},
	}}
	var got []string
	run := func(_ context.Context, args []string) Result { got = args; return Result{Stdout: []byte("{}\n")} }
	cs := connect(t, []ToolSpec{create, upload}, run, Options{Mode: ModeWrite})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "voucher_create" {
		t.Fatalf("Tools %+v", res.Tools)
	}
	b, _ := json.Marshal(res.Tools[0].InputSchema)
	if strings.Contains(string(b), `"file"`) || !strings.Contains(string(b), `"name"`) || !strings.Contains(string(b), `"dry_run"`) {
		t.Errorf("Schema ohne Dateiordner %s", b)
	}
	call := func(args map[string]any) *mcp.CallToolResult {
		r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "voucher_create", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := call(map[string]any{"name": "X"}); r.IsError || !slices.Equal(got, []string{"voucher", "create", "--name=X", "--no-input", "-o", "json", "--"}) {
		t.Errorf("Ergebnis %+v, Argumente %q", r.Content, got)
	}
	got = nil
	if r := call(map[string]any{"name": "X", "file": "beleg.pdf"}); !r.IsError || got != nil ||
		!strings.Contains(text(t, r.Content[0]), "unbekannter Parameter „file“") {
		t.Errorf("file ohne Dateiordner: %+v, Runner %q", r.Content, got)
	}
	// Args applies the same rule
	if _, err := Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x.pdf"}`), Options{Mode: ModeWrite}, t.TempDir()); err == nil {
		t.Error("Pflichtpfad ohne Dateiordner angenommen")
	}
	if _, err := Args(create, json.RawMessage(`{"file":"x.pdf"}`), Options{Mode: ModeWrite}, t.TempDir()); err == nil {
		t.Error("optionaler Pfad ohne Dateiordner angenommen")
	}
	// with a files directory file is a parameter again
	cs = connect(t, []ToolSpec{create}, run, Options{Mode: ModeWrite, FilesDir: t.TempDir()})
	res, err = cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(res.Tools[0].InputSchema); !strings.Contains(string(b), `"file"`) {
		t.Errorf("Schema mit Dateiordner %s", b)
	}
}

// M2: DestructiveHint is true for every class but read_only and create
// (update and archive change or hide existing data, they are not
// additive); read-only and idempotent only for read_only without a file.
func TestAnnotationsPerClass(t *testing.T) {
	for class, want := range map[string][3]bool{
		// destructive, read-only, idempotent
		"read_only":   {false, true, true},
		"create":      {false, false, false},
		"update":      {true, false, false},
		"archive":     {true, false, false},
		"destructive": {true, false, false},
		"send":        {true, false, false},
	} {
		a := annotations(ToolSpec{Name: "x", Class: class})
		if got := [3]bool{*a.DestructiveHint, a.ReadOnlyHint, a.IdempotentHint}; got != want || !*a.OpenWorldHint {
			t.Errorf("%s: %v, erwartet %v", class, got, want)
		}
	}
}
