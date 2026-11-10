package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// commandsContract is the checked-in contract of commands --json (spec 11).
const commandsContract = contractDir + "/commands.json"

// loadCommandsContract loads the contract as an OpenAPI 3 Schema Object
// and checks that it is a valid one.
func loadCommandsContract(t *testing.T) *openapi3.Schema {
	t.Helper()
	return loadContract(t, "commands")
}

// runCommandsJSON runs papierkram commands with args and returns stdout
// (exit 0 required). A token in the environment must never show up.
func runCommandsJSON(t *testing.T, args ...string) []byte {
	t.Helper()
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, append([]string{"commands"}, args...)), 0)
	assertNoToken(t, app)
	out, errs := bufs(app)
	if errs != "" {
		t.Errorf("stderr = %q", errs)
	}
	return []byte(out)
}

// cmdDoc is commands --json decoded for the tests.
type cmdDoc struct {
	Version     int       `json:"version"`
	Commands    []cmdInfo `json:"commands"`
	GlobalFlags []flgInfo `json:"global_flags"`
}

type cmdInfo struct {
	Command     string    `json:"command"`
	Usage       string    `json:"usage"`
	Aliases     []string  `json:"aliases"`
	Short       string    `json:"short"`
	Class       string    `json:"class"`
	Credits     *int      `json:"credits"`
	Blocked     *bool     `json:"blocked_by_read_only"`
	OperationID string    `json:"operation_id"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Flags       []flgInfo `json:"flags"`
	Examples    []string  `json:"examples"`
}

type flgInfo struct {
	Name         string   `json:"name"`
	Shorthand    *string  `json:"shorthand"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	Repeatable   bool     `json:"repeatable"`
	Enum         []string `json:"enum"`
	Description  string   `json:"description"`
	LocalFilter  bool     `json:"local_filter"`
	DefaultFrom  []string `json:"default_from"`
	Env          string   `json:"env"`
	Alternatives []string `json:"alternatives"`
	Credits      *int     `json:"credits"`
	Default      *string  `json:"default"`
	Minimum      *float64 `json:"minimum"`
	Maximum      *float64 `json:"maximum"`
	Resolves     string   `json:"resolves"`
}

func decodeCommands(t *testing.T, raw []byte) cmdDoc {
	t.Helper()
	var d cmdDoc
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		t.Fatalf("commands --json: %v\n%s", err, raw)
	}
	return d
}

func (d cmdDoc) find(t *testing.T, command string) cmdInfo {
	t.Helper()
	for _, c := range d.Commands {
		if c.Command == command {
			return c
		}
	}
	t.Fatalf("%s fehlt in commands --json", command)
	return cmdInfo{}
}

