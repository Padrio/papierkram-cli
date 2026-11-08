package output

import (
	"strings"
	"unicode"
)

// Printable replaces control characters (line breaks, \r, tabs, the ESC
// of ANSI sequences), the Unicode line and paragraph separators and
// invisible format characters (bidi overrides, zero-width spaces) with a
// space; ZWJ, soft hyphen and BOM are removed
// (see clean). Text from outside the CLI (API
// objects, a project file of a cloned repository, its path) goes through
// it before it reaches the terminal, so it cannot inject escape sequences
// or disguise what is shown.
func Printable(s string) string { return clean(s, false) }

// Message is Printable for messages that may span lines (validation
// lists): line breaks stay. WriteError and Warn run every message and hint
// through it, as defense in depth behind the filters of the callers.
func Message(s string) string { return clean(s, true) }

// clean is the one implementation of Printable and Message. The zero
// width joiner (emoji sequences), the soft hyphen and the byte order mark
// disguise nothing and are removed, so a family emoji, a word or a name do
// not get a visible space (review N2); every other control or format
// character (bidi controls, zero width space) becomes a space.
func clean(s string, keepNewline bool) string {
	return strings.Map(func(r rune) rune {
		switch {
		case keepNewline && r == '\n':
			return r
		case r == '\u200d' || r == '\u00ad' || r == '\ufeff':
			return -1
		case Invisible(r):
			return ' '
		}
		return r
	}, s)
}

// Invisible reports a control character, an invisible format character
// (Unicode Cf) or a Unicode line or paragraph separator (U+2028, U+2029,
// categories Zl and Zp): some terminals break the line there like at \n.
func Invisible(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029'
}
