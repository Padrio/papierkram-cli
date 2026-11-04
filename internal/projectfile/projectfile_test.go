package projectfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestLoad_NumericAliasRejected(t *testing.T) {
	for _, alias := range []string{"57", "0", "-3", "+5"} {
		_, err := Load(write(t, t.TempDir(), FileName, "[tasks]\n\""+alias+"\" = 99\n"))
		ce := usageErr(t, err)
		if !strings.Contains(ce.Message, `"`+alias+`"`) || !strings.Contains(ce.Message, "Zahl") {
			t.Errorf("%s: message %q", alias, ce.Message)
		}
	}
	if _, err := Load(write(t, t.TempDir(), FileName, "[tasks]\nt57 = 99\n")); err != nil {
		t.Fatalf("alias with letters must load: %v", err)
	}
}

func TestLoad_ErrorsGermanWithCause(t *testing.T) {
	for name, tc := range map[string]struct{ body, key string }{
		"syntax":         {"project = = 1", ""},
		"string for int": {`project = "abc"`, `"project"`},
		"float for int":  {"company = 1.5", `"company"`},
		"zero id":        {"task = 0", `"task"`},
		"negative id":    {"user = -1", `"user"`},
		"int for string": {"context = 5", `"context"`},
		"table for int":  {"[project]\nx = 1", `"project"`},
	} {
		t.Run(name, func(t *testing.T) {
			p := write(t, t.TempDir(), FileName, tc.body)
			_, err := Load(p)
			ce := usageErr(t, err)
			text := strings.ReplaceAll(ce.Message, p, "<pfad>") + "\n" + ce.Hint
			for _, english := range []string{"toml:", "cannot", "decode", "expected", "invalid"} {
				if strings.Contains(text, english) {
					t.Errorf("English go-toml text %q in %q / %q", english, ce.Message, ce.Hint)
				}
			}
			if tc.key != "" && !strings.Contains(ce.Message, tc.key) {
				t.Errorf("key %s not named: %q", tc.key, ce.Message)
			}
			if tc.key == "" && (ce.Cause == nil || !strings.Contains(ce.Hint, "Zeile")) {
				t.Errorf("syntax error: cause %v, hint %q", ce.Cause, ce.Hint)
			}
		})
	}
}

func i64(v int64) *int64   { return &v }
func str(s string) *string { return &s }

// Save creates a missing file with the given keys only.
func TestSave_Creates(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	res, err := Save(p, Changes{Project: i64(34), Context: str("firma-gmbh")})
	if err != nil || !res.Created || !res.Changed || res.HadComments {
		t.Fatalf("Save = %+v, %v", res, err)
	}
	f, err := Load(p)
	if err != nil || f.Project != 34 || f.Context != "firma-gmbh" || f.Company != 0 || f.Task != 0 || f.User != 0 || len(f.Unknown) != 0 {
		t.Fatalf("Load = %+v, %v", f, err)
	}
}

// Save changes only the given keys; other keys, unknown keys and [tasks]
// stay. Comments cannot be kept and are reported.
func TestSave_UpdatesKeepsOtherKeys(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, FileName, "# Projekt X\ncontext = \"a\"\ncompany = 12 # Kunde\nproject = 34\nnotiz = \"bleibt\"\n\n[tasks]\ntickets = 57\n")
	res, err := Save(p, Changes{Project: i64(35), Task: i64(56)})
	if err != nil || res.Created || !res.Changed || !res.HadComments {
		t.Fatalf("Save = %+v, %v", res, err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := &File{Path: p, Context: "a", Company: 12, Project: 35, Task: 56, Tasks: map[string]int64{"tickets": 57}, Unknown: []string{"notiz"}}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("Load = %+v, want %+v", f, want)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "notiz = 'bleibt'") {
		t.Errorf("unknown key lost:\n%s", data)
	}
}

// Without a change the file is not written, so its comments stay.
func TestSave_UnchangedDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	content := "# Kommentar\nproject = 34\n"
	p := write(t, dir, FileName, content)
	res, err := Save(p, Changes{Project: i64(34)})
	if err != nil || res.Changed || res.Created {
		t.Fatalf("Save = %+v, %v", res, err)
	}
	if data, _ := os.ReadFile(p); string(data) != content {
		t.Errorf("file rewritten:\n%s", data)
	}
	if res, err := Save(filepath.Join(t.TempDir(), FileName), Changes{}); err != nil || res.Changed || res.Created {
		t.Errorf("empty changes on a missing file: %+v, %v", res, err)
	}
}

