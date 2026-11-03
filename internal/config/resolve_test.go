package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func testConfig() *Config {
	return &Config{
		ActiveContext: "active",
		Contexts: map[string]*Context{
			"active": {Subdomain: "sub-active", Token: "test-token-cfg-active"},
			"flag":   {Subdomain: "sub-flag"},
			"envctx": {Subdomain: "sub-env"},
			"proj":   {Subdomain: "sub-proj"},
			"ro":     {Subdomain: "sub-ro", ReadOnly: true},
			"rw":     {Subdomain: "sub-rw", ReadOnly: false},
		},
	}
}

func TestResolveContextPriority(t *testing.T) {
	tests := []struct {
		name         string
		flag, proj   string
		env          map[string]string
		wantCtx, src string
	}{
		{"flag wins over all", "flag", "proj", map[string]string{"PAPIERKRAM_CONTEXT": "envctx"}, "flag", "flag"},
		{"env over project and active", "", "proj", map[string]string{"PAPIERKRAM_CONTEXT": "envctx"}, "envctx", "env"},
		{"project over active", "", "proj", nil, "proj", "projectfile"},
		{"active", "", "", nil, "active", "config"},
		{"flag over active only", "flag", "", nil, "flag", "flag"},
		{"flag over env only", "flag", "", map[string]string{"PAPIERKRAM_CONTEXT": "envctx"}, "flag", "flag"},
		{"env over active only", "", "", map[string]string{"PAPIERKRAM_CONTEXT": "envctx"}, "envctx", "env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Resolve(testConfig(), ResolveInput{FlagContext: tc.flag, ProjectContext: tc.proj, Env: env(tc.env), Store: auth.NewMemory()})
			if err != nil {
				t.Fatal(err)
			}
			if r.Context != tc.wantCtx || r.ContextSource != tc.src {
				t.Fatalf("got %q/%q", r.Context, r.ContextSource)
			}
			if r.Subdomain != "sub-"+map[string]string{"flag": "flag", "envctx": "env", "proj": "proj", "active": "active"}[tc.wantCtx] {
				t.Fatalf("subdomain %q", r.Subdomain)
			}
		})
	}
}

