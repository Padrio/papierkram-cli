package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/projectfile"
)

// local init|show (spec 9.8, 8.3): the project file .papierkram.toml.

// localApp is a test app whose working directory is a fresh, empty
// directory (returned).
func localApp(t *testing.T) (*App, string) {
	t.Helper()
	app, _, _ := newTestApp(t)
	dir := t.TempDir()
	app.Getwd = func() (string, error) { return dir, nil }
	return app, dir
}

func loadFile(t *testing.T, path string) *projectfile.File {
	t.Helper()
	f, err := projectfile.Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return f
}

// Without a terminal local init takes the flags and creates the file in
// the working directory.
func TestLocalInitFlagsCreateFile(t *testing.T) {
	app, dir := localApp(t)
	writeCfg(t, app, twoCtx)
	wantExit(t, app, Run(app, []string{"local", "init", "--context", "a", "--company", "3352", "--project", "4963", "--task", "545", "--user", "1"}), 0)
	path := filepath.Join(dir, projectfile.FileName)
	f := loadFile(t, path)
	if f.Context != "a" || f.Company != 3352 || f.Project != 4963 || f.Task != 545 || f.User != 1 {
		t.Errorf("file = %+v", f)
	}
	out, errs := bufs(app)
	if !strings.Contains(out, "Projektdatei "+path+" angelegt") || !strings.Contains(out, "project = 4963") {
		t.Errorf("stdout = %q", out)
	}
	if errs != "" {
		t.Errorf("stderr = %q", errs)
	}
	// --context of local init names the context of the file; it does not
	// select a context for the command
	if app.Flags.Context != "" {
		t.Errorf("global --context set to %q", app.Flags.Context)
	}
}

// A project file found upwards (Git root) is updated in place: only the
// given keys change, unknown keys and [tasks] stay; lost comments are
// reported.
func TestLocalInitUpdatesFoundFile(t *testing.T) {
	app, _ := localApp(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, projectfile.FileName)
	if err := os.WriteFile(path, []byte("# Kunde X\nproject = 34\ntask = 56\nnotiz = \"bleibt\"\n\n[tasks]\ntickets = 57\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Getwd = func() (string, error) { return sub, nil }
	wantExit(t, app, Run(app, []string{"local", "init", "--task", "tickets", "--user", "1"}), 0)
	if _, err := os.Stat(filepath.Join(sub, projectfile.FileName)); err == nil {
		t.Error("new file created in the working directory")
	}
	f := loadFile(t, path)
	if f.Project != 34 || f.Task != 57 || f.User != 1 || f.Tasks["tickets"] != 57 || strings.Join(f.Unknown, ",") != "notiz" {
		t.Errorf("file = %+v", f)
	}
	out, errs := bufs(app)
	if !strings.Contains(out, "aktualisiert") {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(errs, "Warnung:") || !strings.Contains(errs, "Kommentare") {
		t.Errorf("stderr ohne Kommentar-Warnung: %q", errs)
	}
}

// Unchanged values leave the file (and its comments) alone.
func TestLocalInitUnchanged(t *testing.T) {
	app, _ := localApp(t)
	content := "# Kommentar\nproject = 34\n"
	path := writeProjectFile(t, app, content)
	wantExit(t, app, Run(app, []string{"local", "init", "--project", "34"}), 0)
	if data, _ := os.ReadFile(path); string(data) != content {
		t.Errorf("file rewritten: %q", data)
	}
	if out, errs := bufs(app); !strings.Contains(out, "unverändert") || strings.Contains(errs, "Kommentare") {
		t.Errorf("stdout = %q, stderr = %q", out, errs)
	}
}

// Usage errors come before the read-only guard and before anything is
// written.
func TestLocalInitUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		tty  bool
		want string
	}{
		{"no values without terminal", nil, false, "--project"},
		{"no values with --no-input", []string{"--no-input"}, true, "--project"},
		{"bad id", []string{"--project", "abc"}, false, "--project"},
		{"zero id", []string{"--company", "0"}, false, "--company"},
		{"negative user", []string{"--user", "-1"}, false, "--user"},
		{"bad context", []string{"--context", "firma gmbh"}, false, "Kontextname"},
		{"unknown alias", []string{"--task", "tickets"}, false, "tickets"},
		{"argument", []string{"34"}, false, "unerwartetes Argument"},
		{"dry run", []string{"--project", "34", "--dry-run"}, false, "--dry-run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, dir := localApp(t)
			app.StdinTTY = tc.tty
			wantExit(t, app, Run(app, append([]string{"local", "init"}, tc.args...)), 2)
			if _, errs := bufs(app); !strings.Contains(errs, tc.want) {
				t.Errorf("stderr ohne %q: %s", tc.want, errs)
			}
			if _, err := os.Stat(filepath.Join(dir, projectfile.FileName)); err == nil {
				t.Error("file written")
			}
		})
	}
}

