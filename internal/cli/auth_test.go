package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/config"
)

// unavailableStore behaves like a platform without a usable keyring.
type unavailableStore struct{}

func (unavailableStore) Get(string) (string, error) { return "", auth.ErrUnavailable }
func (unavailableStore) Set(string, string) error   { return auth.ErrUnavailable }
func (unavailableStore) Delete(string) error        { return auth.ErrUnavailable }

func loadCfg(t *testing.T, app *App) *config.Config {
	t.Helper()
	c, err := config.Load(cfgFile(t, app))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func run(t *testing.T, app *App, args ...string) int {
	t.Helper()
	return Run(app, args)
}

// Review Focus 1: a token piped with a Windows line ending is trimmed before
// it is verified and stored.
func TestAuthLoginTokenStdinTrimsCRLF(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, "# eigener Kommentar\n"+twoCtx)
	app.In = strings.NewReader("test-token-9\r\n")
	if code := run(t, app, "auth", "login", "--token-stdin"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if tok, err := app.Keyring.Get("a"); err != nil || tok != "test-token-9" {
		t.Errorf("gespeichert = %q, %v", tok, err)
	}
	// Nothing changed in config.toml, so it is not rewritten.
	if b, _ := os.ReadFile(cfgFile(t, app)); !strings.HasPrefix(string(b), "# eigener Kommentar") {
		t.Errorf("config.toml unnötig neu geschrieben:\n%s", b)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Path != "/info" || reqs[0].Header.Get("Authorization") != "Bearer test-token-9" {
		t.Errorf("requests = %+v", reqs)
	}
	if _, errs := bufs(app); !strings.Contains(errs, `Angemeldet: Kontext "a"`) || !strings.Contains(errs, "Schlüsselbund") {
		t.Errorf("stderr = %q", errs)
	}
	assertNoToken(t, app)
}

func TestAuthLoginTTYPrompt(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.StdinTTY = true
	app.ReadPassword = func() ([]byte, error) { return []byte(" test-token-tty "), nil }
	if code := run(t, app, "auth", "login", "--context", "b"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if tok, _ := app.Keyring.Get("b"); tok != "test-token-tty" {
		t.Errorf("gespeichert = %q", tok)
	}
	if _, errs := bufs(app); !strings.Contains(errs, `Token für Kontext "b"`) {
		t.Errorf("Prompt fehlt: %q", errs)
	}
	assertNoToken(t, app)
}

func TestAuthLoginTTYPromptInterrupted(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.StdinTTY = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	app.ReadPassword = func() ([]byte, error) {
		cancel()
		<-block
		return nil, errors.New("beendet")
	}
	restored := false
	app.restoreTerminal = func() { restored = true }
	done := make(chan int, 1)
	go func() {
		done <- executeContext(ctx, app, NewRootCmd(app), []string{"auth", "login", "--context", "b", "-o", "json"})
	}()
	select {
	case code := <-done:
		if code != 8 {
			_, errs := bufs(app)
			t.Fatalf("exit = %d, stderr = %s", code, errs)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Unterbrechung beendet die Token-Eingabe nicht")
	}
	wantInterruptedPrompt(t, app, "Nichts gesendet oder geändert")
	if !restored {
		t.Error("Terminalzustand nicht wiederhergestellt")
	}
	if tok, err := app.Keyring.Get("b"); err == nil {
		t.Errorf("Token gespeichert: %q", tok)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d Requests", n)
	}
}

func TestAuthLoginWithoutTTYNeedsTokenStdin(t *testing.T) {
	for _, args := range [][]string{{"auth", "login"}, {"auth", "login", "--no-input"}} {
		app, srv := mockApp(t, nil)
		writeCfg(t, app, twoCtx)
		if args[len(args)-1] == "--no-input" {
			app.StdinTTY = true
			app.ReadPassword = func() ([]byte, error) { t.Fatal("Prompt trotz --no-input"); return nil, nil }
		}
		if code := Run(app, args); code != 2 {
			t.Errorf("%v: exit = %d, want 2", args, code)
		}
		if _, errs := bufs(app); !strings.Contains(errs, "--token-stdin") {
			t.Errorf("Hinweis fehlt: %q", errs)
		}
		if len(srv.Requests()) != 0 {
			t.Error("Request trotz Usage-Fehler")
		}
	}
}

func TestAuthLoginEmptyTokenIsAuthError(t *testing.T) {
	for _, in := range []string{"", " \r\n", "test-token-a\ntest-token-b\n"} {
		app, srv := mockApp(t, nil)
		writeCfg(t, app, twoCtx)
		app.In = strings.NewReader(in)
		if code := run(t, app, "auth", "login", "--token-stdin"); code != 3 {
			t.Errorf("%q: exit = %d, want 3", in, code)
		}
		if len(srv.Requests()) != 0 {
			t.Errorf("%q: Request gesendet", in)
		}
		if _, err := app.Keyring.Get("a"); !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("%q: Token gespeichert", in)
		}
		assertNoToken(t, app)
	}
}

func TestAuthLoginRejectedTokenStoresNothing(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		app, srv := mockApp(t, nil)
		writeCfg(t, app, twoCtx)
		srv.On(http.MethodGet, "/info", status, nil)
		app.In = strings.NewReader("test-token-bad\n")
		if code := run(t, app, "auth", "login", "--token-stdin"); code != 3 {
			t.Errorf("%d: exit = %d, want 3", status, code)
		}
		if _, err := app.Keyring.Get("a"); !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("%d: Token trotz Ablehnung gespeichert", status)
		}
		if loadCfg(t, app).Contexts["a"].Token != "" {
			t.Errorf("%d: Token in Config", status)
		}
		assertNoToken(t, app)
	}
}

func TestAuthLoginKeychainUnavailable(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.Keyring = unavailableStore{}
	app.In = strings.NewReader("test-token-k\n")
	if code := run(t, app, "auth", "login", "--token-stdin"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "papierkram auth login --context a --token-stdin --insecure-storage < token.txt") {
		t.Errorf("Hinweis behält Eingabeweg/Kontext nicht: %q", errs)
	}
	if loadCfg(t, app).Contexts["a"].Token != "" {
		t.Error("Token ohne --insecure-storage in Config")
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d Requests (Credit verbraucht), obwohl das Token nicht gespeichert werden kann", n)
	}
	assertNoToken(t, app)

	app, _ = mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.Keyring = unavailableStore{}
	app.In = strings.NewReader("test-token-k\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--insecure-storage", "-o", "json"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if got := loadCfg(t, app).Contexts["a"].Token; got != "test-token-k" {
		t.Errorf("config token = %q", got)
	}
	fi, err := os.Stat(cfgFile(t, app))
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no POSIX file modes (same guard as internal/config).
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("Rechte = %v", fi.Mode().Perm())
	}
	out, errs := bufs(app)
	if !strings.Contains(errs, "Warnung:") || !strings.Contains(errs, "Klartext") {
		t.Errorf("Warnung fehlt: %q", errs)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err, out)
	}
	if got["context"] != "a" || got["subdomain"] != "firma-a" || got["token_source"] != "config" || len(got) != 3 {
		t.Errorf("json = %v", got)
	}
	assertNoToken(t, app)
}

