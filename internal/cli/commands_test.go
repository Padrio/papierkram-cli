package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
)

// commandsContract is the checked-in contract of commands --json (spec 11).
const commandsContract = "../../docs/schemas/commands.json"

// loadCommandsContract loads the contract as an OpenAPI 3 Schema Object
// and checks that it is a valid one.
func loadCommandsContract(t *testing.T) *openapi3.Schema {
	t.Helper()
	raw, err := os.ReadFile(commandsContract)
	if err != nil {
		t.Fatal(err)
	}
	var s openapi3.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("Vertrag ist kein Schema-Objekt: %v", err)
	}
	if err := s.Validate(context.Background()); err != nil {
		t.Fatalf("Vertrag ist kein gültiges OpenAPI-3-Schema: %v", err)
	}
	return &s
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
	mutate := map[string]func(doc map[string]any){
		"unbekanntes Feld oben":       func(d map[string]any) { d["extra"] = 1 },
		"unbekanntes Befehlsfeld":     func(d map[string]any) { firstCmd(d)["use"] = "x" },
		"usage fehlt":                 func(d map[string]any) { delete(firstCmd(d), "usage") },
		"usage ohne papierkram":       func(d map[string]any) { firstCmd(d)["usage"] = "invoice create" },
		"shorthand leer":              func(d map[string]any) { firstFlag(d)["shorthand"] = "" },
		"shorthand null":              func(d map[string]any) { firstFlag(d)["shorthand"] = nil },
		"shorthand zu lang":           func(d map[string]any) { firstFlag(d)["shorthand"] = "ab" },
		"alternatives leer":           func(d map[string]any) { firstFlag(d)["alternatives"] = []any{} },
		"alternative ohne --":         func(d map[string]any) { firstFlag(d)["alternatives"] = []any{"input"} },
		"alternative doppelt":         func(d map[string]any) { firstFlag(d)["alternatives"] = []any{"--input", "--input"} },
		"alternatives ohne required":  func(d map[string]any) { firstFlag(d)["alternatives"] = []any{"--input"} },
		"alternatives kein Array":     func(d map[string]any) { firstFlag(d)["alternatives"] = "--input" },
		"blocked fehlt":               func(d map[string]any) { delete(firstCmd(d), "blocked_by_read_only") },
		"blocked kein bool":           func(d map[string]any) { firstCmd(d)["blocked_by_read_only"] = "ja" },
		"per_request nicht blockiert": func(d map[string]any) { apiCmd(d, "api")["blocked_by_read_only"] = false },
		"Flag-Credits 0":              func(d map[string]any) { firstFlag(d)["credits"] = 0.0 },
		"Flag-Credits Text":           func(d map[string]any) { firstFlag(d)["credits"] = "10" },
		"globaler shorthand null":     func(d map[string]any) { d["global_flags"].([]any)[0].(map[string]any)["shorthand"] = nil },
		"unbekannte Klasse":           func(d map[string]any) { firstCmd(d)["class"] = "write" },
		"Operation ohne Methode":      func(d map[string]any) { delete(apiCmd(d, "invoice create"), "method") },
		"lokal mit Credits":           func(d map[string]any) { apiCmd(d, "version")["credits"] = 1.0 },
		"per_request mit Credits":     func(d map[string]any) { apiCmd(d, "api")["credits"] = 1.0 },
		"unbekanntes Flag-Feld":       func(d map[string]any) { firstFlag(d)["short"] = "x" },
		"unbekannter Flag-Typ":        func(d map[string]any) { firstFlag(d)["type"] = "int64" },
		"enum fehlt":                  func(d map[string]any) { delete(firstFlag(d), "enum") },
		"unbekannte Quelle":           func(d map[string]any) { firstFlag(d)["default_from"] = []any{"cache"} },
		"env fehlt zu default_from": func(d map[string]any) {
			for _, f := range apiCmd(d, "task list")["flags"].([]any) {
				delete(f.(map[string]any), "env")
			}
		},
		"globales Flag-Feld":       func(d map[string]any) { d["global_flags"].([]any)[0].(map[string]any)["x"] = true },
		"falsche Vertragsversion":  func(d map[string]any) { d["version"] = 2.0 },
		"Beispiel ohne papierkram": func(d map[string]any) { apiCmd(d, "invoice create")["examples"] = []any{"invoice create"} },
	}
	for name, fn := range mutate {
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		fn(doc)
		if err := schema.VisitJSON(doc); err == nil {
			t.Errorf("%s: Vertrag lässt das Dokument durch", name)
		}
	}
}

func firstCmd(d map[string]any) map[string]any { return d["commands"].([]any)[0].(map[string]any) }

func apiCmd(d map[string]any, command string) map[string]any {
	for _, c := range d["commands"].([]any) {
		if m := c.(map[string]any); m["command"] == command {
			return m
		}
	}
	panic(command)
}

func firstFlag(d map[string]any) map[string]any {
	return apiCmd(d, "invoice create")["flags"].([]any)[0].(map[string]any)
}

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
// only CreateTimeEntry (time log, time stop) and GetInfo (auth login,
// auth status) are also sent by commands without an entry of their own.
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
		"commands", "schema", "api", "help", "time log", "time start", "time status", "time stop", "time discard"} {
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
		if e.Action == "deliver" { // deliver reads the document back
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
		"api":             {"per_request", -1, ""},
		"time log":        {"create", 10, "CreateTimeEntry"},
		"time stop":       {"create", 10, "CreateTimeEntry"},
		"time start":      {"local", 0, ""},
		"time status":     {"local", 0, ""},
		"time discard":    {"local", 0, ""},
		"local init":      {"local", 0, ""},
		"local show":      {"local", 0, ""},
		"context create":  {"local", 0, ""},
		"context list":    {"local", 0, ""},
		"version":         {"local", 0, ""},
		"commands":        {"local", 0, ""},
		"schema":          {"local", 0, ""},
		"help":            {"local", 0, ""},
		"auth login":      {"local", 1, "GetInfo"},
		"auth logout":     {"local", 0, ""},
		"auth status":     {"read_only", 1, "GetInfo"},
		"invoice pdf":     {"read_only", 1, "GetInvoicePdf"},
		"invoice deliver": {"send", 2, "DeliverInvoice"}, // with the readback
		"voucher pay":     {"destructive", 1, "PayVoucher"},
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
	if f := ic.flag(t, "document-date"); f.Type != "date" || f.Required {
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
	if n != 2 {
		t.Errorf("%d Flags mit credits, erwartet 2", n)
	}
}
