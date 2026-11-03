package cli

import (
	"runtime"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/buildinfo"
)

func TestVersionYAMLFieldsJQ(t *testing.T) {
	app, out, errb := newTestApp(t)
	if code := Run(app, []string{"version", "-o", "yaml"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	for _, want := range []string{"version: ", "commit: ", "date: ", "go_version: " + runtime.Version() + "\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("yaml ohne %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := Run(app, []string{"version", "-o", "json", "--fields", "version"}); code != 0 {
		t.Fatal(code)
	}
	if want := "{\n  \"version\": \"" + buildinfo.Version + "\"\n}\n"; out.String() != want {
		t.Errorf("--fields: %q, want %q", out.String(), want)
	}
	out.Reset()
	// --jq implies JSON even with the default table format.
	if code := Run(app, []string{"version", "--jq", ".go_version"}); code != 0 {
		t.Fatal(code)
	}
	if out.String() != runtime.Version()+"\n" {
		t.Errorf("--jq: %q", out.String())
	}
}

func TestContextListAndActiveYAMLAndJQ(t *testing.T) {
	app, out, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	if code := Run(app, []string{"context", "list", "-o", "yaml"}); code != 0 {
		t.Fatal(errb.String())
	}
	want := `active_context: a
contexts:
- name: a
  subdomain: firma-a
  read_only: false
  user: 0
  active: true
- name: b
  subdomain: firma-b
  read_only: true
  user: 0
  active: false
`
	if out.String() != want {
		t.Errorf("context list -o yaml:\n%s\nwant\n%s", out.String(), want)
	}
	out.Reset()
	if code := Run(app, []string{"context", "list", "--jq", ".contexts[].name"}); code != 0 {
		t.Fatal(errb.String())
	}
	if out.String() != "a\nb\n" {
		t.Errorf("context list --jq: %q", out.String())
	}
	out.Reset()
	if code := Run(app, []string{"context", "active", "-o", "yaml"}); code != 0 {
		t.Fatal(errb.String())
	}
	if out.String() != "active_context: a\n" {
		t.Errorf("context active -o yaml: %q", out.String())
	}
}

const noTokenErrJSON = `{"error":{"code":"auth","message":"Kein Token für Kontext \"a\" gespeichert","hint":"Anmelden: papierkram auth login --context a (oder PAPIERKRAM_TOKEN setzen)","exit_code":3}}`

// Carried from Task 15: auth status without a token prints the known
// fields on stdout and exits 3; with -o json the error additionally appears
// as structured JSON on stderr.
func TestAuthStatusWithoutTokenStdoutAndErrorJSON(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	if code := run(t, app, "auth", "status", "-o", "json", "--fields", "context,subdomain,quota.known"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	out, errs := bufs(app)
	if want := "{\n  \"context\": \"a\",\n  \"subdomain\": \"firma-a\",\n  \"quota\": {\n    \"known\": false\n  }\n}\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	lines := strings.Split(strings.TrimSuffix(errs, "\n"), "\n")
	if lines[0] != `Fehler: Kein Token für Kontext "a" gespeichert` || lines[len(lines)-1] != noTokenErrJSON {
		t.Errorf("stderr =\n%s", errs)
	}
}

func TestAuthStatusWithoutTokenYAML(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	if code := run(t, app, "auth", "status", "-o", "yaml"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	out, errs := bufs(app)
	if !strings.HasPrefix(out, "context: a\ncontext_source: config\nsubdomain: firma-a\n") || !strings.Contains(out, "quota:\n  remaining: 0\n  consumed: 0\n  known: false\n") {
		t.Errorf("stdout:\n%s", out)
	}
	if !strings.HasSuffix(errs, "error:\n  code: auth\n  message: Kein Token für Kontext \"a\" gespeichert\n  hint: \"Anmelden: papierkram auth login --context a (oder PAPIERKRAM_TOKEN setzen)\"\n  exit_code: 3\n") {
		t.Errorf("stderr:\n%s", errs)
	}
}

// The structured error follows the resolved format: output = "json" in the
// context counts like -o json.
func TestErrorJSONFromContextOutput(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\noutput = \"json\"\n")
	if code := run(t, app, "auth", "status"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if _, errs := bufs(app); !strings.HasSuffix(errs, noTokenErrJSON+"\n") {
		t.Errorf("stderr:\n%s", errs)
	}
	// An explicit -o table beats the context value.
	app2, _ := mockApp(t, nil)
	app2.ConfigDir = app.ConfigDir
	if code := run(t, app2, "auth", "status", "-o", "table"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if _, errs := bufs(app2); strings.Contains(errs, `{"error"`) {
		t.Errorf("JSON trotz -o table:\n%s", errs)
	}
}

// Without a resolved configuration the flag decides.
func TestErrorJSONWithoutResolvedConfig(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	if code := Run(app, []string{"context", "use", "nope", "-o", "json"}); code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	lines := strings.Split(strings.TrimSuffix(errb.String(), "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, `{"error":{"code":"not_found",`) || !strings.HasSuffix(last, `"exit_code":4}}`) {
		t.Errorf("stderr:\n%s", errb.String())
	}
}

func TestInvalidJQIsUsageErrorWithJSON(t *testing.T) {
	app, out, errb := newTestApp(t)
	if code := Run(app, []string{"version", "--jq", ".["}); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.HasPrefix(errb.String(), "Fehler: ungültiger jq-Ausdruck") || !strings.Contains(errb.String(), `{"error":{"code":"usage","message":"ungültiger jq-Ausdruck`) {
		t.Errorf("stderr:\n%s", errb.String())
	}
}

func TestTableErrorHasNoStructuredLine(t *testing.T) {
	for _, args := range [][]string{{"context", "use", "nope"}, {"version", "-o", "xml"}} {
		app, _, errb := newTestApp(t)
		writeCfg(t, app, twoCtx)
		if code := Run(app, args); code == 0 {
			t.Errorf("%v: exit 0", args)
		}
		if strings.Contains(errb.String(), "error") || !strings.HasPrefix(errb.String(), "Fehler: ") {
			t.Errorf("%v: stderr:\n%s", args, errb.String())
		}
	}
}

func TestAuthLoginYAML(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.In = strings.NewReader("test-token-y\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "-o", "yaml"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if out, _ := bufs(app); out != "context: a\nsubdomain: firma-a\ntoken_source: keychain\n" {
		t.Errorf("stdout = %q", out)
	}
	assertNoToken(t, app)
}

func TestOutputOptionsColor(t *testing.T) {
	app, _, _ := newTestApp(t)
	app.StdoutTTY = true
	if !app.outputOptions("table").Color {
		t.Error("TTY ohne NO_COLOR: Farbe erwartet")
	}
	app.Env = envMap(map[string]string{"NO_COLOR": "1"})
	if app.outputOptions("table").Color {
		t.Error("NO_COLOR gesetzt: keine Farbe erwartet")
	}
	app.Env = envMap(nil)
	app.Flags.NoColor = true
	if app.outputOptions("table").Color {
		t.Error("--no-color: keine Farbe erwartet")
	}
}