func (c cmdInfo) flag(t *testing.T, name string) flgInfo {
	t.Helper()
	for _, f := range c.Flags {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s: Flag --%s fehlt", c.Command, name)
	return flgInfo{}
}

// The output validates against the checked-in contract (OpenAPI Schema
// Object, kin-openapi), which is strict: an unknown property, an unknown
// class or an operation without method fail.
func TestCommandsJSONMatchesContract(t *testing.T) {
	schema := loadCommandsContract(t)
	raw := runCommandsJSON(t, "--json")
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if err := schema.VisitJSON(v); err != nil {
		t.Fatalf("commands --json verletzt %s: %v", commandsContract, err)
	}

	// strictness: each mutation of a valid document must fail
	// each change must fail for the rule it names: want is a part of the
	// error kin-openapi reports for that rule. Rules inside oneOf/allOf
	// report only one branch at the top, so for them the changed object
	// (obj) is checked against the subschema of the rule (rule) itself.
	cmdItem := schema.Properties["commands"].Value.Items.Value
	flagItem := cmdItem.Properties["flags"].Value.Items.Value
	branch := func(i int) *openapi3.Schema { return cmdItem.OneOf[i].Value }
	type mutation struct {
		fn   func(d map[string]any)
		want string
		rule *openapi3.Schema
		obj  func(d map[string]any) any
	}
	mutate := map[string]mutation{
		"unbekanntes Feld oben":   {func(d map[string]any) { d["extra"] = 1 }, `property "extra" is unsupported`, nil, nil},
		"unbekanntes Befehlsfeld": {func(d map[string]any) { cmdOf(d, "version")["use"] = "x" }, `property "use" is unsupported`, nil, nil},
		"usage fehlt":             {func(d map[string]any) { delete(cmdOf(d, "version"), "usage") }, `property "usage" is missing`, nil, nil},
		"usage ohne papierkram":   {func(d map[string]any) { cmdOf(d, "version")["usage"] = "version" }, "regular expression", nil, nil},
		"shorthand leer":          {func(d map[string]any) { reqFlag(d)["shorthand"] = "" }, "regular expression", nil, nil},
		"shorthand null":          {func(d map[string]any) { reqFlag(d)["shorthand"] = nil }, "Value is not nullable", nil, nil},
		"shorthand zu lang":       {func(d map[string]any) { reqFlag(d)["shorthand"] = "ab" }, "regular expression", nil, nil},
		"alternatives leer":       {func(d map[string]any) { reqFlag(d)["alternatives"] = []any{} }, "minimum number of items is 1", nil, nil},
		"alternative ohne --":     {func(d map[string]any) { reqFlag(d)["alternatives"] = []any{"input"} }, "regular expression", nil, nil},
		"alternative doppelt":     {func(d map[string]any) { reqFlag(d)["alternatives"] = []any{"--input", "--input"} }, "duplicate items found", nil, nil},
		"alternatives ohne required": {func(d map[string]any) { optFlag(d)["alternatives"] = []any{"--input"} }, `any schema from "anyOf"`,
			flagItem.AllOf[0].Value, func(d map[string]any) any { return optFlag(d) }},
		"alternatives kein Array": {func(d map[string]any) { reqFlag(d)["alternatives"] = "--input" }, "value must be an array", nil, nil},
		"blocked fehlt":           {func(d map[string]any) { delete(cmdOf(d, "version"), "blocked_by_read_only") }, `property "blocked_by_read_only" is missing`, nil, nil},
		"blocked kein bool":       {func(d map[string]any) { cmdOf(d, "version")["blocked_by_read_only"] = "ja" }, "value must be a boolean", nil, nil},
		"per_request nicht blockiert": {func(d map[string]any) { cmdOf(d, "api")["blocked_by_read_only"] = false }, `"/blocked_by_read_only"`,
			branch(2), func(d map[string]any) any { return cmdOf(d, "api") }},
		"Mutation nicht blockiert": {func(d map[string]any) { cmdOf(d, "invoice create")["blocked_by_read_only"] = false }, `any schema from "anyOf"`,
			branch(0), func(d map[string]any) any { return cmdOf(d, "invoice create") }},
		"Flag-Credits 0":    {func(d map[string]any) { flagOf(d, "voucher create", "file")["credits"] = 0.0 }, "number must be at least 1", nil, nil},
		"Flag-Credits Text": {func(d map[string]any) { flagOf(d, "voucher create", "file")["credits"] = "10" }, "value must be an integer", nil, nil},
		"default leer":      {func(d map[string]any) { flagOf(d, "invoice list", "limit")["default"] = "" }, "minimum string length is 1", nil, nil},
		"default als Zahl":  {func(d map[string]any) { flagOf(d, "invoice list", "limit")["default"] = 25.0 }, "value must be a string", nil, nil},
		"default null":      {func(d map[string]any) { globalFlag(d, "timeout")["default"] = nil }, "Value is not nullable", nil, nil},
		"minimum als Text":  {func(d map[string]any) { flagOf(d, "invoice list", "limit")["minimum"] = "1" }, "value must be a number", nil, nil},
		"maximum null":      {func(d map[string]any) { flagOf(d, "invoice list", "limit")["maximum"] = nil }, "Value is not nullable", nil, nil},
		"Grenze bei Dauer": {func(d map[string]any) { globalFlag(d, "timeout")["minimum"] = 1.0 }, `any schema from "anyOf"`,
			flagItem.AllOf[2].Value, func(d map[string]any) any { return globalFlag(d, "timeout") }},
		"Grenze bei Schalter": {func(d map[string]any) { flagOf(d, "invoice list", "all")["maximum"] = 1.0 }, `any schema from "anyOf"`,
			flagItem.AllOf[2].Value, func(d map[string]any) any { return flagOf(d, "invoice list", "all") }},
		"globaler shorthand null": {func(d map[string]any) { globalFlag(d, "output")["shorthand"] = nil }, "Value is not nullable", nil, nil},
		// R3-M2: the enum of class itself rejects the value; want is its
		// whole message with the complete list of classes (before, an
		// anyOf branch of a oneOf matched the generic text)
		"unbekannte Klasse": {func(d map[string]any) { cmdOf(d, "version")["class"] = "write" },
			`value is not one of the allowed values ["read_only","create","update","archive","destructive","send","local","per_request"]`,
			cmdItem.Properties["class"].Value, func(d map[string]any) any { return cmdOf(d, "version")["class"] }},
		"Operation ohne Methode": {func(d map[string]any) { delete(cmdOf(d, "invoice create"), "method") }, `property "method" is missing`, nil, nil},
		"lokal mit Credits": {func(d map[string]any) { cmdOf(d, "version")["credits"] = 1.0 }, `"/credits"`,
			branch(1), func(d map[string]any) any { return cmdOf(d, "version") }},
		"per_request mit Credits": {func(d map[string]any) { cmdOf(d, "api")["credits"] = 1.0 }, `"/credits"`,
			branch(2), func(d map[string]any) any { return cmdOf(d, "api") }},
		"unbekanntes Flag-Feld": {func(d map[string]any) { reqFlag(d)["short"] = "x" }, `property "short" is unsupported`, nil, nil},
		"unbekannter Flag-Typ":  {func(d map[string]any) { reqFlag(d)["type"] = "int64" }, "value is not one of the allowed values", nil, nil},
		"enum fehlt":            {func(d map[string]any) { delete(reqFlag(d), "enum") }, `property "enum" is missing`, nil, nil},
		"unbekannte Quelle":     {func(d map[string]any) { flagOf(d, "task list", "project")["default_from"] = []any{"cache"} }, "value is not one of the allowed values", nil, nil},
		"env fehlt zu default_from": {func(d map[string]any) { delete(flagOf(d, "task list", "project"), "env") }, `any schema from "anyOf"`,
			flagItem.AllOf[1].Value, func(d map[string]any) any { return flagOf(d, "task list", "project") }},
		"globales Flag-Feld":       {func(d map[string]any) { globalFlag(d, "output")["x"] = true }, `property "x" is unsupported`, nil, nil},
		"resolves leer":            {func(d map[string]any) { flagOf(d, "invoice create", "customer")["resolves"] = "" }, "minimum string length is 1", nil, nil},
		"resolves als Zahl":        {func(d map[string]any) { flagOf(d, "invoice create", "customer")["resolves"] = 1.0 }, "value must be a string", nil, nil},
		"resolves null":            {func(d map[string]any) { globalFlag(d, "output")["resolves"] = nil }, "Value is not nullable", nil, nil},
		"falsche Vertragsversion":  {func(d map[string]any) { d["version"] = 2.0 }, "value is not one of the allowed values", nil, nil},
		"Beispiel ohne papierkram": {func(d map[string]any) { cmdOf(d, "invoice create")["examples"] = []any{"invoice create"} }, "regular expression", nil, nil},
	}
	for name, m := range mutate {
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if m.rule != nil {
			// the rule holds for the unchanged object
			if err := m.rule.VisitJSON(m.obj(doc)); err != nil {
				t.Errorf("%s: Regel lehnt schon das unveränderte Objekt ab: %v", name, err)
			}
		}
		m.fn(doc)
		err := schema.VisitJSON(doc)
		if m.rule != nil && err != nil {
			err = m.rule.VisitJSON(m.obj(doc))
		}
		switch {
		case err == nil:
			t.Errorf("%s: Vertrag lässt das Dokument durch", name)
		case !strings.Contains(err.Error(), m.want):
			t.Errorf("%s: abgelehnt, aber nicht wegen %q:\n%v", name, m.want, err)
		}
	}
}

// cmdOf is the command of a decoded commands document.
func cmdOf(d map[string]any, command string) map[string]any {
	for _, c := range d["commands"].([]any) {
		if m := c.(map[string]any); m["command"] == command {
			return m
		}
	}
	panic(command)
}

// flagOf is a flag of a command of a decoded commands document.
func flagOf(d map[string]any, command, name string) map[string]any {
	for _, f := range cmdOf(d, command)["flags"].([]any) {
		if m := f.(map[string]any); m["name"] == name {
			return m
		}
	}
	panic(command + " --" + name)
}

// globalFlag is a global flag of a decoded commands document.
func globalFlag(d map[string]any, name string) map[string]any {
	for _, f := range d["global_flags"].([]any) {
		if m := f.(map[string]any); m["name"] == name {
			return m
		}
	}
	panic(name)
}

// reqFlag is a required flag with alternatives (invoice create --name),
// optFlag an optional one without (invoice create --description).
func reqFlag(d map[string]any) map[string]any { return flagOf(d, "invoice create", "name") }
func optFlag(d map[string]any) map[string]any { return flagOf(d, "invoice create", "description") }

// The two flag schemas of the contract (commands[].flags and
// global_flags) are the same.
func TestCommandsContractFlagSchemasEqual(t *testing.T) {
	s := loadCommandsContract(t)
	cmdFlag := s.Properties["commands"].Value.Items.Value.Properties["flags"].Value.Items.Value
	global := s.Properties["global_flags"].Value.Items.Value
	a, _ := json.Marshal(cmdFlag)
	b, _ := json.Marshal(global)
	if !bytes.Equal(a, b) {
		t.Errorf("Flag-Schemas weichen ab:\n%s\n%s", a, b)
	}
}

// Every spec operation is the operation of exactly one registry command;
// only CreateTimeEntry (time log, time stop), GetInfo (auth login,
// auth status) and ListInvoices (invoice paid-status) are also sent by
// commands without an entry of their own.
func TestCommandsJSONCoversAllOperations(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	byOp := map[string][]string{}
	for _, c := range d.Commands {
		if c.OperationID != "" {
			byOp[c.OperationID] = append(byOp[c.OperationID], c.Command)
		}
	}
	ops := spec.Operations()
	if len(ops) != 89 {
		t.Fatalf("Spec hat %d Operationen, erwartet 89", len(ops))
	}
	shared := map[string][]string{
		"CreateTimeEntry": {"time create", "time log", "time stop"},
		"GetInfo":         {"auth login", "auth status", "info"},
		"ListInvoices":    {"invoice list", "invoice paid-status"},
	}
	for _, op := range ops {
		got := byOp[op.Op.OperationID]
		want := shared[op.Op.OperationID]
		if want == nil {
			e, ok := registry.ByOperationID(op.Op.OperationID)
			if !ok {
				t.Errorf("%s: kein Registry-Eintrag", op.Op.OperationID)
				continue
			}
			want = []string{strings.TrimSpace(e.Resource + " " + e.Action)}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: Befehle %v, erwartet %v", op.Op.OperationID, got, want)
		}
		delete(byOp, op.Op.OperationID)
	}
	if len(byOp) > 0 {
		t.Errorf("Operationen außerhalb der Spec: %v", byOp)
	}
}

// Every visible command of the tree that runs something is listed exactly
// once, nothing else; the list is sorted by command.
func TestCommandsJSONListsEveryVisibleCommand(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	var got []string
	for _, c := range d.Commands {
		got = append(got, c.Command)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("nicht nach command sortiert: %v", got)
	}
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	root.InitDefaultHelpCmd() // added by cobra when it executes
	var want []string
	walk(root, func(c *cobra.Command) {
		if c == root || c.Hidden || c.HasSubCommands() {
			return
		}
		want = append(want, strings.TrimPrefix(c.CommandPath(), "papierkram "))
	})
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("Befehle\n got: %v\nwant: %v", got, want)
	}
	for _, meta := range []string{"version", "auth login", "auth logout", "auth status", "context create", "context use",
		"context list", "context active", "context delete", "context rename", "local init", "local show",
		"commands", "schema", "completion", "api", "help", "time log", "time start", "time status", "time stop", "time discard", "mcp serve"} {
		if !slices.Contains(got, meta) {
			t.Errorf("%s fehlt", meta)
		}
	}
}

