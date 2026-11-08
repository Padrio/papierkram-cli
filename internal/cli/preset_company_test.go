package cli

import (
	"slices"
	"strings"
	"testing"
)

// Open point 27 (Task 9): PAPIERKRAM_COMPANY or company of the project
// file presets the path flag --company of the person commands, like the
// other presets: flag > environment > project file > none.

// personCases are the five person commands without --company, with the
// method and path of their request for company 7 (filled in by path).
var personCases = []struct {
	name   string
	args   []string
	method string
	path   func(company string) string
}{
	{"list", []string{"person", "list"}, "GET", func(c string) string { return "/contact/companies/" + c + "/persons" }},
	{"create", []string{"person", "create", "--first-name", "Anna", "--last-name", "Groß"}, "POST", func(c string) string { return "/contact/companies/" + c + "/persons" }},
	{"describe", []string{"person", "describe", "5"}, "GET", func(c string) string { return "/contact/companies/" + c + "/persons/5" }},
	{"update", []string{"person", "update", "5", "--position", "Einkauf"}, "PUT", func(c string) string { return "/contact/companies/" + c + "/persons/5" }},
	{"delete", []string{"person", "delete", "5", "--yes"}, "DELETE", func(c string) string { return "/contact/companies/" + c + "/persons/5" }},
}

func TestPersonCompanyPreset(t *testing.T) {
	for _, tc := range personCases {
		t.Run(tc.name, func(t *testing.T) {
			// environment
			app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "7"})
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "-o", "json")), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Method != tc.method || r.Path != tc.path("7") {
				t.Errorf("env: request = %s %s", r.Method, r.Path)
			}
			if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Firma 7 aus PAPIERKRAM_COMPANY\n") {
				t.Errorf("env: stderr = %q", errs)
			}

			// project file
			app, srv = apiApp(t)
			writeProjectFile(t, app, "company = 8\n")
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "-o", "json")), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Method != tc.method || r.Path != tc.path("8") {
				t.Errorf("projectfile: request = %s %s", r.Method, r.Path)
			}
			if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Firma 8 aus Projektdatei ") {
				t.Errorf("projectfile: stderr = %q", errs)
			}

			// environment beats the project file
			app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "7"})
			writeProjectFile(t, app, "company = 8\n")
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "-o", "json")), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Path != tc.path("7") {
				t.Errorf("env > projectfile: path = %s", r.Path)
			}

			// the flag wins and reads neither (an invalid environment
			// value does not matter), no hint
			app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "abc"})
			writeProjectFile(t, app, "company = 8\n")
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "--company", "9", "-o", "json")), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Path != tc.path("9") {
				t.Errorf("flag: path = %s", r.Path)
			}
			if _, errs := bufs(app); strings.Contains(errs, "Hinweis: Firma") {
				t.Errorf("flag: stderr = %q", errs)
			}
			assertNoToken(t, app)
		})
	}
}

// Without any value the command stays a usage error (exit 2) that names
// the preset sources; an invalid environment value is exit 2 like the
// other presets; neither sends a request.
func TestPersonCompanyPresetErrors(t *testing.T) {
	for _, tc := range personCases {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := apiApp(t)
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "-o", "json")), 2)
			wantRequests(t, srv, 0)
			_, errs := bufs(app)
			for _, want := range []string{
				"Fehler: Pflichtangabe --company fehlt\n",
				"Hinweis: --company angeben oder voreinstellen: PAPIERKRAM_COMPANY oder company in .papierkram.toml (papierkram local init --company <id>)\n",
				`"code":"usage"`,
			} {
				if !strings.Contains(errs, want) {
					t.Errorf("fehlt: stderr ohne %q: %s", want, errs)
				}
			}

			app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "abc"})
			writeProjectFile(t, app, "company = 8\n")
			wantExit(t, app, Run(app, tc.args), 2)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, `ungültiger Wert "abc" für PAPIERKRAM_COMPANY`) || strings.Contains(errs, "Hinweis: Firma") {
				t.Errorf("ungültig: stderr = %q", errs)
			}

			app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "-3"})
			wantExit(t, app, Run(app, tc.args), 2)
			wantRequests(t, srv, 0)
		})
	}
}

