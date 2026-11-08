package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/timer"
)

func cfgFile(t *testing.T, app *App) string {
	t.Helper()
	return filepath.Join(app.ConfigDir, "papierkram", "config.toml")
}

func writeCfg(t *testing.T, app *App, content string) {
	t.Helper()
	p := cfgFile(t, app)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const twoCtx = `active_context = "a"

[contexts.a]
subdomain = "firma-a"
read_only = false

[contexts.b]
subdomain = "firma-b"
read_only = true
`

func roEnv(value string) func(string) string {
	return func(k string) string {
		if k == "PAPIERKRAM_READ_ONLY" {
			return value
		}
		return ""
	}
}

func TestR36_ReadOnlyEnvBlocksContextUse(t *testing.T) {
	cases := map[string][]string{
		"use":    {"context", "use", "b"},
		"create": {"context", "create", "c", "--subdomain", "x"},
		"delete": {"context", "delete", "b"},
		"rename": {"context", "rename", "b", "c"},
		"login":  {"auth", "login", "--token-stdin", "--context", "b"},
		"logout": {"auth", "logout", "--context", "b"},
	}
	// Any non-empty value other than 0/false/no/off enables the hard
	// read-only mode (fail-closed); unknown words additionally warn.
	values := map[string]bool{"1": false, "1 ": false, " 1": false, "true": false, "TRUE": false, "yes": false, "ja": true}
	for name, args := range cases {
		for value, warn := range values {
			t.Run(fmt.Sprintf("%s/%q", name, value), func(t *testing.T) {
				app, _, errb := newTestApp(t)
				app.Env = roEnv(value)
				writeCfg(t, app, twoCtx)
				if err := app.Keyring.Set("b", "test-token-b"); err != nil {
					t.Fatal(err)
				}
				before, _ := os.ReadFile(cfgFile(t, app))
				if code := Run(app, args); code != 9 {
					t.Fatalf("exit = %d, want 9; stderr = %s", code, errb.String())
				}
				if !strings.Contains(errb.String(), "Fehler: Schreibschutz aktiv (PAPIERKRAM_READ_ONLY ist gesetzt)") {
					t.Errorf("Meldung fehlt: %s", errb.String())
				}
				if got := strings.Contains(errb.String(), "Warnung: PAPIERKRAM_READ_ONLY"); got != warn {
					t.Errorf("Warnung = %v, erwartet %v: %s", got, warn, errb.String())
				}
				after, _ := os.ReadFile(cfgFile(t, app))
				if string(before) != string(after) {
					t.Errorf("Config verändert:\n%s", after)
				}
				if tok, err := app.Keyring.Get("b"); err != nil || tok != "test-token-b" {
					t.Errorf("Keychain verändert: %q %v", tok, err)
				}
			})
		}
	}
}

func TestReadOnlyEnvOffValuesAllowConfigWrites(t *testing.T) {
	for _, value := range []string{"", "0", " 0 ", "false", "FALSE", "no", "off"} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			app, _, errb := newTestApp(t)
			app.Env = roEnv(value)
			if code := Run(app, []string{"context", "create", "c", "--subdomain", "x"}); code != 0 {
				t.Fatalf("exit = %d; stderr = %s", code, errb.String())
			}
			if errb.Len() != 0 {
				t.Errorf("unerwartete stderr-Ausgabe: %s", errb.String())
			}
		})
	}
}