// Two runs give byte-identical output.
func TestCommandsJSONDeterministic(t *testing.T) {
	a := runCommandsJSON(t, "--json")
	b := runCommandsJSON(t, "--json")
	if !bytes.Equal(a, b) {
		t.Error("zwei Läufe von commands --json unterscheiden sich")
	}
	if c := runCommandsJSON(t, "-o", "json"); !bytes.Equal(a, c) {
		t.Error("commands -o json weicht von commands --json ab")
	}
}

// Registry commands carry class, credits, operation, method and path of
// their entry and its examples; the reported class matches the pk:class
// annotation the guards use.
func TestCommandsJSONRegistryEntries(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, e := range registry.Entries() {
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		c := d.find(t, name)
		credits := e.Credits
		// deliver reads the document back, invoice update reads the
		// invoice to keep its payment term (ruling E2E-A)
		if e.Action == "deliver" || e.OperationID == "UpdateInvoice" {
			read, _ := registry.Lookup(e.Resource, "describe")
			credits += read.Credits
		}
		if c.Class != string(e.Class) || c.Credits == nil || *c.Credits != credits ||
			c.OperationID != e.OperationID || c.Method != e.Method || c.Path != e.Path {
			t.Errorf("%s: %s %v %s %s %s, erwartet %s %d %s %s %s", name, c.Class, c.Credits, c.OperationID, c.Method, c.Path,
				e.Class, e.Credits, e.OperationID, e.Method, e.Path)
		}
		if !slices.Equal(c.Examples, e.Examples) {
			t.Errorf("%s: Beispiele %q, erwartet %q", name, c.Examples, e.Examples)
		}
		if c.Short != e.Short {
			t.Errorf("%s: short %q, erwartet %q", name, c.Short, e.Short)
		}
	}
}

