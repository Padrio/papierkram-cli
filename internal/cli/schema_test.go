package cli

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// runSchema runs papierkram schema with args and returns stdout (exit 0
// required).
func runSchema(t *testing.T, args ...string) []byte {
	t.Helper()
	app, _ := apiApp(t)
	wantExit(t, app, Run(app, append([]string{"schema"}, args...)), 0)
	assertNoToken(t, app)
	out, errs := bufs(app)
	if errs != "" {
		t.Errorf("stderr = %q", errs)
	}
	return []byte(out)
}

func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	return m
}

// dig follows keys through nested objects.
func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// unresolved reports a $ref or allOf anywhere in v.
func unresolved(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"$ref", "allOf"} {
			if _, ok := x[k]; ok {
				return k
			}
		}
		for _, e := range x {
			if s := unresolved(e); s != "" {
				return s
			}
		}
	case []any:
		for _, e := range x {
			if s := unresolved(e); s != "" {
				return s
			}
		}
	}
	return ""
}

func TestSchemaInvoiceCreate(t *testing.T) {
	m := decodeJSON(t, runSchema(t, "invoice", "create"))
	if len(m) != 2 || m["request"] == nil || m["response"] == nil {
		t.Fatalf("Form {request, response} erwartet: %v", keysOf(m))
	}
	if dig(m, "request", "properties", "line_items", "items", "properties", "name") == nil {
		t.Error("request ohne line_items[].name")
	}
	// the response is InvoiceWithDetailedEmbeds: allOf merged, $ref resolved
	if dig(m, "response", "properties", "invoice_no") == nil || dig(m, "response", "properties", "line_items") == nil {
		t.Errorf("response ohne invoice_no/line_items: %v", keysOf(dig(m, "response", "properties")))
	}
	if s := unresolved(m); s != "" {
		t.Errorf("Schema enthält noch %s", s)
	}
}

