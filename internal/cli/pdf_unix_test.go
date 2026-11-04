//go:build unix

package cli

import (
	"context"
	"io"
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

// foreignNames makes ownedByMe report the files (or links) with these base
// names as owned by another user.
func foreignNames(t *testing.T, names ...string) {
	t.Helper()
	orig := ownedByMe
	ownedByMe = func(fi os.FileInfo) bool {
		for _, n := range names {
			if fi.Name() == n {
				return false
			}
		}
		return orig(fi)
	}
	t.Cleanup(func() { ownedByMe = orig })
}

// A file of another user is replaced like a new file: 0666 minus the
// umask, never its own (looser) mode, and no chmod at all.
func TestPdfForeignTargetGetsNewFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fremd.pdf")
	if err := os.WriteFile(path, []byte("alt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	foreignNames(t, "fremd.pdf")
	var created []os.FileMode
	origCreated, origChmod := pdfTempCreated, chmodFile
	pdfTempCreated = func(name string) {
		if fi, err := os.Stat(name); err == nil {
			created = append(created, fi.Mode().Perm())
		}
	}
	chmodFile = func(name string, mode os.FileMode) error {
		t.Errorf("chmod %s auf %v", name, mode)
		return nil
	}
	t.Cleanup(func() { pdfTempCreated, chmodFile = origCreated, origChmod })

	old := syscall.Umask(0o077)
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	code := Run(app, []string{"invoice", "pdf", "459", "-O", path})
	syscall.Umask(old)
	wantExit(t, app, code, 0)
	if len(created) != 1 || created[0] != 0o600 {
		t.Errorf("Modus der Temp-Datei: %v, erwartet 0600", created)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("Modus = %v, erwartet 0600", fi.Mode().Perm())
	}
	if got, _ := os.ReadFile(path); string(got) != binaryPDF {
		t.Errorf("Datei = %q", got)
	}
}

// A symbolic link is only followed when every link of the chain and the
// final file belong to the user; otherwise exit 2 before the request and
// nothing is written.
func TestPdfForeignSymlinkRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		foreign string
	}{
		{"fremder Verweis", "aktuell.pdf"},
		{"fremder Zwischenverweis", "zwischen.pdf"},
		{"fremdes Ziel", "rechnung.pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			real := filepath.Join(dir, "ablage", "rechnung.pdf")
			if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(real, []byte("alt"), 0o600); err != nil {
				t.Fatal(err)
			}
			// aktuell.pdf -> zwischen.pdf -> ablage/rechnung.pdf (relative)
			if err := os.Symlink(filepath.Join("ablage", "rechnung.pdf"), filepath.Join(dir, "zwischen.pdf")); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "aktuell.pdf")
			if err := os.Symlink("zwischen.pdf", link); err != nil {
				t.Fatal(err)
			}

			// all own: followed through the chain
			app, srv := apiApp(t)
			onPDF(srv, binaryPDF)
			wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", link}), 0)
			if got, _ := os.ReadFile(real); string(got) != binaryPDF {
				t.Fatalf("eigene Kette nicht gefolgt: %q", got)
			}
			if err := os.WriteFile(real, []byte("alt"), 0o600); err != nil {
				t.Fatal(err)
			}

			foreignNames(t, tc.foreign)
			app, srv = apiApp(t)
			wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", link}), 2)
			wantRequests(t, srv, 0)
			if _, errs := bufs(app); !strings.Contains(errs, "anderen Nutzer") {
				t.Errorf("stderr = %q", errs)
			}
			if got, _ := os.ReadFile(real); string(got) != "alt" {
				t.Errorf("Ziel geändert: %q", got)
			}
			wantDirFiles(t, filepath.Dir(real), "rechnung.pdf")
		})
	}
}

// Real ownership: the user's own files count as owned.
func TestOwnedByEUID(t *testing.T) {
	fi, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !ownedByEUID(fi) {
		t.Error("eigenes Verzeichnis gilt als fremd")
	}
}

// A FIFO is written directly; with a reader it gets the exact bytes.
// Without a reader the open blocks, and Ctrl+C ends the wait: exit 10.
func TestPdfFIFORespectsInterrupt(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "rohr")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("kein FIFO: %v", err)
	}

	// with a reader: the bytes arrive unchanged
	got := make(chan []byte, 1)
	go func() {
		f, err := os.Open(fifo)
		if err != nil {
			got <- nil
			return
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		got <- b
	}()
	app, srv := apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, Run(app, []string{"invoice", "pdf", "459", "-O", fifo}), 0)
	if b := <-got; string(b) != binaryPDF {
		t.Errorf("FIFO las %q", b)
	}

	// no reader: the interrupt arrives while the open waits
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orig := pdfDeviceOpening
	pdfDeviceOpening = cancel
	t.Cleanup(func() {
		pdfDeviceOpening = orig
		// release the writer that still waits for a reader
		if f, err := os.Open(fifo); err == nil {
			_, _ = io.Copy(io.Discard, f)
			_ = f.Close()
		}
	})
	app, srv = apiApp(t)
	onPDF(srv, binaryPDF)
	wantExit(t, app, executeContext(ctx, app, NewRootCmd(app), []string{"invoice", "pdf", "459", "-O", fifo}), 10)
	wantRequests(t, srv, 1)
	if _, errs := bufs(app); !strings.Contains(errs, "Abgebrochen") {
		t.Errorf("stderr = %q", errs)
	}
}