// Only real comments count: a # inside a string is none.
func TestHasComments(t *testing.T) {
	for content, want := range map[string]bool{
		"project = 34\n":                             false,
		"context = \"a#b\"\n":                        false,
		"context = 'a#b'\n":                          false,
		"notiz = \"\"\"\n# kein Kommentar\n\"\"\"\n": false,
		"# oben\nproject = 34\n":                     true,
		"project = 34 # hinten\n":                    true,
		"liste = [\n  1, # eins\n  2,\n]\n":          true,
		"[tasks] # Aliase\ntickets = 57\n":           true,
		"[tasks]\ntickets = 57\n# unten\n":           true,
	} {
		if got := hasComments([]byte(content)); got != want {
			t.Errorf("hasComments(%q) = %v, want %v", content, got, want)
		}
	}
}

// The write is atomic: no temporary files stay, and the mode of an
// existing file is kept.
func TestSave_AtomicKeepsMode(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, FileName, "project = 34\n")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(p, Changes{Project: i64(35)}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("files in dir: %v", entries)
	}
	if st, err := os.Stat(p); err == nil && runtime.GOOS != "windows" && st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
}

// A file that is no valid TOML is not overwritten.
func TestSave_InvalidFileKept(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, FileName, "project = \n")
	_, err := Save(p, Changes{Project: i64(35)})
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(p); string(data) != "project = \n" {
		t.Errorf("file changed: %q", data)
	}
}

// The context of a project file follows the rules of context names
// (config.ValidateName); an invalid one is a usage error naming the file.
func TestLoad_InvalidContext(t *testing.T) {
	for _, ctx := range []string{"firma.gmbh", "firma gmbh", `a\u001b[2Jb`, ""} {
		dir := t.TempDir()
		p := write(t, dir, FileName, "context = \""+ctx+"\"\n")
		_, err := Load(p)
		var ce *clierr.Error
		if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, p) || !strings.Contains(ce.Message, "context") {
			t.Errorf("context %q: err = %v", ctx, err)
			continue
		}
		if strings.Contains(ce.Message+ce.Hint, "\x1b") {
			t.Errorf("context %q: escape in message %q", ctx, ce.Message)
		}
	}
}

// Messages never echo control characters of the file: alias names in the
// hint of ResolveTask and the path are made printable.
func TestMessagesArePrintable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), hostileName("evil"))
	// Load rejects such aliases (TestLoad_AliasWithControlCharacters);
	// ResolveTask filters them anyway
	f := &File{Tasks: map[string]int64{"x\x1b[31m": 5}}
	_, err := f.ResolveTask("nix")
	var ce *clierr.Error
	if !errors.As(err, &ce) || strings.Contains(ce.Message+ce.Hint, "\x1b") {
		t.Errorf("ResolveTask: %q / %q", ce.Message, ce.Hint)
	}
	bad := write(t, dir, FileName, "project = \n")
	_, err = Load(bad)
	if !errors.As(err, &ce) || strings.Contains(ce.Message+ce.Hint, "\x1b") {
		t.Errorf("Load: %q / %q", ce.Message, ce.Hint)
	}
}

