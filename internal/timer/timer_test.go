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
	return State{Context: "firma", StartedAt: time.Date(2026, 11, 4, 9, 0, 12, 0, loc), Task: 545, User: 1, Comment: "Abstimmung",
		BaseURL: "https://firma.papierkram.de/api/v1"}
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
		got.User != want.User || got.Comment != want.Comment || got.BaseURL != want.BaseURL {
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
		"comment too long": `{"context":"firma","started_at":"2026-11-04T09:00:00+01:00","task":1,"user":1,"comment":"` +
			strings.Repeat("x", MaxComment+1) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "x")
			if runtime.GOOS != "windows" { // no control characters in Windows file names
				dir += "\x1b[2J"
			}
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

// Review I1: Save and Load share one validation, so what Save writes Load
// can always read: an invalid state is never written, and the limits hold
// at their boundary, also for a comment that JSON escapes to six times
// its length.
func TestSaveValidatesLikeLoad(t *testing.T) {
	dir := t.TempDir()
	for name, mod := range map[string]func(*State){
		"task 0":           func(s *State) { s.Task = 0 },
		"user 0":           func(s *State) { s.User = 0 },
		"negative task":    func(s *State) { s.Task = -1 },
		"no start":         func(s *State) { s.StartedAt = time.Time{} },
		"comment too long": func(s *State) { s.Comment = strings.Repeat("x", MaxComment+1) },
		"state too large":  func(s *State) { s.BaseURL = strings.Repeat("x", MaxSize) },
	} {
		s := state(t)
		mod(&s)
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := Save(path, s); err == nil {
			t.Errorf("%s: Save accepted %+v", name, s)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: file written", name)
		}
	}
	for name, comment := range map[string]string{
		"longest comment":       strings.Repeat("x", MaxComment),
		"longest escaped":       strings.Repeat("<", MaxComment),
		"longest control chars": strings.Repeat("\x01", MaxComment),
	} {
		s := state(t)
		s.Comment = comment
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := Save(path, s); err != nil {
			t.Fatalf("%s: Save = %v", name, err)
		}
		if got, err := Load(path); err != nil || got == nil || got.Comment != comment {
			t.Errorf("%s: Load = %v", name, err)
		}
	}
}

// Review M4: a cache directory where a file blocks the path (ENOTDIR)
// holds no timer; it is no corrupt state.
func TestLoadWithFileInPathIsNoTimer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "papierkram"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := Load(Path(dir, "firma")); st != nil || err != nil {
		t.Errorf("Load = %+v, %v", st, err)
	}
}

// Task 11 (C2): time stop claims the state file by renaming it to
// StoppingPath before it sends the entry. Of two stops only one claim
// succeeds; the other gets ErrClaimed, as does a stop that finds a claim
// left behind. Release puts the claim back, never over a new state file.
func TestClaimRelease(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	if got := StoppingPath(path); got != path+".stopping" {
		t.Errorf("StoppingPath = %q", got)
	}
	// no timer: nothing to claim
	if err := Claim(path); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim without file = %v, want ErrClaimed", err)
	}
	if st, err := LoadStopping(path); st != nil || err != nil {
		t.Errorf("LoadStopping without claim = %+v, %v", st, err)
	}
	want := state(t)
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	if err := Claim(path); err != nil {
		t.Fatalf("Claim = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state file left after Claim: %v", err)
	}
	if st, err := Load(path); st != nil || err != nil {
		t.Errorf("Load after Claim = %+v, %v", st, err)
	}
	got, err := LoadStopping(path)
	if err != nil || got == nil || !got.Equal(want) {
		t.Fatalf("LoadStopping = %+v, %v", got, err)
	}
	// a second claim: the first one is there
	if err := Claim(path); !errors.Is(err, ErrClaimed) {
		t.Errorf("second Claim = %v, want ErrClaimed", err)
	}
	// a new state file next to the claim is not taken either (the claim
	// is checked first, so no rename replaces it, also on Windows)
	other := want
	other.Task = 999
	if err := Save(path, other); err != nil {
		t.Fatal(err)
	}
	if err := Claim(path); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim with a claim present = %v, want ErrClaimed", err)
	}
	if st, _ := LoadStopping(path); st == nil || st.Task != 545 {
		t.Errorf("claim replaced: %+v", st)
	}
	// Release never replaces a state file
	if err := Release(path); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Release over a state file = %v, want fs.ErrExist", err)
	}
	if st, _ := Load(path); st == nil || st.Task != 999 {
		t.Errorf("state file replaced by Release: %+v", st)
	}
	if st, _ := LoadStopping(path); st == nil || st.Task != 545 {
		t.Errorf("claim lost by a failed Release: %+v", st)
	}
	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if err := Release(path); err != nil {
		t.Fatalf("Release = %v", err)
	}
	if st, err := Load(path); err != nil || st == nil || !st.Equal(want) {
		t.Errorf("Load after Release = %+v, %v", st, err)
	}
	if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("claim left after Release: %v", err)
	}
	// nothing to release
	if err := Release(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Release without claim = %v, want fs.ErrNotExist", err)
	}
}

