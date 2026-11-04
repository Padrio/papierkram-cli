package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// The examples of every built registry command are valid command lines
// (spec 7.1, Phase 7): they start with "papierkram <resource> <action>",
// every flag (long, short, global) exists on the built command and parses,
// and the positional arguments pass the command's argument check. Custom
// entries are checked as soon as their handler is in the command tree.
func TestExamplesAreValidCommands(t *testing.T) {
	app, _ := apiApp(t)
	check, missing := exampleEntries(NewRootCmd(app), registry.Entries())
	for _, name := range missing {
		t.Errorf("%s: generischer Befehl fehlt im Befehlsbaum", name)
	}
	for _, e := range check {
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		if len(e.Examples) < 2 {
			t.Errorf("%s: %d Beispiele, erwartet mindestens 2", name, len(e.Examples))
		}
		for _, ex := range e.Examples {
			if err := exampleProblem(t, name, ex); err != nil {
				t.Errorf("%s: Beispiel %q: %v", name, ex, err)
			}
		}
	}
	if len(check) == 0 {
		t.Fatal("keine Registry-Einträge geprüft")
	}
}

// exampleEntries splits entries into those whose command exists in root
// (to be checked) and the names of generic entries without a command. A
// Custom entry without a command is skipped: its handler is not built yet.
func exampleEntries(root *cobra.Command, entries []registry.Entry) (check []registry.Entry, missing []string) {
	for _, e := range entries {
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		cmd, _, err := root.Find(strings.Fields(name))
		switch {
		case err == nil && cmd.CommandPath() == root.Name()+" "+name:
			check = append(check, e)
		case generic(e):
			missing = append(missing, name)
		}
	}
	return check, missing
}

func entryNames(es []registry.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, strings.TrimSpace(e.Resource+" "+e.Action))
	}
	return out
}

func TestExampleEntriesIncludeBuiltCustomCommands(t *testing.T) {
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	var pdf, list registry.Entry
	for _, e := range registry.Entries() {
		switch {
		case e.Resource == "voucher" && e.Action == "pdf":
			pdf = e
		case e.Resource == "voucher" && e.Action == "list":
			list = e
		}
	}
	if !pdf.Custom {
		t.Fatalf("voucher pdf ist kein Custom-Eintrag: %+v", pdf)
	}
	// a Custom entry whose handler does not exist (stable, unlike the
	// registered ones, whose handlers come task by task)
	later := pdf
	later.Action = "spaeter"
	gone := list
	gone.Resource, gone.Action = "voucher", "gibtsnicht"

	// not built yet: skipped; a generic entry without command is missing;
	// a built Custom entry (pdf) is checked
	check, missing := exampleEntries(root, []registry.Entry{later, pdf, list, gone})
	if names := entryNames(check); strings.Join(names, ",") != "voucher pdf,voucher list" || len(missing) != 1 || missing[0] != "voucher gibtsnicht" {
		t.Errorf("vor dem Handler: check %v, missing %v", names, missing)
	}

	// once its handler is in the tree, the Custom entry is checked
	group, _, _ := root.Find([]string{"voucher"})
	group.AddCommand(&cobra.Command{Use: "spaeter <id>", RunE: func(*cobra.Command, []string) error { return nil }})
	check, _ = exampleEntries(root, []registry.Entry{later, list})
	if len(check) != 2 || check[0].Action != "spaeter" {
		t.Errorf("nach dem Handler: check %v", entryNames(check))
	}
}

// exampleProblem parses one example against a fresh command tree, so flag
// values of one example never leak into the next; nil means valid.
func exampleProblem(t *testing.T, name, ex string) error {
	t.Helper()
	words, err := shellWords(ex)
	if err != nil {
		return err
	}
	want := strings.Fields(name)
	if len(words) < 1+len(want) || words[0] != "papierkram" || strings.Join(words[1:1+len(want)], " ") != name {
		return fmt.Errorf("beginnt nicht mit %q", "papierkram "+name)
	}
	app, _ := apiApp(t)
	return commandProblem(app, words, "papierkram "+name)
}

