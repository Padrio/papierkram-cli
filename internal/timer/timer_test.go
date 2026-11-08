package timer

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/safefile"
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

// Task 11 (C2, ruling T11-A): time stop claims the timer by creating a
// copy of the state file exclusively at StoppingPath and then removing the
// state file. Of two stops only one claim succeeds; the other gets
// ErrClaimed, as does a stop that finds the file gone, and ErrChanged when
// the file is another timer. Release creates the state file again, never
// over a new one.
func TestClaimRelease(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	if got := StoppingPath(path); got != path+".stopping" {
		t.Errorf("StoppingPath = %q", got)
	}
	want := state(t)
	// no timer: nothing to claim
	if err := Claim(path, want); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim without file = %v, want ErrClaimed", err)
	}
	if st, err := LoadStopping(path); st != nil || err != nil {
		t.Errorf("LoadStopping without claim = %+v, %v", st, err)
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(path)
	// another timer than the one read: nothing claimed
	other := want
	other.Task = 999
	if err := Claim(path, other); !errors.Is(err, ErrChanged) {
		t.Errorf("Claim of another timer = %v, want ErrChanged", err)
	}
	if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("claim created for another timer: %v", err)
	}
	if err := Claim(path, want); err != nil {
		t.Fatalf("Claim = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state file left after Claim: %v", err)
	}
	if fi, err := os.Stat(StoppingPath(path)); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Errorf("claim: %v, %v", fi, err)
	}
	if got, _ := os.ReadFile(StoppingPath(path)); !bytes.Equal(got, saved) {
		t.Errorf("claim %s, want the state file %s", got, saved)
	}
	got, err := LoadStopping(path)
	if err != nil || got == nil || !got.Equal(want) {
		t.Fatalf("LoadStopping = %+v, %v", got, err)
	}
	// a new state file next to the claim is never claimed over it
	if err := Save(path, other); err != nil {
		t.Fatal(err)
	}
	if err := Claim(path, other); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim with a claim present = %v, want ErrClaimed", err)
	}
	if st, _ := LoadStopping(path); st == nil || st.Task != 545 {
		t.Errorf("claim replaced: %+v", st)
	}
	if st, _ := Load(path); st == nil || st.Task != 999 {
		t.Errorf("new state file changed: %+v", st)
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
	if got, _ := os.ReadFile(path); !bytes.Equal(got, saved) {
		t.Errorf("released state %s, want %s", got, saved)
	}
	if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("claim left after Release: %v", err)
	}
	// nothing to release
	if err := Release(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Release without claim = %v, want fs.ErrNotExist", err)
	}
	// an unusable state file is the error of Load, nothing claimed
	if err := os.WriteFile(path, []byte("kein JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	var ce *clierr.Error
	if err := Claim(path, want); !errors.As(err, &ce) || ce.Exit() != 1 {
		t.Errorf("Claim of an unusable file = %v", err)
	}
	if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("claim created for an unusable file: %v", err)
	}
}

// Of N concurrent claims of one timer exactly one succeeds, and a timer
// started meanwhile (once the winner removed the state file) is never
// overwritten or removed: later claims of the old timer get ErrClaimed
// while the claim exists and ErrChanged after it.
func TestClaimConcurrent(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	old := state(t)
	newer := old
	newer.Task, newer.StartedAt = 777, old.StartedAt.Add(time.Hour)
	claim := func(n int) (won int, errs []error) {
		results := make(chan error, n)
		start := make(chan struct{})
		for range n {
			go func() {
				<-start
				results <- Claim(path, old)
			}()
		}
		close(start)
		for range n {
			if err := <-results; err == nil {
				won++
			} else {
				errs = append(errs, err)
			}
		}
		return won, errs
	}
	for round := range 20 {
		if err := Save(path, old); err != nil {
			t.Fatal(err)
		}
		oldBytes, _ := os.ReadFile(path)
		// a time start racing the claims: it can only save once the
		// winner removed the state file, and nothing may replace its file
		started := make(chan []byte, 1)
		go func() {
			for {
				if err := Save(path, newer); err == nil {
					data, _ := os.ReadFile(path)
					started <- data
					return
				} else if !errors.Is(err, fs.ErrExist) {
					t.Error(err)
					started <- nil
					return
				}
				runtime.Gosched()
			}
		}()
		won, errs := claim(8)
		if won != 1 {
			t.Fatalf("round %d: %d claims succeeded, want 1 (%v)", round, won, errs)
		}
		for _, err := range errs {
			if !errors.Is(err, ErrClaimed) && !errors.Is(err, ErrChanged) {
				t.Errorf("round %d: Claim = %v", round, err)
			}
		}
		newBytes := <-started
		if got, _ := os.ReadFile(StoppingPath(path)); !bytes.Equal(got, oldBytes) {
			t.Fatalf("round %d: claim %s, want the old timer", round, got)
		}
		// while the claim exists, more claims of the old timer find the new one
		if won, errs := claim(4); won != 0 {
			t.Fatalf("round %d: %d late claims succeeded (%v)", round, won, errs)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, newBytes) {
			t.Fatalf("round %d: new timer changed: %s", round, got)
		}
		// the stop is done: the claim goes, the new timer stays
		if err := Delete(StoppingPath(path)); err != nil {
			t.Fatal(err)
		}
		if won, errs := claim(4); won != 0 || slices.ContainsFunc(errs, func(err error) bool { return !errors.Is(err, ErrChanged) }) {
			t.Fatalf("round %d: claims after the stop: %d, %v", round, won, errs)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, newBytes) {
			t.Fatalf("round %d: new timer changed: %s", round, got)
		}
		if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("round %d: claim of a changed timer left", round)
		}
		if err := Delete(path); err != nil {
			t.Fatal(err)
		}
	}
}

