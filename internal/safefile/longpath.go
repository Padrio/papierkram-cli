package safefile

import "strings"

// maxShortPath is the length from which openRead on Windows gives a path
// the extended-length prefix, as fixLongPath of the os package does:
// MAX_PATH (260) less the 12 characters a directory must leave for a
// file name in 8.3 form.
const maxShortPath = 248

// extendedPath returns a Windows path of at least maxShortPath characters
// with the extended-length prefix (\\?\, for a UNC path \\?\UNC\), so
// CreateFile does not stop at MAX_PATH; the os package does the same for
// its own calls (Lstat, Remove), not for a syscall.CreateFile of its
// caller. The prefix turns off the normalization of the path, so full
// (filepath.Abs on Windows) first makes it absolute and clean: no "." or
// ".." elements, only backslashes. A shorter path, one that has a prefix
// already (\\?\, \??\ or the device namespace \\.\) or one full rejects
// is returned unchanged. It is plain string handling, testable on every
// platform.
func extendedPath(path string, full func(string) (string, error)) string {
	if len(path) < maxShortPath || hasNamespacePrefix(path) {
		return path
	}
	p, err := full(path)
	if err != nil {
		return path
	}
	switch {
	case hasNamespacePrefix(p):
		return p
	case strings.HasPrefix(p, `\\`):
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}

// hasNamespacePrefix reports a path in the Win32 file or device namespace
// (\\?\, \??\, \\.\), which needs no prefix.
func hasNamespacePrefix(p string) bool {
	return strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\??\`) || strings.HasPrefix(p, `\\.\`)
}