// Save never writes through a symbolic link (a cloned repository could
// point the project file at ~/.bashrc): it fails closed with a usage
// error and the target stays unchanged.
func TestSave_RefusesSymlink(t *testing.T) {
	outside := write(t, t.TempDir(), ".bashrc", "export PATH=/usr/bin\n")
	dir := t.TempDir()
	link := filepath.Join(dir, FileName)
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	_, err := Save(link, Changes{Project: i64(34)})
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, "symbolischer Link") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(outside); string(data) != "export PATH=/usr/bin\n" {
		t.Errorf("target changed: %q", data)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link replaced: %v %v", st, err)
	}
	// also a dangling link (no file yet behind it) is refused
	dangling := filepath.Join(t.TempDir(), FileName)
	target := filepath.Join(t.TempDir(), "neu")
	if err := os.Symlink(target, dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(dangling, Changes{Project: i64(34)}); !errors.As(err, &ce) || ce.Code != clierr.CodeUsage {
		t.Errorf("dangling: err = %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("file created behind a dangling link")
	}
}

// Security review: an alias with a control or invisible format character
// is rejected (exit 2), and the hint quotes the alias.
func TestLoad_AliasWithControlCharacters(t *testing.T) {
	for _, alias := range []string{`t\u001b[31m`, `t\u202eevil`} {
		p := write(t, t.TempDir(), FileName, "[tasks]\n\""+alias+"\" = 5\n")
		_, err := Load(p)
		var ce *clierr.Error
		if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage {
			t.Errorf("alias %s: err = %v", alias, err)
			continue
		}
		if strings.ContainsAny(ce.Message+ce.Hint, "\x1b\u202e") {
			t.Errorf("alias %s: raw in %q / %q", alias, ce.Message, ce.Hint)
		}
	}
	// the hint of an invalid alias value quotes the alias
	p := write(t, t.TempDir(), FileName, "[tasks]\ntickets = \"x\"\n")
	_, err := Load(p)
	var ce *clierr.Error
	if !errors.As(err, &ce) || !strings.Contains(ce.Hint, `"tickets"`) {
		t.Errorf("hint = %v", err)
	}
}

// Security review: a read error shows the cleaned path and only the
// underlying error, never the raw path of the wrapped *fs.PathError.
func TestLoad_ReadErrorPathPrintable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows erlaubt keine Steuerzeichen in Dateinamen")
	}
	dir := filepath.Join(t.TempDir(), "evil\x1b[2J")
	p := filepath.Join(mkdir(t, dir), FileName) // missing: reading fails
	_, err := Load(p)
	if err == nil || strings.Contains(err.Error(), "\x1b") || !strings.Contains(err.Error(), "evil [2J") {
		t.Errorf("err = %q", err)
	}
	// Save: the same for its errors
	link := filepath.Join(mkdir(t, filepath.Join(t.TempDir(), "x\x1b[2J")), FileName)
	mkdir(t, link)
	if _, err := Save(link, Changes{Project: i64(1)}); err == nil || strings.Contains(err.Error(), "\x1b") {
		t.Errorf("Save err = %q", err)
	}
}

// Review I1: the project file of a cloned repository is read only when it
// is a regular file (a symbolic link to one is fine) of at most MaxSize
// bytes; anything else is a usage error naming the cleaned path, before
// more than MaxSize+1 bytes are read.
func TestLoad_OnlyRegularFilesOfLimitedSize(t *testing.T) {
	wantUsage := func(t *testing.T, p, want string) {
		t.Helper()
		_, err := Load(p)
		var ce *clierr.Error
		if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, want) || strings.Contains(ce.Message, "\x1b") {
			t.Errorf("Load(%s) = %v, want usage error with %q", p, err, want)
		}
	}
	dir := mkdir(t, filepath.Join(t.TempDir(), hostileName("evil")))
	wantUsage(t, mkdir(t, filepath.Join(dir, FileName)), "keine reguläre Datei")

	big := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(big, []byte("# "+strings.Repeat("x", MaxSize-3)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(big); err != nil {
		t.Errorf("file of exactly MaxSize: %v", err)
	}
	if err := os.WriteFile(big, []byte("# "+strings.Repeat("x", MaxSize-2)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantUsage(t, big, "zu groß")

	// a link to a regular file is still read (shared file in a monorepo)
	target := write(t, t.TempDir(), "shared.toml", "project = 34\n")
	link := filepath.Join(t.TempDir(), FileName)
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if f, err := Load(link); err != nil || f.Project != 34 {
		t.Errorf("link to a regular file: %+v, %v", f, err)
	}
}

// Review M8: a context the stricter name rules reject names the way out
// (rename the context, then adapt the file).
func TestLoad_InvalidContextHintsMigration(t *testing.T) {
	p := write(t, t.TempDir(), FileName, "context = \"firma gmbh\"\n")
	_, err := Load(p)
	var ce *clierr.Error
	if !errors.As(err, &ce) || !strings.Contains(ce.Hint, "papierkram context rename") || !strings.Contains(ce.Hint, "Projektdatei") {
		t.Errorf("err = %v / hint %q", err, ce.Hint)
	}
}

// hostileName appends terminal control characters to base where the file
// system allows them; Windows forbids control characters in file names,
// so there the path itself cannot carry the attack.
func hostileName(base string) string {
	if runtime.GOOS == "windows" {
		return base
	}
	return base + "\x1b[2J"
}
