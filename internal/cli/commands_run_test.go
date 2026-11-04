package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/projectfile"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// The contract against the running CLI: commands are run with their own
// examples (against the mock, mutations with --dry-run) and the reported
// flag types and read-only blocking are checked by what the CLI does.

// runFixtures are the input files the examples name (voucher upload,
// voucher create --file, --input, --items, --body-file, the task alias of
// the project file); every run gets them in a fresh directory. The JSON
// bodies are valid create bodies; update drops their line items without
// --replace-items (with a hint).
var runFixtures = map[string]string{
	"beleg.pdf":       pdfContent,
	"quittung.jpg":    "\xff\xd8\xff\xe0\x00\x10JFIF\x00",
	"mail.txt":        "Guten Tag,\n\nanbei {{rechnung.rechnungsnummer}}.\n",
	"positionen.json": `[{"name":"Wartung","quantity":1,"unit":"Monat","price":250,"vat_rate":0.19}]`,
	"firma.json":      `{"name":"Muster GmbH","contact_type":"customer","email":"info@example.org"}`,
	"datenfeld.json":  `{"name":"Kostenstelle"}`,
	"angebot.json":    `{"name":"Website","customer":{"id":3052},"line_items":[{"name":"Konzept","quantity":2,"unit":"Tag","price":800,"vat_rate":0.19}]}`,
	"rechnung.json":   `{"name":"Wartung","customer":{"id":3214},"payment_term":{"id":1130},"line_items":[{"name":"Wartung","quantity":1,"unit":"Monat","price":250,"vat_rate":0.19}]}`,
	"projekt.json":    `{"name":"Relaunch","customer":{"id":3214}}`,
	"artikel.json":    `{"name":"Beratung","article_no":"B-01","time_unit":"hour","proposition_type":"service","price":"100","vat_rate":"0.19"}`,
	"aufgabe.json":    `{"name":"Konzept","project":{"id":4963}}`,
	"zeit.json":       `{"entry_date":"2026-11-03","started_at_time":"09:00","ended_at_time":"10:30","task":{"id":545},"user":{"id":1}}`,
	"beleg.json":      `{"name":"Büromaterial","provenance":"domestic","line_items":[{"name":"Papier","amount":11.9,"vat_rate":0.19,"category":"Bürobedarf"}]}`,
	// task and the alias "tickets" of the examples (time log, time start,
	// local init); no context, so it presets in every context
	projectfile.FileName: "task = 545\n\n[tasks]\ntickets = 57\n",
}

// runLine runs one command line in a fresh working directory with the
// fixtures, so output files (-O beleg.pdf) never land in the package
// directory and no run sees files of another. The environment has token,
// mock and a user preset (time log, time start need one).
func runLine(t *testing.T, env map[string]string, words []string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range runFixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	all := map[string]string{"PAPIERKRAM_TOKEN": "test-token-run", "PAPIERKRAM_USER": "1"}
	for k, v := range env {
		all[k] = v
	}
	app, _ := mockApp(t, all)
	// the project file of the fixtures is found in the run directory
	app.Getwd = func() (string, error) { return dir, nil }
	code := Run(app, words)
	assertNoToken(t, app)
	_, errs := bufs(app)
	return code, errs
}

// exampleRuns returns the examples of c as argument lists that run
// (exit 0): mutations get --dry-run. Examples that need files or a
// terminal do not run here and are left out.
func exampleRuns(t *testing.T, c cmdInfo) [][]string {
	t.Helper()
	var out [][]string
	for _, ex := range c.Examples {
		words, err := shellWords(ex)
		if err != nil {
			t.Fatalf("%s: %v", ex, err)
		}
		words = words[1:]
		if dryRunnable(c) {
			words = append(words, "--dry-run")
		}
		if code, _ := runLine(t, nil, words); code == 0 {
			out = append(out, words)
		}
	}
	return out
}

// dryRunnable: commands whose class is a registry mutation class.
func dryRunnable(c cmdInfo) bool {
	switch registry.Class(c.Class) {
	case registry.ClassCreate, registry.ClassUpdate, registry.ClassArchive, registry.ClassDestructive, registry.ClassSend:
		return true
	}
	return false
}

