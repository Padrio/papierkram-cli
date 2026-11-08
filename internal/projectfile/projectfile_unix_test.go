//go:build unix

package projectfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Review I1: a checked-in link to /dev/zero or a FIFO must not exhaust
// the memory or block: a usage error, at once.
func TestLoad_DeviceAndFIFORejectedQuickly(t *testing.T) {
	zero := filepath.Join(t.TempDir(), FileName)
	if err := os.Symlink("/dev/zero", zero); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(t.TempDir(), FileName)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{zero, fifo} {
		done := make(chan error, 1)
		go func() { _, err := Load(p); done <- err }()
		select {
		case err := <-done:
			var ce *clierr.Error
			if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, "keine reguläre Datei") {
				t.Errorf("Load(%s) = %v", p, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Load(%s) blocks", p)
		}
	}
	// Find takes the FIFO like a file, and Load rejects it (no block)
	if got, ok := Find(filepath.Dir(fifo)); !ok || got != fifo {
		t.Errorf("Find = %q, %v", got, ok)
	}
}

// L81: a project file that is found but cannot be read is an error of the
// CLI (exit 1) whose hint names the file (printable) to check or remove.
func TestLoad_ReadErrorIsCLIError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files without read permission")
	}
	dir := filepath.Join(t.TempDir(), "repo\x1b[31m")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, []byte("project = 34\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	var ce *clierr.Error
	if !errors.As(err, &ce) || ce.Code != clierr.CodeError || ce.Exit() != 1 {
		t.Fatalf("Load = %#v", err)
	}
	shown := strings.ReplaceAll(p, "\x1b", " ")
	if want := "Projektdatei " + shown + " prüfen oder entfernen"; ce.Hint != want {
		t.Errorf("hint = %q, want %q", ce.Hint, want)
	}
	if !strings.Contains(ce.Message, "permission denied") || strings.Contains(ce.Message, "\x1b") || !errors.Is(err, os.ErrPermission) {
		t.Errorf("message = %q, cause = %v", ce.Message, ce.Cause)
	}
}

// Review T18-M1: Save of an existing file it cannot read returns the
// plain error (no *clierr.Error, which only Load builds), so local init
// reports it as "Projektdatei nicht geschrieben: …"; nothing is written.
func TestSave_ReadErrorStaysPlain(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files without read permission")
	}
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte("project = 34\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o600) })
	project := int64(35)
	res, err := Save(p, Changes{Project: &project})
	var ce *clierr.Error
	if err == nil || errors.As(err, &ce) || !errors.Is(err, os.ErrPermission) || res.Changed {
		t.Fatalf("Save = %+v, %#v", res, err)
	}
	if want := p + " lesen: permission denied"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(p); string(data) != "project = 34\n" {
		t.Errorf("file changed: %q", data)
	}
}
