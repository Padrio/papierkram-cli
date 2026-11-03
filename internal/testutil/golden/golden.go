// Package golden compares test output with files under the calling
// package's testdata/golden directory (spec 11, "Golden").
//
// Rewrite the files of one package with the -update flag, e.g.
//
//	go test ./internal/output -run TestTable -update
//
// Only run -update for packages that import this helper: go test rejects
// the unknown flag in all other packages (so not with ./...).
package golden

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "Golden-Dateien unter testdata/golden neu schreiben")

// tokenMarker is the prefix of every test token; golden files must never
// contain one.
const tokenMarker = "test-token-"

// Assert compares got with testdata/golden/<name> (relative to the package
// directory, the working directory of go test). With -update the file is
// (re)written instead. Output containing a test token fails before it is
// compared or written.
func Assert(t testing.TB, name string, got []byte) {
	t.Helper()
	if !filepath.IsLocal(name) { // also rejects "", absolute paths and ".."
		t.Fatalf("golden: ungültiger Name %q (relativer Pfad unter testdata/golden erwartet)", name)
		return
	}
	if bytes.Contains(got, []byte(tokenMarker)) {
		t.Fatalf("golden %s: Ausgabe enthält ein Test-Token (%s…); Tokens dürfen nie in Golden-Dateien stehen", name, tokenMarker)
		return
	}
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden: %v", err)
			return
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (anlegen mit -update)", filepath.ToSlash(path), err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden %s weicht ab (- erwartet, + erhalten; neu schreiben mit -update):%s\n%s",
			filepath.ToSlash(path), lineEndingHint(want, got), diff(want, got))
	}
}

// lineEndingHint explains a mismatch that consists only of CRLF vs. LF
// line endings (typically a git checkout with core.autocrlf). The bytes
// are deliberately not normalized: the mismatch stays a failure.
func lineEndingHint(want, got []byte) string {
	crlf, lf := []byte("\r\n"), []byte("\n")
	if bytes.Equal(bytes.ReplaceAll(want, crlf, lf), bytes.ReplaceAll(got, crlf, lf)) {
		return "\nZeilenenden unterscheiden sich (CRLF/LF) – .gitattributes prüfen (testdata muss byte-genau bleiben)"
	}
	return ""
}

// diff returns a line diff of want and got: unchanged lines with "  ",
// removed with "- ", added with "+ " (longest common subsequence).
func diff(want, got []byte) string {
	a, b := lines(want), lines(got)
	// lcs[i][j] is the LCS length of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var sb strings.Builder
	write := func(prefix, line string) {
		sb.WriteString(prefix)
		sb.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			sb.WriteString("\n\\ kein Zeilenumbruch am Ende\n")
		}
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			write("  ", a[i])
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			write("- ", a[i])
			i++
		default:
			write("+ ", b[j])
			j++
		}
	}
	return sb.String()
}

// lines splits s after each newline; the last line may lack one.
func lines(s []byte) []string {
	l := strings.SplitAfter(string(s), "\n")
	if l[len(l)-1] == "" {
		l = l[:len(l)-1]
	}
	return l
}
