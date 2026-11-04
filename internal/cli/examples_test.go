package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// The examples of every generic registry command are valid command lines
// (spec 7.1, Phase 7): they start with "papierkram <resource> <action>",
// every flag (long, short, global) exists on the built command and parses,
// and the positional arguments pass the command's argument check.
func TestExamplesAreValidCommands(t *testing.T) {
	checked := 0
	for _, e := range registry.Entries() {
		if !generic(e) {
			continue
		}
		name := strings.TrimSpace(e.Resource + " " + e.Action)
		if len(e.Examples) < 2 {
			t.Errorf("%s: %d Beispiele, erwartet mindestens 2", name, len(e.Examples))
		}
		for _, ex := range e.Examples {
			if err := exampleProblem(t, name, ex); err != nil {
				t.Errorf("%s: Beispiel %q: %v", name, ex, err)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("keine generischen Registry-Einträge geprüft")
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
	cmd, rest, err := NewRootCmd(app).Find(words[1:])
	if err != nil || cmd.CommandPath() != "papierkram "+name {
		return fmt.Errorf("unbekannter Befehl (%v)", err)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		return err
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
// removed. Redirections and other shell syntax are not supported (and not
// used by the examples of generic commands).
func shellWords(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
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
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case r == '|' || r == '>' || r == '<' || r == ';' || r == '&':
			return nil, errShellSyntax
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errUnclosedQuote
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

var (
	errShellSyntax   = errors.New("unerlaubte Shell-Syntax (|, >, <, ;, &) außerhalb von Anführungszeichen")
	errUnclosedQuote = errors.New("nicht geschlossenes Anführungszeichen")
)

func TestShellWords(t *testing.T) {
	got, err := shellWords(`papierkram x create --name "Müller & Söhne" --jq '.a[] | .b' -o json`)
	want := []string{"papierkram", "x", "create", "--name", "Müller & Söhne", "--jq", ".a[] | .b", "-o", "json"}
	if err != nil || strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("shellWords = %q, %v", got, err)
	}
	for _, bad := range []string{`a "b`, `a > b`, `a | b`} {
		if _, err := shellWords(bad); err == nil {
			t.Errorf("shellWords(%q): kein Fehler", bad)
		}
	}
}
