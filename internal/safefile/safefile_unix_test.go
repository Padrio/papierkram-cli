//go:build unix

package safefile

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Review N1: a FIFO swapped in between the Stat and the open of
// readRegular must not block the open (O_NONBLOCK); the check on the
// handle then rejects it.
func TestOpenReadDoesNotBlockOnFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, follow := range []bool{true, false} {
		done := make(chan error, 1)
		go func() {
			f, err := openRead(fifo, follow)
			if err == nil {
				st, serr := f.Stat()
				if serr == nil && st.Mode().IsRegular() {
					err = errors.New("FIFO reported as regular file")
				}
				_ = f.Close()
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("follow=%v: %v", follow, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("follow=%v: openRead blocks on a FIFO", follow)
		}
	}
}