func keysOf(v any) []string {
	m, _ := v.(map[string]any)
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// The request schema is the one of the patched spec, with x-cli-*
// extensions (task create: complete as string boolean).
func TestSchemaKeepsCLIExtensions(t *testing.T) {
	out := runSchema(t, "task", "create")
	if !bytes.Contains(out, []byte(`"x-cli-`)) {
		t.Errorf("keine x-cli-Erweiterung im Schema von task create:\n%s", out)
	}
}

func TestSchemaPDFIsBinary(t *testing.T) {
	m := decodeJSON(t, runSchema(t, "voucher", "pdf"))
	if m["request"] != nil {
		t.Errorf("request = %v", m["request"])
	}
	if r, _ := m["response"].(map[string]any); r["type"] != "string" || r["format"] != "binary" {
		t.Errorf("response = %v", m["response"])
	}
}

// Custom actions show the schemas of their operation; aliases of resource
// and action work.
func TestSchemaCustomActionsAndAliases(t *testing.T) {
	deliver := runSchema(t, "invoice", "deliver")
	if dig(decodeJSON(t, deliver), "request", "properties", "send_via") == nil {
		t.Errorf("deliver ohne send_via:\n%s", deliver)
	}
	if alias := runSchema(t, "rechnung", "send"); !bytes.Equal(alias, deliver) {
		t.Error("schema rechnung send weicht von schema invoice deliver ab")
	}
	if dig(decodeJSON(t, runSchema(t, "voucher", "pay")), "request", "properties", "value") == nil {
		t.Error("voucher pay ohne value")
	}
	up := decodeJSON(t, runSchema(t, "beleg", "upload"))
	if dig(up, "request", "properties", "file", "format") != "binary" {
		t.Errorf("upload request = %v", up["request"])
	}
	if del := decodeJSON(t, runSchema(t, "voucher", "document", "delete")); del["request"] != nil || del["response"] != nil {
		t.Errorf("document delete = %v", del)
	}
	// time log and time stop send CreateTimeEntry
	create := runSchema(t, "time", "create")
	for _, a := range []string{"log", "stop"} {
		if got := runSchema(t, "zeit", a); !bytes.Equal(got, create) {
			t.Errorf("schema time %s weicht von time create ab", a)
		}
	}
	// GET /info has no response schema in the spec (only examples)
	if info := decodeJSON(t, runSchema(t, "info")); len(info) != 2 || info["request"] != nil || info["response"] != nil {
		t.Errorf("info = %v", info)
	}
}

// Without action: every action of the resource that sends an operation.
func TestSchemaResourceActions(t *testing.T) {
	m := decodeJSON(t, runSchema(t, "voucher"))
	if m["resource"] != "voucher" {
		t.Errorf("resource = %v", m["resource"])
	}
	actions := keysOf(m["actions"])
	want := []string{"archive", "cancel", "create", "delete", "describe", "document delete", "list", "pay", "pdf", "reverse", "unarchive", "update", "upload"}
	if !slices.Equal(actions, want) {
		t.Errorf("voucher: %v, erwartet %v", actions, want)
	}
	if dig(m, "actions", "create", "request", "properties", "line_items") == nil {
		t.Error("voucher create ohne line_items")
	}
	timeActions := keysOf(decodeJSON(t, runSchema(t, "time"))["actions"])
	for _, a := range []string{"log", "stop", "create"} {
		if !slices.Contains(timeActions, a) {
			t.Errorf("time: %s fehlt in %v", a, timeActions)
		}
	}
	for _, a := range []string{"start", "status", "discard"} {
		if slices.Contains(timeActions, a) {
			t.Errorf("time: lokales %s gelistet", a)
		}
	}
}

// Every resource resolves completely: no $ref, no allOf, no error.
func TestSchemaAllResourcesResolve(t *testing.T) {
	for _, r := range registry.Resources() {
		m := decodeJSON(t, runSchema(t, r.Name))
		if s := unresolved(m); s != "" {
			t.Errorf("%s: Schema enthält noch %s", r.Name, s)
		}
	}
}

func TestSchemaOutputFormats(t *testing.T) {
	a := runSchema(t, "invoice", "create")
	if b := runSchema(t, "invoice", "create"); !bytes.Equal(a, b) {
		t.Error("zwei Läufe unterscheiden sich")
	}
	if b := runSchema(t, "invoice", "create", "-o", "json"); !bytes.Equal(a, b) {
		t.Error("-o json weicht vom Standard ab")
	}
	if y := string(runSchema(t, "invoice", "create", "-o", "yaml")); !strings.HasPrefix(y, "request:\n") {
		t.Errorf("yaml beginnt mit %q", y[:min(40, len(y))])
	}
	if j := strings.TrimSpace(string(runSchema(t, "invoice", "create", "--jq", ".request.properties.line_items.type"))); j != "array" {
		t.Errorf("--jq = %q", j)
	}
	if f := keysOf(decodeJSON(t, runSchema(t, "invoice", "create", "--fields", "request"))); !slices.Equal(f, []string{"request"}) {
		t.Errorf("--fields request = %v", f)
	}
}

// Unknown names, local commands and api: exit 2 with a hint.
func TestSchemaUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rechnug", "create"}, `meintest du "rechnung"?`},
		{[]string{"invoice", "creat"}, `meintest du "create"?`},
		{[]string{"invoice", "create", "x"}, `unbekannte Aktion "create x"`},
		{[]string{"time", "start"}, "sendet keine Anfrage"},
		{[]string{"time", "discard"}, "sendet keine Anfrage"},
		{[]string{"api"}, "papierkram schema invoice create"},
		{[]string{"version"}, "unbekannte Ressource"},
		{[]string{"info", "x"}, `unbekannte Aktion "x"`},
		{[]string{"voucher", "document"}, `unbekannte Aktion "document"`},
		{nil, "zu wenige Argumente"},
	} {
		app, _, _ := newTestApp(t)
		wantExit(t, app, Run(app, append([]string{"schema"}, tc.args...)), 2)
		if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
			t.Errorf("schema %v: stderr %q enthält %q nicht", tc.args, errs, tc.want)
		}
	}
	app, _, _ := newTestApp(t)
	wantExit(t, app, Run(app, []string{"schema", "time", "start"}), 2)
	if _, errs := bufs(app); !strings.Contains(errs, "log") || !strings.Contains(errs, "stop") {
		t.Errorf("Hinweis nennt die Aktionen mit Schema nicht: %q", errs)
	}
}

// allOf: the schema's own keywords win, then earlier branches; properties
// and required are united. A cycle or an unknown reference is an error.
func TestResolveValue(t *testing.T) {
	in := map[string]any{
		"nullable": true,
		"allOf": []any{
			map[string]any{"type": "object", "required": []any{"a"}, "properties": map[string]any{
				"a": map[string]any{"type": "string"},
				"n": map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer"}}},
			}},
			map[string]any{"type": "array", "nullable": false, "required": []any{"b", "a"}, "properties": map[string]any{
				"b": map[string]any{"type": "integer"},
				"n": map[string]any{"properties": map[string]any{"y": map[string]any{"type": "string"}}},
			}},
		},
	}
	got, err := resolveValue(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	want := `{"nullable":true,"properties":{"a":{"type":"string"},"b":{"type":"integer"},"n":{"properties":{"x":{"type":"integer"},"y":{"type":"string"}},"type":"object"}},"required":["a","b"],"type":"object"}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	for _, ref := range []string{"#/components/schemas/Nope", "other.json#/X"} {
		if _, err := resolveValue(map[string]any{"$ref": ref}, nil); err == nil {
			t.Errorf("%s: kein Fehler", ref)
		}
	}
	if _, err := resolveValue(map[string]any{"$ref": componentPrefix + "InvoiceFlat"}, []string{"InvoiceFlat"}); err == nil || !strings.Contains(err.Error(), "zyklisch") {
		t.Errorf("Zyklus: %v", err)
	}
}
