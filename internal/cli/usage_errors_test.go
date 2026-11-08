package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// usageError is the structured error of the last stderr line (-o json).
type usageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func lastJSONError(t *testing.T, errs string) (usageError, bool) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(errs), "\n")
	var e struct {
		Error usageError `json:"error"`
	}
	if json.Unmarshal([]byte(lines[len(lines)-1]), &e) != nil {
		return usageError{}, false
	}
	return e.Error, true
}

// commandTree returns every command of the tree, the help command
// included.
func commandTree(t *testing.T) []*cobra.Command {
	t.Helper()
	app, _, _ := newTestApp(t)
	root := NewRootCmd(app)
	root.InitDefaultHelpCmd()
	var cmds []*cobra.Command
	walk(root, func(c *cobra.Command) { cmds = append(cmds, c) })
	if len(cmds) < 50 {
		t.Fatalf("Baum unvollständig: %d Befehle", len(cmds))
	}
	return cmds
}

// argsOf returns placeholders for the required positional arguments of
// c (one "1" per <…> in its use line).
func argsOf(c *cobra.Command) []string {
	var args []string
	for _, w := range strings.Fields(c.Use)[1:] {
		if strings.HasPrefix(w, "<") {
			args = append(args, "1")
		}
	}
	return args
}

// Every usage error names the way out (task 17, L41): flag and argument
// errors the usage line, an unknown command the help of its group.
func TestUsageErrorsHaveHint(t *testing.T) {
	for _, c := range commandTree(t) {
		path := strings.Fields(c.CommandPath())[1:]
		runs := map[string][]string{
			"unknown flag":  append(append(append([]string{}, path...), argsOf(c)...), "-o", "json", "--nope-flag"),
			"too many args": append(append([]string{}, path...), "-o", "json", "x1", "x2", "x3", "x4", "x5", "x6"),
		}
		for kind, args := range runs {
			app, _, _ := newTestApp(t)
			code := Run(app, args)
			if code != 2 {
				if kind == "unknown flag" {
					t.Errorf("%v: exit = %d, erwartet 2", args, code)
				}
				continue
			}
			_, errs := bufs(app)
			e, ok := lastJSONError(t, errs)
			if !ok {
				t.Errorf("%v: kein JSON-Fehler:\n%s", args, errs)
				continue
			}
			want := "Verwendung: " + useLine(c)
			if strings.HasPrefix(e.Message, "unbekannter Befehl") {
				want = "Befehle anzeigen: " + c.CommandPath() + " --help"
				if c.Name() == "help" {
					want = "Befehle anzeigen: papierkram --help"
				}
			}
			generic := false
			for _, p := range []string{"unbekanntes Flag", "unerwartetes Argument", "zu wenige Argumente", "unbekannter Befehl"} {
				generic = generic || strings.HasPrefix(e.Message, p)
			}
			if e.Hint == "" || (generic && e.Hint != want) {
				t.Errorf("%v (%s): Hinweis = %q, erwartet %q (Meldung %q)", args, kind, e.Hint, want, e.Message)
			}
		}
	}
}

func TestUsageHintsExact(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		msg, hint  string
		wantPrefix bool
	}{
		{[]string{"nope"}, `unbekannter Befehl "nope" für "papierkram"`, "Befehle anzeigen: papierkram --help", false},
		{[]string{"invoice", "nope"}, `unbekannter Befehl "nope" für "papierkram invoice"`, "Befehle anzeigen: papierkram invoice --help", false},
		{[]string{"help", "context", "nope"}, `unbekannter Befehl "nope" für "papierkram context"`, "Befehle anzeigen: papierkram context --help", false},
		{[]string{"version", "x"}, `unerwartetes Argument "x" für "papierkram version"`, "Verwendung: papierkram version [Optionen]", false},
		{[]string{"version", "--nope"}, "unbekanntes Flag: --nope", "Verwendung: papierkram version [Optionen]", false},
		{[]string{"invoice", "list", "--page", "abc"}, `ungültiger Wert "abc" für --page: ganze Zahl erwartet`, "Verwendung: papierkram invoice list [Optionen]", false},
		{[]string{"version", "--timeout", "0s"}, `ungültiger Wert "0s" für --timeout: positive Dauer erwartet (z. B. 30s)`, "Verwendung: papierkram version [Optionen]", false},
		{[]string{"version", "-o", "xml"}, `ungültiger Wert "xml" für -o, --output: erlaubt sind table, wide, json, yaml`, "Verwendung: papierkram version [Optionen]", false},
	} {
		app, _, _ := newTestApp(t)
		if code := Run(app, tc.args); code != 2 {
			t.Errorf("%v: exit = %d", tc.args, code)
			continue
		}
		_, errs := bufs(app)
		want := "Fehler: " + tc.msg + "\nHinweis: " + tc.hint + "\n"
		if errs != want {
			t.Errorf("%v: stderr = %q, erwartet %q", tc.args, errs, want)
		}
	}
}