func TestResolveNoContext(t *testing.T) {
	r, err := Resolve(&Config{}, ResolveInput{Env: env(nil), Store: auth.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	if r.Context != "" || r.ContextSource != "" || r.Token != "" || r.TokenSource != "" || r.BaseURL != "" || r.Subdomain != "" {
		t.Fatalf("%+v", r)
	}
	if r.Timeout != 30*time.Second || !r.CompletionLookup {
		t.Fatalf("defaults: %v %v", r.Timeout, r.CompletionLookup)
	}
	// nil config and nil env/store tolerated
	if _, err := Resolve(nil, ResolveInput{}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveUnknownContext(t *testing.T) {
	_, err := Resolve(testConfig(), ResolveInput{FlagContext: "x", Env: env(nil)})
	wantCode(t, err, clierr.CodeNotFound)
	ce := clierr.From(err)
	if ce.Message != `Kontext "x" nicht gefunden` || !strings.Contains(ce.Hint, "active") || !strings.Contains(ce.Hint, "papierkram context create") {
		t.Fatalf("%q / %q", ce.Message, ce.Hint)
	}
	_, err = Resolve(&Config{}, ResolveInput{FlagContext: "x", Env: env(nil)})
	wantCode(t, err, clierr.CodeNotFound)
}

func TestResolveDirectModeWithoutConfig(t *testing.T) {
	r, err := Resolve(&Config{}, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_TOKEN": "test-token-env", "PAPIERKRAM_SUBDOMAIN": "acme"})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Token != "test-token-env" || r.TokenSource != "env" || r.Subdomain != "acme" || r.BaseURL != "https://acme.papierkram.de/api/v1" || r.Context != "" {
		t.Fatalf("%+v", r)
	}
}

func TestResolveSubdomainAndBaseURL(t *testing.T) {
	cfg := testConfig()
	r, _ := Resolve(cfg, ResolveInput{Env: env(nil)})
	if r.Subdomain != "sub-active" || r.BaseURL != "https://sub-active.papierkram.de/api/v1" {
		t.Fatalf("%+v", r)
	}
	r, _ = Resolve(cfg, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_SUBDOMAIN": "other"})})
	if r.Subdomain != "other" || r.BaseURL != "https://other.papierkram.de/api/v1" {
		t.Fatalf("%+v", r)
	}
	r, _ = Resolve(cfg, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_SUBDOMAIN": "other", "PAPIERKRAM_BASE_URL": "http://127.0.0.1:1/api/v1"})})
	if r.BaseURL != "http://127.0.0.1:1/api/v1" || r.Subdomain != "other" {
		t.Fatalf("%+v", r)
	}
	// base URL alone, no context at all
	r, _ = Resolve(&Config{}, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_BASE_URL": "http://x"})})
	if r.BaseURL != "http://x" {
		t.Fatalf("%+v", r)
	}
}

type errStore struct{ err error }

func (s errStore) Get(string) (string, error) { return "", s.err }
func (s errStore) Set(string, string) error   { return s.err }
func (s errStore) Delete(string) error        { return s.err }

func TestResolveTokenPriority(t *testing.T) {
	cfg := testConfig()
	mem := auth.NewMemory()
	_ = mem.Set("active", "test-token-keychain")

	r, _ := Resolve(cfg, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"}), Store: mem})
	if r.Token != "test-token-env" || r.TokenSource != "env" {
		t.Fatalf("env: %q %q", r.Token, r.TokenSource)
	}
	r, _ = Resolve(cfg, ResolveInput{Env: env(nil), Store: mem})
	if r.Token != "test-token-keychain" || r.TokenSource != "keychain" {
		t.Fatalf("keychain: %q %q", r.Token, r.TokenSource)
	}
	r, _ = Resolve(cfg, ResolveInput{Env: env(nil), Store: auth.NewMemory()}) // ErrNotFound
	if r.Token != "test-token-cfg-active" || r.TokenSource != "config" {
		t.Fatalf("config: %q %q", r.Token, r.TokenSource)
	}
	r, err := Resolve(cfg, ResolveInput{Env: env(nil), Store: errStore{auth.ErrUnavailable}})
	if err != nil || r.Token != "test-token-cfg-active" || r.TokenSource != "config" {
		t.Fatalf("unavailable: %+v %v", r, err)
	}
	// no token anywhere is not an error
	r, err = Resolve(cfg, ResolveInput{FlagContext: "flag", Env: env(nil), Store: auth.NewMemory()})
	if err != nil || r.Token != "" || r.TokenSource != "" {
		t.Fatalf("none: %+v %v", r, err)
	}
	// nil store falls back to config
	r, _ = Resolve(cfg, ResolveInput{Env: env(nil)})
	if r.TokenSource != "config" {
		t.Fatalf("nil store: %q", r.TokenSource)
	}
}

func TestResolveOtherStoreErrorIsWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := Resolve(testConfig(), ResolveInput{Env: env(nil), Store: errStore{boom}})
	if err == nil || !errors.Is(err, boom) || !strings.Contains(err.Error(), "active") {
		t.Fatalf("%v", err)
	}
	// env token short-circuits the store entirely
	if _, err := Resolve(testConfig(), ResolveInput{Env: env(map[string]string{"PAPIERKRAM_TOKEN": "t"}), Store: errStore{boom}}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveReadOnly(t *testing.T) {
	tests := []struct {
		name, ctx, envRO string
		want             bool
		src              string
	}{
		{"env overrides rw context", "rw", "1", true, "env"},
		{"env with ro context -> env", "ro", "1", true, "env"},
		{"ro context", "ro", "", true, "context"},
		{"rw context", "rw", "", false, ""},
		{"env 0 is off", "rw", "0", false, ""},
		{"env false is off", "rw", "false", false, ""},
		{"env empty is off", "rw", "", false, ""},
		{"env trailing space enables", "rw", "1 ", true, "env"},
		{"env true word enables", "rw", "TRUE", true, "env"},
		{"env unknown word enables", "rw", "ja", true, "env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Resolve(testConfig(), ResolveInput{FlagContext: tc.ctx, Env: env(map[string]string{"PAPIERKRAM_READ_ONLY": tc.envRO})})
			if err != nil {
				t.Fatal(err)
			}
			if r.ReadOnly != tc.want || r.ReadOnlySource != tc.src {
				t.Fatalf("got %v/%q", r.ReadOnly, r.ReadOnlySource)
			}
		})
	}
	// env applies without any context too
	r, _ := Resolve(&Config{}, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_READ_ONLY": "1"})})
	if !r.ReadOnly || r.ReadOnlySource != "env" {
		t.Fatalf("%+v", r)
	}
}

func TestResolveTimeoutOutputUserCompletion(t *testing.T) {
	no := false
	cfg := &Config{ActiveContext: "a", Contexts: map[string]*Context{"a": {Subdomain: "a", Timeout: "1m30s", Output: "json", User: 5, CompletionLookup: &no}}}
	r, err := Resolve(cfg, ResolveInput{Env: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 90*time.Second || r.Output != "json" || r.User != 5 || r.CompletionLookup {
		t.Fatalf("%+v", r)
	}
	for _, bad := range []string{"abc", "30", "-5s", "0s"} {
		cfg.Contexts["a"].Timeout = bad
		_, err := Resolve(cfg, ResolveInput{Env: env(nil)})
		wantCode(t, err, clierr.CodeUsage)
	}
}

func TestResolvedNeverPrintsToken(t *testing.T) {
	const tok = "test-token-secret-123"
	r, err := Resolve(testConfig(), ResolveInput{Env: env(map[string]string{"PAPIERKRAM_TOKEN": tok})})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{r, *r, &r} {
		for _, f := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d"} {
			if out := fmt.Sprintf(f, v); strings.Contains(out, tok) {
				t.Errorf("%s leaks token: %s", f, out)
			}
		}
		if strings.Contains(fmt.Sprint(v), tok) || strings.Contains(fmt.Sprintln(v), tok) {
			t.Error("Sprint leaks token")
		}
	}
	if s := r.String(); !strings.Contains(s, "***") || !strings.Contains(s, "sub-active") {
		t.Fatalf("String: %s", s)
	}
	if r.Token != tok {
		t.Fatal("field must stay intact")
	}
	empty := &Resolved{}
	if strings.Contains(empty.String(), "***") {
		t.Fatalf("empty token must not show mask: %s", empty.String())
	}
}

func TestResolveDirectModeSkipsUnknownImplicitContext(t *testing.T) {
	cfg := &Config{ActiveContext: "gone", Contexts: map[string]*Context{}}
	direct := map[string]string{"PAPIERKRAM_TOKEN": "test-token-env", "PAPIERKRAM_SUBDOMAIN": "acme"}
	directBase := map[string]string{"PAPIERKRAM_TOKEN": "test-token-env", "PAPIERKRAM_BASE_URL": "http://127.0.0.1:1/api/v1"}

	for name, in := range map[string]ResolveInput{
		"active_context + subdomain": {Env: env(direct)},
		"active_context + base url":  {Env: env(directBase)},
		"project file + subdomain":   {ProjectContext: "gone", Env: env(direct)},
		"project file + base url":    {ProjectContext: "gone", Env: env(directBase)},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := Resolve(cfg, in)
			if err != nil {
				t.Fatalf("direct mode must ignore the unknown implicit context: %v", err)
			}
			if r.Context != "" || r.ContextSource != "" || r.Token != "test-token-env" || r.TokenSource != SourceEnv || r.BaseURL == "" {
				t.Fatalf("%+v", r)
			}
		})
	}
	for name, in := range map[string]ResolveInput{
		"explicit flag":           {FlagContext: "gone", Env: env(direct)},
		"explicit env":            {Env: env(map[string]string{"PAPIERKRAM_TOKEN": "test-token-env", "PAPIERKRAM_SUBDOMAIN": "acme", "PAPIERKRAM_CONTEXT": "gone"})},
		"token without subdomain": {Env: env(map[string]string{"PAPIERKRAM_TOKEN": "test-token-env"})},
		"subdomain without token": {Env: env(map[string]string{"PAPIERKRAM_SUBDOMAIN": "acme"})},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(cfg, in)
			wantCode(t, err, clierr.CodeNotFound)
		})
	}
}

func TestResolvedTokenNotSerialized(t *testing.T) {
	const tok = "test-token-serialize-123"
	r, err := Resolve(testConfig(), ResolveInput{Env: env(map[string]string{"PAPIERKRAM_TOKEN": tok})})
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	y, err := yaml.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for kind, out := range map[string][]byte{"json": j, "yaml": y} {
		if strings.Contains(string(out), tok) {
			t.Errorf("%s leaks token: %s", kind, out)
		}
		if !strings.Contains(string(out), "sub-active") {
			t.Errorf("%s lacks other fields: %s", kind, out)
		}
	}
}
