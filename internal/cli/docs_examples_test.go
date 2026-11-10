package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// The command lines of the documentation (README.md, AGENTS.md and the
// Claude Code skill) are valid
// commands of the built tree: the command exists, every flag exists and
// parses, and the positional arguments pass the argument check (task 45);
// documented mutations also run with --dry-run against the mock.
// Checked are the shell code blocks (sh, bash, shell, console, powershell
// or without language); a line marked with the comment "# Platzhalter" is
// a syntax sketch and is skipped.

// docFiles are the documents whose command lines are checked, relative
// to the module root with "/".
var docFiles = []string{"README.md", "AGENTS.md", "skills/papierkram/SKILL.md"}

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

// docCmd is one CLI call of a documented line: the environment assignments
// in front of it and its words, the binary named papierkram.
type docCmd struct {
	env   map[string]string
	words []string
}

// docShellCommands are the commands besides the CLI that a checked code
// block may run (installation, completion setup, jq, the MCP setup of
// Claude Code); every other command
// is an error, so a misspelled papierkram is never skipped silently.
var docShellCommands = []string{"brew", "scoop", "go", "source", "jq", "claude"}

// substPlaceholder replaces a $(…) or <(…) in the enclosing line.
const substPlaceholder = "SUBST"

// docLineCommands returns the CLI calls (papierkram or its alias pk) of a
// documented line, with the binary named papierkram. Command and process
// substitutions ($(…), <(…)) are unwrapped: their content is checked as a
// line of its own. Every command must parse and be the CLI or one of
// docShellCommands.
func docLineCommands(text string) ([]docCmd, error) {
	outer, inner, err := unwrapSubstitutions(text)
	if err != nil {
		return nil, err
	}
	cmds, err := shellCommands(outer)
	if err != nil {
		return nil, err
	}
	var out []docCmd
	for _, words := range cmds {
		env := map[string]string{}
		for len(words) > 0 && envAssignment.MatchString(words[0]) {
			k, v, _ := strings.Cut(words[0], "=")
			env[k] = v
			words = words[1:]
		}
		switch {
		case len(words) == 0:
			return nil, fmt.Errorf("kein Befehl nach den Zuweisungen %v", env)
		case words[0] == "papierkram" || words[0] == "pk":
			out = append(out, docCmd{env, append([]string{"papierkram"}, words[1:]...)})
		case len(words) >= 3 && words[0] == "claude" && words[1] == "mcp" && words[2] == "add":
			// claude mcp add <name> -- papierkram mcp serve …: the server
			// command line is checked like any other; a server that is not
			// the CLI is an error
			i := slices.Index(words, "--")
			if i < 0 || i+1 >= len(words) || words[i+1] != "papierkram" && words[i+1] != "pk" {
				return nil, fmt.Errorf("claude mcp add ohne papierkram oder pk nach --")
			}
			out = append(out, docCmd{env, append([]string{"papierkram"}, words[i+2:]...)})
		case !slices.Contains(docShellCommands, words[0]):
			return nil, fmt.Errorf("unbekannter Befehl %q (Tippfehler? sonst in docShellCommands aufnehmen)", words[0])
		}
	}
	for _, in := range inner {
		cmds, err := docLineCommands(in)
		if err != nil {
			return nil, fmt.Errorf("in $(…)/<(…): %w", err)
		}
		out = append(out, cmds...)
	}
	return out, nil
}

// unwrapSubstitutions replaces every $(…) and <(…) outside single quotes
// by substPlaceholder and returns their contents (nested ones stay in the
// content and are unwrapped when it is checked). An unclosed substitution
// is an error.
func unwrapSubstitutions(text string) (string, []string, error) {
	var outer strings.Builder
	var inner []string
	rs := []rune(text)
	var quote rune
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == quote {
				quote = 0
			}
		case r == '\'' && quote == 0:
			quote = r
		case r == '"':
			if quote == '"' {
				quote = 0
			} else {
				quote = r
			}
		case (r == '$' || r == '<') && i+1 < len(rs) && rs[i+1] == '(':
			end, err := closingParen(rs, i+2)
			if err != nil {
				return "", nil, err
			}
			inner = append(inner, string(rs[i+2:end]))
			outer.WriteString(substPlaceholder)
			i = end
			continue
		}
		outer.WriteRune(r)
	}
	return outer.String(), inner, nil
}