// Spec 8.3: PAPIERKRAM_READ_ONLY blocks local init (exit 9) through the
// central guard of config-writing commands; nothing is written.
func TestLocalInitReadOnly(t *testing.T) {
	app, dir := localApp(t)
	app.Env = roEnv("1")
	wantExit(t, app, Run(app, []string{"local", "init", "--project", "34"}), 9)
	if _, err := os.Stat(filepath.Join(dir, projectfile.FileName)); err == nil {
		t.Error("file written under read-only")
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Projektdatei") {
		t.Errorf("stderr = %s", errs)
	}
	cmd, _, err := NewRootCmd(app).Find([]string{"local", "init"})
	if err != nil || cmd.Annotations[annotWritesConfig] != "true" {
		t.Errorf("local init not annotated: %v %v", err, cmd.Annotations)
	}
}

// With a terminal and without flags local init asks for every value;
// Enter keeps the current one.
func TestLocalInitInteractive(t *testing.T) {
	app, _ := localApp(t)
	path := writeProjectFile(t, app, "company = 12\nproject = 34\n\n[tasks]\ntickets = 57\n")
	app.StdinTTY = true
	// company: keep; project: invalid, then 35; task: alias; user: 1; context: b
	app.In = strings.NewReader("\nabc\n35\ntickets\n1\nb\n")
	wantExit(t, app, Run(app, []string{"local", "init"}), 0)
	f := loadFile(t, path)
	if f.Company != 12 || f.Project != 35 || f.Task != 57 || f.User != 1 || f.Context != "b" {
		t.Errorf("file = %+v", f)
	}
	_, errs := bufs(app)
	for _, w := range []string{"Firma-ID [12]: ", "Projekt-ID [34]: ", "ungültig", "Aufgabe", "Benutzer-ID: ", "Kontext: "} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %s", w, errs)
		}
	}

	// end of input keeps everything: nothing is written
	app, dir := localApp(t)
	app.StdinTTY = true
	wantExit(t, app, Run(app, []string{"local", "init"}), 0)
	if _, err := os.Stat(filepath.Join(dir, projectfile.FileName)); err == nil {
		t.Error("file written without values")
	}
}

// Review Task 29 (M2): the hints of task create and project describe name
// papierkram local init --project <id>; exactly that command works and
// the preset then applies.
func TestLocalInitFromPresetHint(t *testing.T) {
	hintRe := regexp.MustCompile(`papierkram (local init --project) <id>`)
	for _, tc := range []struct {
		args, then []string
		want       string
	}{
		{[]string{"task", "create", "--name", "Support"}, []string{"--dry-run", "-o", "json"}, `"id": 4963`},
		{[]string{"project", "describe"}, []string{"-o", "json"}, `"id":`},
	} {
		app, srv := apiApp(t)
		dir := t.TempDir()
		app.Getwd = func() (string, error) { return dir, nil }
		if code := Run(app, tc.args); code == 0 {
			t.Fatalf("%v: exit 0 without project", tc.args)
		}
		_, errs := bufs(app)
		m := hintRe.FindStringSubmatch(errs)
		if m == nil {
			t.Fatalf("%v: hint without local init --project <id>: %s", tc.args, errs)
		}
		wantExit(t, app, Run(app, append(strings.Fields(m[1]), "4963")), 0)
		app.Out.(interface{ Reset() }).Reset()
		wantExit(t, app, Run(app, append(tc.args, tc.then...)), 0)
		if out, _ := bufs(app); !strings.Contains(out, tc.want) {
			t.Errorf("%v: stdout = %s", tc.args, out)
		}
		if tc.args[0] == "project" {
			if r := wantRequests(t, srv, 1)[0]; r.Path != "/projects/4963" {
				t.Errorf("request = %s", r.Path)
			}
		}
	}
}