// valueRejected reports whether a run rejected the value of flag: a usage
// or validation error (exit 2 or 5) whose message calls the value of that
// flag invalid ("ungültiger Wert … für --flag", "ungültige ID … für
// --flag", "--flag: ungültig…"), whatever the value. Other errors that
// name the flag (--desc only with --sort) are no rejection of the value.
func valueRejected(code int, errs, flag string) bool {
	if code != 2 && code != 5 {
		return false
	}
	name := `(-[A-Za-z], )?` + regexp.QuoteMeta(flag) + `([^a-z0-9-]|$)`
	return regexp.MustCompile(`ungültig[^\n]*? für `+name).MatchString(errs) ||
		regexp.MustCompile(`(?m)^Fehler: `+name+`[^\n]*ungültig`).MatchString(errs)
}

func TestValueRejected(t *testing.T) {
	for _, tc := range []struct {
		code       int
		errs, flag string
		want       bool
	}{
		{2, "Fehler: ungültiger Wert \"abc\" für --company: ungültige Syntax\n", "--company", true},
		{5, "Fehler: ungültiger Wert für --team-member: ID muss eine ganze Zahl ≥ 0 sein\n", "--team-member", true}, // value not quoted
		{2, "Fehler: ungültige ID \"x\" für --from-estimate: ganze Zahl ≥ 0 erwartet\n", "--from-estimate", true},
		{2, "Fehler: ungültiger Wert \"x\" für -O, --output-file: …\n", "--output-file", true},
		{5, "Fehler: --date: ungültiges Datum \"31.02.2026\"\n", "--date", true},
		{2, "Fehler: ungültiger Wert \"x\" für --company-id: …\n", "--company", false},
		{2, "Fehler: --desc nur zusammen mit --sort\n", "--desc", false},
		{1, "Fehler: ungültiger Wert \"x\" für --company\n", "--company", false},
		{0, "", "--company", false},
	} {
		if got := valueRejected(tc.code, tc.errs, tc.flag); got != tc.want {
			t.Errorf("valueRejected(%d, %q, %s) = %v", tc.code, tc.errs, tc.flag, got)
		}
	}
}

// typeSamples are a value of each contract type the CLI must reject and
// one it must accept.
var typeSamples = map[string]struct{ bad, good string }{
	"id":       {"abc", "7"},
	"integer":  {"x1", "7"},
	"number":   {"abc", "7.5"},
	"date":     {"31.02.2026", "2026-01-15"},
	"boolean":  {"vielleicht", "true"},
	"duration": {"zehn", "45s"},
}

// Every typed flag of every command: a value of another type is rejected
// naming the flag (exit 2 or 5), a value of the reported type is not
// rejected. ID flags are all sampled; flags of commands without a
// runnable example are listed in noRun.
func TestCommandsJSONFlagTypesMatchCLI(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	// commands without an example that runs here, with the reason
	noRun := map[string]string{
		"auth login": "verlangt ein Terminal oder ein Token auf stdin",
		"time stop":  "verlangt einen laufenden Timer",
	}
	idFlags := 0
	for _, c := range d.Commands {
		var typed []flgInfo
		for _, f := range c.Flags {
			if _, ok := typeSamples[f.Type]; ok || (f.Type == "string" && len(f.Enum) == 0) {
				typed = append(typed, f)
			}
		}
		if len(typed) == 0 {
			continue
		}
		runs := exampleRuns(t, c)
		if len(runs) == 0 {
			if noRun[c.Command] == "" {
				t.Errorf("%s: kein Beispiel läuft, Flag-Typen ungeprüft", c.Command)
			}
			continue
		}
		base := runs[0]
		for _, f := range typed {
			if f.Type == "id" {
				idFlags++
			}
			flag := "--" + f.Name
			if f.Type == "string" {
				// text is accepted as text: no rejection as number or ID
				code, errs := runLine(t, nil, append(append([]string{}, base...), flag+"=abc"))
				if valueRejected(code, errs, flag) && strings.Contains(errs, "Zahl") {
					t.Errorf("%s %s=abc (string): als Zahl abgelehnt: %q", c.Command, flag, errs)
				}
				continue
			}
			s := typeSamples[f.Type]
			code, errs := runLine(t, nil, append(append([]string{}, base...), flag+"="+s.bad))
			if !valueRejected(code, errs, flag) {
				t.Errorf("%s %s=%s (%s): exit %d, stderr %q – nicht abgelehnt", c.Command, flag, s.bad, f.Type, code, errs)
			}
			code, errs = runLine(t, nil, append(append([]string{}, base...), flag+"="+s.good))
			if valueRejected(code, errs, flag) {
				t.Errorf("%s %s=%s (%s): abgelehnt: exit %d, %q", c.Command, flag, s.good, f.Type, code, errs)
			}
		}
	}
	// every ID flag is sampled (none is on a command of noRun)
	all := 0
	for _, c := range d.Commands {
		for _, f := range c.Flags {
			if f.Type == "id" {
				all++
			}
		}
	}
	if idFlags != all || all == 0 {
		t.Errorf("%d von %d ID-Flags geprüft", idFlags, all)
	}
}