// Class and cost of the commands outside the registry (controller ruling,
// see the report of task 39).
func TestCommandsJSONMetaClasses(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	type want struct {
		class   string
		credits int // -1: null
		op      string
	}
	for name, w := range map[string]want{
		"api":                 {"per_request", -1, ""},
		"time log":            {"create", 10, "CreateTimeEntry"},
		"time stop":           {"create", 10, "CreateTimeEntry"},
		"time start":          {"local", 0, ""},
		"time status":         {"local", 0, ""},
		"time discard":        {"local", 0, ""},
		"local init":          {"local", 0, ""},
		"local show":          {"local", 0, ""},
		"context create":      {"local", 0, ""},
		"context list":        {"local", 0, ""},
		"version":             {"local", 0, ""},
		"commands":            {"local", 0, ""},
		"schema":              {"local", 0, ""},
		"completion":          {"local", 0, ""},
		"help":                {"local", 0, ""},
		"mcp serve":           {"local", 0, ""},
		"auth login":          {"local", 1, "GetInfo"},
		"auth logout":         {"local", 0, ""},
		"auth status":         {"read_only", 1, "GetInfo"},
		"invoice pdf":         {"read_only", 1, "GetInvoicePdf"},
		"invoice paid-status": {"read_only", 1, "ListInvoices"}, // 1 per page
		"invoice deliver":     {"send", 2, "DeliverInvoice"},    // with the readback
		"voucher pay":         {"destructive", 1, "PayVoucher"},
	} {
		c := d.find(t, name)
		credits := -1
		if c.Credits != nil {
			credits = *c.Credits
		}
		if c.Class != w.class || credits != w.credits || c.OperationID != w.op {
			t.Errorf("%s: %s/%d/%q, erwartet %s/%d/%q", name, c.Class, credits, c.OperationID, w.class, w.credits, w.op)
		}
		if (c.OperationID == "") != (c.Method == "" && c.Path == "") {
			t.Errorf("%s: operation_id %q, method %q, path %q", name, c.OperationID, c.Method, c.Path)
		}
	}
}

// A command whose pk:class annotation names a mutating class is reported
// with that class (api: per_request; time start only stores the timer:
// local), so no mutation is listed as harmless.
func TestCommandsJSONKeepsMutatingClasses(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	app, _ := apiApp(t)
	walk(NewRootCmd(app), func(c *cobra.Command) {
		class := c.Annotations[annotClass]
		if class == "" || class == string(registry.ClassReadOnly) || c.HasSubCommands() {
			return
		}
		name := strings.TrimPrefix(c.CommandPath(), "papierkram ")
		got := d.find(t, name).Class
		switch {
		case name == "api":
			class = classPerRequest
		case c.Annotations[annotLocal] == "true":
			class = classLocal
		}
		if got != class {
			t.Errorf("%s: Klasse %s, Annotation %s", name, got, c.Annotations[annotClass])
		}
	})
}

