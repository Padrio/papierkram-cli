package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recorder captures failures of Assert without failing the real test.
type recorder struct {
	testing.TB
	fatal, errors []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = append(r.fatal, fmt.Sprintf(format, args...))
}

func TestAssertMatches(t *testing.T) {
	r := &recorder{TB: t}
	Assert(r, "sample.txt", []byte("ID  NAME\n1   Müller\n"))
	if len(r.fatal)+len(r.errors) > 0 {
		t.Errorf("unerwarteter Fehler: %v %v", r.fatal, r.errors)
	}
}

func TestAssertMismatchShowsDiff(t *testing.T) {
	r := &recorder{TB: t}
	Assert(r, "sample.txt", []byte("ID  NAME\n2   Müller\n"))
	if len(r.errors) != 1 {
		t.Fatalf("errors = %v, fatal = %v", r.errors, r.fatal)
	}
	msg := r.errors[0]
	for _, want := range []string{"testdata/golden/sample.txt", "-update", "  ID  NAME\n", "- 1   Müller\n", "+ 2   Müller\n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Meldung ohne %q:\n%s", want, msg)
		}
	}
}

// A mismatch that consists only of CRLF vs. LF line endings stays a
// failure (no silent normalization) but names the likely cause.
func TestAssertLineEndingMismatchHint(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "testdata", "golden", "crlf.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ID  NAME\r\n1   Müller\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &recorder{TB: t}
	Assert(r, "crlf.txt", []byte("ID  NAME\n1   Müller\n"))
	if len(r.errors) != 1 {
		t.Fatalf("errors = %v, fatal = %v", r.errors, r.fatal)
	}
	if !strings.Contains(r.errors[0], "Zeilenenden unterscheiden sich (CRLF/LF) – .gitattributes prüfen") {
		t.Errorf("Meldung ohne CRLF-Hinweis:\n%s", r.errors[0])
	}
}

// A real content mismatch must not blame line endings.
func TestAssertContentMismatchHasNoLineEndingHint(t *testing.T) {
	r := &recorder{TB: t}
	Assert(r, "sample.txt", []byte("ID  NAME\r\n2   Müller\r\n"))
	if len(r.errors) != 1 {
		t.Fatalf("errors = %v, fatal = %v", r.errors, r.fatal)
	}
	if strings.Contains(r.errors[0], "Zeilenenden") {
		t.Errorf("unerwarteter CRLF-Hinweis:\n%s", r.errors[0])
	}
}

func TestAssertMissingFile(t *testing.T) {
	r := &recorder{TB: t}
	Assert(r, "fehlt.txt", []byte("x\n"))
	if len(r.fatal) != 1 || !strings.Contains(r.fatal[0], "-update") {
		t.Errorf("fatal = %v", r.fatal)
	}
}

// Tokens must never end up in golden files: Assert fails before comparing
// or writing.
func TestAssertRejectsTokens(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	setUpdate(t, true)
	r := &recorder{TB: t}
	Assert(r, "token.txt", []byte("Authorization: Bearer test-token-abc\n"))
	if len(r.fatal) != 1 || !strings.Contains(r.fatal[0], "test-token-") {
		t.Errorf("fatal = %v", r.fatal)
	}
	if _, err := os.Stat(filepath.Join(dir, "testdata", "golden", "token.txt")); !os.IsNotExist(err) {
		t.Errorf("Golden-Datei mit Token geschrieben: %v", err)
	}
}

func TestAssertUpdateWritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	setUpdate(t, true)
	r := &recorder{TB: t}
	Assert(r, "neu/liste.txt", []byte("a\n"))
	if len(r.fatal)+len(r.errors) > 0 {
		t.Fatalf("%v %v", r.fatal, r.errors)
	}
	b, err := os.ReadFile(filepath.Join(dir, "testdata", "golden", "neu", "liste.txt"))
	if err != nil || string(b) != "a\n" {
		t.Errorf("Datei: %q, %v", b, err)
	}
}

func TestAssertRejectsPathsOutsideGolden(t *testing.T) {
	for _, name := range []string{"../x.txt", "/tmp/x.txt", "a/../../x.txt", ""} {
		r := &recorder{TB: t}
		Assert(r, name, []byte("x\n"))
		if len(r.fatal) != 1 {
			t.Errorf("%q: fatal = %v", name, r.fatal)
		}
	}
}

func TestDiffMarksMissingFinalNewline(t *testing.T) {
	d := diff([]byte("a\nb\n"), []byte("a\nb"))
	if !strings.Contains(d, "kein Zeilenumbruch am Ende") {
		t.Errorf("diff:\n%s", d)
	}
}

func setUpdate(t *testing.T, v bool) {
	t.Helper()
	old := *update
	*update = v
	t.Cleanup(func() { *update = old })
}