// Of concurrent claims of one state file exactly one succeeds.
func TestClaimConcurrent(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	for range 20 {
		if err := Save(path, state(t)); err != nil {
			t.Fatal(err)
		}
		const n = 8
		errs := make(chan error, n)
		start := make(chan struct{})
		for range n {
			go func() {
				<-start
				errs <- Claim(path)
			}()
		}
		close(start)
		won := 0
		for range n {
			switch err := <-errs; {
			case err == nil:
				won++
			case !errors.Is(err, ErrClaimed) && runtime.GOOS != "windows":
				// Windows may refuse a rename of a file that another
				// rename holds open (sharing violation): still a loser
				t.Errorf("Claim = %v", err)
			}
		}
		if won != 1 {
			t.Fatalf("%d claims succeeded, want 1", won)
		}
		if err := Delete(StoppingPath(path)); err != nil {
			t.Fatal(err)
		}
	}
}

// The claim follows the rules of the state file: LoadStopping rejects
// what Load rejects and names the claim; a symbolic link is renamed and
// released as a link, never followed, and a link at the claim path
// blocks a claim.
func TestClaimSymlinkRule(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	if err := Save(valid, state(t)); err != nil {
		t.Fatal(err)
	}
	path := Path(dir, "firma")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(valid, path); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := Claim(path); err != nil {
		t.Fatalf("Claim of a link = %v", err)
	}
	if fi, err := os.Lstat(StoppingPath(path)); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("claim is no link: %v, %v", fi, err)
	}
	st, err := LoadStopping(path)
	var ce *clierr.Error
	if st != nil || !errors.As(err, &ce) || ce.Exit() != 1 || !strings.Contains(ce.Message, ".stopping") ||
		!strings.Contains(ce.Hint, "papierkram time discard") {
		t.Errorf("LoadStopping of a link = %+v, %v", st, err)
	}
	if err := Release(path); err != nil {
		t.Fatalf("Release of a link = %v", err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("released file is no link: %v, %v", fi, err)
	}
	if _, err := Load(valid); err != nil {
		t.Errorf("link target changed: %v", err)
	}

	// a (dangling) link at the claim path blocks the claim
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, state(t)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nirgends"), StoppingPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := Claim(path); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim with a link at the claim = %v, want ErrClaimed", err)
	}
	if st, err := Load(path); err != nil || st == nil {
		t.Errorf("state file moved: %+v, %v", st, err)
	}
}

// LoadStopping applies the checks of Load to the claim.
func TestLoadStoppingRejectsInvalidClaim(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StoppingPath(path), []byte(`{"context":"firma","task":1,"user":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := LoadStopping(path)
	var ce *clierr.Error
	if st != nil || !errors.As(err, &ce) || ce.Exit() != 1 || !strings.Contains(ce.Message, "Startzeit fehlt") ||
		!strings.Contains(ce.Message, "timer-firma.json.stopping") {
		t.Errorf("LoadStopping = %+v, %v", st, err)
	}
}

// State.Equal compares all fields, the start as an instant.
func TestStateEqual(t *testing.T) {
	a := state(t)
	b := a
	b.StartedAt = a.StartedAt.UTC()
	if !a.Equal(b) {
		t.Error("same instant in another zone is not equal")
	}
	for name, mod := range map[string]func(*State){
		"context": func(s *State) { s.Context = "x" },
		"start":   func(s *State) { s.StartedAt = s.StartedAt.Add(time.Second) },
		"task":    func(s *State) { s.Task++ },
		"user":    func(s *State) { s.User++ },
		"comment": func(s *State) { s.Comment = "x" },
		"base":    func(s *State) { s.BaseURL = "x" },
	} {
		c := a
		mod(&c)
		if a.Equal(c) {
			t.Errorf("%s: equal", name)
		}
	}
}
