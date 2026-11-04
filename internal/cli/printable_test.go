package cli

import "testing"

// Invisible format characters (bidi overrides, zero-width spaces) must not
// reach a confirmation prompt: U+202E would show "kunde@evil.com" reversed.
func TestPrintableReplacesFormatCharacters(t *testing.T) {
	cases := map[string]string{
		"a\u202Eb":    "a b",
		"a\u200Bb":    "a b",
		"x\x1b[31my":  "x [31my",
		"Müller GmbH": "Müller GmbH",
	}
	for in, want := range cases {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}
