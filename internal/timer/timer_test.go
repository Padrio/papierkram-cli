package timer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
)

func TestPath(t *testing.T) {
	base := filepath.Join("base")
	for ctx, want := range map[string]string{
		"firma-gmbh": "timer-firma-gmbh.json",
		"a_1":        "timer-a_1.json",
		// path separators, case and other characters are escaped: a
		// context never names a file outside the directory, and names that
		// differ only in case never share a file (case-insensitive file
		// systems)
		"Firma":    "timer-%46irma.json",
		"a/b":      "timer-a%2Fb.json",
		`a\..\b`:   "timer-a%5C%2E%2E%5Cb.json",
		"müller":   "timer-m%C3%BCller.json",
		"a%2Fb":    "timer-a%252%46b.json",
		"":         "timer-.direct.json",
		"C:x":      "timer-%43%3Ax.json",
		"con":      "timer-con.json",
		"x*?<>|\"": "timer-x%2A%3F%3C%3E%7C%22.json",
	} {
		if got := Path(base, ctx); got != filepath.Join(base, "papierkram", want) {
			t.Errorf("Path(%q) = %q, want %q", ctx, got, want)
		}
	}
	// the direct mode file cannot belong to a context: its name part is
	// no valid context name
	if config.ValidateName(DirectName) == nil {
		t.Errorf("DirectName %q is a valid context name", DirectName)
	}
	// very long names stay short and distinct
	long1, long2 := strings.Repeat("a", 300)+"1", strings.Repeat("a", 300)+"2"
	p1, p2 := filepath.Base(Path(base, long1)), filepath.Base(Path(base, long2))
	if len(p1) > 100 || p1 == p2 || !strings.HasPrefix(p1, "timer-~") {
		t.Errorf("long names: %q, %q", p1, p2)
	}
}

func state(t *testing.T) State {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return State{Context: "firma", StartedAt: time.Date(2026, 11, 4, 9, 0, 12, 0, loc), Task: 545, User: 1, Comment: "Abstimmung"}
}

func TestSaveLoadDelete(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	if st, err := Load(path); st != nil || err != nil {
		t.Fatalf("Load without file = %+v, %v", st, err)
	}
	want := state(t)
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Errorf("state file: %v, %v", fi, err)
	}
	got, err := Load(path)
	if err != nil || got == nil {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if got.Context != want.Context || !got.StartedAt.Equal(want.StartedAt) || got.Task != want.Task ||
		got.User != want.User || got.Comment != want.Comment {
		t.Errorf("Load = %+v, want %+v", got, want)
	}

	// a running timer is never overwritten
	other := want
	other.Task = 999
	if err := Save(path, other); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second Save = %v, want fs.ErrExist", err)
	}
	if got, _ := Load(path); got == nil || got.Task != 545 {
		t.Errorf("state changed by second Save: %+v", got)
	}

	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if st, err := Load(path); st != nil || err != nil {
		t.Errorf("Load after Delete = %+v, %v", st, err)
	}
	if err := Delete(path); err != nil {
		t.Errorf("Delete without file = %v", err)
	}
}

// The state file is local and may have been edited or replaced: anything
// that is no valid state is an error (exit 1) naming time discard, with
// the path printable.
func TestLoadRejectsInvalidState(t *testing.T) {
	for name, content := range map[string]string{
		"no json":         "kein JSON",
		"array":           "[]",
		"null":            "null",
		"trailing":        `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","task":1,"user":1} {}`,
		"no task":         `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","user":1}`,
		"negative user":   `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","task":1,"user":-1}`,
		"no start":        `{"context":"firma","task":1,"user":1}`,
		"bad start":       `{"context":"firma","started_at":"heute","task":1,"user":1}`,
		"task as string":  `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","task":"1","user":1}`,
		"too large":       `{"comment":"` + strings.Repeat("x", MaxSize) + `"}`,
		"empty":           "",
		"comment no text": `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","task":1,"user":1,"comment":5}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "x\x1b[2J")
			path := Path(dir, "firma")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			wantInvalid(t, path)
		})
	}
}

func wantInvalid(t *testing.T, path string) {
	t.Helper()
	st, err := Load(path)
	var ce *clierr.Error
	if st != nil || !errors.As(err, &ce) || ce.Code != clierr.CodeError || ce.Exit() != 1 {
		t.Fatalf("Load = %+v, %v; want exit 1", st, err)
	}
	if !strings.Contains(ce.Hint, "papierkram time discard") || strings.Contains(ce.Message, "\x1b") {
		t.Errorf("message %q, hint %q", ce.Message, ce.Hint)
	}
}

func TestLoadRejectsNonRegularFiles(t *testing.T) {
	// a directory
	path := Path(t.TempDir(), "firma")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	wantInvalid(t, path)

	// a symbolic link, also to a valid state: never followed
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	if err := Save(valid, state(t)); err != nil {
		t.Fatal(err)
	}
	path = Path(dir, "firma")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(valid, path); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	wantInvalid(t, path)
	// Save does not write through it, Delete removes the link only
	if err := Save(path, state(t)); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Save over a link = %v", err)
	}
	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(valid); err != nil {
		t.Errorf("link target removed: %v", err)
	}
}
