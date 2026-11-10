package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// Cross path CLI ↔ MCP (spec v0.2 §3, anti-goal "MCP path and CLI path
// check or send differently"): each case runs one operation once on the
// command line and once as a tool call, each against its own fresh mock
// with the same answers. Both must send the same requests (method, path,
// query, JSON body) and print the same stdout byte for byte (the base URL
// of the mock, which differs by its port, made the same); hints on
// stderr reach the agent unchanged as the second content, an error as
// the same JSON error line.

// crossCase is one operation in both spellings. cli are the arguments of
// the command line (without -o json --no-input, which an agent passes
// as AGENTS.md says and the tool sets itself), tool and args the tool
// call; dir is the files directory of the server, the CLI gets absolute
// paths in it. setup gives both mocks the same answers; exit and reqs
// are the expected exit code and number of requests of the CLI, so no
// case passes by failing the same way twice.
type crossCase struct {
	name       string
	cli        func(dir string) []string
	tool       string
	args       map[string]any
	setup      func(t *testing.T, srv *mockapi.Server)
	exit, reqs int
}

// voucherInput is the body of the voucher create case: as a file for
// --input on the command line, as the object input in the tool call.
const voucherInput = `{"name":"Büromaterial","provenance":"domestic","creditor":{"id":2904},` +
	`"line_items":[{"name":"Papier","amount":11.9,"vat_rate":0.19,"category":"Bürobedarf"}]}`

var crossCases = []crossCase{
	{
		name: "invoice list mit Name und Seitengrenze", exit: 0, reqs: 3,
		cli: func(string) []string {
			return []string{"invoice", "list", "--company", "Mustermann", "--from", "2026-01-01", "--all", "--max-pages", "2"}
		},
		tool: "invoice_list",
		args: map[string]any{"company": "Mustermann", "from": "2026-01-01", "all": true, "max_pages": 2},
		setup: func(t *testing.T, srv *mockapi.Server) {
			srv.OnFunc("GET", "/contact/companies", pagedList(t, 1, []map[string]any{company(1, "ACME"), company(2, "Mustermann")}))
			srv.OnFunc("GET", "/income/invoices", pagedList(t, 3,
				[]map[string]any{{"id": 1, "name": "A"}}, []map[string]any{{"id": 2, "name": "B"}}, []map[string]any{{"id": 3, "name": "C"}}))
		},
	},
	{
		name: "invoice create", exit: 0, reqs: 1,
		cli: func(string) []string {
			return []string{"invoice", "create", "--name", "X", "--customer", "2", "--payment-term", "1130", "--document-date", "heute",
				"--item", "name=A,qty=1,price=100,vat=19"}
		},
		tool: "invoice_create",
		args: map[string]any{"name": "X", "customer": 2, "payment_term": 1130, "document_date": "heute", "item": []string{"name=A,qty=1,price=100,vat=19"}},
	},
	{
		name: "invoice create --dry-run", exit: 0, reqs: 0,
		cli: func(string) []string {
			return []string{"invoice", "create", "--name", "X", "--customer", "2", "--payment-term", "1130", "--document-date", "heute",
				"--item", "name=A,qty=1,price=100,vat=19", "--dry-run"}
		},
		tool: "invoice_create",
		args: map[string]any{"name": "X", "customer": 2, "payment_term": 1130, "document_date": "heute", "item": []string{"name=A,qty=1,price=100,vat=19"}, "dry_run": true},
	},
	{
		name: "invoice update liest die Rechnung", exit: 0, reqs: 2,
		cli:  func(string) []string { return []string{"invoice", "update", "459", "--name", "Y"} },
		tool: "invoice_update",
		args: map[string]any{"id": 459, "name": "Y"},
	},
	{
		name: "company archive", exit: 0, reqs: 1,
		cli:  func(string) []string { return []string{"company", "archive", "2843"} },
		tool: "company_archive",
		args: map[string]any{"id": 2843},
	},
	{
		name: "invoice delete", exit: 0, reqs: 1,
		cli:  func(string) []string { return []string{"invoice", "delete", "459", "--yes"} },
		tool: "invoice_delete",
		args: map[string]any{"id": 459},
	},
	{
		name: "invoice deliver als PDF", exit: 0, reqs: 2,
		cli:  func(string) []string { return []string{"invoice", "deliver", "459", "--via", "pdf", "--yes"} },
		tool: "invoice_deliver",
		args: map[string]any{"id": 459},
	},
	{
		name: "invoice paid-status", exit: 0, reqs: 1,
		cli:  func(string) []string { return []string{"invoice", "paid-status", "--customer", "2", "--overdue"} },
		tool: "invoice_paid_status",
		args: map[string]any{"customer": 2, "overdue": true},
		setup: func(t *testing.T, srv *mockapi.Server) {
			srv.OnFunc("GET", "/income/invoices", pagedList(t, 1, []map[string]any{
				paidEntry(1, "sent", "119.0", "2026-05-15"), paidEntry(2, "sent", "50.5", "2026-07-01"), paidEntry(3, "paid", "0.0", "2026-05-01"),
			}))
		},
	},
	{
		name: "voucher create mit --input", exit: 0, reqs: 1,
		cli: func(dir string) []string {
			return []string{"voucher", "create", "--input", filepath.Join(dir, "beleg.json")}
		},
		tool: "voucher_create",
		args: map[string]any{"input": jsonObject(voucherInput)},
	},
	{
		name: "Fehler: Rechnung fehlt", exit: 4, reqs: 1,
		cli:  func(string) []string { return []string{"invoice", "delete", "999", "--yes"} },
		tool: "invoice_delete",
		args: map[string]any{"id": 999},
		setup: func(t *testing.T, srv *mockapi.Server) {
			srv.On("DELETE", "/income/invoices/999", 404, []byte(`{"type":"error","error":"not_found","message":"nicht gefunden"}`))
		},
	},
}

