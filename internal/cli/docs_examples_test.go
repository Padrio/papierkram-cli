package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// The command lines of the documentation (README.md, AGENTS.md) are valid
// commands of the built tree: the command exists, every flag exists and
// parses, and the positional arguments pass the argument check (task 45).
// Checked are the shell code blocks (sh, bash, shell, console, powershell
// or without language); a line marked with the comment "# Platzhalter" is
// a syntax sketch and is skipped.

// docFiles are the documents whose command lines are checked.
var docFiles = []string{"README.md", "AGENTS.md"}

// shellBlock matches the opening fence of a checked code block.
var shellBlock = regexp.MustCompile("^```(sh|bash|shell|console|powershell)?\\s*$")

// placeholderMark marks a line that is no runnable command.
const placeholderMark = "# Platzhalter"

// docLine is one command line of a document.
type docLine struct {
	file string
	line int
	text string
}

// docCommandLines returns the lines of the checked code blocks of the
// file at path, with continuation lines (ending in \) joined and a
// leading "$ " prompt removed; empty lines and comment lines are left
// out.
func docCommandLines(t *testing.T, path string) []docLine {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []docLine
	in, checked := false, false
	var cont strings.Builder
	start := 0
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !in {
				in, checked = true, shellBlock.MatchString(strings.TrimSpace(line))
			} else {
				in = false
			}
			continue
		}
		if !in || !checked {
			continue
		}
		if cont.Len() == 0 {
			start = n
		}
		if s, ok := strings.CutSuffix(line, `\`); ok {
			cont.WriteString(s + " ")
			continue
		}
		cont.WriteString(line)
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cont.String()), "$ "))
		cont.Reset()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		out = append(out, docLine{filepath.Base(path), start, text})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if in {
		t.Fatalf("%s: Codeblock nicht geschlossen", path)
	}
	return out
}

// envAssignment matches a leading NAME=value of a command.
var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// cliCommands returns the commands of a line that call the CLI (papierkram
// or its alias pk), without leading environment assignments and with the
// binary named papierkram.
func cliCommands(text string) ([][]string, error) {
	cmds, err := shellCommands(text)
	if err != nil {
		return nil, err
	}
	var out [][]string
	for _, words := range cmds {
		for len(words) > 0 && envAssignment.MatchString(words[0]) {
			words = words[1:]
		}
		if len(words) > 0 && (words[0] == "papierkram" || words[0] == "pk") {
			out = append(out, append([]string{"papierkram"}, words[1:]...))
		}
	}
	return out, nil
}

func TestDocCommandsAreValid(t *testing.T) {
	root := modroot.Find(t)
	for _, name := range docFiles {
		lines := docCommandLines(t, filepath.Join(root, name))
		found := 0
		for _, l := range lines {
			if strings.Contains(l.text, placeholderMark) {
				continue
			}
			cmds, err := cliCommands(l.text)
			if err != nil {
				t.Errorf("%s:%d: %q: %v", l.file, l.line, l.text, err)
				continue
			}
			for _, words := range cmds {
				found++
				if err := docCommandProblem(t, words); err != nil {
					t.Errorf("%s:%d: %q: %v", l.file, l.line, strings.Join(words, " "), err)
				}
			}
		}
		if found == 0 {
			t.Errorf("%s: keine Befehlszeilen gefunden", name)
		}
		t.Logf("%s: %d Befehle geprüft", name, found)
	}
}

// docCommandProblem checks one command of the documentation. The reader
// sits at a terminal (auth login and local init without flags ask there).
func docCommandProblem(t *testing.T, words []string) error {
	t.Helper()
	app, _ := apiApp(t)
	app.StdinTTY, app.StdoutTTY = true, true
	return commandProblem(app, words, "")
}

func TestDocCommandLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	doc := "Text papierkram foo\n" +
		"```sh\n" +
		"$ papierkram user list \\\n" +
		"  --limit 5\n" +
		"# Kommentar\n" +
		"\n" +
		"PAPIERKRAM_READ_ONLY=1 pk info | jq .\r\n" +
		"```\n" +
		"```toml\n" +
		"papierkram nicht geprüft\n" +
		"```\n" +
		"```\n" +
		"papierkram version\n" +
		"```\n"
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range docCommandLines(t, path) {
		cmds, err := cliCommands(l.text)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cmds {
			got = append(got, fmt.Sprintf("%d:%s", l.line, strings.Join(c, " ")))
		}
	}
	want := "3:papierkram user list --limit 5|7:papierkram info|13:papierkram version"
	if strings.Join(got, "|") != want {
		t.Errorf("Befehle = %q, want %q", strings.Join(got, "|"), want)
	}
}

func TestDocCommandProblemDetectsInvalidLines(t *testing.T) {
	for _, bad := range []string{
		"papierkram rechnung lst",
		"papierkram gibtsnicht",
		"papierkram invoice list --bogus",
		"papierkram invoice describe",
		"papierkram time log --duration",
		"papierkram --kontext x info",
	} {
		words, err := shellWords(bad)
		if err != nil {
			t.Fatal(err)
		}
		if docCommandProblem(t, words) == nil {
			t.Errorf("%q gilt als gültig", bad)
		}
	}
	for _, good := range []string{
		"papierkram --help",
		"papierkram help time log",
		"papierkram rechnung list --limit 5 -o json",
		"papierkram auth login --context firma-gmbh --subdomain firma-gmbh",
	} {
		words, err := shellWords(good)
		if err != nil {
			t.Fatal(err)
		}
		if err := docCommandProblem(t, words); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}
