// Package timer keeps the state of the local timer of time start|stop
// (spec 9.6): when it was started and for which task, user and comment,
// one file per context in <cacheDir>/papierkram/. There is no timer on
// the server; time stop books the period as a time entry.
//
// The state file is local data a user (or anything with access to the
// cache directory) may have edited or replaced: it is read with the rules
// of the project file (safefile.ReadRegular, no symbolic link, bounded)
// and validated; anything else is an error that names time discard.
package timer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// MaxSize is the largest state file that is read, in bytes; a state with
// a comment needs far less.
const MaxSize = 64 << 10

// MaxComment is the longest comment of a timer, in bytes. JSON escapes a
// byte to at most six (\u00XX), so a state with the longest comment stays
// below MaxSize and what Save writes Load reads.
const MaxComment = 8 << 10

// DirectName is the name part of the state file without a context
// (direct mode over PAPIERKRAM_TOKEN), see config.DirectFileName.
const DirectName = config.DirectFileName

// State is a running timer: its context ("" in direct mode), start, task,
// user and comment ("" without one). BaseURL is the API base URL of the
// account the timer was started for (controller ruling, beyond the
// brief's fields): task and user IDs belong to that account. A state of
// the first format has none ("").
type State struct {
	Context   string    `json:"context"`
	StartedAt time.Time `json:"started_at"`
	Task      int64     `json:"task"`
	User      int64     `json:"user"`
	Comment   string    `json:"comment,omitempty"`
	BaseURL   string    `json:"base_url,omitempty"`
}

// Path is the state file of the context ctx below cacheDir (os.UserCacheDir()
// or PAPIERKRAM_CACHE_DIR, chosen by the caller like for the name cache):
// <cacheDir>/papierkram/timer-<ctx>.json, the context name in its file
// name form (config.FileName: escaped, DirectName without a context).
func Path(cacheDir, ctx string) string {
	return filepath.Join(cacheDir, "papierkram", "timer-"+config.FileName(ctx)+".json")
}

// DiscardHint is the way out of a state that cannot be used.
const DiscardHint = "Timer-Datei verwerfen: papierkram time discard (die bisher erfasste Zeit des Timers geht dabei verloren)"

// Load reads the state at path; nil without a file (also when a file
// blocks the directory). A file that is no
// regular file (also a symbolic link), larger than MaxSize, no valid
// state or unreadable is an error (exit 1) that names time discard.
func Load(path string) (*State, error) {
	st, _, err := load(path)
	return st, err
}

// load is Load that also returns the bytes of the file the state was read
// from (nil without one).
func load(path string) (*State, []byte, error) {
	data, err := safefile.ReadRegular(path, false, MaxSize)
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		// ENOTDIR: a file blocks the cache directory; there is no timer
		// (start then fails to save and names the directory, review M4)
		return nil, nil, nil
	case errors.Is(err, safefile.ErrSymlink), errors.Is(err, safefile.ErrNotRegular):
		return nil, nil, invalid(path, "keine reguläre Datei", nil)
	case errors.Is(err, safefile.ErrTooLarge):
		return nil, nil, invalid(path, fmt.Sprintf("zu groß (mehr als %d KiB)", MaxSize>>10), nil)
	case err != nil:
		return nil, nil, invalid(path, "nicht lesbar", err)
	}
	var s State
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&s); err != nil {
		return nil, nil, invalid(path, "kein gültiges JSON", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, nil, invalid(path, "Daten nach dem JSON-Objekt", nil)
	}
	if why := validate(s); why != "" {
		return nil, nil, invalid(path, why, nil)
	}
	return &s, data, nil
}

// validate is the check of a state that Load and Save share, so a state
// Save writes is one Load accepts: positive task and user, a start, a
// comment of at most MaxComment bytes. It returns why s is invalid, ""
// when it is valid.
func validate(s State) string {
	switch {
	case s.Task <= 0:
		return "Aufgabe fehlt oder ist ungültig"
	case s.User <= 0:
		return "Benutzer fehlt oder ist ungültig"
	case s.StartedAt.IsZero():
		return "Startzeit fehlt"
	case len(s.Comment) > MaxComment:
		return fmt.Sprintf("Kommentar zu lang (mehr als %d KiB)", MaxComment>>10)
	}
	return ""
}

// invalid is the error of a state file that cannot be used; the path is
// printable (the cache directory may come from PAPIERKRAM_CACHE_DIR).
func invalid(path, why string, cause error) error {
	e := clierr.New(clierr.CodeError,
		fmt.Sprintf("Timer-Datei %s ist ungültig: %s", output.Printable(path), why), DiscardHint)
	e.Cause = cause
	return e
}