func TestReadOnlyEnvUnknownValueWarnsOnReadCommands(t *testing.T) {
	app, _, errb := newTestApp(t)
	app.Env = roEnv("enabled")
	writeCfg(t, app, twoCtx)
	if code := Run(app, []string{"context", "list"}); code != 0 {
		t.Fatalf("exit = %d; stderr = %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), `Warnung: PAPIERKRAM_READ_ONLY hat den unbekannten Wert "enabled"`) {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestReadOnlyEnvAllowsListAndActive(t *testing.T) {
	app, _, errb := newTestApp(t)
	app.Env = roEnv("1")
	writeCfg(t, app, twoCtx)
	for _, a := range [][]string{{"context", "list"}, {"context", "active"}} {
		if code := Run(app, a); code != 0 {
			t.Errorf("%v: exit %d %s", a, code, errb.String())
		}
	}
}

func TestContextCreateFirstBecomesActive(t *testing.T) {
	app, out, errb := newTestApp(t)
	if code := Run(app, []string{"context", "create", "firma", "--subdomain", "firma-gmbh", "--user", "7"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "aktiv") {
		t.Errorf("Ausgabe: %s", out.String())
	}
	out.Reset()
	if code := Run(app, []string{"context", "create", "zweit", "--subdomain", "z", "--read-only"}); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	Run(app, []string{"context", "active"})
	if strings.TrimSpace(out.String()) != "firma" {
		t.Errorf("active = %q", out.String())
	}
	data, _ := os.ReadFile(cfgFile(t, app))
	for _, w := range []string{"subdomain = 'firma-gmbh'", "user = 7", "read_only = true"} {
		if !strings.Contains(strings.ReplaceAll(string(data), `"`, "'"), w) {
			t.Errorf("config enthält %q nicht:\n%s", w, data)
		}
	}
}

func TestContextCreateErrors(t *testing.T) {
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"context", "create", "x"}); code != 2 || !strings.Contains(errb.String(), "--subdomain") {
		t.Errorf("ohne subdomain: %d %s", code, errb.String())
	}
	errb.Reset()
	if code := Run(app, []string{"context", "create"}); code != 2 || strings.Contains(errb.String(), "accepts") {
		t.Errorf("ohne Name: %d %s", code, errb.String())
	}
	for _, u := range []string{"7abc", "0", "-1", "x"} {
		errb.Reset()
		if code := Run(app, []string{"context", "create", "x", "--subdomain", "s", "--user", u}); code != 2 || !strings.Contains(errb.String(), "--user") {
			t.Errorf("--user %q: %d %s", u, code, errb.String())
		}
	}
	Run(app, []string{"context", "create", "x", "--subdomain", "s"})
	errb.Reset()
	if code := Run(app, []string{"context", "create", "x", "--subdomain", "s"}); code != 2 || !strings.Contains(errb.String(), "existiert bereits") {
		t.Errorf("doppelt: %d %s", code, errb.String())
	}
}

func TestContextUseListActive(t *testing.T) {
	app, out, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	if code := Run(app, []string{"context", "use", "b"}); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	if code := Run(app, []string{"context", "list", "-o", "json"}); code != 0 {
		t.Fatal(errb.String())
	}
	var got struct {
		Active   string `json:"active_context"`
		Contexts []struct {
			Name, Subdomain string
			ReadOnly        bool `json:"read_only"`
			Active          bool
		} `json:"contexts"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got.Active != "b" || len(got.Contexts) != 2 || got.Contexts[0].Name != "a" || got.Contexts[0].Active || !got.Contexts[1].Active || !got.Contexts[1].ReadOnly || got.Contexts[1].Subdomain != "firma-b" {
		t.Errorf("json: %+v", got)
	}
	out.Reset()
	Run(app, []string{"context", "list"})
	if want := "NAME  SUBDOMAIN  SCHREIBSCHUTZ  AKTIV\na     firma-a    nein\nb     firma-b    ja             *\n"; out.String() != want {
		t.Errorf("Tabelle:\n%s\nwant\n%s", out.String(), want)
	}
	out.Reset()
	Run(app, []string{"context", "active", "-o", "json"})
	if !strings.Contains(out.String(), `"active_context": "b"`) {
		t.Errorf("active json: %s", out.String())
	}
	if code := Run(app, []string{"context", "use", "nope"}); code != 4 {
		t.Errorf("use unbekannt: exit %d", code)
	}
}

func TestContextDeleteRemovesKeychain(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	_ = app.Keyring.Set("a", "test-token-a")
	if code := Run(app, []string{"context", "delete", "a"}); code != 0 {
		t.Fatal(errb.String())
	}
	if _, err := app.Keyring.Get("a"); err != auth.ErrNotFound {
		t.Errorf("Token noch da: %v", err)
	}
	data, _ := os.ReadFile(cfgFile(t, app))
	if strings.Contains(string(data), "firma-a") || strings.Contains(string(data), `active_context = 'a'`) {
		t.Errorf("config:\n%s", data)
	}
}

func TestContextRenameMovesKeychain(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	_ = app.Keyring.Set("a", "test-token-a")
	if code := Run(app, []string{"context", "rename", "a", "neu"}); code != 0 {
		t.Fatal(errb.String())
	}
	if tok, err := app.Keyring.Get("neu"); err != nil || tok != "test-token-a" {
		t.Errorf("neu: %q %v", tok, err)
	}
	if _, err := app.Keyring.Get("a"); err != auth.ErrNotFound {
		t.Errorf("alt noch da: %v", err)
	}
	data, _ := os.ReadFile(cfgFile(t, app))
	if !strings.Contains(string(data), "neu") {
		t.Errorf("config:\n%s", data)
	}
	// ohne Keychain-Eintrag: nur Config
	if code := Run(app, []string{"context", "rename", "b", "c"}); code != 0 {
		t.Fatal(errb.String())
	}
	cfg, _, err := app.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Contexts["b"] != nil || cfg.Contexts["c"] == nil || cfg.Contexts["c"].Subdomain != "firma-b" || !cfg.Contexts["c"].ReadOnly || cfg.ActiveContext != "neu" {
		t.Errorf("Config nach zweitem Umbenennen: %+v", cfg)
	}
	if _, err := app.Keyring.Get("c"); err != auth.ErrNotFound {
		t.Errorf("Keychain-Eintrag für c entstanden: %v", err)
	}
}

type failSetStore struct{ auth.Store }

func (failSetStore) Set(string, string) error { return os.ErrPermission }

func TestContextRenameKeychainFailureKeepsConfig(t *testing.T) {
	app, _, errb := newTestApp(t)
	mem := auth.NewMemory()
	_ = mem.Set("a", "test-token-a")
	app.Keyring = failSetStore{mem}
	writeCfg(t, app, twoCtx)
	before, _ := os.ReadFile(cfgFile(t, app))
	if code := Run(app, []string{"context", "rename", "a", "neu"}); code == 0 {
		t.Fatal("erwartete Fehler")
	}
	after, _ := os.ReadFile(cfgFile(t, app))
	if string(before) != string(after) {
		t.Errorf("Config verändert")
	}
	if strings.Contains(errb.String(), "test-token") {
		t.Error("Token in Fehlermeldung")
	}
}

func TestContextBrokenConfigGermanError(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, "this is = = not toml [")
	code := Run(app, []string{"context", "list"})
	if code == 0 {
		t.Fatal("erwartete Fehler")
	}
	if !strings.Contains(errb.String(), "Hinweis: Konfigurationsdatei prüfen: "+cfgFile(t, app)) {
		t.Errorf("stderr: %s", errb.String())
	}
}

func TestConfigDirEnvOverride(t *testing.T) {
	app, _, errb := newTestApp(t)
	other := t.TempDir()
	app.Env = func(k string) string {
		if k == "PAPIERKRAM_CONFIG_DIR" {
			return other
		}
		return ""
	}
	if code := Run(app, []string{"context", "create", "x", "--subdomain", "s"}); code != 0 {
		t.Fatal(errb.String())
	}
	if _, err := os.Stat(filepath.Join(other, "papierkram", "config.toml")); err != nil {
		t.Error(err)
	}
}

func TestContextUnknownConfigKeyIsUsageError(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, "active_context = \"a\"\n\n[contexts.a]\nsubdomain = \"firma-a\"\nreadonly = true\n")
	before, _ := os.ReadFile(cfgFile(t, app))
	if code := Run(app, []string{"context", "use", "a"}); code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %s", code, errb.String())
	}
	for _, w := range []string{`unbekannter Schlüssel "readonly"`, cfgFile(t, app), `Meinten Sie "read_only"?`} {
		if !strings.Contains(errb.String(), w) {
			t.Errorf("stderr lacks %q: %s", w, errb.String())
		}
	}
	after, _ := os.ReadFile(cfgFile(t, app))
	if string(before) != string(after) {
		t.Errorf("Config verändert:\n%s", after)
	}
}

func TestContextHelpMentionsLostComments(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"context", "--help"}); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "Kommentare") {
		t.Errorf("Hilfe erwähnt verlorene Kommentare nicht:\n%s", out.String())
	}
}

func TestContextCreateValidatesSubdomain(t *testing.T) {
	for in, hint := range map[string]string{
		"firma.papierkram.de": "Nur die Subdomain angeben, z. B. firma",
		"evil.example#":       "Nur die Subdomain angeben, z. B. firma",
		"-firma":              "Bindestriche",
	} {
		app, _, errb := newTestApp(t)
		if code := Run(app, []string{"context", "create", "x", "--subdomain", in}); code != 2 {
			t.Errorf("%q: exit = %d, want 2", in, code)
		}
		if !strings.Contains(errb.String(), `ungültige Subdomain "`+in+`" (--subdomain)`) || !strings.Contains(errb.String(), hint) {
			t.Errorf("%q: stderr = %s", in, errb.String())
		}
		if _, err := os.Stat(cfgFile(t, app)); !os.IsNotExist(err) {
			t.Errorf("%q: Config geschrieben", in)
		}
	}
	app, _, errb := newTestApp(t)
	if code := Run(app, []string{"context", "create", "x", "--subdomain", "Firma-GmbH"}); code != 0 {
		t.Fatal(errb.String())
	}
	data, _ := os.ReadFile(cfgFile(t, app))
	if !strings.Contains(string(data), "firma-gmbh") {
		t.Errorf("Subdomain nicht normalisiert:\n%s", data)
	}
}

// configWriteSampleArgs holds valid arguments for every command annotated as
// config-writing; a new annotated command must be added here.
var configWriteSampleArgs = map[string][]string{
	"papierkram context create": {"c", "--subdomain", "x"},
	"papierkram context use":    {"b"},
	"papierkram context delete": {"b"},
	"papierkram context rename": {"b", "c"},
	"papierkram auth login":     {"--token-stdin"},
	"papierkram auth logout":    {},
	"papierkram local init":     {"--project", "34"},
}

func TestConfigWritingCommandsAnnotatedAndBlocked(t *testing.T) {
	app, _, _ := newTestApp(t)
	var annotated []string
	walk(NewRootCmd(app), func(c *cobra.Command) {
		if c.Annotations[annotWritesConfig] == "true" {
			annotated = append(annotated, c.CommandPath())
		}
	})
	for _, want := range []string{"papierkram context create", "papierkram context use", "papierkram context delete", "papierkram context rename", "papierkram auth login", "papierkram auth logout", "papierkram local init"} {
		if !slices.Contains(annotated, want) {
			t.Errorf("%q ist nicht als config-schreibend annotiert", want)
		}
	}
	for _, path := range annotated {
		sample, ok := configWriteSampleArgs[path]
		if !ok {
			t.Errorf("Beispielargumente für %q fehlen in configWriteSampleArgs", path)
			continue
		}
		app, _, errb := newTestApp(t)
		app.Env = roEnv("1")
		writeCfg(t, app, twoCtx)
		args := append(strings.Fields(path)[1:], sample...)
		if code := Run(app, args); code != 9 {
			t.Errorf("%v: exit = %d, want 9; stderr = %s", args, code, errb.String())
		}
	}
	for _, path := range []string{"papierkram context list", "papierkram context active", "papierkram version", "papierkram auth status", "papierkram local show"} {
		if slices.Contains(annotated, path) {
			t.Errorf("%q schreibt nicht und darf nicht annotiert sein", path)
		}
	}
}

func TestConfigWriteGuardIsCentral(t *testing.T) {
	app, _, errb := newTestApp(t)
	app.Env = roEnv("1")
	root := NewRootCmd(app)
	ran := false
	group := &cobra.Command{
		Use:               "group",
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
	}
	group.AddCommand(&cobra.Command{
		Use:         "write",
		Annotations: map[string]string{annotWritesConfig: "true"},
		RunE:        func(*cobra.Command, []string) error { ran = true; return nil },
	})
	root.AddCommand(group)
	if code := execute(app, root, []string{"group", "write"}); code != 9 {
		t.Errorf("exit = %d, want 9; stderr = %s", code, errb.String())
	}
	if ran {
		t.Error("RunE lief trotz Schreibschutz")
	}
}

func TestContextCreateRemovesStaleKeychainEntry(t *testing.T) {
	app, _, errb := newTestApp(t)
	_ = app.Keyring.Set("c", "test-token-stale")
	if code := Run(app, []string{"context", "create", "c", "--subdomain", "x"}); code != 0 {
		t.Fatalf("exit = %d; stderr = %s", code, errb.String())
	}
	if _, err := app.Keyring.Get("c"); err != auth.ErrNotFound {
		t.Errorf("altes Token noch vorhanden: %v", err)
	}
	if !strings.Contains(errb.String(), `Hinweis: Im Schlüsselbund lag noch ein Token für "c"`) || strings.Contains(errb.String(), "test-token") {
		t.Errorf("stderr = %q", errb.String())
	}
	// no stale entry: no hint
	errb.Reset()
	if code := Run(app, []string{"context", "create", "d", "--subdomain", "x"}); code != 0 || errb.Len() != 0 {
		t.Errorf("exit = %d; stderr = %q", code, errb.String())
	}
}

type failDeleteStore struct{ auth.Store }

func (failDeleteStore) Delete(string) error { return os.ErrPermission }

func TestContextCreateStaleKeychainDeleteFailureKeepsConfig(t *testing.T) {
	app, _, errb := newTestApp(t)
	mem := auth.NewMemory()
	_ = mem.Set("c", "test-token-stale")
	app.Keyring = failDeleteStore{mem}
	if code := Run(app, []string{"context", "create", "c", "--subdomain", "x"}); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr = %s", code, errb.String())
	}
	if _, err := os.Stat(cfgFile(t, app)); !os.IsNotExist(err) {
		t.Error("Config trotz Fehler geschrieben")
	}
}

func TestContextRenameRefusesExistingTargetKeychainEntry(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, twoCtx)
	_ = app.Keyring.Set("a", "test-token-a")
	_ = app.Keyring.Set("neu", "test-token-fremd")
	before, _ := os.ReadFile(cfgFile(t, app))
	if code := Run(app, []string{"context", "rename", "b", "neu"}); code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), `Im Schlüsselbund existiert bereits ein Token für "neu"`) || strings.Contains(errb.String(), "test-token") {
		t.Errorf("stderr = %q", errb.String())
	}
	after, _ := os.ReadFile(cfgFile(t, app))
	if string(before) != string(after) {
		t.Errorf("Config verändert:\n%s", after)
	}
	for name, want := range map[string]string{"a": "test-token-a", "neu": "test-token-fremd"} {
		if tok, err := app.Keyring.Get(name); err != nil || tok != want {
			t.Errorf("%s: %q %v", name, tok, err)
		}
	}
}

// A running timer belongs to its context by file name: renaming or
// deleting the context would orphan it (M3). Both refuse with exit 7
// before anything changes; without a timer they run as before.
func TestContextRenameDeleteRefuseWithTimer(t *testing.T) {
	for _, args := range [][]string{{"context", "rename", "a", "neu"}, {"context", "delete", "a"}} {
		t.Run(args[1], func(t *testing.T) {
			app, out, errb := newTestApp(t)
			writeCfg(t, app, twoCtx)
			_ = app.Keyring.Set("a", "test-token-a")
			path := timer.Path(app.CacheDir, "a")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			// any file counts, also one time status would call invalid
			if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(cfgFile(t, app))
			if code := Run(app, append(args, "-o", "json")); code != 7 {
				t.Fatalf("exit %d, stderr %s", code, errb.String())
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q", out.String())
			}
			if after, _ := os.ReadFile(cfgFile(t, app)); string(after) != string(before) {
				t.Errorf("config changed:\n%s", after)
			}
			if tok, err := app.Keyring.Get("a"); err != nil || tok != "test-token-a" {
				t.Errorf("keychain a: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("timer file: %v", err)
			}
			var got struct {
				Error struct{ Code, Hint string } `json:"error"`
			}
			if err := json.Unmarshal([]byte(lastLine(errb.String())), &got); err != nil {
				t.Fatalf("%v: %s", err, errb.String())
			}
			if e := got.Error; e.Code != "conflict" || !strings.HasPrefix(e.Hint, "erst papierkram time stop oder papierkram time discard") ||
				!strings.Contains(e.Hint, "--context a") {
				t.Errorf("error = %+v", e)
			}

			// the claim of an unfinished time stop counts too (task 11)
			leaveClaim(t, path)
			errb.Reset()
			if code := Run(app, args); code != 7 || !strings.Contains(errb.String(), ".stopping") {
				t.Fatalf("with a claim: exit %d, stderr %s", code, errb.String())
			}

			// the timer of another context does not block
			if err := os.Rename(timer.StoppingPath(path), timer.Path(app.CacheDir, "b")); err != nil {
				t.Fatal(err)
			}
			errb.Reset()
			if code := Run(app, args); code != 0 {
				t.Fatalf("without timer: exit %d, stderr %s", code, errb.String())
			}
		})
	}
}
