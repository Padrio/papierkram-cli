package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// mockApp returns a test app whose PAPIERKRAM_BASE_URL points to a fresh
// mock server; extra env values are added to the environment.
func mockApp(t *testing.T, extra map[string]string) (*App, *mockapi.Server) {
	t.Helper()
	app, _, _ := newTestApp(t)
	srv := mockapi.New(t)
	env := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()}
	for k, v := range extra {
		env[k] = v
	}
	app.Env = envMap(env)
	return app, srv
}

func bufs(app *App) (string, string) {
	return app.Out.(interface{ String() string }).String(), app.Err.(interface{ String() string }).String()
}

func assertNoToken(t *testing.T, app *App) {
	t.Helper()
	out, errs := bufs(app)
	if strings.Contains(out, "test-token") || strings.Contains(errs, "test-token") {
		t.Errorf("Token in Ausgabe:\nstdout: %s\nstderr: %s", out, errs)
	}
}

func TestClientResolvesContextAndCallsAPI(t *testing.T) {
	app, srv := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("a", "test-token-a"); err != nil {
		t.Fatal(err)
	}
	c, r, err := app.Client(nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Context != "a" || r.ContextSource != "config" || r.TokenSource != "keychain" {
		t.Errorf("resolved = %v", r)
	}
	if _, err := c.Call(context.Background(), "GetInfo", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "Bearer test-token-a" {
		t.Errorf("requests = %+v", reqs)
	}
	assertNoToken(t, app)
}

func TestClientUsesProjectFileContext(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".papierkram.toml"), []byte("context = \"b\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.Getwd = func() (string, error) { return dir, nil }
	if err := app.Keyring.Set("b", "test-token-b"); err != nil {
		t.Fatal(err)
	}
	_, r, err := app.Client(nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Context != "b" || r.ContextSource != "projectfile" || !r.ReadOnly || r.ReadOnlySource != "context" {
		t.Errorf("resolved = %v", r)
	}
}

func TestClientNoTokenIsAuthError(t *testing.T) {
	app, _ := mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	_, _, err := app.Client(nil)
	ce := clierr.From(err)
	if ce.Exit() != 3 || !strings.Contains(ce.Hint, "papierkram auth login") || !strings.Contains(ce.Hint, "PAPIERKRAM_TOKEN") {
		t.Errorf("err = %+v", ce)
	}
	if !strings.Contains(ce.Message, `"a"`) {
		t.Errorf("Meldung nennt den Kontext nicht: %q", ce.Message)
	}
}

func TestClientNoSubdomainIsUsageError(t *testing.T) {
	app, _, _ := newTestApp(t)
	_, _, err := app.Client(nil)
	ce := clierr.From(err)
	if ce.Exit() != 2 || !strings.Contains(ce.Hint, "papierkram context create") {
		t.Errorf("err = %+v", ce)
	}
}

func TestClientBrokenConfigGermanError(t *testing.T) {
	app, _, _ := newTestApp(t)
	writeCfg(t, app, "active_context = \n")
	_, _, err := app.Client(nil)
	ce := clierr.From(err)
	if ce.Exit() != 1 || !strings.Contains(ce.Hint, "Konfigurationsdatei prüfen") {
		t.Errorf("err = %+v", ce)
	}
}

func TestClientInvalidContextOutput(t *testing.T) {
	app, _, _ := newTestApp(t)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\noutput = \"xml\"\n")
	_, _, err := app.Client(nil)
	ce := clierr.From(err)
	if ce.Exit() != 2 || !strings.Contains(ce.Message, `"xml"`) {
		t.Errorf("err = %+v", ce)
	}
}

func TestClientTimeoutPrecedence(t *testing.T) {
	cfg := "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\ntimeout = \"45s\"\n[contexts.b]\nsubdomain = \"firma-b\"\n"
	tests := []struct {
		name    string
		ctx     string
		flag    time.Duration
		flagSet bool
		want    time.Duration
	}{
		{"context wins over default", "a", 30 * time.Second, false, 45 * time.Second},
		{"explicit flag wins over context", "a", 5 * time.Second, true, 5 * time.Second},
		{"explicit flag equal to default still wins", "a", 30 * time.Second, true, 30 * time.Second},
		{"no context timeout", "b", 30 * time.Second, false, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-x"})
			writeCfg(t, app, cfg)
			app.Flags.Context, app.Flags.Timeout, app.Flags.TimeoutSet = tt.ctx, tt.flag, tt.flagSet
			r, err := app.resolveConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got := app.clientOptions(r).Timeout; got != tt.want {
				t.Errorf("timeout = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOutputFormatPrecedence(t *testing.T) {
	app, _, _ := newTestApp(t)
	app.Flags.Output = "table"
	if got := app.outputFormat(nil); got != "table" {
		t.Errorf("default = %q", got)
	}
	r := &config.Resolved{Output: "json"}
	if got := app.outputFormat(r); got != "json" {
		t.Errorf("context = %q", got)
	}
	app.Flags.Output, app.Flags.OutputSet = "yaml", true
	if got := app.outputFormat(r); got != "yaml" {
		t.Errorf("flag = %q", got)
	}
}

// M4: env credentials silently overriding an explicitly chosen context are
// reported on stderr.
func TestClientWarnsWhenEnvOverridesExplicitContext(t *testing.T) {
	tests := []struct {
		name     string
		flagCtx  string
		env      map[string]string
		wantVars []string
	}{
		{"flag + token", "a", map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"}, []string{"PAPIERKRAM_TOKEN"}},
		{"env context + subdomain", "", map[string]string{"PAPIERKRAM_CONTEXT": "a", "PAPIERKRAM_SUBDOMAIN": "andere"}, []string{"PAPIERKRAM_SUBDOMAIN"}},
		{"flag + base url + token", "a", map[string]string{"PAPIERKRAM_BASE_URL": "http://127.0.0.1:1/api/v1", "PAPIERKRAM_TOKEN": "test-token-env"}, []string{"PAPIERKRAM_TOKEN", "PAPIERKRAM_BASE_URL"}},
		{"implicit context", "", map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"}, nil},
		{"explicit without env", "a", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, errb := newTestApp(t)
			app.Env = envMap(tt.env)
			writeCfg(t, app, twoCtx)
			app.Flags.Context = tt.flagCtx
			if _, err := app.resolveConfig(); err != nil {
				t.Fatal(err)
			}
			got := errb.String()
			if len(tt.wantVars) == 0 {
				if got != "" {
					t.Errorf("unerwartete Warnung: %s", got)
				}
				return
			}
			if strings.Count(got, "Warnung:") != 1 || !strings.Contains(got, `Kontext "a"`) {
				t.Errorf("stderr = %q", got)
			}
			for _, v := range tt.wantVars {
				if !strings.Contains(got, v) {
					t.Errorf("Warnung nennt %s nicht: %q", v, got)
				}
			}
			assertNoToken(t, app)
		})
	}
}

func TestQuotaWarningOncePerInvocation(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-q", "PAPIERKRAM_SUBDOMAIN": "firma"})
	srv.OnFunc(http.MethodGet, "/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Remaining-Quota", "50")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api":{"version":"1.0.0"}}`))
	})
	for range 2 {
		c, _, err := app.Client(nil)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := c.Call(context.Background(), "GetInfo", nil, nil, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, errs := bufs(app)
	if n := strings.Count(errs, "Warnung: Restkontingent niedrig: 50 Credits"); n != 1 {
		t.Errorf("Warnung %d-mal, erwartet 1: %q", n, errs)
	}
	if app.quota != (api.Quota{Remaining: 50, RequestCost: 1, Known: true, CostKnown: true}) {
		t.Errorf("quota = %+v", app.quota)
	}
}

func TestQuotaNoWarningAt100(t *testing.T) {
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-q", "PAPIERKRAM_SUBDOMAIN": "firma"})
	srv.On(http.MethodGet, "/info", http.StatusOK, []byte(`{}`), "X-Remaining-Quota", "100")
	c, _, err := app.Client(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(context.Background(), "GetInfo", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, errs := bufs(app); strings.Contains(errs, "Restkontingent") {
		t.Errorf("Warnung bei 100: %q", errs)
	}
}

func TestDebugReadOnlyLevels(t *testing.T) {
	app, _, _ := newTestApp(t)
	writeCfg(t, app, twoCtx)
	for ctx, want := range map[string]string{"a": "Schreibschutz: aus", "b": "Schreibschutz: context"} {
		app.Flags.Context = ctx
		r, err := app.resolveConfig()
		if err != nil {
			t.Fatal(err)
		}
		if got := app.debugInfo(r); !strings.Contains(got, want) {
			t.Errorf("%s: %q enthält nicht %q", ctx, got, want)
		}
	}
}

func TestTimeoutHelpSaysPerAttempt(t *testing.T) {
	app, out, _ := newTestApp(t)
	if code := Run(app, []string{"--help"}); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "pro Versuch; GET wird bis zu zweimal wiederholt") {
		t.Errorf("--timeout-Hilfe:\n%s", out.String())
	}
}

// A base URL without https outside loopback gets a warning on stderr that
// names the URL without userinfo and never the token (M4). No request
// leaves the test: the transport answers itself.
func TestInsecureBaseURLWarning(t *testing.T) {
	for base, warn := range map[string]bool{
		"http://nutzer:geheim@firma.example/api/v1": true,
		"https://firma.example/api/v1":              false,
		"http://127.0.0.1:9/api/v1":                 false,
		"http://localhost:9/api/v1":                 false,
		"http://[::1]:9/api/v1":                     false,
	} {
		app, _, errb := newTestApp(t)
		app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": base, "PAPIERKRAM_TOKEN": "test-token-insecure"})
		var sent int
		app.HTTPClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			sent++
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":1}`)), Request: r}, nil
		})}
		if code := Run(app, []string{"project", "describe", "1", "-o", "json"}); code != 0 {
			t.Fatalf("%s: exit %d: %s", base, code, errb.String())
		}
		errs := errb.String()
		got := strings.Contains(errs, "kein https")
		if got != warn || sent != 1 {
			t.Errorf("%s: warning %t (want %t), %d requests: %q", base, got, warn, sent, errs)
		}
		if warn && (!strings.Contains(errs, "http://firma.example/api/v1") || strings.Count(errs, "kein https") != 1) {
			t.Errorf("%s: stderr = %q", base, errs)
		}
		if strings.Contains(errs, "geheim") || strings.Contains(errs, "nutzer") {
			t.Errorf("%s: userinfo in stderr: %q", base, errs)
		}
		assertNoToken(t, app)
	}
}

// A base URL without "//" (http:nutzer:geheim@host) is insecure as well;
// its credentials sit in the opaque part and are masked in the warning
// (N4), whatever the request then does.
func TestInsecureBaseURLWarningMasksOpaqueCredentials(t *testing.T) {
	app, _, errb := newTestApp(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": "http:nutzer:geheim@firma.example/api/v1", "PAPIERKRAM_TOKEN": "test-token-insecure"})
	app.HTTPClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":1}`)), Request: r}, nil
	})}
	_ = Run(app, []string{"project", "describe", "1", "-o", "json"})
	errs := errb.String()
	if !strings.Contains(errs, "Basis-URL http:***@firma.example/api/v1 nutzt kein https") {
		t.Errorf("stderr = %q", errs)
	}
	if strings.Contains(errs, "geheim") || strings.Contains(errs, "nutzer") {
		t.Errorf("credentials in stderr: %q", errs)
	}
	assertNoToken(t, app)
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
