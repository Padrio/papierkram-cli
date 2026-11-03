package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func TestPath(t *testing.T) {
	p, err := Path("/x")
	if err != nil || p != filepath.Join("/x", "papierkram", "config.toml") {
		t.Fatalf("got %q, %v", p, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	t.Setenv("HOME", "/home/u")
	if p, err := Path(""); err != nil || filepath.Base(p) != "config.toml" {
		t.Fatalf("default: %q, %v", p, err)
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope", "config.toml"))
	if err != nil || c == nil || len(c.Contexts) != 0 || c.ActiveContext != "" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(p, []byte("active_context = ["), 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("want error")
	}
}

func TestSaveLoadRoundTripAndPerms(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "papierkram", "config.toml")
	yes := false
	c := &Config{}
	if err := c.Create("firma", &Context{Subdomain: "firma", ReadOnly: true, Output: "json", Timeout: "10s", User: 7, Token: "test-token-1", CompletionLookup: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := c.Use("firma"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("file perm %v", fi.Mode().Perm())
		}
		di, _ := os.Stat(filepath.Dir(p))
		if di.Mode().Perm() != 0o700 {
			t.Fatalf("dir perm %v", di.Mode().Perm())
		}
	}
	// no temp leftovers
	if es, _ := os.ReadDir(filepath.Dir(p)); len(es) != 1 {
		t.Fatalf("leftovers: %v", es)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	g := got.Contexts["firma"]
	if got.ActiveContext != "firma" || g == nil || g.Subdomain != "firma" || !g.ReadOnly || g.Output != "json" ||
		g.Timeout != "10s" || g.User != 7 || g.Token != "test-token-1" || g.CompletionLookup == nil || *g.CompletionLookup {
		t.Fatalf("round trip: %+v", g)
	}
}

func TestSaveOverwritesExistingWithLoosePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	if err := (&Config{}).Save(p); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", fi.Mode().Perm())
	}
}

func wantCode(t *testing.T, err error, code clierr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s error, got nil", code)
	}
	if ce := clierr.From(err); ce.Code != code {
		t.Fatalf("want %s, got %s (%v)", code, ce.Code, err)
	}
}

func TestContextNameValidation(t *testing.T) {
	for _, n := range []string{"", " ", "a b", "a.b", "a\tb", "a\nb"} {
		wantCode(t, (&Config{}).Create(n, &Context{}), clierr.CodeUsage)
	}
	for _, n := range []string{"firma-gmbh", "agent_analyse", "Firma1", "müller"} {
		if err := (&Config{}).Create(n, &Context{}); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
}

func TestCreateDuplicateUseDeleteRename(t *testing.T) {
	c := &Config{}
	_ = c.Create("a", &Context{Subdomain: "a"})
	wantCode(t, c.Create("a", &Context{}), clierr.CodeUsage)
	wantCode(t, c.Use("zzz"), clierr.CodeNotFound)
	wantCode(t, c.Delete("zzz"), clierr.CodeNotFound)
	wantCode(t, c.Rename("zzz", "b"), clierr.CodeNotFound)
	_ = c.Create("b", &Context{})
	wantCode(t, c.Rename("a", "b"), clierr.CodeUsage)
	wantCode(t, c.Rename("a", "x.y"), clierr.CodeUsage)

	_ = c.Use("a")
	if err := c.Rename("a", "c"); err != nil {
		t.Fatal(err)
	}
	if c.ActiveContext != "c" || c.Contexts["a"] != nil || c.Contexts["c"].Subdomain != "a" {
		t.Fatalf("rename: %+v", c)
	}
	// renaming a non-active context keeps active_context
	if err := c.Rename("b", "d"); err != nil || c.ActiveContext != "c" {
		t.Fatalf("%v %q", err, c.ActiveContext)
	}
	if err := c.Delete("c"); err != nil {
		t.Fatal(err)
	}
	if c.ActiveContext != "" || c.Contexts["c"] != nil {
		t.Fatalf("delete: %+v", c)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	tests := []struct {
		name, body          string
		wantMsg, wantInHint []string
	}{
		{"readonly in context", "[contexts.a]\nsubdomain = \"a\"\nreadonly = true\n",
			[]string{`unbekannter Schlüssel "readonly"`, "[contexts.a]"}, []string{`Meinten Sie "read_only"?`}},
		{"read-only in context", "[contexts.a]\nsubdomain = \"a\"\nread-only = true\n",
			[]string{`unbekannter Schlüssel "read-only"`, "[contexts.a]"}, []string{`Meinten Sie "read_only"?`}},
		{"typo top-level", "activ_context = \"a\"\n",
			[]string{`unbekannter Schlüssel "activ_context"`}, []string{`Meinten Sie "active_context"?`}},
		{"unrelated top-level", "foo = 1\n",
			[]string{`unbekannter Schlüssel "foo"`}, []string{"Erlaubte Schlüssel: active_context, contexts"}},
		{"unrelated in context", "[contexts.a]\nbanana = 1\n",
			[]string{`unbekannter Schlüssel "banana"`}, []string{"Erlaubte Schlüssel in [contexts.<name>]: subdomain, read_only, output, timeout, user, token, completion_lookup"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(p, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			wantCode(t, err, clierr.CodeUsage)
			ce := clierr.From(err)
			for _, w := range append(tc.wantMsg, p) {
				if !strings.Contains(ce.Message, w) {
					t.Errorf("message %q lacks %q", ce.Message, w)
				}
			}
			for _, w := range tc.wantInHint {
				if !strings.Contains(ce.Hint, w) {
					t.Errorf("hint %q lacks %q", ce.Hint, w)
				}
			}
			if strings.Contains(ce.Message+ce.Hint, "strict mode") {
				t.Errorf("english go-toml text leaked: %q / %q", ce.Message, ce.Hint)
			}
			if ce.Cause == nil {
				t.Error("original decode error must be kept as cause")
			}
		})
	}
	if _, err := Load(writeTemp(t, "[contexts.a]\nsubdomain = \"a\"\nread_only = true\n")); err != nil {
		t.Fatalf("known keys must load: %v", err)
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
