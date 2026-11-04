package e2eguard

import (
	"bytes"
	"errors"
	"regexp"
	"testing"
)

func TestRunIDFromGitHub(t *testing.T) {
	id, err := RunID(envOf(map[string]string{"GITHUB_RUN_ID": "1234567890"}), errReader{})
	if err != nil || id != "1234567890" {
		t.Errorf("RunID = %q, %v; want the GitHub run ID", id, err)
	}
}

// A re-run of a workflow keeps GITHUB_RUN_ID; the attempt keeps the
// prefixes of the attempts apart (leftovers of attempt 1 are leftovers for
// attempt 2).
func TestRunIDWithAttempt(t *testing.T) {
	for attempt, want := range map[string]string{"": "1234567890", "1": "1234567890", "2": "1234567890a2", "x": "1234567890", "-1": "1234567890"} {
		id, err := RunID(envOf(map[string]string{"GITHUB_RUN_ID": "1234567890", "GITHUB_RUN_ATTEMPT": attempt}), errReader{})
		if err != nil || id != want {
			t.Errorf("attempt %q: RunID = %q, %v; want %q", attempt, id, err, want)
		}
	}
}

// A GITHUB_RUN_ID that does not fit a name prefix is not used.
func TestRunIDIgnoresOddGitHubValue(t *testing.T) {
	for _, v := range []string{"", "12 34", "a-b", "x/y", "ÄÖ", "0123456789abcdef0123456789abcdef0"} {
		id, err := RunID(envOf(map[string]string{"GITHUB_RUN_ID": v}), bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02}))
		if err != nil || id != "deadbeef0102" {
			t.Errorf("GITHUB_RUN_ID %q: RunID = %q, %v; want random deadbeef0102", v, id, err)
		}
	}
}

func TestRunIDRandomFormat(t *testing.T) {
	id, err := RunID(envOf(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(id) {
		t.Errorf("RunID = %q, want 12 hex digits", id)
	}
	other, _ := RunID(envOf(nil), nil)
	if other == id {
		t.Errorf("two random run IDs are equal: %q", id)
	}
}

func TestRunIDRandomError(t *testing.T) {
	if _, err := RunID(envOf(nil), errReader{}); err == nil {
		t.Error("RunID without randomness: no error")
	}
}

func TestPrefix(t *testing.T) {
	if got := Prefix("abc123"); got != "pk-e2e-abc123-" {
		t.Errorf("Prefix = %q", got)
	}
}

// Cleanup touches only objects whose name starts with pk-e2e- (spec 11.1,
// controller ruling): exact, case-sensitive, at the start.
func TestOwned(t *testing.T) {
	for name, want := range map[string]bool{
		"pk-e2e-abc-kunde":      true,
		"pk-e2e-":               true,
		"pk-e2e":                false,
		"PK-E2E-abc-kunde":      false,
		" pk-e2e-abc":           false,
		"Kunde pk-e2e-abc":      false,
		"pk_e2e-abc":            false,
		"":                      false,
		"Mustermann GmbH":       false,
		"pk-e2e\u200b-abc-name": false,
	} {
		if got := Owned(name); got != want {
			t.Errorf("Owned(%q) = %v, want %v", name, got, want)
		}
	}
}

// A leftover belongs to an earlier run: owned, but not of this run.
func TestLeftover(t *testing.T) {
	for name, want := range map[string]bool{
		"pk-e2e-old1-kunde":  true,
		"pk-e2e-run1-kunde":  false,
		"pk-e2e-run10-kunde": true,
		"Mustermann GmbH":    false,
	} {
		if got := Leftover(name, "run1"); got != want {
			t.Errorf("Leftover(%q, run1) = %v, want %v", name, got, want)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("no randomness") }