func TestAuthLoginKeychainClearsConfigToken(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx+"token = \"test-token-old\"\n")
	app.In = strings.NewReader("test-token-new\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--context", "b"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if got := loadCfg(t, app).Contexts["b"].Token; got != "" {
		t.Errorf("alter Klartext-Token blieb: %q", got)
	}
	if tok, _ := app.Keyring.Get("b"); tok != "test-token-new" {
		t.Errorf("keychain = %q", tok)
	}
}

func TestAuthLoginCreatesContext(t *testing.T) {
	app, _ := mockApp(t, nil)
	app.In = strings.NewReader("test-token-n\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--context", "neu", "--subdomain", " Neu-GmbH ", "-o", "json"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	cfg := loadCfg(t, app)
	if cfg.ActiveContext != "neu" || cfg.Contexts["neu"] == nil || cfg.Contexts["neu"].Subdomain != "neu-gmbh" {
		t.Errorf("config = %+v", cfg)
	}
	out, _ := bufs(app)
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err, out)
	}
	if got["context"] != "neu" || got["subdomain"] != "neu-gmbh" || got["token_source"] != "keychain" {
		t.Errorf("json = %v", got)
	}
	assertNoToken(t, app)
}

// Spec v0.2 §1: auth login --subdomain takes the host, with the same hint
// as context create.
func TestAuthLoginSubdomainURL(t *testing.T) {
	app, srv := mockApp(t, nil)
	app.In = strings.NewReader("test-token-u\n")
	if code := run(t, app, "auth", "login", "--context", "neu", "--subdomain", "pkrason.papierkram.de", "--token-stdin"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if got := loadCfg(t, app).Contexts["neu"]; got == nil || got.Subdomain != "pkrason" {
		t.Errorf("Kontext = %+v", got)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/info" {
		t.Errorf("requests = %+v", reqs)
	}
	if _, errs := bufs(app); !strings.HasPrefix(errs, "Hinweis: Subdomain „pkrason“ aus „pkrason.papierkram.de“\n") {
		t.Errorf("stderr = %q", errs)
	}
	assertNoToken(t, app)
}

func TestAuthLoginContextErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"missing context without subdomain", []string{"--context", "fehlt"}, 2},
		{"no context at all", nil, 2},
		{"subdomain differs from existing context", []string{"--subdomain", "anders"}, 2},
		{"invalid subdomain", []string{"--context", "neu", "--subdomain", "a.b"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, srv := mockApp(t, nil)
			if tt.name != "no context at all" {
				writeCfg(t, app, twoCtx)
			}
			app.In = strings.NewReader("test-token-e\n")
			args := append([]string{"auth", "login", "--token-stdin"}, tt.args...)
			if code := Run(app, args); code != tt.want {
				_, errs := bufs(app)
				t.Errorf("exit = %d, want %d; stderr = %s", code, tt.want, errs)
			}
			if len(srv.Requests()) != 0 {
				t.Error("Request gesendet")
			}
		})
	}
}

