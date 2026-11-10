package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --no-clobber: anything at the target stops the command before the
// request (exit 2, no credit) and stays unchanged.
func TestPdfNoClobberRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "rechnung.pdf")
	if err := os.WriteFile(file, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "ordner")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	targets := []string{file, sub}
	link, dangling := filepath.Join(dir, "verweis.pdf"), filepath.Join(dir, "weg.pdf")
	if err := os.Symlink(file, link); err == nil {
		if err := os.Symlink(filepath.Join(dir, "fehlt.pdf"), dangling); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, link, dangling)
	}
	if runtime.GOOS != "windows" {
		targets = append(targets, os.DevNull)
	}
	for _, target := range targets {
		app, srv := apiApp(t)
		onPDF(srv, binaryPDF)
		wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", target, "--no-clobber", "-o", "json"}), 2)
		wantRequests(t, srv, 0)
		if out, errs := bufs(app); out != "" || !strings.Contains(errs, "--no-clobber nicht ersetzt") {
			t.Errorf("%s: stdout %q, stderr %q", target, out, errs)
		}
	}
	if got, _ := os.ReadFile(file); string(got) != "alt" {
		t.Errorf("Datei verändert: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "fehlt.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Ziel des Verweises angelegt: %v", err)
	}
	// stdout is no file: --no-clobber is a usage error there
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", "-", "--no-clobber"}), 2)
	wantRequests(t, srv, 0)
}

// A new target is written as without the option.
func TestPdfNoClobberWritesNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "neu.pdf")
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", path, "--no-clobber"}), 0)
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "neu.pdf")
}

// A target that appears during the download stays unchanged: exit 1, the
// temporary file is removed. Also without hard links (fallback).
func TestPdfNoClobberTargetAppears(t *testing.T) {
	for _, links := range []bool{true, false} {
		dir := t.TempDir()
		path := filepath.Join(dir, "neu.pdf")
		app, srv := apiApp(t)
		onPDF(srv, binaryPDF)
		prev := pdfTempCreated
		pdfTempCreated = func(string) {
			if err := os.WriteFile(path, []byte("fremd"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		prevLink := linkFile
		if !links {
			linkFile = func(string, string) error { return errors.New("keine harten Verweise") }
		}
		code := Run(app, []string{"invoice", "pdf", "459", "-O", path, "--no-clobber"})
		pdfTempCreated, linkFile = prev, prevLink
		wantExit(t, app, code, 1)
		if got, _ := os.ReadFile(path); string(got) != "fremd" {
			t.Errorf("links %v: Datei ersetzt: %q", links, got)
		}
		wantDirFiles(t, dir, "neu.pdf")
		if _, errs := bufs(app); !strings.Contains(errs, "während des Downloads") {
			t.Errorf("stderr %q", errs)
		}
	}
	// without hard links and without a target: the rename
	dir := t.TempDir()
	path := filepath.Join(dir, "neu.pdf")
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	prevLink := linkFile
	linkFile = func(string, string) error { return errors.New("keine harten Verweise") }
	code := Run(app, []string{"invoice", "pdf", "459", "-O", path, "--no-clobber"})
	linkFile = prevLink
	wantExit(t, app, code, 0)
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "neu.pdf")
}

// M4: when the temporary name cannot be removed after the link, discard
// removes it: no temporary file stays, the PDF is saved.
func TestPdfNoClobberRemovesTempAfterLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "neu.pdf")
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	prev := removeFile
	removeFile = func(string) error { return errors.New("belegt") }
	code := Run(app, []string{"invoice", "pdf", "459", "-O", path, "--no-clobber"})
	removeFile = prev
	wantExit(t, app, code, 0)
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
	wantDirFiles(t, dir, "neu.pdf")
}