// commandProblem checks the words of one command line (words[0] is the
// binary) against a fresh command tree of app: the command exists (and is
// want, when given), every flag exists and parses, and the positional
// arguments pass the command's argument check; nil means valid.
func commandProblem(app *App, words []string, want string) error {
	root := NewRootCmd(app)
	root.InitDefaultHelpCmd()
	cmd, rest, err := root.Find(words[1:])
	switch {
	case err != nil:
		return fmt.Errorf("unbekannter Befehl (%v)", err)
	case want != "" && cmd.CommandPath() != want:
		return fmt.Errorf("unbekannter Befehl (%s statt %s)", cmd.CommandPath(), want)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		return err
	}
	if cmd == root && len(cmd.Flags().Args()) > 0 {
		return fmt.Errorf("unbekannter Befehl %q", cmd.Flags().Arg(0))
	}
	if cmd.Args != nil {
		if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
			return fmt.Errorf("ungültige Argumente: %w", err)
		}
	}
	return nil
}

func TestExampleProblemDetectsInvalidExamples(t *testing.T) {
	for _, tc := range []struct{ name, ex string }{
		{"user list", "papierkram user describe 1"},
		{"user list", "papierkram benutzer list"},
		{"user list", "papierkram user list --bogus"},
		{"user list", "papierkram user list -x"},
		{"user list", "papierkram user list --limit viele"},
		{"user list", "papierkram user list 5"},
		{"user describe", "papierkram user describe"},
		{"info", "papierkram info 1"},
		{"transaction list", "papierkram transaction list --bank-connection"},
		{"user list", `papierkram user list --jq ".id`},
	} {
		if exampleProblem(t, tc.name, tc.ex) == nil {
			t.Errorf("%s: %q gilt als gültig", tc.name, tc.ex)
		}
	}
	for _, tc := range []struct{ name, ex string }{
		{"user list", "papierkram user list --limit 5 -o json --context prod"},
		{"info", `papierkram info --jq '.settings.custom_templates.invoices[].name'`},
		{"person describe", "papierkram person describe 1063 --company 2862 -o yaml"},
	} {
		if err := exampleProblem(t, tc.name, tc.ex); err != nil {
			t.Errorf("%s: %q: %v", tc.name, tc.ex, err)
		}
	}
}

// shellWords splits a command line like a POSIX shell for the examples:
// whitespace separates words, single and double quotes group and are
// removed. A redirection (> file, >> file, < file) is dropped with its
// target, since it is no argument of the command (pdf -O - > beleg.pdf);
// a line with more than one command (|, ;, &&, ||) or & is an error.
func shellWords(s string) ([]string, error) {
	cmds, err := shellCommands(s)
	if err != nil {
		return nil, err
	}
	switch len(cmds) {
	case 0:
		return nil, nil
	case 1:
		return cmds[0], nil
	}
	return nil, errShellSyntax
}

