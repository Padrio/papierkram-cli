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
