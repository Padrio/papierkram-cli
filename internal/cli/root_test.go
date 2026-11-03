package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/buildinfo"
	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func newTestApp(t *testing.T) (*App, *bytes.Buffer, *bytes.Buffer) {
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	app := NewApp()
	app.In = strings.NewReader("")
	app.Out = out
	app.Err = errb
	app.Env = func(string) string { return "" }
	app.ConfigDir = t.TempDir()
	app.CacheDir = t.TempDir()
	app.Keyring = auth.NewMemory()
	cwd := t.TempDir()
	app.Getwd = func() (string, error) { return cwd, nil }
	app.ReadPassword = func() ([]byte, error) { return nil, errors.New("kein Terminal im Test") }
	app.Sleep = func(time.Duration) {}
	app.StdinTTY = false
	app.StdoutTTY = false
	return app, out, errb
}

func TestRootHelpGerman(t *testing.T) {
	app, out, errb := newTestApp(t)
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
	app, out, errb := newTestApp(t)
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
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"papierkram", buildinfo.Version, buildinfo.Commit, buildinfo.Date} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Ausgabe enthält %q nicht: %s", want, out.String())
		}
	}
}

func TestUnknownFlagGerman(t *testing.T) {
	app, _, errb := newTestApp(t)
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
	app, _, _ := newTestApp(t)
	if code := Run(app, []string{"--context", "x", "-y", "--no-input", "--no-color", "--debug", "--fields", "id", "--jq", ".", "--timeout", "5s", "-o", "json", "version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	f := app.Flags
	if f.Context != "x" || !f.Yes || !f.NoInput || !f.NoColor || !f.Debug || f.Fields != "id" || f.JQ != "." || f.Timeout.Seconds() != 5 || f.Output != "json" {
		t.Errorf("Flags = %+v", f)
	}

	// --dry-run is parsed, but version only reads: exit 2 (Task 24)
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"--dry-run", "version"}); code != 2 || !app.Flags.DryRun {
		t.Fatalf("exit = %d, DryRun = %t", code, app.Flags.DryRun)
	}
	if !strings.Contains(errb.String(), "--dry-run gilt nur für ändernde Befehle") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestUnknownCommandGerman(t *testing.T) {
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"nope"}); code != 2 {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "unbekannter Befehl") || strings.Contains(errb.String(), " for ") {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, _ = newTestApp(t)
	if code := Run(app, []string{"help", "nope"}); code != 2 {
		t.Errorf("help nope: exit = %d", code)
	}
	app, out, errb := newTestApp(t)
	if code := Run(app, []string{"help", "context", "nope"}); code != 2 {
		t.Errorf("help context nope: exit = %d", code)
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), `unbekannter Befehl "nope" für "papierkram context"`) {
		t.Errorf("help context nope: stdout = %q, stderr = %q", out.String(), errb.String())
	}
}

// TestHelpAndUsageHintsGermanEverywhere walks the whole command tree and
// checks help output and usage-error hints for English cobra/pflag remnants.
func TestHelpAndUsageHintsGermanEverywhere(t *testing.T) {
	app, _, _ := newTestApp(t)
	root := NewRootCmd(app)
	// cobra adds the help command only in ExecuteC; add it here so the walk
	// covers it as well.
	root.InitDefaultHelpCmd()
	var paths [][]string
	walk(root, func(c *cobra.Command) {
		paths = append(paths, strings.Fields(c.CommandPath())[1:])
	})
	if len(paths) < 10 || !slices.ContainsFunc(paths, func(p []string) bool { return slices.Equal(p, []string{"help"}) }) {
		t.Fatalf("Baum unvollständig (help fehlt?): %v", paths)
	}
	bad := []string{"(default", "[flags]", "Usage:"}
	for _, p := range paths {
		runs := map[string][]string{
			"--help":        append(slices.Clone(p), "--help"),
			"help cmd":      append([]string{"help"}, p...),
			"too many args": append(slices.Clone(p), "x1", "x2", "x3", "x4"),
			"no args":       slices.Clone(p),
		}
		for kind, args := range runs {
			app, out, errb := newTestApp(t)
			code := Run(app, args)
			got := out.String() + errb.String()
			for _, b := range bad {
				if strings.Contains(got, b) {
					t.Errorf("%v (%s, exit %d): enthält %q:\n%s", args, kind, code, b, got)
				}
			}
			if m := englishFlagTypeRe.FindString(got); m != "" {
				t.Errorf("%v (%s): englischer Typname %q in der Optionsliste:\n%s", args, kind, m, got)
			}
		}
	}
}