// closingParen returns the index of the ) that closes the substitution
// whose content starts at start, skipping quoted text and nested
// parentheses.
func closingParen(rs []rune, start int) (int, error) {
	depth := 0
	var quote rune
	for i := start; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			if depth == 0 {
				return i, nil
			}
			depth--
		}
	}
	return 0, errors.New("$(…) bzw. <(…) nicht geschlossen")
}

func TestDocLineCommandsUnwrapAndReject(t *testing.T) {
	// want: the CLI calls joined by "/" ("" none); "-" an error
	for line, want := range map[string]string{
		"source <(papierkram completion bash)":                              "papierkram completion bash",
		`brew install x && echo "$(pk version -o json)"`:                    "-",
		"PAPIERKRAM_READ_ONLY=1 papierkram info | jq .":                     "PAPIERKRAM_READ_ONLY=1 papierkram info",
		"go install github.com/Padrio/papierkram-cli/cmd/papierkram@latest": "",
		`papierkram completion zsh > "${fpath[1]}/_papierkram"`:             "papierkram completion zsh",
		"source <(papierkram completion $(echo bash))":                      "-",
		"jq . <(papierkram invoice list -o json)":                           "papierkram invoice list -o json",
		`papierkram invoice list --jq '.entries[] | "$(x)"'`:                "papierkram invoice list --jq .entries[] | \"$(x)\"",
		"claude mcp add papierkram -- papierkram mcp serve --allow-writes":  "papierkram mcp serve --allow-writes",
		"claude mcp list": "",
		"claude mcp add pk -- pk mcp serve --files-dir /x": "papierkram mcp serve --files-dir /x",
		"claude mcp add x -- npx other-server":             "-",
		"claude mcp add x":                                 "-",
		"claude mcp add x --":                              "-",
	} {
		cmds, err := docLineCommands(line)
		var got []string
		for _, c := range cmds {
			var env []string
			for k, v := range c.env {
				env = append(env, k+"="+v)
			}
			got = append(got, strings.Join(append(env, c.words...), " "))
		}
		if want == "-" {
			if err == nil {
				t.Errorf("docLineCommands(%q) = %q: kein Fehler", line, got)
			}
			continue
		}
		if err != nil || strings.Join(got, "/") != want {
			t.Errorf("docLineCommands(%q) = %q, %v, want %q", line, got, err, want)
		}
	}
	for _, bad := range []string{
		"papierkam invoice list",           // typo: unknown command
		`papierkram invoice list --jq ".a`, // does not parse
		"echo $(papierkram invoice list",   // unclosed substitution
		"source <(papierkram x | )",        // inner does not parse
		"ls -l",                            // not allowed
	} {
		if cmds, err := docLineCommands(bad); err == nil {
			t.Errorf("docLineCommands(%q) = %v: kein Fehler", bad, cmds)
		}
	}
}

func TestDocCommandsAreValid(t *testing.T) {
	root := modroot.Find(t)
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, name := range docFiles {
		lines := docCommandLines(t, filepath.Join(root, filepath.FromSlash(name)))
		found, dryRuns := 0, 0
		for _, l := range lines {
			if strings.Contains(l.text, placeholderMark) {
				continue
			}
			cmds, err := docLineCommands(l.text)
			if err != nil {
				t.Errorf("%s:%d: %q: %v", l.file, l.line, l.text, err)
				continue
			}
			for _, c := range cmds {
				found++
				if err := docCommandProblem(t, c.words); err != nil {
					t.Errorf("%s:%d: %q: %v", l.file, l.line, strings.Join(c.words, " "), err)
				} else if run, ok := docDryRun(t, d, c.words); ok {
					dryRuns++
					code, errs, srv := runLineServer(t, c.env, run)
					if code != 0 {
						t.Errorf("%s:%d: %q: exit %d, stderr %q", l.file, l.line, strings.Join(run, " "), code, errs)
					}
					if err := dryRunRequests(srv, run); err != nil {
						t.Errorf("%s:%d: %q: %v", l.file, l.line, strings.Join(run, " "), err)
					}
				}
			}
		}
		if found == 0 {
			t.Errorf("%s: keine Befehlszeilen gefunden", name)
		}
		t.Logf("%s: %d Befehle geprüft, davon %d Änderungen per --dry-run gegen den Mock", name, found, dryRuns)
	}
}