// The project file of another context presets nothing: warning once,
// then the missing --company (exit 2), no request.
func TestPersonCompanyPresetOtherContext(t *testing.T) {
	for _, tc := range personCases {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := contextApp(t, nil)
			writeProjectFile(t, app, "context = \"a\"\ncompany = 8\n")
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "--context", "b")), 2)
			wantRequests(t, srv, 0)
			_, errs := bufs(app)
			for _, want := range []string{`Warnung: Projektdatei `, `gilt für Kontext "a"`, `Kontext "b" (--context)`, "ignoriert", "Pflichtangabe --company fehlt"} {
				if !strings.Contains(errs, want) {
					t.Errorf("stderr ohne %q: %s", want, errs)
				}
			}
			if n := strings.Count(errs, "Warnung: Projektdatei"); n != 1 {
				t.Errorf("Warnung %d-mal: %s", n, errs)
			}
			if strings.Contains(errs, "Hinweis: Firma 8") {
				t.Errorf("Firma 8 trotz fremdem Kontext: %s", errs)
			}

			// the same context: the file presets
			app, srv = contextApp(t, nil)
			writeProjectFile(t, app, "context = \"a\"\ncompany = 8\n")
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "-o", "json")), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Path != tc.path("8") {
				t.Errorf("Kontext a: path = %s", r.Path)
			}
		})
	}
}

// The company preset does not make the person ID optional: only a
// describe whose preset is its own resource (project describe) takes the
// ID from the preset.
func TestPersonDescribeWithoutIDStaysUsageError(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "7"})
	wantExit(t, app, Run(app, []string{"person", "describe"}), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); strings.Contains(errs, "Hinweis: Firma") {
		t.Errorf("stderr = %q", errs)
	}
	app, _ = apiApp(t)
	wantExit(t, app, Run(app, []string{"person", "describe", "--help"}), 0)
	if out := mustOut(app); !strings.Contains(out, "person describe <id>") || strings.Contains(out, "[<id>]") {
		t.Errorf("Hilfe:\n%s", out)
	}

	// project describe without ID: unchanged
	app, srv = apiApp(t)
	writeProjectFile(t, app, "company = 8\nproject = 34\n")
	wantExit(t, app, Run(app, []string{"project", "describe", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/projects/34" {
		t.Errorf("project describe: path = %s", r.Path)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt 34 aus Projektdatei") || strings.Contains(errs, "Firma") {
		t.Errorf("project describe: stderr = %q", errs)
	}
}

// --dry-run shows the request with the preset company and prints the
// hint; read-only stays exit 9 without a request, but an invalid preset
// is reported first (exit 2, like presetArgs before the gate).
func TestPersonCompanyPresetDryRunAndReadOnly(t *testing.T) {
	for _, tc := range personCases {
		if tc.method == "GET" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "7"})
			wantExit(t, app, Run(app, append(slices.Clone(tc.args), "--dry-run", "-o", "json")), 0)
			wantRequests(t, srv, 0)
			out, errs := bufs(app)
			if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, tc.path("7")+`"`) || !strings.Contains(out, `"method": "`+tc.method+`"`) {
				t.Errorf("stdout:\n%s", out)
			}
			if !strings.Contains(errs, "Hinweis: Firma 7 aus PAPIERKRAM_COMPANY") {
				t.Errorf("stderr = %q", errs)
			}

			ro := map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1", "PAPIERKRAM_COMPANY": "7"}
			for _, extra := range [][]string{nil, {"--dry-run"}} {
				app, srv = mockApp(t, ro)
				wantExit(t, app, Run(app, append(slices.Clone(tc.args), extra...)), 9)
				wantRequests(t, srv, 0)
				if _, errs := bufs(app); strings.Contains(errs, "Hinweis: Firma") {
					t.Errorf("read-only %v: Hinweis trotz Exit 9: %s", extra, errs)
				}
			}

			ro["PAPIERKRAM_COMPANY"] = "abc"
			app, srv = mockApp(t, ro)
			wantExit(t, app, Run(app, tc.args), 2)
			wantRequests(t, srv, 0)
		})
	}
	// reading stays allowed under read-only, with the preset
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1", "PAPIERKRAM_COMPANY": "7"})
	wantExit(t, app, Run(app, []string{"person", "list", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/contact/companies/7/persons" {
		t.Errorf("path = %s", r.Path)
	}
}

// Like the other presets (review N1, M7): flag errors come before the
// preset is resolved, and a failing command never names the preset.
func TestPersonCompanyPresetAfterFlagChecks(t *testing.T) {
	for _, args := range [][]string{
		{"person", "list", "--limit", "0"},
		{"person", "list", "--page", "-1"},
		{"person", "create", "--input", ""},
		{"person", "update", "5", "--input", ""},
	} {
		app, srv := contextApp(t, nil)
		writeProjectFile(t, app, "context = \"a\"\ncompany = 8\n")
		wantExit(t, app, Run(app, append(slices.Clone(args), "--context", "b")), 2)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); strings.Contains(errs, "Pflichtangabe --company") {
			t.Errorf("%v: Preset vor Flag-Fehler geprüft: %s", args, errs)
		}
		if _, errs := bufs(app); strings.Contains(errs, "Warnung") || strings.Contains(errs, "Hinweis: Firma") {
			t.Errorf("%v: stderr = %q", args, errs)
		}
	}

	// review T9-M2: a body error is no flag error. The preset path flag
	// is resolved before the body is built (runWriteThen), so person
	// create without --last-name (exit 5 with a company) in another
	// context warns about the project file and fails on the missing
	// --company (exit 2), nothing sent.
	args := []string{"person", "create", "--first-name", "Anna", "-o", "json"}
	app, srv := contextApp(t, nil)
	writeProjectFile(t, app, "context = \"a\"\ncompany = 8\n")
	wantExit(t, app, Run(app, append(slices.Clone(args), "--context", "a")), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `Fehler: Pflichtfeld "last_name" fehlt`) {
		t.Errorf("Kontext a: stderr = %q", errs)
	}
	app, srv = contextApp(t, nil)
	writeProjectFile(t, app, "context = \"a\"\ncompany = 8\n")
	wantExit(t, app, Run(app, append(slices.Clone(args), "--context", "b")), 2)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	for _, want := range []string{`gilt für Kontext "a"`, "Fehler: Pflichtangabe --company fehlt\n", `"code":"usage"`} {
		if !strings.Contains(errs, want) {
			t.Errorf("Kontext b: stderr ohne %q: %s", want, errs)
		}
	}
	if n := strings.Count(errs, "Warnung: Projektdatei"); n != 1 || strings.Contains(errs, "last_name") || strings.Contains(errs, "Hinweis: Firma") {
		t.Errorf("Kontext b: stderr = %q", errs)
	}
}