func TestAuthLoginReadOnlyEnvSendsNothing(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	writeCfg(t, app, twoCtx)
	app.In = strings.NewReader("test-token-ro\n")
	if code := run(t, app, "auth", "login", "--token-stdin"); code != 9 {
		t.Errorf("exit = %d, want 9", code)
	}
	if len(srv.Requests()) != 0 {
		t.Error("Request trotz Schreibschutz")
	}
	if _, err := app.Keyring.Get("a"); !errors.Is(err, auth.ErrNotFound) {
		t.Error("Token trotz Schreibschutz gespeichert")
	}
}

func TestAuthLogout(t *testing.T) {
	app, _, errb := newTestApp(t)
	writeCfg(t, app, twoCtx+"token = \"test-token-cfg\"\n")
	if err := app.Keyring.Set("b", "test-token-b"); err != nil {
		t.Fatal(err)
	}
	if code := run(t, app, "auth", "logout", "--context", "b"); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if _, err := app.Keyring.Get("b"); !errors.Is(err, auth.ErrNotFound) {
		t.Error("Keychain-Eintrag blieb")
	}
	if loadCfg(t, app).Contexts["b"].Token != "" {
		t.Error("Config-Token blieb")
	}
	if !strings.Contains(errb.String(), `Abgemeldet: Kontext "b"`) {
		t.Errorf("stderr = %q", errb.String())
	}

	// Second logout: nothing left, exit 0 with a note.
	errb.Reset()
	if code := run(t, app, "auth", "logout", "--context", "b"); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), `Kontext "b" war bereits abgemeldet`) {
		t.Errorf("stderr = %q", errb.String())
	}
	assertNoToken(t, app)
}

func TestAuthLogoutUnknownContext(t *testing.T) {
	app, _, _ := newTestApp(t)
	writeCfg(t, app, twoCtx)
	if code := run(t, app, "auth", "logout", "--context", "fehlt"); code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
}