// shellCommands splits a command line into its commands, separated by
// |, ||, && or ; outside quotes, and each command into words like
// shellWords. An unquoted # at the start of a word starts a comment up to
// the end of the line. An empty command (a | at the end, "a ;; b") and a
// single & (background) are errors.
func shellCommands(s string) ([][]string, error) {
	var cmds [][]string
	var words []string
	var cur strings.Builder
	inWord := false
	redirect := false // the next word is a redirection target
	var quote rune
	end := func() {
		if !inWord {
			return
		}
		if redirect {
			redirect = false
		} else {
			words = append(words, cur.String())
		}
		cur.Reset()
		inWord = false
	}
	next := func() error {
		end()
		if redirect || len(words) == 0 {
			return errShellSyntax
		}
		cmds = append(cmds, words)
		words = nil
		return nil
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t':
			end()
		case r == '#' && !inWord:
			i = len(rs)
		case r == '>' || r == '<':
			end()
			redirect = true
		case r == '|' || r == ';' || r == '&':
			// && and || are one separator; a single & is not supported
			if (r == '&' || r == '|') && i+1 < len(rs) && rs[i+1] == r {
				i++
			} else if r == '&' {
				return nil, errShellSyntax
			}
			if err := next(); err != nil {
				return nil, err
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	end()
	if redirect {
		return nil, errShellSyntax
	}
	if len(words) > 0 {
		cmds = append(cmds, words)
	} else if len(cmds) > 0 {
		return nil, errShellSyntax // ends with a separator
	}
	return cmds, nil
}

var (
	errShellSyntax   = errors.New("unerlaubte Shell-Syntax (|, ;, & oder Umleitung ohne Ziel) außerhalb von Anführungszeichen")
	errUnclosedQuote = errors.New("nicht geschlossenes Anführungszeichen")
)

func TestShellWords(t *testing.T) {
	for in, want := range map[string][]string{
		`papierkram x create --name "Müller & Söhne" --jq '.a[] | .b' -o json`: {"papierkram", "x", "create", "--name", "Müller & Söhne", "--jq", ".a[] | .b", "-o", "json"},
		// redirections and their targets are not arguments of the command
		"papierkram x pdf 1 -O - > beleg.pdf":             {"papierkram", "x", "pdf", "1", "-O", "-"},
		"papierkram x update 1 --input - <a.json -o json": {"papierkram", "x", "update", "1", "--input", "-", "-o", "json"},
		// a comment ends the line; # inside a word or quotes is text
		"papierkram x list --sort a#b # Kommentar | y": {"papierkram", "x", "list", "--sort", "a#b"},
		`papierkram x create --name '# eins'`:          {"papierkram", "x", "create", "--name", "# eins"},
	} {
		got, err := shellWords(in)
		if err != nil || strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("shellWords(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{`a "b`, `a | b`, `a ; b`, `a &`, `a >`, `a && b`, `a || b`} {
		if _, err := shellWords(bad); err == nil {
			t.Errorf("shellWords(%q): kein Fehler", bad)
		}
	}
}

func TestShellCommands(t *testing.T) {
	for in, want := range map[string]string{
		"a 1 | b 2":                     "a 1/b 2",
		"a && b; c || d":                "a/b/c/d",
		`echo "x | y" | b --jq '.a|.b'`: "echo x | y/b --jq .a|.b",
		"a > f.txt | b":                 "a/b",
		"":                              "",
		"# nur Kommentar":               "",
	} {
		cmds, err := shellCommands(in)
		var got []string
		for _, c := range cmds {
			got = append(got, strings.Join(c, " "))
		}
		if err != nil || strings.Join(got, "/") != want {
			t.Errorf("shellCommands(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"a |", "a | | b", "| a", "a & b", "a ;; b", "a > | b"} {
		if cmds, err := shellCommands(bad); err == nil {
			t.Errorf("shellCommands(%q) = %q: kein Fehler", bad, cmds)
		}
	}
}

// The examples of the commands outside the registry (time log, the timer
// commands time start|status|stop|discard, local init|show, context
// create|use|list|active|delete|rename, auth logout|status, version, api,
// commands, schema, completion, gen-docs) parse against their command like the registry examples:
// TestExamplesAreValidCommands covers registry entries only. auth login
// is left out: its argument check wants a terminal for the hidden token
// input, which its example assumes.
func TestCustomCommandExamplesAreValid(t *testing.T) {
	app, _ := apiApp(t)
	registered := map[string]bool{}
	for _, e := range registry.Entries() {
		registered[strings.TrimSpace("papierkram "+e.Resource+" "+e.Action)] = true
	}
	var checked []string
	walk(NewRootCmd(app), func(c *cobra.Command) {
		if c.Example == "" || registered[c.CommandPath()] || c.CommandPath() == "papierkram auth login" {
			return
		}
		name := strings.TrimPrefix(c.CommandPath(), "papierkram ")
		checked = append(checked, name)
		lines := strings.Split(c.Example, "\n")
		if len(lines) < 2 {
			t.Errorf("%s: %d Beispiele, erwartet mindestens 2", name, len(lines))
		}
		for _, line := range lines {
			if err := exampleProblem(t, name, strings.TrimSpace(line)); err != nil {
				t.Errorf("%s: Beispiel %q: %v", name, line, err)
			}
		}
	})
	for _, want := range []string{"time log", "time start", "time status", "time stop", "time discard", "local init", "local show", "api", "commands", "schema",
		"auth logout", "auth status", "context active", "context delete", "context list", "context rename", "context use", "version",
		"completion", "gen-docs"} {
		if !slices.Contains(checked, want) {
			t.Errorf("%s: keine Beispiele geprüft (geprüft: %v)", want, checked)
		}
	}
}
