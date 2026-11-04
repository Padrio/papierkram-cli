package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

func strPtr(s string) *string { return &s }

// Spec 9.8: flag > environment > project file > context default; the
// result names its source and origin (local show, Task 36).
func TestResolvePresetOrder(t *testing.T) {
	file := &projectfile.File{Path: "/p/.papierkram.toml", Project: 34, Task: 56, User: 3, Tasks: map[string]int64{"tickets": 57}}
	load := func() (*projectfile.File, error) { return file, nil }
	env := envMap(map[string]string{"PAPIERKRAM_PROJECT": "7", "PAPIERKRAM_TASK": "tickets"})
	none := envMap(nil)
	for _, tc := range []struct {
		name string
		key  string
		src  presetSources
		want preset
	}{
		{"flag beats all", "project", presetSources{Flag: strPtr("5"), Env: env, File: load}, preset{5, "flag", "--project"}},
		{"env beats file", "project", presetSources{Env: env, File: load}, preset{7, "env", "PAPIERKRAM_PROJECT"}},
		{"file", "project", presetSources{Env: none, File: load}, preset{34, "projectfile", "/p/.papierkram.toml"}},
		{"file beats context", "user", presetSources{Env: none, File: load, Context: 9, ContextName: "firma"}, preset{3, "projectfile", "/p/.papierkram.toml"}},
		{"context", "user", presetSources{Env: none, Context: 9, ContextName: "firma"}, preset{9, "context", "firma"}},
		{"nothing", "company", presetSources{Env: none, File: load}, preset{}},
		{"no project file", "project", presetSources{Env: none, File: func() (*projectfile.File, error) { return nil, nil }}, preset{}},
		{"id 0 is a value (R24)", "project", presetSources{Flag: strPtr("0")}, preset{0, "flag", "--project"}},
		{"task alias from env", "task", presetSources{Env: env, File: load}, preset{57, "env", "PAPIERKRAM_TASK"}},
		{"task alias from flag", "task", presetSources{Flag: strPtr("tickets"), File: load}, preset{57, "flag", "--task"}},
		{"task id from flag", "task", presetSources{Flag: strPtr("12"), File: load}, preset{12, "flag", "--task"}},
	} {
		got, err := resolvePreset(tc.key, tc.src)
		if err != nil || got != tc.want {
			t.Errorf("%s: resolvePreset(%s) = %+v, %v; want %+v", tc.name, tc.key, got, err, tc.want)
		}
	}
}

func TestResolvePresetErrors(t *testing.T) {
	broken := errors.New("kaputt")
	for _, tc := range []struct {
		name, key string
		src       presetSources
		exit      int
		want      string
	}{
		{"invalid env", "project", presetSources{Env: envMap(map[string]string{"PAPIERKRAM_PROJECT": "abc"})}, 2, "PAPIERKRAM_PROJECT"},
		{"negative flag", "project", presetSources{Flag: strPtr("-3")}, 2, "--project"},
		{"unknown task alias", "task", presetSources{Flag: strPtr("gibtsnicht"),
			File: func() (*projectfile.File, error) {
				return &projectfile.File{Tasks: map[string]int64{"tickets": 57}}, nil
			}}, 2, "tickets"},
		{"broken project file", "project", presetSources{File: func() (*projectfile.File, error) { return nil, broken }}, 1, "kaputt"},
	} {
		_, err := resolvePreset(tc.key, tc.src)
		ce := clierr.From(err)
		if err == nil || ce.Exit() != tc.exit || !strings.Contains(ce.Message+" "+ce.Hint, tc.want) {
			t.Errorf("%s: err = %v (exit %d), want exit %d mentioning %q", tc.name, err, ce.Exit(), tc.exit, tc.want)
		}
	}
	// the environment wins, so a broken project file is not read for it
	p, err := resolvePreset("project", presetSources{Env: envMap(map[string]string{"PAPIERKRAM_PROJECT": "7"}),
		File: func() (*projectfile.File, error) { return nil, broken }})
	if err != nil || p.ID != 7 {
		t.Errorf("env with broken file: %+v, %v", p, err)
	}
}