// Flags: types, required, enums, local filters, presets and environment
// variables (tasks 25, 29, 36).
func TestCommandsJSONFlags(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))

	typ := d.find(t, "company list").flag(t, "type")
	if !typ.LocalFilter || !slices.Equal(typ.Enum, []string{"customer", "supplier"}) || typ.Type != "string" {
		t.Errorf("company list --type = %+v", typ)
	}
	if f := d.find(t, "invoice list").flag(t, "company"); f.LocalFilter || f.Type != "id" || f.Required {
		t.Errorf("invoice list --company = %+v", f)
	}
	if f := d.find(t, "invoice list").flag(t, "from"); f.Type != "date" {
		t.Errorf("invoice list --from = %+v", f)
	}
	if f := d.find(t, "time list").flag(t, "since"); f.Type != "string" {
		t.Errorf("time list --since (RFC 3339 oder Datum) = %+v", f)
	}
	if f := d.find(t, "time list").flag(t, "state"); !slices.Equal(f.Enum, []string{"billed", "unbilled", "billable", "unbillable", "archived"}) {
		t.Errorf("time list --state = %+v", f)
	}

	for _, name := range []string{"task list", "task create", "time list"} {
		f := d.find(t, name).flag(t, "project")
		if !slices.Equal(f.DefaultFrom, []string{"flag", "env", "projectfile"}) || f.Env != "PAPIERKRAM_PROJECT" {
			t.Errorf("%s --project = %+v", name, f)
		}
	}
	if f := d.find(t, "task create").flag(t, "project"); !f.Required || f.Type != "id" {
		t.Errorf("task create --project = %+v", f)
	}
	if f := d.find(t, "invoice list").flag(t, "project"); f.DefaultFrom != nil || f.Env != "" {
		t.Errorf("invoice list --project hat keine Voreinstellung: %+v", f)
	}
	for _, name := range []string{"time log", "time start"} {
		c := d.find(t, name)
		// required: a value must come from some source, here flag or preset
		if f := c.flag(t, "task"); !f.Required || !slices.Equal(f.DefaultFrom, []string{"flag", "env", "projectfile"}) || f.Env != "PAPIERKRAM_TASK" {
			t.Errorf("%s --task = %+v", name, f)
		}
		if f := c.flag(t, "user"); !f.Required || !slices.Equal(f.DefaultFrom, []string{"flag", "env", "projectfile", "context"}) || f.Env != "PAPIERKRAM_USER" || f.Type != "id" {
			t.Errorf("%s --user = %+v", name, f)
		}
	}

	ic := d.find(t, "invoice create")
	if f := ic.flag(t, "name"); !f.Required || f.Type != "string" || f.Description != "Wert für name (Pflicht)" {
		t.Errorf("invoice create --name = %+v", f)
	}
	if f := ic.flag(t, "item"); !f.Required || !f.Repeatable || f.Type != "string" {
		t.Errorf("invoice create --item = %+v", f)
	}
	// required like the estimate's (measured live 2026-11-08); the
	// estimate does not fill it, so --from-estimate is no alternative
	if f := ic.flag(t, "document-date"); f.Type != "date" || !f.Required || !slices.Equal(f.Alternatives, []string{"--input"}) {
		t.Errorf("invoice create --document-date = %+v", f)
	}
	if f := ic.flag(t, "flagged"); f.Type != "boolean" {
		t.Errorf("invoice create --flagged = %+v", f)
	}
	if f := d.find(t, "invoice update").flag(t, "name"); f.Required {
		t.Errorf("invoice update --name = %+v", f)
	}
	if f := d.find(t, "company create").flag(t, "contact-type"); !f.Required || !slices.Equal(f.Enum, []string{"customer", "supplier"}) {
		t.Errorf("company create --contact-type = %+v", f)
	}
	if f := d.find(t, "person describe").flag(t, "company"); !f.Required || f.Type != "id" {
		t.Errorf("person describe --company = %+v", f)
	}
	if f := d.find(t, "invoice deliver").flag(t, "via"); !f.Required || !slices.Equal(f.Enum, []string{"email", "pdf"}) {
		t.Errorf("invoice deliver --via = %+v", f)
	}
	if f := d.find(t, "invoice deliver").flag(t, "to"); f.Required {
		t.Errorf("invoice deliver --to ist nur bei --via email Pflicht: %+v", f)
	}
	if f := d.find(t, "voucher pay").flag(t, "reason"); !slices.Equal(f.Enum, []string{"sonstige", "mahnung", "teilzahlung", "skonto", "schmaelerung"}) {
		t.Errorf("voucher pay --reason = %+v", f)
	}
	for _, name := range []string{"amount", "transaction"} {
		// either one is required: alternatives name the other and --input
		if f := d.find(t, "voucher pay").flag(t, name); !f.Required || !strings.Contains(f.Description, "(Pflicht; eines von --amount und --transaction oder --input)") {
			t.Errorf("voucher pay --%s = %+v", name, f)
		}
	}
	if f := d.find(t, "voucher pay").flag(t, "amount"); f.Type != "number" {
		t.Errorf("voucher pay --amount = %+v", f)
	}
	if f := d.find(t, "context create").flag(t, "subdomain"); !f.Required {
		t.Errorf("context create --subdomain = %+v", f)
	}
	for _, name := range []string{"project create", "project update"} {
		if f := d.find(t, name).flag(t, "team-member"); f.Type != "id" || !f.Repeatable {
			t.Errorf("%s --team-member = %+v", name, f)
		}
	}
	if f := d.find(t, "voucher create").flag(t, "entertainment-person"); f.Type != "string" || !f.Repeatable {
		t.Errorf("voucher create --entertainment-person = %+v", f)
	}
	if f := d.find(t, "api").flag(t, "raw-field"); !f.Repeatable {
		t.Errorf("api --raw-field = %+v", f)
	}
	// local init shadows the global --context with its own
	if f := d.find(t, "local init").flag(t, "context"); f.Description != "Kontext, für den die Projektdatei gilt" {
		t.Errorf("local init --context = %+v", f)
	}

	for _, c := range d.Commands {
		for _, f := range c.Flags {
			if f.Name == "help" {
				t.Errorf("%s: --help gelistet", c.Command)
			}
			if f.Enum == nil {
				t.Errorf("%s --%s: enum fehlt (leeres Array erwartet)", c.Command, f.Name)
			}
		}
	}

	// required mirrors the help: "Pflicht" there, conditional ones ("Pflicht
	// bei --via email") are not required
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			marked := strings.Contains(f.Description, "(Pflicht)") || strings.Contains(f.Description, "Pflicht;") || strings.Contains(f.Description, ", Pflicht)")
			if f.Required != marked {
				t.Errorf("%s --%s: required %v, Hilfe %q", c.Command, f.Name, f.Required, f.Description)
			}
		}
	}

	var names []string
	for _, f := range d.GlobalFlags {
		names = append(names, f.Name)
	}
	want := []string{"context", "debug", "dry-run", "fields", "jq", "no-color", "no-input", "output", "timeout", "yes"}
	if !slices.Equal(names, want) {
		t.Errorf("global_flags = %v, erwartet %v", names, want)
	}
	for _, f := range d.GlobalFlags {
		switch f.Name {
		case "output":
			if !slices.Equal(f.Enum, []string{"table", "wide", "json", "yaml"}) {
				t.Errorf("--output = %+v", f)
			}
		case "context":
			// flag > PAPIERKRAM_CONTEXT > project file > active_context (config.SelectContext)
			if f.Env != "PAPIERKRAM_CONTEXT" || !slices.Equal(f.DefaultFrom, []string{"flag", "env", "projectfile", "config"}) {
				t.Errorf("--context = %+v", f)
			}
		case "timeout":
			if f.Type != "duration" {
				t.Errorf("--timeout = %+v", f)
			}
		}
	}
	if len(d.find(t, "invoice create").Flags) == len(d.GlobalFlags) {
		t.Error("globale Flags in den Befehlen wiederholt?")
	}
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			if f.Name == "jq" || f.Name == "dry-run" {
				t.Errorf("%s: globales Flag --%s wiederholt", c.Command, f.Name)
			}
		}
	}
}

// Aliases are all other spellings of the command path.
func TestCommandsJSONAliases(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	if got := d.find(t, "invoice deliver").Aliases; !slices.Equal(got, []string{"invoice send", "rechnung deliver", "rechnung send"}) {
		t.Errorf("invoice deliver: %v", got)
	}
	if got := d.find(t, "voucher document delete").Aliases; !slices.Equal(got, []string{"beleg document delete", "expense document delete"}) {
		t.Errorf("voucher document delete: %v", got)
	}
	if got := d.find(t, "version").Aliases; got == nil || len(got) != 0 {
		t.Errorf("version: %v", got)
	}
}

// Examples of commands outside the registry come from the cobra example,
// one per line; commands and schema have their own.
func TestCommandsJSONExamples(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	if got := d.find(t, "time log").Examples; !slices.Equal(got, timeLogExamples) {
		t.Errorf("time log: %q", got)
	}
	for _, name := range []string{"commands", "schema"} {
		if got := d.find(t, name).Examples; len(got) < 2 {
			t.Errorf("%s: %q", name, got)
		}
	}
	// every command has at least two examples (help included)
	for _, c := range d.Commands {
		if len(c.Examples) < 2 {
			t.Errorf("%s: %d Beispiele", c.Command, len(c.Examples))
		}
	}
}