// englishFlagTypeRe matches a pflag type placeholder directly after a flag
// name in an option list, e.g. "--timeout duration" or "-o, --output string".
var englishFlagTypeRe = regexp.MustCompile(`(?m)^\s+(?:-\w, )?--[\w-]+ (string|strings|stringArray|duration|int|ints|uint|uints|float|bool|bools|count|value|ip|ipMask|ipNet)\b`)

func TestFlagTypeNamesGerman(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"context", "create", "--help"}); code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{"--subdomain Text ", "--timeout Dauer ", "-o, --output Text ", "--user Text "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Hilfe enthält %q nicht:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), " string ") || strings.Contains(out.String(), "--timeout duration") {
		t.Errorf("englische Typnamen:\n%s", out.String())
	}
}

func TestGermanFlagUsagesAllTypes(t *testing.T) {
	fs := pflag.NewFlagSet("x", pflag.ContinueOnError)
	fs.String("s", "", "Text-Flag")
	fs.Duration("d", 0, "Dauer-Flag")
	fs.Int("i", 0, "Zahl-Flag")
	fs.Int64("i64", 5, "Zahl mit Standard")
	fs.Uint("u", 0, "vorzeichenlos")
	fs.Float64("f", 0, "Dezimal")
	fs.StringSlice("ss", nil, "Liste")
	fs.StringArray("sa", nil, "Feld")
	fs.IntSlice("is", nil, "Zahlenliste")
	fs.Bool("b", false, "Schalter")
	fs.Count("c", "Zähler")
	fs.String("named", "", "eigener `Name` bleibt")
	got := germanFlagUsages(fs)
	for _, want := range []string{
		"--s Text ", "--d Dauer ", "--i Zahl ", "--i64 Zahl ", "Zahl mit Standard (Standard: 5)", "--u Zahl ",
		"--f Dezimalzahl ", "--ss Texte ", "--sa Texte ", "--is Zahlen ", "--c Anzahl ", "--named Name ", "eigener Name bleibt",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fehlt %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--b ") && !strings.Contains(got, "--b  ") {
		t.Errorf("bool-Flag darf keinen Typnamen haben:\n%s", got)
	}
	if m := englishFlagTypeRe.FindString(got); m != "" {
		t.Errorf("englischer Typname %q:\n%s", m, got)
	}
	// Without type names and defaults the layout matches pflag exactly.
	plain := pflag.NewFlagSet("y", pflag.ContinueOnError)
	plain.BoolP("yes", "y", false, "bejahen")
	plain.Bool("no-color", false, "Farben aus\nzweite Zeile")
	if g, w := germanFlagUsages(plain), plain.FlagUsages(); g != w {
		t.Errorf("Layout weicht von pflag ab:\n%q\n%q", g, w)
	}
}

func TestFlagDefaultsRenderedGerman(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"--help"}); code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{`(Standard: "table")`, "(Standard: 30s)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Hilfe enthält %q nicht:\n%s", want, out.String())
		}
	}
}