func TestAuthStatusTokenSources(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		keychain bool
		cfgToken bool
		want     string
	}{
		{"env", map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"}, true, true, "env"},
		{"keychain", nil, true, true, "keychain"},
		{"config", nil, false, true, "config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, srv := mockApp(t, tt.env)
			cfg := twoCtx
			if tt.cfgToken {
				cfg = strings.Replace(cfg, "subdomain = \"firma-a\"\n", "subdomain = \"firma-a\"\ntoken = \"test-token-cfg\"\n", 1)
			}
			writeCfg(t, app, cfg)
			if tt.keychain {
				if err := app.Keyring.Set("a", "test-token-kc"); err != nil {
					t.Fatal(err)
				}
			}
			srv.On(http.MethodGet, "/info", http.StatusOK, []byte(`{}`), "X-Remaining-Quota", "8000", "X-Consumed-Quota", "2000")
			if code := run(t, app, "auth", "status", "-o", "json"); code != 0 {
				_, errs := bufs(app)
				t.Fatalf("exit = %d, stderr = %s", code, errs)
			}
			if n := len(srv.Requests()); n != 1 {
				t.Errorf("auth status sendete %d Requests, erwartet genau 1 (1 Credit)", n)
			}
			out, _ := bufs(app)
			var got statusJSON
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err, out)
			}
			want := statusJSON{
				Context: "a", ContextSource: "config", Subdomain: "firma-a", BaseURL: srv.URL(),
				TokenSource: tt.want, Quota: quotaJSON{Remaining: 8000, RequestCost: intPtr(2000), Known: true},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("status = %+v\nwant     %+v", got, want)
			}
			assertNoToken(t, app)
		})
	}
}

func TestAuthStatusJSONKeys(t *testing.T) {
	app, _ := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-j", "PAPIERKRAM_SUBDOMAIN": "firma"})
	if code := run(t, app, "auth", "status", "-o", "json"); code != 0 {
		t.Fatal(code)
	}
	out, _ := bufs(app)
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"context", "context_source", "subdomain", "base_url", "token_source", "read_only", "read_only_source", "quota"} {
		if _, ok := got[k]; !ok {
			t.Errorf("Schlüssel %q fehlt: %v", k, got)
		}
	}
	q, _ := got["quota"].(map[string]any)
	// X-Consumed-Quota is the cost of the one request (the GET /info of
	// auth status), not a monthly consumption (measured 2026-11-04): the
	// key says so
	for _, k := range []string{"remaining", "request_cost", "known"} {
		if _, ok := q[k]; !ok {
			t.Errorf("quota.%s fehlt: %v", k, q)
		}
	}
	if _, ok := q["consumed"]; ok {
		t.Errorf("quota.consumed gibt es nicht mehr: %v", q)
	}
}

func intPtr(n int) *int { return &n }

func TestAuthStatusText(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("b", "test-token-b"); err != nil {
		t.Fatal(err)
	}
	srv.On(http.MethodGet, "/info", http.StatusOK, []byte(`{}`), "X-Remaining-Quota", "50")
	if code := run(t, app, "auth", "status", "--context", "b"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	out, errs := bufs(app)
	if strings.Contains(out, "verbraucht") {
		t.Errorf("stdout nennt einen Verbrauch, die API meldet nur die Kosten der Abfrage:\n%s", out)
	}
	for _, want := range []string{"b (--context)", "firma-b", "keychain", "an (env: PAPIERKRAM_READ_ONLY)", "50 Credits übrig (Kosten dieser Abfrage: 1 Credit)"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout ohne %q:\n%s", want, out)
		}
	}
	if n := strings.Count(errs, "Warnung: Restkontingent niedrig: 50 Credits"); n != 1 {
		t.Errorf("Quota-Warnung %d-mal: %q", n, errs)
	}
	assertNoToken(t, app)
}

func TestAuthStatusWithoutTokenPrintsKnownAndExits3(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	if code := run(t, app, "auth", "status", "-o", "json"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	out, errs := bufs(app)
	var got statusJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err, out)
	}
	if got.Context != "a" || got.Subdomain != "firma-a" || got.TokenSource != "" || got.Quota.Known {
		t.Errorf("status = %+v", got)
	}
	if !strings.Contains(errs, "papierkram auth login") {
		t.Errorf("Hinweis fehlt: %q", errs)
	}
	if len(srv.Requests()) != 0 {
		t.Error("Request ohne Token")
	}
}

