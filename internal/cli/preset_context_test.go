package cli

import (
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// twoWritableCtx are two writable contexts; the mock answers both
// (PAPIERKRAM_BASE_URL), the tokens are in the keyring.
const twoWritableCtx = "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\n[contexts.b]\nsubdomain = \"firma-b\"\n"

// contextApp is a mock app with the contexts a and b; env is added to the
// environment (PAPIERKRAM_BASE_URL is set by mockApp).
func contextApp(t *testing.T, env map[string]string) (*App, *mockapi.Server) {
	t.Helper()
	app, srv := mockApp(t, env)
	writeCfg(t, app, twoWritableCtx)
	for _, ctx := range []string{"a", "b"} {
		if err := app.Keyring.Set(ctx, "test-token-"+ctx); err != nil {
			t.Fatal(err)
		}
	}
	return app, srv
}

// Controller ruling (review M1): a project file bound to context a must
// not preset IDs while --context or PAPIERKRAM_CONTEXT choose another
// context; its values belong to another account. A warning names both
// contexts; environment presets still apply.
func TestPresetIgnoresProjectFileOfOtherContext(t *testing.T) {
	const file = "context = \"a\"\nproject = 34\n"
	wantIgnored := func(t *testing.T, app *App, how string) {
		t.Helper()
		_, errs := bufs(app)
		for _, want := range []string{`Warnung: Projektdatei `, `gilt für Kontext "a"`, `Kontext "b" (` + how + `)`, "ignoriert"} {
			if !strings.Contains(errs, want) {
				t.Errorf("stderr ohne %q: %s", want, errs)
			}
		}
		if strings.Contains(errs, "Hinweis: Projekt 34") {
			t.Errorf("Projekt 34 trotz fremdem Kontext verwendet: %s", errs)
		}
		assertNoToken(t, app)
	}
	for _, sel := range []struct {
		how  string
		args []string
		env  map[string]string
	}{
		{"--context", []string{"--context", "b"}, nil},
		{"PAPIERKRAM_CONTEXT", nil, map[string]string{"PAPIERKRAM_CONTEXT": "b"}},
	} {
		t.Run(sel.how, func(t *testing.T) {
			// task create: never into project 34 of account b
			app, srv := contextApp(t, sel.env)
			writeProjectFile(t, app, file)
			wantExit(t, app, Run(app, append([]string{"task", "create", "--name", "Support"}, sel.args...)), 5)
			wantRequests(t, srv, 0)
			wantIgnored(t, app, sel.how)

			// task list: no project filter
			app, srv = contextApp(t, sel.env)
			writeProjectFile(t, app, file)
			wantExit(t, app, Run(app, append([]string{"task", "list", "-o", "json"}, sel.args...)), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != "page_size=25" {
				t.Errorf("task list: query = %s", r.Query.Encode())
			}
			wantIgnored(t, app, sel.how)

			// project describe without ID: no preset, exit 2
			app, srv = contextApp(t, sel.env)
			writeProjectFile(t, app, file)
			wantExit(t, app, Run(app, append([]string{"project", "describe"}, sel.args...)), 2)
			wantRequests(t, srv, 0)
			wantIgnored(t, app, sel.how)
		})
	}

	// environment presets still apply, the project file does not decide
	app, srv := contextApp(t, map[string]string{"PAPIERKRAM_PROJECT": "7"})
	writeProjectFile(t, app, file)
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--context", "b", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":7}}`)
	if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt 7 aus PAPIERKRAM_PROJECT") || strings.Contains(errs, "ignoriert") {
		t.Errorf("stderr = %q", errs)
	}

	// the same context, or a project file without context: preset applies
	for _, tc := range []struct{ file, ctx string }{
		{file, "a"},
		{"project = 34\n", "b"},
	} {
		app, srv := contextApp(t, nil)
		writeProjectFile(t, app, tc.file)
		wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--context", tc.ctx, "-o", "json"}), 0)
		wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":34}}`)
		if _, errs := bufs(app); !strings.Contains(errs, "Hinweis: Projekt 34 aus Projektdatei") || strings.Contains(errs, "ignoriert") {
			t.Errorf("%q --context %s: stderr = %q", tc.file, tc.ctx, errs)
		}
	}
}