// The claim follows the rules of the state file: a symbolic link as state
// file is never claimed (the error of Load) and never followed; a link at
// the claim path blocks a claim and is not released; LoadStopping rejects
// it and names the claim.
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
	var ce *clierr.Error
	if err := Claim(path, state(t)); !errors.As(err, &ce) || ce.Exit() != 1 {
		t.Errorf("Claim of a link = %v", err)
	}
	if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("claim created for a link: %v", err)
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("link changed: %v, %v", fi, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// a (dangling) link at the claim path blocks the claim
	if err := Save(path, state(t)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nirgends"), StoppingPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := Claim(path, state(t)); !errors.Is(err, ErrClaimed) {
		t.Errorf("Claim with a link at the claim = %v, want ErrClaimed", err)
	}
	if st, err := Load(path); err != nil || st == nil {
		t.Errorf("state file changed: %+v, %v", st, err)
	}
	// a link as claim is not released, and LoadStopping rejects it
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(StoppingPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(valid, StoppingPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := Release(path); err == nil {
		t.Error("Release followed a link")
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state file created from a link: %v", err)
	}
	st, err := LoadStopping(path)
	if st != nil || !errors.As(err, &ce) || ce.Exit() != 1 || !strings.Contains(ce.Message, ".stopping") ||
		!strings.Contains(ce.Hint, "papierkram time discard") {
		t.Errorf("LoadStopping of a link = %+v, %v", st, err)
	}
	if _, err := Load(valid); err != nil {
		t.Errorf("link target changed: %v", err)
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
		"unbillable set": func(s *State) {
			v := false
			s.Unbillable = &v
		},
	} {
		c := a
		mod(&c)
		if a.Equal(c) {
			t.Errorf("%s: equal", name)
		}
	}
	// unbillable compares the value, not the pointer
	yes, yes2, no := true, true, false
	a.Unbillable, b.Unbillable = &yes, &yes2
	if !a.Equal(b) {
		t.Error("unbillable true and true: not equal")
	}
	b.Unbillable = &no
	if a.Equal(b) {
		t.Error("unbillable true and false: equal")
	}
}

// Review T12-M2: Equal ends with s == o, which compares pointers. Every
// pointer field of State must be compared by its value before (like
// Unbillable), so two states whose field points to equal values are
// equal, and a set field never equals a nil one.
func TestStateEqualPointerFields(t *testing.T) {
	typ := reflect.TypeFor[State]()
	n := 0
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Type.Kind() != reflect.Pointer {
			continue
		}
		n++
		a, b := state(t), state(t)
		reflect.ValueOf(&a).Elem().Field(i).Set(reflect.New(f.Type.Elem()))
		reflect.ValueOf(&b).Elem().Field(i).Set(reflect.New(f.Type.Elem()))
		if !a.Equal(b) {
			t.Errorf("%s: two pointers to the same value are not equal (compared as pointers)", f.Name)
		}
		if c := state(t); a.Equal(c) || c.Equal(a) {
			t.Errorf("%s: set and nil are equal", f.Name)
		}
	}
	if n == 0 {
		t.Fatal("no pointer field in State: test outdated")
	}
}

// Task 12: unbillable is stored when set (true and false) and missing in
// the file when not; a state file of an older version without it loads
// with nil, and Claim accepts it as the timer it read.
func TestStateUnbillable(t *testing.T) {
	for name, v := range map[string]*bool{"nil": nil, "true": new(true), "false": new(false)} {
		path := Path(t.TempDir(), "firma")
		want := state(t)
		want.Unbillable = v
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if has := bytes.Contains(data, []byte(`"unbillable"`)); has != (v != nil) {
			t.Errorf("%s: file %s", name, data)
		}
		got, err := Load(path)
		if err != nil || got == nil || !got.Equal(want) {
			t.Errorf("%s: Load = %+v, %v", name, got, err)
		}
	}
	path := Path(t.TempDir(), "firma")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"context":"firma","started_at":"2026-11-04T09:00:12+01:00","task":545,"user":1,"comment":"Abstimmung","base_url":"https://firma.papierkram.de/api/v1"}` + "\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got == nil || got.Unbillable != nil || !got.Equal(state(t)) {
		t.Fatalf("Load of an old state = %+v, %v", got, err)
	}
	if err := Claim(path, state(t)); err != nil {
		t.Errorf("Claim of an old state = %v", err)
	}
	if data, _ := os.ReadFile(StoppingPath(path)); string(data) != old {
		t.Errorf("claim = %s, want the old bytes", data)
	}
}

// withHooks sets afterClaim and removeFile for one test.
func withHooks(t *testing.T, after func(), remove func(string) error) {
	t.Helper()
	oldAfter, oldRemove := afterClaim, removeFile
	t.Cleanup(func() { afterClaim, removeFile = oldAfter, oldRemove })
	if after != nil {
		afterClaim = after
	}
	if remove != nil {
		removeFile = remove
	}
}

// Review of task 11: what happens between creating the claim and removing
// the state file. A state file replaced or removed meanwhile, or one that
// cannot be removed, leaves no claim, and nothing else is touched.
func TestClaimBetweenCreateAndRemove(t *testing.T) {
	want := state(t)
	newer := want
	newer.Task = 777
	for _, tc := range []struct {
		name   string
		after  func(t *testing.T, path string)
		remove func(path string) func(string) error
		check  func(t *testing.T, err error, path string)
	}{
		{name: "replaced",
			after: func(t *testing.T, path string) {
				if err := Delete(path); err != nil {
					t.Error(err)
				}
				if err := Save(path, newer); err != nil {
					t.Error(err)
				}
			},
			check: func(t *testing.T, err error, path string) {
				if !errors.Is(err, ErrChanged) {
					t.Errorf("Claim = %v, want ErrChanged", err)
				}
				if st, _ := Load(path); st == nil || !st.Equal(newer) {
					t.Errorf("new timer changed: %+v", st)
				}
			}},
		{name: "symlink swapped in",
			after: func(t *testing.T, path string) {
				if err := Delete(path); err != nil {
					t.Error(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(path), "x"), path); err != nil {
					t.Skipf("no symlinks: %v", err)
				}
			},
			check: func(t *testing.T, err error, path string) {
				if !errors.Is(err, ErrChanged) {
					t.Errorf("Claim = %v, want ErrChanged", err)
				}
				if fi, err := os.Lstat(path); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
					t.Errorf("link removed: %v", err)
				}
			}},
		{name: "removed",
			after: func(t *testing.T, path string) {
				if err := Delete(path); err != nil {
					t.Error(err)
				}
			},
			check: func(t *testing.T, err error, path string) {
				if !errors.Is(err, ErrClaimed) {
					t.Errorf("Claim = %v, want ErrClaimed", err)
				}
			}},
		{name: "remove fails",
			remove: func(path string) func(string) error {
				return func(p string) error {
					if p == path {
						return &fs.PathError{Op: "remove", Path: p, Err: fs.ErrPermission}
					}
					return os.Remove(p)
				}
			},
			check: func(t *testing.T, err error, path string) {
				if err == nil || errors.Is(err, ErrClaimed) || errors.Is(err, ErrChanged) || !errors.Is(err, fs.ErrPermission) ||
					!strings.Contains(err.Error(), "nicht entfernt") {
					t.Errorf("Claim = %v", err)
				}
				if st, _ := Load(path); st == nil || !st.Equal(want) {
					t.Errorf("state file changed: %+v", st)
				}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := Path(t.TempDir(), "firma")
			if err := Save(path, want); err != nil {
				t.Fatal(err)
			}
			var after func()
			if tc.after != nil {
				// the hook reports to the subtest it runs in (t.Skipf of
				// the parent would end the wrong test)
				after = func() { tc.after(t, path) }
			}
			var remove func(string) error
			if tc.remove != nil {
				remove = tc.remove(path)
			}
			withHooks(t, after, remove)
			err := Claim(path, want)
			tc.check(t, err, path)
			if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("claim left: %v", err)
			}
		})
	}
}

// Review of task 11: the error branches of Release, all German. A claim
// that cannot be removed after the state file is back matches
// ErrClaimLeft: the timer runs again.
func TestReleaseErrors(t *testing.T) {
	path := Path(t.TempDir(), "firma")
	if err := Release(path); !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "fehlt") {
		t.Errorf("Release without claim = %v", err)
	}
	if err := Save(StoppingPath(path), state(t)); err != nil {
		t.Fatal(err)
	}
	withHooks(t, nil, func(p string) error {
		if p == StoppingPath(path) {
			return &fs.PathError{Op: "remove", Path: p, Err: fs.ErrPermission}
		}
		return os.Remove(p)
	})
	err := Release(path)
	if !errors.Is(err, ErrClaimLeft) || !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "nicht gelöscht") {
		t.Errorf("Release with a claim that cannot be removed = %v", err)
	}
	if st, err := Load(path); err != nil || st == nil || !st.Equal(state(t)) {
		t.Errorf("state file not back: %+v, %v", st, err)
	}
	if st, _ := LoadStopping(path); st == nil {
		t.Error("claim removed")
	}
	// a claim that is no regular file is not read
	dir := Path(t.TempDir(), "firma")
	if err := os.MkdirAll(StoppingPath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Release(dir); err == nil || !strings.Contains(err.Error(), "keine verwendbare Datei") {
		t.Errorf("Release of a directory claim = %v", err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state file created: %v", err)
	}
}

// CI run 42453374759: a state file replaced while it is read (a parallel
// claim removes it, a time start creates a new one) made Load and Claim
// report "keine reguläre Datei" (exit 1). That is a race, no unusable
// file: Load reads again and sees the new state or none; Claim ends with
// ErrChanged or ErrClaimed and claims nothing.
func TestLoadAndClaimOfSwappedFile(t *testing.T) {
	old := state(t)
	newer := old
	newer.Task = 777
	// swapping returns ErrSwapped from the first read after doing swap
	swapping := func(t *testing.T, times int, swap func(path string)) {
		t.Helper()
		orig := readFile
		t.Cleanup(func() { readFile = orig })
		n := 0
		readFile = func(p string, follow bool, limit int64) ([]byte, error) {
			if n < times {
				n++
				if n == 1 && swap != nil {
					swap(p)
				}
				return nil, safefile.ErrSwapped
			}
			return orig(p, follow, limit)
		}
	}
	replace := func(p string) {
		if err := Delete(p); err != nil {
			t.Error(err)
		}
		if err := Save(p, newer); err != nil {
			t.Error(err)
		}
	}
	remove := func(p string) {
		if err := Delete(p); err != nil {
			t.Error(err)
		}
	}
	for _, tc := range []struct {
		name      string
		swap      func(string)
		times     int
		wantLoad  *State // nil: no timer
		loadErr   bool
		wantClaim error
	}{
		{"replaced", replace, 1, &newer, false, ErrChanged},
		{"removed", remove, 1, nil, false, ErrClaimed},
		{"swapped on every read", nil, 1 << 30, nil, true, ErrChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, op := range []string{"load", "claim"} {
				path := Path(t.TempDir(), "firma")
				if err := Save(path, old); err != nil {
					t.Fatal(err)
				}
				swapping(t, tc.times, tc.swap)
				if op == "load" {
					st, err := Load(path)
					var ce *clierr.Error
					switch {
					case tc.loadErr && (!errors.As(err, &ce) || ce.Exit() != 1 || !strings.Contains(ce.Message, "beim Lesen ersetzt")):
						t.Errorf("Load = %+v, %v", st, err)
					case !tc.loadErr && err != nil:
						t.Errorf("Load = %v", err)
					case tc.wantLoad == nil && st != nil, tc.wantLoad != nil && (st == nil || !st.Equal(*tc.wantLoad)):
						t.Errorf("Load = %+v, want %+v", st, tc.wantLoad)
					}
					continue
				}
				if err := Claim(path, old); !errors.Is(err, tc.wantClaim) {
					t.Errorf("Claim = %v, want %v", err, tc.wantClaim)
				}
				if _, err := os.Lstat(StoppingPath(path)); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("claim created: %v", err)
				}
			}
		})
	}
}