// docCommandProblem checks one command of the documentation. The reader
// sits at a terminal (auth login and local init without flags ask there).
// Beyond command, flags and arguments the output options must fit: the
// --jq program compiles like the CLI compiles it, and every --fields path
// exists in the response schema of the command's operation (review M1).
func docCommandProblem(t *testing.T, words []string) error {
	t.Helper()
	app, _ := apiApp(t)
	app.StdinTTY, app.StdoutTTY = true, true
	if err := commandProblem(app, words, ""); err != nil {
		return err
	}
	if expr, ok := flagValue(words, "--jq"); ok {
		if err := output.CheckJQ(expr); err != nil {
			return fmt.Errorf("--jq: %w", err)
		}
	}
	if fields, ok := flagValue(words, "--fields"); ok {
		return fieldsProblem(app, words, output.ParseFields(fields))
	}
	return nil
}

// flagValue returns the value of a long flag given as "--name value" or
// "--name=value" (the last one wins, like pflag).
func flagValue(words []string, name string) (string, bool) {
	value, found := "", false
	for i, w := range words {
		if v, ok := strings.CutPrefix(w, name+"="); ok {
			value, found = v, true
		} else if w == name && i+1 < len(words) {
			value, found = words[i+1], true
		}
	}
	return value, found
}

// fieldsProblem checks --fields paths the way the CLI applies them: each
// path is projected by the output package (output.RenderValue with
// Fields) onto a skeleton of the value the command prints, and must come
// out again. The printed value is the one the CLI really writes:
//   - --dry-run: docs/schemas/dry-run.json
//   - pdf: the message {path, bytes} (pdfSaved), only with -O datei; a
//     PDF on stdout takes no --fields (the CLI refuses it too)
//   - deliver: docs/schemas/deliver.json; delete: docs/schemas/delete.json
//   - voucher create --file: the voucher or, on exit 11,
//     docs/schemas/partial.json (a path of either passes)
//   - otherwise the response schema of the operation.
//
// Commands without an operation or without a JSON response schema (local
// commands, api, upload, info) print a shape no schema describes; they
// are not checked.
func fieldsProblem(app *App, words, fields []string) error {
	cmd, _, err := NewRootCmd(app).Find(words[1:])
	if err != nil {
		return nil
	}
	name := strings.Join(commandWords(cmd), " ")
	contract := func(n string) func([]string) any {
		return func(p []string) any { return skeleton(contractMap(n), p) }
	}
	var shapes []func([]string) any
	switch {
	case slices.Contains(words, "--dry-run"):
		shapes = append(shapes, contract("dry-run"))
	case cmd.Name() == "pdf":
		if dest, ok := pdfTarget(words); !ok || dest == pdfStdout {
			return errors.New("--fields gilt nicht für ein PDF auf stdout; nur mit -O datei")
		}
		shapes = append(shapes, func([]string) any { return pdfSaved{} })
	case cmd.Name() == "deliver":
		shapes = append(shapes, contract("deliver"))
	case cmd.Name() == "delete":
		shapes = append(shapes, contract("delete"))
	case !metaOf(cmd).op:
		return nil
	default:
		pair, err := operationSchemas(metaOf(cmd).entry.OperationID)
		if err != nil {
			return err
		}
		schema, ok := pair.Response.(map[string]any)
		if !ok || schema["format"] == "binary" {
			return nil
		}
		shapes = append(shapes, func(p []string) any { return skeleton(schema, p) })
		if name == "voucher create" && hasFlag(words, "--file") {
			shapes = append(shapes, contract("partial"))
		}
	}
	for _, f := range fields {
		path := strings.Split(f, ".")
		if !slices.ContainsFunc(shapes, func(shape func([]string) any) bool { return projects(shape(path), f, path) }) {
			return fmt.Errorf("--fields: %q steht nicht in der Ausgabe von papierkram %s", f, name)
		}
	}
	return nil
}