// Without --json commands prints a German table.
func TestCommandsTable(t *testing.T) {
	out := string(runCommandsJSON(t))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if f := strings.Fields(lines[0]); !slices.Equal(f, []string{"COMMAND", "KLASSE", "CREDITS", "KURZ"}) {
		t.Errorf("Kopfzeile %q", lines[0])
	}
	var row string
	for _, l := range lines {
		if strings.HasPrefix(l, "invoice create ") {
			row = l
		}
	}
	if f := strings.Fields(row); len(f) < 5 || f[2] != "create" || f[3] != "50" || !strings.Contains(row, "Rechnung anlegen") {
		t.Errorf("Zeile invoice create = %q", row)
	}
	// yaml and --jq go through the output package
	if y := string(runCommandsJSON(t, "-o", "yaml")); !strings.HasPrefix(y, "version: 1\n") {
		t.Errorf("yaml beginnt mit %q", y[:min(40, len(y))])
	}
	if j := strings.TrimSpace(string(runCommandsJSON(t, "--json", "--jq", `.commands[] | select(.command == "invoice create") | .credits`))); j != "50" {
		t.Errorf("--jq = %q", j)
	}
}

func TestCommandsUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"commands", "x"},
		{"commands", "--json", "-o", "yaml"},
		{"commands", "--dry-run"},
	} {
		app, _, _ := newTestApp(t)
		wantExit(t, app, Run(app, args), 2)
	}
}

// The registry's view and the command tree agree on what each registry
// command is (no test helper hides a drift): annotations and entries.
func TestCommandsJSONRegistryAnnotations(t *testing.T) {
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	for _, e := range registry.Entries() {
		c, _, err := root.Find(strings.Fields(e.Resource + " " + e.Action))
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Annotations[annotClass]; got != string(e.Class) {
			t.Errorf("%s %s: pk:class %q, Registry %q", e.Resource, e.Action, got, e.Class)
		}
		if !reflect.DeepEqual(commandExamples(c), e.Examples) {
			t.Errorf("%s %s: Beispiele weichen ab", e.Resource, e.Action)
		}
	}
}

// usage is the German use line with the positional arguments; shorthand
// is the one-letter flag, missing when the flag has none.
func TestCommandsJSONUsageAndShorthand(t *testing.T) {
	raw := runCommandsJSON(t, "--json")
	d := decodeCommands(t, raw)
	for name, want := range map[string]string{
		"invoice describe":        "papierkram invoice describe <id> [Optionen]",
		"project describe":        "papierkram project describe [<id>] [Optionen]",
		"voucher upload":          "papierkram voucher upload <beleg-id> <datei> [Optionen]",
		"voucher document delete": "papierkram voucher document delete <beleg-id> <datei-id> [Optionen]",
		"api":                     "papierkram api <METHOD> <pfad> [Optionen]",
		"info":                    "papierkram info [Optionen]",
		"schema":                  "papierkram schema <ressource> [<aktion>] [Optionen]",
	} {
		if got := d.find(t, name).Usage; got != want {
			t.Errorf("%s: usage %q, erwartet %q", name, got, want)
		}
	}
	short := func(f flgInfo) string {
		if f.Shorthand == nil {
			return ""
		}
		return *f.Shorthand
	}
	for _, tc := range []struct{ cmd, flag, want string }{
		{"time log", "comments", "m"},
		{"invoice pdf", "output-file", "O"},
		{"api", "raw-field", "f"},
		{"api", "field", "F"},
		{"api", "input", ""},
		{"invoice create", "name", ""},
	} {
		if got := short(d.find(t, tc.cmd).flag(t, tc.flag)); got != tc.want {
			t.Errorf("%s --%s: shorthand %q, erwartet %q", tc.cmd, tc.flag, got, tc.want)
		}
	}
	globals := map[string]string{}
	for _, f := range d.GlobalFlags {
		globals[f.Name] = short(f)
	}
	if globals["output"] != "o" || globals["yes"] != "y" || globals["jq"] != "" {
		t.Errorf("global shorthands = %v", globals)
	}
	// a flag without shorthand has no key at all (never null or "")
	if bytes.Contains(raw, []byte(`"shorthand": null`)) || bytes.Contains(raw, []byte(`"shorthand": ""`)) {
		t.Error("shorthand null oder leer statt weggelassen")
	}
}

// alternatives name the flags that satisfy the same requirement as a
// required flag (--items and --input for --item, --from-estimate for the
// fields invoice create takes from the estimate).
func TestCommandsJSONAlternatives(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, tc := range []struct {
		cmd, flag string
		want      []string
	}{
		{"invoice create", "item", []string{"--from-estimate", "--input", "--items"}},
		{"invoice create", "name", []string{"--from-estimate", "--input"}},
		{"invoice create", "payment-term", []string{"--input"}},
		{"estimate create", "item", []string{"--input", "--items"}},
		{"voucher create", "item", []string{"--input", "--items"}},
		{"voucher create", "provenance", []string{"--input"}},
		{"company create", "name", []string{"--input"}},
		{"task create", "project", []string{"--input"}},
		{"invoice deliver", "via", []string{"--input"}},
		{"voucher pay", "amount", []string{"--input", "--transaction"}},
		{"voucher pay", "transaction", []string{"--amount", "--input"}},
		{"person describe", "company", nil},
		{"time log", "task", nil},
		{"context create", "subdomain", nil},
	} {
		if got := d.find(t, tc.cmd).flag(t, tc.flag).Alternatives; !slices.Equal(got, tc.want) {
			t.Errorf("%s --%s: alternatives %q, erwartet %q", tc.cmd, tc.flag, got, tc.want)
		}
	}
	for _, c := range d.Commands {
		names := map[string]bool{}
		for _, f := range c.Flags {
			names["--"+f.Name] = true
		}
		for _, f := range c.Flags {
			if len(f.Alternatives) > 0 && !f.Required {
				t.Errorf("%s --%s: alternatives ohne required", c.Command, f.Name)
			}
			for _, a := range f.Alternatives {
				if !names[a] {
					t.Errorf("%s --%s: Alternative %s ist kein Flag des Befehls", c.Command, f.Name, a)
				}
			}
		}
	}
}

