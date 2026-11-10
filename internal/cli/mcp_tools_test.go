package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/mcpserver"
)

// testToolSpecs are the tool specs of the tree of a test app.
func testToolSpecs(t *testing.T) []mcpserver.ToolSpec {
	t.Helper()
	app, _, _ := newTestApp(t)
	specs, err := mcpToolSpecs(NewRootCmd(app))
	if err != nil {
		t.Fatal(err)
	}
	return specs
}

func specByName(t *testing.T, specs []mcpserver.ToolSpec, name string) mcpserver.ToolSpec {
	t.Helper()
	for _, s := range specs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("Tool %s fehlt", name)
	return mcpserver.ToolSpec{}
}

// wantNoTool reports whether commands --json lists c without a tool
// (spec v0.2 §3, brief task 5), independently of mcpToolSpecs.
func wantNoTool(c cmdInfo) bool {
	first := strings.Fields(c.Command)[0]
	return c.Class == classLocal || c.Class == classPerRequest ||
		slices.Contains([]string{"auth", "context", "local", "completion", "api", "commands", "schema", "version", "help", "gen-docs", "mcp"}, first) ||
		slices.Contains([]string{"time start", "time stop", "time status", "time discard"}, c.Command)
}

func TestMCPToolSpecsReadMode(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	var want []string
	for _, c := range d.Commands {
		if c.Class == "read_only" && !wantNoTool(c) {
			want = append(want, strings.NewReplacer(" ", "_", "-", "_").Replace(c.Command))
		}
	}
	slices.Sort(want)
	var got []string
	for _, s := range mcpserver.Tools(testToolSpecs(t), mcpserver.ModeRead) {
		got = append(got, s.Name)
		for _, p := range s.Params {
			if p.Pos < 0 && slices.Contains([]string{"id", "voucher_id", "document_id", "file", "dry_run"}, p.Name) {
				t.Errorf("%s: Flag-Parameter %s kollidiert", s.Name, p.Name)
			}
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("Tools\n got %v\nwant %v", got, want)
	}
	for _, name := range []string{"invoice_list", "invoice_describe", "invoice_pdf", "invoice_paid_status", "info", "user_list"} {
		if !slices.Contains(got, name) {
			t.Errorf("%s fehlt", name)
		}
	}
	for _, name := range []string{"auth_status", "invoice_create", "time_status", "commands", "version", "context_list", "api"} {
		if slices.Contains(got, name) {
			t.Errorf("%s darf kein Tool sein", name)
		}
	}
}

// Every command but the excluded ones has a tool, with a unique name and
// unique parameters; all mode filters see the same specs.
func TestMCPToolSpecsAllCommands(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	specs := testToolSpecs(t)
	names := map[string]string{}
	for _, s := range specs {
		if prev, ok := names[s.Name]; ok {
			t.Errorf("Name %s doppelt (%s)", s.Name, prev)
		}
		names[s.Name] = strings.Join(s.Command, " ")
		seen := map[string]bool{}
		for _, p := range s.Params {
			if seen[p.Name] {
				t.Errorf("%s: Parameter %s doppelt", s.Name, p.Name)
			}
			seen[p.Name] = true
		}
	}
	n := 0
	for _, c := range d.Commands {
		if wantNoTool(c) {
			continue
		}
		n++
		name := strings.NewReplacer(" ", "_", "-", "_").Replace(c.Command)
		if names[name] != c.Command {
			t.Errorf("%s: Tool %s für %q", c.Command, name, names[name])
		}
	}
	if n != len(specs) {
		t.Errorf("%d Tools, erwartet %d", len(specs), n)
	}
	for name, cmd := range map[string]string{"voucher_document_delete": "voucher document delete", "payment_term_list": "payment-term list", "info": "info"} {
		if names[name] != cmd {
			t.Errorf("%s = %q, erwartet %q", name, names[name], cmd)
		}
	}
}

func toolSchema(t *testing.T, s mcpserver.ToolSpec) map[string]any {
	t.Helper()
	b, err := json.Marshal(mcpserver.InputSchema(s))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func propJSON(t *testing.T, schema map[string]any, name string, drop ...string) string {
	t.Helper()
	p, ok := schema["properties"].(map[string]any)[name].(map[string]any)
	if !ok {
		t.Fatalf("Eigenschaft %s fehlt", name)
	}
	for _, k := range drop {
		delete(p, k)
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func TestMCPToolSchemasFromCommands(t *testing.T) {
	specs := testToolSpecs(t)
	list := toolSchema(t, specByName(t, specs, "invoice_list"))
	if got := propJSON(t, list, "limit", "description"); got != `{"default":25,"maximum":100,"minimum":1,"type":"integer"}` {
		t.Errorf("invoice_list.limit = %s", got)
	}
	if got := propJSON(t, list, "company", "description"); got != `{"type":["integer","string"]}` {
		t.Errorf("invoice_list.company = %s", got)
	}
	if got := propJSON(t, list, "all", "description"); got != `{"type":"boolean"}` {
		t.Errorf("invoice_list.all = %s", got)
	}
	if got := propJSON(t, list, "max_pages", "description"); got != `{"minimum":1,"type":"integer"}` {
		t.Errorf("invoice_list.max_pages = %s", got)
	}
	if !strings.Contains(propJSON(t, list, "from"), "YYYY-MM-DD") {
		t.Errorf("invoice_list.from = %s", propJSON(t, list, "from"))
	}
	if list["additionalProperties"] != false || list["required"] != nil {
		t.Errorf("invoice_list: %v", list)
	}
	if got := propJSON(t, toolSchema(t, specByName(t, specs, "time_list")), "state", "description"); got !=
		`{"enum":["billed","unbilled","billable","unbillable","archived"],"type":"string"}` {
		t.Errorf("time_list.state = %s", got)
	}
	if got := propJSON(t, toolSchema(t, specByName(t, specs, "invoice_paid_status")), "max_pages", "description"); got != `{"default":5,"minimum":1,"type":"integer"}` {
		t.Errorf("invoice_paid_status.max_pages = %s", got)
	}
	// time list --invoice has no name resolution: an integer ID ≥ 0
	if got := propJSON(t, toolSchema(t, specByName(t, specs, "time_list")), "invoice", "description"); got != `{"minimum":0,"type":"integer"}` {
		t.Errorf("time_list.invoice = %s", got)
	}

	// pdf tools never replace a file: --no-clobber is fixed, no parameter
	for _, name := range []string{"invoice_pdf", "estimate_pdf", "voucher_pdf"} {
		ps := specByName(t, specs, name)
		if !slices.Equal(ps.Fixed, []string{"--no-clobber"}) || slices.ContainsFunc(ps.Params, func(p mcpserver.Param) bool { return p.Flag == "no-clobber" }) {
			t.Errorf("%s: Fixed %v, Params %+v", name, ps.Fixed, ps.Params)
		}
	}
	if s := specByName(t, specs, "invoice_list"); len(s.Fixed) != 0 {
		t.Errorf("invoice_list Fixed %v", s.Fixed)
	}
	pdf := toolSchema(t, specByName(t, specs, "invoice_pdf"))
	if req, _ := json.Marshal(pdf["required"]); string(req) != `["id","output_file"]` {
		t.Errorf("invoice_pdf required = %s", req)
	}
	if got := propJSON(t, pdf, "id"); got != `{"description":"ID","minimum":0,"type":"integer"}` {
		t.Errorf("invoice_pdf.id = %s", got)
	}
	if d := propJSON(t, pdf, "output_file"); strings.Contains(d, "- für stdout") || !strings.Contains(d, `"type":"string"`) {
		t.Errorf("invoice_pdf.output_file = %s", d)
	}
	// a required flag with a preset (person list --company) is not
	// required in the schema: the preset may fill it
	if r := toolSchema(t, specByName(t, specs, "person_list"))["required"]; r != nil {
		t.Errorf("person_list required = %v", r)
	}
	// project describe [<id>]: optional
	if r := toolSchema(t, specByName(t, specs, "project_describe"))["required"]; r != nil {
		t.Errorf("project_describe required = %v", r)
	}

	// write tools (task 6 serves them): kinds of input, items, file and
	// repeatable flags, and required without alternatives
	create := specByName(t, specs, "invoice_create")
	cs := toolSchema(t, create)
	if got := propJSON(t, cs, "input", "description"); got != `{"type":"object"}` {
		t.Errorf("invoice_create.input = %s", got)
	}
	if got := propJSON(t, cs, "items", "description"); got != `{"type":"array"}` {
		t.Errorf("invoice_create.items = %s", got)
	}
	if got := propJSON(t, cs, "item", "description"); got != `{"items":{"type":"string"},"type":"array"}` {
		t.Errorf("invoice_create.item = %s", got)
	}
	if strings.Contains(propJSON(t, cs, "input"), "Standardeingabe") {
		t.Errorf("invoice_create.input nennt stdin: %s", propJSON(t, cs, "input"))
	}
	if cs["required"] != nil {
		t.Errorf("invoice_create required = %v (alle Pflichtflags haben Alternativen)", cs["required"])
	}
	up := toolSchema(t, specByName(t, specs, "voucher_upload"))
	if req, _ := json.Marshal(up["required"]); string(req) != `["file","voucher_id"]` {
		t.Errorf("voucher_upload required = %s", req)
	}
	upSpec := specByName(t, specs, "voucher_upload")
	for _, p := range upSpec.Params {
		if (p.Name == "voucher_id" && (p.Pos != 0 || p.Kind != mcpserver.KindInteger)) || (p.Name == "file" && (p.Pos != 1 || p.Kind != mcpserver.KindPath)) {
			t.Errorf("voucher_upload %+v", p)
		}
	}
	dd := toolSchema(t, specByName(t, specs, "voucher_document_delete"))
	if req, _ := json.Marshal(dd["required"]); string(req) != `["document_id","voucher_id"]` {
		t.Errorf("voucher_document_delete required = %s", req)
	}
	vc := specByName(t, specs, "voucher_create")
	for _, p := range vc.Params {
		if p.Name == "file" && (p.Pos != -1 || p.Kind != mcpserver.KindPath || p.Flag != "file") {
			t.Errorf("voucher_create.file %+v", p)
		}
	}
	// time log --task and --user are required, but presets may fill them
	if r := toolSchema(t, specByName(t, specs, "time_log"))["required"]; r != nil {
		t.Errorf("time_log required = %v", r)
	}
	// the rule itself: required only without alternatives and presets
	for _, tc := range []struct {
		f    flagInfo
		want bool
	}{
		{flagInfo{Name: "a", Type: "string", Required: true}, true},
		{flagInfo{Name: "a", Type: "string", Required: true, Alternatives: []string{"--input"}}, false},
		{flagInfo{Name: "a", Type: "id", Required: true, DefaultFrom: []string{"flag", "env"}}, false},
		{flagInfo{Name: "a", Type: "string"}, false},
		{flagInfo{Name: "output-file", Type: "string"}, true},
	} {
		p, err := mcpFlagParam(tc.f)
		if err != nil || p.Required != tc.want {
			t.Errorf("%+v: Required %v (%v), erwartet %v", tc.f, p.Required, err, tc.want)
		}
	}
}

func TestMCPToolDescriptions(t *testing.T) {
	specs := testToolSpecs(t)
	for name, want := range map[string]string{
		"invoice_list":         "Rechnungen auflisten. Klasse read_only, 1 Credit je Seite; ID-Felder nehmen auch Namen (1 Credit je geladener Seite)",
		"invoice_describe":     "Rechnung anzeigen. Klasse read_only, 1 Credit je Aufruf",
		"invoice_create":       "Rechnung anlegen. Klasse create, 50 Credits je Aufruf; ID-Felder nehmen auch Namen (1 Credit je geladener Seite)",
		"invoice_update":       "Rechnung ändern. Klasse update, 2 Credits je Aufruf; ID-Felder nehmen auch Namen (1 Credit je geladener Seite)",
		"bank_connection_list": "Bankverbindungen auflisten. Klasse read_only, 1 Credit je Seite",
	} {
		s := specByName(t, specs, name)
		if s.Description != want {
			t.Errorf("%s: %q\nerwartet %q", name, s.Description, want)
		}
	}
	// a flag that costs extra says so
	for _, p := range specByName(t, specs, "voucher_create").Params {
		if p.Name == "file" && !strings.Contains(p.Description, "10 weitere Credits") {
			t.Errorf("voucher_create.file: %q", p.Description)
		}
	}
}

// Every positional token of a command with a tool is mapped (fail
// closed: an unknown token stops mcpToolSpecs).
func TestMCPPositionalTokensKnown(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, c := range d.Commands {
		if wantNoTool(c) {
			continue
		}
		for _, tok := range strings.Fields(strings.TrimPrefix(c.Usage, "papierkram "+c.Command)) {
			if tok == "[Optionen]" {
				continue
			}
			if _, ok := mcpPositionals[strings.TrimSuffix(strings.TrimPrefix(tok, "["), "]")]; !ok {
				t.Errorf("%s: Token %s ohne Zuordnung", c.Command, tok)
			}
		}
	}
	for _, usage := range []string{"papierkram x y <name> [Optionen]", "papierkram x y <id> extra", "papierkram x y [Optionen] <id>", "papierkram z <id> [Optionen]"} {
		if _, err := mcpToolSpec(commandInfo{Command: "x y", Usage: usage, Class: "read_only", Credits: new(int)}); err == nil {
			t.Errorf("%s: kein Fehler", usage)
		}
	}
	// a flag type without kind and a flag named like a positional or
	// dry_run stop it as well
	for _, f := range []flagInfo{{Name: "x", Type: "weird"}, {Name: "id", Type: "string"}, {Name: "dry-run", Type: "boolean"}} {
		c := commandInfo{Command: "x y", Usage: "papierkram x y <id> [Optionen]", Class: "read_only", Credits: new(int), Flags: []flagInfo{f}}
		if _, err := mcpToolSpec(c); err == nil {
			t.Errorf("Flag %+v: kein Fehler", f)
		}
	}
}

func TestMCPFlagParamKinds(t *testing.T) {
	one, five := 1, 5
	for _, tc := range []struct {
		f    flagInfo
		kind mcpserver.Kind
		def  any
		desc string
	}{
		{flagInfo{Name: "a", Type: "number", Default: "1.5"}, mcpserver.KindNumber, 1.5, ""},
		{flagInfo{Name: "a", Type: "boolean", Default: "true"}, mcpserver.KindBool, true, ""},
		{flagInfo{Name: "a", Type: "string", Default: "heute", Description: "Tag"}, mcpserver.KindString, "heute", "Tag"},
		{flagInfo{Name: "a", Type: "date", Description: "Wert"}, mcpserver.KindString, nil, "Wert (Datum: YYYY-MM-DD, heute, gestern)"},
		{flagInfo{Name: "a", Type: "date", Description: "ab (YYYY-MM-DD)"}, mcpserver.KindString, nil, "ab (YYYY-MM-DD)"},
		{flagInfo{Name: "a", Type: "duration", Description: "Dauer"}, mcpserver.KindString, nil, "Dauer (Dauer, z. B. 30s, 5m oder 1h30m)"},
		{flagInfo{Name: "a", Type: "integer", Credits: &five, Description: "x"}, mcpserver.KindInteger, nil, "x (zusätzlich 5 Credits)"},
		{flagInfo{Name: "a", Type: "integer", Credits: &one, Description: "y (1 Credit)"}, mcpserver.KindInteger, nil, "y (1 Credit)"},
		{flagInfo{Name: "team-member", Type: "id", Repeatable: true, Resolves: "user"}, mcpserver.KindStrings, nil, ""},
		{flagInfo{Name: "body-file", Type: "string", Description: "Text aus Datei oder - für die Standardeingabe"}, mcpserver.KindPath, nil,
			"Text aus Datei; relativer Pfad im Dateiordner des Servers (mcp serve --files-dir), ohne „..“"},
	} {
		p, err := mcpFlagParam(tc.f)
		if err != nil {
			t.Errorf("%+v: %v", tc.f, err)
			continue
		}
		if p.Kind != tc.kind || p.Default != tc.def || p.Description != tc.desc || p.Name != mcpName(tc.f.Name) {
			t.Errorf("%+v: %+v", tc.f, p)
		}
	}
	// a default of a kind without one, an unparsable default: errors
	for _, f := range []flagInfo{{Name: "a", Type: "string", Repeatable: true, Default: "x"}, {Name: "a", Type: "integer", Default: "x"}} {
		if _, err := mcpFlagParam(f); err == nil {
			t.Errorf("%+v: kein Fehler", f)
		}
	}
}

// Two commands with the same tool name stop mcpToolSpecsOf.
func TestMCPToolSpecsDuplicateName(t *testing.T) {
	zero := 0
	cmds := []commandInfo{
		{Command: "payment-term list", Usage: "papierkram payment-term list [Optionen]", Class: "read_only", Credits: &zero},
		{Command: "payment term list", Usage: "papierkram payment term list [Optionen]", Class: "read_only", Credits: &zero},
	}
	if _, err := mcpToolSpecsOf(cmds); err == nil || !strings.Contains(err.Error(), "payment_term_list") {
		t.Errorf("Fehler %v", err)
	}
	if _, err := mcpToolSpecsOf(cmds[:1]); err != nil {
		t.Error(err)
	}
	if _, err := mcpToolSpecsOf([]commandInfo{{Command: "x", Usage: "papierkram x <name> [Optionen]", Class: "read_only", Credits: &zero}}); err == nil {
		t.Error("unbekanntes Token ohne Fehler")
	}
}