// jsonObject decodes a JSON object of a test case.
func jsonObject(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return m
}

func TestMCPCrossPath(t *testing.T) {
	for _, tc := range crossCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "beleg.json"), []byte(voucherInput), 0o600); err != nil {
				t.Fatal(err)
			}

			cli, cliSrv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-mcp"})
			app, srv, _ := mcpApp(t)
			if tc.setup != nil {
				tc.setup(t, cliSrv)
				tc.setup(t, srv)
			}
			exit := Run(cli, append(tc.cli(dir), "-o", "json", "--no-input"))
			stdout, stderr := bufs(cli)
			// the only intended difference: each mock has its own port
			// (the URL of a dry run)
			stdout = strings.ReplaceAll(stdout, cliSrv.URL(), srv.URL())

			cs := mcpSession(t, app, MCPOptions{AllowDestructive: true, FilesDir: dir})
			res := callTool(t, cs, tc.tool, tc.args)

			want, got := cliSrv.Requests(), srv.Requests()
			if exit != tc.exit || len(want) != tc.reqs {
				t.Fatalf("CLI Exit %d mit %d Anfragen, erwartet %d mit %d\nstderr %s", exit, len(want), tc.exit, tc.reqs, stderr)
			}
			if len(want) != len(got) {
				t.Fatalf("Anfragen CLI %d, MCP %d\nCLI %+v\nMCP %+v", len(want), len(got), want, got)
			}
			for i := range want {
				w, g := want[i], got[i]
				sameBody := len(w.Body) == 0 && len(g.Body) == 0 || jsonEqual(w.Body, g.Body)
				if w.Method != g.Method || w.Path != g.Path || w.Query.Encode() != g.Query.Encode() || !sameBody {
					t.Errorf("Anfrage %d\nCLI %s %s?%s %s\nMCP %s %s?%s %s", i,
						w.Method, w.Path, w.Query.Encode(), w.Body, g.Method, g.Path, g.Query.Encode(), g.Body)
				}
			}
			if (exit != 0) != res.IsError {
				t.Fatalf("CLI Exit %d, MCP isError %v\nstderr %s\nMCP %+v", exit, res.IsError, stderr, res.Content)
			}
			if exit != 0 {
				if line := lastLine(stderr); contentText(t, res, 0) != line {
					t.Errorf("Fehlerzeile\nMCP %s\nCLI %s", contentText(t, res, 0), line)
				}
				return
			}
			if contentText(t, res, 0) != stdout {
				t.Errorf("stdout\nMCP %s\nCLI %s", contentText(t, res, 0), stdout)
			}
			hints := ""
			if len(res.Content) > 1 {
				hints = strings.TrimPrefix(contentText(t, res, 1), "Hinweise (stderr):\n")
			}
			if hints != stderr {
				t.Errorf("stderr\nMCP %q\nCLI %q", hints, stderr)
			}
		})
	}
}
