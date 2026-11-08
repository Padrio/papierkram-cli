package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// L77: Save flushes the temporary file before the rename and, except on
// Windows, the directory after it, so a crash leaves the old or the new
// file. A failed file sync keeps the old file; a failed directory sync is
// no error (the new file is in place, callers must not roll back).
func TestSaveSyncsFileThenDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "papierkram", "config.toml")
	origFile, origDir := syncFile, syncDir
	t.Cleanup(func() { syncFile, syncDir = origFile, origDir })
	var calls []string
	syncFile = func(f *os.File) error {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("file synced after the rename: %v", err)
		}
		calls = append(calls, "file")
		return origFile(f)
	}
	syncDir = func(dir string) error {
		if dir != filepath.Dir(path) {
			t.Errorf("synced directory %q, want %q", dir, filepath.Dir(path))
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("directory synced before the rename: %v", err)
		}
		calls = append(calls, "dir")
		return origDir(dir)
	}
	if err := (&Config{ActiveContext: "alt"}).Save(path); err != nil {
		t.Fatal(err)
	}
	want := []string{"file", "dir"}
	if runtime.GOOS == "windows" {
		want = want[:1]
	}
	if !slices.Equal(calls, want) {
		t.Errorf("syncs = %v, want %v", calls, want)
	}
	old, _ := os.ReadFile(path)

	syncFile = func(*os.File) error { return errors.New("E/A-Fehler") }
	err := (&Config{ActiveContext: "neu"}).Save(path)
	if err == nil || !strings.Contains(err.Error(), "E/A-Fehler") {
		t.Errorf("failed file sync: %v", err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(old) {
		t.Errorf("file replaced although the sync failed:\n%s", now)
	}
	if es, _ := os.ReadDir(filepath.Dir(path)); len(es) != 1 {
		t.Errorf("leftovers: %v", es)
	}

	syncFile = origFile
	syncDir = func(string) error { return errors.New("nicht unterstützt") }
	if err := (&Config{ActiveContext: "neu"}).Save(path); err != nil {
		t.Errorf("failed directory sync: %v", err)
	}
	if c, err := Load(path); err != nil || c.ActiveContext != "neu" {
		t.Errorf("after a failed directory sync: %+v, %v", c, err)
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

// A context name with a control or invisible format character is
// rejected: names from a project file of a cloned repository are printed
// in messages and must not carry terminal escape sequences.
func TestValidateNameRejectsControlCharacters(t *testing.T) {
	for _, name := range []string{"a\x1b[31mb", "a\u202eb", "a\x00b"} {
		err := ValidateName(name)
		var ce *clierr.Error
		if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || strings.ContainsAny(ce.Message, "\x1b\u202e\x00") {
			t.Errorf("ValidateName(%q) = %v", name, err)
		}
	}
	if err := ValidateName("firma-gmbh"); err != nil {
		t.Errorf("valid name: %v", err)
	}
}