// Flags that send another request name its cost; deliver includes the
// readback in the cost of the command.
func TestCommandsJSONExtraCredits(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	if f := d.find(t, "voucher create").flag(t, "file"); f.Credits == nil || *f.Credits != 10 || !strings.Contains(f.Description, "10 weitere Credits") {
		t.Errorf("voucher create --file = %+v", f)
	}
	if f := d.find(t, "invoice create").flag(t, "from-estimate"); f.Credits == nil || *f.Credits != 1 {
		t.Errorf("invoice create --from-estimate = %+v", f)
	}
	for _, name := range []string{"invoice deliver", "estimate deliver"} {
		if c := d.find(t, name); c.Credits == nil || *c.Credits != 2 {
			t.Errorf("%s: credits %v, erwartet 2 (Versand + Nachlesen)", name, c.Credits)
		}
	}
	n := 0
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			if f.Credits != nil {
				n++
			}
		}
	}
	// invoice paid-status --details: 1 per invoice read (task 4)
	if f := d.find(t, "invoice paid-status").flag(t, "details"); f.Credits == nil || *f.Credits != 1 {
		t.Errorf("invoice paid-status --details = %+v", f)
	}
	if n != 3 {
		t.Errorf("%d Flags mit credits, erwartet 3", n)
	}
}

// default is the pflag default unless it is the zero value of its type
// (like "(Standard: …)" in the help); minimum and maximum are the bounds
// the list and api checks use (spec 7.2).
func TestCommandsJSONDefaultsAndBounds(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	num := func(v float64) *float64 { return &v }
	str := func(v string) *string { return &v }
	type want struct {
		def      *string
		min, max *float64
	}
	check := func(where string, f flgInfo, w want) {
		t.Helper()
		if !reflect.DeepEqual(f.Default, w.def) || !reflect.DeepEqual(f.Minimum, w.min) || !reflect.DeepEqual(f.Maximum, w.max) {
			t.Errorf("%s: default %v minimum %v maximum %v, erwartet %v %v %v", where,
				show(f.Default), show(f.Minimum), show(f.Maximum), show(w.def), show(w.min), show(w.max))
		}
	}
	il := d.find(t, "invoice list")
	check("invoice list --limit", il.flag(t, "limit"), want{str("25"), num(1), num(100)})
	check("invoice list --page", il.flag(t, "page"), want{nil, num(1), nil})
	check("invoice list --max-pages", il.flag(t, "max-pages"), want{nil, num(1), nil})
	check("invoice list --all (Schalter)", il.flag(t, "all"), want{})
	check("invoice list --sort (leerer Text)", il.flag(t, "sort"), want{})
	check("invoice list --company (ID 0)", il.flag(t, "company"), want{})
	check("company list --limit", d.find(t, "company list").flag(t, "limit"), want{str("25"), num(1), num(100)})
	check("api --max-pages", d.find(t, "api").flag(t, "max-pages"), want{nil, num(1), nil})
	check("api --paginate (Schalter)", d.find(t, "api").flag(t, "paginate"), want{})
	check("invoice create --flagged (Schalter)", d.find(t, "invoice create").flag(t, "flagged"), want{})
	check("invoice create --item (Liste)", d.find(t, "invoice create").flag(t, "item"), want{})
	globals := map[string]want{"timeout": {def: str("30s")}, "output": {def: str("table")}}
	for _, f := range d.GlobalFlags {
		check("--"+f.Name, f, globals[f.Name])
	}

	// default exactly where and as the help names it: "(Standard: …)"
	// after the flag text, bounds only where a check uses them
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	help := map[string]map[string]string{} // command, flag: help text
	walk(root, func(c *cobra.Command) {
		rows := map[string]string{}
		for _, r := range germanFlagRows(c.LocalNonPersistentFlags()) {
			name, _, _ := strings.Cut(r.flag[strings.Index(r.flag, "--")+2:], " ")
			rows[name] = r.usage
		}
		help[strings.TrimPrefix(c.CommandPath(), "papierkram ")] = rows
	})
	globalHelp := map[string]string{}
	for _, r := range germanFlagRows(root.PersistentFlags()) {
		name, _, _ := strings.Cut(r.flag[strings.Index(r.flag, "--")+2:], " ")
		globalHelp[name] = r.usage
	}
	helpOf := func(where string, f flgInfo, got string) {
		t.Helper()
		suffix := ""
		switch {
		case f.Default != nil && f.Type == "string":
			suffix = fmt.Sprintf(" (Standard: %q)", *f.Default)
		case f.Default != nil:
			suffix = " (Standard: " + *f.Default + ")"
		}
		if got != f.Description+suffix {
			t.Errorf("%s: default %v, Hilfe %q", where, show(f.Default), got)
		}
	}
	for _, f := range d.GlobalFlags {
		helpOf("--"+f.Name, f, globalHelp[f.Name])
	}
	bounded := []string{"limit", "page", "max-pages"}
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			helpOf(c.Command+" --"+f.Name, f, help[c.Command][f.Name])
			if (f.Minimum != nil || f.Maximum != nil) && !slices.Contains(bounded, f.Name) {
				t.Errorf("%s --%s: unerwartete Grenzen %v–%v", c.Command, f.Name, show(f.Minimum), show(f.Maximum))
			}
		}
	}
}

// show prints a pointer value for test messages ("–" for nil).
func show[T any](p *T) string {
	if p == nil {
		return "–"
	}
	return fmt.Sprint(*p)
}