// Save writes s as a new state file at path (mode 0600, the directory
// 0700) when it passes the validation of Load and fits MaxSize,
// atomically and only when there is none: an existing file, also
// a symbolic link, is never replaced or followed; the error then matches
// fs.ErrExist.
func Save(path string, s State) error {
	if why := validate(s); why != "" {
		return errors.New("ungültiger Timer: " + why)
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(data)+1 > MaxSize {
		// only a huge context name or base URL gets here
		return fmt.Errorf("ungültiger Timer: zu groß (mehr als %d KiB)", MaxSize>>10)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return safefile.CreateAtomic(path, append(data, '\n'), 0o600)
}

// ErrClaimed reports that the timer cannot be claimed: another time stop
// holds the claim, or the state file is gone (another stop or a discard
// was faster).
var ErrClaimed = errors.New("timer already claimed by another time stop")

// ErrChanged reports that the state file is no longer the timer the stop
// read (another stop booked it and a new one was started); nothing was
// claimed.
var ErrChanged = errors.New("timer replaced since it was read")

// StoppingPath is the claim of the state file at path: the file time stop
// holds while it sends the entry (task 11, C2).
func StoppingPath(path string) string { return path + ".stopping" }

// Claim takes the timer want at path for one time stop (ruling T11-A). The
// claim is a copy of the state file created exclusively at
// StoppingPath(path) with safefile.CreateAtomic (hard link, otherwise
// O_EXCL): of concurrent claims exactly one creates it, the others get
// ErrClaimed. A rename would not do: os.Rename replaces an existing target
// on every platform (on Windows with MOVEFILE_REPLACE_EXISTING). Only then
// the state file is removed, and only while it still holds the bytes that
// were copied; otherwise the claim is removed again: ErrClaimed when the
// file is gone, ErrChanged when it is another timer. A newly started timer
// is thus never overwritten by a claim, and removed only when a time
// discard and a time start fall between the check and the removal. The state file is read
// with the rules of Load (no symbolic link, MaxSize, valid); an unusable
// one is the error of Load. Other errors are German, wrap the error of the
// file system and leave nothing claimed.
func Claim(path string, want State) error {
	st, data, err := load(path)
	switch {
	case err != nil:
		return err
	case st == nil:
		return ErrClaimed
	case !st.Equal(want):
		return ErrChanged
	}
	claim := StoppingPath(path)
	if err := safefile.CreateAtomic(claim, data, 0o600); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrClaimed
		}
		return fmt.Errorf("%s nicht angelegt: %w", output.Printable(claim), err)
	}
	afterClaim()
	// the claim is ours: no other claim can be created while it exists
	err = removeIfSame(path, data)
	if err != nil {
		_ = removeFile(claim)
		return err
	}
	return nil
}

// removeIfSame removes the state file at path while it holds data: a file
// that is gone is ErrClaimed (another stop or a discard was faster), one
// with other content or no regular file (also a symbolic link swapped in)
// ErrChanged; other errors are German and wrap the cause.
func removeIfSame(path string, data []byte) error {
	now, err := safefile.ReadRegular(path, false, MaxSize)
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return ErrClaimed
	case errors.Is(err, safefile.ErrSymlink), errors.Is(err, safefile.ErrNotRegular), errors.Is(err, safefile.ErrTooLarge):
		return ErrChanged
	case err != nil:
		return fmt.Errorf("Timer-Datei %s nicht lesbar: %w", output.Printable(path), err)
	case !bytes.Equal(now, data):
		return ErrChanged
	}
	if err := removeFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrClaimed
		}
		return fmt.Errorf("Timer-Datei %s nicht entfernt: %w", output.Printable(path), err)
	}
	return nil
}

// afterClaim runs between creating the claim and removing the state
// file; tests change the state file there.
var afterClaim = func() {}

// removeFile is os.Remove; tests make it fail.
var removeFile = os.Remove

// ErrClaimLeft reports that Release put the state file back but could not
// remove the claim: the timer runs again, the claim must be deleted by
// hand (it blocks time start and time stop).
var ErrClaimLeft = errors.New("claim not removed after release")

// Release puts the claim of path back (the stop failed, the timer keeps
// running): the state file is created again from the claim with
// safefile.CreateAtomic, so a state file at path (also a symbolic link)
// is never replaced; it is an error matching fs.ErrExist and the claim
// stays. Then the claim is removed; when that fails, the error matches
// ErrClaimLeft. Without a claim the error matches fs.ErrNotExist; a claim
// that is no regular file is not read. Errors are German.
func Release(path string) error {
	claim := StoppingPath(path)
	data, err := safefile.ReadRegular(claim, false, MaxSize)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s fehlt: %w", output.Printable(claim), fs.ErrNotExist)
	case errors.Is(err, safefile.ErrSymlink), errors.Is(err, safefile.ErrNotRegular), errors.Is(err, safefile.ErrTooLarge):
		return fmt.Errorf("%s ist keine verwendbare Datei: %w", output.Printable(claim), err)
	case err != nil:
		return fmt.Errorf("%s nicht lesbar: %w", output.Printable(claim), err)
	}
	if err := safefile.CreateAtomic(path, data, 0o600); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("Timer-Datei %s ist schon vorhanden: %w", output.Printable(path), fs.ErrExist)
		}
		return fmt.Errorf("Timer-Datei %s nicht angelegt: %w", output.Printable(path), err)
	}
	if err := removeFile(claim); err != nil {
		return fmt.Errorf("%w: %s nicht gelöscht: %w", ErrClaimLeft, output.Printable(claim), err)
	}
	return nil
}

// LoadStopping reads the claim of path with the rules of Load: nil
// without one, an error (exit 1) naming the claim and time discard when
// it cannot be used.
func LoadStopping(path string) (*State, error) { return Load(StoppingPath(path)) }

// Equal reports whether s and o are the same timer: all fields equal, the
// start as the same instant (its zone may differ).
func (s State) Equal(o State) bool {
	if !s.StartedAt.Equal(o.StartedAt) {
		return false
	}
	o.StartedAt = s.StartedAt
	return s == o
}

// Delete removes the state file at path (a symbolic link itself, never
// its target); without a file it does nothing.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
