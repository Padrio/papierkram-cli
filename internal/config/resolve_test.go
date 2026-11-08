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

// prioritySources are the four sources of the context in the order of
// SelectContext; each names its own context, whose output, timeout and
// read-only option differ from the others.
var prioritySources = []struct {
	source, context, output string
	timeout                 time.Duration
	readOnly                bool
}{
	{SourceFlag, "flag", "json", 5 * time.Second, false},
	{SourceEnv, "envctx", "yaml", 10 * time.Second, true},
	{SourceProjectFile, "proj", "wide", 15 * time.Second, false},
	{SourceConfig, "active", "table", 20 * time.Second, true},
}

// L77: the resolution order over every combination of the sources flag,
// PAPIERKRAM_CONTEXT, project file and active_context, each with and
// without PAPIERKRAM_READ_ONLY: the first source set selects the context,
// output and timeout come from that context (default 30s without one),
// read-only from the environment, else from that context. (-o and
// --timeout beat the context in internal/cli: TestOutputFormatPrecedence,
// TestClientTimeoutPrecedence.)
func TestResolveContextPriority(t *testing.T) {
	cfg := &Config{Contexts: map[string]*Context{}}
	for _, s := range prioritySources {
		cfg.Contexts[s.context] = &Context{Subdomain: "sub-" + s.context, Output: s.output, Timeout: s.timeout.String(), ReadOnly: s.readOnly}
	}
	cases := 0
	for set := range 1 << len(prioritySources) {
		for _, roEnv := range []string{"", "1"} {
			var in ResolveInput
			e := map[string]string{"PAPIERKRAM_READ_ONLY": roEnv}
			c := *cfg
			c.ActiveContext = ""
			var names []string
			for i, s := range prioritySources {
				if set&(1<<i) == 0 {
					continue
				}
				names = append(names, s.source)
				switch s.source {
				case SourceFlag:
					in.FlagContext = s.context
				case SourceEnv:
					e["PAPIERKRAM_CONTEXT"] = s.context
				case SourceProjectFile:
					in.ProjectContext = s.context
				case SourceConfig:
					c.ActiveContext = s.context
				}
			}
			in.Env, in.Store = env(e), auth.NewMemory()
			name := strings.Join(names, "+")
			if name == "" {
				name = "keine"
			}
			if roEnv != "" {
				name += "/READ_ONLY"
			}
			t.Run(name, func(t *testing.T) {
				cases++
				want := Resolved{Timeout: defaultTimeout}
				for i, s := range prioritySources {
					if set&(1<<i) != 0 {
						want.Context, want.ContextSource, want.Subdomain = s.context, s.source, "sub-"+s.context
						want.Output, want.Timeout = s.output, s.timeout
						if s.readOnly {
							want.ReadOnly, want.ReadOnlySource = true, SourceContext
						}
						break
					}
				}
				if roEnv != "" {
					want.ReadOnly, want.ReadOnlySource = true, SourceEnv
				}
				r, err := Resolve(&c, in)
				if err != nil {
					t.Fatal(err)
				}
				if r.Context != want.Context || r.ContextSource != want.ContextSource || r.Subdomain != want.Subdomain ||
					r.Output != want.Output || r.Timeout != want.Timeout ||
					r.ReadOnly != want.ReadOnly || r.ReadOnlySource != want.ReadOnlySource {
					t.Errorf("got  %v\nwant %v", r, want)
				}
			})
		}
	}
	if cases != 32 {
		t.Errorf("%d Kombinationen geprüft, erwartet 32", cases)
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
	for _, v := range []any{r, *r} {
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

// SelectContext is the selection step of Resolve, usable without the
// context having to exist (auth login creates it).
func TestSelectContext(t *testing.T) {
	c := &Config{ActiveContext: "active"}
	tests := []struct {
		flag, proj   string
		env          map[string]string
		wantCtx, src string
	}{
		{"f", "p", map[string]string{"PAPIERKRAM_CONTEXT": "e"}, "f", SourceFlag},
		{"", "p", map[string]string{"PAPIERKRAM_CONTEXT": "e"}, "e", SourceEnv},
		{"", "p", nil, "p", SourceProjectFile},
		{"", "", nil, "active", SourceConfig},
	}
	for _, tt := range tests {
		name, src := SelectContext(c, ResolveInput{FlagContext: tt.flag, ProjectContext: tt.proj, Env: env(tt.env)})
		if name != tt.wantCtx || src != tt.src {
			t.Errorf("%+v: got %q/%q, want %q/%q", tt, name, src, tt.wantCtx, tt.src)
		}
	}
	if name, src := SelectContext(&Config{}, ResolveInput{}); name != "" || src != "" {
		t.Errorf("empty: got %q/%q", name, src)
	}
	if name, _ := SelectContext(nil, ResolveInput{}); name != "" {
		t.Errorf("nil config: got %q", name)
	}
}

// Controller ruling N1: the base URL in the printed form of Resolved has
// no userinfo, also when it cannot be parsed.
func TestResolvedNeverPrintsBaseURLCredentials(t *testing.T) {
	for _, u := range []string{"http://alice:s3cret@127.0.0.1:1/api/v1", "http://alice:s3cret@127.0.0.1:badport/api/v1"} {
		r := Resolved{BaseURL: u}
		for _, s := range []string{r.String(), fmt.Sprintf("%v %+v %#v %s", r, r, r, r)} {
			if strings.Contains(s, "alice") || strings.Contains(s, "s3cret") {
				t.Errorf("%s", s)
			}
		}
	}
}

// A base URL without https sends the token in clear text, unless it stays
// on this machine (loopback: tests, local mocks).
func TestInsecureBaseURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://firma.papierkram.de/api/v1":    false,
		"HTTPS://firma.papierkram.de/api/v1":    false,
		"http://firma.papierkram.de/api/v1":     true,
		"HTTP://firma.papierkram.de/api/v1":     true,
		"http://user:pw@firma.example/api/v1":   true,
		"ftp://firma.example/api/v1":            true,
		"http://10.0.0.1:8080/api/v1":           true,
		"http://127.0.0.1:8080/api/v1":          false,
		"http://127.9.8.7/api/v1":               false,
		"http://localhost:3000/api/v1":          false,
		"http://LOCALHOST/api/v1":               false,
		"http://localhost./api/v1":              false,
		"http://[::1]:8080/api/v1":              false,
		"http://localhost.firma.example/api/v1": true,
		"http://127.0.0.1.firma.example/api/v1": true,
		"":                                      false,
		"://kaputt":                             false, // reported where it is used
	} {
		if got := InsecureBaseURL(raw); got != want {
			t.Errorf("InsecureBaseURL(%q) = %t, want %t", raw, got, want)
		}
	}
}
