package output

import (
	"strings"
	"unicode"
)

// Printable replaces control characters (line breaks, \r, tabs, the ESC
// of ANSI sequences) and invisible format characters (bidi overrides,
// zero-width spaces) with a space. Text from outside the CLI (API
// objects, a project file of a cloned repository, its path) goes through
// it before it reaches the terminal, so it cannot inject escape sequences
// or disguise what is shown.
func Printable(s string) string { return clean(s, false) }

// Message is Printable for messages that may span lines (validation
// lists): line breaks stay. WriteError and Warn run every message and hint
// through it, as defense in depth behind the filters of the callers.
func Message(s string) string { return clean(s, true) }

// clean is the one implementation of Printable and Message.
func clean(s string, keepNewline bool) string {
	return strings.Map(func(r rune) rune {
		if keepNewline && r == '\n' {
			return r
		}
		if Invisible(r) {
			return ' '
		}
		return r
	}, s)
}

// Invisible reports a control character or an invisible format character
// (Unicode Cf).
func Invisible(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}
