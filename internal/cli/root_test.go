package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
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
	app.Location, app.Now = testClock(t)
	return app, out, errb
}

// testClock is the fixed clock of the test apps: Wednesday, 2026-06-10,
// 12:00 in Europe/Berlin, as host zone too. Relative times (time log
// --duration without --from, heute/gestern) then never cross midnight, a
// weekend or a month boundary, whatever the real clock or the TZ of the
// test run (fix F1: the doc examples failed between 00:00 and 01:30). A
// test that needs another time sets app.Now (and app.Location) itself.
func testClock(t *testing.T) (*time.Location, func() time.Time) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, loc)
	return loc, func() time.Time { return now }
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
	// completion is the German command of task 40, not cobra's English default
	if !strings.Contains(got, "Skript für die Shell-Vervollständigung ausgeben") {
		t.Errorf("Hilfe ohne deutschen completion-Befehl:\n%s", got)
	}
	for _, bad := range []string{"Usage:", "help for", "Available Commands:", "Flags:\n", "Global Flags:", "autocompletion", "[flags]"} {
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
	// L37: the exact line, not only its parts
	want := "papierkram " + buildinfo.Version + " (Commit " + buildinfo.Commit + ", Build " + buildinfo.Date + ", " + runtime.Version() + ")\n"
	if out.String() != want {
		t.Errorf("Ausgabe = %q, erwartet %q", out.String(), want)
	}
	if want != "papierkram dev (Commit none, Build unknown, "+runtime.Version()+")\n" {
		t.Errorf("Standardwerte von buildinfo geändert: %q", want)
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

// englishFlagTypes are the type names pflag shows after a flag name
// (pflag's UnquoteUsage: "strings" for stringSlice, "ints" for intSlice,
// …), the pflag types themselves and the types of the raw create/update
// field flags (id, integer, number, date, bool).
var englishFlagTypes = []string{
	"string", "strings", "stringArray", "stringSlice", "stringToString", "stringToInt64",
	"duration", "durationSlice", "count", "value", "bool", "bools", "boolSlice", "boolfunc", "func",
	"int", "int8", "int16", "int32", "int64", "ints", "intSlice", "int32Slice", "int64Slice",
	"uint", "uint8", "uint16", "uint32", "uint64", "uints", "uintSlice",
	"float", "float32", "float64", "float32Slice", "float64Slice", "floats",
	"ip", "ipSlice", "ipMask", "ipNet", "ipNetSlice", "bytesHex", "bytesBase64", "time",
	"id", "integer", "number", "date",
}

// englishFlagTypeRe matches an English type placeholder directly after a
// flag name in an option list, e.g. "--timeout duration" or "-o, --output
// string".
var englishFlagTypeRe = regexp.MustCompile(`(?m)^\s+(?:-\w, )?--[\w-]+ (` + strings.Join(englishFlagTypes, "|") + `)\b`)

// englishFlagTypeRe knows every type a flag of the tree has (task 17,
// L123): a new flag type cannot slip into the help unnoticed.
func TestEnglishFlagTypeReCoversAllTypes(t *testing.T) {
	types := map[string]bool{}
	for _, c := range commandTree(t) {
		c.Flags().VisitAll(func(f *pflag.Flag) { types[f.Value.Type()] = true })
		c.InheritedFlags().VisitAll(func(f *pflag.Flag) { types[f.Value.Type()] = true })
	}
	if len(types) < 5 {
		t.Fatalf("nur %d Typen: %v", len(types), types)
	}
	for typ := range types {
		if !slices.Contains(englishFlagTypes, typ) {
			t.Errorf("Typ %q fehlt in englishFlagTypes", typ)
		}
		// what pflag would show for a flag of this type
		if line := "  --x " + typ + "   Hilfe"; !englishFlagTypeRe.MatchString(line) {
			t.Errorf("englishFlagTypeRe erkennt %q nicht", line)
		}
	}
}

// The value of a flag without argument shows like pflag's: [="wert"] with
// the value raw, no Go escaping (task 17, L123).
func TestNoOptDefValLikePflag(t *testing.T) {
	for _, v := range []string{"Bär", `C:\Temp`, `a"b`, "tab\tx"} {
		fs := pflag.NewFlagSet("x", pflag.ContinueOnError)
		fs.String("pfad", "", "Pfad")
		fs.Lookup("pfad").NoOptDefVal = v
		want := `[="` + v + `"]`
		if !strings.Contains(fs.FlagUsages(), want) {
			t.Fatalf("pflag zeigt %q nicht: %q", want, fs.FlagUsages())
		}
		if got := germanFlagUsages(fs); !strings.Contains(got, "--pfad Text"+want+" ") {
			t.Errorf("%q: %q, erwartet --pfad Text%s", v, got, want)
		}
	}
}

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
	// Without type names and defaults the layout matches pflag exactly,
	// except that pflag (since 1.1.0) writes bool flags as
	// --yes[=true|false]; the help keeps the short form.
	plain := pflag.NewFlagSet("y", pflag.ContinueOnError)
	plain.BoolP("yes", "y", false, "bejahen")
	plain.Bool("no-color", false, "Farben aus\nzweite Zeile")
	want := plain.FlagUsages()
	if strings.Contains(want, "[=true|false]") {
		// every row here is a bool, so the column shrinks by the suffix;
		// continuation lines are indented to the column
		want = strings.ReplaceAll(want, "[=true|false]", "")
		want = strings.ReplaceAll(want, "\n"+strings.Repeat(" ", len("[=true|false]")), "\n")
	}
	if g, w := germanFlagUsages(plain), want; g != w {
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
	if errb.String() != "Fehler: ungültige Flag-Syntax: ---x\nHinweis: Verwendung: papierkram [Optionen]\n" {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, errb = newTestApp(t)
	Run(app, []string{"-x"})
	if errb.String() != "Fehler: unbekanntes Kurz-Flag: 'x' in -x\nHinweis: Verwendung: papierkram [Optionen]\n" {
		t.Errorf("stderr = %q", errb.String())
	}
	app, _, errb = newTestApp(t)
	Run(app, []string{"-y=maybe"})
	if !strings.Contains(errb.String(), `ungültiger Wert "maybe" für -y, --yes: Wahrheitswert erwartet (true oder false)`) {
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
	if errb.String() != "Fehler: unbekanntes Flag: --nope\nHinweis: Verwendung: papierkram [Optionen]\n" {
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

func TestGermanCauseReparse(t *testing.T) {
	for _, tc := range []struct{ cause, value, typ, want string }{
		{"must be an integer", `"abc"`, "int", "ganze Zahl erwartet"},
		{"must be an integer", `"99999999999999999999"`, "int", "Wert außerhalb des Bereichs"},
		{"must be a non-negative integer", `"-1"`, "uint", "ganze Zahl erwartet"},
		{"must be a number", `"x"`, "float64", "Zahl erwartet (z. B. 12.5)"},
		{"must be true or false", `"maybe"`, "bool", "Wahrheitswert erwartet (true oder false)"},
		{`must be a duration like "30s" or "5m"`, `"1x"`, "duration", `ungültige Dauer "1x": unbekannte Einheit "x" (erlaubt: ms, s, m, h)`},
		{`must be a duration like "30s" or "5m"`, `"30"`, "duration", `ungültige Dauer "30": Einheit fehlt (z. B. 30s)`},
		// fallback: no scalar type to reparse
		{"must be an integer", `"abc"`, "", "ganze Zahl erwartet"},
		{"must be a non-negative integer", `"x"`, "", "ganze Zahl ab 0 erwartet"},
		{`must be a duration like "30s" or "5m"`, `"x"`, "", "Dauer erwartet (z. B. 30s oder 5m)"},
	} {
		if got := germanCause(tc.cause, tc.value, tc.typ); got != tc.want {
			t.Errorf("germanCause(%q, %s, %q) = %q, erwartet %q", tc.cause, tc.value, tc.typ, got, tc.want)
		}
	}
}