// Spec 9.8: local show lists the effective values with their source.
func TestLocalShowSources(t *testing.T) {
	app, srv := contextApp(t, map[string]string{"PAPIERKRAM_TASK": "20"})
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\nuser = 7\n[contexts.b]\nsubdomain = \"firma-b\"\n")
	path := writeProjectFile(t, app, "context = \"a\"\nproject = 34\ntask = 30\n")
	wantExit(t, app, Run(app, []string{"local", "show", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	out, _ := bufs(app)
	var got localShowJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("no JSON: %v\n%s", err, out)
	}
	if got.ProjectFile == nil || *got.ProjectFile != path || got.ProjectFileIgnored {
		t.Errorf("project file = %v, ignored %v", got.ProjectFile, got.ProjectFileIgnored)
	}
	want := map[string]string{
		"context": "a projectfile " + path,
		"company": "<nil> default ",
		"project": "34 projectfile " + path,
		"task":    "20 env PAPIERKRAM_TASK",
		"user":    "7 context a",
	}
	if len(got.Values) != len(want) {
		t.Errorf("values = %+v", got.Values)
	}
	for _, v := range got.Values {
		if s := jsonText(v.Value) + " " + v.Source + " " + v.Origin; s != want[v.Key] {
			t.Errorf("%s = %q, want %q", v.Key, s, want[v.Key])
		}
	}

	// --context b: the file belongs to a, its values are ignored
	app.Out.(interface{ Reset() }).Reset()
	app.Err.(interface{ Reset() }).Reset()
	wantExit(t, app, Run(app, []string{"local", "show", "--context", "b", "-o", "json"}), 0)
	out, errs := bufs(app)
	got = localShowJSON{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("no JSON: %v\n%s", err, out)
	}
	if !got.ProjectFileIgnored || got.ProjectFileContext != "a" {
		t.Errorf("ignored = %v, context %q", got.ProjectFileIgnored, got.ProjectFileContext)
	}
	for _, v := range got.Values {
		s := jsonText(v.Value) + " " + v.Source
		switch v.Key {
		case "context":
			if s != "b flag" {
				t.Errorf("context = %q", s)
			}
		case "project", "user":
			if v.Source != "default" {
				t.Errorf("%s = %q, want default", v.Key, s)
			}
		}
	}
	if strings.Contains(errs, "Warnung") {
		t.Errorf("duplicate warning on stderr: %s", errs)
	}

	// table: one row per key, then the project file
	app.Out.(interface{ Reset() }).Reset()
	wantExit(t, app, Run(app, []string{"local", "show", "--context", "b"}), 0)
	out, _ = bufs(app)
	for _, w := range []string{"SCHLÜSSEL", "HERKUNFT", "context", "flag", "Projektdatei: " + path, `gilt für Kontext "a"`, "ignoriert"} {
		if !strings.Contains(out, w) {
			t.Errorf("table ohne %q:\n%s", w, out)
		}
	}
}

// Without a project file local show says so; it reads only.
func TestLocalShowWithoutFile(t *testing.T) {
	app, _ := localApp(t)
	wantExit(t, app, Run(app, []string{"local", "show"}), 0)
	if out, _ := bufs(app); !strings.Contains(out, "Projektdatei: keine") || !strings.Contains(out, "default") {
		t.Errorf("stdout = %s", out)
	}
	app, _ = localApp(t)
	wantExit(t, app, Run(app, []string{"local", "show", "--dry-run"}), 2)
	app, _ = localApp(t)
	app.Env = roEnv("1")
	wantExit(t, app, Run(app, []string{"local", "show"}), 0)
}

// A context that is not in the configuration is written with a warning:
// commands in this directory would not find it.
func TestLocalInitWarnsUnknownContext(t *testing.T) {
	app, dir := localApp(t)
	writeCfg(t, app, twoCtx)
	wantExit(t, app, Run(app, []string{"local", "init", "--context", "c"}), 0)
	if f := loadFile(t, filepath.Join(dir, projectfile.FileName)); f.Context != "c" {
		t.Errorf("file = %+v", f)
	}
	if _, errs := bufs(app); !strings.Contains(errs, `Kontext "c"`) || !strings.Contains(errs, "context create c") {
		t.Errorf("stderr = %s", errs)
	}
}

// Spec 9.8: unknown keys of the project file are a warning (once per
// invocation), no error.
func TestProjectFileUnknownKeysWarnOnce(t *testing.T) {
	app, srv := apiApp(t)
	writeProjectFile(t, app, "project = 34\nprojekt = 35\nfoo = 1\n")
	wantExit(t, app, Run(app, []string{"task", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	_, errs := bufs(app)
	if n := strings.Count(errs, "unbekannte Schlüssel"); n != 1 {
		t.Errorf("%d warnings: %s", n, errs)
	}
	if !strings.Contains(errs, `"foo", "projekt"`) {
		t.Errorf("stderr = %s", errs)
	}
}

func jsonText(v any) string {
	if v == nil {
		return "<nil>"
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

// Security review: local init never writes through a symbolic link; the
// target outside the directory stays unchanged (exit 2).
func TestLocalInitRefusesSymlink(t *testing.T) {
	app, dir := localApp(t)
	outside := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(outside, []byte("export PATH=/usr/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, projectfile.FileName)); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	// the target is no valid project file: point at a valid one too
	for _, content := range []string{"export PATH=/usr/bin\n", "project = 1\n"} {
		if err := os.WriteFile(outside, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		app.Err.(interface{ Reset() }).Reset()
		code := Run(app, []string{"local", "init", "--project", "34"})
		if data, _ := os.ReadFile(outside); string(data) != content {
			t.Errorf("target changed: %q", data)
		}
		if code != 2 {
			t.Errorf("content %q: exit = %d, want 2", content, code)
		}
	}
	if _, errs := bufs(app); !strings.Contains(errs, "symbolischer Link") {
		t.Errorf("stderr = %s", errs)
	}
}

// Security review: content of the project file (unknown keys) and its
// path never reach the terminal with control or invisible characters, on
// every path that prints them: warnings and preset hints on stderr, the
// table, YAML, --jq and --fields of local show, the output of local init
// and its prompts. Every scenario must succeed and show its marker, so
// the test cannot silently fall into an error path (aliases with such
// characters are rejected by Load, see TestLoad_AliasWithControlCharacters).
func TestProjectFileContentIsPrintable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows erlaubt keine Steuerzeichen in Dateinamen; die Bereinigung selbst ist plattformunabhängig und läuft auf Unix")
	}
	dir := filepath.Join(t.TempDir(), "repo\x1b[2J\u202eX")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, projectfile.FileName),
		[]byte("project = 34\n\"x\\u001b]0;pwned\\u0007\" = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean := func(t *testing.T, what, s string) {
		t.Helper()
		if strings.ContainsAny(s, "\x1b\x07\u202e") {
			t.Errorf("%s: control characters in %q", what, s)
		}
	}
	for _, tc := range []struct {
		args           []string
		stdout, stderr string
	}{
		{[]string{"task", "list", "-o", "json"}, "", "Hinweis: Projekt 34 aus Projektdatei"},
		{[]string{"task", "list", "-o", "json"}, "", "unbekannte Schlüssel"},
		{[]string{"local", "show"}, "Projektdatei: ", ""},
		{[]string{"local", "show"}, "projectfile", ""},
		{[]string{"local", "show", "-o", "yaml"}, "project_file: ", ""},
		{[]string{"local", "show", "-o", "json"}, `"project_file": `, ""},
		{[]string{"local", "show", "--jq", ".project_file"}, "repo", ""},
		{[]string{"local", "show", "--fields", "project_file"}, "repo", ""},
		{[]string{"local", "init", "--project", "35"}, "aktualisiert", ""},
	} {
		app, _ := apiApp(t)
		app.Getwd = func() (string, error) { return dir, nil }
		wantExit(t, app, Run(app, tc.args), 0)
		out, errs := bufs(app)
		if !strings.Contains(out, tc.stdout) || !strings.Contains(errs, tc.stderr) {
			t.Errorf("%v: marker %q/%q missing:\nstdout %q\nstderr %q", tc.args, tc.stdout, tc.stderr, out, errs)
		}
		clean(t, strings.Join(tc.args, " "), out+errs)
	}

	// a file of another context (direct mode: none is active): the
	// footer saying its values are ignored
	other := filepath.Join(t.TempDir(), "other\x1b[2J\u202eX")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, projectfile.FileName), []byte("context = \"a\"\nproject = 34\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, _ := apiApp(t)
	app.Getwd = func() (string, error) { return other, nil }
	wantExit(t, app, Run(app, []string{"local", "show"}), 0)
	if out, _ := bufs(app); !strings.Contains(out, "ihre Werte werden ignoriert") {
		t.Errorf("ignored footer missing: %q", out)
	}
	clean(t, "local show ignored", mustOut(app))

	// the prompts of local init: header with the path, defaults
	app, _ = localApp(t)
	app.Getwd = func() (string, error) { return dir, nil }
	app.StdinTTY = true
	wantExit(t, app, Run(app, []string{"local", "init"}), 0)
	_, errs := bufs(app)
	for _, w := range []string{"Projektdatei ", "repo", "Firma-ID", "Projekt-ID [35]"} {
		if !strings.Contains(errs, w) {
			t.Errorf("prompts without %q: %q", w, errs)
		}
	}
	clean(t, "prompts", errs)
}

// Review M5: local show uses the output format of the effective context
// like auth status; -o still wins.
func TestLocalShowContextOutput(t *testing.T) {
	app, _ := contextApp(t, nil)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\noutput = \"json\"\n")
	wantExit(t, app, Run(app, []string{"local", "show"}), 0)
	if out := mustOut(app); !strings.HasPrefix(out, "{") {
		t.Errorf("context output json: %q", out)
	}
	app.Out.(interface{ Reset() }).Reset()
	wantExit(t, app, Run(app, []string{"local", "show", "-o", "table"}), 0)
	if out := mustOut(app); !strings.HasPrefix(out, "SCHLÜSSEL") {
		t.Errorf("-o table: %q", out)
	}
}

// Review M3: local init warns when it writes into a file of another than
// the effective context (PAPIERKRAM_CONTEXT): its values only apply there.
func TestLocalInitWarnsFileOfOtherContext(t *testing.T) {
	app, _ := contextApp(t, map[string]string{"PAPIERKRAM_CONTEXT": "a"})
	path := writeProjectFile(t, app, "context = \"b\"\nproject = 4\n")
	wantExit(t, app, Run(app, []string{"local", "init", "--project", "5"}), 0)
	if f := loadFile(t, path); f.Project != 5 || f.Context != "b" {
		t.Errorf("file = %+v", f)
	}
	_, errs := bufs(app)
	for _, w := range []string{"Warnung:", `Kontext "b"`, `Kontext "a"`, "PAPIERKRAM_CONTEXT"} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr ohne %q: %s", w, errs)
		}
	}
	// no warning without the mismatch or when --context sets the context
	for _, tc := range []struct {
		env  map[string]string
		args []string
	}{
		{nil, []string{"--project", "6"}},
		{map[string]string{"PAPIERKRAM_CONTEXT": "a"}, []string{"--project", "6", "--context", "a"}},
	} {
		app, _ := contextApp(t, tc.env)
		writeProjectFile(t, app, "context = \"b\"\nproject = 4\n")
		wantExit(t, app, Run(app, append([]string{"local", "init"}, tc.args...)), 0)
		if _, errs := bufs(app); strings.Contains(errs, "Warnung") {
			t.Errorf("%v: unexpected warning: %s", tc.args, errs)
		}
	}
}