// Review T9-M1: the preset hint is printed before the confirmation, so
// person delete without --yes (exit 8, nothing sent) names the company it
// would have used.
func TestPersonDeletePresetHintBeforeConfirmation(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_COMPANY": "7"})
	wantExit(t, app, Run(app, []string{"person", "delete", "5", "-o", "json", "--no-input"}), 8)
	wantRequests(t, srv, 0)
	out, errs := bufs(app)
	if out != "" {
		t.Errorf("stdout = %q", out)
	}
	hint := strings.Index(errs, "Hinweis: Firma 7 aus PAPIERKRAM_COMPANY\n")
	confirm := strings.Index(errs, "Fehler: Bestätigung erforderlich: Kontaktperson 5 löschen\n")
	if hint < 0 || confirm < 0 || hint > confirm || !strings.Contains(errs, `"code":"confirmation_required"`) {
		t.Errorf("stderr = %q", errs)
	}
	if n := strings.Count(errs, "Hinweis: Firma"); n != 1 {
		t.Errorf("Hinweis %d-mal: %s", n, errs)
	}
}

// commands --json names where --company of the person commands comes
// from; it stays required (like time log --task).
func TestCommandsJSONPersonCompanyPreset(t *testing.T) {
	d := decodeCommands(t, runCommandsJSON(t, "--json"))
	for _, name := range []string{"person list", "person create", "person describe", "person update", "person delete"} {
		f := d.find(t, name).flag(t, "company")
		if !f.Required || f.Type != "id" || !slices.Equal(f.DefaultFrom, []string{"flag", "env", "projectfile"}) || f.Env != "PAPIERKRAM_COMPANY" {
			t.Errorf("%s --company = %+v", name, f)
		}
		if f.Description != "ID des übergeordneten Objekts (company_id); ohne Angabe PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (Pflicht)" {
			t.Errorf("%s --company: Beschreibung %q", name, f.Description)
		}
	}
}

// local show and local init say which commands use company.
func TestLocalHelpNamesCompanyUse(t *testing.T) {
	for _, args := range [][]string{{"local", "show", "--help"}, {"local", "init", "--help"}} {
		app, _ := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		if out := mustOut(app); !strings.Contains(out, "genutzt von person … --company") {
			t.Errorf("%v:\n%s", args, out)
		}
	}
}