// Review N1: flag errors come before the configuration is resolved for a
// preset (plan order): exit 2, no request, no context warning, whatever
// the configuration or project file say.
func TestPresetResolvedAfterFlagChecks(t *testing.T) {
	for _, tc := range []struct {
		name, cfg, file string
		args            []string
	}{
		{"context missing from config", twoWritableCtx, "context = \"c\"\nproject = 34\n", nil},
		{"broken config", "active_context = \n", "context = \"a\"\nproject = 34\n", nil},
		{"other context", twoWritableCtx, "context = \"a\"\nproject = 34\n", []string{"--context", "b"}},
	} {
		for _, flags := range [][]string{
			{"--proposition", "-1"},
			{"--proposition", "abc"},
			{"--limit", "0"},
		} {
			app, srv := mockApp(t, nil)
			writeCfg(t, app, tc.cfg)
			writeProjectFile(t, app, tc.file)
			args := append(append([]string{"task", "list"}, flags...), tc.args...)
			wantExit(t, app, Run(app, args), 2)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); strings.Contains(errs, "Warnung") || strings.Contains(errs, "Kontext") {
				t.Errorf("%s %v: stderr = %q", tc.name, flags, errs)
			}
		}
	}

	// create: a body that fails coercion is reported without the context
	// warning (the preset is applied right before validation)
	app, srv := contextApp(t, nil)
	writeProjectFile(t, app, "context = \"a\"\nproject = 34\n")
	app.In = strings.NewReader(`{"name":"Support","complete":"vielleicht"}`)
	wantExit(t, app, Run(app, []string{"task", "create", "--input", "-", "--context", "b"}), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); strings.Contains(errs, "Projektdatei") {
		t.Errorf("create: stderr = %q", errs)
	}
}

// Controller ruling (open case of M1): project file presets apply only
// when the effective context equals the file's context. In direct mode
// (environment token, the file's context missing from the config) no
// context is active: the presets are ignored with a warning.
func TestPresetIgnoresProjectFileWithoutActiveContext(t *testing.T) {
	const file = "context = \"a\"\nproject = 34\n"
	wantIgnored := func(t *testing.T, app *App) {
		t.Helper()
		_, errs := bufs(app)
		for _, want := range []string{`Warnung: Projektdatei `, `gilt für Kontext "a"`, "kein Kontext aktiv", "ignoriert"} {
			if !strings.Contains(errs, want) {
				t.Errorf("stderr ohne %q: %s", want, errs)
			}
		}
		if strings.Contains(errs, "Hinweis: Projekt 34") {
			t.Errorf("Projekt 34 ohne aktiven Kontext verwendet: %s", errs)
		}
		assertNoToken(t, app)
	}

	app, srv := apiApp(t) // PAPIERKRAM_TOKEN + PAPIERKRAM_BASE_URL, no config
	writeProjectFile(t, app, file)
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support"}), 5)
	wantRequests(t, srv, 0)
	wantIgnored(t, app)

	app, srv = apiApp(t)
	writeProjectFile(t, app, file)
	wantExit(t, app, Run(app, []string{"task", "list", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != "page_size=25" {
		t.Errorf("task list: query = %s", r.Query.Encode())
	}
	wantIgnored(t, app)

	app, srv = apiApp(t)
	writeProjectFile(t, app, file)
	wantExit(t, app, Run(app, []string{"project", "describe"}), 2)
	wantRequests(t, srv, 0)
	wantIgnored(t, app)

	// direct mode with the file's context in the config: that context is
	// effective, the presets apply
	app, srv = contextApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"})
	writeProjectFile(t, app, file)
	wantExit(t, app, Run(app, []string{"task", "list", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != "page_size=25&project_id=34" {
		t.Errorf("Kontext a vorhanden: query = %s", r.Query.Encode())
	}
	if _, errs := bufs(app); strings.Contains(errs, "ignoriert") {
		t.Errorf("Kontext a vorhanden: stderr = %q", errs)
	}

	// without direct mode a missing context is the configuration error it
	// always was, before any request
	app, srv = mockApp(t, nil)
	writeCfg(t, app, twoWritableCtx)
	writeProjectFile(t, app, "context = \"c\"\nproject = 34\n")
	wantExit(t, app, Run(app, []string{"task", "list"}), 4)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, `Kontext "c" nicht gefunden`) {
		t.Errorf("stderr = %q", errs)
	}
}
