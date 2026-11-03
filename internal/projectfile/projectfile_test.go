package projectfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFind_CwdHit(t *testing.T) {
	root := t.TempDir()
	want := write(t, root, FileName, "")
	got, ok := Find(root)
	if !ok || got != want {
		t.Fatalf("Find = %q, %v; want %q", got, ok, want)
	}
}

func TestFind_WalksUpToGitRootDir(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".git"))
	want := write(t, root, FileName, "")
	sub := mkdir(t, filepath.Join(root, "a", "b"))
	got, ok := Find(sub)
	if !ok || got != want {
		t.Fatalf("Find = %q, %v; want %q", got, ok, want)
	}
}

func TestFind_GitFileMarksRoot(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".git", "gitdir: /elsewhere\n")
	want := write(t, root, FileName, "")
	sub := mkdir(t, filepath.Join(root, "x"))
	got, ok := Find(sub)
	if !ok || got != want {
		t.Fatalf("Find = %q, %v; want %q", got, ok, want)
	}
}

func TestFind_NearestWins(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".git"))
	write(t, root, FileName, "")
	sub := filepath.Join(root, "a")
	want := write(t, sub, FileName, "")
	got, ok := Find(mkdir(t, filepath.Join(sub, "b")))
	if !ok || got != want {
		t.Fatalf("Find = %q, %v; want %q", got, ok, want)
	}
}

func TestFind_NeverAboveGitRoot(t *testing.T) {
	outer := t.TempDir()
	write(t, outer, FileName, "")
	repo := mkdir(t, filepath.Join(outer, "repo"))
	mkdir(t, filepath.Join(repo, ".git"))
	sub := mkdir(t, filepath.Join(repo, "sub"))
	if got, ok := Find(sub); ok {
		t.Fatalf("Find above git root: %q", got)
	}
}

func TestFind_OutsideGitOnlyCwd(t *testing.T) {
	outer := t.TempDir()
	write(t, outer, FileName, "")
	sub := mkdir(t, filepath.Join(outer, "sub"))
	if got, ok := Find(sub); ok {
		t.Fatalf("Find outside git walked up: %q", got)
	}
}

func TestFind_DirectoryNamedLikeFileIgnored(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, FileName))
	if got, ok := Find(root); ok {
		t.Fatalf("directory matched: %q", got)
	}
}

func TestLoad_Full(t *testing.T) {
	p := write(t, t.TempDir(), FileName, `context = "firma-gmbh"
company = 12
project = 34
task = 56
user = 1
zeta = true
alpha = 1

[tasks]
tickets = 57
maintenance = 58
`)
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != p || f.Context != "firma-gmbh" || f.Company != 12 || f.Project != 34 || f.Task != 56 || f.User != 1 {
		t.Fatalf("unexpected file: %+v", f)
	}
	if f.Tasks["tickets"] != 57 || f.Tasks["maintenance"] != 58 {
		t.Fatalf("tasks: %v", f.Tasks)
	}
	if !reflect.DeepEqual(f.Unknown, []string{"alpha", "zeta"}) {
		t.Fatalf("Unknown = %v", f.Unknown)
	}
}

func TestLoad_Empty(t *testing.T) {
	f, err := Load(write(t, t.TempDir(), FileName, ""))
	if err != nil {
		t.Fatal(err)
	}
	if f.Company != 0 || len(f.Unknown) != 0 {
		t.Fatalf("%+v", f)
	}
}

func usageErr(t *testing.T, err error) *clierr.Error {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage {
		t.Fatalf("want usage error, got %v", err)
	}
	return ce
}

func TestLoad_TypeError(t *testing.T) {
	p := write(t, t.TempDir(), FileName, `project = "abc"`)
	_, err := Load(p)
	ce := usageErr(t, err)
	if !strings.Contains(ce.Message, p) || ce.Hint == "" {
		t.Fatalf("message/hint: %q / %q", ce.Message, ce.Hint)
	}
}

func TestLoad_SyntaxError(t *testing.T) {
	_, err := Load(write(t, t.TempDir(), FileName, "project = = 1"))
	_ = usageErr(t, err)
}

func TestLoad_InvalidAliasValues(t *testing.T) {
	for name, body := range map[string]string{
		"string": "[tasks]\ntickets = \"x\"",
		"zero":   "[tasks]\ntickets = 0",
		"neg":    "[tasks]\ntickets = -3",
		"float":  "[tasks]\ntickets = 1.5",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, t.TempDir(), FileName, body))
			ce := usageErr(t, err)
			if !strings.Contains(ce.Message, `"tickets"`) {
				t.Fatalf("alias not named: %q", ce.Message)
			}
		})
	}
}

func TestLoad_TasksNotTable(t *testing.T) {
	_, err := Load(write(t, t.TempDir(), FileName, "tasks = 5"))
	_ = usageErr(t, err)
}

func TestLoad_Missing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), FileName))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTask(t *testing.T) {
	f := &File{Tasks: map[string]int64{"tickets": 57, "maintenance": 58}}
	if id, err := f.ResolveTask("57"); err != nil || id != 57 {
		t.Fatalf("57 -> %d, %v", id, err)
	}
	if id, err := f.ResolveTask("tickets"); err != nil || id != 57 {
		t.Fatalf("tickets -> %d, %v", id, err)
	}
	_, err := f.ResolveTask("x")
	ce := usageErr(t, err)
	if ce.Message != `unbekannter Aufgaben-Alias "x"` {
		t.Fatalf("message: %q", ce.Message)
	}
	if !strings.Contains(ce.Hint, "maintenance, tickets") {
		t.Fatalf("hint: %q", ce.Hint)
	}
}

func TestResolveTask_NoAliases(t *testing.T) {
	for _, f := range []*File{{}, nil} {
		_, err := f.ResolveTask("x")
		ce := usageErr(t, err)
		if !strings.Contains(ce.Hint, "[tasks]") {
			t.Fatalf("hint: %q", ce.Hint)
		}
	}
	var f *File
	if id, err := f.ResolveTask("12"); err != nil || id != 12 {
		t.Fatalf("%d %v", id, err)
	}
}

func TestResolveTask_NonPositiveIsAliasLookup(t *testing.T) {
	f := &File{Tasks: map[string]int64{"a": 1}}
	_, err := f.ResolveTask("0")
	_ = usageErr(t, err)
}
