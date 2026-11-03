package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func newTestApp() (*App, *bytes.Buffer, *bytes.Buffer) {
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	app := NewApp()
	app.In = strings.NewReader("")
	app.Out = out
	app.Err = errb
	return app, out, errb
}

func TestRootHelpGerman(t *testing.T) {
	app, out, errb := newTestApp()
	code := Run(app, []string{"--help"})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{"Verwendung:", "Verfügbare Befehle:", "Optionen:", "Hilfe zu papierkram"} {
		if !strings.Contains(got, want) {
			t.Errorf("Hilfe enthält %q nicht:\n%s", want, got)
		}
	}
	for _, bad := range []string{"Usage:", "help for", "Available Commands:", "Flags:\n", "Global Flags:", "completion", "[flags]"} {
		if strings.Contains(got, bad) {
			t.Errorf("Hilfe enthält unerwartet %q:\n%s", bad, got)
		}
	}
}

func TestVersionJSON(t *testing.T) {
	app, out, errb := newTestApp()
	if code := Run(app, []string{"version", "-o", "json"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("kein JSON: %v\n%s", err, out.String())
	}
	for _, k := range []string{"version", "commit", "date", "go_version"} {
		if _, ok := m[k]; !ok {
			t.Errorf("Schlüssel %q fehlt: %v", k, m)
		}
	}
}

func TestVersionText(t *testing.T) {
	app, out, _ := newTestApp()
	if code := Run(app, []string{"version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "papierkram") {
		t.Errorf("unerwartete Ausgabe: %s", out.String())
	}
}

func TestUnknownFlagGerman(t *testing.T) {
	app, _, errb := newTestApp()
	code := Run(app, []string{"--nope"})
	if code != 2 {
		t.Errorf("exit = %d, erwartet 2", code)
	}
	if !strings.Contains(errb.String(), "unbekanntes Flag") {
		t.Errorf("stderr = %q", errb.String())
	}
	if strings.Contains(errb.String(), "unknown flag") {
		t.Errorf("englische Meldung: %q", errb.String())
	}
}

func TestGlobalFlagsParsed(t *testing.T) {
	app, _, _ := newTestApp()
	if code := Run(app, []string{"--context", "x", "-y", "--no-input", "--dry-run", "--no-color", "--debug", "--fields", "id", "--jq", ".", "--timeout", "5s", "-o", "json", "version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	f := app.Flags
	if f.Context != "x" || !f.Yes || !f.NoInput || !f.DryRun || !f.NoColor || !f.Debug || f.Fields != "id" || f.JQ != "." || f.Timeout.Seconds() != 5 || f.Output != "json" {
		t.Errorf("Flags = %+v", f)
	}
}

func TestUnknownCommandGerman(t *testing.T) {
	app, _, errb := newTestApp()
	if code := Run(app, []string{"nope"}); code != 2 {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "unbekannter Befehl") || strings.Contains(errb.String(), " for ") {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, _ = newTestApp()
	if code := Run(app, []string{"help", "nope"}); code != 2 {
		t.Errorf("help nope: exit = %d", code)
	}
}

func TestSubcommandHelpGerman(t *testing.T) {
	app, out, _ := newTestApp()
	if code := Run(app, []string{"version", "--help"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"Verwendung:", "Globale Optionen:", "Hilfe zu version", "[Optionen]"} {
		if !strings.Contains(got, want) {
			t.Errorf("fehlt %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Global Flags") || strings.Contains(got, "help for") {
		t.Errorf("englischer Text:\n%s", got)
	}
}
