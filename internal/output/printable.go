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
func Printable(s string) string {
	return strings.Map(func(r rune) rune {
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