// hasFlag reports whether the long flag is given ("--name x" or "--name=x").
func hasFlag(words []string, name string) bool {
	_, ok := flagValue(words, name)
	return ok
}

// pdfTarget returns the value of -O/--output-file.
func pdfTarget(words []string) (string, bool) {
	dest, ok := flagValue(words, "--output-file")
	if v, short := flagValue(words, "-O"); short {
		dest, ok = v, true
	}
	return dest, ok
}

// contractMap is the contract docs/schemas/<name>.json as plain JSON.
func contractMap(name string) map[string]any {
	s, err := contractSchema(name)
	if err != nil {
		panic(err)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	return m
}

// skeleton is a value of schema with every declared property (scalars
// null, enums their first value, arrays one element). A free-form object
// (no declared properties, or additionalProperties) gets the key the
// path asks for, so any name passes there. A list envelope (type "list")
// carries the path into its entries, which --fields projects one by one.
func skeleton(schema map[string]any, path []string) any {
	if enum, _ := schema["enum"].([]any); len(enum) > 0 {
		return enum[0]
	}
	if items, ok := schema["items"].(map[string]any); ok {
		return []any{skeleton(items, path)}
	}
	props, _ := schema["properties"].(map[string]any)
	extra, extraSet := schema["additionalProperties"]
	freeForm := (extraSet && extra != false) || (props == nil && (schema["type"] == "object" || schema["type"] == nil))
	if props == nil && !freeForm {
		return nil // scalar
	}
	out := map[string]any{}
	for k, ps := range props {
		var rest []string
		if len(path) > 0 && path[0] == k {
			rest = path[1:]
		}
		child, _ := ps.(map[string]any)
		out[k] = skeleton(child, rest)
	}
	if _, declared := props[firstOf(path)]; freeForm && len(path) > 0 && !declared {
		child, _ := extra.(map[string]any)
		out[path[0]] = skeleton(child, path[1:])
	}
	if t, _ := out["type"].(string); t == "list" {
		if entries, ok := props["entries"].(map[string]any); ok {
			items, _ := entries["items"].(map[string]any)
			out["entries"] = []any{skeleton(items, path)}
		}
	}
	return out
}

func firstOf(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[0]
}

// projects reports whether --fields f keeps path of v: v is rendered with
// the CLI's own projection and the path looked up in the result (in the
// first entry of a list).
func projects(v any, f string, path []string) bool {
	var b strings.Builder
	if err := output.RenderValue(output.Options{Format: output.FormatJSON, Fields: []string{f}, Out: &b}, v, output.Columns{}); err != nil {
		return false
	}
	var got any
	if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
		return false
	}
	if m, ok := got.(map[string]any); ok && m["type"] == "list" {
		entries, _ := m["entries"].([]any)
		if len(entries) == 0 {
			return false
		}
		got = entries[0]
	}
	for _, k := range path {
		m, ok := got.(map[string]any)
		if !ok {
			return false
		}
		if got, ok = m[k]; !ok {
			return false
		}
	}
	return true
}

// docDryRun returns the arguments to run a documented mutation with
// --dry-run against the mock (task 28 M2, like the registry examples) and
// whether it runs. Mutations are the commands whose class in commands
// --json is a mutation class (dryRunnable); commands that read stdin (an
// argument "-"), help calls (--help, -h: nothing would run) and time stop
// (needs a running timer) are only parsed.
func docDryRun(t *testing.T, d cmdDoc, words []string) ([]string, bool) {
	t.Helper()
	app, _ := apiApp(t)
	cmd, _, err := NewRootCmd(app).Find(words[1:])
	if err != nil || slices.Contains(words, "-") || slices.Contains(words, "--help") || slices.Contains(words, "-h") {
		return nil, false
	}
	name := strings.TrimPrefix(cmd.CommandPath(), "papierkram ")
	i := slices.IndexFunc(d.Commands, func(c cmdInfo) bool { return c.Command == name })
	if i < 0 || !dryRunnable(d.Commands[i]) || name == "time stop" {
		return nil, false
	}
	run := append([]string{}, words[1:]...)
	if !slices.Contains(run, "--dry-run") {
		run = append(run, "--dry-run")
	}
	return run, true
}

