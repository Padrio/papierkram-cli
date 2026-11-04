package e2eguard

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// NamePrefix starts the name of every object the live suite creates.
// Cleanup touches only objects whose name starts with it (spec 11.1).
const NamePrefix = "pk-e2e-"

// runIDRe is the form of a run ID taken from GITHUB_RUN_ID.
var runIDRe = regexp.MustCompile(`^[0-9a-z]{1,32}$`)

// attemptRe is a GITHUB_RUN_ATTEMPT that marks a re-run.
var attemptRe = regexp.MustCompile(`^[2-9][0-9]{0,3}$|^1[0-9]{1,3}$`)

// RunID is the ID of this run: GITHUB_RUN_ID when it is set and fits a
// name (digits and lower-case letters, at most 32), with "a<attempt>"
// appended for a re-run (GITHUB_RUN_ATTEMPT 2 and up), otherwise 12
// random hex digits from random (nil: crypto/rand).
func RunID(getenv func(string) string, random io.Reader) (string, error) {
	if v := getenv("GITHUB_RUN_ID"); runIDRe.MatchString(v) {
		if a := getenv("GITHUB_RUN_ATTEMPT"); attemptRe.MatchString(a) {
			v += "a" + a
		}
		return v, nil
	}
	if random == nil {
		random = rand.Reader
	}
	b := make([]byte, 6)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("e2e: Lauf-ID nicht erzeugbar: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Prefix is the name prefix of run runID: pk-e2e-<runid>-.
func Prefix(runID string) string { return NamePrefix + runID + "-" }

// Owned reports whether name belongs to the live suite (starts exactly
// with pk-e2e-). Every other object is never touched.
func Owned(name string) bool { return strings.HasPrefix(name, NamePrefix) }

// Leftover reports whether name belongs to an earlier run than runID: owned
// by the suite, but without the prefix of this run.
func Leftover(name, runID string) bool {
	return Owned(name) && !strings.HasPrefix(name, Prefix(runID))
}