func TestAuthStatusRejectedTokenExits3(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-x", "PAPIERKRAM_SUBDOMAIN": "firma"})
	srv.On(http.MethodGet, "/info", http.StatusUnauthorized, nil)
	if code := run(t, app, "auth", "status"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if out, _ := bufs(app); !strings.Contains(out, "env") {
		t.Errorf("Status nicht ausgegeben: %q", out)
	}
	assertNoToken(t, app)
}

func TestAuthStatusHelpMentionsCredit(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := run(t, app, "auth", "status", "--help"); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "1 Credit") {
		t.Errorf("Hilfe: %s", out.String())
	}
}

func TestDebugShowsContextAndReadOnlyLevel(t *testing.T) {
	app, _ := mockApp(t, map[string]string{"PAPIERKRAM_READ_ONLY": "1"})
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("a", "test-token-dbg"); err != nil {
		t.Fatal(err)
	}
	if code := Run(app, []string{"auth", "status", "--debug"}); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	_, errs := bufs(app)
	for _, want := range []string{"Bearer ***", "Schreibschutz: env", `Kontext: "a" (config)`} {
		if !strings.Contains(errs, want) {
			t.Errorf("--debug ohne %q:\n%s", want, errs)
		}
	}
	assertNoToken(t, app)
}

// A UTF-8 BOM (Windows editors) in front of the token is removed as well.
func TestAuthLoginTokenStdinStripsBOM(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.In = strings.NewReader("\uFEFFtest-token-bom\r\n")
	if code := run(t, app, "auth", "login", "--token-stdin"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if tok, _ := app.Keyring.Get("a"); tok != "test-token-bom" {
		t.Errorf("gespeichert = %q", tok)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "Bearer test-token-bom" {
		t.Errorf("requests = %+v", reqs)
	}
}

// The keychain write succeeded but removing the old plain-text token from
// config.toml failed: the login counts (the keychain wins), the leftover is
// named in a warning.
func TestAuthLoginStaleConfigTokenSaveFailure(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx+"token = \"test-token-old\"\n")
	app.saveConfig = func(*config.Config, string) error { return errors.New("Datenträger voll") }
	app.In = strings.NewReader("test-token-new\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--context", "b"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if tok, _ := app.Keyring.Get("b"); tok != "test-token-new" {
		t.Errorf("keychain = %q", tok)
	}
	_, errs := bufs(app)
	want := "Warnung: Token im Schlüsselbund gespeichert, altes Klartext-Token in " + cfgFile(t, app) + " konnte nicht entfernt werden"
	if !strings.Contains(errs, want) {
		t.Errorf("stderr = %q", errs)
	}
	assertNoToken(t, app)
}

// The login path validates the context option output like the client path.
func TestAuthLoginInvalidContextOutput(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\noutput = \"xml\"\n")
	app.In = strings.NewReader("test-token-o\n")
	if code := run(t, app, "auth", "login", "--token-stdin"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if _, errs := bufs(app); !strings.Contains(errs, `"xml"`) {
		t.Errorf("stderr = %q", errs)
	}
	if len(srv.Requests()) != 0 {
		t.Error("Request trotz ungültiger Kontext-Option")
	}
}

// A retried GET reports the quota twice; the warning still appears once.
func TestQuotaWarningOnceAcrossRetry(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-r", "PAPIERKRAM_SUBDOMAIN": "firma"})
	calls := 0
	srv.OnFunc(http.MethodGet, "/info", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("X-Remaining-Quota", "50")
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"type":"error","error_type":"service_unavailable","message":"kurz weg"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	if code := run(t, app, "auth", "status"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (503, dann 200)", calls)
	}
	if _, errs := bufs(app); strings.Count(errs, "Warnung: Restkontingent niedrig: 50 Credits") != 1 {
		t.Errorf("stderr = %q", errs)
	}
}

// A created context whose save fails after the keychain write is rolled
// back: no keychain entry stays behind and the command fails.
func TestAuthLoginNewContextSaveFailureRollsBack(t *testing.T) {
	app, _ := mockApp(t, nil)
	app.saveConfig = func(*config.Config, string) error { return errors.New("Datenträger voll") }
	app.In = strings.NewReader("test-token-n\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--context", "neu", "--subdomain", "neu-gmbh"); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if _, err := app.Keyring.Get("neu"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("Keychain-Eintrag nicht zurückgerollt: %v", err)
	}
	assertNoToken(t, app)
}

// For a login that creates the context, the hint keeps --subdomain, so
// following it does not end in a usage error.
func TestAuthLoginKeychainUnavailableHintKeepsSubdomain(t *testing.T) {
	app, srv := mockApp(t, nil)
	app.Keyring = unavailableStore{}
	app.In = strings.NewReader("test-token-k\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--context", "neu", "--subdomain", "neu-gmbh"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "papierkram auth login --context neu --subdomain neu-gmbh --token-stdin --insecure-storage < token.txt") {
		t.Errorf("Hinweis ohne --subdomain: %q", errs)
	}
	if len(srv.Requests()) != 0 {
		t.Error("Request trotz fehlendem Schlüsselbund")
	}
}

// countingStore counts Get calls on top of an unavailable keychain.
type countingStore struct {
	unavailableStore
	gets *int
}

func (s countingStore) Get(c string) (string, error) { *s.gets++; return s.unavailableStore.Get(c) }

// With --insecure-storage the keychain is not probed (no pointless macOS
// keychain prompt); Set alone decides.
func TestAuthLoginInsecureSkipsKeychainProbe(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	gets := 0
	app.Keyring = countingStore{gets: &gets}
	app.In = strings.NewReader("test-token-i\n")
	if code := run(t, app, "auth", "login", "--token-stdin", "--insecure-storage"); code != 0 {
		_, errs := bufs(app)
		t.Fatalf("exit = %d, stderr = %s", code, errs)
	}
	if gets != 0 {
		t.Errorf("Keyring.Get %d-mal aufgerufen, erwartet 0", gets)
	}
}

// L149: when GET /info fails (exit 3, 4) and the output fails as well (a
// --jq runtime error), the code of the call wins; the output error is a
// warning on stderr. Without a call error the output error stays exit 1.
func TestAuthStatusCallErrorWinsOverOutputError(t *testing.T) {
	for status, exit := range map[int]int{http.StatusUnauthorized: 3, http.StatusNotFound: 4} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-x", "PAPIERKRAM_SUBDOMAIN": "firma"})
		srv.On(http.MethodGet, "/info", status, nil)
		if code := run(t, app, "auth", "status", "--jq", `.context | error("kaputt")`); code != exit {
			t.Errorf("%d: exit = %d, want %d", status, code, exit)
		}
		_, errs := bufs(app)
		if !strings.Contains(errs, "Warnung: Ausgabe fehlgeschlagen: error: kaputt") || strings.Contains(errs, "Befehl ausgeführt") {
			t.Errorf("%d: stderr = %q", status, errs)
		}
		assertNoToken(t, app)
	}
	// without a token nothing is sent: exit 3 as well
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	if code := run(t, app, "auth", "status", "--jq", `.context | error("kaputt")`); code != 3 {
		t.Errorf("ohne Token: exit = %d, want 3", code)
	}
	// the call succeeded: the output error is the error (exit 1)
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-x", "PAPIERKRAM_SUBDOMAIN": "firma"})
	srv.On(http.MethodGet, "/info", http.StatusOK, []byte(`{}`))
	if code := run(t, app, "auth", "status", "--jq", `.context | error("kaputt")`); code != 1 {
		t.Errorf("Erfolg: exit = %d, want 1", code)
	}
	if _, errs := bufs(app); !strings.Contains(errs, "Befehl ausgeführt, Ausgabe fehlgeschlagen: error: kaputt") {
		t.Errorf("Erfolg: stderr = %q", errs)
	}
}
