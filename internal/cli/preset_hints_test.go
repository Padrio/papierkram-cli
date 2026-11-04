package cli

import (
	"strings"
	"testing"
)

// Review M5: an invalid environment preset is a usage error on create
// too, before any request; an explicit --project does not read it.
func TestTaskCreateInvalidEnvPreset(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "abc"})
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support"}), 2)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "PAPIERKRAM_PROJECT") {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "abc"})
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--project", "5", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":5}}`)
}

// Review M6: a missing project on create names the preset sources
// (exit 5 stays).
func TestTaskCreateMissingProjectNamesPresetSources(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support"}), 5)
	wantRequests(t, srv, 0)
	_, errs := bufs(app)
	for _, want := range []string{"project", "--project", "PAPIERKRAM_PROJECT", ".papierkram.toml"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr ohne %q: %s", want, errs)
		}
	}
}

// Review M7: the preset hint only appears once all flags and the body are
// valid; a failing command never claims a project was chosen.
func TestPresetHintOnlyAfterValidation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
	}{
		{[]string{"task", "list", "--proposition", "-1"}, 2},
		{[]string{"task", "list", "--limit", "0"}, 2},
		{[]string{"task", "create", "--name", "T", "--deadline", "15.12.2026"}, 5},
		{[]string{"task", "create", "--name", "T", "--complete=vielleicht"}, 5},
	} {
		app, srv := apiApp(t)
		writeProjectFile(t, app, "project = 34\n")
		wantExit(t, app, Run(app, tc.args), tc.exit)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); strings.Contains(errs, "Hinweis: Projekt") {
			t.Errorf("%v: Hinweis trotz Fehler: %s", tc.args, errs)
		}
	}
}