// Bounds of a spec schema become minimum and maximum of the flag; a bound
// made exclusive (OpenAPI 3.0: exclusiveMinimum true) is left out. The
// embedded spec names none today (spec-driven, prepared).
func TestMarkSchemaBounds(t *testing.T) {
	lo, hi := 0.0, 99.5
	yes, no := true, false
	cases := map[string]struct {
		s        *openapi3.Schema
		min, max *float64
	}{
		"ohne Schema":    {nil, nil, nil},
		"ohne Grenzen":   {&openapi3.Schema{}, nil, nil},
		"beide":          {&openapi3.Schema{Min: &lo, Max: &hi}, &lo, &hi},
		"exklusiv unten": {&openapi3.Schema{Min: &lo, Max: &hi, ExclusiveMin: openapi3.ExclusiveBound{Bool: &yes}}, nil, &hi},
		"exklusiv oben":  {&openapi3.Schema{Min: &lo, Max: &hi, ExclusiveMax: openapi3.ExclusiveBound{Bool: &yes}}, &lo, nil},
		"exklusiv false": {&openapi3.Schema{Min: &lo, ExclusiveMin: openapi3.ExclusiveBound{Bool: &no}}, &lo, nil},
	}
	for name, tc := range cases {
		fs := pflag.NewFlagSet("x", pflag.ContinueOnError)
		fs.Var(&rawValue{typ: "number"}, "amount", "Betrag")
		markSchemaBounds(fs, "amount", tc.s)
		infos, err := flagInfos(fs)
		if err != nil {
			t.Fatal(err)
		}
		if f := infos[0]; !reflect.DeepEqual(f.Minimum, tc.min) || !reflect.DeepEqual(f.Maximum, tc.max) {
			t.Errorf("%s: minimum %v maximum %v, erwartet %v %v", name, show(f.Minimum), show(f.Maximum), show(tc.min), show(tc.max))
		}
	}
}

// Review T16-M3: only a filter that takes a number shows the bounds of
// its schema: an id filter, or one of type integer or number; a date,
// datetime or text filter never does.
func TestFilterBounds(t *testing.T) {
	lo := 1.0
	str := &openapi3.Schema{Type: &openapi3.Types{"string"}, Min: &lo}
	num := &openapi3.Schema{Type: &openapi3.Types{"number"}, Min: &lo}
	integer := &openapi3.Schema{Type: &openapi3.Types{"integer"}, Min: &lo}
	for i, tc := range []struct {
		kind string
		s    *openapi3.Schema
		want *openapi3.Schema
	}{
		{"id", integer, integer},
		{"id", str, str},
		{"id", nil, nil},
		{"enum", integer, integer},
		{localKind, num, num},
		{"date", str, nil},
		{"datetime_from", str, nil},
		{"enum", str, nil},
		{localKind, str, nil},
		{"date", &openapi3.Schema{Min: &lo}, nil},
	} {
		if got := filterBounds(registry.Filter{Kind: tc.kind}, tc.s); got != tc.want {
			t.Errorf("Fall %d (%s): %p, erwartet %p", i, tc.kind, got, tc.want)
		}
	}
}

// The --json sentence of commands --help lists the flag details once,
// without a double "und" (task 17, T16-M4).
func TestCommandsLongWording(t *testing.T) {
	want := "Flags mit Kurzform, Typ, Pflicht (mit gleichwertigen Flags unter alternatives), Wiederholbarkeit, erlaubten Werten, " +
		"Standardwert wie „Standard:“ in der Hilfe (Feld default), geprüften Grenzen (Felder minimum, maximum), " +
		"Namensauflösung (Feld resolves: das ID-Flag nimmt auch einen Namen dieser Ressource) und Hilfetext, dazu Beispiele;"
	if !strings.Contains(commandsLong, want) {
		t.Errorf("commandsLong ohne %q:\n%s", want, commandsLong)
	}
}

// Review T16-M1: every bound commands --json names is the one the CLI
// checks. For each flag with minimum or maximum, min-1 and max+1 are exit
// 2 naming the flag before any request; min and max are accepted (exit
// 0 against the mock). Required id flags get 1; --max-pages needs --all
// (papierkram api: --paginate and a list path; invoice paid-status
// nothing).
func TestFlagBoundsEnforced(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	checked := 0
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			if f.Minimum == nil && f.Maximum == nil {
				continue
			}
			if f.Type != "integer" {
				t.Errorf("%s --%s: Grenzen bei Typ %q (Test kennt nur integer)", c.Command, f.Name, f.Type)
				continue
			}
			base := strings.Fields(c.Command)
			for _, r := range c.Flags {
				if r.Required && r.Type == "id" {
					base = append(base, "--"+r.Name, "1")
				}
			}
			switch {
			case c.Command == "api":
				base = append(base, "GET", "/income/invoices", "--paginate")
			case c.Command == "invoice paid-status": // --max-pages without --all
			case f.Name == "max-pages":
				base = append(base, "--all")
			}
			type probe struct {
				value int
				ok    bool
			}
			var probes []probe
			if f.Minimum != nil {
				probes = append(probes, probe{int(*f.Minimum) - 1, false}, probe{int(*f.Minimum), true})
			}
			if f.Maximum != nil {
				probes = append(probes, probe{int(*f.Maximum), true}, probe{int(*f.Maximum) + 1, false})
			}
			for _, p := range probes {
				args := append(slices.Clone(base), "--"+f.Name, strconv.Itoa(p.value), "-o", "json")
				app, srv := apiApp(t)
				code := Run(app, args)
				_, errs := bufs(app)
				switch {
				case p.ok && (code != 0 || len(srv.Requests()) == 0):
					t.Errorf("%v: Exit %d, %d Anfragen, erwartet angenommen:\n%s", args, code, len(srv.Requests()), errs)
				case !p.ok && (code != 2 || len(srv.Requests()) != 0 ||
					!strings.Contains(errs, fmt.Sprintf("ungültiger Wert %d für --%s", p.value, f.Name)) || !strings.Contains(errs, `"code":"usage"`)):
					t.Errorf("%v: Exit %d, %d Anfragen, erwartet Exit 2 ohne Anfrage:\n%s", args, code, len(srv.Requests()), errs)
				}
				checked++
			}
		}
	}
	if checked < 100 {
		t.Errorf("nur %d Grenzwerte geprüft", checked)
	}
}