// blocked_by_read_only says whether a command can exit 9 under
// PAPIERKRAM_READ_ONLY: it is derived from the guard annotations
// (pk:writes-config, a pk:class other than read_only) and checked by
// running every example of every command under read-only: a blocked
// command exits 9 with one of them, a free one with none and with exit 0
// for at least one.
func TestCommandsJSONReadOnlyBlocking(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	root.InitDefaultHelpCmd()
	for _, c := range d.Commands {
		if c.Blocked == nil {
			t.Fatalf("%s: blocked_by_read_only fehlt", c.Command)
		}
		cmd, _, err := root.Find(strings.Fields(c.Command))
		if err != nil {
			t.Fatal(err)
		}
		class := cmd.Annotations[annotClass]
		guarded := cmd.Annotations[annotWritesConfig] == "true" || (class != "" && class != string(registry.ClassReadOnly))
		if *c.Blocked != guarded {
			t.Errorf("%s: blocked_by_read_only %v, Schutz-Annotationen %v", c.Command, *c.Blocked, guarded)
		}
		exit9, exit0 := false, false
		for _, ex := range c.Examples {
			words, err := shellWords(ex)
			if err != nil {
				t.Fatal(err)
			}
			words = words[1:]
			if dryRunnable(c) {
				words = append(words, "--dry-run")
			}
			switch code, _ := runLine(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"}, words); code {
			case 9:
				exit9 = true
			case 0:
				exit0 = true
			}
		}
		if exit9 != *c.Blocked {
			t.Errorf("%s: blocked_by_read_only %v, unter PAPIERKRAM_READ_ONLY Exit 9: %v", c.Command, *c.Blocked, exit9)
		}
		// a free command really runs under read-only: some example ends
		// with exit 0, so "no exit 9" is not only the result of earlier
		// errors
		if !*c.Blocked && !exit0 {
			t.Errorf("%s: frei, aber kein Beispiel endet unter PAPIERKRAM_READ_ONLY mit Exit 0", c.Command)
		}
	}
}

// The alternatives of deliver --via hold at run time: --input with
// send_via replaces --via (--dry-run, exit 0), without both the command
// stops (exit 2, --via named), for invoice and estimate.
func TestDeliverViaAlternativeRuns(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, res := range []string{"invoice", "estimate"} {
		via := d.find(t, res+" deliver").flag(t, "via")
		if !via.Required || len(via.Alternatives) == 0 {
			t.Fatalf("%s deliver --via = %+v", res, via)
		}
		for _, alt := range via.Alternatives {
			if alt != "--input" {
				t.Fatalf("%s deliver --via: Alternative %s ohne Lauf-Test", res, alt)
			}
			body := filepath.Join(t.TempDir(), "deliver.json")
			if err := os.WriteFile(body, []byte(`{"send_via":"pdf"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if code, errs := runLine(t, nil, []string{res, "deliver", "5", alt, body, "--dry-run"}); code != 0 {
				t.Errorf("%s deliver %s ohne --via: exit %d, %q", res, alt, code, errs)
			}
		}
		if code, errs := runLine(t, nil, []string{res, "deliver", "5", "--dry-run"}); code != 2 || !strings.Contains(errs, "--via") {
			t.Errorf("%s deliver ohne --via und Alternative: exit %d, %q", res, code, errs)
		}
	}
}

// Every example of a mutation runs with --dry-run against the mock and
// ends with exit 0 (task 28 M2): the examples are not only parseable but
// valid requests (body checked against the request schema, references
// and files present). Examples of time stop need a running timer and are
// covered by its own tests.
func TestMutationExamplesDryRun(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	checked := 0
	for _, c := range d.Commands {
		if !dryRunnable(c) || c.Command == "time stop" {
			continue
		}
		for _, ex := range c.Examples {
			words, err := shellWords(ex)
			if err != nil {
				t.Fatalf("%s: %v", ex, err)
			}
			words = words[1:]
			if !slices.Contains(words, "--dry-run") {
				words = append(words, "--dry-run")
			}
			checked++
			if code, errs := runLine(t, nil, words); code != 0 {
				t.Errorf("%s: exit %d, stderr %q", strings.Join(words, " "), code, errs)
			}
		}
	}
	if checked == 0 {
		t.Fatal("keine Beispiele geprüft")
	}
}
