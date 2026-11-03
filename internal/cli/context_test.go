package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/auth"
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

func TestR36_ReadOnlyEnvBlocksContextUse(t *testing.T) {
	cases := map[string][]string{
		"use":    {"context", "use", "b"},
		"create": {"context", "create", "c", "--subdomain", "x"},
		"delete": {"context", "delete", "b"},
		"rename": {"context", "rename", "b", "c"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			app, _, errb := newTestApp(t)
			app.Env = func(k string) string {
				if k == "PAPIERKRAM_READ_ONLY" {
					return "1"
				}
				return ""
			}
			writeCfg(t, app, twoCtx)
			if err := app.Keyring.Set("b", "test-token-b"); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(cfgFile(t, app))
			if code := Run(app, args); code != 9 {
				t.Fatalf("exit = %d, want 9; stderr = %s", code, errb.String())
			}
			if !strings.Contains(errb.String(), "Schreibschutz aktiv (PAPIERKRAM_READ_ONLY=1)") {
				t.Errorf("Meldung fehlt: %s", errb.String())
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

func TestReadOnlyEnvAllowsListAndActive(t *testing.T) {
	app, _, errb := newTestApp(t)
	app.Env = func(k string) string {
		if k == "PAPIERKRAM_READ_ONLY" {
			return "1"
		}
		return ""
	}
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
	if !strings.Contains(out.String(), "SCHREIBSCHUTZ") || !strings.Contains(out.String(), "* ") {
		t.Errorf("Tabelle:\n%s", out.String())
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
