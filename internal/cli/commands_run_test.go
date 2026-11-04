package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// The contract against the running CLI: commands are run with their own
// examples (against the mock, mutations with --dry-run) and the reported
// flag types and read-only blocking are checked by what the CLI does.

// runEnv is the environment of the runs: token, mock and a user preset
// (time log, time start need one).
func runLine(t *testing.T, env map[string]string, words []string) (int, string) {
	t.Helper()
	all := map[string]string{"PAPIERKRAM_TOKEN": "test-token-run", "PAPIERKRAM_USER": "1"}
	for k, v := range env {
		all[k] = v
	}
	app, _ := mockApp(t, all)
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
				_, errs := runLine(t, nil, append(append([]string{}, base...), flag+"=abc"))
				if strings.Contains(errs, flag) && strings.Contains(errs, "Zahl") {
					t.Errorf("%s %s=abc (string): als Zahl abgelehnt: %q", c.Command, flag, errs)
				}
				continue
			}
			s := typeSamples[f.Type]
			code, errs := runLine(t, nil, append(append([]string{}, base...), flag+"="+s.bad))
			if (code != 2 && code != 5) || !strings.Contains(errs, flag) {
				t.Errorf("%s %s=%s (%s): exit %d, stderr %q – nicht abgelehnt", c.Command, flag, s.bad, f.Type, code, errs)
			}
			_, errs = runLine(t, nil, append(append([]string{}, base...), flag+"="+s.good))
			if strings.Contains(errs, fmt.Sprintf("%q", s.good)) {
				t.Errorf("%s %s=%s (%s): abgelehnt: %q", c.Command, flag, s.good, f.Type, errs)
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
