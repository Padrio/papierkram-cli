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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// MaxSize is the largest state file that is read, in bytes; a state with
// a comment needs far less.
const MaxSize = 64 << 10

// DirectName is the name part of the state file without a context
// (direct mode over PAPIERKRAM_TOKEN): it has a dot, so it is no valid
// context name (config.ValidateName) and the escaping of Path never
// produces it for one.
const DirectName = ".direct"

// State is a running timer: its context ("" in direct mode), start, task,
// user and comment ("" without one).
type State struct {
	Context   string    `json:"context"`
	StartedAt time.Time `json:"started_at"`
	Task      int64     `json:"task"`
	User      int64     `json:"user"`
	Comment   string    `json:"comment,omitempty"`
}

// Path is the state file of the context ctx below cacheDir (os.UserCacheDir()
// or PAPIERKRAM_CACHE_DIR, chosen by the caller like for the name cache):
// <cacheDir>/papierkram/timer-<ctx>.json. A context name may hold any
// character but spaces and dots, so every character other than a–z, 0–9,
// - and _ is escaped as %XX per byte: the name is never a path, and
// contexts that differ only in case get files that differ in more than
// case (case-insensitive file systems). An empty ctx is DirectName; a
// name escaped to more than 128 bytes is replaced by a hash (~<hex>).
func Path(cacheDir, ctx string) string {
	name := DirectName
	if ctx != "" {
		name = escape(ctx)
		if len(name) > 128 {
			h := sha256.Sum256([]byte(ctx))
			name = "~" + hex.EncodeToString(h[:16])
		}
	}
	return filepath.Join(cacheDir, "papierkram", "timer-"+name+".json")
}

// escape keeps a–z, 0–9, - and _ and writes every other byte as %XX.
func escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// DiscardHint is the way out of a state that cannot be used.
const DiscardHint = "Timer-Datei verwerfen: papierkram time discard (die bisher erfasste Zeit des Timers geht dabei verloren)"

// Load reads the state at path; nil without a file. A file that is no
// regular file (also a symbolic link), larger than MaxSize, no valid
// state or unreadable is an error (exit 1) that names time discard.
func Load(path string) (*State, error) {
	data, err := safefile.ReadRegular(path, false, MaxSize)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case errors.Is(err, safefile.ErrSymlink), errors.Is(err, safefile.ErrNotRegular):
		return nil, invalid(path, "keine reguläre Datei", nil)
	case errors.Is(err, safefile.ErrTooLarge):
		return nil, invalid(path, fmt.Sprintf("zu groß (mehr als %d KiB)", MaxSize>>10), nil)
	case err != nil:
		return nil, invalid(path, "nicht lesbar", err)
	}
	var s State
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&s); err != nil {
		return nil, invalid(path, "kein gültiges JSON", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid(path, "Daten nach dem JSON-Objekt", nil)
	}
	switch {
	case s.Task <= 0:
		return nil, invalid(path, "Aufgabe fehlt oder ist ungültig", nil)
	case s.User <= 0:
		return nil, invalid(path, "Benutzer fehlt oder ist ungültig", nil)
	case s.StartedAt.IsZero():
		return nil, invalid(path, "Startzeit fehlt", nil)
	}
	return &s, nil
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
// 0700), atomically and only when there is none: an existing file, also
// a symbolic link, is never replaced or followed; the error then matches
// fs.ErrExist.
func Save(path string, s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return safefile.CreateAtomic(path, append(data, '\n'), 0o600)
}

// Delete removes the state file at path (a symbolic link itself, never
// its target); without a file it does nothing.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
