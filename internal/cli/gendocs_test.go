package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// genDocs runs papierkram gen-docs with args and returns the exit code.
func genDocs(t *testing.T, args ...string) (int, *App) {
	t.Helper()
	app, _, _ := newTestApp(t)
	return Run(app, append([]string{"gen-docs"}, args...)), app
}

// docNames are the files gen-docs must write: one per visible command
// (cobra/doc: available, not the help command) and the root.
func docNames(t *testing.T) (md, man []string) {
	t.Helper()
	app, _, _ := newTestApp(t)
	root := NewRootCmd(app)
	root.InitDefaultHelpCmd()
	walk(root, func(c *cobra.Command) {
		if c != root && (c.Hidden || hiddenParents(c) > 0 || c.Name() == "help") {
			return
		}
		md = append(md, strings.ReplaceAll(c.CommandPath(), " ", "_")+".md")
		man = append(man, strings.ReplaceAll(c.CommandPath(), " ", "-")+".1")
	})
	slices.Sort(md)
	slices.Sort(man)
	// every command of commands --json but help has its page
	for _, c := range commandsList(root) {
		if name := strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"; c.Name() != "help" && !slices.Contains(md, name) {
			t.Errorf("%s fehlt in der Erwartung", name)
		}
	}
	return md, man
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// gen-docs writes Markdown for every visible command and man pages
// (section 1) with the German help, into directories it creates.
func TestGenDocsWritesEveryVisibleCommand(t *testing.T) {
	base := t.TempDir()
	md, man := filepath.Join(base, "docs", "reference"), filepath.Join(base, "man")
	code, app := genDocs(t, "--dir", md, "--man", man)
	wantExit(t, app, code, 0)
	wantMD, wantMan := docNames(t)
	if got := dirNames(t, md); !slices.Equal(got, wantMD) {
		t.Errorf("Markdown\n got: %v\nwant: %v", got, wantMD)
	}
	if got := dirNames(t, man); !slices.Equal(got, wantMan) {
		t.Errorf("Man-Pages\n got: %v\nwant: %v", got, wantMan)
	}
	for _, name := range []string{"papierkram_completion.md", "papierkram_invoice_create.md", "papierkram.md"} {
		if !slices.Contains(wantMD, name) {
			t.Errorf("%s nicht erwartet", name)
		}
	}
	if slices.Contains(wantMD, "papierkram_gen-docs.md") {
		t.Error("versteckter Befehl gen-docs dokumentiert")
	}

	page := readDoc(t, filepath.Join(md, "papierkram_invoice_create.md"))
	for _, want := range []string{"# papierkram invoice create\n", "Rechnung anlegen", "Verwendung:", "Optionen:", "Globale Optionen:",
		"Beispiele:", "## Siehe auch", "[papierkram invoice](papierkram_invoice.md)"} {
		if !strings.Contains(page, want) {
			t.Errorf("Markdown ohne %q:\n%s", want, page)
		}
	}
	for _, bad := range []string{"Usage:", "Flags:", "Auto generated", "SEE ALSO", "(default"} {
		if strings.Contains(page, bad) {
			t.Errorf("Markdown enthält %q", bad)
		}
	}
	group := readDoc(t, filepath.Join(md, "papierkram_invoice.md"))
	if !strings.Contains(group, "[papierkram invoice create](papierkram_invoice_create.md)") {
		t.Errorf("Gruppe ohne Unterbefehl:\n%s", group)
	}

	mp := readDoc(t, filepath.Join(man, "papierkram-invoice-create.1"))
	for _, want := range []string{`.TH "PAPIERKRAM-INVOICE-CREATE" "1"`, "BEZEICHNUNG", "BESCHREIBUNG", "SIEHE AUCH", "Rechnung anlegen", "Globale Optionen:", "papierkram-invoice(1)"} {
		if !strings.Contains(mp, want) {
			t.Errorf("Man-Page ohne %q:\n%s", want, mp)
		}
	}
	if strings.Contains(mp, "Auto generated") || strings.Contains(mp, "SYNOPSIS") {
		t.Errorf("Man-Page mit englischen Teilen:\n%s", mp)
	}
	out, _ := bufs(app)
	if !strings.Contains(out, "Markdown") {
		t.Errorf("stdout %q", out)
	}
}

// Two runs write byte-identical files (no date, no generator tag).
func TestGenDocsDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if code, app := genDocs(t, "--dir", filepath.Join(a, "md"), "--man", filepath.Join(a, "man")); code != 0 {
		wantExit(t, app, code, 0)
	}
	if code, app := genDocs(t, "--dir", filepath.Join(b, "md"), "--man", filepath.Join(b, "man")); code != 0 {
		wantExit(t, app, code, 0)
	}
	for _, sub := range []string{"md", "man"} {
		names := dirNames(t, filepath.Join(a, sub))
		if !slices.Equal(names, dirNames(t, filepath.Join(b, sub))) {
			t.Fatalf("%s: Dateilisten weichen ab", sub)
		}
		for _, n := range names {
			if !bytes.Equal([]byte(readDoc(t, filepath.Join(a, sub, n))), []byte(readDoc(t, filepath.Join(b, sub, n)))) {
				t.Errorf("%s/%s weicht ab", sub, n)
			}
		}
	}
}

// Only one of the targets is enough; without any, or with arguments or
// --dry-run, gen-docs is a usage error.
func TestGenDocsUsage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "man")
	code, app := genDocs(t, "--man", dir)
	wantExit(t, app, code, 0)
	if len(dirNames(t, dir)) == 0 {
		t.Error("keine Man-Pages")
	}
	for _, args := range [][]string{{}, {"x", "--dir", t.TempDir()}, {"--dir", t.TempDir(), "--dry-run"}} {
		code, app := genDocs(t, args...)
		wantExit(t, app, code, 2)
	}
}

// gen-docs never writes through a symbolic link: a linked target directory
// is refused before anything is written, a link at a file's place is
// replaced, not followed; a file as target directory is refused.
func TestGenDocsRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolische Links brauchen Rechte")
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(t.TempDir(), "md")
	for _, args := range [][]string{{"--dir", link}, {"--dir", md, "--man", link}} {
		code, app := genDocs(t, args...)
		wantExit(t, app, code, 1)
		if _, errs := bufs(app); !strings.Contains(errs, "symbolischer Link") {
			t.Errorf("stderr %q", errs)
		}
	}
	if n := dirNames(t, target); len(n) != 0 {
		t.Errorf("durch den Link geschrieben: %v", n)
	}
	if _, err := os.Stat(md); err == nil {
		t.Error("--dir trotz abgelehntem --man angelegt")
	}

	file := filepath.Join(t.TempDir(), "datei")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, app := genDocs(t, "--dir", file)
	wantExit(t, app, code, 1)

	// a link where a page goes is replaced, its target stays untouched
	dir, outside := t.TempDir(), filepath.Join(t.TempDir(), "fremd.md")
	if err := os.WriteFile(outside, []byte("fremd"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "papierkram.md")); err != nil {
		t.Fatal(err)
	}
	code, app = genDocs(t, "--dir", dir)
	wantExit(t, app, code, 0)
	if readDoc(t, outside) != "fremd" {
		t.Error("Linkziel überschrieben")
	}
	if st, err := os.Lstat(filepath.Join(dir, "papierkram.md")); err != nil || !st.Mode().IsRegular() {
		t.Errorf("papierkram.md: %v, %v", st, err)
	}
}
