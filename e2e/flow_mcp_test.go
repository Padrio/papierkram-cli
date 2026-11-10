//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Spec v0.2 §3, §7: a read tool of the MCP server against the sandbox,
// in-process over the in-memory transport of the SDK (mcpSession). Hard:
// the tools of the default mode contain user_list but neither
// invoice_create (write) nor auth_status (local), each name once (no
// request); user_list with limit 1 answers without IsError with the JSON
// of user list (an array entries), and no content contains the token. A
// tool error with exit_code 6 (HTTP 429) stops the run like the quota of
// a CLI run (markQuota) and skips.
func TestE2E_MCPReadTool(t *testing.T) {
	s := live(t)
	s.need(t, 1)
	cs := s.mcpSession(t, t.TempDir())
	ctx := context.Background()

	// 1. the tools (no request)
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		s.fatalf(t, "tools/list: %v", err)
	}
	seen := map[string]int{}
	for _, tool := range list.Tools {
		seen[tool.Name]++
	}
	for name, n := range seen {
		if n > 1 {
			s.errorf(t, "Tool %s %d-mal gelistet", name, n)
		}
	}
	switch {
	case seen["user_list"] == 0:
		s.fatalf(t, "Tool user_list fehlt (%d Tools)", len(list.Tools))
	case seen["invoice_create"] > 0:
		s.errorf(t, "Tool invoice_create im Lesemodus gelistet")
	case seen["auth_status"] > 0:
		s.errorf(t, "Tool auth_status (lokaler Befehl) gelistet")
	}
	s.logf(t, "MCP-Server: %d Tools im Lesemodus", len(list.Tools))

	// 2. user_list (1)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "user_list", Arguments: map[string]any{"limit": 1}})
	if err != nil {
		s.fatalf(t, "tools/call user_list: %v", err)
	}
	var texts []string
	for i, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			s.errorf(t, "user_list: Inhalt %d ist kein Text (%T)", i, c)
			continue
		}
		if s.red.Contains(tc.Text) {
			s.errorf(t, "user_list: Inhalt %d enthält das Token", i)
			continue
		}
		texts = append(texts, tc.Text)
	}
	if len(texts) == 0 {
		s.fatalf(t, "user_list: kein Textinhalt (IsError %t)", res.IsError)
	}
	if res.IsError {
		var line struct {
			Error struct {
				Code     string `json:"code"`
				ExitCode int    `json:"exit_code"`
			} `json:"error"`
		}
		_ = json.Unmarshal([]byte(texts[0]), &line)
		if line.Error.ExitCode == 6 {
			s.markQuota()
			s.skipf(t, "Kontingent erschöpft (HTTP 429) bei Tool user_list: Test übersprungen")
		}
		s.fatalf(t, "user_list: Tool-Fehler %s", texts[0])
	}
	m := tryDecode(texts[0])
	if _, ok := m["entries"].([]any); !ok {
		s.errorf(t, "user_list: erster Inhalt ist kein JSON mit Array entries (%d Bytes)", len(texts[0]))
		return
	}
	s.measuref(t, "MCP user_list", "%d Inhalt(e), entries %d, total_entries %v", len(res.Content), len(entries(m)), m["total_entries"])
}
