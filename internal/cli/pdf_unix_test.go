//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A new PDF file gets 0666 minus the umask, like files of curl, wget or
// os.WriteFile: invoices stay private under umask 077.
func TestPdfNewFileRespectsUmask(t *testing.T) {
	for _, tc := range []struct {
		umask int
		want  os.FileMode
	}{
		{0o022, 0o644},
		{0o077, 0o600},
	} {
		old := syscall.Umask(tc.umask)
		app, srv := apiApp(t)
		onPDF(srv, binaryPDF)
		path := filepath.Join(t.TempDir(), "neu.pdf")
		code := Run(app, []string{"invoice", "pdf", "459", "-O", path})
		syscall.Umask(old)
		wantExit(t, app, code, 0)
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != tc.want {
			t.Errorf("umask %o: Modus = %v, erwartet %v (%v)", tc.umask, fi.Mode().Perm(), tc.want, err)
		}
	}
}

// A directory that cannot be searched is no missing directory: exit 1 with
// the system error, no request.
func TestPdfInaccessibleDirectoryIsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root umgeht Verzeichnisrechte")
	}
	outer := t.TempDir()
	inner := filepath.Join(outer, "ablage")
	if err := os.Mkdir(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outer, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outer, 0o700) })
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", filepath.Join(inner, "r.pdf")}), 1)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); strings.Contains(errs, "existiert nicht") || !strings.Contains(errs, "permission denied") {
		t.Errorf("stderr = %q", errs)
	}
}

// A device such as /dev/null is written directly, without a temporary
// file in /dev.
func TestPdfWritesToDevice(t *testing.T) {
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", os.DevNull}), 0)
	if _, errs := bufs(app); !strings.Contains(errs, "PDF gespeichert: "+os.DevNull+" ") {
		t.Errorf("stderr = %q", errs)
	}
}

// Replacing a private file never exposes the PDF: the temporary file has
// no group or other bits from its creation on, before any byte is written
// (umask 022 would otherwise allow 0644 until the chmod).
func TestPdfTempFileOfPrivateTargetStaysPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rechnung.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	var created []os.FileMode
	orig := pdfTempCreated
	pdfTempCreated = func(name string) {
		fi, err := os.Stat(name)
		if err != nil {
			t.Errorf("Temp-Datei: %v", err)
			return
		}
		if fi.Size() != 0 {
			t.Errorf("Temp-Datei hat schon %d Bytes", fi.Size())
		}
		created = append(created, fi.Mode().Perm())
	}
	t.Cleanup(func() { pdfTempCreated = orig })
	old := syscall.Umask(0o022)
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	code := Run(app, []string{"invoice", "pdf", "459", "-O", path})
	syscall.Umask(old)
	wantExit(t, app, code, 0)
	if len(created) != 1 || created[0]&0o077 != 0 {
		t.Errorf("Modus der Temp-Datei beim Anlegen: %v, erwartet ohne Gruppen-/Fremdrechte", created)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("Modus = %v", fi.Mode().Perm())
	}
}