func TestDocDryRun(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for line, want := range map[string]string{
		"papierkram invoice update 459 --name X":        "invoice update 459 --name X --dry-run",
		"papierkram company delete 12 --dry-run --yes":  "company delete 12 --dry-run --yes",
		"papierkram invoice list":                       "",
		"papierkram invoice update 459 --input -":       "",
		"papierkram time stop":                          "",
		"papierkram invoice create --help":              "",
		"papierkram context create x --subdomain x":     "",
		"papierkram api DELETE /tracker/time_entries/7": "",
	} {
		words, err := shellWords(line)
		if err != nil {
			t.Fatal(err)
		}
		run, ok := docDryRun(t, d, words)
		if got := strings.Join(run, " "); got != want || ok != (want != "") {
			t.Errorf("docDryRun(%q) = %q, %v, want %q", line, got, ok, want)
		}
	}
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
		cmds, err := docLineCommands(l.text)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cmds {
			got = append(got, fmt.Sprintf("%d:%s", l.line, strings.Join(c.words, " ")))
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
		// review M1: the jq program compiles, --fields names exist
		`papierkram invoice list -o json --jq '.entries[] | select('`,
		`papierkram invoice list --jq='.entries[] | bogus'`,
		"papierkram invoice list --fields id,bogusfield",
		"papierkram invoice list --fields=billing.bogus",
		"papierkram invoice describe 459 --fields id,entries",
		"papierkram time list --fields id,started_at.x",
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
		`papierkram invoice list --jq '(now | strftime("%Y-%m-%d")) as $heute | .entries[] | select(.due_date < $heute)'`,
		"papierkram invoice list --fields id,billing.company,due_date",
		"papierkram invoice describe 459 --fields=id,line_items",
		"papierkram time status --fields running",
		"papierkram commands --json --jq .commands[].command",
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

// The error recipe of AGENTS.md works on the real output: with -o json and
// --jq the last stderr line is the JSON error, with -o yaml the YAML
// document from the last line "error:" to the end.
func TestAgentsErrorRecipe(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(modroot.Find(t), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range []string{
		"die **letzte Zeile** von stderr als JSON lesen",
		"stderr ab der **letzten** Zeile, die genau `error:` lautet, bis zum Ende als YAML lesen",
	} {
		if !strings.Contains(string(doc), recipe) {
			t.Errorf("AGENTS.md: Rezept %q fehlt", recipe)
		}
	}
	type errDoc struct {
		Error struct {
			Code     string `json:"code" yaml:"code"`
			ExitCode int    `json:"exit_code" yaml:"exit_code"`
		} `json:"error" yaml:"error"`
	}
	base := []string{"company", "delete", "5", "--no-input"}
	for _, format := range [][]string{{"-o", "json"}, {"--jq", ".id"}, {"-o", "yaml"}} {
		code, errs := runLine(t, nil, append(append([]string{}, base...), format...))
		lines := strings.Split(strings.TrimRight(errs, "\n"), "\n")
		var d errDoc
		if format[1] == "yaml" {
			start := -1
			for i, l := range lines {
				if l == "error:" {
					start = i
				}
			}
			if start < 0 {
				t.Fatalf("%v: keine Zeile error: in %q", format, errs)
			}
			err = yaml.Unmarshal([]byte(strings.Join(lines[start:], "\n")), &d)
		} else {
			err = json.Unmarshal([]byte(lines[len(lines)-1]), &d)
		}
		if err != nil || code != 8 || d.Error.Code != "confirmation_required" || d.Error.ExitCode != 8 {
			t.Errorf("%v: exit %d, %+v, %v; stderr %q", format, code, d, err, errs)
		}
	}
}

// The Claude Code skill starts with YAML frontmatter that names it and
// says when to use it in one paragraph (task 46); a colon or # in the
// description would make the YAML invalid and the skill unloadable.
func TestSkillFrontmatter(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(modroot.Find(t), "skills", "papierkram", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(doc), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	front, body, closed := strings.Cut(rest, "\n---\n")
	if !ok || !closed {
		t.Fatal("SKILL.md: Frontmatter zwischen zwei Zeilen --- fehlt")
	}
	var meta map[string]string
	if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
		t.Fatalf("SKILL.md: Frontmatter ist kein gültiges YAML: %v", err)
	}
	if meta["name"] != "papierkram" || len(meta) != 2 {
		t.Errorf("SKILL.md: Frontmatter = %q, want name papierkram und description", meta)
	}
	desc := meta["description"]
	if desc == "" || strings.Contains(desc, "\n") || len(desc) > 1024 {
		t.Errorf("SKILL.md: description muss ein Absatz mit höchstens 1024 Zeichen sein: %q", desc)
	}
	if !strings.Contains(body, "AGENTS.md") {
		t.Error("SKILL.md: Verweis auf AGENTS.md fehlt")
	}
}

// skillPaidStatus returns the invoice paid-status lines of the skill
// (workflow "offene Posten"), in document order, as the lines themselves
// (environment and words).
func skillPaidStatus(t *testing.T) []docCmd {
	t.Helper()
	var out []docCmd
	for _, l := range docCommandLines(t, filepath.Join(modroot.Find(t), "skills", "papierkram", "SKILL.md")) {
		cmds, err := docLineCommands(l.text)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cmds {
			if len(c.words) > 2 && c.words[1] == "invoice" && c.words[2] == "paid-status" {
				out = append(out, c)
			}
		}
	}
	return out
}

// The paid-status lines of "offene Posten prüfen" run on a list the way
// the API answers (outstanding_amount as a string): drafts, canceled and
// deleted invoices are no open items even with an amount left (the spec example
// of a canceled invoice keeps "119.0"), paid ones ("0.0") neither, and
// only due dates before today count as overdue (review I1, task 4: the
// report replaces the jq programs; ruling T4-A: deleted). Each line runs read-only and prints
// the invoice numbers of want, in order.
func TestSkillOpenItems(t *testing.T) {
	list := `{"type":"list","page":1,"page_size":100,"total_pages":1,"total_entries":6,"has_more":false,"entries":[
	{"id":1,"invoice_no":"RE-1","state":"unpaid","due_date":"2000-01-31","total_gross":19.0,"outstanding_amount":"19.0","billing":{"company":"Muster GmbH"}},
	{"id":2,"invoice_no":"RE-2","state":"partially_paid","due_date":"2999-12-31","total_gross":200.0,"outstanding_amount":"100.0","billing":{"company":"Muster GmbH"}},
	{"id":3,"invoice_no":"RE-3","state":"paid","due_date":"2000-01-31","total_gross":10.0,"outstanding_amount":"0.0","billing":{"company":"Muster GmbH"}},
	{"id":4,"invoice_no":"RE-4","state":"canceled","due_date":"2000-01-31","total_gross":119.0,"outstanding_amount":"119.0","billing":{"company":"Muster GmbH"}},
	{"id":5,"invoice_no":null,"state":"draft","due_date":null,"total_gross":10.0,"outstanding_amount":"10.0","billing":{"company":"Muster GmbH"}},
	{"id":6,"invoice_no":"RE-6","state":"deleted","due_date":"2000-01-31","total_gross":50.0,"outstanding_amount":"50.0","billing":{"company":"Muster GmbH"}}]}`
	want := [][]string{{"RE-1"}, {"RE-1", "RE-2"}, {"RE-1", "RE-2"}}
	lines := skillPaidStatus(t)
	if len(lines) != len(want) {
		t.Fatalf("SKILL.md: %d Zeilen invoice paid-status, want %d (überfällige, Kunde, Zeitraum mit --jq)", len(lines), len(want))
	}
	for i, c := range lines {
		env := map[string]string{"PAPIERKRAM_TOKEN": "test-token-skill"}
		for k, v := range c.env {
			env[k] = v
		}
		if env["PAPIERKRAM_READ_ONLY"] != "1" {
			t.Errorf("%s: ohne PAPIERKRAM_READ_ONLY=1", strings.Join(c.words, " "))
		}
		app, srv := mockApp(t, env)
		srv.On("GET", "/income/invoices", 200, []byte(list))
		code := Run(app, c.words[1:])
		out, errs := bufs(app)
		if code != 0 {
			t.Fatalf("%s\nexit %d, stderr %q", strings.Join(c.words, " "), code, errs)
		}
		var docs []map[string]any
		if slices.Contains(c.words, "--jq") {
			for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
				var d map[string]any
				if err := json.Unmarshal([]byte(line), &d); err != nil {
					t.Fatalf("%s: %v\n%s", strings.Join(c.words, " "), err, out)
				}
				docs = append(docs, d)
			}
		} else {
			wantContract(t, "paid-status", []byte(out))
			var report struct{ Entries []map[string]any }
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatal(err)
			}
			docs = report.Entries
		}
		var got []string
		for _, d := range docs {
			got = append(got, fmt.Sprint(d["invoice_no"]))
		}
		if !slices.Equal(got, want[i]) {
			t.Errorf("%s\nRechnungen %v, want %v\n%s", strings.Join(c.words, " "), got, want[i], out)
		}
		assertNoToken(t, app)
	}
}

// --fields is checked against the shape the CLI really prints and with
// the CLI's own projection (review round 2): a path never runs through an
// array but the entries of a list, and outputs the CLI builds itself are
// checked against their contract in docs/schemas (dry-run, deliver,
// delete, the partial result of voucher create --file) or Go type (the
// pdf message {path, bytes}). Per case: lines that pass, lines that fail.
func TestDocFieldsFollowTheOutputShape(t *testing.T) {
	const voucher = `papierkram voucher create --name X --provenance domestic --item "name=a,amount=1,vat=19" --file beleg.pdf`
	const invoice = `papierkram invoice create --name X --customer 1 --payment-term 2 --document-date 2026-11-04 --item "name=a,qty=1,price=1,vat=19"`
	for _, tc := range []struct {
		name      string
		good, bad []string
	}{
		{"array", // output.lookup reads objects only
			[]string{"papierkram invoice describe 459 --fields id,line_items", "papierkram voucher describe 295 --fields documents",
				"papierkram company describe 1 --fields custom_attributes.foo.value", "papierkram transaction list --fields from.name,saldo.value"},
			[]string{"papierkram invoice describe 459 --fields id,line_items.name", "papierkram voucher describe 295 --fields documents.id"}},
		{"dry-run",
			[]string{invoice + " --dry-run --fields method,url,credits,body.name", "papierkram company delete 5 --dry-run --fields dry_run,url"},
			[]string{invoice + " --dry-run --fields id", "papierkram company delete 5 --dry-run --fields deleted"}},
		{"pdf",
			[]string{"papierkram invoice pdf 459 -O r.pdf --fields path,bytes", "papierkram invoice pdf 459 --output-file=r.pdf --fields bytes"},
			[]string{"papierkram invoice pdf 459 -O r.pdf --fields path,size", "papierkram invoice pdf 459 -O - --fields path", "papierkram invoice pdf 459 --fields path"}},
		{"partial", // voucher create --file: the voucher, or partial.json on exit 11
			[]string{voucher + " --fields id,name,completed,retry_command,warnings"},
			[]string{voucher + " --fields id,bogus"}},
		{"deliver",
			[]string{"papierkram invoice deliver 459 --via pdf --yes --fields id,invoice_no,sent_on,delivered,warnings"},
			[]string{"papierkram invoice deliver 459 --via pdf --yes --fields id,total_gross"}},
		{"delete",
			[]string{"papierkram company delete 5 --yes --fields id,deleted"},
			[]string{"papierkram company delete 5 --yes --fields id,name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, line := range append(append([]string{}, tc.good...), tc.bad...) {
				words, err := shellWords(line)
				if err != nil {
					t.Fatal(err)
				}
				err = docCommandProblem(t, words)
				if slices.Contains(tc.good, line) && err != nil {
					t.Errorf("%q: %v", line, err)
				} else if slices.Contains(tc.bad, line) && err == nil {
					t.Errorf("%q gilt als gültig", line)
				}
			}
		})
	}
}