func TestUsageHintUsesGermanUseLine(t *testing.T) {
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"context", "rename", "a"}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "Hinweis: Verwendung: papierkram context rename <alt> <neu> [Optionen]") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestSubcommandHelpGerman(t *testing.T) {
	app, out, _ := newTestApp(t)
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

func TestFlagErrorsFullyGerman(t *testing.T) {
	cases := [][]string{
		{"--timeout", "abc"},
		{"-y=maybe"},
		{"--timeout"},
		{"-x"},
		{"--timeout", "1x"},
		{"--timeout", "30"},
		{"---x"},
	}
	for _, args := range cases {
		app, _, errb := newTestApp(t)
		if code := Run(app, args); code != 2 {
			t.Errorf("%v: exit = %d, erwartet 2", args, code)
		}
		for _, bad := range []string{" for ", " flag:", "invalid", "unknown", "needs an argument", "shorthand", "bad flag syntax", "unit", "duration", "missing", "time:"} {
			if strings.Contains(errb.String(), bad) {
				t.Errorf("%v: englisches Fragment %q in %q", args, bad, errb.String())
			}
		}
	}
	app, _, errb := newTestApp(t)
	Run(app, []string{"--timeout", "abc"})
	if !strings.Contains(errb.String(), `ungültiger Wert "abc" für --timeout: `) {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, errb = newTestApp(t)
	Run(app, []string{"--timeout", "1x"})
	if !strings.Contains(errb.String(), `ungültiger Wert "1x" für --timeout: ungültige Dauer "1x": unbekannte Einheit "x"`) {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, errb = newTestApp(t)
	Run(app, []string{"---x"})
	if errb.String() != "Fehler: ungültige Flag-Syntax: ---x\n" {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, errb = newTestApp(t)
	Run(app, []string{"-y=maybe"})
	if !strings.Contains(errb.String(), `ungültiger Wert "maybe" für -y, --yes: ungültige Syntax`) {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestHelpCommandHelpGerman(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"help", "--help"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "help for") || !strings.Contains(out.String(), "Hilfe zu help") {
		t.Errorf("Hilfe:\n%s", out.String())
	}
}

func TestExtraArgsAreUsageErrors(t *testing.T) {
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"version", "extra"}); code != 2 {
		t.Errorf("exit = %d", code)
	}
	if strings.Contains(errb.String(), "accepts") {
		t.Errorf("englisch: %q", errb.String())
	}
}

func TestRunMapsTypedErrors(t *testing.T) {
	app, _, errb := newTestApp(t)
	root := NewRootCmd(app)
	root.AddCommand(&cobra.Command{
		Use: "boom",
		RunE: func(*cobra.Command, []string) error {
			e := clierr.New(clierr.CodeNotFound, "Beleg nicht gefunden", "ID prüfen")
			e.CorrelationID = "abc-123"
			return fmt.Errorf("ctx: %w", e)
		},
	})
	if code := execute(app, root, []string{"boom"}); code != 4 {
		t.Errorf("exit = %d, erwartet 4", code)
	}
	want := "Fehler: Beleg nicht gefunden\nHinweis: ID prüfen\nCorrelation-ID: abc-123\n"
	if errb.String() != want {
		t.Errorf("stderr = %q, erwartet %q", errb.String(), want)
	}

	app, _, errb = newTestApp(t)
	root = NewRootCmd(app)
	root.AddCommand(&cobra.Command{Use: "plain", RunE: func(*cobra.Command, []string) error { return errors.New("kaputt") }})
	if code := execute(app, root, []string{"plain"}); code != 1 {
		t.Errorf("exit = %d, erwartet 1", code)
	}
	if errb.String() != "Fehler: kaputt\n" {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestUnknownFlagOutput(t *testing.T) {
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"--nope"}); code != 2 {
		t.Errorf("exit = %d", code)
	}
	if errb.String() != "Fehler: unbekanntes Flag: --nope\n" {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestGlobalFlagValidation(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"timeout zero":     {[]string{"--timeout", "0s", "version"}, `ungültiger Wert "0s" für --timeout: positive Dauer erwartet (z. B. 30s)`},
		"timeout negative": {[]string{"--timeout=-5s", "version"}, `ungültiger Wert "-5s" für --timeout: positive Dauer erwartet (z. B. 30s)`},
		"output xml":       {[]string{"-o", "xml", "version"}, `ungültiger Wert "xml" für -o, --output: erlaubt sind table, wide, json, yaml`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app, out, errb := newTestApp(t)
			if code := Run(app, tc.args); code != 2 {
				t.Fatalf("exit = %d, want 2; stderr = %s", code, errb.String())
			}
			if !strings.Contains(errb.String(), tc.want) || out.Len() != 0 {
				t.Errorf("stdout = %q, stderr = %q", out.String(), errb.String())
			}
		})
	}
	// Flag errors come before the read-only guard (exit 2 before 9).
	app, _, _ := newTestApp(t)
	app.Env = roEnv("1")
	if code := Run(app, []string{"-o", "xml", "context", "create", "c", "--subdomain", "x"}); code != 2 {
		t.Errorf("read-only + -o xml: exit = %d, want 2", code)
	}
}

func TestGlobalFlagsRecordWhetherSet(t *testing.T) {
	app, _, _ := newTestApp(t)
	if code := Run(app, []string{"version"}); code != 0 {
		t.Fatal(code)
	}
	if app.Flags.OutputSet || app.Flags.TimeoutSet {
		t.Errorf("defaults must not count as set: %+v", app.Flags)
	}
	app, _, _ = newTestApp(t)
	if code := Run(app, []string{"version", "-o", "table", "--timeout", "30s"}); code != 0 {
		t.Fatal(code)
	}
	if !app.Flags.OutputSet || !app.Flags.TimeoutSet {
		t.Errorf("explicit flags must count as set: %+v", app.Flags)
	}
}
