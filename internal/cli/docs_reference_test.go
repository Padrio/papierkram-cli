package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// referenceDir is the committed command reference (task 46), relative to
// the module root.
const referenceDir = "docs/reference"

// regenerateHint names the command that brings docs/reference up to date.
const regenerateHint = "neu erzeugen mit: make docs (go run ./cmd/papierkram gen-docs --dir docs/reference)"

// TestReferenceUpToDate: the committed docs/reference is exactly what
// gen-docs writes now: the same file names and the same bytes. The pages
// are regenerated into a temporary directory; the repository is only
// read.
func TestReferenceUpToDate(t *testing.T) {
	committed := filepath.Join(modroot.Find(t), filepath.FromSlash(referenceDir))
	fresh := filepath.Join(t.TempDir(), "reference")
	code, app := genDocs(t, "--dir", fresh)
	wantExit(t, app, code, 0)
	for _, p := range referenceDiff(t, committed, fresh) {
		t.Errorf("%s: %s", referenceDir, p)
	}
}

// referenceDiff compares the committed reference with a fresh one: files
// missing in committed, extra files there and files whose bytes differ
// (with the first differing line).
func referenceDiff(t *testing.T, committed, fresh string) []string {
	t.Helper()
	if _, err := os.Stat(committed); err != nil {
		return []string{fmt.Sprintf("fehlt (%v); %s", err, regenerateHint)}
	}
	have, want := dirNames(t, committed), dirNames(t, fresh)
	var out []string
	for _, name := range want {
		if !slices.Contains(have, name) {
			out = append(out, fmt.Sprintf("%s fehlt; %s", name, regenerateHint))
			continue
		}
		got, exp := readDoc(t, filepath.Join(committed, name)), readDoc(t, filepath.Join(fresh, name))
		if got != exp {
			out = append(out, fmt.Sprintf("%s veraltet (%s); %s", name, firstLineDiff(got, exp), regenerateHint))
		}
	}
	for _, name := range have {
		if !slices.Contains(want, name) {
			out = append(out, fmt.Sprintf("%s wird nicht mehr erzeugt (entfernen); %s", name, regenerateHint))
		}
	}
	return out
}

// firstLineDiff describes the first line where got and want differ.
func firstLineDiff(got, want string) string {
	g, w := strings.SplitAfter(got, "\n"), strings.SplitAfter(want, "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return fmt.Sprintf("Zeile %d: %q, erwartet %q", i+1, gl, wl)
		}
	}
	return "gleiche Zeilen"
}

// The comparison finds missing, extra and changed files, including a
// changed line ending, and names the command that regenerates them.
func TestReferenceDiffFindsEveryDifference(t *testing.T) {
	base := t.TempDir()
	committed, fresh := filepath.Join(base, "committed"), filepath.Join(base, "fresh")
	for dir, files := range map[string]map[string]string{
		fresh:     {"a.md": "x\ny\n", "b.md": "b\n", "c.md": "c\n", "d.md": "d\n"},
		committed: {"a.md": "x\nz\n", "c.md": "c\n", "d.md": "d\r\n", "alt.md": "alt\n"},
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	got := referenceDiff(t, committed, fresh)
	want := []string{
		`a.md veraltet (Zeile 2: "z\n", erwartet "y\n")`,
		"b.md fehlt",
		`d.md veraltet (Zeile 1: "d\r\n", erwartet "d\n")`,
		"alt.md wird nicht mehr erzeugt (entfernen)",
	}
	if len(got) != len(want) {
		t.Fatalf("Unterschiede = %q, want %q", got, want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]+"; ") || !strings.HasSuffix(got[i], regenerateHint) {
			t.Errorf("Unterschied %d = %q, want %q + Hinweis", i, got[i], want[i])
		}
	}
	if p := referenceDiff(t, filepath.Join(base, "nichts"), fresh); len(p) != 1 || !strings.Contains(p[0], regenerateHint) {
		t.Errorf("fehlendes Verzeichnis: %q", p)
	}
	if p := referenceDiff(t, fresh, fresh); len(p) != 0 {
		t.Errorf("gleiche Verzeichnisse: %q", p)
	}
}