// englishWords are remnants of pflag, strconv and time messages;
// "ungültige Syntax" is the word-for-word translation of strconv's
// "invalid syntax" that does not say what is expected.
var englishWords = regexp.MustCompile(`invalid|parsing|syntax|argument|flag needs|out of range|strconv|unknown unit|missing unit|ungültige Syntax`)

// invalidValues are wrong values per contract type of a flag
// (commands --json).
var invalidValues = map[string][]string{
	"integer":  {"abc", "1.5", "99999999999999999999999"},
	"id":       {"abc", "1.5", "99999999999999999999999"},
	"number":   {"abc", "1,2,3"},
	"boolean":  {"vielleicht", "2"},
	"duration": {"1x", "30", "abc", "9999999999999999999h"},
	"date":     {"2026-13-45", "morgen"},
}

// A wrong value of any flag of any command is reported in German (task
// 17, L41): the flag types come from the contract annotations.
func TestInvalidFlagValuesGermanEverywhere(t *testing.T) {
	seenTypes := map[string]bool{}
	checked := 0
	for _, c := range commandTree(t) {
		if !c.Runnable() {
			continue
		}
		path := strings.Fields(c.CommandPath())[1:]
		fs := c.LocalFlags()
		if c.Parent() == nil || c.Name() == "version" {
			fs = c.Flags() // the global flags, once
		}
		var flags []*pflag.Flag
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Name != "help" && !f.Hidden {
				flags = append(flags, f)
			}
		})
		for _, f := range flags {
			typ, ok := flagTypes[f.Value.Type()]
			if a := f.Annotations[flagValueType]; len(a) == 1 {
				typ, ok = a[0], true
			}
			if !ok {
				t.Errorf("%v --%s: Typ %q ohne Vertragstyp", path, f.Name, f.Value.Type())
				continue
			}
			seenTypes[typ] = true
			for _, v := range invalidValues[typ] {
				args := append(append(append([]string{}, path...), argsOf(c)...), "-o", "json", "--no-input", fmt.Sprintf("--%s=%s", f.Name, v))
				app, srv := apiApp(t)
				code := Run(app, args)
				_, errs := bufs(app)
				e, _ := lastJSONError(t, errs)
				checked++
				if code == 0 {
					t.Errorf("%v: ungültiger Wert angenommen (%d Requests)", args, len(srv.Requests()))
					continue
				}
				if m := englishWords.FindString(errs); m != "" {
					t.Errorf("%v (exit %d): englisch %q:\n%s", args, code, m, errs)
				}
				if e.Message == "" {
					t.Errorf("%v (exit %d): kein JSON-Fehler:\n%s", args, code, errs)
				}
			}
		}
	}
	for typ := range invalidValues {
		if !seenTypes[typ] {
			t.Errorf("Typ %s kommt in keinem Flag vor (Testwerte veraltet?)", typ)
		}
	}
	if checked < 100 {
		t.Errorf("nur %d Fälle geprüft", checked)
	}
}

// Review T19a-M1: every argument error is typed (exit 2, German). The
// validators of the built tree, the help command included, return a
// *clierr.Error for every count of arguments they reject; the command
// cobra adds only when it executes (__complete and its alias
// __completeNoDesc, with cobra's own check for at least one argument)
// ends with exit 2 too, like the own completion and help commands run
// with wrong arguments.
func TestArgsErrorsTyped(t *testing.T) {
	const completeErr = "Fehler: zu wenige Argumente für \"papierkram __complete\" (erwartet: mindestens 1, erhalten: 0)\n" +
		"Hinweis: Interner Befehl der Shell-Vervollständigung; einrichten mit papierkram completion <shell>\n"
	checked := 0
	for _, c := range commandTree(t) {
		if c.Args == nil {
			continue
		}
		for n := range 7 {
			args := make([]string, n)
			for i := range args {
				args[i] = "1"
			}
			err := c.Args(c, args)
			if err == nil {
				continue
			}
			checked++
			var ce *clierr.Error
			if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || ce.Exit() != 2 {
				t.Errorf("%s mit %d Argumenten: %T %v", c.CommandPath(), n, err, err)
			}
		}
	}
	if checked < 50 {
		t.Errorf("nur %d Argumentfehler geprüft", checked)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{cobra.ShellCompRequestCmd}, completeErr},
		{[]string{cobra.ShellCompNoDescRequestCmd}, completeErr},
		{[]string{"completion"}, "Fehler: zu wenige Argumente für \"papierkram completion\" (erwartet: 1, erhalten: 0)\n"},
		{[]string{"completion", "bash", "zsh"}, "Fehler: "},
		{[]string{"help", "invoice", "x", "y"}, "Fehler: unbekannter Befehl"},
	} {
		app, _, _ := newTestApp(t)
		if code := Run(app, tc.args); code != 2 {
			t.Errorf("%v: Exit %d, erwartet 2", tc.args, code)
		}
		if _, errs := bufs(app); !strings.HasPrefix(errs, tc.want) || !strings.Contains(errs, "Hinweis: ") || strings.Contains(errs, "requires") {
			t.Errorf("%v: stderr = %q", tc.args, errs)
		}
	}
}
